package director_tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromosTool_ListsLiveOffersAndBundles(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(&database.Offer{}, &database.Bundle{}))
	bizID := createTestBusiness(t, db, "Promo Lounge")
	target := "demo-steak"
	require.NoError(t, db.GetGorm().Create(&database.Offer{
		BusinessID:    bizID,
		Name:          "$5 Off the Steak Plate",
		DiscountType:  "fixed",
		DiscountValue: 5,
		IsActive:      true,
		ApplicableTo:  "item",
		TargetID:      &target,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Bundle{
		BusinessID: bizID,
		Name:       "Date Night for Two",
		Price:      42,
		Items:      `[{"menu_item_id":"demo-steak","name":"Steak Plate","quantity":1}]`,
		IsActive:   true,
	}).Error)

	tool := &PromosTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "en", DB: db,
	})
	require.NoError(t, err)

	offers, ok := result.Data["offers"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, offers, 1)
	assert.Equal(t, "$5 Off the Steak Plate", offers[0]["name"])
	assert.Equal(t, true, offers[0]["active"])

	bundles, ok := result.Data["bundles"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, bundles, 1)
	assert.Equal(t, "Date Night for Two", bundles[0]["name"])
	assert.Equal(t, true, bundles[0]["active"])

	blob := result.Summary + " "
	assert.Contains(t, blob, "Date Night")
	assert.Contains(t, blob, "Steak")
	assert.NotContains(t, blob, "qty_sold")
	assert.NotContains(t, strings.ToLower(blob), "get_promos")
}

func TestPromosTool_DoesNotClaimMissingLivePromos(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(&database.Offer{}, &database.Bundle{}))
	bizID := createTestBusiness(t, db, "Empty Promo Lounge")

	tool := &PromosTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "en", DB: db,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Data["offers_count"])
	assert.Equal(t, 0, result.Data["bundles_count"])
	assert.Contains(t, strings.ToLower(result.Summary), "no live")
}
