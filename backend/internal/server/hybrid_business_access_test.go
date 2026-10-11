package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// setupHybridBusinessAccessDB is a sqlite fixture for the list/open ownership
// rule. AutoMigrate is test-only — production schema stays numbered SQL.
func setupHybridBusinessAccessDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevDB := database.GetDB()
	prevStore := session.GlobalStore
	prevSecret := structs.SecretKey
	t.Cleanup(func() {
		database.SetTestDB(prevDB)
		session.GlobalStore = prevStore
		structs.SecretKey = prevSecret
	})

	gdb, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&database.Business{}, &database.User{}))

	database.SetTestDB(gdb)
	session.GlobalStore = nil
	structs.SecretKey = []byte("test-secret-key-for-hybrid-business-access")
	return gdb
}

func seedListedDemoBusiness(t *testing.T, gdb *gorm.DB, ownerUserID uint, slug, name string) *database.Business {
	t.Helper()
	uid := ownerUserID
	biz := &database.Business{
		BusinessId:      slug,
		Name:            name,
		OwnerAddress:    "0xDEMOOWNER000000000000000000000000000001",
		UserID:          &uid,
		DemoOwnerUserID: &uid,
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		IsActive:        true,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gdb.Create(biz).Error)
	return biz
}

func issueInsideUserToken(t *testing.T, userID uint, email, role string) string {
	t.Helper()
	if role == string(structs.RoleAdmin) {
		// HybridAuth re-checks admin claims against users.role (M-role).
		require.NoError(t, database.GetDB().Where(database.User{ID: userID}).
			Assign(database.User{Email: email, Role: role}).
			FirstOrCreate(&database.User{}).Error)
	}
	token, err := GenerateUserToken(userID, email, "", role)
	require.NoError(t, err)
	return token
}

func performHybridGetBusiness(t *testing.T, token, identifier string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/api/v1/inside/businesses/:id", GetBusiness)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/"+identifier, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Issue 362: platform admin can list demo venues they do not own, then Open
// business hits GET /inside/businesses/:slug and must use the same rule.
func TestHybridAuth_PlatformAdminCanGetListedDemoBusiness(t *testing.T) {
	gdb := setupHybridBusinessAccessDB(t)
	const (
		ownerID = uint(1)
		adminID = uint(8)
	)
	biz := seedListedDemoBusiness(t, gdb, ownerID, "demo-admin-1-ai-pro", "Payverge AI Pro Demo Lounge")

	adminToken := issueInsideUserToken(t, adminID, "qa-admin@example.com", string(structs.RoleAdmin))
	w := performHybridGetBusiness(t, adminToken, biz.BusinessId)

	require.Equal(t, http.StatusOK, w.Code, "platform admin must GET a demo they can list; body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), `"code":"BIZ_NOT_OWNER"`)
	assert.Contains(t, w.Body.String(), "Payverge AI Pro Demo Lounge")
	assert.Contains(t, w.Body.String(), "0x1111111111111111111111111111111111111111",
		"admin impersonate must receive the full owner record, not the public projection")
}

// BenchmarkHybridAuthGetBusiness is a Backend Performance Gate probe for the
// hybrid middleware + GET /inside/businesses/:id hot path after the shared
// list/open rule. Compare with BenchmarkGetBusinessAuthScope for the loader.
func BenchmarkHybridAuthGetBusiness(b *testing.B) {
	gin.SetMode(gin.TestMode)
	prevDB := database.GetDB()
	prevStore := session.GlobalStore
	prevSecret := structs.SecretKey
	b.Cleanup(func() {
		database.SetTestDB(prevDB)
		session.GlobalStore = prevStore
		structs.SecretKey = prevSecret
	})

	gdb, err := gorm.Open(sqlite.Open("file:hybrid-biz-access-bench?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := gdb.AutoMigrate(&database.Business{}); err != nil {
		b.Fatal(err)
	}
	database.SetTestDB(gdb)
	session.GlobalStore = nil
	structs.SecretKey = []byte("test-secret-key-for-hybrid-business-access")

	uid := uint(1)
	biz := &database.Business{
		BusinessId:     "bench-owner-biz",
		Name:           "Bench Cafe",
		OwnerAddress:   "0xBENCH",
		UserID:         &uid,
		IsActive:       true,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	if err := gdb.Where("business_id = ?", biz.BusinessId).FirstOrCreate(biz).Error; err != nil {
		b.Fatal(err)
	}
	token, err := GenerateUserToken(uid, "owner@bench.test", "", string(structs.RoleUser))
	if err != nil {
		b.Fatal(err)
	}

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/api/v1/inside/businesses/:id", GetBusiness)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/"+biz.BusinessId, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestHybridAuth_PlatformAdminCannotGetUnownedLiveBusiness(t *testing.T) {
	gdb := setupHybridBusinessAccessDB(t)
	ownerUID := uint(1)
	live := &database.Business{
		BusinessId:     "live-customer-cafe",
		Name:           "Live Customer Cafe",
		OwnerAddress:   "0xLIVEOWNER",
		UserID:         &ownerUID,
		IsDemo:         false,
		Kind:           database.BusinessKindReal,
		IsActive:       true,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gdb.Create(live).Error)

	adminToken := issueInsideUserToken(t, 8, "qa-admin@example.com", string(structs.RoleAdmin))
	w := performHybridGetBusiness(t, adminToken, live.BusinessId)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), ErrCodeNotBusinessOwner)
}

func TestHybridAuth_RegularUserStill403OnUnownedBusiness(t *testing.T) {
	gdb := setupHybridBusinessAccessDB(t)
	biz := seedListedDemoBusiness(t, gdb, 1, "demo-admin-1-core", "Payverge Core Demo Kitchen")

	strangerToken := issueInsideUserToken(t, 99, "stranger@example.com", string(structs.RoleUser))
	w := performHybridGetBusiness(t, strangerToken, biz.BusinessId)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), ErrCodeNotBusinessOwner)
}

func TestGetMyBusinesses_PlatformAdminListsUnownedDemo(t *testing.T) {
	gdb := setupHybridBusinessAccessDB(t)
	_ = seedListedDemoBusiness(t, gdb, 1, "demo-admin-1-ai-pro", "Payverge AI Pro Demo Lounge")
	_ = seedListedDemoBusiness(t, gdb, 1, "demo-admin-1-core", "Payverge Core Demo Kitchen")

	ownerUID := uint(8)
	owned := &database.Business{
		BusinessId:     "admin-8-owned",
		Name:           "Admin Owned Cafe",
		OwnerAddress:   "0xADMIN8",
		UserID:         &ownerUID,
		IsActive:       true,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gdb.Create(owned).Error)

	c, w := makeTestContext(http.MethodGet, "/api/v1/inside/businesses", nil)
	c.Set("token_type", "user")
	c.Set("user_id", float64(8))
	c.Set("role", string(structs.RoleAdmin))

	GetMyBusinesses(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listed []database.Business
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	names := make([]string, 0, len(listed))
	for _, b := range listed {
		names = append(names, b.Name)
	}
	assert.Contains(t, names, "Payverge AI Pro Demo Lounge")
	assert.Contains(t, names, "Payverge Core Demo Kitchen")
	assert.Contains(t, names, "Admin Owned Cafe")
}

func TestGetMyBusinesses_RegularUserDoesNotListOthersDemo(t *testing.T) {
	gdb := setupHybridBusinessAccessDB(t)
	_ = seedListedDemoBusiness(t, gdb, 1, "demo-admin-1-ai-pro", "Payverge AI Pro Demo Lounge")

	c, w := makeTestContext(http.MethodGet, "/api/v1/inside/businesses", nil)
	c.Set("token_type", "user")
	c.Set("user_id", float64(99))
	c.Set("role", string(structs.RoleUser))

	GetMyBusinesses(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listed []database.Business
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Empty(t, listed)
}
