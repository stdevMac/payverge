package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFloorLoyaltyDB(t *testing.T) (*Business, *Table, *Table) {
	t.Helper()
	setupTableFloorTestDB(t)
	require.NoError(t, db.AutoMigrate(&CustomerBusiness{}))
	biz, source, target := seedFloorBizAndTables(t)
	require.NoError(t, db.Model(biz).Updates(map[string]interface{}{
		"tax_rate":         0,
		"service_fee_rate": 0,
	}).Error)
	return biz, source, target
}

func insertFloorItem(t *testing.T, billID uint, id, name string, price, subtotal float64, qty int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		id, billID, name, price, qty, subtotal, "menu_item", time.Now(),
	).Error)
}

func setBillLoyalty(t *testing.T, billID uint, discount int64, points int, customerID *uint) {
	t.Helper()
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", billID).Updates(map[string]interface{}{
		"loyalty_discount_cents":          discount,
		"loyalty_points_redeemed":         points,
		"loyalty_redeemed_by_customer_id": customerID,
	}).Error)
}

func seedLoyaltyCustomer(t *testing.T, businessID, customerID uint, points int) uint {
	t.Helper()
	cb := &CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: points,
		IsActive:      true,
	}
	require.NoError(t, db.Create(cb).Error)
	return cb.CustomerID
}

func mergeHistory(t *testing.T, billID uint) BillHistoryEvent {
	t.Helper()
	var ev BillHistoryEvent
	require.NoError(t, db.Where(
		"bill_id = ? AND event_type = ? AND reason = ?",
		billID, BillHistoryEventBillUpdated, "Merged from Live View",
	).First(&ev).Error)
	return ev
}

func TestMergeTableChecks_MovesSourceLoyaltyOntoClearTarget(t *testing.T) {
	biz, source, target := setupFloorLoyaltyDB(t)
	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)
	tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
	require.NoError(t, err)

	insertFloorItem(t, srcBill.ID, "src-20", "Steak", 20, 20, 1)
	insertFloorItem(t, tgtBill.ID, "tgt-10", "Salad", 10, 10, 1)
	customerID := seedLoyaltyCustomer(t, biz.ID, 1, 100)
	setBillLoyalty(t, srcBill.ID, 500, 50, &customerID)

	mergedTarget, mergedSource, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)
	require.NotNil(t, mergedTarget)
	require.NotNil(t, mergedSource)

	assert.Equal(t, int64(2500), mergedTarget.TotalAmount)
	assert.Equal(t, int64(500), mergedTarget.LoyaltyDiscountCents)
	assert.Equal(t, 50, mergedTarget.LoyaltyPointsRedeemed)
	require.NotNil(t, mergedTarget.LoyaltyRedeemedByCustomerID)
	assert.Equal(t, customerID, *mergedTarget.LoyaltyRedeemedByCustomerID)

	assert.Equal(t, BillStatusVoided, mergedSource.Status)
	assert.Nil(t, mergedSource.SettledAt)
	assert.Equal(t, int64(0), mergedSource.LoyaltyDiscountCents)
	assert.Equal(t, 0, mergedSource.LoyaltyPointsRedeemed)
	assert.Nil(t, mergedSource.LoyaltyRedeemedByCustomerID)

	var cb CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, biz.ID).First(&cb).Error)
	assert.Equal(t, 100, cb.LoyaltyPoints, "moving the redemption must not also credit the points")

	ev := mergeHistory(t, mergedTarget.ID)
	assert.Equal(t, true, ev.Details["loyalty_moved"])
	_, restored := ev.Details["loyalty_points_restored"]
	assert.False(t, restored)
}

func TestMergeTableChecks_RestoresSourcePointsWhenTargetAlreadyRedeemed(t *testing.T) {
	biz, source, target := setupFloorLoyaltyDB(t)
	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)
	tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
	require.NoError(t, err)

	insertFloorItem(t, srcBill.ID, "src-20", "Steak", 20, 20, 1)
	insertFloorItem(t, tgtBill.ID, "tgt-10", "Salad", 10, 10, 1)

	sourceCustomer := seedLoyaltyCustomer(t, biz.ID, 1, 100)
	targetCustomer := seedLoyaltyCustomer(t, biz.ID, 2, 80)
	setBillLoyalty(t, srcBill.ID, 500, 50, &sourceCustomer)
	setBillLoyalty(t, tgtBill.ID, 400, 40, &targetCustomer)

	mergedTarget, mergedSource, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)

	// 20.00 + 10.00, tax 0, minus the target's own 4.00 discount.
	assert.Equal(t, int64(2600), mergedTarget.TotalAmount)
	assert.Equal(t, int64(400), mergedTarget.LoyaltyDiscountCents)
	assert.Equal(t, 40, mergedTarget.LoyaltyPointsRedeemed)
	require.NotNil(t, mergedTarget.LoyaltyRedeemedByCustomerID)
	assert.Equal(t, targetCustomer, *mergedTarget.LoyaltyRedeemedByCustomerID)

	assert.Equal(t, int64(0), mergedSource.LoyaltyDiscountCents)
	assert.Equal(t, 0, mergedSource.LoyaltyPointsRedeemed)
	assert.Nil(t, mergedSource.LoyaltyRedeemedByCustomerID)

	var sourceCB, targetCB CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", sourceCustomer, biz.ID).First(&sourceCB).Error)
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", targetCustomer, biz.ID).First(&targetCB).Error)
	assert.Equal(t, 150, sourceCB.LoyaltyPoints)
	assert.Equal(t, 80, targetCB.LoyaltyPoints)

	ev := mergeHistory(t, mergedTarget.ID)
	assert.Equal(t, float64(50), ev.Details["loyalty_points_restored"])
	_, moved := ev.Details["loyalty_moved"]
	assert.False(t, moved)
}

func TestMergeTableChecks_PreservesTargetLoyaltyDiscount(t *testing.T) {
	biz, source, target := setupFloorLoyaltyDB(t)
	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)
	tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
	require.NoError(t, err)

	insertFloorItem(t, srcBill.ID, "src-20", "Steak", 20, 20, 1)
	insertFloorItem(t, tgtBill.ID, "tgt-10", "Salad", 10, 10, 1)
	setBillLoyalty(t, tgtBill.ID, 300, 0, nil)

	mergedTarget, mergedSource, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)

	assert.Equal(t, int64(2700), mergedTarget.TotalAmount)
	assert.Equal(t, int64(300), mergedTarget.LoyaltyDiscountCents)
	assert.Equal(t, 0, mergedTarget.LoyaltyPointsRedeemed)
	assert.Nil(t, mergedTarget.LoyaltyRedeemedByCustomerID)
	assert.Equal(t, int64(0), mergedSource.LoyaltyDiscountCents)
	assert.Nil(t, mergedSource.LoyaltyRedeemedByCustomerID)

	ev := mergeHistory(t, mergedTarget.ID)
	_, moved := ev.Details["loyalty_moved"]
	_, restored := ev.Details["loyalty_points_restored"]
	assert.False(t, moved)
	assert.False(t, restored)
}

// When the target already holds a redemption, the source's redemption can only
// survive the merge by going back to the guest. If there is nowhere to return
// it — no points/redeemer behind the discount, or no active membership row —
// the merge is refused instead of dropping the value.
func TestMergeTableChecks_RefusesWhenSourceLoyaltyCannotBeReturned(t *testing.T) {
	cases := []struct {
		name   string
		points int
	}{
		{name: "discount_without_points_or_redeemer", points: 0},
		{name: "redeemer_without_active_membership", points: 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			biz, source, target := setupFloorLoyaltyDB(t)
			srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
			require.NoError(t, err)
			tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
			require.NoError(t, err)
			insertFloorItem(t, srcBill.ID, "src-20", "Steak", 20, 20, 1)
			insertFloorItem(t, tgtBill.ID, "tgt-10", "Salad", 10, 10, 1)

			targetCustomer := seedLoyaltyCustomer(t, biz.ID, 2, 80)
			setBillLoyalty(t, tgtBill.ID, 400, 40, &targetCustomer)
			if tc.points > 0 {
				sourceCustomer := uint(1)
				setBillLoyalty(t, srcBill.ID, 500, tc.points, &sourceCustomer)
			} else {
				setBillLoyalty(t, srcBill.ID, 500, 0, nil)
			}

			_, _, err = MergeTableChecks(MergeTablesInput{
				BusinessID:    biz.ID,
				SourceTableID: source.ID,
				TargetTableID: target.ID,
				Actor:         "host",
			})
			require.ErrorIs(t, err, ErrFloorMergeLoyaltyBlock)

			var src Bill
			require.NoError(t, db.First(&src, srcBill.ID).Error)
			assert.Equal(t, BillStatusOpen, src.Status, "refused merge leaves the source check alone")
			assert.Equal(t, int64(500), src.LoyaltyDiscountCents)
		})
	}
}
