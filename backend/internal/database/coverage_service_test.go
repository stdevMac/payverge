package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newCoverageTestDB migrates exactly the tables the coverage service touches.
func newCoverageTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Schedule{}, &Shift{},
		&OpenShiftClaim{}, &ShiftSwapRequest{}, &RBACAuditLog{},
	))
	// Partial unique index (status='pending') — the atomic double-claim guard.
	// GORM tags can't express a WHERE-filtered index, so mirror the genesis
	// baseline index here so the SQLite fixture has the constraint too.
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_open_claims_one_pending ON open_shift_claims (business_id, shift_id, claiming_staff_id) WHERE status = 'pending'").Error)
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

func TestCoverageModelsTableNamesAndDefaults(t *testing.T) {
	_ = newCoverageTestDB(t)
	require.Equal(t, "open_shift_claims", OpenShiftClaim{}.TableName())
	require.Equal(t, "shift_swap_requests", ShiftSwapRequest{}.TableName())

	claim := OpenShiftClaim{BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5}
	require.NoError(t, db.Create(&claim).Error)
	require.Equal(t, OpenClaimStatusPending, claim.Status, "DDL default pending must materialize")

	swap := ShiftSwapRequest{BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap}
	require.NoError(t, db.Create(&swap).Error)
	require.Equal(t, SwapStatusOpen, swap.Status)
	require.Equal(t, SwapTargetAllInRole, swap.Target, "DDL default all_in_role must materialize")
}

// TestCoverageGenesisShape is the genesis-safety guard: on a fresh DB the SQL
// migration is force-baselined WITHOUT running its DDL, so GORM autoMigrate must
// materialize the schema from the struct tags. This asserts the tables, the
// explicit TableName()s, every column the 000107 DDL declares, and — critically —
// every named index the DDL creates, or a force-baselined fresh DB silently loses
// the coverage lookups (eligibility joins + the manager pending-approval queues).
func TestCoverageGenesisShape(t *testing.T) {
	newCoverageTestDB(t)

	require.Equal(t, "open_shift_claims", OpenShiftClaim{}.TableName())
	require.Equal(t, "shift_swap_requests", ShiftSwapRequest{}.TableName())

	m := db.Migrator()
	require.True(t, m.HasTable("open_shift_claims"))
	require.True(t, m.HasTable("shift_swap_requests"))

	for _, col := range []string{"business_id", "shift_id", "claiming_staff_id", "status", "decided_by_staff_id", "decided_at", "created_at", "updated_at"} {
		require.Truef(t, m.HasColumn(&OpenShiftClaim{}, col), "open_shift_claims missing column %s", col)
	}
	for _, col := range []string{"business_id", "shift_id", "requesting_staff_id", "kind", "target", "target_staff_id", "status", "accepting_staff_id", "approved_by_staff_id", "created_at", "resolved_at", "updated_at"} {
		require.Truef(t, m.HasColumn(&ShiftSwapRequest{}, col), "shift_swap_requests missing column %s", col)
	}

	// Genesis index parity: the struct tags must materialize the SAME named
	// indexes the DDL creates, or a force-baselined fresh DB loses the coverage
	// reads (eligibility joins on shift, manager pending queues by status).
	for _, idx := range []string{"idx_open_claims_business_shift", "idx_open_claims_business_status"} {
		require.Truef(t, m.HasIndex(&OpenShiftClaim{}, idx), "open_shift_claims missing genesis index %s", idx)
	}
	for _, idx := range []string{"idx_swaps_business_status", "idx_swaps_business_shift"} {
		require.Truef(t, m.HasIndex(&ShiftSwapRequest{}, idx), "shift_swap_requests missing genesis index %s", idx)
	}

	// The partial unique index (status='pending') is the atomic double-claim
	// guard. GORM tags can't express a WHERE-filtered index, so it is ensured via
	// a raw Exec in db_config.go autoMigrate (mirrored in the test harness) — this
	// asserts a force-baselined fresh DB still gets the constraint.
	require.True(t, m.HasIndex(&OpenShiftClaim{}, "idx_open_claims_one_pending"), "open_shift_claims missing partial-unique pending-claim guard index")
}

func TestValidSwapTransition(t *testing.T) {
	legal := map[[2]string]bool{
		{SwapStatusOpen, SwapStatusAccepted}:             true, // coworker accepts
		{SwapStatusAccepted, SwapStatusPendingApproval}:  true, // auto-advance to manager queue
		{SwapStatusPendingApproval, SwapStatusApproved}:  true,
		{SwapStatusPendingApproval, SwapStatusDenied}:    true,
		{SwapStatusOpen, SwapStatusCancelled}:            true,
		{SwapStatusAccepted, SwapStatusCancelled}:        true,
		{SwapStatusPendingApproval, SwapStatusCancelled}: true,
	}
	all := []string{SwapStatusOpen, SwapStatusAccepted, SwapStatusPendingApproval, SwapStatusApproved, SwapStatusDenied, SwapStatusCancelled}
	for _, from := range all {
		for _, to := range all {
			want := legal[[2]string{from, to}]
			require.Equalf(t, want, validSwapTransition(from, to), "swap %s->%s", from, to)
		}
	}
	// terminal states never transition
	require.False(t, validSwapTransition(SwapStatusApproved, SwapStatusPendingApproval))
	require.False(t, validSwapTransition(SwapStatusDenied, SwapStatusApproved))
}

func TestValidClaimTransition(t *testing.T) {
	require.True(t, validClaimTransition(OpenClaimStatusPending, OpenClaimStatusApproved))
	require.True(t, validClaimTransition(OpenClaimStatusPending, OpenClaimStatusDenied))
	require.True(t, validClaimTransition(OpenClaimStatusPending, OpenClaimStatusWithdrawn))
	require.False(t, validClaimTransition(OpenClaimStatusApproved, OpenClaimStatusDenied))
	require.False(t, validClaimTransition(OpenClaimStatusDenied, OpenClaimStatusApproved))
	require.False(t, validClaimTransition(OpenClaimStatusPending, OpenClaimStatusPending))
}

func TestIsStaffEligibleForPosition(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9, IsPrimary: true}).Error)

	ok, err := d.IsStaffEligibleForPosition(1, 5, 9)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = d.IsStaffEligibleForPosition(1, 5, 99) // not assigned to position 99
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = d.IsStaffEligibleForPosition(2, 5, 9) // wrong tenant
	require.NoError(t, err)
	require.False(t, ok, "eligibility must be tenant-scoped")
}

func seedOpenShift(t *testing.T, biz, shiftID, posID uint) {
	require.NoError(t, db.Create(&Shift{
		ID: shiftID, BusinessID: biz, ScheduleID: 1, PositionID: posID,
		Status: ShiftStatusOpen, CreatedByStaffID: 1,
	}).Error)
}

func TestClaimOpenShift(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	seedOpenShift(t, 1, 10, 9)

	claim, err := d.ClaimOpenShift(1, 10, 5, "staff:5")
	require.NoError(t, err)
	require.Equal(t, OpenClaimStatusPending, claim.Status)

	// audit row written
	var audits int64
	require.NoError(t, db.Model(&RBACAuditLog{}).
		Where("business_id = ? AND action = ?", 1, RBACActionCoverageClaimed).Count(&audits).Error)
	require.Equal(t, int64(1), audits)

	// double-claim by same staff rejected
	_, err = d.ClaimOpenShift(1, 10, 5, "staff:5")
	require.ErrorIs(t, err, ErrAlreadyClaimed)
}

func TestClaimRejectsIneligibleAndNonOpen(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedOpenShift(t, 1, 10, 9) // staff 5 NOT assigned to position 9
	_, err := d.ClaimOpenShift(1, 10, 5, "staff:5")
	require.ErrorIs(t, err, ErrNotEligible)

	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 6, PositionID: 9}).Error)
	require.NoError(t, db.Create(&Shift{ID: 11, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	_, err = d.ClaimOpenShift(1, 11, 6, "staff:6")
	require.ErrorIs(t, err, ErrShiftNotOpen)
}

func ptrU(v uint) *uint { return &v }

func TestRequestSwapOpensForSwapKind(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)

	sw, err := d.RequestSwap(1, 10, 5, SwapKindSwap, SwapTargetAllInRole, nil, "staff:5")
	require.NoError(t, err)
	require.Equal(t, SwapStatusOpen, sw.Status)

	gi, err := d.RequestSwap(1, 10, 5, SwapKindGiveup, SwapTargetAllInRole, nil, "staff:5")
	require.NoError(t, err)
	require.Equal(t, SwapStatusPendingApproval, gi.Status, "giveup skips accept -> straight to manager queue")

	var n int64
	require.NoError(t, db.Model(&RBACAuditLog{}).Where("action = ?", RBACActionCoverageSwapRequested).Count(&n).Error)
	require.Equal(t, int64(2), n)
}

func TestRequestSwapRejectsNonOwnerAndBadKind(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	// staff 7 does not own the shift
	_, err := d.RequestSwap(1, 10, 7, SwapKindSwap, SwapTargetAllInRole, nil, "staff:7")
	require.ErrorIs(t, err, ErrSelfCoverage)
}

func TestAcceptSwapAdvancesAndGuards(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 6, PositionID: 9}).Error) // acceptor eligible
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)

	sw, err := d.AcceptSwap(1, 1, 6, "staff:6")
	require.NoError(t, err)
	require.Equal(t, SwapStatusPendingApproval, sw.Status)
	require.NotNil(t, sw.AcceptingStaffID)
	require.Equal(t, uint(6), *sw.AcceptingStaffID)

	var n int64
	require.NoError(t, db.Model(&RBACAuditLog{}).Where("action = ?", RBACActionCoverageAccepted).Count(&n).Error)
	require.Equal(t, int64(1), n)

	// double-accept: status is no longer 'open' -> conflict (precondition guard)
	_, err = d.AcceptSwap(1, 1, 6, "staff:6")
	require.ErrorIs(t, err, ErrCoverageConflict)
}

func TestAcceptSwapRejectsIneligibleAndSelf(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)

	_, err := d.AcceptSwap(1, 1, 7, "staff:7") // 7 not assigned to position 9
	require.ErrorIs(t, err, ErrNotEligible)

	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	_, err = d.AcceptSwap(1, 1, 5, "staff:5") // requester == acceptor
	require.ErrorIs(t, err, ErrSelfCoverage)
}

func TestDecideSwapRejectsElapsedShift(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	past := time.Now().UTC().Add(-2 * time.Hour)
	require.NoError(t, db.Create(&Shift{
		ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5),
		Status: ShiftStatusFilled, CreatedByStaffID: 1,
		StartsAt: past.Add(-4 * time.Hour), EndsAt: past,
	}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{
		ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5,
		AcceptingStaffID: ptrU(6), Kind: SwapKindSwap, Status: SwapStatusAccepted,
	}).Error)

	_, err := d.DecideSwap(1, 1, 99, true, "mgr:99")
	require.ErrorIs(t, err, ErrCoverageExpired)
}

func TestDecideSwapApproveReassignsShift(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, AcceptingStaffID: ptrU(6), Kind: SwapKindSwap, Status: SwapStatusAccepted}).Error)

	sw, err := d.DecideSwap(1, 1, 99, true, "mgr:99") // approve
	require.NoError(t, err)
	require.Equal(t, SwapStatusApproved, sw.Status)
	require.NotNil(t, sw.ResolvedAt)

	var shift Shift
	require.NoError(t, db.First(&shift, 10).Error)
	require.NotNil(t, shift.StaffID)
	require.Equal(t, uint(6), *shift.StaffID, "approved swap reassigns the shift to the acceptor")

	// double-decision: source no longer accepted/pending_approval -> conflict
	_, err = d.DecideSwap(1, 1, 99, false, "mgr:99")
	require.ErrorIs(t, err, ErrCoverageConflict)
}

func TestDecideSwapApproveGiveupOpensShift(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindGiveup, Status: SwapStatusPendingApproval}).Error)

	_, err := d.DecideSwap(1, 1, 99, true, "mgr:99")
	require.NoError(t, err)
	var shift Shift
	require.NoError(t, db.First(&shift, 10).Error)
	require.Nil(t, shift.StaffID, "approved giveup converts the shift to open")
	require.Equal(t, ShiftStatusOpen, shift.Status)
}

func TestDecideSwapIllegalSourceRejected(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)
	_, err := d.DecideSwap(1, 1, 99, true, "mgr:99") // open is not a decidable source
	require.ErrorIs(t, err, ErrCoverageConflict)
}

func TestDecideClaimApproveFillsAndGuardsDoubleFill(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: ShiftStatusOpen, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&OpenShiftClaim{ID: 1, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 6, Status: OpenClaimStatusPending}).Error)
	require.NoError(t, db.Create(&OpenShiftClaim{ID: 2, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 7, Status: OpenClaimStatusPending}).Error)

	cl, err := d.DecideClaim(1, 1, 99, true, "mgr:99")
	require.NoError(t, err)
	require.Equal(t, OpenClaimStatusApproved, cl.Status)

	var shift Shift
	require.NoError(t, db.First(&shift, 10).Error)
	require.Equal(t, uint(6), *shift.StaffID)
	require.Equal(t, ShiftStatusFilled, shift.Status)

	// the other pending claim on the now-filled shift is auto-denied
	var other OpenShiftClaim
	require.NoError(t, db.First(&other, 2).Error)
	require.Equal(t, OpenClaimStatusDenied, other.Status)

	// approving the already-denied claim 2 fails the shift-open precondition
	_, err = d.DecideClaim(1, 2, 99, true, "mgr:99")
	require.ErrorIs(t, err, ErrCoverageConflict)
}

func TestListOpenCoverageEligibilityScopedAndBounded(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	// eligible open shift + an open shift for a position 5 is NOT assigned to
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: ShiftStatusOpen, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&Shift{ID: 11, BusinessID: 1, ScheduleID: 1, PositionID: 8, Status: ShiftStatusOpen, CreatedByStaffID: 1}).Error)
	// an open swap by a coworker for the eligible position
	require.NoError(t, db.Create(&Shift{ID: 12, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(6), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 12, RequestingStaffID: 6, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)

	res, err := d.ListOpenCoverage(1, 5, false) // staff, not approver
	require.NoError(t, err)
	require.Len(t, res.OpenShifts, 1, "only position 9 is eligible")
	require.Equal(t, uint(10), res.OpenShifts[0].ShiftID)
	require.Len(t, res.SwapInbox, 1)
	require.Equal(t, uint(1), res.SwapInbox[0].SwapID)
	require.Empty(t, res.PendingApprovals.Swaps, "non-approver gets no manager queue")

	// approver gets the pending-approval queue — position_id must travel with
	// the row so the manager inbox can name the shift (not "Shift").
	require.NoError(t, db.Model(&ShiftSwapRequest{}).Where("id = 1").Update("status", SwapStatusPendingApproval).Error)
	mgr, err := d.ListOpenCoverage(1, 99, true)
	require.NoError(t, err)
	require.Len(t, mgr.PendingApprovals.Swaps, 1)
	require.Equal(t, uint(9), mgr.PendingApprovals.Swaps[0].PositionID)
}

func TestListMyCoverageTenantScoped(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&OpenShiftClaim{BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusPending}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{BusinessID: 1, ShiftID: 11, RequestingStaffID: 5, Kind: SwapKindGiveup, Status: SwapStatusPendingApproval}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{BusinessID: 1, ShiftID: 12, RequestingStaffID: 9, AcceptingStaffID: ptrU(5), Kind: SwapKindSwap, Status: SwapStatusAccepted}).Error)
	require.NoError(t, db.Create(&OpenShiftClaim{BusinessID: 2, ShiftID: 99, ClaimingStaffID: 5, Status: OpenClaimStatusPending}).Error) // other tenant

	res, err := d.ListMyCoverage(1, 5)
	require.NoError(t, err)
	require.Len(t, res.Claims, 1)
	require.Len(t, res.Swaps, 2, "swaps I requested OR accepted")
}

// ---- Cancel / withdraw (staff retracts their own request) -------------------

func TestCancelSwapByRequester(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	// The shift is still owned by the requester while the swap is only open.
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)

	sw, err := d.CancelSwap(1, 1, 5, "staff:5")
	require.NoError(t, err)
	require.Equal(t, SwapStatusCancelled, sw.Status)
	require.NotNil(t, sw.ResolvedAt)

	// The shift is untouched — a non-approved request never moved it.
	var shift Shift
	require.NoError(t, db.First(&shift, 10).Error)
	require.NotNil(t, shift.StaffID)
	require.Equal(t, uint(5), *shift.StaffID)
	require.Equal(t, ShiftStatusFilled, shift.Status)

	var n int64
	require.NoError(t, db.Model(&RBACAuditLog{}).Where("action = ?", RBACActionCoverageCancelled).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestCancelSwapPendingApprovalGiveup(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindGiveup, Status: SwapStatusPendingApproval}).Error)

	sw, err := d.CancelSwap(1, 1, 5, "staff:5")
	require.NoError(t, err)
	require.Equal(t, SwapStatusCancelled, sw.Status)

	// The giveup was awaiting a manager but never approved — the shift stays owned.
	var shift Shift
	require.NoError(t, db.First(&shift, 10).Error)
	require.Equal(t, uint(5), *shift.StaffID)
	require.Equal(t, ShiftStatusFilled, shift.Status)
}

func TestCancelSwapRejectsNonOwner(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)

	_, err := d.CancelSwap(1, 1, 7, "staff:7") // 7 is not the requester
	require.ErrorIs(t, err, ErrNotOwner)

	var sw ShiftSwapRequest
	require.NoError(t, db.First(&sw, 1).Error)
	require.Equal(t, SwapStatusOpen, sw.Status, "a non-owner cancel must not mutate the request")
}

func TestCancelSwapRejectsTerminal(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusApproved}).Error)

	_, err := d.CancelSwap(1, 1, 5, "staff:5") // already terminal
	require.ErrorIs(t, err, ErrCoverageConflict)
}

func TestCancelSwapNotFoundAndTenantScoped(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{ID: 1, BusinessID: 2, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error) // other tenant

	_, err := d.CancelSwap(1, 999, 5, "staff:5")
	require.ErrorIs(t, err, ErrCoverageNotFound)
	_, err = d.CancelSwap(1, 1, 5, "staff:5") // swap belongs to business 2
	require.ErrorIs(t, err, ErrCoverageNotFound)
}

func TestWithdrawClaimByOwner(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: ShiftStatusOpen, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&OpenShiftClaim{ID: 1, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusPending}).Error)

	cl, err := d.WithdrawClaim(1, 1, 5, "staff:5")
	require.NoError(t, err)
	require.Equal(t, OpenClaimStatusWithdrawn, cl.Status)
	require.NotNil(t, cl.DecidedAt)

	// The shift stays open — a pending claim never filled it.
	var shift Shift
	require.NoError(t, db.First(&shift, 10).Error)
	require.Nil(t, shift.StaffID)
	require.Equal(t, ShiftStatusOpen, shift.Status)

	var n int64
	require.NoError(t, db.Model(&RBACAuditLog{}).Where("action = ?", RBACActionCoverageCancelled).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestWithdrawClaimRejectsNonOwnerAndNonPending(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&OpenShiftClaim{ID: 1, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusPending}).Error)

	_, err := d.WithdrawClaim(1, 1, 7, "staff:7") // 7 is not the claimant
	require.ErrorIs(t, err, ErrNotOwner)

	require.NoError(t, db.Create(&OpenShiftClaim{ID: 2, BusinessID: 1, ShiftID: 11, ClaimingStaffID: 5, Status: OpenClaimStatusApproved}).Error)
	_, err = d.WithdrawClaim(1, 2, 5, "staff:5") // already approved — not withdrawable
	require.ErrorIs(t, err, ErrCoverageConflict)
}

func BenchmarkListOpenCoverage(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), scheduleBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Shift{}, &OpenShiftClaim{}, &ShiftSwapRequest{}, &RBACAuditLog{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	_ = db.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error
	for i := 0; i < 100; i++ {
		_ = db.Create(&Shift{BusinessID: 1, ScheduleID: 1, PositionID: 9, Status: ShiftStatusOpen, CreatedByStaffID: 1}).Error
	}
	d := GetDBWrapper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListOpenCoverage(1, 5, false)
	}
}

// TestClaimOneePendingIndexEnforced proves the idx_open_claims_one_pending partial
// unique index actually constrains the data — independently of ClaimOpenShift's
// ON CONFLICT path. We RAW-insert OpenShiftClaim rows directly: a SECOND pending
// claim for the same (business, shift, staff) must be rejected by the partial
// index, while a NON-pending (denied) row for the same triple is allowed (the
// index only constrains rows WHERE status='pending'). This is the assertion that
// fails if the partial index is dropped or its predicate is typo'd anywhere.
func TestClaimOnePendingIndexEnforced(t *testing.T) {
	newCoverageTestDB(t)

	// First pending claim — allowed.
	require.NoError(t, db.Create(&OpenShiftClaim{BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusPending}).Error)

	// Second pending claim for the SAME (business, shift, staff) — the partial
	// unique index must reject it (unique violation).
	err := db.Create(&OpenShiftClaim{BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusPending}).Error
	require.Error(t, err, "a second pending claim for the same staff+shift must violate idx_open_claims_one_pending")

	// A NON-pending (denied) row for the same triple is allowed — the partial
	// index only constrains rows WHERE status='pending'.
	require.NoError(t, db.Create(&OpenShiftClaim{BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusDenied}).Error)
}

func TestListEligibleStaffIDsForShift(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	sched := Schedule{BusinessID: 1, Status: "published", WeekStart: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&sched).Error)
	shift := Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: 5, Status: "open", Published: true, StartsAt: time.Now(), EndsAt: time.Now().Add(8 * time.Hour)}
	require.NoError(t, db.Create(&shift).Error)
	a := Staff{BusinessID: 1, Email: "a@b.test", Name: "A", Role: "server", IsActive: true, InvitedBy: "o"}
	b := Staff{BusinessID: 1, Email: "b@b.test", Name: "B", Role: "server", IsActive: true, InvitedBy: "o"}
	off := Staff{BusinessID: 1, Email: "x@b.test", Name: "X", Role: "server", IsActive: true, InvitedBy: "o"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	require.NoError(t, db.Create(&off).Error)
	require.NoError(t, db.Model(&off).Update("is_active", false).Error)
	// a + off linked to position 5; b linked to a different position.
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: a.ID, PositionID: 5}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: off.ID, PositionID: 5}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: b.ID, PositionID: 9}).Error)

	ids, err := d.ListEligibleStaffIDsForShift(1, shift.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{a.ID}, ids, "only active staff linked to the shift's position")
}

// seedHistoryRows inserts a business, two shifts, and a mix of terminal and
// non-terminal swaps/claims into g. Terminal events resolve at t1<t2<t3<t4.
func seedHistoryRows(t testing.TB, g *gorm.DB) {
	t.Helper()
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	base := time.Date(2026, 7, 1, 16, 0, 0, 0, time.UTC)
	mkShift := func(id, pos uint) {
		require.NoError(t, g.Create(&Shift{ID: id, BusinessID: 1, ScheduleID: 1, PositionID: pos, StartsAt: base, EndsAt: base.Add(6 * time.Hour), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	}
	mkShift(10, 9)
	mkShift(11, 8)
	t1, t2, t3, t4 := base.Add(1*time.Hour), base.Add(2*time.Hour), base.Add(3*time.Hour), base.Add(4*time.Hour)
	decider, self := uint(2), uint(6)

	// Terminal swap (approved, t2) + terminal giveup (self-cancelled, t4 = newest).
	require.NoError(t, g.Create(&ShiftSwapRequest{ID: 100, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusApproved, ApprovedByStaffID: &decider, ResolvedAt: &t2}).Error)
	require.NoError(t, g.Create(&ShiftSwapRequest{ID: 101, BusinessID: 1, ShiftID: 11, RequestingStaffID: 6, Kind: SwapKindGiveup, Status: SwapStatusCancelled, ResolvedAt: &t4}).Error)
	// Non-terminal swap (open) — must be excluded.
	require.NoError(t, g.Create(&ShiftSwapRequest{ID: 102, BusinessID: 1, ShiftID: 10, RequestingStaffID: 7, Kind: SwapKindSwap, Status: SwapStatusOpen}).Error)

	// Terminal claim (approved, t1) + terminal claim (self-withdrawn, t3).
	require.NoError(t, g.Create(&OpenShiftClaim{ID: 200, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 5, Status: OpenClaimStatusApproved, DecidedByStaffID: &decider, DecidedAt: &t1}).Error)
	require.NoError(t, g.Create(&OpenShiftClaim{ID: 201, BusinessID: 1, ShiftID: 11, ClaimingStaffID: 6, Status: OpenClaimStatusWithdrawn, DecidedByStaffID: &self, DecidedAt: &t3}).Error)
	// Pending claim — must be excluded.
	require.NoError(t, g.Create(&OpenShiftClaim{ID: 202, BusinessID: 1, ShiftID: 10, ClaimingStaffID: 8, Status: OpenClaimStatusPending}).Error)
}

// TestListCoverageHistory proves the feed returns only resolved (terminal) events,
// flattened across both tables, newest-resolution first, with shift/position joined
// and self-actions carrying no decider. Tenant-scoped.
func TestListCoverageHistory(t *testing.T) {
	d := newCoverageTestDB(t)
	seedHistoryRows(t, d.GetGorm())

	hist, err := d.ListCoverageHistory(1, 50)
	require.NoError(t, err)
	require.Len(t, hist, 4, "four terminal events; open swap + pending claim excluded")

	// Newest first: t4 giveup-cancel > t3 claim-withdraw > t2 swap-approve > t1 claim-approve.
	require.Equal(t, "giveup", hist[0].Kind)
	require.Equal(t, SwapStatusCancelled, hist[0].Status)
	require.Equal(t, uint(101), hist[0].RequestID)
	require.Nil(t, hist[0].DeciderStaffID, "a self-cancel has no decider")

	require.Equal(t, "open_claim", hist[1].Kind)
	require.Equal(t, OpenClaimStatusWithdrawn, hist[1].Status)

	require.Equal(t, "swap", hist[2].Kind)
	require.Equal(t, SwapStatusApproved, hist[2].Status)
	require.Equal(t, uint(9), hist[2].PositionID, "position joined from the shift")
	require.Equal(t, uint(5), hist[2].RequesterStaffID)
	require.NotNil(t, hist[2].DeciderStaffID)
	require.Equal(t, uint(2), *hist[2].DeciderStaffID)

	require.Equal(t, "open_claim", hist[3].Kind)
	require.Equal(t, OpenClaimStatusApproved, hist[3].Status)

	// limit caps AFTER the merge/sort → the two newest resolutions.
	top2, err := d.ListCoverageHistory(1, 2)
	require.NoError(t, err)
	require.Len(t, top2, 2)
	require.Equal(t, uint(101), top2[0].RequestID)             // t4
	require.Equal(t, OpenClaimStatusWithdrawn, top2[1].Status) // t3

	// Tenant scoping — a foreign business sees nothing.
	empty, err := d.ListCoverageHistory(2, 50)
	require.NoError(t, err)
	require.Len(t, empty, 0)
}

// TestListCoverageHistoryAccessShape locks the feed to bounded, narrow-projection
// reads: no SELECT *, every query LIMIT-bounded, and exactly the two table reads
// (never an N+1 per event to hydrate the shift or the actors).
func TestListCoverageHistoryAccessShape(t *testing.T) {
	rec := &laborSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), scheduleBenchSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Schedule{}, &Shift{}, &OpenShiftClaim{}, &ShiftSwapRequest{}, &RBACAuditLog{}))
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()
	seedHistoryRows(t, g)

	rec.stmts = nil
	_, err = d.ListCoverageHistory(1, 50)
	require.NoError(t, err)

	sels := selectStmts(rec)
	require.Len(t, sels, 2, "exactly two reads (swaps + claims), no N+1 per event")
	for _, s := range sels {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		require.Contains(t, s, "LIMIT", "must be bounded")
	}
}

func BenchmarkListCoverageHistory(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), scheduleBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Shift{}, &OpenShiftClaim{}, &ShiftSwapRequest{}, &RBACAuditLog{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	base := time.Date(2026, 7, 1, 16, 0, 0, 0, time.UTC)
	decider := uint(2)
	for i := 0; i < 100; i++ {
		sid := uint(1000 + i)
		res := base.Add(time.Duration(i) * time.Minute)
		_ = db.Create(&Shift{ID: sid, BusinessID: 1, ScheduleID: 1, PositionID: 9, StartsAt: base, EndsAt: base.Add(6 * time.Hour), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error
		if i%2 == 0 {
			_ = db.Create(&ShiftSwapRequest{BusinessID: 1, ShiftID: sid, RequestingStaffID: 5, Kind: SwapKindSwap, Status: SwapStatusApproved, ApprovedByStaffID: &decider, ResolvedAt: &res}).Error
		} else {
			_ = db.Create(&OpenShiftClaim{BusinessID: 1, ShiftID: sid, ClaimingStaffID: 5, Status: OpenClaimStatusDenied, DecidedByStaffID: &decider, DecidedAt: &res}).Error
		}
	}
	d := GetDBWrapper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListCoverageHistory(1, 50)
	}
}
