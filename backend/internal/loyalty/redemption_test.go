package loyalty

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLoyaltyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:loyalty-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Customer{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
		&database.Bill{},
		&database.Counter{},
	))
	return db
}

// TestRedeemPointsBakesDiscountIntoBillTotal is the core fix: redeeming points
// must reduce the bill's TotalAmount (what is owed), not merely record a discount
// that nobody applies. Without this the guest spends points and still pays full
// price, because every payment-owed calculation reads TotalAmount.
func TestRedeemPointsBakesDiscountIntoBillTotal(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100, // 500 pts => $5.00 => 500¢
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)
	// Gross bill: subtotal 2000¢ + tax 160¢ + fee 100¢ = total 2260¢.
	bill := &database.Bill{
		BusinessID:       businessID,
		TableID:          1,
		BillNumber:       "LOYALTY-TOTAL-1",
		Items:            "[]",
		Status:           database.BillStatusOpen,
		Subtotal:         2000,
		TaxAmount:        160,
		ServiceFeeAmount: 100,
		TotalAmount:      2260,
	}
	require.NoError(t, db.Create(bill).Error)

	dc, _, _, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.NoError(t, err)
	require.Equal(t, int64(500), dc)

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(500), got.LoyaltyDiscountCents, "discount must be recorded")
	require.Equal(t, int64(1760), got.TotalAmount, "total must be net of discount (2260 - 500)")
	// Gross components are untouched so the breakdown still reconstructs.
	require.Equal(t, int64(2000), got.Subtotal)
	require.Equal(t, int64(160), got.TaxAmount)
	require.Equal(t, int64(100), got.ServiceFeeAmount)
	require.Nil(t, got.ClosedAt, "a redemption that leaves a balance due must not close the bill")
}

// TestUndoRedemptionRestoresBillTotal proves the redeem→undo round trip restores
// the gross total exactly, not just the points — otherwise undoing a redemption
// would leave the bill permanently discounted.
func TestUndoRedemptionRestoresBillTotal(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID:       businessID,
		TableID:          1,
		BillNumber:       "LOYALTY-TOTAL-2",
		Items:            "[]",
		Status:           database.BillStatusOpen,
		Subtotal:         2000,
		TaxAmount:        160,
		ServiceFeeAmount: 100,
		TotalAmount:      2260,
	}
	require.NoError(t, db.Create(bill).Error)

	_, _, _, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.NoError(t, err)
	require.NoError(t, UndoRedemption(db, customerID, bill.ID, businessID))

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(0), got.LoyaltyDiscountCents, "discount cleared")
	require.Equal(t, int64(2260), got.TotalAmount, "total restored to gross")
}

// TestRedeemDiscountFlooredAtBillTotal guards the over-redemption edge: a discount
// worth more than the bill must zero the total, never make the guest owe a
// negative (credit) amount — AND must consume only the points the bill's value is
// worth, never the guest's whole balance (the surplus-point-burn fix). With rate
// 100 pts/$ and a 300¢ bill, redeeming 500 points must spend only 300 points
// (worth $3.00) and leave the other 200 intact.
func TestRedeemDiscountFlooredAtBillTotal(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100, // 500 pts => 500¢ discount, bill is only 300¢
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID:  businessID,
		TableID:     1,
		BillNumber:  "LOYALTY-TOTAL-3",
		Items:       "[]",
		Status:      database.BillStatusOpen,
		Subtotal:    300,
		TotalAmount: 300,
	}
	require.NoError(t, db.Create(bill).Error)

	dc, pointsDeducted, remaining, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.NoError(t, err)
	require.Equal(t, int64(300), dc, "discount clamped to the 300¢ bill")
	require.Equal(t, 300, pointsDeducted, "only the points worth the bill are spent, not all 500")
	require.Equal(t, 200, remaining, "surplus points stay on the balance — never burned")

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(0), got.TotalAmount, "total floored at zero, never negative")
	require.Equal(t, 300, got.LoyaltyPointsRedeemed, "bill records the clamped points for void/undo")

	// The unspent surplus really is still redeemable: it survives in the balance.
	var cb database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	require.Equal(t, 200, cb.LoyaltyPoints)
}

// TestRedeemPointsRecordsRedeemerForVoidRefund proves the redeem path records who
// spent how many points on the bill, so a later void can return them. Without
// this linkage the void path has no way to know whom to credit.
func TestRedeemPointsRecordsRedeemerForVoidRefund(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID:  businessID,
		TableID:     1,
		BillNumber:  "LOYALTY-REDEEMER-1",
		Items:       "[]",
		Status:      database.BillStatusOpen,
		Subtotal:    2000,
		TotalAmount: 2000,
	}
	require.NoError(t, db.Create(bill).Error)

	_, _, _, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.NoError(t, err)

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.NotNil(t, got.LoyaltyRedeemedByCustomerID, "redeemer must be recorded for void-refund")
	require.Equal(t, customerID, *got.LoyaltyRedeemedByCustomerID)
	require.Equal(t, 500, got.LoyaltyPointsRedeemed, "exact points spent must be recorded")

	// Undo must clear the redeemer/points so a later void can't double-refund.
	require.NoError(t, UndoRedemption(db, customerID, bill.ID, businessID))
	var afterUndo database.Bill
	require.NoError(t, db.First(&afterUndo, bill.ID).Error)
	require.Nil(t, afterUndo.LoyaltyRedeemedByCustomerID, "undo must clear the redeemer")
	require.Equal(t, 0, afterUndo.LoyaltyPointsRedeemed, "undo must clear the recorded points")
}

// TestRedeemThenUndoIsPointsNeutral_NonDivisorRate guards against silent point
// loss on a redeem→undo round-trip. With a non-divisor rate (3 points/$), the
// discount rounds to whole cents on redeem; UndoRedemption reverse-computes the
// points from those cents, and must use the SAME rounding discipline the redeem
// path documents ("Round, don't truncate") — otherwise the customer gets back
// fewer points than they redeemed.
func TestRedeemThenUndoIsPointsNeutral_NonDivisorRate(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           3, // non-divisor: 10 pts => $3.333... => 333¢
		RedemptionPointsPerDollar: 3,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 10,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID: businessID,
		TableID:    1,
		BillNumber: "LOYALTY-1",
		Items:      "[]",
		Status:     database.BillStatusOpen,
		Subtotal:   2000, // headroom so the 333¢ discount is not clamped to the bill
	}
	require.NoError(t, db.Create(bill).Error)

	// Redeem all 10 points: discount = Round(10/3 * 100) = 333¢.
	dc, _, remaining, err := RedeemPoints(db, customerID, bill.ID, businessID, 10)
	require.NoError(t, err)
	require.Equal(t, int64(333), dc)
	require.Equal(t, 0, remaining)

	// Undo must restore the FULL 10 points — never silently lose one to a
	// round-trip rounding asymmetry.
	require.NoError(t, UndoRedemption(db, customerID, bill.ID, businessID))

	var cb database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	require.Equal(t, 10, cb.LoyaltyPoints, "redeem→undo must be points-neutral (got %d, want 10)", cb.LoyaltyPoints)
}

// TestRedeemThenUndoIsPointsNeutral_DivisorRate is the regression guard for the
// common path: a rate that divides cleanly into cents must stay points-neutral
// (math.Round and the old int() agree on whole numbers, so the fix is safe here).
func TestRedeemThenUndoIsPointsNeutral_DivisorRate(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           10, // 50 pts => $5.00 => 500¢, clean
		RedemptionPointsPerDollar: 10,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 50,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID: businessID,
		TableID:    1,
		BillNumber: "LOYALTY-2",
		Items:      "[]",
		Status:     database.BillStatusOpen,
		Subtotal:   2000, // headroom so the 500¢ discount is not clamped to the bill
	}
	require.NoError(t, db.Create(bill).Error)

	dc, _, remaining, err := RedeemPoints(db, customerID, bill.ID, businessID, 50)
	require.NoError(t, err)
	require.Equal(t, int64(500), dc)
	require.Equal(t, 0, remaining)

	require.NoError(t, UndoRedemption(db, customerID, bill.ID, businessID))

	var cb database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	require.Equal(t, 50, cb.LoyaltyPoints)
}

// TestSecondRedemptionRejectedOnSharedBill is the money fix for the shared-bill
// overwrite bug. A restaurant table shares ONE open bill across every guest
// seated there. If guest A redeems and then guest B redeems on the same bill, an
// unconditional discount write would overwrite A's redeemer record and shrink the
// discount to B's — A's points were already deducted but the bill would record
// only B, so a later void refunds only B and silently loses A's points. The fix
// enforces "at most one active redemption per bill": B's redemption is rejected,
// A's redeemer/points/discount are untouched, and B's points are NOT deducted
// (the transaction rolls the deduction back).
func TestSecondRedemptionRejectedOnSharedBill(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID uint = 1
	const customerA, customerB uint = 1, 2

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100, // 100 pts => $1.00 => 100¢
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerA,
		BusinessID:    businessID,
		LoyaltyPoints: 100,
		IsActive:      true,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerB,
		BusinessID:    businessID,
		LoyaltyPoints: 50,
		IsActive:      true,
	}).Error)
	// One shared open bill for the whole table.
	bill := &database.Bill{
		BusinessID:  businessID,
		TableID:     1,
		BillNumber:  "SHARED-BILL-1",
		Items:       "[]",
		Status:      database.BillStatusOpen,
		Subtotal:    2000,
		TotalAmount: 2000,
	}
	require.NoError(t, db.Create(bill).Error)

	// Guest A redeems 100 points => 100¢ discount.
	dc, _, _, err := RedeemPoints(db, customerA, bill.ID, businessID, 100)
	require.NoError(t, err)
	require.Equal(t, int64(100), dc)

	// Guest B redeems 50 points on the SAME bill: must be rejected.
	_, _, _, err = RedeemPoints(db, customerB, bill.ID, businessID, 50)
	require.Error(t, err, "second redemption on an already-discounted bill must be rejected")
	require.Contains(t, err.Error(), "already applied")

	// Bill still records guest A's redemption exactly — discount, points, redeemer.
	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(100), got.LoyaltyDiscountCents, "A's discount must be untouched")
	require.Equal(t, int64(1900), got.TotalAmount, "total must still reflect A's discount (2000 - 100)")
	require.NotNil(t, got.LoyaltyRedeemedByCustomerID)
	require.Equal(t, customerA, *got.LoyaltyRedeemedByCustomerID, "A must remain the redeemer")
	require.Equal(t, 100, got.LoyaltyPointsRedeemed, "A's recorded points must be untouched")

	// Guest A's points were deducted exactly once; guest B's were NOT deducted
	// (the rejected redemption's point deduction rolled back).
	var cbA, cbB database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerA, businessID).First(&cbA).Error)
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerB, businessID).First(&cbB).Error)
	require.Equal(t, 0, cbA.LoyaltyPoints, "A spent 100 of 100")
	require.Equal(t, 50, cbB.LoyaltyPoints, "B's 50 points must NOT have been deducted (rolled back)")
}

// TestUndoRedemptionRejectedForNonRedeemer is the security fix for the
// undo-and-steal bug. A table shares one open bill; without an ownership guard any
// authenticated guest could undo another guest's redemption and pocket the
// re-credited points. The fix requires the caller to be the recorded redeemer:
// a non-redeemer's undo is rejected, the discount stays, and neither guest's point
// balance changes. The original redeemer can still undo normally.
func TestUndoRedemptionRejectedForNonRedeemer(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID uint = 1
	const customerA, customerB uint = 1, 2

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerA,
		BusinessID:    businessID,
		LoyaltyPoints: 100,
		IsActive:      true,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerB,
		BusinessID:    businessID,
		LoyaltyPoints: 0,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID:  businessID,
		TableID:     1,
		BillNumber:  "SHARED-BILL-2",
		Items:       "[]",
		Status:      database.BillStatusOpen,
		Subtotal:    2000,
		TotalAmount: 2000,
	}
	require.NoError(t, db.Create(bill).Error)

	// Guest A redeems 100 points (now A has 0 in this business).
	_, _, _, err := RedeemPoints(db, customerA, bill.ID, businessID, 100)
	require.NoError(t, err)

	// Guest B (the non-redeemer) tries to undo A's redemption: must be rejected.
	err = UndoRedemption(db, customerB, bill.ID, businessID)
	require.Error(t, err, "a non-redeemer must not be able to undo another guest's redemption")
	require.Contains(t, err.Error(), "only the guest who applied")

	// Discount and redeemer unchanged; neither balance moved (B did NOT steal points).
	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(100), got.LoyaltyDiscountCents, "discount must remain after a rejected undo")
	require.NotNil(t, got.LoyaltyRedeemedByCustomerID)
	require.Equal(t, customerA, *got.LoyaltyRedeemedByCustomerID)

	var cbA, cbB database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerA, businessID).First(&cbA).Error)
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerB, businessID).First(&cbB).Error)
	require.Equal(t, 0, cbA.LoyaltyPoints, "A's balance unchanged by B's rejected undo")
	require.Equal(t, 0, cbB.LoyaltyPoints, "B must NOT have received A's points")

	// The original redeemer (A) can still undo — points return to A, not B.
	require.NoError(t, UndoRedemption(db, customerA, bill.ID, businessID))
	var afterUndo database.Bill
	require.NoError(t, db.First(&afterUndo, bill.ID).Error)
	require.Equal(t, int64(0), afterUndo.LoyaltyDiscountCents, "redeemer's undo clears the discount")
	require.Nil(t, afterUndo.LoyaltyRedeemedByCustomerID)
	require.Equal(t, int64(2000), afterUndo.TotalAmount, "total restored to gross")

	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerA, businessID).First(&cbA).Error)
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerB, businessID).First(&cbB).Error)
	require.Equal(t, 100, cbA.LoyaltyPoints, "A's 100 points returned to A")
	require.Equal(t, 0, cbB.LoyaltyPoints, "B still has nothing")
}

// TestRedemptionRate_NoSilentDefault (B15) proves the silent 100-point default is
// gone: a program with RedemptionPointsPerDollar == 0, or a disabled program, or a
// nil program must yield rate 0 ("not configured"), NOT the old 100 fallback.
// Callers treat 0 as an error condition.
func TestRedemptionRate_NoSilentDefault(t *testing.T) {
	// Unset rate on an enabled program → 0, not 100.
	require.Equal(t, 0.0, redemptionRate(&database.LoyaltyProgram{Enabled: true, PointsPerDollar: 1, RedemptionPointsPerDollar: 0}),
		"an enabled program with no rate must yield 0, not the old 100 default")

	// Disabled program (regardless of rate) → 0.
	require.Equal(t, 0.0, redemptionRate(&database.LoyaltyProgram{Enabled: false, PointsPerDollar: 1, RedemptionPointsPerDollar: 50}),
		"a disabled program must yield 0")

	// Nil program → 0.
	require.Equal(t, 0.0, redemptionRate(nil), "a nil program must yield 0")

	// A correctly configured program still returns its real rate unchanged.
	require.Equal(t, 25.0, redemptionRate(&database.LoyaltyProgram{Enabled: true, PointsPerDollar: 1, RedemptionPointsPerDollar: 25}),
		"a configured program must return its exact rate")
}

// TestRedeemPoints_ErrorsWhenRateUnset (B15) proves RedeemPoints refuses to redeem
// when an enabled program has no configured rate, instead of silently falling back
// to 100 points/$. No points are deducted and no discount is recorded.
func TestRedeemPoints_ErrorsWhenRateUnset(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	// Enabled program but the redeem rate is unconfigured
	// (RedemptionPointsPerDollar == 0). GORM's `default:100` tag rewrites a
	// struct's zero value on insert, so force the 0 with an explicit UPDATE.
	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           1,
		RedemptionPointsPerDollar: 1,
	}).Error)
	require.NoError(t, db.Model(&database.LoyaltyProgram{}).
		Where("business_id = ?", businessID).
		Update("redemption_points_per_dollar", 0).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID:  businessID,
		TableID:     1,
		BillNumber:  "RATE-UNSET-1",
		Items:       "[]",
		Status:      database.BillStatusOpen,
		Subtotal:    2000,
		TotalAmount: 2000,
	}
	require.NoError(t, db.Create(bill).Error)

	dc, _, remaining, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.Error(t, err, "redeeming with an unconfigured rate must error, not use the old 100 default")
	require.Contains(t, err.Error(), "redemption rate", "error must mention the redemption rate is not configured")
	require.Equal(t, int64(0), dc, "no discount on a failed redemption")
	require.Equal(t, 0, remaining)

	// Nothing must have changed: points untouched, no discount recorded on the bill.
	var cb database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	require.Equal(t, 500, cb.LoyaltyPoints, "points must NOT be deducted when the rate is unconfigured")

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(0), got.LoyaltyDiscountCents, "no discount may be recorded when the rate is unconfigured")
	require.Equal(t, int64(2000), got.TotalAmount, "bill total must be untouched")
}

// TestUndoRedemptionAfterRateCleared (B15) proves undo is rate-independent: it
// sources the points actually recorded at redeem time, so it stays correct even if
// the rate was changed or cleared between redeem and undo. Here we redeem at a real
// rate, then DISABLE the program, then undo — the exact redeemed points must still
// be returned even though redemptionRate(program) is now 0.
func TestUndoRedemptionAfterRateCleared(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	program := &database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)
	bill := &database.Bill{
		BusinessID:  businessID,
		TableID:     1,
		BillNumber:  "RATE-CLEARED-1",
		Items:       "[]",
		Status:      database.BillStatusOpen,
		Subtotal:    2000,
		TotalAmount: 2000,
	}
	require.NoError(t, db.Create(bill).Error)

	_, _, _, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.NoError(t, err)

	// Operator clears/disables the rate AFTER the redemption.
	require.NoError(t, db.Model(&database.LoyaltyProgram{}).
		Where("business_id = ?", businessID).
		Updates(map[string]interface{}{"enabled": false, "points_per_dollar": 0}).Error)

	// Undo must still return the EXACT 500 points recorded at redeem time, not a
	// rate-based recompute (which would now be 0 and silently lose the points).
	require.NoError(t, UndoRedemption(db, customerID, bill.ID, businessID))

	var cb database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	require.Equal(t, 500, cb.LoyaltyPoints, "undo must restore the exact recorded points even after the rate was cleared")

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Equal(t, int64(0), got.LoyaltyDiscountCents, "discount cleared")
	require.Equal(t, int64(2000), got.TotalAmount, "total restored to gross")
}

// TestRedeemPointsClampsToUnpaidRemainder: on a partially paid bill the discount
// must clamp to the unpaid remainder so total_amount never drops below paid_amount.
// When payments + discount cover the bill, status settles to paid.
func TestRedeemPointsClampsToUnpaidRemainder(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	// Seed business/customer/program exactly like the first test in this file:
	// LoyaltyProgram{BusinessID: businessID, Enabled: true, PointsPerDollar: 100},
	// CustomerBusiness{CustomerID: customerID, BusinessID: businessID, LoyaltyPoints: 5000, IsActive: true}.
	// (Reuse/copy that seed block; only the bill differs.)
	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 5000,
		IsActive:      true,
	}).Error)
	bill := database.Bill{
		BusinessID:       businessID,
		TableID:          1,
		BillNumber:       "LOYALTY-PARTIAL-1",
		Items:            "[]",
		Status:           database.BillStatusPartial,
		Subtotal:         2000,
		TaxAmount:        160,
		ServiceFeeAmount: 100,
		TotalAmount:      2260,
		PaidAmount:       2000, // only 260¢ still owed
	}
	require.NoError(t, db.Create(&bill).Error)
	counter := database.Counter{
		BusinessID: businessID, CounterNumber: 1, Name: "Loyalty Counter",
		IsActive: true, CurrentBillID: &bill.ID,
	}
	require.NoError(t, db.Create(&counter).Error)
	require.NoError(t, db.Model(&bill).Update("counter_id", counter.ID).Error)

	hookFires := 0
	database.SetBillPaidInTxHook(func(tx *gorm.DB, paidBill *database.Bill, paymentID, alternativePaymentID *uint) error {
		hookFires++
		require.Equal(t, bill.ID, paidBill.ID)
		require.Nil(t, paymentID, "loyalty settlement creates no new payment")
		require.Nil(t, alternativePaymentID, "loyalty settlement creates no new alternative payment")
		return nil
	})
	t.Cleanup(func() { database.SetBillPaidInTxHook(nil) })

	dc, _, _, err := RedeemPoints(db, customerID, bill.ID, businessID, 5000)
	require.NoError(t, err)
	require.LessOrEqual(t, dc, int64(260), "discount must never eat into already-collected money")

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.GreaterOrEqual(t, got.TotalAmount, got.PaidAmount,
		"total_amount must never drop below paid_amount")
	require.Equal(t, database.BillStatusPaid, got.Status,
		"a bill fully covered by payments + discount must settle")
	require.NotNil(t, got.ClosedAt,
		"a loyalty-settled bill must stamp closed_at — closed-bill analytics, feedback emails, and ledger classification key on it")
	require.NotNil(t, got.SettledAt,
		"a loyalty-settled bill must stamp the canonical settlement time")
	require.Equal(t, 1, hookFires,
		"a loyalty-settled bill must invoke the paid transition hook exactly once")
	var released database.Counter
	require.NoError(t, db.First(&released, counter.ID).Error)
	require.Nil(t, released.CurrentBillID,
		"a loyalty-settled bill must release counter occupancy")
}

// TestRedeemPointsRejectsFullyPaidBill: when paid_amount already covers the gross
// total there is no remainder to discount — redemption must be rejected.
func TestRedeemPointsRejectsFullyPaidBill(t *testing.T) {
	db := setupLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	// Same seed block as above.
	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           100,
		RedemptionPointsPerDollar: 100,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 5000,
		IsActive:      true,
	}).Error)
	bill := database.Bill{
		BusinessID:       businessID,
		TableID:          1,
		BillNumber:       "LOYALTY-FULLPAID-1",
		Items:            "[]",
		Status:           database.BillStatusPartial, // stale status, but amounts say fully paid
		Subtotal:         2000,
		TaxAmount:        160,
		ServiceFeeAmount: 100,
		TotalAmount:      2260,
		PaidAmount:       2260,
	}
	require.NoError(t, db.Create(&bill).Error)

	_, _, _, err := RedeemPoints(db, customerID, bill.ID, businessID, 500)
	require.Error(t, err, "no remainder to discount — redemption must be rejected")

	var got database.Bill
	require.NoError(t, db.First(&got, bill.ID).Error)
	require.Zero(t, got.LoyaltyDiscountCents)
}
