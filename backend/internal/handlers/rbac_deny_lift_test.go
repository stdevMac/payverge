package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestActorMayTouchPermission_ManagerCannotTouchOwnerPerm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	perm := permissionAbsentFromManager(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("staff_role", "manager")
	require.False(t, actorMayTouchPermission(c, perm, "lift"))
	require.Equal(t, http.StatusForbidden, w.Code)

	ownerW := httptest.NewRecorder()
	ownerC, _ := gin.CreateTestContext(ownerW)
	ownerC.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	require.True(t, actorMayTouchPermission(ownerC, perm, "lift"))
	require.Equal(t, http.StatusOK, ownerW.Code)
}

func TestRemovePermissionDeny_StaffCannotLiftOutsideRoleOrWildcard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACLimitDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(
		&database.StaffPermissionDeny{},
		&database.StaffMembership{},
	))

	outside := permissionAbsentFromManager(t)
	require.NotContains(t, permissionStrings(server.RolePermissions(database.StaffRoleManager)), outside)
	require.NotContains(t, permissionStrings(server.RolePermissions(database.StaffRoleManager)), "*:*")

	owner := "0xownerdenylift"
	business := &database.Business{
		BusinessId:      "biz-deny-lift",
		Name:            "Deny Lift",
		OwnerAddress:    owner,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		IsActive:        true,
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	manager := &database.Staff{
		BusinessID: business.ID,
		Email:      "manager-deny-lift@example.com",
		Name:       "Manager",
		Role:       database.StaffRoleManager,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, db.GetGorm().Create(manager).Error)
	target := &database.Staff{
		BusinessID: business.ID,
		Email:      "server-deny-lift@example.com",
		Name:       "Server",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, db.GetGorm().Create(target).Error)

	for _, perm := range []string{outside, "*:*"} {
		require.NoError(t, db.GetGorm().Create(&database.StaffPermissionDeny{
			BusinessID: business.ID,
			StaffID:    target.ID,
			Permission: perm,
			CreatedBy:  owner,
			Reason:     "seed",
		}).Error)
	}

	handlers := NewRBACHandlers(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Actor") == "owner" {
			c.Set("token_type", "web3")
			c.Set("address", owner)
			c.Next()
			return
		}
		c.Set("token_type", "staff")
		c.Set("staff_id", manager.ID)
		c.Set("staff_role", "manager")
		c.Set("staff_business_id", business.ID)
		c.Set("staff_email", manager.Email)
		c.Next()
	})
	router.DELETE("/inside/businesses/:id/staff/:staffId/permission-denies", handlers.RemovePermissionDeny)

	lift := func(actor, perm string) *httptest.ResponseRecorder {
		url := fmt.Sprintf("/inside/businesses/%d/staff/%d/permission-denies", business.ID, target.ID)
		body := fmt.Sprintf(`{"permission":%q,"reason":"test"}`, perm)
		req := httptest.NewRequest(http.MethodDelete, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Actor", actor)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusForbidden, lift("manager", outside).Code, "manager must not lift %s", outside)
	require.Equal(t, http.StatusForbidden, lift("manager", "*:*").Code, "manager must not lift *:*")
	require.Equal(t, int64(2), countDenies(t, db, target.ID))

	outsideResp := lift("owner", outside)
	require.Equal(t, http.StatusOK, outsideResp.Code, "owner lift %s body=%s", outside, outsideResp.Body.String())
	wildResp := lift("owner", "*:*")
	require.Equal(t, http.StatusOK, wildResp.Code, "owner lift *:* body=%s", wildResp.Body.String())
	require.Equal(t, int64(0), countDenies(t, db, target.ID))
}

func permissionAbsentFromManager(t *testing.T) string {
	t.Helper()
	held := map[string]struct{}{}
	for _, p := range server.RolePermissions(database.StaffRoleManager) {
		held[string(p)] = struct{}{}
	}
	for _, candidate := range []string{"payroll:write", "fiscal:credit", "fiscal:credentials"} {
		if _, ok := held[candidate]; !ok {
			return candidate
		}
	}
	t.Fatal("manager role unexpectedly holds every owner-only candidate")
	return ""
}

func permissionStrings(perms []server.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, string(p))
	}
	return out
}

func countDenies(t *testing.T, db *database.DB, staffID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.GetGorm().Model(&database.StaffPermissionDeny{}).Where("staff_id = ?", staffID).Count(&n).Error)
	return n
}
