package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTableHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	disableAsyncOnboardingStampForTest(t)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.Table{},
	))
	InitializeRBAC(database.GetDBWrapper())

	return gormDB
}

func createTableHandlerBusiness(t *testing.T, suffix string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("table-biz-%s", suffix),
		Name:            fmt.Sprintf("Table Biz %s", suffix),
		OwnerAddress:    fmt.Sprintf("0x%s", suffix),
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createTableHandlerStaff(t *testing.T, businessID uint, role database.StaffRole, email string) *database.Staff {
	t.Helper()

	staff := &database.Staff{
		BusinessID: businessID,
		Email:      email,
		Name:       string(role),
		Role:       role,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	return staff
}

func createTableHandlerRecord(t *testing.T, businessID uint, name string) *database.Table {
	t.Helper()

	table := &database.Table{
		BusinessID: businessID,
		TableCode:  fmt.Sprintf("T-%d-%s", businessID, name),
		Name:       name,
		Capacity:   4,
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	return table
}

func performTableHandlerRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestUpdateTableDetails_StaffIsScopedToOwnBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)

	ownerBusiness := createTableHandlerBusiness(t, "owner")
	otherBusiness := createTableHandlerBusiness(t, "other")
	manager := createTableHandlerStaff(t, ownerBusiness.ID, database.StaffRoleManager, "manager@example.com")

	ownedTable := createTableHandlerRecord(t, ownerBusiness.ID, "Owned")
	otherTable := createTableHandlerRecord(t, otherBusiness.ID, "Other")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", ownerBusiness.ID)
		c.Next()
	})
	router.PUT("/inside/tables/:id", RoleBasedAccessMiddleware("tables:write"), UpdateTableDetails)

	t.Run("same business staff can update the table", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodPut, fmt.Sprintf("/inside/tables/%d", ownedTable.ID), map[string]any{
			"name":      "Renamed",
			"capacity":  6,
			"is_active": true,
		})

		assert.Equal(t, http.StatusOK, w.Code)

		reloaded, err := database.GetTableByID(ownedTable.ID)
		require.NoError(t, err)
		assert.Equal(t, "Renamed", reloaded.Name)
		assert.Equal(t, 6, reloaded.Capacity)
	})

	t.Run("cross business staff is forbidden", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodPut, fmt.Sprintf("/inside/tables/%d", otherTable.ID), map[string]any{
			"name":      "Hacked",
			"capacity":  9,
			"is_active": true,
		})

		assert.Equal(t, http.StatusForbidden, w.Code)

		reloaded, err := database.GetTableByID(otherTable.ID)
		require.NoError(t, err)
		assert.Equal(t, "Other", reloaded.Name)
		assert.Equal(t, 4, reloaded.Capacity)
	})
}

func TestDeleteTableSoft_StaffCanDeleteOnlyOwnBusinessTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)

	ownerBusiness := createTableHandlerBusiness(t, "delete-owner")
	otherBusiness := createTableHandlerBusiness(t, "delete-other")
	manager := createTableHandlerStaff(t, ownerBusiness.ID, database.StaffRoleManager, "delete-manager@example.com")

	ownedTable := createTableHandlerRecord(t, ownerBusiness.ID, "OwnedDelete")
	otherTable := createTableHandlerRecord(t, otherBusiness.ID, "OtherDelete")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", ownerBusiness.ID)
		c.Next()
	})
	router.DELETE("/inside/tables/:id", RoleBasedAccessMiddleware("tables:delete"), DeleteTableSoft)

	t.Run("same business staff can delete", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodDelete, fmt.Sprintf("/inside/tables/%d", ownedTable.ID), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		reloaded, err := database.GetTableByID(ownedTable.ID)
		require.NoError(t, err)
		assert.False(t, reloaded.IsActive)
	})

	t.Run("cross business staff is forbidden", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodDelete, fmt.Sprintf("/inside/tables/%d", otherTable.ID), nil)
		assert.Equal(t, http.StatusForbidden, w.Code)

		reloaded, err := database.GetTableByID(otherTable.ID)
		require.NoError(t, err)
		assert.Equal(t, otherBusiness.ID, reloaded.BusinessID)
	})

	// L3-30: soft-delete on an already-inactive row must not silent-200.
	// ownedTable was soft-deleted in the first subtest.
	t.Run("already inactive table returns 404", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodDelete, fmt.Sprintf("/inside/tables/%d", ownedTable.ID), nil)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})
}

func TestHybridAuth_DoesNotTreatLegacyTableIDRouteAsBusinessID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Table{}))

	_ = createOwnedBusiness(t, "0xOtherTableOwner", "Other Table Middleware Biz")
	business := createOwnedBusiness(t, "0xTableOwner", "Target Table Middleware Biz")
	manager := createStaffMember(t, business.ID, "table-middleware@example.com", "Table Middleware Staff")
	token := generateStaffTokenWithSession(t, manager)
	table := createTableHandlerRecord(t, business.ID, "Patio")

	require.NotEqual(t, business.ID, table.ID, "test must prove :id is the table id, not the business id")

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.PUT("/inside/tables/:id", RoleBasedAccessMiddleware("tables:write"), UpdateTableDetails)

	body, err := json.Marshal(map[string]any{
		"name":      "Patio Renamed",
		"capacity":  8,
		"is_active": true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/inside/tables/%d", table.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	reloaded, err := database.GetTableByID(table.ID)
	require.NoError(t, err)
	assert.Equal(t, "Patio Renamed", reloaded.Name)
	assert.Equal(t, 8, reloaded.Capacity)
}

func TestGetTable_UsesRegisteredBusinessIDParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	business := createTableHandlerBusiness(t, "getroute")
	table := createTableHandlerRecord(t, business.ID, "Window")

	router := gin.New()
	router.GET("/inside/businesses/:id/tables/:tableId", func(c *gin.Context) {
		c.Set("address", business.OwnerAddress)
		GetTable(c)
	})

	w := performTableHandlerRequest(t, router, http.MethodGet,
		"/inside/businesses/"+business.BusinessId+"/tables/"+strconv.Itoa(int(table.ID)),
		nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, table.ID, resp["id"])
	require.Equal(t, "Window", resp["name"])
}

func TestCreateTableWithQR_ResponseIncludesCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	business := createTableHandlerBusiness(t, "createcap")

	router := gin.New()
	router.POST("/inside/businesses/:id/tables", func(c *gin.Context) {
		c.Set("address", business.OwnerAddress)
		CreateTableWithQR(c)
	})

	w := performTableHandlerRequest(t, router, http.MethodPost,
		"/inside/businesses/"+business.BusinessId+"/tables",
		CreateTableRequest{Name: "Patio", Capacity: 10})

	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Regression: the create response previously hand-rolled a subset of fields
	// and omitted capacity (and business_id, updated_at, QR overrides…), so a
	// freshly created table rendered 0 seats in the operator list — and any
	// other field the row/drawer reads stayed stale — until a full refresh
	// refetched it from /tables/status. The response must now carry the full
	// persisted table so the optimistic list insert is identical to a refetch.
	capacity, ok := resp["capacity"]
	require.True(t, ok, "create response must include capacity")
	require.EqualValues(t, 10, capacity)

	businessID, ok := resp["business_id"]
	require.True(t, ok, "create response must include business_id")
	require.EqualValues(t, business.ID, businessID)

	require.NotEmpty(t, resp["table_code"], "create response must include table_code")
	require.Equal(t, true, resp["is_active"], "create response must include is_active")
	require.NotEmpty(t, resp["updated_at"], "create response must include updated_at")
}

func TestUpdateTable_AppliesCapacityWhenProvided(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	business := createTableHandlerBusiness(t, "cap")
	table := createTableHandlerRecord(t, business.ID, "Patio")

	router := gin.New()
	router.PUT("/inside/businesses/:id/tables/:tableId", func(c *gin.Context) {
		c.Set("address", business.OwnerAddress)
		UpdateTable(c)
	})

	cap := 8
	active := true
	w := performTableHandlerRequest(t, router, http.MethodPut,
		"/inside/businesses/"+business.BusinessId+"/tables/"+strconv.Itoa(int(table.ID)),
		UpdateTableRequest{Name: "Patio", Capacity: &cap, IsActive: &active})

	require.Equal(t, http.StatusOK, w.Code)

	reloaded, err := database.GetTableByID(table.ID)
	require.NoError(t, err)
	require.Equal(t, 8, reloaded.Capacity)
}

func TestUpdateTable_LeavesCapacityUnchangedWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	business := createTableHandlerBusiness(t, "capomit")
	table := createTableHandlerRecord(t, business.ID, "Booth") // created with Capacity 4

	router := gin.New()
	router.PUT("/inside/businesses/:id/tables/:tableId", func(c *gin.Context) {
		c.Set("address", business.OwnerAddress)
		UpdateTable(c)
	})

	active := true
	w := performTableHandlerRequest(t, router, http.MethodPut,
		"/inside/businesses/"+business.BusinessId+"/tables/"+strconv.Itoa(int(table.ID)),
		UpdateTableRequest{Name: "Booth", IsActive: &active}) // no capacity

	require.Equal(t, http.StatusOK, w.Code)

	reloaded, err := database.GetTableByID(table.ID)
	require.NoError(t, err)
	require.Equal(t, 4, reloaded.Capacity)
}

// R3-OK-1: the operator UI sends partial bodies (name-only, capacity-only) when
// renaming or changing seats. The old handler bound a non-pointer request whose
// is_active defaulted to false, silently deactivating the table and breaking its
// guest QR. A partial update that omits is_active must leave activation intact.
func TestUpdateTableDetails_PartialUpdatePreservesIsActive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	business := createTableHandlerBusiness(t, "partialactive")
	table := createTableHandlerRecord(t, business.ID, "Window") // created IsActive: true

	router := gin.New()
	router.PUT("/inside/tables/:id", func(c *gin.Context) {
		c.Set("address", business.OwnerAddress)
		UpdateTableDetails(c)
	})

	t.Run("name-only update keeps table active", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodPut,
			"/inside/tables/"+strconv.Itoa(int(table.ID)),
			map[string]any{"name": "Window Renamed"})

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		reloaded, err := database.GetTableByID(table.ID)
		require.NoError(t, err)
		assert.Equal(t, "Window Renamed", reloaded.Name)
		assert.True(t, reloaded.IsActive, "partial update must NOT deactivate the table")
	})

	t.Run("capacity-only update keeps table active", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodPut,
			"/inside/tables/"+strconv.Itoa(int(table.ID)),
			map[string]any{"capacity": 6})

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		reloaded, err := database.GetTableByID(table.ID)
		require.NoError(t, err)
		assert.Equal(t, 6, reloaded.Capacity)
		assert.True(t, reloaded.IsActive, "capacity-only update must NOT deactivate the table")
	})

	t.Run("explicit is_active:false still deactivates", func(t *testing.T) {
		w := performTableHandlerRequest(t, router, http.MethodPut,
			"/inside/tables/"+strconv.Itoa(int(table.ID)),
			map[string]any{"is_active": false})

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		reloaded, err := database.GetTableByID(table.ID)
		require.NoError(t, err)
		assert.False(t, reloaded.IsActive, "explicit is_active:false must deactivate")
	})
}

// R3-OK-5: the update response must carry the full table row so the operator
// UI's local-state swap doesn't wipe qr_*/business_id fields. The previous
// 7-field subset dropped them, blanking QR customization until a refetch.
func TestUpdateTableDetails_ReturnsFullTableRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	business := createTableHandlerBusiness(t, "fullrow")
	table := createTableHandlerRecord(t, business.ID, "Booth")
	// Seed a QR override so we can prove it survives the round-trip.
	table.QRForegroundColor = "#123456"
	table.QRBackgroundColor = "#abcdef"
	require.NoError(t, database.UpdateTable(table))

	router := gin.New()
	router.PUT("/inside/tables/:id", func(c *gin.Context) {
		c.Set("address", business.OwnerAddress)
		UpdateTableDetails(c)
	})

	w := performTableHandlerRequest(t, router, http.MethodPut,
		"/inside/tables/"+strconv.Itoa(int(table.ID)),
		map[string]any{"name": "Booth 2"})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	businessID, ok := resp["business_id"]
	require.True(t, ok, "update response must include business_id")
	require.EqualValues(t, business.ID, businessID)
	assert.Equal(t, "#123456", resp["qr_foreground_color"], "QR foreground must survive the update response")
	assert.Equal(t, "#abcdef", resp["qr_background_color"], "QR background must survive the update response")
	require.NotEmpty(t, resp["table_code"], "update response must include table_code")
}
