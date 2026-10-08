package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type reservationSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *reservationSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *reservationSQLRecorder) selectStarCount(table string) int {
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

func setupReservationPerfTestDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db = gormDB
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &TableReservation{}))
	return gormDB
}

func helperReservationPerfBusiness(t testing.TB) *Business {
	t.Helper()

	business := &Business{
		Name: "Reservation Perf Resto",
		BusinessId: fmt.Sprintf(
			"reservation-perf-%d",
			time.Now().UnixNano(),
		),
		OwnerAddress: fmt.Sprintf("0xreservationperf%x", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

func createTestReservationRecord(t *testing.T, businessID uint, reservationTime time.Time, status string, partySize int) {
	t.Helper()

	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       businessID,
		CustomerName:     fmt.Sprintf("Guest %s", status),
		CustomerEmail:    fmt.Sprintf("%s@example.com", status),
		PartySize:        partySize,
		ReservationTime:  reservationTime,
		Duration:         60,
		Status:           status,
		Source:           "staff",
		ConfirmationCode: fmt.Sprintf("CONF-%d-%s", reservationTime.UnixNano(), status),
	}).Error)
}

func TestGetReservationsByBusinessIDPaginatedDefaultsAndTotals(t *testing.T) {
	setupOrderTestDB(t)
	require.NoError(t, db.AutoMigrate(&TableReservation{}))
	biz := helperBusiness(t, 0, 0)
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)

	for i := 0; i < 25; i++ {
		createTestReservationRecord(t, biz.ID, start.Add(time.Duration(i)*time.Minute), "confirmed", 2)
	}

	result, err := GetReservationsByBusinessIDPaginatedWithSearch(biz.ID, start, start.Add(24*time.Hour), "", "", PaginationParams{})
	require.NoError(t, err)
	assert.Len(t, result.Data, 20)
	assert.Equal(t, int64(25), result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 20, result.PageSize)
	assert.Equal(t, 2, result.TotalPages)
}

func TestGetReservationStatsUsesSingleAggregateQuery(t *testing.T) {
	setupOrderTestDB(t)
	require.NoError(t, db.AutoMigrate(&TableReservation{}))
	biz := helperBusiness(t, 0, 0)
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)

	createTestReservationRecord(t, biz.ID, start.Add(1*time.Hour), "pending", 2)
	createTestReservationRecord(t, biz.ID, start.Add(2*time.Hour), "confirmed", 4)
	createTestReservationRecord(t, biz.ID, start.Add(3*time.Hour), "confirmed", 3)
	createTestReservationRecord(t, biz.ID, start.Add(4*time.Hour), "cancelled", 2)
	createTestReservationRecord(t, biz.ID, start.Add(5*time.Hour), "no_show", 4)

	// A table-assigned confirmed booking: counts toward covers, but must NOT
	// count as needs_table.
	table := &Table{BusinessID: biz.ID, TableCode: "STATS-T1", Name: "Stats Table", Capacity: 6, IsActive: true}
	require.NoError(t, db.Create(table).Error)
	tableID := table.ID
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       biz.ID,
		TableID:          &tableID,
		CustomerName:     "Guest seated-with-table",
		CustomerEmail:    "with-table@example.com",
		PartySize:        6,
		ReservationTime:  start.Add(6 * time.Hour),
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "CONF-STATS-TABLE",
	}).Error)

	queryCount := 0
	callbackName := "payverge:test_reservation_stats_query_counter"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queryCount++
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	stats, err := GetReservationStats(biz.ID, start, start.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(6), stats["total"])
	assert.Equal(t, int64(1), stats["pending"])
	assert.Equal(t, int64(3), stats["confirmed"])
	assert.Equal(t, int64(1), stats["cancelled"])
	assert.Equal(t, int64(1), stats["no_show"])
	// Covers = SUM(party_size) over non-cancelled/non-no-show rows:
	// 2 + 4 + 3 + 6 = 15 (guests, not bookings — the KPI label says "Covers").
	assert.Equal(t, int64(15), stats["covers"])
	// Needs-table = active bookings (pending/confirmed/waitlist) without a
	// table: pending(2) + confirmed(4) + confirmed(3) = 3 rows. The
	// table-assigned confirmed row is excluded.
	assert.Equal(t, int64(3), stats["needs_table"])
	assert.LessOrEqual(t, queryCount, 1)
}

// The stats aggregate must not fail (NULL SUM scan) on a window with zero rows.
func TestGetReservationStatsEmptyWindow(t *testing.T) {
	setupOrderTestDB(t)
	require.NoError(t, db.AutoMigrate(&TableReservation{}))
	biz := helperBusiness(t, 0, 0)
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)

	stats, err := GetReservationStats(biz.ID, start, start.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats["total"])
	assert.Equal(t, int64(0), stats["covers"])
	assert.Equal(t, int64(0), stats["needs_table"])
}

func TestGetReservationsByBusinessIDPaginatedPreloadsTableSummaryOnly(t *testing.T) {
	recorder := &reservationSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationPerfTestDB(t, recorder)

	biz := helperReservationPerfBusiness(t)
	table := &Table{
		BusinessID: biz.ID,
		TableCode:  "RES-T01",
		Name:       "Window Table",
		Capacity:   4,
		QRCode:     strings.Repeat("qr-payload", 256),
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	tableID := table.ID
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       biz.ID,
		TableID:          &tableID,
		CustomerName:     "Guest",
		CustomerEmail:    "guest@example.com",
		PartySize:        2,
		ReservationTime:  start.Add(time.Hour),
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "CONF-TABLE-SUMMARY",
	}).Error)

	result, err := GetReservationsByBusinessIDPaginatedWithSearch(biz.ID, start, start.Add(24*time.Hour), "", "", PaginationParams{PageSize: 20})
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	require.NotNil(t, result.Data[0].Table)
	assert.Equal(t, "Window Table", result.Data[0].Table.Name)
	assert.Equal(t, "RES-T01", result.Data[0].Table.TableCode)
	assert.Equal(t, 4, result.Data[0].Table.Capacity)
	assert.Empty(t, result.Data[0].Table.QRCode, "reservation list should not hydrate QR payloads from table rows")
	assert.Zero(t, recorder.selectStarCount("tables"), "reservation list should project the table summary instead of SELECT *")
}

func BenchmarkGetReservationsByBusinessIDPaginatedSQLite(b *testing.B) {
	setupReservationPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	biz := helperReservationPerfBusiness(b)
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	largeQRPayload := strings.Repeat("qr-payload", 512)

	for i := 0; i < 500; i++ {
		table := &Table{
			BusinessID: biz.ID,
			TableCode:  fmt.Sprintf("RES-BENCH-%03d", i),
			Name:       fmt.Sprintf("Reservation Bench Table %03d", i),
			Capacity:   2 + (i % 6),
			QRCode:     largeQRPayload,
			IsActive:   true,
		}
		require.NoError(b, db.Create(table).Error)
		tableID := table.ID
		require.NoError(b, db.Create(&TableReservation{
			BusinessID:       biz.ID,
			TableID:          &tableID,
			CustomerName:     fmt.Sprintf("Benchmark Guest %03d", i),
			CustomerEmail:    fmt.Sprintf("bench-%03d@example.com", i),
			CustomerPhone:    "+15550000000",
			PartySize:        2 + (i % 4),
			ReservationTime:  start.Add(time.Duration(i) * time.Minute),
			Duration:         90,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("CONF-RES-BENCH-%03d", i),
			SpecialRequests:  strings.Repeat("quiet corner ", 24),
			Notes:            strings.Repeat("internal note ", 24),
		}).Error)
	}

	params := PaginationParams{Page: 1, PageSize: 100}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := GetReservationsByBusinessIDPaginatedWithSearch(biz.ID, start, start.Add(24*time.Hour), "", "", params)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Data) != 100 {
			b.Fatalf("expected 100 reservations, got %d", len(result.Data))
		}
	}
}

// BenchmarkGetReservationStatsSQLite guards the stats aggregate used by the
// dashboard KPI cards + overview today-card (single-scan SUM CASE query).
func BenchmarkGetReservationStatsSQLite(b *testing.B) {
	setupReservationPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	biz := helperReservationPerfBusiness(b)
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	statuses := []string{"pending", "confirmed", "waitlist", "seated", "completed", "cancelled", "no_show"}

	for i := 0; i < 2000; i++ {
		require.NoError(b, db.Create(&TableReservation{
			BusinessID:       biz.ID,
			CustomerName:     fmt.Sprintf("Stats Guest %04d", i),
			CustomerEmail:    fmt.Sprintf("stats-%04d@example.com", i),
			PartySize:        2 + (i % 5),
			ReservationTime:  start.Add(time.Duration(i) * time.Minute),
			Duration:         90,
			Status:           statuses[i%len(statuses)],
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("CONF-STATS-%04d", i),
		}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats, err := GetReservationStats(biz.ID, start, start.Add(48*time.Hour))
		if err != nil {
			b.Fatal(err)
		}
		if stats["total"].(int64) != 2000 {
			b.Fatalf("expected 2000 total, got %v", stats["total"])
		}
	}
}

// BenchmarkGetReservationsByBusinessIDPaginatedSearchSQLite measures the
// optional customer-field search path (q param) under the same 500-row seed.
func BenchmarkGetReservationsByBusinessIDPaginatedSearchSQLite(b *testing.B) {
	setupReservationPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	biz := helperReservationPerfBusiness(b)
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	largeQRPayload := strings.Repeat("qr-payload", 512)

	for i := 0; i < 500; i++ {
		table := &Table{
			BusinessID: biz.ID,
			TableCode:  fmt.Sprintf("RES-SRCH-%03d", i),
			Name:       fmt.Sprintf("Reservation Search Table %03d", i),
			Capacity:   2 + (i % 6),
			QRCode:     largeQRPayload,
			IsActive:   true,
		}
		require.NoError(b, db.Create(table).Error)
		tableID := table.ID
		require.NoError(b, db.Create(&TableReservation{
			BusinessID:       biz.ID,
			TableID:          &tableID,
			CustomerName:     fmt.Sprintf("Benchmark Guest %03d", i),
			CustomerEmail:    fmt.Sprintf("bench-%03d@example.com", i),
			CustomerPhone:    fmt.Sprintf("+1555000%04d", i),
			PartySize:        2 + (i % 4),
			ReservationTime:  start.Add(time.Duration(i) * time.Minute),
			Duration:         90,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("CONF-RES-SRCH-%03d", i),
			SpecialRequests:  strings.Repeat("quiet corner ", 24),
			Notes:            strings.Repeat("internal note ", 24),
		}).Error)
	}

	params := PaginationParams{Page: 1, PageSize: 100}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, start.Add(24*time.Hour), "", "Guest 00", params,
		)
		if err != nil {
			b.Fatal(err)
		}
		if result.Total < 1 {
			b.Fatalf("expected search hits, got total=%d", result.Total)
		}
	}
}
