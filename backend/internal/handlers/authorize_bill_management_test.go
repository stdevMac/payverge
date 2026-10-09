package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func TestAuthorizeBillManagement_AdminOwnerAndUnrelated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(41)
	business := createPaymentRegressionBusiness(t, "bill-mgmt-auth", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)

	t.Run("platform admin is authorized", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("token_type", "user")
		c.Set("role", string(structs.RoleAdmin))
		c.Set("user_role", string(structs.RoleAdmin))
		c.Set("user_id", uint(9001))
		c.Set("email", "admin@local.test")

		actor, ok := handler.authorizeBillManagement(c, bill)
		require.True(t, ok, "platform admin must recover bills they can already open")
		require.NotEmpty(t, actor)
	})

	t.Run("unrelated user is denied", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("token_type", "user")
		c.Set("user_id", uint(888))
		c.Set("email", "other@example.com")

		actor, ok := handler.authorizeBillManagement(c, bill)
		require.False(t, ok, "unrelated user must not authorize bill management")
		require.Empty(t, actor)
	})

	t.Run("owner is allowed", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")

		actor, ok := handler.authorizeBillManagement(c, bill)
		require.True(t, ok, "business owner must still authorize bill management")
		require.Equal(t, "owner@example.com", actor)
	})

	t.Run("staff is still denied", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", uint(7))
		c.Set("staff_name", "Floor Manager")

		actor, ok := handler.authorizeBillManagement(c, bill)
		require.False(t, ok, "staff must not gain void/refund via the admin recovery branch")
		require.Empty(t, actor)
	})
}

func TestGetPendingAlternativePayments_PlatformAdminCanList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(42)
	business := createPaymentRegressionBusiness(t, "bill-mgmt-pending", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	path := fmt.Sprintf("/inside/bills/%d/pending-alternative-payments", bill.ID)

	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("role", string(structs.RoleAdmin))
		c.Set("user_role", string(structs.RoleAdmin))
		c.Set("user_id", uint(9001))
		c.Set("email", "admin@local.test")
		c.Next()
	})
	adminRouter.GET("/inside/bills/:bill_id/pending-alternative-payments", handler.GetPendingAlternativePayments)

	adminResp := performPaymentRegressionRequest(t, adminRouter, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, adminResp.Code, adminResp.Body.String())
	require.Contains(t, adminResp.Body.String(), `"pending_payments"`)

	strangerRouter := gin.New()
	strangerRouter.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", uint(888))
		c.Set("email", "other@example.com")
		c.Next()
	})
	strangerRouter.GET("/inside/bills/:bill_id/pending-alternative-payments", handler.GetPendingAlternativePayments)

	strangerResp := performPaymentRegressionRequest(t, strangerRouter, http.MethodGet, path, nil)
	require.Equal(t, http.StatusForbidden, strangerResp.Code, strangerResp.Body.String())
	require.Contains(t, strangerResp.Body.String(), "not bill owner")
}

func TestGetBillAuditLog_PlatformAdminCanRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.CompVoidAudit{}))

	ownerID := uint(43)
	business := createPaymentRegressionBusiness(t, "bill-mgmt-audit", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	path := fmt.Sprintf("/inside/bills/%d/audit", bill.ID)

	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("role", string(structs.RoleAdmin))
		c.Set("user_role", string(structs.RoleAdmin))
		c.Set("user_id", uint(9001))
		c.Set("email", "admin@local.test")
		c.Next()
	})
	adminRouter.GET("/inside/bills/:bill_id/audit", handler.GetBillAuditLog)

	adminResp := performPaymentRegressionRequest(t, adminRouter, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, adminResp.Code, adminResp.Body.String())
	require.Contains(t, adminResp.Body.String(), `"entries"`)

	strangerRouter := gin.New()
	strangerRouter.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", uint(888))
		c.Set("email", "other@example.com")
		c.Next()
	})
	strangerRouter.GET("/inside/bills/:bill_id/audit", handler.GetBillAuditLog)

	strangerResp := performPaymentRegressionRequest(t, strangerRouter, http.MethodGet, path, nil)
	require.Equal(t, http.StatusForbidden, strangerResp.Code, strangerResp.Body.String())
}
