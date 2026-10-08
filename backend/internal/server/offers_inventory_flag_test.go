package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// #835: an Active offer targeting an inventory-blocked dish ("$5 Off the
// Steak Plate" while Premium Beef is at zero) must be visibly flagged on the
// operator Offers list. Guest surfaces already suppress it; the operator list
// returned raw rows with no inventory annotation, regressing #653/#345.
func TestGetOffers_FlagsOfferTargetingInventoryBlockedDish(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{}, &database.Offer{},
	))
	database.SetTestDB(db)

	business := database.Business{BusinessId: "demo-admin-8-ai-pro", Name: "AI Pro", OwnerAddress: "0xowner"}
	require.NoError(t, db.Create(&business).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true},
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeWarn,
	}).Error)
	greens := database.InventoryItem{BusinessID: business.ID, Name: "Mixed Greens", Unit: "kg", CurrentQuantity: 33.25, IsActive: true}
	beef := database.InventoryItem{BusinessID: business.ID, Name: "Premium Beef", Unit: "kg", CurrentQuantity: 0, IsActive: true}
	require.NoError(t, db.Create(&greens).Error)
	require.NoError(t, db.Create(&beef).Error)
	require.NoError(t, db.Create([]database.InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl", InventoryItemID: greens.ID, QuantityRequired: 0.25},
		{BusinessID: business.ID, MenuItemID: "demo-steak", MenuItemName: "Steak Plate", InventoryItemID: beef.ID, QuantityRequired: 0.35},
	}).Error)

	steakTarget := "demo-steak"
	bowlTarget := "demo-bowl"
	require.NoError(t, db.Create([]database.Offer{
		{BusinessID: business.ID, Name: "$5 Off the Steak Plate", DiscountType: "fixed", DiscountValue: 5, IsActive: true, ApplicableTo: "item", TargetID: &steakTarget, WeekdayMask: 127},
		{BusinessID: business.ID, Name: "Bowl Deal", DiscountType: "percentage", DiscountValue: 10, IsActive: true, ApplicableTo: "item", TargetID: &bowlTarget, WeekdayMask: 127},
		{BusinessID: business.ID, Name: "Everything 5% Off", DiscountType: "percentage", DiscountValue: 5, IsActive: true, ApplicableTo: "all", WeekdayMask: 127},
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/businesses/%d/offers", business.ID), nil)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	GetOffers(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response []struct {
		Name             string `json:"name"`
		IsActive         bool   `json:"is_active"`
		InventoryBlocked bool   `json:"inventory_blocked"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response, 3)
	byName := map[string]bool{}
	for _, offer := range response {
		byName[offer.Name] = offer.InventoryBlocked
	}
	assert.True(t, byName["$5 Off the Steak Plate"],
		"offer targeting the 86'd steak must be flagged inventory_blocked")
	assert.False(t, byName["Bowl Deal"], "offer on a sellable dish must not be flagged")
	assert.False(t, byName["Everything 5% Off"], "broad offers are never inventory-flagged")
}

// A manual operator 86 on the target dish must flag its offer too — the guest
// filter hides those offers, so the operator list has to say why.
func TestGetOffers_FlagsOfferOnManuallyDisabledDish(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{}, &database.Offer{},
	))
	database.SetTestDB(db)

	business := database.Business{BusinessId: "manual-86-offers", Name: "Manual 86", OwnerAddress: "0xowner"}
	require.NoError(t, db.Create(&business).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-tacos", Name: "Market Tacos", Price: 14, IsAvailable: false},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)

	tacosTarget := "demo-tacos"
	require.NoError(t, db.Create(&database.Offer{
		BusinessID: business.ID, Name: "Taco Tuesday", DiscountType: "percentage", DiscountValue: 15,
		IsActive: true, ApplicableTo: "item", TargetID: &tacosTarget, WeekdayMask: 127,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/businesses/%d/offers", business.ID), nil)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	GetOffers(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response []struct {
		Name             string `json:"name"`
		InventoryBlocked bool   `json:"inventory_blocked"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.True(t, response[0].InventoryBlocked)
}
