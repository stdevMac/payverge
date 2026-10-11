package database

import (
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

// setupOccupiedTablesDB seeds mixed table / counter / delivery open bills for
// Task 16 open-tables counting (distinct occupied tables, not bill count).
func setupOccupiedTablesDB(t testing.TB, gormLogger logger.Interface) (wrapped *DB, businessID uint) {
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

	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Bill{}))

	biz := &Business{
		BusinessId:      fmt.Sprintf("occ-tables-%d", time.Now().UnixNano()),
		Name:            "Occupied Tables Test",
		OwnerAddress:    "0xOccupiedTablesOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, gormDB.Create(biz).Error)

	// Two tables occupied (table 1 has two open bills; table 2 has one).
	// Three counter/delivery bills with table_id=0 must NOT inflate open tables.
	seeds := []Bill{
		{BusinessID: biz.ID, BillNumber: "T1-A", Status: BillStatusOpen, TableID: 1},
		{BusinessID: biz.ID, BillNumber: "T1-B", Status: BillStatusPartial, TableID: 1},
		{BusinessID: biz.ID, BillNumber: "T2-A", Status: BillStatusOpen, TableID: 2},
		{BusinessID: biz.ID, BillNumber: "DELIV-1", Status: BillStatusOpen, TableID: 0},
		{BusinessID: biz.ID, BillNumber: "DELIV-2", Status: BillStatusOpen, TableID: 0},
		{BusinessID: biz.ID, BillNumber: "COUNTER-1", Status: BillStatusOpen, TableID: 0},
		{BusinessID: biz.ID, BillNumber: "PAID-1", Status: BillStatusPaid, TableID: 3},
		{BusinessID: biz.ID, BillNumber: "ABAND-1", Status: BillStatusAbandoned, TableID: 4},
	}
	for i := range seeds {
		require.NoError(t, gormDB.Create(&seeds[i]).Error)
	}

	return &DB{conn: gormDB}, biz.ID
}

// Task 16 (c) access-shape: open tables = distinct table_id > 0 with an active bill.
func TestCountOccupiedTablesByBusinessID_DistinctTablesOnly(t *testing.T) {
	recorder := &activeBillCountSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	wrapped, bizID := setupOccupiedTablesDB(t, recorder)

	recorder.reset()
	n, err := wrapped.CountOccupiedTablesByBusinessID(bizID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "three delivery/counter bills must not inflate open tables")

	assert.True(t, recorder.statementContains("count"),
		"must issue a COUNT aggregate; got SQL: %v", recorder.statements)
	assert.False(t, recorder.statementContains("select *"),
		"must not hydrate full rows; got SQL: %v", recorder.statements)
	assert.Len(t, recorder.statements, 1,
		"exactly one query; got: %v", recorder.statements)
}

func TestCountOccupiedTablesByBusinessID_ZeroWhenNone(t *testing.T) {
	wrapped, _ := setupOccupiedTablesDB(t, logger.Default.LogMode(logger.Silent))
	n, err := wrapped.CountOccupiedTablesByBusinessID(99999)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// Baseline (before): derive open-tables by hydrating active bills and counting
// distinct table IDs in Go — the shape TodayLive was accidentally approximating
// with active_bills (bill count).
func BenchmarkCountOccupiedTables_HydrateThenDistinct(b *testing.B) {
	wrapped, bizID := setupOccupiedTablesDB(b, logger.Default.LogMode(logger.Silent))
	// Pad to a busy floor: 40 table-bound open bills across 20 tables + 30 no-table.
	for i := 0; i < 40; i++ {
		require.NoError(b, wrapped.conn.Create(&Bill{
			BusinessID: bizID,
			BillNumber: fmt.Sprintf("BENCH-T-%d", i),
			Status:     BillStatusOpen,
			TableID:    uint(i%20) + 10,
		}).Error)
	}
	for i := 0; i < 30; i++ {
		require.NoError(b, wrapped.conn.Create(&Bill{
			BusinessID: bizID,
			BillNumber: fmt.Sprintf("BENCH-D-%d", i),
			Status:     BillStatusOpen,
			TableID:    0,
		}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bills, err := wrapped.GetActiveBillsByBusinessID(bizID)
		if err != nil {
			b.Fatal(err)
		}
		seen := make(map[uint]struct{})
		for _, bill := range bills {
			if bill.TableID == 0 {
				continue
			}
			seen[bill.TableID] = struct{}{}
		}
		_ = len(seen)
	}
}

// After: SQL COUNT(DISTINCT table_id) aggregate — polled-route friendly.
func BenchmarkCountOccupiedTablesByBusinessID(b *testing.B) {
	wrapped, bizID := setupOccupiedTablesDB(b, logger.Default.LogMode(logger.Silent))
	for i := 0; i < 40; i++ {
		require.NoError(b, wrapped.conn.Create(&Bill{
			BusinessID: bizID,
			BillNumber: fmt.Sprintf("BENCH2-T-%d", i),
			Status:     BillStatusOpen,
			TableID:    uint(i%20) + 10,
		}).Error)
	}
	for i := 0; i < 30; i++ {
		require.NoError(b, wrapped.conn.Create(&Bill{
			BusinessID: bizID,
			BillNumber: fmt.Sprintf("BENCH2-D-%d", i),
			Status:     BillStatusOpen,
			TableID:    0,
		}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := wrapped.CountOccupiedTablesByBusinessID(bizID)
		if err != nil {
			b.Fatal(err)
		}
		_ = n
	}
}
