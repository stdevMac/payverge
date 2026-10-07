package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestServiceCallStatusQueryShape is the perf-gate access-shape regression
// test for the public guest service-call poll: the status read must be a
// narrow projection (status, resolved_at, metadata) with LIMIT 1 — never
// SELECT * over operational_alerts — and the table lookup must stay on the
// projected public-columns join, not a full-row hydration. Metadata is
// included so the guest UI can echo the last enum reason (#82).
func TestServiceCallStatusQueryShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: recorder})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))

	business := createOwnedBusiness(t, "0xOwnerSCShape", "Service Call Shape Biz")
	table := createServiceCallTable(t, business.ID, "TS", "svc-call-shape")

	// Force the table lookup through the DB (not the pricing cache) so its
	// query shape is captured too.
	services.ResetPricingCache()
	recorder.statements = nil

	w := getServiceCallStatus(t, table.TableCode)
	require.Equal(t, http.StatusOK, w.Code)

	var alertSelects, tableSelects []string
	for _, stmt := range recorder.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(stmt), " "))
		if !strings.HasPrefix(normalized, "select") {
			continue
		}
		if strings.Contains(normalized, "operational_alerts") {
			alertSelects = append(alertSelects, normalized)
		}
		if strings.Contains(normalized, "from `tables`") || strings.Contains(normalized, "from \"tables\"") {
			tableSelects = append(tableSelects, normalized)
		}
	}

	require.Len(t, alertSelects, 1, "status poll must issue exactly one operational_alerts read")
	statusQuery := alertSelects[0]
	assert.True(t, strings.Contains(statusQuery, "status") && strings.Contains(statusQuery, "resolved_at"),
		"status query must project status/resolved_at, got: %s", statusQuery)
	assert.Contains(t, statusQuery, "last_event_at", "status query must include last_event_at for the seating SLA (#729)")
	assert.Contains(t, statusQuery, "metadata", "status query must include metadata for the guest reason echo (#82)")
	assert.NotContains(t, statusQuery, "select *", "status query must not hydrate full alert rows")
	assert.NotContains(t, statusQuery, "title", "status query must not pull staff-facing title")
	assert.NotContains(t, statusQuery, "body", "status query must not pull staff-facing body")
	assert.Contains(t, statusQuery, "limit 1", "status query must be bounded to the newest row")

	require.Len(t, tableSelects, 1, "table-code resolution must be a single query")
	assert.NotContains(t, tableSelects[0], "select *", "table lookup must stay a narrow projection")
	assert.Contains(t, tableSelects[0], "business", "table lookup must join the projected business columns in one query")
}

// BenchmarkGuestServiceCallStatus exercises the guest poll endpoint
// end-to-end (route → table resolve → narrow status read) against sqlite.
func BenchmarkGuestServiceCallStatus(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	gormDB := setupBenchmarkStaffHandlerDB(b)
	gormDB.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(b, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))

	business := &database.Business{
		BusinessId:     "biz-svc-call-bench",
		Name:           "Service Call Bench Biz",
		OwnerAddress:   "0xOwnerSCBench",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(b, database.GetDB().Create(business).Error)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "TB",
		TableCode:  strings.ToUpper("svc-call-bench"),
		IsActive:   true,
	}
	require.NoError(b, database.GetDB().Create(table).Error)

	services.ResetPricingCache()

	router := gin.New()
	router.GET("/api/v1/guest/table/:code/service-call", GetServiceCallStatusByTableCode)
	target := "/api/v1/guest/table/" + table.TableCode + "/service-call"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
		}
	}
}
