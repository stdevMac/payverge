package llm

import (
	"context"
	"time"
)

// AICostGate enforces a per-business (or feature-scoped) daily dollar ceiling
// across AI lanes. Authorization reads the durable BudgetStore when configured;
// DailyCostRollup is metrics-only and is only consulted when no store is wired
// (unit tests / legacy uncapped-local paths).
//
// A nil gate or capUSD<=0 disables enforcement (no-op), so AI keeps working when
// no cap is configured. The message-count budget remains the cheap first line;
// this is the dollar backstop the count-cap cannot provide.
type AICostGate struct {
	store  *BudgetStore
	rollup *DailyCostRollup // metrics / test fallback only — never authorizes when store is set
	capUSD float64
	// capMicro is the durable integer ceiling (micro-USD). Derived from capUSD.
	capMicro int64
	// feature, when non-empty, scopes the ceiling to a single feature label
	// (e.g. "guest") instead of the owner scope ("").
	feature string
	// globalCapMicro, when > 0, also refuses once the instance-wide scope
	// (business 0, BudgetScopeGlobal) is spent — every lane shares it.
	globalCapMicro int64
	// guestPoolCapMicro, when > 0, also refuses once the instance-wide guest
	// pool (business 0, BudgetScopeGuestPool) shared by guest turns at every
	// business is spent. Owner gates leave it unset so guests
	// cannot deny them.
	guestPoolCapMicro int64
}

// NewAICostGate builds a gate over the running cost rollup (test/metrics path).
// capUSD<=0 disables it.
func NewAICostGate(rollup *DailyCostRollup, capUSD float64) *AICostGate {
	return &AICostGate{
		rollup:   rollup,
		capUSD:   capUSD,
		capMicro: DollarsToMicroUSD(capUSD),
	}
}

// NewFeatureCostGate builds a gate whose ceiling counts only the named feature's
// spend for the queried business id. capUSD<=0 disables it.
func NewFeatureCostGate(rollup *DailyCostRollup, capUSD float64, feature string) *AICostGate {
	return &AICostGate{
		rollup:   rollup,
		capUSD:   capUSD,
		capMicro: DollarsToMicroUSD(capUSD),
		feature:  feature,
	}
}

// NewDurableAICostGate builds a business-total gate backed by BudgetStore.
func NewDurableAICostGate(store *BudgetStore, capUSD float64) *AICostGate {
	return &AICostGate{
		store:    store,
		capUSD:   capUSD,
		capMicro: DollarsToMicroUSD(capUSD),
	}
}

// NewDurableFeatureCostGate builds a feature-scoped gate backed by BudgetStore.
func NewDurableFeatureCostGate(store *BudgetStore, capUSD float64, feature string) *AICostGate {
	return &AICostGate{
		store:    store,
		capUSD:   capUSD,
		capMicro: DollarsToMicroUSD(capUSD),
		feature:  feature,
	}
}

// WithGlobalCap makes OverBudget also report true once the instance-wide
// daily scope reaches capUSD (<=0 disables the global check).
func (g *AICostGate) WithGlobalCap(capUSD float64) *AICostGate {
	if g == nil {
		return nil
	}
	g.globalCapMicro = DollarsToMicroUSD(capUSD)
	return g
}

// WithGuestPoolCap makes OverBudget also report true once the instance-wide
// guest pool reaches capUSD (<=0 disables the pool check). Only the guest
// gate should set it.
func (g *AICostGate) WithGuestPoolCap(capUSD float64) *AICostGate {
	if g == nil {
		return nil
	}
	g.guestPoolCapMicro = DollarsToMicroUSD(capUSD)
	return g
}

// CapMicroUSD returns the configured ceiling in micro-USD (0 = disabled).
func (g *AICostGate) CapMicroUSD() int64 {
	if g == nil {
		return 0
	}
	return g.capMicro
}

// GuestPoolCapMicroUSD returns the guest pool sub-cap in micro-USD (0 = not checked).
func (g *AICostGate) GuestPoolCapMicroUSD() int64 {
	if g == nil {
		return 0
	}
	return g.guestPoolCapMicro
}

// GlobalCapMicroUSD returns the instance-wide cap in micro-USD (0 = not checked).
func (g *AICostGate) GlobalCapMicroUSD() int64 {
	if g == nil {
		return 0
	}
	return g.globalCapMicro
}

// FeatureScope returns the gate's feature scope ("" = business total).
func (g *AICostGate) FeatureScope() string {
	if g == nil {
		return ""
	}
	return g.feature
}

// OverBudget reports whether the business has reached its daily USD ceiling.
// With a BudgetStore: uses finalized+reserved micro-USD for today's UTC date.
// Store read errors fail closed (treat as over budget). Without a store: falls
// back to the in-memory rollup for tests.
func (g *AICostGate) OverBudget(businessID uint) bool {
	if g == nil || (g.capMicro <= 0 && g.globalCapMicro <= 0 && g.guestPoolCapMicro <= 0) {
		return false
	}
	if g.store != nil {
		var checks [3]ScopeCap
		n := 0
		if g.capMicro > 0 {
			checks[n] = ScopeCap{BusinessID: businessID, FeatureScope: g.feature, CapMicroUSD: g.capMicro}
			n++
		}
		if g.guestPoolCapMicro > 0 {
			checks[n] = ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGuestPool, CapMicroUSD: g.guestPoolCapMicro}
			n++
		}
		if g.globalCapMicro > 0 {
			checks[n] = ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: g.globalCapMicro}
			n++
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// One round trip for every scope (was one query per scope).
		spent, err := g.store.CommittedSpendScopes(ctx, checks[:n], UTCDate(time.Now().UTC()))
		if err != nil {
			// Fail closed on read errors: cannot confirm headroom.
			return true
		}
		for i, sc := range checks[:n] {
			if spent[i] >= sc.CapMicroUSD {
				return true
			}
		}
		return false
	}
	if g.capMicro <= 0 {
		return false
	}
	// Legacy / unit-test path: rollup is metrics-shaped float USD.
	if g.rollup == nil || g.capUSD <= 0 {
		return false
	}
	if g.feature != "" {
		return g.rollup.RunningFeatureUSD(businessID, g.feature) >= g.capUSD
	}
	return g.rollup.RunningTotalUSD(businessID) >= g.capUSD
}
