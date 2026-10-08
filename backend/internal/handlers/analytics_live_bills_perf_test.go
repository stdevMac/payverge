package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
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

type analyticsSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *analyticsSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *analyticsSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *analyticsSQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func setupAnalyticsLiveBillsPerfDB(t testing.TB, gormLogger logger.Interface) (*database.Business, []*database.Table) {
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
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

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

	business := &database.Business{
		BusinessId:     fmt.Sprintf("analytics-live-%d", time.Now().UnixNano()),
		Name:           "Analytics Live Perf",
		OwnerAddress:   fmt.Sprintf("0xliveperf%x", time.Now().UnixNano()),
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(business).Error)

	tables := make([]*database.Table, 0, 50)
	for i := 0; i < 50; i++ {
		table := &database.Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("LIVE-%02d", i),
			Name:       fmt.Sprintf("Live Table %02d", i),
			Capacity:   2 + (i % 6),
			QRCode:     strings.Repeat("qr-payload", 128),
			IsActive:   true,
		}
		require.NoError(t, gormDB.Create(table).Error)
		tables = append(tables, table)
	}

	legacyItemsPayload := strings.Repeat(`{"id":"item","name":"Bench","price":12},`, 128)
	now := time.Now().UTC()
	for i := 0; i < 250; i++ {
		table := tables[i%len(tables)]
		require.NoError(t, gormDB.Create(&database.Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("LIVE-BILL-%03d", i),
			Items:          "[" + legacyItemsPayload + "{}]",
			Subtotal:       4000,
			TotalAmount:    4500,
			PaidAmount:     int64(i % 3 * 500),
			TipAmount:      300,
			Status:         database.BillStatusOpen,
			SettlementAddr: "settlement",
			TippingAddr:    "tipping",
			CreatedAt:      now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      now.Add(-time.Duration(i) * time.Minute),
		}).Error)
	}

	return business, tables
}

func performAnalyticsLiveBillsRequest(t testing.TB, handler *AnalyticsHandler, business *database.Business) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/live-bills", business.ID), nil)
	c.Set("address", business.OwnerAddress)
	handler.GetLiveBills(c)
	return w
}

func TestAnalyticsLiveBillsUsesProjectedBillAndTableReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &analyticsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, _ := setupAnalyticsLiveBillsPerfDB(t, recorder)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	w := performAnalyticsLiveBillsRequest(t, handler, business)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
	assert.Zero(t, recorder.selectStarCount("bills"), "live bills should project summary bill fields instead of hydrating full bill rows")
	assert.Zero(t, recorder.selectStarCount("tables"), "live bills should project table name/code instead of hydrating full table rows")
}

// TestAnalyticsLiveBillsBoundsPayload asserts the live-bills endpoint no longer
// returns an unbounded list: the 250 seeded open bills are capped and the
// response flags it. Before this fix the handler streamed every active bill on
// every poll cycle.
func TestAnalyticsLiveBillsBoundsPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := setupAnalyticsLiveBillsPerfDB(t, nil)

	handler := NewAnalyticsHandler(database.GetDBWrapper())

	// Default cap: 250 seeded bills > DefaultActiveBillSummaryLimit (200).
	w := performAnalyticsLiveBillsRequest(t, handler, business)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"capped":true`, "250 open bills must trip the capped flag at the default limit")

	// An explicit small limit bounds the returned data and still reports capping.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c2.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/live-bills?limit=10", business.ID), nil)
	// gin.CreateTestContext doesn't parse the raw URL query into c.Query without
	// a matching route; set it explicitly via the request URL.
	c2.Set("address", business.OwnerAddress)
	handler.GetLiveBills(c2)
	require.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), `"limit":10`)
	assert.Contains(t, w2.Body.String(), `"capped":true`)
}

// TestGetActiveBillSummariesBoundedCapsAndFlags is the database-level regression:
// the bounded summary read returns at most `limit` rows and reports capping.
func TestGetActiveBillSummariesBoundedCapsAndFlags(t *testing.T) {
	business, _ := setupAnalyticsLiveBillsPerfDB(t, nil)
	db := database.GetDBWrapper()

	rows, capped, err := db.GetActiveBillSummariesByBusinessIDBounded(business.ID, 25)
	require.NoError(t, err)
	assert.Len(t, rows, 25, "bounded read must not exceed the limit")
	assert.True(t, capped, "250 open bills at limit 25 must report capped")

	// A limit above the hard cap is clamped to DefaultActiveBillSummaryLimit.
	all, cappedAll, err := db.GetActiveBillSummariesByBusinessIDBounded(business.ID, 10000)
	require.NoError(t, err)
	assert.Len(t, all, database.DefaultActiveBillSummaryLimit)
	assert.True(t, cappedAll, "250 open bills at the hard cap must report capped")
}

func TestAnalyticsLiveBillsClampsOversizedLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &analyticsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, _ := setupAnalyticsLiveBillsPerfDB(t, recorder)
	before := len(recorder.statements)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/live-bills?limit=1000000", business.ID), nil)
	c.Set("address", business.OwnerAddress)
	handler.GetLiveBills(c)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data  []json.RawMessage `json:"data"`
		Limit int               `json:"limit"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.LessOrEqual(t, len(body.Data), database.DefaultActiveBillSummaryLimit)
	assert.LessOrEqual(t, body.Limit, database.DefaultActiveBillSummaryLimit)
	assert.NotEmpty(t, body.Data)

	limits := sqlLimitValues(recorder.statements[before:])
	require.NotEmpty(t, limits, "expected a LIMIT clause, got %v", recorder.statements[before:])
	for _, n := range limits {
		assert.LessOrEqual(t, n, database.DefaultActiveBillSummaryLimit+1)
	}
}

func TestAnalyticsPopularItemsClampsOversizedLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &analyticsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, _ := setupAnalyticsLiveBillsPerfDB(t, recorder)
	require.NoError(t, database.GetDB().Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
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
	before := len(recorder.statements)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/items?limit=1000000&period=week", business.ID), nil)
	c.Set("address", business.OwnerAddress)
	handler.GetItemAnalytics(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	limits := sqlLimitValues(recorder.statements[before:])
	require.NotEmpty(t, limits, "expected a LIMIT clause, got %v", recorder.statements[before:])
	for _, n := range limits {
		assert.LessOrEqual(t, n, 100)
	}
}

var sqlLimitPattern = regexp.MustCompile(`(?i)\blimit\s+(\d+)`)

func sqlLimitValues(statements []string) []int {
	var out []int
	for _, statement := range statements {
		for _, match := range sqlLimitPattern.FindAllStringSubmatch(statement, -1) {
			n, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}
			out = append(out, n)
		}
	}
	return out
}

func BenchmarkAnalyticsLiveBillsSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	business, _ := setupAnalyticsLiveBillsPerfDB(b, logger.Default.LogMode(logger.Silent))
	handler := NewAnalyticsHandler(database.GetDBWrapper())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := performAnalyticsLiveBillsRequest(b, handler, business)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}

// Live-bills cards must be able to label tableless bills: table_id = 0 is the
// "no table" sentinel (delivery/counter), and the frontend distinguishes the
// two via counter_id. Without these discriminators the dashboard rendered a
// blank name and a dangling "$" for delivery bills.
func TestAnalyticsLiveBillsCarriesTableAndCounterDiscriminators(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := setupAnalyticsLiveBillsPerfDB(t, nil)

	counterID := uint(7)
	gormDB := database.GetDB()
	require.NoError(t, gormDB.Create(&database.Bill{
		BusinessID: business.ID,
		TableID:    0,
		CounterID:  &counterID,
		BillNumber: "LIVE-COUNTER-1",
		Status:     database.BillStatusOpen,
	}).Error)
	require.NoError(t, gormDB.Create(&database.Bill{
		BusinessID: business.ID,
		TableID:    0,
		BillNumber: "LIVE-DELIVERY-1",
		Status:     database.BillStatusOpen,
	}).Error)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	w := performAnalyticsLiveBillsRequest(t, handler, business)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"table_id":0`, "tableless bills must expose the table_id sentinel")
	assert.Contains(t, body, `"counter_id":7`, "counter bills must expose counter_id so the UI can label them")
	assert.Contains(t, body, `"counter_id":null`, "delivery bills must expose a null counter_id")
}
