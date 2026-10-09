package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const authorizationMatrixOrigin = "https://operator.authorization.test"

type authorizationMatrixUserAuth struct {
	ID             uint `gorm:"primaryKey"`
	UserID         uint `gorm:"index"`
	Provider       string
	ProviderUserID string
	EmailVerified  bool
}

func (authorizationMatrixUserAuth) TableName() string { return "user_auths" }

type authorizationMatrixFixture struct {
	db                *gorm.DB
	activeBusiness    *database.Business
	suspendedBusiness *database.Business
	ownerToken        string
	suspendedOwner    string
	wrongOwnerToken   string
	staffWithoutToken string
	staffWithToken    string
	inactiveStaff     string
	verifiedUserToken string
	adminToken        string
	unverifiedAdmin   string
}

func TestProtectedRouteAuthorizationActorMatrix(t *testing.T) {
	// The matrix models a deployment that delivers mail, where an unverified
	// email identity is not an operator (EMAIL_VERIFICATION=off is covered in
	// internal/server/verified_email_session_boundary_test.go).
	t.Setenv("EMAIL_VERIFICATION", "required")
	fixture := newAuthorizationMatrixFixture(t)

	type routeCase struct {
		name      string
		sourceKey string
		method    string
		path      func(*authorizationMatrixFixture) string
	}
	routes := []routeCase{
		{
			name:      "uploads",
			sourceKey: "POST /api/v1/inside/businesses/:id/uploads",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/uploads", f.activeBusiness.ID)
			},
		},
		{
			name:      "settings",
			sourceKey: "GET /api/v1/inside/businesses/:id/operating-hours",
			method:    http.MethodGet,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", f.activeBusiness.ID)
			},
		},
		{
			name:      "payments",
			sourceKey: "GET /api/v1/inside/businesses/:id/mercadopago/terminals",
			method:    http.MethodGet,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/mercadopago/terminals", f.activeBusiness.ID)
			},
		},
		{
			name:      "staff",
			sourceKey: "POST /api/v1/inside/businesses/:id/staff/invite",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/staff/invite", f.activeBusiness.ID)
			},
		},
		{
			name:      "plugins",
			sourceKey: "POST /api/v1/inside/businesses/:id/plugins/:plugin_id/enable",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/plugins/1/enable", f.activeBusiness.ID)
			},
		},
		{
			name:      "fiscal",
			sourceKey: "POST /api/v1/inside/businesses/:id/fiscal/receipts/issue",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/fiscal/receipts/issue", f.activeBusiness.ID)
			},
		},
		{
			name:      "printing",
			sourceKey: "POST /api/v1/inside/businesses/:id/printers",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/printers", f.activeBusiness.ID)
			},
		},
		{
			name:      "crm",
			sourceKey: "POST /api/v1/inside/businesses/:id/crm/customers",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/crm/customers", f.activeBusiness.ID)
			},
		},
		{
			name:      "inventory",
			sourceKey: "POST /api/v1/inside/businesses/:id/inventory/adjustments",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/inventory/adjustments", f.activeBusiness.ID)
			},
		},
		{
			name:      "analytics",
			sourceKey: "GET /api/v1/inside/businesses/:id/analytics/sales",
			method:    http.MethodGet,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/analytics/sales", f.activeBusiness.ID)
			},
		},
		{
			name:      "ai",
			sourceKey: "POST /api/v1/inside/businesses/:id/ai/director/ask",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/ai/director/ask", f.activeBusiness.ID)
			},
		},
		{
			name:      "operational-menu-mutation",
			sourceKey: "POST /api/v1/inside/businesses/:id/menu",
			method:    http.MethodPost,
			path: func(f *authorizationMatrixFixture) string {
				return fmt.Sprintf("/api/v1/inside/businesses/%d/menu", f.activeBusiness.ID)
			},
		},
	}
	sourceKeys := make([]string, 0, len(routes))
	for _, route := range routes {
		sourceKeys = append(sourceKeys, route.sourceKey)
	}
	tenantRouter := newAuthorizationMatrixRouterFromSource(t, sourceKeys)

	actors := []struct {
		name       string
		token      func(*testing.T, *authorizationMatrixFixture, string) string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "unverified", token: func(t *testing.T, f *authorizationMatrixFixture, route string) string {
			return f.createUserToken(t, "unverified-"+route+"@authorization.test", string(structs.RoleUser), false)
		}, wantStatus: http.StatusForbidden},
		{name: "wrong-tenant-owner", token: func(_ *testing.T, f *authorizationMatrixFixture, _ string) string { return f.wrongOwnerToken }, wantStatus: http.StatusForbidden},
		{name: "correct-owner", token: func(_ *testing.T, f *authorizationMatrixFixture, _ string) string { return f.ownerToken }, wantStatus: http.StatusNoContent},
		{name: "staff-without-permission", token: func(_ *testing.T, f *authorizationMatrixFixture, _ string) string { return f.staffWithoutToken }, wantStatus: http.StatusForbidden},
		{name: "staff-with-permission", token: func(_ *testing.T, f *authorizationMatrixFixture, _ string) string { return f.staffWithToken }, wantStatus: http.StatusNoContent},
		{name: "inactive-staff", token: func(_ *testing.T, f *authorizationMatrixFixture, _ string) string { return f.inactiveStaff }, wantStatus: http.StatusUnauthorized},
	}

	for _, route := range routes {
		route := route
		for _, actor := range actors {
			actor := actor
			t.Run(route.name+"/"+actor.name, func(t *testing.T) {
				token := ""
				if actor.token != nil {
					token = actor.token(t, fixture, route.name)
				}
				response := performAuthorizationMatrixRequest(tenantRouter, route.method, route.path(fixture), token)
				require.Equal(t, actor.wantStatus, response.Code, response.Body.String())
			})
		}
	}

	// Suspended businesses retain the explicitly pre-activation upload and
	// settings reads, while an operational mutation must stop at the
	// administrator suspension gate.
	for _, test := range []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "uploads-remain-available",
			method:     http.MethodPost,
			path:       fmt.Sprintf("/api/v1/inside/businesses/%d/uploads", fixture.suspendedBusiness.ID),
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "settings-read-remains-available",
			method:     http.MethodGet,
			path:       fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", fixture.suspendedBusiness.ID),
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "operational-mutation-is-denied",
			method:     http.MethodPost,
			path:       fmt.Sprintf("/api/v1/inside/businesses/%d/menu", fixture.suspendedBusiness.ID),
			wantStatus: http.StatusForbidden,
			wantCode:   "business_suspended",
		},
	} {
		test := test
		t.Run("suspended-business/"+test.name, func(t *testing.T) {
			response := performAuthorizationMatrixRequest(tenantRouter, test.method, test.path, fixture.suspendedOwner)
			require.Equal(t, test.wantStatus, response.Code, response.Body.String())
			if test.wantCode != "" {
				requireErrorCode(t, response, test.wantCode)
			}
		})
	}
}

func TestAdministratorRouteAuthorizationActorMatrix(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "required")
	fixture := newAuthorizationMatrixFixture(t)
	const sourceKey = "GET /api/v1/admin/users"
	router := newAuthorizationMatrixRouterFromSource(t, []string{sourceKey})

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "unverified-administrator", token: fixture.unverifiedAdmin, wantStatus: http.StatusForbidden},
		{name: "verified-non-admin", token: fixture.verifiedUserToken, wantStatus: http.StatusForbidden},
		{name: "verified-administrator", token: fixture.adminToken, wantStatus: http.StatusNoContent},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := performAuthorizationMatrixRequest(router, http.MethodGet, "/api/v1/admin/users", test.token)
			require.Equal(t, test.wantStatus, response.Code, response.Body.String())
		})
	}
}

func newAuthorizationMatrixRouterFromSource(t *testing.T, sourceKeys []string) *gin.Engine {
	t.Helper()
	routes := parseProtectedRouteInventory(t)
	router := gin.New()
	for _, key := range sourceKeys {
		route, ok := routes[key]
		require.Truef(t, ok, "real protected route %s must be wired", key)
		handlers := authorizationMiddlewareFromSource(t, key, route)
		handlers = append(handlers, authorizationMatrixOK)
		router.Handle(route.method, route.path, handlers...)
	}
	return router
}

func authorizationMiddlewareFromSource(t *testing.T, key string, route sourceProtectedRoute) []gin.HandlerFunc {
	t.Helper()
	handlers := make([]gin.HandlerFunc, 0, len(route.middleware))
	for _, expression := range route.middleware {
		if identifier, ok := expression.(*ast.Ident); ok {
			require.Equal(t, "adminRateLimiter", identifier.Name, "%s has an unsupported middleware identifier", key)
			continue
		}
		call, ok := expression.(*ast.CallExpr)
		if !ok {
			t.Fatalf("%s has non-call middleware expression %T", key, expression)
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			t.Fatalf("%s has unsupported middleware call %T", key, call.Fun)
		}
		switch selector.Sel.Name {
		case "RequireTrustedOriginForMutations":
			handlers = append(handlers, middleware.RequireTrustedOriginForMutations([]string{authorizationMatrixOrigin}))
		case "HybridAuthenticationMiddleware":
			handlers = append(handlers, server.HybridAuthenticationMiddleware())
		case "AuthenticationAdminMiddleware":
			handlers = append(handlers, server.AuthenticationAdminMiddleware())
		case "RequireOperationalBusiness":
			handlers = append(handlers, server.RequireOperationalBusiness())
		case "RoleBasedAccessMiddleware":
			require.NotEmpty(t, call.Args, "%s has an RBAC middleware without a permission", key)
			permission, ok := routeStringLiteral(call.Args[0])
			require.Truef(t, ok, "%s has a non-literal RBAC permission", key)
			handlers = append(handlers, server.RoleBasedAccessMiddleware(permission))
		case "RateLimit":
			// Rate limiting is inherited by admin routes but is orthogonal to the
			// actor authorization matrix. Authentication is still sourced from the
			// real route declaration above.
		default:
			t.Fatalf("%s has unsupported authorization middleware %s", key, selector.Sel.Name)
		}
	}
	return handlers
}

func authorizationMatrixOK(c *gin.Context) { c.Status(http.StatusNoContent) }

func performAuthorizationMatrixRequest(router http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", authorizationMatrixOrigin)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func requireErrorCode(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body), response.Body.String())
	require.Equal(t, want, body.Code)
}

func newAuthorizationMatrixFixture(t *testing.T) *authorizationMatrixFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(db)
	structs.SecretKey = []byte("route-authorization-actor-matrix-secret")
	require.NoError(t, db.AutoMigrate(
		&database.User{},
		&database.Business{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&session.UserSession{},
		&authorizationMatrixUserAuth{},
	))
	session.GlobalStore = session.NewStore(db)
	server.InitializeRBAC(database.GetDBWrapper())
	t.Cleanup(func() {
		session.GlobalStore = nil
		require.NoError(t, sqlDB.Close())
	})

	activeBusiness := &database.Business{
		BusinessId:     "authorization-active",
		Name:           "Authorization Active",
		OwnerAddress:   "0xauthorizationowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(activeBusiness).Error)

	suspendedBusiness := &database.Business{
		BusinessId:     "authorization-suspended",
		Name:           "Authorization Suspended",
		OwnerAddress:   "0xsuspendedowner",
		SettlementAddr: "0x3333333333333333333333333333333333333333",
		TippingAddr:    "0x4444444444444444444444444444444444444444",
		IsActive:       true,
	}
	require.NoError(t, db.Create(suspendedBusiness).Error)
	// gorm's default:true on is_active overrides a false value on Create, so
	// the administrator suspension is applied as an explicit column update.
	require.NoError(t, db.Model(suspendedBusiness).UpdateColumn("is_active", false).Error)
	suspendedBusiness.IsActive = false

	fixture := &authorizationMatrixFixture{
		db:                db,
		activeBusiness:    activeBusiness,
		suspendedBusiness: suspendedBusiness,
	}
	fixture.ownerToken = fixture.createWeb3Token(t, activeBusiness.OwnerAddress)
	fixture.suspendedOwner = fixture.createWeb3Token(t, suspendedBusiness.OwnerAddress)
	fixture.wrongOwnerToken = fixture.createWeb3Token(t, "0xwrongtenantowner")
	fixture.verifiedUserToken = fixture.createUserToken(t, "verified@authorization.test", string(structs.RoleUser), true)
	fixture.adminToken = fixture.createUserToken(t, "admin@authorization.test", string(structs.RoleAdmin), true)
	fixture.unverifiedAdmin = fixture.createUserToken(t, "unverified-admin@authorization.test", string(structs.RoleAdmin), false)
	fixture.staffWithoutToken = fixture.createStaffToken(t, activeBusiness.ID, "without@authorization.test", nil, true)
	launchPermissions := []string{
		"files:upload",
		"financial:read",
		"plugins:payments",
		"staff:invite",
		"settings:read",
		"plugins:write",
		"fiscal:issue",
		"printers:write",
		"crm:write",
		"inventory:adjust",
		"analytics:sales",
		"director:write",
		"menu:write",
	}
	fixture.staffWithToken = fixture.createStaffToken(t, activeBusiness.ID, "with@authorization.test", launchPermissions, true)
	fixture.inactiveStaff = fixture.createStaffToken(t, activeBusiness.ID, "inactive@authorization.test", launchPermissions, false)
	return fixture
}

func (f *authorizationMatrixFixture) createWeb3Token(t *testing.T, address string) string {
	t.Helper()
	sess, err := session.GlobalStore.Create(session.CreateInput{
		Address: address, Provider: "web3", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	token, err := server.GenerateWeb3Token(address, structs.RoleUser, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return token
}

func (f *authorizationMatrixFixture) createUserToken(t *testing.T, email, role string, verified bool) string {
	t.Helper()
	user := &database.User{Email: email, Name: email, Role: role, AuthMethod: "email", EmailVerified: verified}
	require.NoError(t, f.db.Create(user).Error)
	require.NoError(t, f.db.Create(&authorizationMatrixUserAuth{
		UserID: user.ID, Provider: "email", ProviderUserID: user.Email, EmailVerified: verified,
	}).Error)
	uid := user.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID: &uid, Provider: "email", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	token, err := server.GenerateUserToken(user.ID, user.Email, user.Address, user.Role, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return token
}

func (f *authorizationMatrixFixture) createStaffToken(t *testing.T, businessID uint, email string, permissions []string, active bool) string {
	t.Helper()
	encodedPermissions, err := json.Marshal(permissions)
	require.NoError(t, err)
	staff := &database.Staff{
		BusinessID: businessID, Email: email, Name: email, Role: database.StaffRoleKitchen,
		IsActive: true, InvitedBy: "owner@authorization.test", CustomPermissions: string(encodedPermissions), AuthzVersion: 1,
	}
	require.NoError(t, f.db.Create(staff).Error)
	uid := staff.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID: &uid, Provider: "staff", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	token, err := server.GenerateStaffToken(staff, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	if !active {
		require.NoError(t, f.db.Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)
	}
	return token
}
