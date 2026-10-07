package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupMenuEngineeringDB reuses the accounting handler harness and additionally
// migrates the recipe-cost tables and creates the bill_items table that the
// food-cost calculator's GetRecipeCostsForBusiness + analytics GetPopularItems
// path reads. Without bill_items the analytics raw query would fail with
// "no such table" and the handler would surface 500 instead of an empty report.
func setupMenuEngineeringDB(t *testing.T) *gorm.DB {
	t.Helper()

	gormDB := setupAccountingHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.InventoryItem{},
		&database.InventoryRecipe{},
	))
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)
	return gormDB
}

func ownerRouter(ownerAddress string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", ownerAddress)
		c.Set("business_owner_address", ownerAddress)
		c.Next()
	})
	return router
}

// TestMenuEngineering_OwnerGets200AndEnvelope verifies the authorized owner gets
// a 200 with the {success,data} envelope and that the menu-engineering report
// shape is present. An empty menu is a valid input: Classify returns an empty
// (but non-null) dishes array and sparse=true.
func TestMenuEngineering_OwnerGets200AndEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuEngineeringDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := ownerRouter("0xOwner")
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/menu-engineering",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetMenuEngineering)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/menu-engineering", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// dishes must serialize as an array literal, never null.
	assert.Contains(t, w.Body.String(), "\"dishes\":[")

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Dishes            []map[string]any `json:"dishes"`
			Rollups           []map[string]any `json:"rollups"`
			MedianFoodCostPct float64          `json:"median_food_cost_pct"`
			ItemsNeedingCost  int              `json:"items_needing_cost"`
			Sparse            bool             `json:"sparse"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data.Dishes, "dishes must be a present array, not null")
	assert.Len(t, resp.Data.Dishes, 0, "empty menu classifies to zero dishes")
	assert.True(t, resp.Data.Sparse, "empty menu is below the sparse threshold")
}

// TestMenuEngineering_CrossTenantIsForbidden locks the IDOR guard: the owner of
// business A requesting business B's id must not receive a 200 or B's data.
// loadBusiness -> CheckBusinessAccess denies (web3 owner address mismatch) and
// responds 403.
func TestMenuEngineering_CrossTenantIsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuEngineeringDB(t)
	_ = createAccountingHandlerBusiness(t, "0xOwner")
	businessB := createAccountingHandlerBusiness(t, "0xOwnerB")

	// Authenticate as the owner of A, request B's id.
	router := ownerRouter("0xOwner")
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/menu-engineering",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetMenuEngineering)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/menu-engineering", businessB.ID), nil)

	assert.NotEqual(t, http.StatusOK, w.Code, "cross-tenant request must not succeed")
	assert.Equal(t, http.StatusForbidden, w.Code, "loadBusiness denies non-owned business with 403")
	assert.NotContains(t, w.Body.String(), "\"success\":true", "must not return B's data envelope")
}

// TestMenuEngineering_BadPeriodIs400 verifies an unsupported period maps to 400
// via the foodcost.ErrUnsupportedPeriod branch (mirrors GetFoodCost).
func TestMenuEngineering_BadPeriodIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuEngineeringDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := ownerRouter("0xOwner")
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/menu-engineering",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetMenuEngineering)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/menu-engineering?period=decade", business.ID), nil)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}
