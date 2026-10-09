package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L1-22: out-of-stock SELECT must carry quantity so callers can branch
// zero vs oversold copy (negative stock is not "at zero").
func TestGetInventoryOutOfStock_CarriesQuantityAndOrdersNegativesFirst(t *testing.T) {
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.InventoryItem{}))

	business := createBusinessHandlerTestBusiness(t, "0xL122Owner", "l1-22-insight")

	require.NoError(t, database.GetDB().Create(&database.InventoryItem{
		BusinessID:      business.ID,
		Name:            "Zero Flour",
		Unit:            "kg",
		CurrentQuantity: 0,
		IsActive:        true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.InventoryItem{
		BusinessID:      business.ID,
		Name:            "Oversold Oil",
		Unit:            "kg",
		CurrentQuantity: -0.35,
		IsActive:        true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.InventoryItem{
		BusinessID:      business.ID,
		Name:            "In Stock Salt",
		Unit:            "kg",
		CurrentQuantity: 5,
		IsActive:        true,
	}).Error)

	items := getInventoryOutOfStock(business.ID)
	require.Len(t, items, 2)
	// Negatives first (ORDER BY current_quantity ASC).
	assert.Equal(t, "Oversold Oil", items[0].Name)
	assert.InDelta(t, -0.35, items[0].Quantity, 0.001)
	assert.Equal(t, "Zero Flour", items[1].Name)
	assert.InDelta(t, 0.0, items[1].Quantity, 0.001)

	insights := buildProactiveInsights(business.ID)
	var stock *proactiveInsight
	for i := range insights {
		if insights[i].Type == "inventory_out_of_stock" {
			stock = &insights[i]
			break
		}
	}
	require.NotNil(t, stock)
	assert.Equal(t, true, stock.Params["has_oversold"])
	assert.Equal(t, 1, stock.Params["oversold_count"])
	assert.Equal(t, 2, stock.Params["count"])

	// R2-7: has_oversold is a boolean OR across the set, so a mixed set read as
	// "2 inventory items are oversold: Oversold Oil, Zero Flour". Carry each
	// group's own names so the copy can attribute them correctly.
	assert.Equal(t, []string{"Oversold Oil"}, stock.Params["oversold_names"])
	assert.Equal(t, []string{"Zero Flour"}, stock.Params["zero_names"])
}
