package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// dashboardSummariesSQLRecorder captures executed SQL so the batching test can
// assert the multi-business handler loads businesses with a single IN(...) query
// instead of one First() per id (N1-01).
type dashboardSummariesSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *dashboardSummariesSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

// businessSelectCount counts how many SELECT ... FROM businesses statements ran.
func (r *dashboardSummariesSQLRecorder) businessSelectCount() int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `businesses`") ||
			strings.Contains(normalized, "from \"businesses\"") ||
			strings.Contains(normalized, "from businesses") {
			count++
		}
	}
	return count
}

// businessInQueryCount counts business SELECTs that use a WHERE ... id IN (...)
// shape — the batched read the fix introduces.
func (r *dashboardSummariesSQLRecorder) businessInQueryCount() int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if !(strings.Contains(normalized, "from `businesses`") ||
			strings.Contains(normalized, "from \"businesses\"") ||
			strings.Contains(normalized, "from businesses")) {
			continue
		}
		if strings.Contains(normalized, "id in (") {
			count++
		}
	}
	return count
}

func setupDashboardSummariesPerfDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BusinessRevenueAggregate{},
	))
	server.InitializeRBAC(database.GetDBWrapper())
	return gormDB
}

func createDashboardSummariesBusiness(t testing.TB, ownerAddress, suffix string, mutate func(*database.Business)) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId:      fmt.Sprintf("dash-sum-%s", suffix),
		Name:            "Dashboard Summaries Biz " + suffix,
		OwnerAddress:    ownerAddress,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	if mutate != nil {
		mutate(business)
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func performDashboardSummariesRequest(t testing.TB, handler *AnalyticsHandler, ownerAddress string, ids []uint) (int, gin.H) {
	t.Helper()

	payload, err := json.Marshal(map[string]interface{}{"business_ids": ids})
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", ownerAddress)
		c.Set("business_owner_address", ownerAddress)
		c.Next()
	})
	router.POST(
		"/inside/analytics/dashboard-summaries",
		server.AllowUserLevelPermissions(),
		server.RoleBasedAccessMiddleware("overview:kpi"),
		handler.GetDashboardSummaries,
	)

	req := httptest.NewRequest(http.MethodPost, "/inside/analytics/dashboard-summaries", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var parsed gin.H
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w.Code, parsed
}

// TestDashboardSummariesBatchesBusinessLoad proves the multi-business summary
// handler loads businesses in a single IN(...) query rather than one First()
// per requested id (N1-01). RED before the fix: the old loop issues M selects.
func TestDashboardSummariesBatchesBusinessLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &dashboardSummariesSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupDashboardSummariesPerfDB(t, recorder)

	const owner = "0xBatchDashOwner"
	const m = 6
	ids := make([]uint, 0, m)
	for i := 0; i < m; i++ {
		b := createDashboardSummariesBusiness(t, owner, fmt.Sprintf("batch-%d", i), nil)
		ids = append(ids, b.ID)
	}

	// Reset capture to ignore the seeding inserts/selects.
	recorder.statements = nil

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	code, body := performDashboardSummariesRequest(t, handler, owner, ids)
	require.Equal(t, http.StatusOK, code, "body: %v", body)

	assert.Equal(t, 1, recorder.businessSelectCount(),
		"business load must be a single batched query, not one First() per id")
	assert.Equal(t, 1, recorder.businessInQueryCount(),
		"business load must use a WHERE id IN (...) batched read")
}

// TestDashboardSummariesLockStateParity is the bug guard: it proves the
// projected columns reproduce IsBusinessOperational exactly. An
// administrator-closed business must be skipped; an active one must be
// allowed. Dropping is_active or closed_at from the projection silently
// fail-opens/closes the gate.
func TestDashboardSummariesLockStateParity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupDashboardSummariesPerfDB(t, logger.Default.LogMode(logger.Silent))

	const owner = "0xLockParityOwner"
	closedAt := time.Now().UTC().Add(-time.Hour)

	locked := createDashboardSummariesBusiness(t, owner, "closed", func(b *database.Business) {
		b.ClosedAt = &closedAt
	})
	active := createDashboardSummariesBusiness(t, owner, "active", nil)

	handler := NewAnalyticsHandler(database.GetDBWrapper())

	// Full-row oracle: load each row in full and compute the expected gate so the
	// test asserts parity against the real consumer, not a hand-coded constant.
	var lockedFull, activeFull database.Business
	require.NoError(t, database.GetDB().First(&lockedFull, locked.ID).Error)
	require.NoError(t, database.GetDB().First(&activeFull, active.ID).Error)
	require.False(t, database.IsBusinessOperational(&lockedFull), "oracle: closed business should be inactive")
	require.True(t, database.IsBusinessOperational(&activeFull), "oracle: active business should be active")

	// Closed → skipped (200 with empty data). One inactive business must not
	// fail the whole batch on the cross-business overview page.
	code, body := performDashboardSummariesRequest(t, handler, owner, []uint{locked.ID})
	assert.Equal(t, http.StatusOK, code, "closed-only batch should succeed with empty data; body: %v", body)
	data, _ := body["data"].(map[string]interface{})
	require.NotNil(t, data)
	assert.NotContains(t, data, fmt.Sprint(locked.ID))

	// Active → 200 and the business is present in the data map.
	code, body = performDashboardSummariesRequest(t, handler, owner, []uint{active.ID})
	require.Equal(t, http.StatusOK, code, "active business must be allowed; body: %v", body)
	data, _ = body["data"].(map[string]interface{})
	require.NotNil(t, data)
	assert.Contains(t, data, fmt.Sprint(active.ID))
}

// TestDashboardSummariesShortCircuits preserves the exact 400/404/403 order
// and dedup behavior across the batched rewrite.
func TestDashboardSummariesShortCircuits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupDashboardSummariesPerfDB(t, logger.Default.LogMode(logger.Silent))

	const owner = "0xShortCircuitOwner"
	const otherOwner = "0xOtherOwner"
	mine := createDashboardSummariesBusiness(t, owner, "mine", nil)
	other := createDashboardSummariesBusiness(t, otherOwner, "other", nil)
	handler := NewAnalyticsHandler(database.GetDBWrapper())

	t.Run("zero id → 400", func(t *testing.T) {
		code, body := performDashboardSummariesRequest(t, handler, owner, []uint{0})
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Invalid business id", body["error"])
	})

	t.Run("missing id → 404", func(t *testing.T) {
		code, body := performDashboardSummariesRequest(t, handler, owner, []uint{999999})
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "Business not found", body["error"])
	})

	t.Run("not owned → 403", func(t *testing.T) {
		code, body := performDashboardSummariesRequest(t, handler, owner, []uint{other.ID})
		assert.Equal(t, http.StatusForbidden, code)
		assert.Equal(t, "Access denied", body["error"])
	})

	t.Run("duplicate ids deduped", func(t *testing.T) {
		code, body := performDashboardSummariesRequest(t, handler, owner, []uint{mine.ID, mine.ID, mine.ID})
		require.Equal(t, http.StatusOK, code, "body: %v", body)
		data, _ := body["data"].(map[string]interface{})
		require.NotNil(t, data)
		assert.Len(t, data, 1)
		assert.Contains(t, data, fmt.Sprint(mine.ID))
	})
}

func BenchmarkDashboardSummariesBusinessLoadSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	setupDashboardSummariesPerfDB(b, logger.Default.LogMode(logger.Silent))

	const owner = "0xBenchDashOwner"
	const m = 20
	ids := make([]uint, 0, m)
	for i := 0; i < m; i++ {
		biz := createDashboardSummariesBusiness(b, owner, fmt.Sprintf("bench-%d", i), nil)
		ids = append(ids, biz.ID)
	}
	handler := NewAnalyticsHandler(database.GetDBWrapper())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		code, body := performDashboardSummariesRequest(b, handler, owner, ids)
		if code != http.StatusOK {
			b.Fatalf("expected 200, got %d: %v", code, body)
		}
	}
}
