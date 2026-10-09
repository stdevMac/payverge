package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestPriceOrderInputsPromoCodeDoesNotAutoApply: a coded offer must not
// discount a public/operator price call unless the caller presents the code.
// An uncoded offer still auto-applies through both entry points. The coded
// offer stays in the cached snapshot, so a later WithPromo call on the same
// business sees it without a cache refresh.
func TestPriceOrderInputsPromoCodeDoesNotAutoApply(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := createTestHospitalityBusiness(t, db, "promo-code-gate")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID: "burger", Name: "Burger", Price: 10, IsAvailable: true,
	})
	code := "SAVE10"
	target := "burger"
	require.NoError(t, db.Create(&database.Offer{
		BusinessID:    business.ID,
		Name:          "Save 10",
		DiscountType:  "percentage",
		DiscountValue: 10,
		IsActive:      true,
		ApplicableTo:  "item",
		TargetID:      &target,
		Code:          &code,
		WeekdayMask:   127,
	}).Error)
	InvalidatePricingCache(business.ID)

	input := []PromotionInputLine{{
		MenuItemID: "burger",
		Name:       "Burger",
		Quantity:   1,
		UnitPrice:  10,
	}}

	noCode, _, err := PriceOrderInputsByBusinessID(business.ID, input)
	require.NoError(t, err)
	require.Zero(t, noCode.DiscountTotal)
	require.Empty(t, noCode.AppliedOffers)

	wrong, _, err := PriceOrderInputsByBusinessIDWithPromo(business.ID, input, "WRONG")
	require.NoError(t, err)
	require.Zero(t, wrong.DiscountTotal)
	require.Empty(t, wrong.AppliedOffers)

	// Same cached snapshot: the coded offer was not stripped from it.
	withCode, _, err := PriceOrderInputsByBusinessIDWithPromo(business.ID, input, "save10")
	require.NoError(t, err)
	require.InDelta(t, 1.0, withCode.DiscountTotal, 0.001)
	require.Len(t, withCode.AppliedOffers, 1)
	require.Equal(t, "Save 10", withCode.AppliedOffers[0].Name)

	padded, _, err := PriceOrderInputsByBusinessIDWithPromo(business.ID, input, "  save10  ")
	require.NoError(t, err)
	require.InDelta(t, 1.0, padded.DiscountTotal, 0.001)

	require.NoError(t, db.Create(&database.Offer{
		BusinessID:    business.ID,
		Name:          "Auto",
		DiscountType:  "percentage",
		DiscountValue: 20,
		IsActive:      true,
		ApplicableTo:  "item",
		TargetID:      &target,
		WeekdayMask:   127,
	}).Error)
	InvalidatePricingCache(business.ID)

	auto, _, err := PriceOrderInputsByBusinessID(business.ID, input)
	require.NoError(t, err)
	require.InDelta(t, 2.0, auto.DiscountTotal, 0.001)
	require.Len(t, auto.AppliedOffers, 1)
	require.Equal(t, "Auto", auto.AppliedOffers[0].Name)

	autoWrong, _, err := PriceOrderInputsByBusinessIDWithPromo(business.ID, input, "WRONG")
	require.NoError(t, err)
	require.InDelta(t, 2.0, autoWrong.DiscountTotal, 0.001)
	require.Len(t, autoWrong.AppliedOffers, 1)
	require.Equal(t, "Auto", autoWrong.AppliedOffers[0].Name)

	both, _, err := PriceOrderInputsByBusinessIDWithPromo(business.ID, input, "SAVE10")
	require.NoError(t, err)
	require.InDelta(t, 3.0, both.DiscountTotal, 0.001)
	applied := map[string]float64{}
	for _, offer := range both.AppliedOffers {
		applied[offer.Name] = offer.Amount
	}
	require.InDelta(t, 1.0, applied["Save 10"], 0.001)
	require.InDelta(t, 2.0, applied["Auto"], 0.001)
}
