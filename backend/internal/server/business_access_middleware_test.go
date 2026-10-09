package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBusinessAccessMiddlewareTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func setupBillAccessMiddlewareDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	assert.NoError(t, err)
	sqlDB, err := gormDB.DB()
	assert.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")
	assert.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.BillHistoryEvent{},
		&database.Payment{},
		&database.Staff{},
		&session.UserSession{},
	))
	assert.NoError(t, gormDB.Exec(`
		CREATE TABLE bill_items (
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
	session.GlobalStore = session.NewStore(gormDB)
	InitializeRBAC(database.GetDBWrapper())
	return gormDB
}

func createBillAccessFixture(t *testing.T, ownerAddress string, suspended bool) (*database.Business, *database.Bill) {
	t.Helper()
	business := &database.Business{
		BusinessId:     fmt.Sprintf("bill-access-%s", ownerAddress),
		Name:           fmt.Sprintf("Bill Access %s", ownerAddress),
		OwnerAddress:   ownerAddress,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	assert.NoError(t, database.GetDB().Create(business).Error)
	if suspended {
		assert.NoError(t, database.GetDB().Model(business).UpdateColumn("is_active", false).Error)
	}

	table := &database.Table{BusinessID: business.ID, TableCode: fmt.Sprintf("T-%d", business.ID), Name: "A1"}
	assert.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("BILL-%d", business.ID),
		TotalAmount:    1200,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	assert.NoError(t, database.GetDB().Create(bill).Error)
	return business, bill
}

func TestRequireBillBusinessAccess_AllowsPlatformAdminListedDemo(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	demoOwnerID := uint(99)
	business := &database.Business{
		BusinessId:      "demo-admin-1-core",
		Name:            "Payverge Core Demo Kitchen",
		OwnerAddress:    "0xDemoSeedOwner",
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		DemoOwnerUserID: &demoOwnerID,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		IsActive:        true,
	}
	assert.NoError(t, database.GetDB().Create(business).Error)
	table := &database.Table{BusinessID: business.ID, TableCode: fmt.Sprintf("T-%d", business.ID), Name: "A1"}
	assert.NoError(t, database.GetDB().Create(table).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("BILL-DEMO-%d", business.ID),
		TotalAmount:    1200,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
	}
	assert.NoError(t, database.GetDB().Create(bill).Error)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "email")
			c.Set("role", "admin")
			c.Set("user_role", "admin")
			c.Set("user_id", uint(8))
			c.Set("address", "0xPlatformAdminWallet")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestGetBill_AllowsPlatformAdminListedDemoAfterMiddleware(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	demoOwnerID := uint(99)
	business := &database.Business{
		BusinessId:      "demo-admin-1-core-getbill",
		Name:            "Payverge Core Demo Kitchen",
		OwnerAddress:    "0xDemoSeedOwner",
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		DemoOwnerUserID: &demoOwnerID,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		IsActive:        true,
	}
	assert.NoError(t, database.GetDB().Create(business).Error)
	table := &database.Table{BusinessID: business.ID, TableCode: fmt.Sprintf("T-%d", business.ID), Name: "A1"}
	assert.NoError(t, database.GetDB().Create(table).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("BILL-DEMO-GET-%d", business.ID),
		TotalAmount:    1200,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
	}
	assert.NoError(t, database.GetDB().Create(bill).Error)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "email")
			c.Set("role", "admin")
			c.Set("user_role", "admin")
			c.Set("user_id", uint(8))
			c.Set("address", "0xPlatformAdminWallet")
		},
		RequireBillBusinessAccess(),
		GetBill,
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestResolveBillBusinessAccess_ReloadsDemoFlagsForPlatformAdmin(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	demoOwnerID := uint(99)
	business := &database.Business{
		BusinessId:      "demo-admin-1-core-reload",
		Name:            "Payverge Core Demo Kitchen",
		OwnerAddress:    "0xDemoSeedOwner",
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		DemoOwnerUserID: &demoOwnerID,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		IsActive:        true,
	}
	assert.NoError(t, database.GetDB().Create(business).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("token_type", "email")
	c.Set("role", "admin")
	c.Set("user_role", "admin")
	c.Set("user_id", uint(8))
	c.Set("address", "0xPlatformAdminWallet")

	stripped := &database.Business{ID: business.ID}
	assert.False(t, CheckBusinessAccess(c, stripped), "zeroed is_demo/kind must deny before reload")

	resolved, ok := resolveBillBusinessAccess(c, stripped)
	assert.True(t, ok)
	if assert.NotNil(t, resolved) {
		assert.True(t, resolved.IsDemo)
		assert.Equal(t, database.BusinessKindDemo, resolved.Kind)
	}
}

func TestResolveBillBusinessAccess_HydratesLiveAdminRole(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	assert.NoError(t, database.GetDB().AutoMigrate(&database.User{}))
	admin := &database.User{Email: "qa-live-admin@local.test", Role: "admin"}
	assert.NoError(t, database.GetDB().Create(admin).Error)
	demoOwnerID := uint(99)
	business := &database.Business{
		BusinessId:      "demo-admin-live-role",
		Name:            "Payverge Core Demo Kitchen",
		OwnerAddress:    "0xDemoSeedOwner",
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		DemoOwnerUserID: &demoOwnerID,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		IsActive:        true,
	}
	assert.NoError(t, database.GetDB().Create(business).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("token_type", "email")
	c.Set("user_id", admin.ID)
	c.Set("address", "0xPlatformAdminWallet")
	// Intentionally omit role / user_role — production JWT claim may be missing.

	assert.False(t, CheckBusinessAccess(c, business), "without a role claim the listed demo must be denied")
	resolved, ok := resolveBillBusinessAccess(c, business)
	assert.True(t, ok)
	assert.NotNil(t, resolved)
	assert.True(t, hasPlatformAdminRole(c))
}

func TestRequireBillBusinessAccess_RejectsPlatformAdminUnownedLiveTenant(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xLiveTenantOwner", false)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "email")
			c.Set("role", "admin")
			c.Set("user_role", "admin")
			c.Set("user_id", uint(8))
			c.Set("address", "0xPlatformAdminWallet")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireBillBusinessAccess_AllowsOwnerForOwnBill(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", false)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xOwnerA")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRequireBillBusinessAccess_PopulatesOwnerContextForWeb3RBAC proves the fix
// for the web3-owner denial on bill routes: because bill routes carry :bill_id
// (not :id), the auth middleware never set business_owner_address, so the RBAC
// web3-owner branch fell through to a denial while OAuth owners passed. The
// middleware now populates the owner context from the verified business, so a
// web3 (SIWE) owner passes the downstream permission check exactly like OAuth.
func TestRequireBillBusinessAccess_PopulatesOwnerContextForWeb3RBAC(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", false)

	var ownerAddr interface{}
	var web3OwnerAllowed bool

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xOwnerA")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) {
			ownerAddr, _ = c.Get("business_owner_address")
			rbac := &RBACMiddleware{db: nil}
			web3OwnerAllowed = rbac.hasPermissions(c, "bills:refund")
			c.Status(http.StatusOK)
		},
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "0xOwnerA", ownerAddr, "owner address must be populated so the RBAC web3 branch can match")
	assert.True(t, web3OwnerAllowed, "web3 owner must now pass the bill-permission check")
}

func TestRequireBillBusinessAccess_RejectsCrossTenantOwner(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", false)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xOwnerB")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireBillBusinessAccess_RejectsCrossTenantOwnerBeforeSuspension(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", true)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xOwnerB")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireBillBusinessAccess_RejectsSuspendedBusiness(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", true)

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xOwnerA")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "business_suspended")
}

func TestRequireBillBusinessAccess_ProtectsBillPaymentRoutesForSuspendedBusiness(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", true)

	for _, tc := range []struct {
		method string
		route  string
		path   string
	}{
		{http.MethodPost, "/bills/:bill_id/alternative-payment", fmt.Sprintf("/bills/%d/alternative-payment", bill.ID)},
		{http.MethodGet, "/bills/:bill_id/pending-alternative-payments", fmt.Sprintf("/bills/%d/pending-alternative-payments", bill.ID)},
		{http.MethodPost, "/bills/:bill_id/pending-alternative-payments/:request_id/cancel", fmt.Sprintf("/bills/%d/pending-alternative-payments/1/cancel", bill.ID)},
		{http.MethodPost, "/bills/:bill_id/pending-alternative-payments/:request_id/reject", fmt.Sprintf("/bills/%d/pending-alternative-payments/1/reject", bill.ID)},
		{http.MethodGet, "/bills/:bill_id/payment-breakdown", fmt.Sprintf("/bills/%d/payment-breakdown", bill.ID)},
	} {
		t.Run(tc.route, func(t *testing.T) {
			r := setupBusinessAccessMiddlewareTest(t)
			r.Handle(tc.method, tc.route,
				func(c *gin.Context) {
					c.Set("token_type", "web3")
					c.Set("address", "0xOwnerA")
				},
				RequireBillBusinessAccess(),
				func(c *gin.Context) { c.Status(http.StatusOK) },
			)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Contains(t, w.Body.String(), "business_suspended")
		})
	}
}

func TestRequireBillBusinessAccess_AllowsSameBusinessStaffWithRBAC(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	business, bill := createBillAccessFixture(t, "0xOwnerA", false)
	staff := createStaffMember(t, business.ID, "server@example.com", "Server")

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "staff")
			c.Set("staff_id", staff.ID)
			c.Set("staff_role", string(database.StaffRoleServer))
			c.Set("staff_business_id", float64(business.ID))
			c.Set("staff_permission_denies", []string{})
		},
		RequireBillBusinessAccess(),
		RoleBasedAccessMiddleware("bills:read"),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireBillBusinessAccess_RejectsCrossBusinessStaffBeforeRBAC(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	business, bill := createBillAccessFixture(t, "0xOwnerA", false)
	otherBusiness, _ := createBillAccessFixture(t, "0xOwnerB", false)
	staff := createStaffMember(t, otherBusiness.ID, "server@example.com", "Server")

	r := setupBusinessAccessMiddlewareTest(t)
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "staff")
			c.Set("staff_id", staff.ID)
			c.Set("staff_role", string(database.StaffRoleServer))
			c.Set("staff_business_id", float64(otherBusiness.ID))
		},
		RequireBillBusinessAccess(),
		RoleBasedAccessMiddleware("bills:read"),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/bills/%d", bill.ID), nil)
	r.ServeHTTP(w, req)
	assert.NotEqual(t, business.ID, otherBusiness.ID)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func createTableAccessFixture(t *testing.T, ownerAddress string, ownerUserID *uint, suspended bool) (*database.Business, *database.Table) {
	t.Helper()
	business := &database.Business{
		BusinessId:     fmt.Sprintf("table-access-%s", ownerAddress),
		Name:           fmt.Sprintf("Table Access %s", ownerAddress),
		OwnerAddress:   ownerAddress,
		UserID:         ownerUserID,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	assert.NoError(t, database.GetDB().Create(business).Error)
	if suspended {
		assert.NoError(t, database.GetDB().Model(business).UpdateColumn("is_active", false).Error)
	}

	table := &database.Table{BusinessID: business.ID, TableCode: fmt.Sprintf("TBL-%d", business.ID), Name: "A1"}
	assert.NoError(t, database.GetDB().Create(table).Error)
	return business, table
}

// SEC-8 / #305: table middleware must verify tenant access and populate owner
// context so OAuth/web3 owners pass RBAC without fail-open.
func TestRequireTableBusinessAccess_PopulatesOwnerContextForOAuthRBAC(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	ownerUID := uint(77)
	_, table := createTableAccessFixture(t, "0xOwnerTable", &ownerUID, false)

	var ownerUIDCtx interface{}
	var oauthOwnerAllowed bool

	r := setupBusinessAccessMiddlewareTest(t)
	r.PUT("/tables/:id",
		func(c *gin.Context) {
			c.Set("token_type", "user")
			c.Set("user_id", float64(ownerUID))
		},
		RequireTableBusinessAccess(),
		func(c *gin.Context) {
			ownerUIDCtx, _ = c.Get("business_owner_user_id")
			rbac := &RBACMiddleware{db: nil}
			oauthOwnerAllowed = rbac.hasPermissions(c, "tables:write")
			c.Status(http.StatusOK)
		},
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/tables/%d", table.ID), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, ownerUID, ownerUIDCtx)
	assert.True(t, oauthOwnerAllowed, "OAuth owner must pass tables:write after owner context is populated")
}

func TestRequireTableBusinessAccess_RejectsCrossTenantOAuth(t *testing.T) {
	setupBillAccessMiddlewareDB(t)
	ownerUID := uint(77)
	_, table := createTableAccessFixture(t, "0xOwnerTable", &ownerUID, false)

	r := setupBusinessAccessMiddlewareTest(t)
	r.PUT("/tables/:id",
		func(c *gin.Context) {
			c.Set("token_type", "user")
			c.Set("user_id", float64(99))
		},
		RequireTableBusinessAccess(),
		RoleBasedAccessMiddleware("tables:write"),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/tables/%d", table.ID), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
