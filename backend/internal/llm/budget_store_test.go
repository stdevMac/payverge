package llm

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

// requireBudgetPostgres opens an isolated Postgres database at the production
// schema (genesis baseline plus pending migrations). Skips when
// TEST_DATABASE_URL is unset so `go test ./internal/llm/...` stays green in
// environments without a database (same pattern as server pg tests).
func requireBudgetPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	if err != nil {
		t.Fatalf("start genesis postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	return pg.DB
}

// TestBudgetSchemaArtifacts pins the AI spend schema the budget store relies
// on: the reconciliation states, the durable controls table and the
// multi-scope reservation family link.
func TestBudgetSchemaArtifacts(t *testing.T) {
	db := requireBudgetPostgres(t)

	var definition string
	if err := db.Raw(`
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conname = 'ai_spend_reservations_status_check'
	`).Scan(&definition).Error; err != nil {
		t.Fatalf("read status constraint: %v", err)
	}
	if !strings.Contains(definition, "consumed") {
		t.Fatalf("status constraint does not permit consumed: %s", definition)
	}

	var controlsTable sql.NullString
	if err := db.Raw(`SELECT to_regclass('public.ai_budget_controls')`).Scan(&controlsTable).Error; err != nil {
		t.Fatalf("check controls table: %v", err)
	}
	if !controlsTable.Valid {
		t.Fatal("ai_budget_controls is missing")
	}

	var parent sql.NullString
	if err := db.Raw(`SELECT column_name FROM information_schema.columns
		WHERE table_name = 'ai_spend_reservations' AND column_name = 'parent_reservation_id'`).Scan(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if !parent.Valid {
		t.Fatal("ai_spend_reservations.parent_reservation_id is missing")
	}
}

func testReserveParams(biz uint, scope string, cap, amount int64, now time.Time) ReserveParams {
	return ReserveParams{
		ReservationID:        uuid.New().String(),
		BusinessID:           biz,
		FeatureScope:         scope,
		UsageDate:            UTCDate(now),
		CapMicroUSD:          cap,
		ConservativeMicroUSD: amount,
		Model:                "google/gemini-2.5-flash",
		InputTokens:          100,
		MaxOutputTokens:      256,
		ExpiresAt:            now.Add(2 * time.Minute),
		Now:                  now,
	}
}

func TestBudgetStore_ConcurrentReplicasAtCap(t *testing.T) {
	db := requireBudgetPostgres(t)
	storeA := NewBudgetStore(db)
	storeB := NewBudgetStore(db) // second replica / process
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)
	const cap int64 = 1_000_000 // $1.00

	var (
		wg       sync.WaitGroup
		okCount  atomic.Int64
		errCount atomic.Int64
	)
	// Two concurrent full-cap reservations — exactly one must win.
	for _, store := range []*BudgetStore{storeA, storeB} {
		wg.Add(1)
		go func(s *BudgetStore) {
			defer wg.Done()
			p := testReserveParams(42, "", cap, cap, now)
			err := s.Reserve(ctx, p)
			if err == nil {
				okCount.Add(1)
				return
			}
			if errors.Is(err, ErrBudgetExceeded) {
				errCount.Add(1)
				return
			}
			t.Errorf("unexpected reserve error: %v", err)
		}(store)
	}
	wg.Wait()

	if okCount.Load() != 1 {
		t.Fatalf("successful reserves = %d, want exactly 1", okCount.Load())
	}
	if errCount.Load() != 1 {
		t.Fatalf("budget-exceeded errors = %d, want exactly 1", errCount.Load())
	}

	fin, res, err := storeA.CommittedSpend(ctx, 42, "", UTCDate(now))
	if err != nil {
		t.Fatal(err)
	}
	if fin+res > cap {
		t.Fatalf("total spend %d exceeds cap %d (finalized=%d reserved=%d)", fin+res, cap, fin, res)
	}
	if fin+res != cap {
		t.Fatalf("committed spend = %d, want full cap %d reserved by winner", fin+res, cap)
	}
}

func TestBudgetStore_RestartSeesFinalizedSpend(t *testing.T) {
	db := requireBudgetPostgres(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	const cap int64 = 5_000_000

	store1 := NewBudgetStore(db)
	p := testReserveParams(7, "", cap, 1_500_000, now)
	if err := store1.Reserve(ctx, p); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store1.Finalize(ctx, FinalizeParams{
		ReservationID:  p.ReservationID,
		ActualMicroUSD: 1_200_000,
		ServedModel:    "google/gemini-2.5-flash",
		InputTokens:    80,
		OutputTokens:   40,
		Now:            now,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// New store instance (simulates process restart on another replica).
	store2 := NewBudgetStore(db)
	fin, res, err := store2.CommittedSpend(ctx, 7, "", UTCDate(now))
	if err != nil {
		t.Fatal(err)
	}
	if fin != 1_200_000 || res != 0 {
		t.Fatalf("after restart: finalized=%d reserved=%d, want finalized=1200000 reserved=0", fin, res)
	}

	// Remaining headroom is cap - finalized; a reserve that would exceed fails.
	p2 := testReserveParams(7, "", cap, 4_000_000, now)
	if err := store2.Reserve(ctx, p2); err == nil {
		t.Fatal("expected ErrBudgetExceeded after prior finalized spend, got nil")
	} else if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
	// Fits remaining headroom.
	p3 := testReserveParams(7, "", cap, 3_800_000, now)
	if err := store2.Reserve(ctx, p3); err != nil {
		t.Fatalf("reserve within remaining headroom: %v", err)
	}
}

func TestBudgetStore_UTCMidnightRollover(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	const cap int64 = 1_000_000

	day1 := time.Date(2026, 7, 10, 23, 30, 0, 0, time.UTC)
	p1 := testReserveParams(9, "", cap, cap, day1)
	if err := store.Reserve(ctx, p1); err != nil {
		t.Fatalf("day1 reserve: %v", err)
	}
	if err := store.Finalize(ctx, FinalizeParams{
		ReservationID:  p1.ReservationID,
		ActualMicroUSD: cap,
		ServedModel:    p1.Model,
		Now:            day1,
	}); err != nil {
		t.Fatalf("day1 finalize: %v", err)
	}

	// First instant of next UTC day has a fresh budget.
	day2 := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	p2 := testReserveParams(9, "", cap, cap, day2)
	if err := store.Reserve(ctx, p2); err != nil {
		t.Fatalf("day2 reserve after rollover: %v", err)
	}
	fin1, res1, _ := store.CommittedSpend(ctx, 9, "", UTCDate(day1))
	fin2, res2, _ := store.CommittedSpend(ctx, 9, "", UTCDate(day2))
	if fin1 != cap || res1 != 0 {
		t.Fatalf("day1 spend finalized=%d reserved=%d", fin1, res1)
	}
	if fin2 != 0 || res2 != cap {
		t.Fatalf("day2 spend finalized=%d reserved=%d, want reserved=cap", fin2, res2)
	}
}

func TestBudgetStore_ConservativeThenActualFinalize(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	const cap int64 = 10_000_000

	p := testReserveParams(11, "", cap, 2_000_000, now)
	if err := store.Reserve(ctx, p); err != nil {
		t.Fatal(err)
	}
	fin, res, _ := store.CommittedSpend(ctx, 11, "", UTCDate(now))
	if fin != 0 || res != 2_000_000 {
		t.Fatalf("after reserve: finalized=%d reserved=%d", fin, res)
	}

	if err := store.Finalize(ctx, FinalizeParams{
		ReservationID:  p.ReservationID,
		ActualMicroUSD: 750_000,
		ServedModel:    "google/gemini-2.5-flash",
		InputTokens:    50,
		OutputTokens:   20,
		Now:            now,
	}); err != nil {
		t.Fatal(err)
	}
	fin, res, _ = store.CommittedSpend(ctx, 11, "", UTCDate(now))
	if fin != 750_000 || res != 0 {
		t.Fatalf("after finalize: finalized=%d reserved=%d, want 750000 / 0", fin, res)
	}

	// Idempotent finalize.
	if err := store.Finalize(ctx, FinalizeParams{
		ReservationID:  p.ReservationID,
		ActualMicroUSD: 750_000,
		ServedModel:    "google/gemini-2.5-flash",
		Now:            now,
	}); err != nil {
		t.Fatalf("idempotent finalize: %v", err)
	}
	fin, res, _ = store.CommittedSpend(ctx, 11, "", UTCDate(now))
	if fin != 750_000 || res != 0 {
		t.Fatalf("after idempotent finalize: finalized=%d reserved=%d", fin, res)
	}
}

func TestBudgetStore_ReleaseOnProviderFailure(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC)
	const cap int64 = 5_000_000

	p := testReserveParams(13, "", cap, 3_000_000, now)
	if err := store.Reserve(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := store.Release(ctx, p.ReservationID, now); err != nil {
		t.Fatalf("release: %v", err)
	}
	fin, res, _ := store.CommittedSpend(ctx, 13, "", UTCDate(now))
	if fin != 0 || res != 0 {
		t.Fatalf("after release: finalized=%d reserved=%d, want 0/0", fin, res)
	}
	// Idempotent release.
	if err := store.Release(ctx, p.ReservationID, now); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	// Headroom fully restored.
	p2 := testReserveParams(13, "", cap, cap, now)
	if err := store.Reserve(ctx, p2); err != nil {
		t.Fatalf("re-reserve after release: %v", err)
	}
}

func TestBudgetStore_ExpireStaleGuardedIdempotent(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	const cap int64 = 4_000_000

	p := testReserveParams(15, "", cap, 2_000_000, now)
	p.ExpiresAt = now.Add(-time.Second) // already expired
	if err := store.Reserve(ctx, p); err != nil {
		t.Fatal(err)
	}

	n, err := store.ExpireStale(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ExpireStale recovered %d, want 1", n)
	}
	fin, res, _ := store.CommittedSpend(ctx, 15, "", UTCDate(now))
	if fin != 2_000_000 || res != 0 {
		t.Fatalf("after fail-closed expiry: finalized=%d reserved=%d, want conservative 2000000/0", fin, res)
	}

	// Second pass is a no-op (idempotent).
	n2, err := store.ExpireStale(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("second ExpireStale recovered %d, want 0", n2)
	}
	// Once fail-closed reconciliation finalized the conservative charge, a
	// delayed provider-failure callback must not release/erase that spend.
	if err := store.Release(ctx, p.ReservationID, now); err == nil {
		t.Fatal("release after stale reconciliation must fail")
	}
}

func TestBudgetStore_StaleConsumedReservationReconcilesRecordedActual(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	p := testReserveParams(16, "", 4_000_000, 2_000_000, now)
	p.ExpiresAt = now.Add(-time.Second)
	requireNoError(t, store.Reserve(ctx, p))
	requireNoError(t, store.RecordConsumed(ctx, FinalizeParams{
		ReservationID: p.ReservationID, ActualMicroUSD: 725_000,
		ServedModel: p.Model, InputTokens: 100, OutputTokens: 20, Now: now,
	}))

	n, err := store.ExpireStale(ctx, now)
	requireNoError(t, err)
	if n != 1 {
		t.Fatalf("reconciled = %d, want 1", n)
	}
	fin, reserved, err := store.CommittedSpend(ctx, 16, "", now)
	requireNoError(t, err)
	if fin != 725_000 || reserved != 0 {
		t.Fatalf("reconciled spend = %d/%d, want 725000/0", fin, reserved)
	}
}

func TestCallBudget_UnknownServedModelDurablyStopsReplicasAndKeepsConservativeCharge(t *testing.T) {
	db := requireBudgetPostgres(t)
	ctx := context.Background()
	storeA := NewBudgetStore(db)
	storeB := NewBudgetStore(db)
	budgetA := NewCallBudget(storeA, 10_000_000)
	budgetB := NewCallBudget(storeB, 10_000_000)
	req := GenerateRequest{
		Model: "google/gemini-2.5-flash", Feature: "director", BusinessID: 44,
		Messages: []Message{{Role: RoleUser, Text: "hello"}}, MaxTokens: 1000,
	}
	id, applied, err := budgetA.ReserveForRequest(ctx, req)
	requireNoError(t, err)
	if !applied {
		t.Fatal("expected reservation")
	}
	wantConservative, _ := EstimateCostMicroUSD(req.Model, EstimateRequestInputTokens(req), 0, req.MaxTokens)
	requireNoError(t, budgetA.FinalizeRequest(ctx, id, &Response{
		Model: "vendor/new-expensive-fallback",
		Usage: Usage{PromptTokens: 1, CompletionTokens: 1},
	}, req.Model))

	fin, reserved, err := storeA.CommittedSpend(ctx, 44, "", time.Now())
	requireNoError(t, err)
	if fin != wantConservative || reserved != 0 {
		t.Fatalf("unknown fallback charge = %d/%d, want conservative %d/0", fin, reserved, wantConservative)
	}
	_, _, err = budgetB.ReserveForRequest(ctx, req)
	if !errors.Is(err, ErrUnpricedModel) {
		t.Fatalf("second replica reserve error = %v, want ErrUnpricedModel", err)
	}
	budgetAfterRestart := NewCallBudget(NewBudgetStore(db), 10_000_000)
	_, _, err = budgetAfterRestart.ReserveForRequest(ctx, req)
	if !errors.Is(err, ErrUnpricedModel) {
		t.Fatalf("restarted replica reserve error = %v, want ErrUnpricedModel", err)
	}
	_, _, err = budgetAfterRestart.ReserveForRequest(ctx, GenerateRequest{
		Model: "google/gemini-2.5-flash", Feature: "future_global_surface",
	})
	if !errors.Is(err, ErrUnpricedModel) {
		t.Fatalf("uncapped/unknown surface bypassed durable shutdown: %v", err)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBudgetStore_BusinessVsInstanceScopeIndependent(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	const bizCap int64 = 1_000_000
	const instanceCap int64 = 500_000

	// Exhaust business total scope for business 1.
	pBiz := testReserveParams(1, "", bizCap, bizCap, now)
	if err := store.Reserve(ctx, pBiz); err != nil {
		t.Fatalf("business reserve: %v", err)
	}
	// The instance-wide scope on synthetic business 0 is independent.
	pCon := testReserveParams(0, BudgetScopeGlobal, instanceCap, instanceCap, now)
	if err := store.Reserve(ctx, pCon); err != nil {
		t.Fatalf("instance reserve: %v", err)
	}

	// Filling the instance scope must not free business budget, and vice versa.
	pBiz2 := testReserveParams(1, "", bizCap, 1, now)
	if err := store.Reserve(ctx, pBiz2); err == nil || !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("business should still be over cap, err=%v", err)
	}
	pCon2 := testReserveParams(0, BudgetScopeGlobal, instanceCap, 1, now)
	if err := store.Reserve(ctx, pCon2); err == nil || !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("instance scope should still be over cap, err=%v", err)
	}
	// Unrelated business still has budget.
	pOther := testReserveParams(2, "", bizCap, bizCap, now)
	if err := store.Reserve(ctx, pOther); err != nil {
		t.Fatalf("unrelated business: %v", err)
	}
}

func TestBudgetStore_UnpricedConfiguredFallbackFailsClosed(t *testing.T) {
	// Does not require Postgres — pricing table is the fail-closed gate.
	cfg := ModelConfig{
		Chat:          "google/gemini-2.5-flash",
		Menu:          "google/gemini-2.5-flash",
		Image:         "google/gemini-2.5-flash-image",
		Director:      "google/gemini-2.5-flash",
		Guardrail:     "google/gemini-2.5-flash",
		ChatFallbacks: []string{"totally/unknown-unpriced-model"},
	}
	err := ValidateConfiguredModelsPriced(cfg)
	if err == nil {
		t.Fatal("expected fail-closed error for unpriced configured fallback")
	}
	if !errors.Is(err, ErrUnpricedModel) {
		t.Fatalf("want ErrUnpricedModel, got %v", err)
	}
}

func TestBudgetStore_ReserveIdempotentOnSameKey(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC)
	const cap int64 = 2_000_000

	p := testReserveParams(21, "", cap, 1_000_000, now)
	if err := store.Reserve(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Same reservation id must not double-count reserved.
	if err := store.Reserve(ctx, p); err != nil {
		t.Fatalf("idempotent reserve: %v", err)
	}
	fin, res, _ := store.CommittedSpend(ctx, 21, "", UTCDate(now))
	if fin != 0 || res != 1_000_000 {
		t.Fatalf("after idempotent reserve: finalized=%d reserved=%d", fin, res)
	}
}

func TestDollarsToMicroUSD(t *testing.T) {
	if got := DollarsToMicroUSD(1.0); got != 1_000_000 {
		t.Fatalf("DollarsToMicroUSD(1) = %d", got)
	}
	if got := DollarsToMicroUSD(0.01); got != 10_000 {
		t.Fatalf("DollarsToMicroUSD(0.01) = %d", got)
	}
	if got := DollarsToMicroUSD(10.5); got != 10_500_000 {
		t.Fatalf("DollarsToMicroUSD(10.5) = %d", got)
	}
}

func TestCallBudget_DoesNotAttributeAnonymousGlobalCallToBusinessZero(t *testing.T) {
	b := NewCallBudget(&BudgetStore{}, 1_000_000).WithGlobalCap(500_000)
	scopes := b.scopesFor(GenerateRequest{Feature: "ai", BusinessID: 0})
	if len(scopes) != 1 || scopes[0] != (ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: 500_000}) {
		t.Fatalf("anonymous platform call key = %+v", scopes)
	}
	_, _, err := b.ReserveForRequest(context.Background(), GenerateRequest{
		Feature: "director", Model: "google/gemini-2.5-flash", BusinessID: 0,
	})
	if !errors.Is(err, ErrMissingBusinessID) {
		t.Fatalf("restaurant request with missing key error = %v, want ErrMissingBusinessID", err)
	}
	_, _, err = b.ReserveForRequest(context.Background(), GenerateRequest{
		Feature: "guardrail", Model: "google/gemini-2.5-flash", BusinessID: 0,
	})
	if !errors.Is(err, ErrMissingBusinessID) {
		t.Fatalf("restaurant guardrail with missing key error = %v, want ErrMissingBusinessID", err)
	}
}

func TestEstimateCostMicroUSD_Priced(t *testing.T) {
	// 1M input @ $0.30/M + 200k output @ $2.50/M = $0.80 = 800_000 micro
	micro, ok := EstimateCostMicroUSD("google/gemini-2.5-flash", 1_000_000, 0, 200_000)
	if !ok {
		t.Fatal("expected priced")
	}
	if micro != 800_000 {
		t.Fatalf("micro = %d, want 800000", micro)
	}
	_, ok = EstimateCostMicroUSD("totally/unknown", 1000, 0, 1000)
	if ok {
		t.Fatal("unknown model must not be priced")
	}
}
