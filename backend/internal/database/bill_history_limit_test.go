package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBillHistoryDB(t testing.TB, events int) uint {
	t.Helper()
	dsn := fmt.Sprintf("file:bill-hist-%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&BillHistoryEvent{}))

	now := time.Now().UTC()
	for i := 0; i < events; i++ {
		require.NoError(t, db.Create(&BillHistoryEvent{
			BillID:    1,
			EventType: "bill.updated",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error)
	}
	return 1
}

// TestGetBillHistoryLimitedReturnsNewestPlusTotal proves the modal can page:
// with a limit it returns only the newest N events but the honest total count.
func TestGetBillHistoryLimitedReturnsNewestPlusTotal(t *testing.T) {
	billID := setupBillHistoryDB(t, 120)

	events, total, err := GetBillHistoryByBillIDLimited(billID, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 120, total, "total must reflect every event, not the page size")
	assert.Len(t, events, 50, "must cap at the requested limit")

	// Newest-first ordering: each event is 1s newer than the previous insert,
	// so the first row is the most recent created_at.
	for i := 1; i < len(events); i++ {
		assert.Falsef(
			t,
			events[i].CreatedAt.After(events[i-1].CreatedAt),
			"history must be newest-first; row %d is newer than row %d",
			i, i-1,
		)
	}
}

// TestGetBillHistoryLimitedZeroIsUnbounded locks the legacy shape: limit<=0
// returns every event (what the handler does when no history_limit is sent).
func TestGetBillHistoryLimitedZeroIsUnbounded(t *testing.T) {
	billID := setupBillHistoryDB(t, 30)
	events, total, err := GetBillHistoryByBillIDLimited(billID, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 30, total)
	assert.Len(t, events, 30)
}

func BenchmarkGetBillHistoryLimitedSQLite(b *testing.B) {
	billID := setupBillHistoryDB(b, 500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, _, err := GetBillHistoryByBillIDLimited(billID, 50)
		if err != nil {
			b.Fatal(err)
		}
		if len(events) != 50 {
			b.Fatalf("expected 50 events, got %d", len(events))
		}
	}
}
