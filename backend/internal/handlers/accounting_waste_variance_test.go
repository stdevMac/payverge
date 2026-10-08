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

// setupWasteVarianceHandlerDB reuses the menu-engineering harness and
// additionally migrates inventory_movements, which the waste-variance
// calculator's AggregateInventoryMovements path reads.
func setupWasteVarianceHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB := setupMenuEngineeringDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.InventoryMovement{}))
	return gormDB
}

// TestWasteVariance_OwnerGets200AndEnvelope verifies the authorised owner
// receives a 200 with the {success,data} envelope and that the waste-variance
// report shape is present. An empty inventory is valid: ingredients serialises
// as an empty array (never null) and sparse is true.
func TestWasteVariance_OwnerGets200AndEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWasteVarianceHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xWVOwner")

	router := ownerRouter("0xWVOwner")
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/waste-variance",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetWasteVariance)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/waste-variance", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	body := w.Body.String()

	// ingredients must serialize as an array literal, never null (the FE maps over it).
	assert.Contains(t, body, `"ingredients":[`, "ingredients must be an array, not null")
	assert.Contains(t, body, `"tracked_loss_cost":`, "tracked_loss_cost key must be present")
	assert.Contains(t, body, `"sparse":true`, "empty inventory must be sparse")

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Period            string      `json:"period"`
			TrackedLossCost   float64     `json:"tracked_loss_cost"`
			TotalVarianceCost float64     `json:"total_variance_cost"`
			LossByReason      interface{} `json:"loss_by_reason"`
			Ingredients       interface{} `json:"ingredients"`
			Sparse            bool        `json:"sparse"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Success)
	assert.True(t, resp.Data.Sparse, "empty inventory is below the sparse threshold")
}

// TestWasteVariance_CrossTenantIsForbidden locks the IDOR guard: the owner
// of business A requesting business B's id must not receive a 200 or B's data.
func TestWasteVariance_CrossTenantIsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWasteVarianceHandlerDB(t)
	_ = createAccountingHandlerBusiness(t, "0xWVOwnerA")
	businessB := createAccountingHandlerBusiness(t, "0xWVOwnerB")

	// Authenticated as owner of A, requesting B's id.
	router := ownerRouter("0xWVOwnerA")
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/waste-variance",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetWasteVariance)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/waste-variance", businessB.ID), nil)

	assert.NotEqual(t, http.StatusOK, w.Code, "cross-tenant request must not succeed")
	assert.Equal(t, http.StatusForbidden, w.Code, "loadBusiness denies non-owned business with 403")
	assert.NotContains(t, w.Body.String(), `"success":true`, "must not return B's data envelope")
}

// TestWasteVariance_BadPeriodIs400 verifies that an unsupported period maps
// to 400 via the foodcost.ErrUnsupportedPeriod branch.
func TestWasteVariance_BadPeriodIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWasteVarianceHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xWVOwnerBad")

	router := ownerRouter("0xWVOwnerBad")
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/waste-variance",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetWasteVariance)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/waste-variance?period=decade", business.ID), nil)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}
