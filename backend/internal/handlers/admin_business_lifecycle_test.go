package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lifecycleTestAdminUserID = uint(4242)

// adminLifecycleRouter wires suspend/reactivate behind a stub that injects the
// admin identity AuthenticationAdminMiddleware sets in production, plus one
// operator route gated by RequireOperationalBusiness.
func adminLifecycleRouter() *gin.Engine {
	handler := NewAdminBusinessHandler(database.GetDB())
	router := gin.New()
	admin := router.Group("/admin", func(c *gin.Context) {
		c.Set("user_id", lifecycleTestAdminUserID)
		c.Next()
	})
	admin.POST("/businesses/:id/suspend", handler.SuspendBusiness)
	admin.POST("/businesses/:id/reactivate", handler.ReactivateBusiness)
	router.GET("/businesses/:id/gated", server.RequireOperationalBusiness(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func setupAdminLifecycleBusiness(t *testing.T, owner string) *database.Business {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gormDB := setupAccountingHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.AdminAction{}))
	business := createAccountingHandlerBusiness(t, owner)
	require.True(t, business.IsActive, "fixture business must start active")
	return business
}

func reloadLifecycleBusiness(t *testing.T, id uint) *database.Business {
	t.Helper()
	var b database.Business
	require.NoError(t, database.GetDB().First(&b, id).Error)
	return &b
}

func lifecycleAdminActions(t *testing.T, actionType string) []database.AdminAction {
	t.Helper()
	var actions []database.AdminAction
	require.NoError(t, database.GetDB().Where("action_type = ?", actionType).Find(&actions).Error)
	return actions
}

func TestSuspendBusiness_DeactivatesAndAudits(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleSuspend")
	router := adminLifecycleRouter()

	w := performAccountingRequest(t, router, http.MethodPost,
		fmt.Sprintf("/admin/businesses/%d/suspend", business.ID),
		map[string]any{"reason": "chargeback investigation"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.False(t, reloadLifecycleBusiness(t, business.ID).IsActive)

	actions := lifecycleAdminActions(t, "suspend_business")
	require.Len(t, actions, 1)
	assert.Equal(t, lifecycleTestAdminUserID, actions[0].AdminUserID)
	var details map[string]any
	require.NoError(t, json.Unmarshal(actions[0].Details, &details))
	assert.Equal(t, "chargeback investigation", details["reason"])
	assert.EqualValues(t, business.ID, details["business_id"])
}

func TestSuspendBusiness_AlreadySuspendedReturnsConflict(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleTwice")
	router := adminLifecycleRouter()
	path := fmt.Sprintf("/admin/businesses/%d/suspend", business.ID)

	first := performAccountingRequest(t, router, http.MethodPost, path, map[string]any{"reason": "first"})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := performAccountingRequest(t, router, http.MethodPost, path, map[string]any{"reason": "second"})
	assert.Equal(t, http.StatusConflict, second.Code, second.Body.String())
	assert.Len(t, lifecycleAdminActions(t, "suspend_business"), 1, "a no-op suspend must not write an audit row")
}

func TestReactivateBusiness_RestoresActiveAndAudits(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleReactivate")
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", business.ID).Update("is_active", false).Error)
	router := adminLifecycleRouter()

	w := performAccountingRequest(t, router, http.MethodPost,
		fmt.Sprintf("/admin/businesses/%d/reactivate", business.ID),
		map[string]any{"reason": "resolved"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
	assert.NotContains(t, resp, "warning")
	assert.True(t, reloadLifecycleBusiness(t, business.ID).IsActive)
	assert.Len(t, lifecycleAdminActions(t, "reactivate_business"), 1)
}

func TestReactivateBusiness_AlreadyActiveReturnsConflict(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleActive")
	router := adminLifecycleRouter()

	w := performAccountingRequest(t, router, http.MethodPost,
		fmt.Sprintf("/admin/businesses/%d/reactivate", business.ID),
		map[string]any{"reason": "noop"})
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Empty(t, lifecycleAdminActions(t, "reactivate_business"))
}

func TestReactivateBusiness_ClosedBusinessStaysLocked(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleClosed")
	closedAt := time.Now().Add(-time.Hour)
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", business.ID).
		Updates(map[string]any{"is_active": false, "closed_at": closedAt}).Error)
	router := adminLifecycleRouter()

	w := performAccountingRequest(t, router, http.MethodPost,
		fmt.Sprintf("/admin/businesses/%d/reactivate", business.ID),
		map[string]any{"reason": "reopen attempt"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["warning"])
	assert.Equal(t, string(database.BusinessStatusClosed), fmt.Sprint(resp["status"]))

	gated := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/gated", business.ID), nil)
	assert.Equal(t, http.StatusForbidden, gated.Code)
	assert.Contains(t, gated.Body.String(), server.ErrCodeBusinessClosed)
}

func TestSuspendReactivate_EmptyReasonReturnsBadRequest(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleReason")
	router := adminLifecycleRouter()

	for _, action := range []string{"suspend", "reactivate"} {
		w := performAccountingRequest(t, router, http.MethodPost,
			fmt.Sprintf("/admin/businesses/%d/%s", business.ID, action),
			map[string]any{"reason": "   "})
		assert.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", action, w.Body.String())
	}
	assert.True(t, reloadLifecycleBusiness(t, business.ID).IsActive)
}

func TestSuspendReactivate_UnknownBusinessReturnsNotFound(t *testing.T) {
	setupAdminLifecycleBusiness(t, "0xLifecycleUnknown")
	router := adminLifecycleRouter()

	for _, action := range []string{"suspend", "reactivate"} {
		w := performAccountingRequest(t, router, http.MethodPost,
			fmt.Sprintf("/admin/businesses/%d/%s", 999999, action),
			map[string]any{"reason": "missing"})
		assert.Equal(t, http.StatusNotFound, w.Code, "%s: %s", action, w.Body.String())
	}
}

func TestSuspendBusiness_LocksOperationalRoutes(t *testing.T) {
	business := setupAdminLifecycleBusiness(t, "0xLifecycleGate")
	router := adminLifecycleRouter()
	gatedPath := fmt.Sprintf("/businesses/%d/gated", business.ID)

	before := performAccountingRequest(t, router, http.MethodGet, gatedPath, nil)
	require.Equal(t, http.StatusOK, before.Code, before.Body.String())

	w := performAccountingRequest(t, router, http.MethodPost,
		fmt.Sprintf("/admin/businesses/%d/suspend", business.ID),
		map[string]any{"reason": "lock"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	after := performAccountingRequest(t, router, http.MethodGet, gatedPath, nil)
	assert.Equal(t, http.StatusForbidden, after.Code)
	assert.Contains(t, after.Body.String(), server.ErrCodeBusinessSuspended)

	w = performAccountingRequest(t, router, http.MethodPost,
		fmt.Sprintf("/admin/businesses/%d/reactivate", business.ID),
		map[string]any{"reason": "unlock"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	restored := performAccountingRequest(t, router, http.MethodGet, gatedPath, nil)
	assert.Equal(t, http.StatusOK, restored.Code, restored.Body.String())
}
