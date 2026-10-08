package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- scope selection (no database) -------------------------------------------

func TestCallBudget_ScopesSplitGuestOwnerAndAddGlobal(t *testing.T) {
	b := NewCallBudget(&BudgetStore{}, 5_000_000).WithGlobalCap(20_000_000)
	global := ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: 20_000_000}
	cases := []struct {
		name string
		req  GenerateRequest
		want []ScopeCap
	}{
		{"guest waiter", GenerateRequest{Feature: "waiter", BusinessID: 7},
			[]ScopeCap{{7, BudgetScopeGuest, 5_000_000}, global}},
		{"guest whatsapp", GenerateRequest{Feature: "waiter_whatsapp", BusinessID: 7},
			[]ScopeCap{{7, BudgetScopeGuest, 5_000_000}, global}},
		{"guest guardrail by audience", GenerateRequest{Feature: "guardrail", BusinessID: 7, BudgetAudience: BudgetAudienceGuest},
			[]ScopeCap{{7, BudgetScopeGuest, 5_000_000}, global}},
		{"owner guardrail", GenerateRequest{Feature: "guardrail", BusinessID: 7},
			[]ScopeCap{{7, BudgetScopeOwner, 5_000_000}, global}},
		{"owner director", GenerateRequest{Feature: "director", BusinessID: 7},
			[]ScopeCap{{7, BudgetScopeOwner, 5_000_000}, global}},
		{"explicit owner overrides guest feature", GenerateRequest{Feature: "waiter", BusinessID: 7, BudgetAudience: BudgetAudienceOwner},
			[]ScopeCap{{7, BudgetScopeOwner, 5_000_000}, global}},
		{"unattributed platform call hits global only", GenerateRequest{Feature: "ai"},
			[]ScopeCap{global}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := b.scopesFor(tc.req)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("scopesFor = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCallBudget_NoGlobalCapKeepsSingleScope(t *testing.T) {
	b := NewCallBudget(&BudgetStore{}, 5_000_000)
	if got := b.scopesFor(GenerateRequest{Feature: "waiter", BusinessID: 3}); len(got) != 1 || got[0].FeatureScope != BudgetScopeGuest {
		t.Fatalf("scopes = %+v", got)
	}
	if got := b.scopesFor(GenerateRequest{Feature: "ai"}); len(got) != 0 {
		t.Fatalf("an unattributed call with no global cap must be uncapped, got %+v", got)
	}
	if !NewCallBudget(&BudgetStore{}, 0).WithGlobalCap(1).Active() {
		t.Fatal("a global cap alone must activate the budget")
	}
}

func TestReservationScopes_SortsAndRejectsBadInput(t *testing.T) {
	p := ReserveParams{BusinessID: 9, FeatureScope: BudgetScopeGuest, CapMicroUSD: 10,
		ExtraScopes: []ScopeCap{{0, BudgetScopeGlobal, 20}}}
	got, err := reservationScopes(p)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].BusinessID != 0 || got[1].BusinessID != 9 {
		t.Fatalf("scopes must lock in (business_id, scope) order, got %+v", got)
	}
	p.ExtraScopes = []ScopeCap{{9, BudgetScopeGuest, 10}}
	if _, err := reservationScopes(p); err == nil {
		t.Fatal("duplicate scope must be rejected")
	}
	p.ExtraScopes = []ScopeCap{{0, BudgetScopeGlobal, 0}}
	if _, err := reservationScopes(p); err == nil {
		t.Fatal("non-positive extra cap must be rejected")
	}
}

func TestAICostGate_GlobalCeilingNilSafe(t *testing.T) {
	var g *AICostGate
	if g.WithGlobalCap(1) != nil {
		t.Fatal("nil gate must stay nil")
	}
	if g.OverBudget(1) {
		t.Fatal("nil gate must not block")
	}
}

// --- Postgres integration ----------------------------------------------------

func multiScopeParams(biz uint, scope string, bizCap, globalCap, amount int64, now time.Time) ReserveParams {
	p := testReserveParams(biz, scope, bizCap, amount, now)
	p.ExtraScopes = []ScopeCap{{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: globalCap}}
	return p
}

func committed(t *testing.T, s *BudgetStore, biz uint, scope string, now time.Time) (int64, int64) {
	t.Helper()
	fin, res, err := s.CommittedSpend(context.Background(), biz, scope, UTCDate(now))
	if err != nil {
		t.Fatal(err)
	}
	return fin, res
}

func TestBudgetStore_GlobalScopeBoundsEveryBusiness(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	const bizCap, globalCap int64 = 1_000_000, 1_500_000

	if err := store.Reserve(ctx, multiScopeParams(1, BudgetScopeOwner, bizCap, globalCap, 1_000_000, now)); err != nil {
		t.Fatalf("first business: %v", err)
	}
	// Business 2 has its own per-business headroom but the instance is spent.
	err := store.Reserve(ctx, multiScopeParams(2, BudgetScopeOwner, bizCap, globalCap, 600_000, now))
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("global ceiling must refuse business 2, err=%v", err)
	}
	// The refused reservation must not leave a partial hold on any scope.
	if fin, res := committed(t, store, 2, BudgetScopeOwner, now); fin+res != 0 {
		t.Fatalf("refused reservation leaked into business scope: fin=%d res=%d", fin, res)
	}
	if fin, res := committed(t, store, 0, BudgetScopeGlobal, now); fin+res != 1_000_000 {
		t.Fatalf("global committed = %d, want 1000000", fin+res)
	}
	if err := store.Reserve(ctx, multiScopeParams(2, BudgetScopeOwner, bizCap, globalCap, 500_000, now)); err != nil {
		t.Fatalf("remaining global headroom: %v", err)
	}
}

func TestBudgetStore_GuestAndOwnerScopesAreIndependent(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC)
	const bizCap, globalCap int64 = 1_000_000, 10_000_000

	if err := store.Reserve(ctx, multiScopeParams(5, BudgetScopeGuest, bizCap, globalCap, bizCap, now)); err != nil {
		t.Fatal(err)
	}
	if err := store.Reserve(ctx, multiScopeParams(5, BudgetScopeGuest, bizCap, globalCap, 1, now)); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("guest scope should be exhausted, err=%v", err)
	}
	// Guests burning their scope must not lock the owner out of their tools.
	if err := store.Reserve(ctx, multiScopeParams(5, BudgetScopeOwner, bizCap, globalCap, bizCap, now)); err != nil {
		t.Fatalf("owner scope must be independent of guest spend: %v", err)
	}
}

func TestBudgetStore_MultiScopeFinalizeReleaseAndExpire(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC)
	const bizCap, globalCap int64 = 5_000_000, 5_000_000

	// Finalize: the actual charge lands in both scopes, holds drop to zero.
	p := multiScopeParams(11, BudgetScopeGuest, bizCap, globalCap, 400_000, now)
	if err := store.Reserve(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := store.Finalize(ctx, FinalizeParams{ReservationID: p.ReservationID, ActualMicroUSD: 150_000, Now: now}); err != nil {
		t.Fatal(err)
	}
	for _, sc := range []ScopeCap{{11, BudgetScopeGuest, 0}, {0, BudgetScopeGlobal, 0}} {
		if fin, res := committed(t, store, sc.BusinessID, sc.FeatureScope, now); fin != 150_000 || res != 0 {
			t.Fatalf("after finalize %d/%q: fin=%d res=%d", sc.BusinessID, sc.FeatureScope, fin, res)
		}
	}
	// Finalize is idempotent across the family.
	if err := store.Finalize(ctx, FinalizeParams{ReservationID: p.ReservationID, ActualMicroUSD: 150_000, Now: now}); err != nil {
		t.Fatalf("idempotent finalize: %v", err)
	}
	if fin, _ := committed(t, store, 0, BudgetScopeGlobal, now); fin != 150_000 {
		t.Fatalf("double finalize double-charged global: %d", fin)
	}

	// Release: every scope gets its headroom back.
	p2 := multiScopeParams(11, BudgetScopeGuest, bizCap, globalCap, 300_000, now)
	if err := store.Reserve(ctx, p2); err != nil {
		t.Fatal(err)
	}
	if err := store.Release(ctx, p2.ReservationID, now); err != nil {
		t.Fatal(err)
	}
	for _, sc := range []ScopeCap{{11, BudgetScopeGuest, 0}, {0, BudgetScopeGlobal, 0}} {
		if _, res := committed(t, store, sc.BusinessID, sc.FeatureScope, now); res != 0 {
			t.Fatalf("after release %d/%q reserved=%d", sc.BusinessID, sc.FeatureScope, res)
		}
	}

	// ExpireStale: a crashed replica's hold is charged conservatively in both scopes.
	p3 := multiScopeParams(11, BudgetScopeGuest, bizCap, globalCap, 200_000, now)
	if err := store.Reserve(ctx, p3); err != nil {
		t.Fatal(err)
	}
	n, err := store.ExpireStale(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expired rows = %d, want primary + global child", n)
	}
	for _, sc := range []ScopeCap{{11, BudgetScopeGuest, 0}, {0, BudgetScopeGlobal, 0}} {
		if fin, res := committed(t, store, sc.BusinessID, sc.FeatureScope, now); fin != 350_000 || res != 0 {
			t.Fatalf("after expire %d/%q: fin=%d res=%d", sc.BusinessID, sc.FeatureScope, fin, res)
		}
	}
	// A late Finalize on the expired family is a no-op, never a double charge.
	if err := store.Finalize(ctx, FinalizeParams{ReservationID: p3.ReservationID, ActualMicroUSD: 1, Now: now}); err != nil {
		t.Fatalf("late finalize after expiry: %v", err)
	}
	if fin, _ := committed(t, store, 0, BudgetScopeGlobal, now); fin != 350_000 {
		t.Fatalf("late finalize changed global: %d", fin)
	}
}

func TestBudgetStore_ConcurrentMultiScopeNeverOvershootsGlobal(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC)
	const bizCap, globalCap, amount int64 = 10_000_000, 1_000_000, 100_000

	var wg sync.WaitGroup
	var ok, refused atomic.Int64
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scope := BudgetScopeOwner
			if i%2 == 0 {
				scope = BudgetScopeGuest
			}
			err := store.Reserve(ctx, multiScopeParams(uint(1+i%4), scope, bizCap, globalCap, amount, now))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrBudgetExceeded):
				refused.Add(1)
			default:
				t.Errorf("reserve: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() != globalCap/amount || refused.Load() != 40-globalCap/amount {
		t.Fatalf("ok=%d refused=%d, want exactly %d admitted", ok.Load(), refused.Load(), globalCap/amount)
	}
	if fin, res := committed(t, store, 0, BudgetScopeGlobal, now); fin+res != globalCap {
		t.Fatalf("global committed = %d, want %d", fin+res, globalCap)
	}
}

func TestAICostGate_DurableGlobalCeiling(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	now := time.Now().UTC()
	gate := NewDurableFeatureCostGate(store, 5, BudgetScopeGuest).WithGlobalCap(1)
	if gate.OverBudget(8) {
		t.Fatal("fresh day must not be over budget")
	}
	// Another business spends the whole instance ceiling.
	if err := store.Reserve(context.Background(), multiScopeParams(9, BudgetScopeOwner, 5_000_000, 1_000_000, 1_000_000, now)); err != nil {
		t.Fatal(err)
	}
	if !gate.OverBudget(8) {
		t.Fatal("global ceiling spent elsewhere must close business 8's guest gate")
	}
	if NewDurableFeatureCostGate(store, 5, BudgetScopeGuest).OverBudget(8) {
		t.Fatal("without the global cap the per-business guest gate is still open")
	}
}

// --- guest pool sub-cap ---------------------------------------------------------

func TestCallBudget_GuestPoolChargesGuestTurnsOnly(t *testing.T) {
	b := NewCallBudget(&BudgetStore{}, 5_000_000).
		WithGuestCap(2_500_000).
		WithGlobalCap(20_000_000).
		WithGuestPoolCap(10_000_000)
	global := ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: 20_000_000}
	pool := ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGuestPool, CapMicroUSD: 10_000_000}
	guest := ScopeCap{BusinessID: 7, FeatureScope: BudgetScopeGuest, CapMicroUSD: 2_500_000}
	owner := ScopeCap{BusinessID: 7, FeatureScope: BudgetScopeOwner, CapMicroUSD: 5_000_000}
	cases := []struct {
		name string
		req  GenerateRequest
		want []ScopeCap
	}{
		{"guest waiter", GenerateRequest{Feature: "waiter", BusinessID: 7}, []ScopeCap{guest, pool, global}},
		{"guest whatsapp", GenerateRequest{Feature: "waiter_whatsapp", BusinessID: 7}, []ScopeCap{guest, pool, global}},
		{"guest guardrail by audience", GenerateRequest{Feature: "guardrail", BusinessID: 7, BudgetAudience: BudgetAudienceGuest},
			[]ScopeCap{guest, pool, global}},
		{"owner director never charges the pool", GenerateRequest{Feature: "director", BusinessID: 7}, []ScopeCap{owner, global}},
		{"owner guardrail never charges the pool", GenerateRequest{Feature: "guardrail", BusinessID: 7}, []ScopeCap{owner, global}},
		{"explicit owner audience on a guest feature", GenerateRequest{Feature: "waiter", BusinessID: 7, BudgetAudience: BudgetAudienceOwner},
			[]ScopeCap{owner, global}},
		{"unattributed platform call", GenerateRequest{Feature: "ai"}, []ScopeCap{global}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := b.scopesFor(tc.req)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("scopesFor = %+v, want %+v", got, tc.want)
			}
			if _, err := reservationScopes(ReserveParams{
				BusinessID: got[0].BusinessID, FeatureScope: got[0].FeatureScope,
				CapMicroUSD: got[0].CapMicroUSD, ExtraScopes: got[1:],
			}); err != nil {
				t.Fatalf("scopes must form a valid reservation family: %v", err)
			}
		})
	}
	if !NewCallBudget(&BudgetStore{}, 0).WithGuestPoolCap(1).Active() {
		t.Fatal("a guest pool cap alone must activate the budget")
	}
	var nilBudget *CallBudget
	if nilBudget.WithGuestPoolCap(1) != nil || nilBudget.WithGuestCap(1) != nil {
		t.Fatal("nil budget must stay nil")
	}
}

func TestCallBudget_GuestCapFallsBackToBusinessCap(t *testing.T) {
	waiter := GenerateRequest{Feature: "waiter", BusinessID: 3}
	director := GenerateRequest{Feature: "director", BusinessID: 3}
	if got := NewCallBudget(&BudgetStore{}, 5_000_000).scopesFor(waiter); fmt.Sprint(got) != fmt.Sprint([]ScopeCap{{3, BudgetScopeGuest, 5_000_000}}) {
		t.Fatalf("no guest cap: guest scope must use the business cap, got %+v", got)
	}
	b := NewCallBudget(&BudgetStore{}, 0).WithGuestCap(1)
	if !b.Active() {
		t.Fatal("a guest cap alone must activate the budget")
	}
	if got := b.scopesFor(waiter); fmt.Sprint(got) != fmt.Sprint([]ScopeCap{{3, BudgetScopeGuest, 1}}) {
		t.Fatalf("guest-only cap: got %+v", got)
	}
	if got := b.scopesFor(director); len(got) != 0 {
		t.Fatalf("a guest cap must not cap owner calls, got %+v", got)
	}
}

func TestAICostGate_GuestPoolCapNilSafeAndFailsClosed(t *testing.T) {
	var g *AICostGate
	if g.WithGuestPoolCap(1) != nil {
		t.Fatal("nil gate must stay nil")
	}
	// A store that cannot be read must refuse rather than admit spend.
	if !NewDurableFeatureCostGate(&BudgetStore{}, 0, BudgetScopeGuest).WithGuestPoolCap(1).OverBudget(1) {
		t.Fatal("pool-only gate over an unreadable store must fail closed")
	}
}

// reserveVia reserves amount against exactly the scopes CallBudget would pick
// for req, so the test exercises scope selection and ledger enforcement together.
func reserveVia(t *testing.T, b *CallBudget, req GenerateRequest, amount int64, now time.Time) error {
	t.Helper()
	_, err := reserveViaID(t, b, req, amount, now)
	return err
}

func reserveViaID(t *testing.T, b *CallBudget, req GenerateRequest, amount int64, now time.Time) (string, error) {
	t.Helper()
	scopes := b.scopesFor(req)
	if len(scopes) == 0 {
		t.Fatalf("request %+v selected no scopes", req)
	}
	p := testReserveParams(scopes[0].BusinessID, scopes[0].FeatureScope, scopes[0].CapMicroUSD, amount, now)
	p.ExtraScopes = scopes[1:]
	return p.ReservationID, b.Store.Reserve(context.Background(), p)
}

func TestBudgetStore_GuestPoolKeepsOwnerReserve(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	now := time.Now().UTC()
	const perBiz, guestShare, pool, global int64 = 5_000_000, 1_000_000, 3_000_000, 5_000_000
	b := NewCallBudget(store, perBiz).WithGuestCap(guestShare).WithGlobalCap(global).WithGuestPoolCap(pool)

	// Guests of N different businesses each spend their full share; together
	// they exhaust the guest pool.
	const businesses = 4
	admitted := 0
	for biz := uint(1); biz <= businesses; biz++ {
		err := reserveVia(t, b, GenerateRequest{Feature: "waiter", BusinessID: biz}, guestShare, now)
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, ErrBudgetExceeded):
		default:
			t.Fatalf("guest reserve biz %d: %v", biz, err)
		}
	}
	if admitted != int(pool/guestShare) {
		t.Fatalf("admitted %d guest turns, want exactly %d under the guest pool", admitted, pool/guestShare)
	}
	guestGate := NewDurableFeatureCostGate(store, 1, BudgetScopeGuest).WithGuestPoolCap(3).WithGlobalCap(5)
	if !guestGate.OverBudget(businesses + 1) {
		t.Fatal("guest pre-check must close for a fresh business once the guest pool is spent")
	}
	// The owner reserve (global - pool) is untouched.
	ownerGate := NewDurableAICostGate(store, 5).WithGlobalCap(5)
	if ownerGate.OverBudget(1) {
		t.Fatal("owner pre-check must stay open: guests cannot spend the owner reserve")
	}
	if err := reserveVia(t, b, GenerateRequest{Feature: "director", BusinessID: 1}, global-pool, now); err != nil {
		t.Fatalf("owner reservation must fit in the owner reserve: %v", err)
	}
	if fin, res := committed(t, store, 0, BudgetScopeGuestPool, now); fin+res != pool {
		t.Fatalf("guest pool committed = %d, want %d (owner calls must not charge it)", fin+res, pool)
	}
	if fin, res := committed(t, store, 0, BudgetScopeGlobal, now); fin+res != global {
		t.Fatalf("global committed = %d, want %d", fin+res, global)
	}
}

// F1R (round-2 review): with the default caps (cmd/app resolveAIBudgetConfig:
// $5 per business, guest share $2.50, guest pool $10, global $20) one venue's
// guests spending their full allowance must not refuse another venue's
// guests. It takes
// guestPoolMinVenues (4) venues at their full share to close the pool, and
// even then owners keep their reserve.
func TestBudgetStore_DefaultCapsOneVenueCannotCloseOtherVenues(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	now := time.Now().UTC()
	const perBiz, guestShare, pool, global int64 = 5_000_000, 2_500_000, 10_000_000, 20_000_000
	b := NewCallBudget(store, perBiz).WithGuestCap(guestShare).WithGlobalCap(global).WithGuestPoolCap(pool)
	waiter := func(biz uint) GenerateRequest { return GenerateRequest{Feature: "waiter", BusinessID: biz} }

	// Venue A's guests (or a hostile tenant) spend everything they can.
	if err := reserveVia(t, b, waiter(1), guestShare, now); err != nil {
		t.Fatalf("venue A guest share: %v", err)
	}
	if err := reserveVia(t, b, waiter(1), 1, now); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("venue A must stop at its guest share, err=%v", err)
	}
	guestGate := NewDurableFeatureCostGate(store, 2.5, BudgetScopeGuest).WithGuestPoolCap(10).WithGlobalCap(20)
	if guestGate.OverBudget(2) {
		t.Fatal("venue B's guest pre-check must stay open")
	}
	probeID, err := reserveViaID(t, b, waiter(2), 1, now)
	if err != nil {
		t.Fatalf("venue B's guests must not be refused after venue A: %v", err)
	}
	if err := store.Release(context.Background(), probeID, now); err != nil {
		t.Fatal(err)
	}

	// Three more venues at their full share fill the pool exactly; only then
	// does a fifth venue get refused.
	for biz := uint(2); biz <= guestPoolMinVenuesForTest; biz++ {
		if err := reserveVia(t, b, waiter(biz), guestShare, now); err != nil {
			t.Fatalf("venue %d full share: %v", biz, err)
		}
	}
	if err := reserveVia(t, b, waiter(guestPoolMinVenuesForTest+1), 1, now); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("the guest pool must close only after %d venues, err=%v", guestPoolMinVenuesForTest, err)
	}
	// Owners keep the reserve (global - pool); one full owner cap fits in it.
	if err := reserveVia(t, b, GenerateRequest{Feature: "director", BusinessID: 9}, perBiz, now); err != nil {
		t.Fatalf("owner reserve: %v", err)
	}
}

// guestPoolMinVenuesForTest mirrors cmd/app guestPoolMinVenues (pool / share).
const guestPoolMinVenuesForTest = 4

func TestBudgetStore_CommittedSpendScopesSingleRead(t *testing.T) {
	db := requireBudgetPostgres(t)
	store := NewBudgetStore(db)
	now := time.Date(2026, 10, 3, 16, 0, 0, 0, time.UTC)
	p := multiScopeParams(21, BudgetScopeGuest, 5_000_000, 5_000_000, 250_000, now)
	p.ExtraScopes = append(p.ExtraScopes, ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGuestPool, CapMicroUSD: 5_000_000})
	if err := store.Reserve(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := store.Finalize(context.Background(), FinalizeParams{ReservationID: p.ReservationID, ActualMicroUSD: 100_000, Now: now}); err != nil {
		t.Fatal(err)
	}
	scopes := []ScopeCap{
		{BusinessID: 0, FeatureScope: BudgetScopeGlobal},
		{BusinessID: 21, FeatureScope: BudgetScopeOwner}, // no row yet
		{BusinessID: 21, FeatureScope: BudgetScopeGuest},
		{BusinessID: 0, FeatureScope: BudgetScopeGuestPool},
	}
	got, err := store.CommittedSpendScopes(context.Background(), scopes, now)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint([]int64{100_000, 0, 100_000, 100_000}) {
		t.Fatalf("CommittedSpendScopes = %v", got)
	}
	if other, err := store.CommittedSpendScopes(context.Background(), scopes, now.AddDate(0, 0, 1)); err != nil || fmt.Sprint(other) != "[0 0 0 0]" {
		t.Fatalf("another day must read zero, got %v err=%v", other, err)
	}
	if empty, err := store.CommittedSpendScopes(context.Background(), nil, now); err != nil || len(empty) != 0 {
		t.Fatalf("no scopes: %v %v", empty, err)
	}
}
