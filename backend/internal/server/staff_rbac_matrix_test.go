//go:build integration
// +build integration

package server_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// TestStaffRBACMatrix is the table-driven test that locks in the per-role
// behavior of every endpoint touched by the RBAC sweep. Each row asserts the
// HTTP status the role gets when hitting (method, path) with an empty body.
//
// Conventions:
//   - 200 / 400 / 404: role passed auth+RBAC; 400 means body validation
//     failed (success signal — auth let us through), 200 means GET succeeded,
//     404 means business not found (also past auth).
//   - 500: also past auth — handler reached but a downstream dependency
//     (e.g., a table not in this harness's AutoMigrate) returned an error.
//     Used sparingly when the request struct can't be made to fail body
//     validation with an empty body.
//   - 403: role correctly denied (RBAC working as intended).
//   - 401: a regression — token missing/invalid (must never happen here).
//
// Skips automatically when TEST_DATABASE_URL is unset.
func TestStaffRBACMatrix(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping RBAC matrix test")
	}

	// Match the in-package convention for setting up the JWT secret in tests
	// (see auth_handlers_test.go, staff_handlers_test.go). Direct assignment
	// is needed because GetSecretKey() caches into structs.SecretKey on first
	// call; t.Setenv on JWT_SECRET_KEY is a no-op when another test in the
	// binary has already triggered the cache.
	if len(structs.SecretKey) == 0 {
		structs.SecretKey = []byte("rbac-matrix-test-secret")
	}

	gin.SetMode(gin.TestMode)

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open postgres")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping(), "ping postgres")

	// Minimal AutoMigrate — tables involved in the matrix.
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.StaffInvitation{},
	), "automigrate matrix tables")
	useIntegrationSessionStore(t, gormDB)

	// Seed 1 business.
	bizID := time.Now().UnixNano()
	biz := &database.Business{
		BusinessId:     fmt.Sprintf("rbac-matrix-%d", bizID),
		OwnerAddress:   "0x0000000000000000000000000000000000000abc",
		Name:           "RBAC Matrix Biz",
		SettlementAddr: "0x0000000000000000000000000000000000000def",
		TippingAddr:    "0x0000000000000000000000000000000000000fed",
	}
	require.NoError(t, gormDB.Create(biz).Error, "create biz")

	// Seed 1 staff per role.
	roles := []database.StaffRole{
		database.StaffRoleManager,
		database.StaffRoleServer,
		database.StaffRoleHost,
		database.StaffRoleKitchen,
	}
	tokens := map[database.StaffRole]string{}
	for _, r := range roles {
		s := &database.Staff{
			BusinessID: biz.ID,
			Name:       fmt.Sprintf("Test %s", r),
			Email:      fmt.Sprintf("test-%s-%d@payverge.test", r, bizID),
			Role:       r,
			IsActive:   true,
		}
		require.NoError(t, gormDB.Create(s).Error, "create staff %s", r)
		tokens[r] = generateIntegrationStaffToken(t, s)
	}

	// Cleanup so the test is rerunnable.
	t.Cleanup(func() {
		if err := gormDB.Exec("DELETE FROM staff_invitations WHERE business_id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup staff_invitations: %v", err)
		}
		if err := gormDB.Exec("DELETE FROM staff WHERE business_id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup staff: %v", err)
		}
		if err := gormDB.Exec("DELETE FROM businesses WHERE id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup businesses: %v", err)
		}
	})

	router := server.BuildRouterForTest(gormDB)

	probe := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Single sanity row to prove the harness runs end-to-end.
	w := probe("GET", fmt.Sprintf("/api/v1/inside/businesses/%d", biz.ID), tokens[database.StaffRoleManager], "")
	require.NotEqual(t, http.StatusUnauthorized, w.Code, "harness sanity: manager hits GET /businesses/:id without 401 — got body=%s", w.Body.String())
	require.NotEqual(t, http.StatusForbidden, w.Code, "harness sanity: manager hits GET /businesses/:id without 403")

	// The rbacCase table is appended to in subsequent tasks (settings, menu,
	// bills, staff, plugins). The runner below stays the same.
	type rbacCase struct {
		name       string
		method     string
		pathFn     func(uint) string
		role       database.StaffRole
		wantStatus int
		bodyJSON   string
	}
	cases := []rbacCase{
		// PUT operating-hours: manager allowed (400 = body invalid past auth), others 403
		{name: "settings:operating-hours/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "settings:operating-hours/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "settings:operating-hours/host", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "settings:operating-hours/kitchen", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		{name: "settings:special-features/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/special-features", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "settings:special-features/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/special-features", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		{name: "settings:gallery-images/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/gallery-images", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "settings:gallery-images/host", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/gallery-images", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// design-settings accepts an empty struct body, so manager hits 200 (past auth — success signal).
		{name: "settings:design-settings/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/design-settings", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: "{}"},
		{name: "settings:design-settings/kitchen", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/design-settings", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// hospitality-settings accepts an empty struct body, so manager hits 200 (past auth — success signal).
		{name: "settings:hospitality-settings/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/hospitality-settings", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: "{}"},
		{name: "settings:hospitality-settings/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/hospitality-settings", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// Google info — manager-only via settings:google
		{name: "settings:google/put/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/google", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "settings:google/put/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/google", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "settings:google/put/kitchen", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/google", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		{name: "settings:google/delete/manager", method: "DELETE",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/google", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: ""},
		{name: "settings:google/delete/host", method: "DELETE",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/google", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: ""},

		// Toggle kitchen/orders — manager-only via settings:write
		{name: "settings:toggle-kitchen-orders/manager", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/toggle-kitchen-orders", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: "{}"},
		{name: "settings:toggle-kitchen-orders/server", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/toggle-kitchen-orders", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "settings:toggle-kitchen-orders/kitchen", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/toggle-kitchen-orders", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// Menu items POST/PUT: empty `{}` body now fails product validation with 400
		// (e.g. blank name rejected before DB) — past-auth signal, no longer 500
		// from missing menus table.
		{name: "menu:items/post/manager", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/items", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "menu:items/post/server", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/items", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "menu:items/post/host", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/items", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "menu:items/post/kitchen", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/items", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		{name: "menu:items/put/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/items", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "menu:items/put/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/items", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// Menu categories POST: AddCategoryRequest.Name has binding:"required" — empty
		// body fails validation cleanly with 400, the documented "auth passed" signal.
		{name: "menu:categories/post/manager", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/categories", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "menu:categories/post/host", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/menu/categories", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// Bills create — manager + server allowed via bills:create; host/kitchen denied.
		// Empty body fails CreateBillRequest validation past auth (400 = past-auth signal).
		{name: "bills:create/manager", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/bills", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "bills:create/server", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/bills", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "bills:create/host", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/bills", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "bills:create/kitchen", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/bills", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// PUT /businesses/:id — manager allowed via business:settings, others 403.
		// UpdateBusinessRequest has no required fields, so {"name":"Updated"} succeeds (200).
		{name: "business:update/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: `{"name":"Updated"}`},
		{name: "business:update/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: `{"name":"Updated"}`},

		// DELETE /businesses/:id — manager passes the route-level RBAC gate
		// (business:settings is in their perm list) but the handler uses
		// requireBusinessOwnership. Manager is staff, not an owner, so 403 from
		// the handler. This is the defense-in-depth we want for owner-only ops.
		{name: "business:delete/manager", method: "DELETE",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusForbidden, bodyJSON: ""},

		// Staff list — staff:read is in every role's perm set, so all four roles
		// pass the route gate. The handler returns the staff list (200 OK).
		{name: "staff:read/manager", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: ""},
		{name: "staff:read/server", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusOK, bodyJSON: ""},
		{name: "staff:read/host", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusOK, bodyJSON: ""},
		{name: "staff:read/kitchen", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusOK, bodyJSON: ""},

		// Staff invite — manager-only via staff:invite. Manager hits 400 because
		// InviteStaffRequest has required Email/Name/Role fields and the empty
		// body fails validation past auth (400 = past-auth signal).
		{name: "staff:invite/manager", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff/invite", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "staff:invite/server", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff/invite", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "staff:invite/host", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff/invite", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "staff:invite/kitchen", method: "POST",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/staff/invite", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},

		// Plugins list — manager-only via plugins:read. Other roles get 403.
		{name: "plugins:read/manager", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/plugins", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusOK, bodyJSON: ""},
		{name: "plugins:read/server", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/plugins", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: ""},
		{name: "plugins:read/host", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/plugins", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: ""},
		{name: "plugins:read/kitchen", method: "GET",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/plugins", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: ""},

		// counter:settings — manager-only via counter:settings perm
		{name: "counter:settings/manager", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/counters/settings", id) },
			role:   database.StaffRoleManager, wantStatus: http.StatusBadRequest, bodyJSON: "{}"},
		{name: "counter:settings/server", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/counters/settings", id) },
			role:   database.StaffRoleServer, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "counter:settings/host", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/counters/settings", id) },
			role:   database.StaffRoleHost, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
		{name: "counter:settings/kitchen", method: "PUT",
			pathFn: func(id uint) string { return fmt.Sprintf("/api/v1/inside/businesses/%d/counters/settings", id) },
			role:   database.StaffRoleKitchen, wantStatus: http.StatusForbidden, bodyJSON: "{}"},
	}
	// Tasks 3–7 append to `cases` here. Initially empty.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := probe(tc.method, tc.pathFn(biz.ID), tokens[tc.role], tc.bodyJSON)
			require.Equalf(t, tc.wantStatus, w.Code,
				"%s want=%d got=%d body=%s",
				tc.name, tc.wantStatus, w.Code, w.Body.String())
		})
	}
}
