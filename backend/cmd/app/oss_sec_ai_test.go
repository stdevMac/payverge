package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func budgetEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func collectWarnings(warnings *[]string) func(string, ...any) {
	return func(f string, a ...any) { *warnings = append(*warnings, fmt.Sprintf(f, a...)) }
}

func TestResolveAIBudgetConfig_DefaultsAreMandatory(t *testing.T) {
	var warnings []string
	warn := collectWarnings(&warnings)
	want := aiBudgetConfig{PerBusinessUSD: 5, GuestPerBusinessUSD: 2.5, GlobalUSD: 20, GuestPoolUSD: 10}

	got := resolveAIBudgetConfig(budgetEnv(nil), warn)
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
	if len(warnings) != 0 {
		t.Fatalf("unset values must not warn: %v", warnings)
	}
	if got.OwnerReserveUSD() != 10 {
		t.Fatalf("default owner reserve = %.2f, want 10", got.OwnerReserveUSD())
	}

	// Zero, negative, NaN and garbage cannot disable a ceiling.
	got = resolveAIBudgetConfig(budgetEnv(map[string]string{
		"AI_DAILY_BUDGET_USD":          "0",
		"AI_BUDGET_GLOBAL_USD_DAY":     "-3",
		"AI_BUDGET_GUEST_POOL_USD_DAY": "off",
	}), warn)
	if got != want {
		t.Fatalf("invalid values must fall back to defaults, got %+v", got)
	}
	if len(warnings) != 3 {
		t.Fatalf("want one warning per rejected value, got %v", warnings)
	}
}

func TestResolveAIBudgetConfig_OverridesAndAlias(t *testing.T) {
	noWarn := func(f string, a ...any) { t.Fatalf("unexpected warning: "+f, a...) }
	got := resolveAIBudgetConfig(budgetEnv(map[string]string{
		"AI_DAILY_BUDGET_USD":          "2.5",
		"AI_BUDGET_GLOBAL_USD_DAY":     "100",
		"AI_BUDGET_GUEST_POOL_USD_DAY": "40",
	}), noWarn)
	if got != (aiBudgetConfig{PerBusinessUSD: 2.5, GuestPerBusinessUSD: 2.5, GlobalUSD: 100, GuestPoolUSD: 40}) {
		t.Fatalf("overrides = %+v", got)
	}
	got = resolveAIBudgetConfig(budgetEnv(map[string]string{"AI_BUDGET_PER_BUSINESS_USD_DAY": "7"}), noWarn)
	if got.PerBusinessUSD != 7 {
		t.Fatalf("alias ignored: %+v", got)
	}
	got = resolveAIBudgetConfig(budgetEnv(map[string]string{
		"AI_DAILY_BUDGET_USD": "3", "AI_BUDGET_PER_BUSINESS_USD_DAY": "7",
	}), noWarn)
	if got.PerBusinessUSD != 3 {
		t.Fatalf("canonical name must win over alias: %+v", got)
	}
}

// A larger per-business budget must never be silently truncated by the
// default instance-wide cap: unset values derive from the ones above them.
func TestResolveAIBudgetConfig_DerivesCoherentDefaults(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want aiBudgetConfig
	}{
		{map[string]string{"AI_DAILY_BUDGET_USD": "50"},
			aiBudgetConfig{PerBusinessUSD: 50, GuestPerBusinessUSD: 12.5, GlobalUSD: 100, GuestPoolUSD: 50}},
		{map[string]string{"AI_DAILY_BUDGET_USD": "2"},
			aiBudgetConfig{PerBusinessUSD: 2, GuestPerBusinessUSD: 2, GlobalUSD: 20, GuestPoolUSD: 10}},
		{map[string]string{"AI_BUDGET_GLOBAL_USD_DAY": "8"},
			aiBudgetConfig{PerBusinessUSD: 5, GuestPerBusinessUSD: 1, GlobalUSD: 8, GuestPoolUSD: 4}},
		{map[string]string{"AI_BUDGET_GLOBAL_USD_DAY": "200", "AI_BUDGET_GUEST_POOL_USD_DAY": "150"},
			aiBudgetConfig{PerBusinessUSD: 5, GuestPerBusinessUSD: 5, GlobalUSD: 200, GuestPoolUSD: 150}},
	}
	for _, tc := range cases {
		var warnings []string
		got := resolveAIBudgetConfig(budgetEnv(tc.env), collectWarnings(&warnings))
		if got != tc.want {
			t.Errorf("env %v: got %+v, want %+v", tc.env, got, tc.want)
		}
		if len(warnings) != 0 {
			t.Errorf("env %v: coherent config must not warn: %v", tc.env, warnings)
		}
		if got.OwnerReserveUSD() <= 0 {
			t.Errorf("env %v: derived caps must keep an owner reserve: %+v", tc.env, got)
		}
	}
}

// F1R: no single venue's guests (anonymous visitors or a hostile tenant) may
// be able to close guest AI at every other venue. With every configuration the
// share one business's guests can spend must leave pool headroom after
// guestPoolMinVenues-1 venues are exhausted, and owners never draw from the
// guest pool.
func TestResolveAIBudgetConfig_NoSingleVenueClosesGuestPool(t *testing.T) {
	envs := []map[string]string{
		nil,
		{"AI_DAILY_BUDGET_USD": "10"}, // round-2 review: perBiz == derived pool/1
		{"AI_DAILY_BUDGET_USD": "50"},
		{"AI_DAILY_BUDGET_USD": "5", "AI_BUDGET_GUEST_POOL_USD_DAY": "5"},
		{"AI_DAILY_BUDGET_USD": "5", "AI_BUDGET_GUEST_POOL_USD_DAY": "7", "AI_BUDGET_GLOBAL_USD_DAY": "40"},
		{"AI_BUDGET_GUEST_POOL_USD_DAY": "500"}, // clamped to global
	}
	for _, env := range envs {
		cfg := resolveAIBudgetConfig(budgetEnv(env), func(string, ...any) {})
		if cfg.GuestPerBusinessUSD > cfg.PerBusinessUSD {
			t.Errorf("env %v: guest share %.2f above the per-business cap %.2f", env, cfg.GuestPerBusinessUSD, cfg.PerBusinessUSD)
		}
		if exhausted := float64(guestPoolMinVenues-1) * cfg.GuestPerBusinessUSD; exhausted >= cfg.GuestPoolUSD {
			t.Errorf("env %v: %d venues at their full share ($%.2f) already exhaust the guest pool $%.2f",
				env, guestPoolMinVenues-1, exhausted, cfg.GuestPoolUSD)
		}
		w := buildAIBudgetWiring(&llm.BudgetStore{}, cfg)
		if w.Owner.GuestPoolCapMicroUSD() != 0 {
			t.Errorf("env %v: only the guest gate may be bound by the guest pool", env)
		}
	}
}

func TestResolveAIBudgetConfig_WarnsOnIncoherentExplicitCaps(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		want     aiBudgetConfig
		contains string
	}{
		{"global below one business", map[string]string{"AI_DAILY_BUDGET_USD": "50", "AI_BUDGET_GLOBAL_USD_DAY": "30"},
			aiBudgetConfig{PerBusinessUSD: 50, GuestPerBusinessUSD: 3.75, GlobalUSD: 30, GuestPoolUSD: 15}, "AI_BUDGET_GLOBAL_USD_DAY=$30.00 is below"},
		{"guest pool above global is clamped", map[string]string{"AI_BUDGET_GUEST_POOL_USD_DAY": "25"},
			aiBudgetConfig{PerBusinessUSD: 5, GuestPerBusinessUSD: 5, GlobalUSD: 20, GuestPoolUSD: 20}, "clamping the guest pool"},
		{"guest pool equal to global", map[string]string{"AI_BUDGET_GUEST_POOL_USD_DAY": "20"},
			aiBudgetConfig{PerBusinessUSD: 5, GuestPerBusinessUSD: 5, GlobalUSD: 20, GuestPoolUSD: 20}, "deny owners"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var warnings []string
			got := resolveAIBudgetConfig(budgetEnv(tc.env), collectWarnings(&warnings))
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.contains) {
				t.Fatalf("want exactly one warning containing %q, got %q", tc.contains, warnings)
			}
		})
	}
}

func TestBuildAIBudgetWiring_EveryGateIsCapped(t *testing.T) {
	w := buildAIBudgetWiring(&llm.BudgetStore{}, aiBudgetConfig{
		PerBusinessUSD: 5, GuestPerBusinessUSD: 2.5, GlobalUSD: 20, GuestPoolUSD: 10,
	})
	if w.Owner.CapMicroUSD() != 5_000_000 || w.Owner.FeatureScope() != llm.BudgetScopeOwner {
		t.Fatalf("owner gate = %d/%q", w.Owner.CapMicroUSD(), w.Owner.FeatureScope())
	}
	if w.Guest.CapMicroUSD() != 2_500_000 || w.Guest.FeatureScope() != llm.BudgetScopeGuest {
		t.Fatalf("guest gate = %d/%q", w.Guest.CapMicroUSD(), w.Guest.FeatureScope())
	}
	for name, g := range map[string]*llm.AICostGate{"owner": w.Owner, "guest": w.Guest} {
		if g.GlobalCapMicroUSD() != 20_000_000 {
			t.Errorf("%s gate global cap = %d", name, g.GlobalCapMicroUSD())
		}
	}
	// Only guests draw from the guest pool: owners keep their reserve.
	if w.Guest.GuestPoolCapMicroUSD() != 10_000_000 {
		t.Fatalf("guest pool cap on the guest gate = %d", w.Guest.GuestPoolCapMicroUSD())
	}
	if w.Owner.GuestPoolCapMicroUSD() != 0 {
		t.Fatalf("owner gate must not be bound by the guest pool: %d", w.Owner.GuestPoolCapMicroUSD())
	}
	cb := w.CallBudget
	if !cb.Active() || cb.BusinessCapMicro != 5_000_000 || cb.GuestCapMicro != 2_500_000 || cb.GlobalCapMicro != 20_000_000 ||
		cb.GuestPoolCapMicro != 10_000_000 {
		t.Fatalf("call budget = %+v", cb)
	}
}
