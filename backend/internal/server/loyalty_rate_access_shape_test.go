package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLoyaltyRateAccessDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()
	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
	))
	services.ResetPricingCache()
	t.Cleanup(services.ResetPricingCache)
	return gormDB
}

func seedLoyaltyRateVenue(t testing.TB, tableCode string, rate float64) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId:      fmt.Sprintf("loyalty-rate-%d", time.Now().UnixNano()),
		Name:            "Loyalty Rate Bistro",
		OwnerAddress:    "0xLOYALTYRATEOWNER",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		IsActive:        true,
		OnboardingState: database.JSONRawMessage(`{"step":"should-not-be-selected"}`),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	require.NoError(t, database.GetDB().Create(&database.Table{
		BusinessID: business.ID,
		TableCode:  tableCode,
		Name:       "Rate Table",
		Capacity:   2,
		IsActive:   true,
	}).Error)
	program := &database.LoyaltyProgram{
		BusinessID:                business.ID,
		Enabled:                   true,
		PointsPerDollar:           1.25,
		RedemptionPointsPerDollar: rate,
	}
	require.NoError(t, database.GetDB().Create(program).Error)
	require.NoError(t, database.GetDB().Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Gold",
		MinLifetimeSpentCents: 50000,
		SortOrder:             1,
		Color:                 "#c9a227",
	}).Error)
	return business
}

func invokeGetLoyaltyRate(tableCode string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: tableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/table/"+tableCode+"/loyalty-rate", nil)
	GetLoyaltyRate(c)
	return w
}

// TestGetLoyaltyRateAccessShape is the #560 handler gate: one float, no
// discarded Tiers preload, no SELECT * on businesses (GetBusinessByID).
func TestGetLoyaltyRateAccessShape(t *testing.T) {
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupLoyaltyRateAccessDB(t, recorder)
	const tableCode = "RATE01"
	seedLoyaltyRateVenue(t, tableCode, 80)
	services.ResetPricingCache()

	recorder.statements = nil
	w := invokeGetLoyaltyRate(tableCode)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, 80.0, payload["points_per_currency_unit"])

	joined := strings.ToLower(strings.Join(recorder.statements, "\n"))
	assert.NotContains(t, joined, "loyalty_tiers",
		"GET loyalty-rate must not query loyalty_tiers; Tiers are discarded")
	assert.Zero(t, recorder.selectStarCount("businesses"),
		"GET loyalty-rate must not SELECT * businesses (use the cached public projection)")
	assert.Zero(t, recorder.selectStarCount("loyalty_programs"),
		"GET loyalty-rate must project the redeem float, not SELECT * loyalty_programs")

	selects := 0
	for _, stmt := range recorder.statements {
		normalized := strings.ToLower(strings.TrimSpace(stmt))
		if strings.HasPrefix(normalized, "select") {
			selects++
		}
	}
	assert.LessOrEqual(t, selects, 2,
		"loyalty-rate should be table+business join + one rate projection, got %d: %v",
		selects, recorder.statements)
}

func TestGetLoyaltyRate_ClosedBusiness(t *testing.T) {
	setupLoyaltyRateAccessDB(t, nil)
	const tableCode = "CLOSED1"
	business := seedLoyaltyRateVenue(t, tableCode, 80)
	require.NoError(t, database.GetDB().Model(business).UpdateColumn("closed_at", time.Now().Add(-time.Hour)).Error)
	services.ResetPricingCache()

	w := invokeGetLoyaltyRate(tableCode)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

// A suspended business is hidden by the public table loader, exactly like an
// unknown table code.
func TestGetLoyaltyRate_SuspendedBusinessIsNotFound(t *testing.T) {
	setupLoyaltyRateAccessDB(t, nil)
	const tableCode = "SUSPENDED1"
	business := seedLoyaltyRateVenue(t, tableCode, 80)
	require.NoError(t, database.GetDB().Model(business).UpdateColumn("is_active", false).Error)
	services.ResetPricingCache()

	w := invokeGetLoyaltyRate(tableCode)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetLoyaltyRate_UnknownTable(t *testing.T) {
	setupLoyaltyRateAccessDB(t, nil)
	w := invokeGetLoyaltyRate("MISSING")
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func BenchmarkGetLoyaltyRate(b *testing.B) {
	setupLoyaltyRateAccessDB(b, logger.Default.LogMode(logger.Silent))
	const tableCode = "BENCH01"
	seedLoyaltyRateVenue(b, tableCode, 100)
	services.ResetPricingCache()
	// Prime the table-context cache the way a warmed guest poll would.
	_, _, err := loadPublicGuestTableContext(tableCode)
	if err != nil {
		// Current code does not use the cached loader; fall through and
		// measure the live handler either way.
		_ = err
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := invokeGetLoyaltyRate(tableCode)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
	}
}
