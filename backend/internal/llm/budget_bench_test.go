package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

// openBudgetBenchDB is the testing.B twin of requireBudgetPostgres: an
// isolated database at the production schema.
func openBudgetBenchDB(b *testing.B) *gorm.DB {
	b.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		b.Skip("TEST_DATABASE_URL not set; skipping Postgres budget benchmark")
	}
	pg, err := genesisdb.Start(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	return pg.DB
}

func benchReserveFinalize(b *testing.B, budget *CallBudget) {
	ctx := context.Background()
	req := GenerateRequest{
		Model: "google/gemini-2.5-flash", Feature: "waiter", BusinessID: 77, MaxTokens: 256,
		System: "You are a waiter.", Messages: []Message{{Role: RoleUser, Text: "Is the soup vegan?"}},
	}
	resp := &Response{Model: "google/gemini-2.5-flash", Usage: Usage{PromptTokens: 40, CompletionTokens: 30}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, applied, err := budget.ReserveForRequest(ctx, req)
		if err != nil || !applied {
			b.Fatalf("reserve applied=%v err=%v", applied, err)
		}
		if err := budget.FinalizeRequest(ctx, id, resp, req.Model); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCallBudgetReserveFinalize measures one guarded provider call's
// ledger round trip (reserve before, finalize after) on Postgres.
func BenchmarkCallBudgetReserveFinalize(b *testing.B) {
	db := openBudgetBenchDB(b)
	const bigCap = int64(1) << 50 // never trips; we measure the ledger cost
	b.Run("business_scope", func(b *testing.B) {
		benchReserveFinalize(b, NewCallBudget(NewBudgetStore(db), bigCap))
	})
	b.Run("business_plus_global_scope", func(b *testing.B) {
		benchReserveFinalize(b, NewCallBudget(NewBudgetStore(db), bigCap).WithGlobalCap(bigCap))
	})
	// Guest turn with the guest-pool sub-cap: three locked rows.
	b.Run("guest_plus_pool_plus_global_scope", func(b *testing.B) {
		benchReserveFinalize(b, NewCallBudget(NewBudgetStore(db), bigCap).WithGlobalCap(bigCap).WithGuestPoolCap(bigCap))
	})
}

// seedBenchGateSpend creates today's ledger rows for business 77's guest
// scope and the business-0 "global" / "guest_pool" scopes, so the gate benchmark
// reads existing rows (the steady state on a live instance).
func seedBenchGateSpend(b *testing.B, store *BudgetStore) {
	b.Helper()
	const bigCap = int64(1) << 50
	now := time.Now().UTC()
	if err := store.Reserve(context.Background(), ReserveParams{
		BusinessID: 77, FeatureScope: BudgetScopeGuest, CapMicroUSD: bigCap,
		ExtraScopes: []ScopeCap{
			{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: bigCap},
			{BusinessID: 0, FeatureScope: BudgetScopeGuestPool, CapMicroUSD: bigCap},
		},
		ConservativeMicroUSD: 1_000, UsageDate: UTCDate(now), Now: now,
	}); err != nil {
		b.Fatal(err)
	}
}

func benchGateOverBudget(b *testing.B, gate *AICostGate) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if gate.OverBudget(77) {
			b.Fatal("gate must have headroom")
		}
	}
}

// BenchmarkAICostGateOverBudget measures the dollar pre-check every guest AI
// turn runs (guestAIOverDollarBudget -> AICostGate.OverBudget) on Postgres.
func BenchmarkAICostGateOverBudget(b *testing.B) {
	db := openBudgetBenchDB(b)
	store := NewBudgetStore(db)
	seedBenchGateSpend(b, store)
	b.Run("guest_plus_global", func(b *testing.B) {
		benchGateOverBudget(b, NewDurableFeatureCostGate(store, 1e6, BudgetScopeGuest).WithGlobalCap(1e6))
	})
	b.Run("guest_plus_pool_plus_global", func(b *testing.B) {
		benchGateOverBudget(b, NewDurableFeatureCostGate(store, 1e6, BudgetScopeGuest).WithGuestPoolCap(1e6).WithGlobalCap(1e6))
	})
}
