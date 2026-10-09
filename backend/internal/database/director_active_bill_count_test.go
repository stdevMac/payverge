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

// setupActiveBillCountDB opens an isolated in-memory SQLite DB, migrates Bill
// and Business, seeds a mix of active and non-active bills, and wires the DB
// wrapper used by the Director service. It returns the wrapped *DB plus the
// businessID.
func setupActiveBillCountDB(t testing.TB, gormLogger logger.Interface) (wrapped *DB, businessID uint) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err, "open in-memory SQLite")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// Wire both the package-level 'db' (for standalone helpers) and the wrapped
	// *DB struct (used by director_console_service via s.db.GetActive*).
	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Bill{}))

	biz := &Business{
		BusinessId:      fmt.Sprintf("active-bill-count-biz-%d", time.Now().UnixNano()),
		Name:            "Active Bill Count Test",
		OwnerAddress:    "0xActiveBillCountOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, gormDB.Create(biz).Error)

	// Seed 3 active bills (open + partial) and 2 non-active bills (paid + void).
	activeSeed := []Bill{
		{BusinessID: biz.ID, BillNumber: "B-OPEN-1", Status: BillStatusOpen},
		{BusinessID: biz.ID, BillNumber: "B-OPEN-2", Status: BillStatusOpen},
		{BusinessID: biz.ID, BillNumber: "B-PARTIAL-1", Status: BillStatusPartial},
	}
	for i := range activeSeed {
		require.NoError(t, gormDB.Create(&activeSeed[i]).Error)
	}
	nonActiveSeed := []Bill{
		{BusinessID: biz.ID, BillNumber: "B-PAID-1", Status: BillStatusPaid},
		{BusinessID: biz.ID, BillNumber: "B-VOIDED-1", Status: BillStatusVoided},
	}
	for i := range nonActiveSeed {
		require.NoError(t, gormDB.Create(&nonActiveSeed[i]).Error)
	}

	wrapped = &DB{conn: gormDB}
	return wrapped, biz.ID
}

// activeBillCountSQLRecorder captures SQL for assertion.
type activeBillCountSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *activeBillCountSQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *activeBillCountSQLRecorder) reset() { r.statements = nil }

func (r *activeBillCountSQLRecorder) statementContains(sub string) bool {
	needle := strings.ToLower(sub)
	for _, s := range r.statements {
		if strings.Contains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
}

// TestGetActiveBillCountByBusinessID_CountsActiveOnly is the load-bearing
// access-shape guard for OF-05. It verifies:
//  1. The count equals len(GetActiveBillsByBusinessID) for the same seed.
//  2. The generated SQL contains "count" (aggregate), NOT a full-row SELECT.
//  3. Non-active bills (paid/void) are excluded from the count.
//
// The test is RED before GetActiveBillCountByBusinessID exists.
func TestGetActiveBillCountByBusinessID_CountsActiveOnly(t *testing.T) {
	recorder := &activeBillCountSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	wrapped, bizID := setupActiveBillCountDB(t, recorder)

	// Parity check: the full-row loader to compare against.
	rowBills, err := wrapped.GetActiveBillsByBusinessID(bizID)
	require.NoError(t, err)
	expectedCount := int64(len(rowBills))
	require.Equal(t, int64(3), expectedCount, "seed must have 3 active bills (sanity check)")

	recorder.reset() // only capture the query under test

	// OF-05: count path must return the same value as len(GetActiveBillsByBusinessID).
	n, err := wrapped.GetActiveBillCountByBusinessID(bizID)
	require.NoError(t, err)
	assert.Equal(t, expectedCount, n, "GetActiveBillCountByBusinessID must equal len(GetActiveBillsByBusinessID)")

	// SQL-capture: the query must use COUNT, not SELECT *.
	assert.True(t, recorder.statementContains("count"),
		"GetActiveBillCountByBusinessID must issue a COUNT aggregate; got SQL: %v", recorder.statements)
	assert.False(t, recorder.statementContains("select *"),
		"GetActiveBillCountByBusinessID must NOT issue SELECT *; got SQL: %v", recorder.statements)

	// Only one statement should have been emitted (the COUNT query).
	assert.Len(t, recorder.statements, 1,
		"GetActiveBillCountByBusinessID must issue exactly 1 query; got: %v", recorder.statements)
}

// TestGetActiveBillCountByBusinessID_ZeroWhenNone verifies a business with no
// active bills returns 0, not an error.
func TestGetActiveBillCountByBusinessID_ZeroWhenNone(t *testing.T) {
	wrapped, _ := setupActiveBillCountDB(t, logger.Default.LogMode(logger.Silent))

	// Use a non-existent businessID.
	n, err := wrapped.GetActiveBillCountByBusinessID(99999)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "count must be 0 for a business with no active bills")
}

// BenchmarkGetActiveBillsByBusinessID_Before is the OF-05 baseline: full-row
// hydration of GetActiveBillsByBusinessID over 20 active bills.
func BenchmarkGetActiveBillsByBusinessID_Before(b *testing.B) {
	wrapped, bizID := setupActiveBillCountDB(b, logger.Default.LogMode(logger.Silent))

	// Add 17 more bills (3 already seeded → 20 total active).
	for i := 0; i < 17; i++ {
		require.NoError(b, wrapped.conn.Create(&Bill{
			BusinessID: bizID,
			BillNumber: fmt.Sprintf("B-BENCH-OPEN-%d", i),
			Status:     BillStatusOpen,
		}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bills, err := wrapped.GetActiveBillsByBusinessID(bizID)
		if err != nil {
			b.Fatal(err)
		}
		_ = len(bills)
	}
}

// BenchmarkGetActiveBillCountByBusinessID_After is the OF-05 after-benchmark:
// COUNT aggregate over the same 20 active bills.
func BenchmarkGetActiveBillCountByBusinessID_After(b *testing.B) {
	wrapped, bizID := setupActiveBillCountDB(b, logger.Default.LogMode(logger.Silent))

	// Add 17 more bills (3 already seeded → 20 total active).
	for i := 0; i < 17; i++ {
		require.NoError(b, wrapped.conn.Create(&Bill{
			BusinessID: bizID,
			BillNumber: fmt.Sprintf("B-BENCH-COUNT-%d", i),
			Status:     BillStatusOpen,
		}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := wrapped.GetActiveBillCountByBusinessID(bizID)
		if err != nil {
			b.Fatal(err)
		}
		_ = n
	}
}
