package loyalty

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedRedeemedBill writes an open bill that already carries a loyalty discount,
// as RedeemPoints leaves it, without going through RedeemPoints (so no loyalty
// program row has to exist).
func seedRedeemedBill(t *testing.T, db *gorm.DB, businessID uint, number string, redeemer *uint, points int) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:                  businessID,
		TableID:                     1,
		BillNumber:                  number,
		Items:                       "[]",
		Status:                      database.BillStatusOpen,
		Subtotal:                    2000,
		TotalAmount:                 1500,
		LoyaltyDiscountCents:        500,
		LoyaltyPointsRedeemed:       points,
		LoyaltyRedeemedByCustomerID: redeemer,
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

func seedMember(t *testing.T, db *gorm.DB, customerID, businessID uint, points int) {
	t.Helper()
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: points,
		IsActive:      true,
	}).Error)
}

func memberPoints(t *testing.T, db *gorm.DB, customerID, businessID uint) int {
	t.Helper()
	var cb database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	return cb.LoyaltyPoints
}

// A bill id from another business must not be undoable through this business:
// the lookup is scoped by id AND business_id, so it reads as not found.
func TestUndoRedemptionRejectsBillFromAnotherBusiness(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessA, businessB, customerID uint = 1, 2, 7
	redeemer := customerID
	seedMember(t, db, customerID, businessA, 0)
	seedMember(t, db, customerID, businessB, 0)
	bill := seedRedeemedBill(t, db, businessB, "UNDO-XBIZ-1", &redeemer, 500)

	err := UndoRedemption(db, customerID, bill.ID, businessA)
	require.ErrorIs(t, err, ErrBillNotOpen)

	var after database.Bill
	require.NoError(t, db.First(&after, bill.ID).Error)
	require.Equal(t, int64(500), after.LoyaltyDiscountCents, "other business's discount untouched")
	require.Equal(t, 500, after.LoyaltyPointsRedeemed)
	require.NotNil(t, after.LoyaltyRedeemedByCustomerID)
	require.Equal(t, 0, memberPoints(t, db, customerID, businessA))
	require.Equal(t, 0, memberPoints(t, db, customerID, businessB))
}

// A discount with no recorded redeemer is not undoable by any guest.
func TestUndoRedemptionRejectsNilRedeemer(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 7
	seedMember(t, db, customerID, businessID, 0)
	bill := seedRedeemedBill(t, db, businessID, "UNDO-NIL-1", nil, 500)

	err := UndoRedemption(db, customerID, bill.ID, businessID)
	require.ErrorIs(t, err, ErrUndoNotOwner)

	var after database.Bill
	require.NoError(t, db.First(&after, bill.ID).Error)
	require.Equal(t, int64(500), after.LoyaltyDiscountCents)
	require.Equal(t, 0, memberPoints(t, db, customerID, businessID))
}

// Undo returns the points recorded at redeem time and never consults the
// loyalty program: it succeeds with no program row at all.
func TestUndoRedemptionWithoutLoyaltyProgramRestoresRecordedPoints(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 7
	redeemer := customerID
	seedMember(t, db, customerID, businessID, 40)
	bill := seedRedeemedBill(t, db, businessID, "UNDO-NOPROG-1", &redeemer, 500)

	var programs int64
	require.NoError(t, db.Model(&database.LoyaltyProgram{}).Count(&programs).Error)
	require.Zero(t, programs)

	require.NoError(t, UndoRedemption(db, customerID, bill.ID, businessID))
	require.Equal(t, 540, memberPoints(t, db, customerID, businessID))

	var after database.Bill
	require.NoError(t, db.First(&after, bill.ID).Error)
	require.Equal(t, int64(0), after.LoyaltyDiscountCents)
	require.Equal(t, int64(2000), after.TotalAmount)
	require.Nil(t, after.LoyaltyRedeemedByCustomerID)
}

// Without recorded points there is no rate-based fallback: the undo refuses
// rather than guessing from the current redemption rate.
func TestUndoRedemptionWithoutRecordedPointsHasNoRateFallback(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 7
	redeemer := customerID
	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}).Error)
	seedMember(t, db, customerID, businessID, 0)
	bill := seedRedeemedBill(t, db, businessID, "UNDO-NOPTS-1", &redeemer, 0)

	err := UndoRedemption(db, customerID, bill.ID, businessID)
	require.ErrorIs(t, err, ErrCannotDetermineRefund)

	var after database.Bill
	require.NoError(t, db.First(&after, bill.ID).Error)
	require.Equal(t, int64(500), after.LoyaltyDiscountCents, "discount kept when the refund is unknown")
	require.Equal(t, 0, memberPoints(t, db, customerID, businessID))
}
