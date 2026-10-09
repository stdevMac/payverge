package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tablesStatusRouter mirrors the live route wiring
// (RoleBasedAccessMiddleware("tables:read") -> GetTablesWithStatus) with the
// context a staff token gets from HybridAuthenticationMiddleware: staff_id,
// staff_business_id, staff_role and token_type, but no address or user_id.
func tablesStatusRouter(setCtx func(c *gin.Context)) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if setCtx != nil {
			setCtx(c)
		}
		c.Next()
	})
	router.GET("/businesses/:id/tables/status", RoleBasedAccessMiddleware("tables:read"), GetTablesWithStatus)
	return router
}

// setupTablesStatusTestDB extends the AI waiter fixture with the reservations
// table the floor query reads for the next booking per table.
func setupTablesStatusTestDB(t *testing.T) {
	t.Helper()
	gormDB := setupAIWaiterTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.TableReservation{}))
}

func staffTokenCtx(role database.StaffRole, staffID, businessID uint) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(role))
		c.Set("staff_id", staffID)
		c.Set("staff_business_id", businessID)
	}
}

// Regression: waiter/host sessions got 401 "User not authenticated" on the
// floor occupancy endpoint because the handler only accepted address/user_id.
func TestGetTablesWithStatus_StaffOfOwnBusinessSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTablesStatusTestDB(t)

	biz := createAIWaiterBusiness(t, "floor-own", true)
	require.NoError(t, database.GetDB().Create(&database.Table{
		BusinessID: biz.ID, Name: "Mesa 1", TableCode: "floor-own-1", Capacity: 4, IsActive: true,
	}).Error)

	for _, role := range []database.StaffRole{database.StaffRoleServer, database.StaffRoleHost, database.StaffRoleManager} {
		t.Run(string(role), func(t *testing.T) {
			staff := createAIWaiterStaff(t, biz.ID, role, fmt.Sprintf("floor-%s@example.com", role))
			w := performAIWaiterRequest(t, tablesStatusRouter(staffTokenCtx(role, staff.ID, biz.ID)),
				http.MethodGet, fmt.Sprintf("/businesses/%d/tables/status", biz.ID), nil)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), `"tables"`)
			assert.Contains(t, w.Body.String(), "Mesa 1")
		})
	}
}

func TestGetTablesWithStatus_StaffOfOtherBusinessRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTablesStatusTestDB(t)

	target := createAIWaiterBusiness(t, "floor-target", true)
	other := createAIWaiterBusiness(t, "floor-other", true)
	staff := createAIWaiterStaff(t, other.ID, database.StaffRoleServer, "floor-other@example.com")

	// Staff token is scoped to "other" but requests "target".
	w := performAIWaiterRequest(t, tablesStatusRouter(staffTokenCtx(database.StaffRoleServer, staff.ID, other.ID)),
		http.MethodGet, fmt.Sprintf("/businesses/%d/tables/status", target.ID), nil)
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), `"tables"`)
}

func TestGetTablesWithStatus_KitchenWithoutTablesReadRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTablesStatusTestDB(t)

	biz := createAIWaiterBusiness(t, "floor-kitchen", true)
	staff := createAIWaiterStaff(t, biz.ID, database.StaffRoleKitchen, "floor-kitchen@example.com")

	w := performAIWaiterRequest(t, tablesStatusRouter(staffTokenCtx(database.StaffRoleKitchen, staff.ID, biz.ID)),
		http.MethodGet, fmt.Sprintf("/businesses/%d/tables/status", biz.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestGetTablesWithStatus_UnauthenticatedRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTablesStatusTestDB(t)

	biz := createAIWaiterBusiness(t, "floor-anon", true)

	// Handler-level guard: no owner or staff identity in context.
	router := gin.New()
	router.GET("/businesses/:id/tables/status", GetTablesWithStatus)
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/tables/status", biz.ID), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestGetTablesWithStatus_OwnerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTablesStatusTestDB(t)

	biz := createAIWaiterBusiness(t, "floor-owner", true)
	// Handler-level: the owner identity (address) still passes the guard and
	// the ownership check.
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("address", biz.OwnerAddress)
		c.Next()
	})
	router.GET("/businesses/:id/tables/status", GetTablesWithStatus)
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/tables/status", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
