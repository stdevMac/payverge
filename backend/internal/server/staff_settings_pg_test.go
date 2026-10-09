//go:build integration
// +build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// TestManagerUpdatesOperatingHours_PostgresIntegration verifies the full chain
// for the highest-stakes settings write: auth + RBAC + handler + persistence.
// The status-code matrix can't catch the case where the gate lets the request
// through but the row never lands; this test reads back from the table.
func TestManagerUpdatesOperatingHours_PostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}

	if len(structs.SecretKey) == 0 {
		structs.SecretKey = []byte("rbac-pg-test-secret")
	}

	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open postgres")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping(), "ping postgres")

	database.SetTestDB(gormDB)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.BusinessOperatingHours{},
	), "automigrate operating-hours tables")
	useIntegrationSessionStore(t, gormDB)

	bizID := time.Now().UnixNano()
	biz := &database.Business{
		BusinessId:     fmt.Sprintf("oph-%d", bizID),
		OwnerAddress:   "0x0000000000000000000000000000000000000abc",
		Name:           "OpHours Biz",
		SettlementAddr: "0x0000000000000000000000000000000000000def",
		TippingAddr:    "0x0000000000000000000000000000000000000fed",
	}
	require.NoError(t, gormDB.Create(biz).Error, "create business")

	mgr := &database.Staff{
		BusinessID: biz.ID,
		Name:       "Manager",
		Email:      fmt.Sprintf("mgr-%d@payverge.test", bizID),
		Role:       database.StaffRoleManager,
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(mgr).Error, "create staff manager")

	t.Cleanup(func() {
		if err := gormDB.Exec("DELETE FROM business_operating_hours WHERE business_id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup business_operating_hours: %v", err)
		}
		if err := gormDB.Exec("DELETE FROM staff WHERE business_id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup staff: %v", err)
		}
		if err := gormDB.Exec("DELETE FROM businesses WHERE id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup businesses: %v", err)
		}
	})

	tok := generateIntegrationStaffToken(t, mgr)

	router := server.BuildRouterForTest(gormDB)

	// The handler binds to []database.BusinessOperatingHours. Send a realistic
	// week-shaped payload so we can verify the row count and field values.
	hours := []database.BusinessOperatingHours{
		{DayOfWeek: 1, OpenTime: "09:00", CloseTime: "22:00", IsClosed: false},
		{DayOfWeek: 2, OpenTime: "09:00", CloseTime: "22:00", IsClosed: false},
		{DayOfWeek: 0, OpenTime: "", CloseTime: "", IsClosed: true},
	}
	body, err := json.Marshal(hours)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/inside/businesses/%d/operating-hours", biz.ID),
		bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: tok})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equalf(t, http.StatusOK, w.Code,
		"manager should be able to update operating hours; got body=%s", w.Body.String())

	// Verify the rows landed in the operating-hours table for this business.
	var persisted []database.BusinessOperatingHours
	require.NoError(t,
		gormDB.Where("business_id = ?", biz.ID).Order("day_of_week ASC").Find(&persisted).Error,
		"reload persisted operating hours")
	require.Len(t, persisted, 3, "all three days persisted")

	byDay := map[int]database.BusinessOperatingHours{}
	for _, h := range persisted {
		byDay[h.DayOfWeek] = h
	}

	assert.Equal(t, "09:00", byDay[1].OpenTime, "monday open time persisted")
	assert.Equal(t, "22:00", byDay[1].CloseTime, "monday close time persisted")
	assert.False(t, byDay[1].IsClosed, "monday open")
	assert.True(t, byDay[0].IsClosed, "sunday closed flag persisted")
}

// TestUpdateBusiness_StripsWalletForStaff_PostgresIntegration verifies that
// when a staff manager hits PUT /businesses/:id with a settlement_address in
// the body, the field is silently dropped — name updates land, wallet doesn't.
// This is the security-critical assertion the matrix can't make: matrix only
// checks status code, not whether the wallet field was actually overwritten.
func TestUpdateBusiness_StripsWalletForStaff_PostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}

	if len(structs.SecretKey) == 0 {
		structs.SecretKey = []byte("rbac-pg-test-secret")
	}

	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open postgres")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping(), "ping postgres")

	database.SetTestDB(gormDB)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
	), "automigrate business+staff")
	useIntegrationSessionStore(t, gormDB)

	bizID := time.Now().UnixNano()
	originalSettlement := "0x0000000000000000000000000000000000000aaa"
	originalTipping := "0x0000000000000000000000000000000000000bbb"
	biz := &database.Business{
		BusinessId:     fmt.Sprintf("strip-%d", bizID),
		OwnerAddress:   "0x0000000000000000000000000000000000000abc",
		Name:           "Strip Test",
		SettlementAddr: originalSettlement,
		TippingAddr:    originalTipping,
	}
	require.NoError(t, gormDB.Create(biz).Error, "create business")

	mgr := &database.Staff{
		BusinessID: biz.ID,
		Name:       "Mgr",
		Email:      fmt.Sprintf("strip-mgr-%d@payverge.test", bizID),
		Role:       database.StaffRoleManager,
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(mgr).Error, "create staff manager")

	t.Cleanup(func() {
		if err := gormDB.Exec("DELETE FROM staff WHERE business_id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup staff: %v", err)
		}
		if err := gormDB.Exec("DELETE FROM businesses WHERE id = ?", biz.ID).Error; err != nil {
			t.Logf("cleanup businesses: %v", err)
		}
	})

	tok := generateIntegrationStaffToken(t, mgr)

	router := server.BuildRouterForTest(gormDB)

	body, err := json.Marshal(map[string]any{
		"name":               "Renamed by Manager",
		"settlement_address": "0x000000000000000000000000000000000000dEaD",
		"tipping_address":    "0x000000000000000000000000000000000000bEEf",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/inside/businesses/%d", biz.ID),
		bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: tok})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equalf(t, http.StatusOK, w.Code,
		"manager updates business name OK; body=%s", w.Body.String())

	var reloaded database.Business
	require.NoError(t, gormDB.First(&reloaded, biz.ID).Error, "reload business")
	assert.Equal(t, "Renamed by Manager", reloaded.Name, "name field landed")
	assert.Equal(t, originalSettlement, reloaded.SettlementAddr,
		"settlement address unchanged — sanitizer dropped the staff write")
	assert.Equal(t, originalTipping, reloaded.TippingAddr,
		"tipping address unchanged — sanitizer dropped the staff write")
}
