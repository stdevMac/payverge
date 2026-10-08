package main

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// AI spend ceilings (open-source release, H-ai-cost). Whenever an LLM provider
// is configured every ceiling is mandatory: an unset, unparseable or
// non-positive value falls back to the default instead of disabling the cap,
// in every mode (not only production), so a fresh self-hosted install can never
// run unbounded model spend.
//
// Scope layout per UTC day (defaults in brackets):
//
//	global [$20] (instance-wide, every lane)
//	├── guest pool [$10]: guest waiter turns (web + WhatsApp) at every business
//	│   └── each business's guests: at most min(per-business, pool / 4) [$2.50]
//	└── owner reserve = global - pool [$10]: owner/staff lanes only
//
// Each business additionally has its own owner/staff scope capped at the
// per-business amount [$5]. Because one business's guests can take at most a
// quarter of the pool, no single venue (anonymous visitors or a hostile
// tenant) can close guest AI at the other venues: that takes at least guestPoolMinVenues venues
// spending their full share on the same day.
const (
	// AI_DAILY_BUDGET_USD (existing name; AI_BUDGET_PER_BUSINESS_USD_DAY is
	// accepted as an alias): per business per UTC day for the owner/staff
	// scope, and the upper bound of the business's guest share.
	envAIBudgetPerBusiness      = "AI_DAILY_BUDGET_USD"
	envAIBudgetPerBusinessAlias = "AI_BUDGET_PER_BUSINESS_USD_DAY"
	// AI_BUDGET_GLOBAL_USD_DAY: instance-wide ceiling across every lane.
	envAIBudgetGlobal = "AI_BUDGET_GLOBAL_USD_DAY"
	// AI_BUDGET_GUEST_POOL_USD_DAY: sub-cap inside global shared by guest
	// waiter turns at every business.
	envAIBudgetGuestPool = "AI_BUDGET_GUEST_POOL_USD_DAY"

	defaultAIBudgetPerBusinessUSD = 5.0
	// minDefaultAIBudgetGlobalUSD is the floor of the derived global default
	// max($20, 2 x per-business): an unset global never truncates a single
	// business's owner + guest allowance.
	minDefaultAIBudgetGlobalUSD = 20.0
	// guestPoolMinVenues is how many businesses' guests must each spend their
	// full share before the guest pool closes for every venue: a business's
	// guest share is at most pool / guestPoolMinVenues.
	guestPoolMinVenues = 4
)

// aiBudgetConfig is the resolved set of daily USD ceilings.
type aiBudgetConfig struct {
	PerBusinessUSD      float64 // owner/staff scope, per business
	GuestPerBusinessUSD float64 // guest scope, per business: min(PerBusinessUSD, GuestPoolUSD / guestPoolMinVenues)
	GlobalUSD           float64
	GuestPoolUSD        float64
}

// OwnerReserveUSD is the part of the global cap no anonymous lane can spend.
func (c aiBudgetConfig) OwnerReserveUSD() float64 {
	return c.GlobalUSD - c.GuestPoolUSD
}

// resolveAIBudgetConfig reads the ceilings from getenv. Unset values are
// derived from the ones above them so the defaults are always coherent:
//
//	per-business = $5
//	global       = max($20, 2 x per-business)
//	guest pool   = global / 2
//	guest share  = min(per-business, guest pool / 4)   (never configured directly)
//	owner reserve = global - guest pool                (>= global / 2 by default)
//
// warn receives one line per value that was present but rejected, and one per
// explicit combination that makes a lower cap unreachable or leaves owners no
// reserve.
func resolveAIBudgetConfig(getenv func(string) string, warn func(format string, args ...any)) aiBudgetConfig {
	perBizName := envAIBudgetPerBusiness
	if strings.TrimSpace(getenv(perBizName)) == "" && strings.TrimSpace(getenv(envAIBudgetPerBusinessAlias)) != "" {
		perBizName = envAIBudgetPerBusinessAlias
	}
	perBiz, _ := positiveUSD(getenv, perBizName, defaultAIBudgetPerBusinessUSD, warn)

	global, globalSet := positiveUSD(getenv, envAIBudgetGlobal, math.Max(minDefaultAIBudgetGlobalUSD, 2*perBiz), warn)

	pool, poolSet := positiveUSD(getenv, envAIBudgetGuestPool, global/2, warn)
	if poolSet && pool > global {
		warn("%s=$%.2f exceeds %s=$%.2f; clamping the guest pool to the global cap, which leaves owners no reserved headroom",
			envAIBudgetGuestPool, pool, envAIBudgetGlobal, global)
		pool = global
	} else if pool >= global {
		warn("%s ($%.2f) reaches %s ($%.2f): guests can exhaust the whole instance budget and deny owners; lower it to keep an owner reserve",
			envAIBudgetGuestPool, pool, envAIBudgetGlobal, global)
	}

	guestShare := math.Min(perBiz, pool/guestPoolMinVenues)
	if globalSet && global < perBiz+guestShare {
		warn("%s=$%.2f is below one business's combined owner + guest allowance ($%.2f + $%.2f); the instance-wide cap will refuse AI before a single busy business reaches its own caps",
			envAIBudgetGlobal, global, perBiz, guestShare)
	}

	return aiBudgetConfig{
		PerBusinessUSD:      perBiz,
		GuestPerBusinessUSD: guestShare,
		GlobalUSD:           global,
		GuestPoolUSD:        pool,
	}
}

// positiveUSD parses name as a positive dollar amount. It returns def (and
// explicit=false) when the variable is unset or rejected.
func positiveUSD(getenv func(string) string, name string, def float64, warn func(string, ...any)) (value float64, explicit bool) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return def, false
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		warn("%s=%q is not a positive dollar amount; AI spend caps cannot be disabled, using default $%.2f/day", name, raw, def)
		return def, false
	}
	return f, true
}

// aiBudgetWiring is what main.go needs back from wireAIBudgets.
type aiBudgetWiring struct {
	Config     aiBudgetConfig
	Owner      *llm.AICostGate // owner/staff lanes (director, ops assistant, wizard, ...)
	Guest      *llm.AICostGate // guest lanes (AI waiter web + WhatsApp); also bound by the guest pool
	CallBudget *llm.CallBudget
}

// wireAIBudgets builds the durable dollar gates and the provider-path
// reserver from the environment and installs the server-side gates.
func wireAIBudgets(store *llm.BudgetStore) aiBudgetWiring {
	cfg := resolveAIBudgetConfig(os.Getenv, logger.Logger.Warnf)
	w := buildAIBudgetWiring(store, cfg)
	server.SetAICostGate(w.Owner)
	server.SetGuestAICostGate(w.Guest)
	logger.Logger.Infof("AI daily spend caps: $%.2f instance-wide; $%.2f per business for owner/staff tools; guest pool $%.2f shared by every venue's guests, at most $%.2f per venue (%d venues to exhaust it); owner reserve $%.2f",
		cfg.GlobalUSD, cfg.PerBusinessUSD, cfg.GuestPoolUSD, cfg.GuestPerBusinessUSD,
		int(math.Ceil(cfg.GuestPoolUSD/cfg.GuestPerBusinessUSD)), cfg.OwnerReserveUSD())
	return w
}

func buildAIBudgetWiring(store *llm.BudgetStore, cfg aiBudgetConfig) aiBudgetWiring {
	return aiBudgetWiring{
		Config: cfg,
		Owner:  llm.NewDurableAICostGate(store, cfg.PerBusinessUSD).WithGlobalCap(cfg.GlobalUSD),
		Guest: llm.NewDurableFeatureCostGate(store, cfg.GuestPerBusinessUSD, llm.BudgetScopeGuest).
			WithGuestPoolCap(cfg.GuestPoolUSD).WithGlobalCap(cfg.GlobalUSD),
		CallBudget: llm.NewCallBudget(store,
			llm.DollarsToMicroUSD(cfg.PerBusinessUSD),
		).WithGuestCap(llm.DollarsToMicroUSD(cfg.GuestPerBusinessUSD)).
			WithGlobalCap(llm.DollarsToMicroUSD(cfg.GlobalUSD)).
			WithGuestPoolCap(llm.DollarsToMicroUSD(cfg.GuestPoolUSD)),
	}
}
