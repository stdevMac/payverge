package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAcceptSwapRejectsNonTargetedStaffForSpecificSwap guards a swap that was
// directed at a SPECIFIC coworker: only that coworker may accept it. Previously
// AcceptSwap checked position-eligibility and self-coverage but ignored the
// Target/TargetStaffID, so any eligible coworker in the business could hijack a
// swap offered to someone else.
func TestAcceptSwapRejectsNonTargetedStaffForSpecificSwap(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Both 6 (intended target) and 7 (interloper) are eligible for the position.
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 6, PositionID: 9}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: 9}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{
		ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5,
		Kind: SwapKindSwap, Target: SwapTargetSpecific, TargetStaffID: ptrU(6),
		Status: SwapStatusOpen,
	}).Error)

	// The interloper (eligible, same business, but NOT the target) is refused.
	_, err := d.AcceptSwap(1, 1, 7, "staff:7")
	require.ErrorIs(t, err, ErrNotEligible, "only the targeted staff may accept a specific swap")

	// The swap must remain open for the intended target.
	var reloaded ShiftSwapRequest
	require.NoError(t, db.First(&reloaded, 1).Error)
	require.Equal(t, SwapStatusOpen, reloaded.Status, "a rejected hijack must not advance the swap")
	require.Nil(t, reloaded.AcceptingStaffID)

	// The intended target accepts successfully.
	sw, err := d.AcceptSwap(1, 1, 6, "staff:6")
	require.NoError(t, err)
	require.Equal(t, SwapStatusPendingApproval, sw.Status)
	require.NotNil(t, sw.AcceptingStaffID)
	require.Equal(t, uint(6), *sw.AcceptingStaffID)
}

// TestAcceptSwapAllInRoleStillOpenToAnyEligible is the positive control: an
// all_in_role swap remains acceptable by any eligible coworker (the specific-
// target guard must not regress the default broadcast behavior).
func TestAcceptSwapAllInRoleStillOpenToAnyEligible(t *testing.T) {
	d := newCoverageTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: 9}).Error)
	require.NoError(t, db.Create(&Shift{ID: 10, BusinessID: 1, ScheduleID: 1, PositionID: 9, StaffID: ptrU(5), Status: ShiftStatusFilled, CreatedByStaffID: 1}).Error)
	require.NoError(t, db.Create(&ShiftSwapRequest{
		ID: 1, BusinessID: 1, ShiftID: 10, RequestingStaffID: 5,
		Kind: SwapKindSwap, Target: SwapTargetAllInRole,
		Status: SwapStatusOpen,
	}).Error)

	sw, err := d.AcceptSwap(1, 1, 7, "staff:7")
	require.NoError(t, err)
	require.Equal(t, SwapStatusPendingApproval, sw.Status)
}
