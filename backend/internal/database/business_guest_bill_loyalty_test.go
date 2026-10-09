package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetPublicBillByToken_ProjectsLoyaltyDiscount locks the fix for the
// loyalty discount vanishing on bill reload: the public guest projection
// omitted loyalty_discount_cents / loyalty_points_redeemed /
// loyalty_redeemed_by_customer_id, so a re-fetched bill returned them as zero
// and the guest's "Loyalty discount -$X" line (gated on >0) disappeared —
// leaving an unexplained subtotal-vs-total gap at the moment of payment.
func TestGetPublicBillByToken_ProjectsLoyaltyDiscount(t *testing.T) {
	_, _, publicToken := setupGuestBillAccessDB(t)

	customerID := uint(4242)
	require.NoError(t, db.Model(&Bill{}).
		Where("public_token = ?", publicToken).
		Updates(map[string]any{
			"loyalty_discount_cents":          500,
			"loyalty_points_redeemed":         500,
			"loyalty_redeemed_by_customer_id": customerID,
		}).Error)

	bill, _, err := GetPublicBillByToken(publicToken)
	require.NoError(t, err)
	require.NotNil(t, bill)

	assert.EqualValues(t, 500, bill.LoyaltyDiscountCents,
		"loyalty discount must survive the public projection so the discount line renders on reload")
	assert.Equal(t, 500, bill.LoyaltyPointsRedeemed)
	if assert.NotNil(t, bill.LoyaltyRedeemedByCustomerID) {
		assert.EqualValues(t, customerID, *bill.LoyaltyRedeemedByCustomerID)
	}
}

// TestGetPublicGuestOpenBillByTableID_ProjectsLoyaltyDiscount locks the
// table-code bill/status surfaces: those routes share a narrower Select that
// used to drop loyalty_discount_cents, so GET /guest/table/:code/bill and
// /status always re-emitted 0 after a redemption.
func TestGetPublicGuestOpenBillByTableID_ProjectsLoyaltyDiscount(t *testing.T) {
	businessID, _, publicToken := setupGuestBillAccessDB(t)

	table := &Table{
		BusinessID: businessID,
		Name:       "Loyalty",
		TableCode:  "LOYOPEN01",
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	customerID := uint(4242)
	require.NoError(t, db.Model(&Bill{}).
		Where("public_token = ?", publicToken).
		Updates(map[string]any{
			"table_id":                        table.ID,
			"status":                          BillStatusOpen,
			"loyalty_discount_cents":          275,
			"loyalty_points_redeemed":         275,
			"loyalty_redeemed_by_customer_id": customerID,
		}).Error)

	bill, _, err := GetPublicGuestOpenBillByTableID(table.ID)
	require.NoError(t, err)
	require.NotNil(t, bill)

	assert.EqualValues(t, 275, bill.LoyaltyDiscountCents,
		"open-by-table projection must include loyalty_discount_cents so guest status/bill can emit dollars")
}
