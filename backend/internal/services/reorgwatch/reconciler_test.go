package reorgwatch

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// --- fakes ---------------------------------------------------------------

type checkResult struct {
	hash   string
	number uint64
	found  bool
	err    error
}

type fakeChecker struct {
	results map[string]checkResult // keyed by txHash
	calls   int
}

func (f *fakeChecker) CanonicalBlock(_ context.Context, txHash string) (string, uint64, bool, error) {
	f.calls++
	r, ok := f.results[txHash]
	if !ok {
		// Default: still canonical at an unknown hash — treated as re-inclusion.
		return "0xdefault", 1, true, nil
	}
	return r.hash, r.number, r.found, r.err
}

type fakeAlerter struct {
	alerts []ReorgAlert
}

func (a *fakeAlerter) ReorgSuspected(_ context.Context, ev ReorgAlert) {
	a.alerts = append(a.alerts, ev)
}

// --- harness -------------------------------------------------------------

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:reorg_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: glog.Default.LogMode(glog.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.PaymentRefund{},
	))
	return db
}

func ptrI64(v int64) *int64          { return &v }
func ptrStr(v string) *string        { return &v }
func ptrTime(t time.Time) *time.Time { return &t }

// seedConfirmedPayment inserts a bill + a confirmed crypto payment carrying
// block evidence. suspectedAt/checkedAt may be nil.
func seedConfirmedPayment(t *testing.T, db *gorm.DB, businessID uint, txHash, blockHash string, confirmedAt time.Time, suspectedAt, checkedAt *time.Time) *database.Payment {
	t.Helper()
	bill := &database.Bill{BusinessID: businessID, Status: database.BillStatusPaid, TotalAmount: 5000}
	require.NoError(t, db.Create(bill).Error)
	p := &database.Payment{
		BillID:           bill.ID,
		PayerAddr:        "crypto_guest",
		Amount:           5000,
		TxHash:           txHash,
		Status:           database.PaymentStatusConfirmed,
		PaymentMethod:    "crypto",
		ConfirmedAt:      ptrTime(confirmedAt),
		BlockNumber:      ptrI64(100),
		BlockHash:        ptrStr(blockHash),
		ReorgSuspectedAt: suspectedAt,
		ReorgCheckedAt:   checkedAt,
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

func seedConfirmedRefund(t *testing.T, db *gorm.DB, businessID, billID, paymentID uint, txHash, blockHash string, confirmedAt time.Time, suspectedAt *time.Time) *database.PaymentRefund {
	t.Helper()
	r := &database.PaymentRefund{
		BusinessID:        businessID,
		BillID:            billID,
		PaymentID:         paymentID,
		ChainID:           8453,
		Token:             "USDC",
		AmountBaseUnits:   5_000_000,
		VerifiedRecipient: "0xrecipient",
		Reason:            "test",
		RequestedBy:       "owner",
		Status:            database.PaymentRefundStatusConfirmed,
		IdempotencyKey:    "idem-" + txHash,
		SubmittedTxHash:   ptrStr(txHash),
		Confirmations:     3,
		ConfirmedAt:       ptrTime(confirmedAt),
		BlockNumber:       ptrI64(100),
		BlockHash:         ptrStr(blockHash),
		ReorgSuspectedAt:  suspectedAt,
	}
	require.NoError(t, db.Create(r).Error)
	return r
}

func newReconciler(db *gorm.DB, checker CanonicalChecker, alerter Alerter, now time.Time) *Reconciler {
	r := New(db, checker, alerter, Config{FinalityWindow: 30 * time.Minute, Cooldown: 2 * time.Minute, BatchSize: 100})
	r.now = func() time.Time { return now }
	return r
}

func reloadPayment(t *testing.T, db *gorm.DB, id uint) database.Payment {
	t.Helper()
	var p database.Payment
	require.NoError(t, db.First(&p, id).Error)
	return p
}

func reloadRefund(t *testing.T, db *gorm.DB, id uint) database.PaymentRefund {
	t.Helper()
	var r database.PaymentRefund
	require.NoError(t, db.First(&r, id).Error)
	return r
}

// --- tests ---------------------------------------------------------------

// A confirmed payment still canonical: suspicion cleared, check clock advanced,
// no alert.
func TestSweep_CanonicalClearsSuspicionAndAdvancesChecked(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	priorSuspicion := now.Add(-5 * time.Minute)
	p := seedConfirmedPayment(t, db, 1, "0xtx1", "0xAAA", now.Add(-3*time.Minute), &priorSuspicion, nil)

	checker := &fakeChecker{results: map[string]checkResult{"0xtx1": {hash: "0xAAA", number: 100, found: true}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, stats.Canonical)
	require.Equal(t, 0, stats.Orphaned)
	require.Empty(t, alerter.alerts)
	got := reloadPayment(t, db, p.ID)
	require.Nil(t, got.ReorgSuspectedAt, "canonical re-verification must clear prior suspicion")
	require.NotNil(t, got.ReorgCheckedAt)
}

// The FIRST observed miss must NOT alert — it may be a lagging/pruned/load-
// balanced RPC replica returning NotFound for a genuinely-canonical tx. The row
// is flagged + watched; the next canonical read clears it with no alert. This is
// the false-positive guard (review finding #1).
func TestSweep_FirstMissWatchesDoesNotAlert(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	p := seedConfirmedPayment(t, db, 1, "0xtxLag", "0xAAA", now.Add(-3*time.Minute), nil, nil)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxLag": {found: false}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, stats.Suspected)
	require.Equal(t, 0, stats.Orphaned)
	require.Empty(t, alerter.alerts, "a single transient NotFound must not fire an alert")
	got := reloadPayment(t, db, p.ID)
	require.NotNil(t, got.ReorgSuspectedAt, "first miss flags the row for watching")

	// The replica catches up: next sweep sees it canonical → suspicion clears, no
	// alert ever fired (the transient miss self-healed).
	checker.results["0xtxLag"] = checkResult{hash: "0xAAA", number: 100, found: true}
	later := now.Add(3 * time.Minute) // past cooldown so it is re-checked
	stats2, err := newReconciler(db, checker, alerter, later).Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, stats2.Canonical)
	require.Empty(t, alerter.alerts, "a self-healed transient miss must never alert")
	got = reloadPayment(t, db, p.ID)
	require.Nil(t, got.ReorgSuspectedAt, "canonical re-read clears the transient suspicion")
}

// A confirmed payment whose tx stays missing across the grace window: alerted.
func TestSweep_PersistentOrphanAlertsAfterGrace(t *testing.T) {
	db := setupDB(t)
	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	p := seedConfirmedPayment(t, db, 7, "0xtxOrphan", "0xAAA", base.Add(-3*time.Minute), nil, nil)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxOrphan": {found: false}}}
	alerter := &fakeAlerter{}

	// Sweep 1: first miss → watch, no alert.
	stats1, err := newReconciler(db, checker, alerter, base).Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, stats1.Suspected)
	require.Empty(t, alerter.alerts)

	// Sweep 2: still missing, now past the 5m grace window → alert.
	later := base.Add(6 * time.Minute)
	stats2, err := newReconciler(db, checker, alerter, later).Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, stats2.Orphaned)
	require.Len(t, alerter.alerts, 1)
	al := alerter.alerts[0]
	require.Equal(t, KindPayment, al.Kind)
	require.Equal(t, uint(7), al.BusinessID)
	require.Equal(t, p.ID, al.RecordID)
	require.Equal(t, "0xtxOrphan", al.TxHash)
	require.Equal(t, "0xAAA", al.StoredBlockHash)

	got := reloadPayment(t, db, p.ID)
	require.NotNil(t, got.ReorgSuspectedAt, "orphaned payment stays flagged")
}

// A row still missing but INSIDE the grace window keeps watching (no alert). The
// once-per-episode alert dedup is enforced at the operational_alerts Upsert layer
// (the reconciler re-raises on every post-grace sweep so a failed alert retries).
func TestSweep_WithinGraceDoesNotAlert(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	// Suspected 2m ago; grace is 5m → still watching.
	prior := now.Add(-2 * time.Minute)
	checkedLongAgo := now.Add(-3 * time.Minute)
	p := seedConfirmedPayment(t, db, 1, "0xtxStill", "0xAAA", now.Add(-5*time.Minute), &prior, &checkedLongAgo)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxStill": {found: false}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 0, stats.Orphaned)
	require.Equal(t, 1, stats.StillSuspected)
	require.Empty(t, alerter.alerts, "a miss still inside the grace window must not alert yet")
	got := reloadPayment(t, db, p.ID)
	require.NotNil(t, got.ReorgSuspectedAt)
	require.True(t, got.ReorgCheckedAt.After(checkedLongAgo), "check clock must advance")
}

// The same tx re-mined in a different block: hash updated, no alert (funds are
// intact — identical tx effects).
func TestSweep_ReInclusionUpdatesHashNoAlert(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	p := seedConfirmedPayment(t, db, 1, "0xtxReinc", "0xAAA", now.Add(-3*time.Minute), nil, nil)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxReinc": {hash: "0xBBB", number: 200, found: true}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, stats.ReIncluded)
	require.Empty(t, alerter.alerts)
	got := reloadPayment(t, db, p.ID)
	require.NotNil(t, got.BlockHash)
	require.Equal(t, "0xBBB", *got.BlockHash, "re-included tx must record its new canonical block")
	require.NotNil(t, got.BlockNumber)
	require.Equal(t, int64(200), *got.BlockNumber)
	require.Nil(t, got.ReorgSuspectedAt)
}

// A transient RPC error must never flag a row or advance its clock — a flaky
// node cannot masquerade as a reorg.
func TestSweep_TransientErrorLeavesRowUntouched(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	p := seedConfirmedPayment(t, db, 1, "0xtxErr", "0xAAA", now.Add(-3*time.Minute), nil, nil)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxErr": {err: errors.New("connection refused")}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, stats.Errors)
	require.Equal(t, 0, stats.Orphaned)
	require.Empty(t, alerter.alerts)
	got := reloadPayment(t, db, p.ID)
	require.Nil(t, got.ReorgSuspectedAt, "transient error must not flag a reorg")
	require.Nil(t, got.ReorgCheckedAt, "transient error must not advance the check clock (so it retries)")
}

// Rows confirmed before the finality window are not candidates and the checker
// is never called for them.
func TestSweep_FinalityWindowExcludesOldRows(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	// Confirmed 2h ago, well outside the 30m finality window.
	seedConfirmedPayment(t, db, 1, "0xtxOld", "0xAAA", now.Add(-2*time.Hour), nil, nil)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxOld": {found: false}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 0, stats.Checked)
	require.Equal(t, 0, checker.calls, "finalized rows must not be re-read")
	require.Empty(t, alerter.alerts)
}

// Rows re-checked within the cooldown are skipped.
func TestSweep_CooldownSkipsRecentlyChecked(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	recentCheck := now.Add(-10 * time.Second) // Cooldown is 2m
	seedConfirmedPayment(t, db, 1, "0xtxRecent", "0xAAA", now.Add(-3*time.Minute), nil, &recentCheck)

	checker := &fakeChecker{results: map[string]checkResult{"0xtxRecent": {found: false}}}
	stats, err := newReconciler(db, checker, &fakeAlerter{}, now).Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, stats.Checked, "row checked within cooldown must be skipped")
	require.Equal(t, 0, checker.calls)
}

// An outbound refund missing past the grace window is alerted with refund
// identity (Kind=refund, so the operational-alerts layer scopes it to a distinct
// resource type and it cannot collide with a payment sharing the same id).
func TestSweep_RefundOrphanedAlerts(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	// Suspected 6m ago (past the 5m grace) so this sweep escalates to an alert.
	suspected := now.Add(-6 * time.Minute)
	r := seedConfirmedRefund(t, db, 42, 9, 15, "0xrefundOrphan", "0xAAA", now.Add(-8*time.Minute), &suspected)

	checker := &fakeChecker{results: map[string]checkResult{"0xrefundOrphan": {found: false}}}
	alerter := &fakeAlerter{}
	stats, err := newReconciler(db, checker, alerter, now).Sweep(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, stats.Orphaned)
	require.Len(t, alerter.alerts, 1)
	al := alerter.alerts[0]
	require.Equal(t, KindRefund, al.Kind)
	require.Equal(t, uint(42), al.BusinessID)
	require.Equal(t, uint(9), al.BillID)
	require.Equal(t, uint(15), al.PaymentID)
	require.Equal(t, r.ID, al.RecordID)

	got := reloadRefund(t, db, r.ID)
	require.NotNil(t, got.ReorgSuspectedAt)
}

// Payments without block evidence (verify-only / non-crypto) are never
// candidates.
func TestSweep_NoBlockEvidenceIsNotACandidate(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	bill := &database.Bill{BusinessID: 1, Status: database.BillStatusPaid, TotalAmount: 5000}
	require.NoError(t, db.Create(bill).Error)
	// Confirmed payment with NO block hash (e.g. manual close / verify-only).
	require.NoError(t, db.Create(&database.Payment{
		BillID: bill.ID, PayerAddr: "cash", Amount: 5000, TxHash: "manual-1",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "cash",
		ConfirmedAt: ptrTime(now.Add(-1 * time.Minute)),
	}).Error)

	checker := &fakeChecker{}
	stats, err := newReconciler(db, checker, &fakeAlerter{}, now).Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, stats.Checked)
	require.Equal(t, 0, checker.calls)
}

// A nil checker makes Sweep a safe no-op (verifier/RPC unavailable in prod).
func TestSweep_NoCheckerIsNoop(t *testing.T) {
	db := setupDB(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	seedConfirmedPayment(t, db, 1, "0xtx", "0xAAA", now.Add(-1*time.Minute), nil, nil)
	r := New(db, nil, &fakeAlerter{}, Config{})
	r.now = func() time.Time { return now }
	stats, err := r.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, stats.Checked)
}
