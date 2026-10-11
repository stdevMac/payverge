package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// L2-3: an offer-derived discount line must be recomputed when the bill's
// lines change. The audited production behavior: editing a line's quantity
// recomputed the subtotal but left the automatic discount pinned to its
// pre-edit absolute value, so the bill charged the wrong amount.

func helperOffer(t *testing.T, businessID uint, discountType string, value float64) *Offer {
	t.Helper()
	offer := &Offer{
		BusinessID:    businessID,
		Name:          fmt.Sprintf("%s %.0f", discountType, value),
		DiscountType:  discountType,
		DiscountValue: value,
		IsActive:      true,
		ApplicableTo:  "all",
	}
	require.NoError(t, db.Create(offer).Error)
	return offer
}

func offerDiscountLine(id string, offerID uint, orderID *uint, amount float64) BillItem {
	return BillItem{
		ID:            id,
		MenuItemID:    fmt.Sprintf("offer:%d", offerID),
		Name:          "Offer discount",
		Price:         -amount,
		Quantity:      1,
		Subtotal:      -amount,
		ItemType:      "discount",
		SourceOfferID: &offerID,
		OrderID:       orderID,
	}
}

func findBillItem(items []BillItem, id string) *BillItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func TestAdjustBillItem_PercentageOfferDiscountRecomputesOnQtyEdit(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	offer := helperOffer(t, biz.ID, "percentage", 10)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "burger", Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item"},
		offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 1),
	}, 9)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a001", "manager", intPtr(3), false, "")
	require.NoError(t, err)

	disc := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001")
	require.NotNil(t, disc, "discount line must survive while eligible items remain")
	require.InDelta(t, -3.0, disc.Subtotal, 0.001,
		"10%% offer must recompute against the new $30 subtotal, not stay frozen at -$1")
	require.Equal(t, int64(2700), updatedBill.Subtotal,
		"bill subtotal must be $30 items - $3 recomputed discount")
	require.Equal(t, int64(2700), updatedBill.TotalAmount)
}

func TestAdjustBillItem_VoidLastEligibleItemDropsOfferDiscount(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	offer := helperOffer(t, biz.ID, "percentage", 10)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "burger", Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item"},
		offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 1),
	}, 9)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a001", "manager", nil, true, "guest cancelled")
	require.NoError(t, err)

	require.Nil(t, findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001"),
		"voiding the only eligible item must drop the derived discount, not leave a negative-only bill")
	require.Equal(t, int64(0), updatedBill.Subtotal)
	require.Equal(t, int64(0), updatedBill.TotalAmount)
}

func TestAdjustBillItem_FixedOfferDiscountDoublesWhenQtyIncreases(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	target := "item-bife"
	offer := &Offer{
		BusinessID:    biz.ID,
		Name:          "AR$ 3.000 menos en el bife",
		DiscountType:  "fixed",
		DiscountValue: 3000,
		IsActive:      true,
		ApplicableTo:  "item",
		TargetID:      &target,
	}
	require.NoError(t, db.Create(offer).Error)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "item-bife", Name: "Bife", Price: 34000, Quantity: 1, Subtotal: 34000, ItemType: "menu_item"},
		offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 3000),
	}, 31000)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a001", "manager", intPtr(2), false, "")
	require.NoError(t, err)

	disc := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001")
	require.NotNil(t, disc, "discount line must survive the qty edit")
	require.InDelta(t, -6000.0, disc.Subtotal, 0.001,
		"raising qty 1→2 on a qualifying plate must double a 3000 fixed offer")
	require.Equal(t, int64(6200000), updatedBill.Subtotal,
		"bill subtotal must be 2×34000 items − 6000 recomputed discount")
	require.Equal(t, int64(6200000), updatedBill.TotalAmount)
}

func TestAdjustBillItem_FixedOfferDiscountQtyIncreaseRecapsToEligibleSubtotal(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	target := "item-cheap-bife"
	offer := &Offer{
		BusinessID:    biz.ID,
		Name:          "AR$ 3.000 menos en el bife",
		DiscountType:  "fixed",
		DiscountValue: 3000,
		IsActive:      true,
		ApplicableTo:  "item",
		TargetID:      &target,
	}
	require.NoError(t, db.Create(offer).Error)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "item-cheap-bife", Name: "Cheap Bife", Price: 1000, Quantity: 1, Subtotal: 1000, ItemType: "menu_item"},
		offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 1000),
	}, 0)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a001", "manager", intPtr(2), false, "")
	require.NoError(t, err)

	disc := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001")
	require.NotNil(t, disc)
	require.InDelta(t, -2000.0, disc.Subtotal, 0.001,
		"2×3000 off must recap to the 2000 remaining eligible subtotal")
	require.Equal(t, int64(0), updatedBill.Subtotal)
	require.Equal(t, int64(0), updatedBill.TotalAmount)
}

func TestAdjustBillItem_FixedOfferDiscountRecapsToRemainingEligibleSubtotal(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	offer := helperOffer(t, biz.ID, "fixed", 5)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "burger", Name: "Burger", Price: 3, Quantity: 1, Subtotal: 3, ItemType: "menu_item"},
		{ID: "00000000-0000-4000-8000-00000000a002", MenuItemID: "fries", Name: "Fries", Price: 6, Quantity: 1, Subtotal: 6, ItemType: "menu_item"},
		offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 5),
	}, 4)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a002", "manager", nil, true, "wrong order")
	require.NoError(t, err)

	disc := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001")
	require.NotNil(t, disc)
	require.InDelta(t, -3.0, disc.Subtotal, 0.001,
		"a $5 fixed discount must re-cap to the $3 of eligible items left on the bill")
	require.Equal(t, int64(0), updatedBill.Subtotal)
	require.Equal(t, int64(0), updatedBill.TotalAmount)
}

func TestAdjustBillItem_PercentageDiscountRecomputesWithinItsOrderCohort(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	offer := helperOffer(t, biz.ID, "percentage", 10)
	orderA := uint(1)
	orderB := uint(2)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a00a", MenuItemID: "burger", Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item", OrderID: &orderA},
		offerDiscountLine("00000000-0000-4000-8000-00000000d00a", offer.ID, &orderA, 1),
		{ID: "00000000-0000-4000-8000-00000000a00b", MenuItemID: "fries", Name: "Fries", Price: 20, Quantity: 1, Subtotal: 20, ItemType: "menu_item", OrderID: &orderB},
		offerDiscountLine("00000000-0000-4000-8000-00000000d00b", offer.ID, &orderB, 2),
	}, 27)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a00a", "manager", intPtr(2), false, "")
	require.NoError(t, err)

	discA := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d00a")
	require.NotNil(t, discA)
	require.InDelta(t, -2.0, discA.Subtotal, 0.001,
		"order A's discount must recompute against order A's lines only (10%% of $20)")

	discB := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d00b")
	require.NotNil(t, discB)
	require.InDelta(t, -2.0, discB.Subtotal, 0.001,
		"order B's discount must not be re-derived from the whole bill — that would double-count the shared offer")

	require.Equal(t, int64(3600), updatedBill.Subtotal,
		"$20 + $20 items - $2 - $2 cohort-scoped discounts")
}

func TestOfferMatchesBillLine_ItemScopedUsesCatalogIDNotName(t *testing.T) {
	target := "demo-steak"
	offer := &Offer{ApplicableTo: "item", TargetID: &target}
	noopCategory := func(string, string) (string, string) { return "", "" }

	tea := &BillItem{ItemType: "menu_item", MenuItemID: "demo-tea", Name: "Steak Plate", Subtotal: 5}
	steak := &BillItem{ItemType: "menu_item", MenuItemID: "demo-steak", Name: "Iced Tea", Subtotal: 42}

	require.False(t, offerMatchesBillLine(offer, tea, noopCategory),
		"forged steak name on tea must not match a steak-id offer")
	require.True(t, offerMatchesBillLine(offer, steak, noopCategory),
		"steak catalog id must match even if the stored name is wrong")

	nameTarget := "Steak Plate"
	nameOffer := &Offer{ApplicableTo: "item", TargetID: &nameTarget}
	require.False(t, offerMatchesBillLine(nameOffer, tea, noopCategory),
		"item offers must not match the client/stored name")
}

// #946 guard: a fixed offer whose scope is the whole check ("all", or the
// legacy empty string the offer form used to persist) is "$X off the check".
// Per-plate scaling must not apply to it, or an $8-off-the-check offer becomes
// $24 on a three-plate order.
func TestAdjustBillItem_FixedCheckWideOfferDoesNotScaleWithQty(t *testing.T) {
	for _, scope := range []string{"all", ""} {
		scope := scope
		name := scope
		if name == "" {
			name = "legacy-empty"
		}
		t.Run(name, func(t *testing.T) {
			setupOrderTestDB(t)
			biz := helperBusiness(t, 0, 0)
			offer := &Offer{
				BusinessID:    biz.ID,
				Name:          "$8 off the check",
				DiscountType:  "fixed",
				DiscountValue: 8,
				IsActive:      true,
				ApplicableTo:  scope,
			}
			require.NoError(t, db.Create(offer).Error)

			bill := helperBill(t, biz, []BillItem{
				{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "burger", Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item"},
				offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 8),
			}, 2)

			updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a001", "manager", intPtr(3), false, "")
			require.NoError(t, err)

			disc := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001")
			require.NotNil(t, disc, "check-wide discount line must survive the qty edit")
			require.InDelta(t, -8.0, disc.Subtotal, 0.001,
				"$8 off the check must stay $8 on 3 plates, not scale to $24")
			require.Equal(t, int64(2200), updatedBill.Subtotal,
				"bill subtotal must be $30 items - $8 check-wide discount")
			require.Equal(t, int64(2200), updatedBill.TotalAmount)
		})
	}
}

func TestAdjustBillItem_FixedCheckWideOfferCapsAtEligibleSubtotal(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	offer := helperOffer(t, biz.ID, "fixed", 8)

	bill := helperBill(t, biz, []BillItem{
		{ID: "00000000-0000-4000-8000-00000000a001", MenuItemID: "chips", Name: "Chips", Price: 3, Quantity: 3, Subtotal: 9, ItemType: "menu_item"},
		offerDiscountLine("00000000-0000-4000-8000-00000000d001", offer.ID, nil, 8),
	}, 1)

	updatedBill, updatedItems, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-00000000a001", "manager", intPtr(1), false, "")
	require.NoError(t, err)

	disc := findBillItem(updatedItems, "00000000-0000-4000-8000-00000000d001")
	require.NotNil(t, disc)
	require.InDelta(t, -3.0, disc.Subtotal, 0.001,
		"$8 off the check must recap to the $3 remaining eligible subtotal")
	require.Equal(t, int64(0), updatedBill.Subtotal)
	require.Equal(t, int64(0), updatedBill.TotalAmount)
}
