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

func setupTableFloorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:table-floor-%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	prev := db
	SetTestDB(gdb)
	t.Cleanup(func() { SetTestDB(prev) })

	require.NoError(t, gdb.AutoMigrate(
		&Business{},
		&Table{},
		&Bill{},
		&BillHistoryEvent{},
		&TableReservation{},
		&Order{},
	))
	require.NoError(t, gdb.Exec(`
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
	require.NoError(t, gdb.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_bills_active_per_table ON bills (table_id) `+
			`WHERE table_id IS NOT NULL AND table_id <> 0 AND status IN ('open','partial')`,
	).Error)
	return gdb
}

func seedFloorBizAndTables(t *testing.T) (*Business, *Table, *Table) {
	t.Helper()
	biz := &Business{
		BusinessId:      "floor-" + t.Name(),
		Name:            "Floor Biz",
		OwnerAddress:    "0xfloor",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		TaxRate:         10,
		ServiceFeeRate:  0,
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, db.Create(biz).Error)

	t1 := &Table{BusinessID: biz.ID, TableCode: "T1-" + t.Name(), Name: "Table 1", Capacity: 4, IsActive: true}
	t2 := &Table{BusinessID: biz.ID, TableCode: "T2-" + t.Name(), Name: "Table 2", Capacity: 4, IsActive: true}
	require.NoError(t, db.Create(t1).Error)
	require.NoError(t, db.Create(t2).Error)
	return biz, t1, t2
}

func TestSeatWalkIn_OpensEmptyCheck(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, table, _ := seedFloorBizAndTables(t)

	bill, err := SeatWalkIn(SeatWalkInInput{
		BusinessID: biz.ID,
		TableID:    table.ID,
		PartySize:  3,
		Actor:      "host:ana",
	})
	require.NoError(t, err)
	require.NotNil(t, bill)
	assert.Equal(t, BillStatusOpen, bill.Status)
	assert.Equal(t, table.ID, bill.TableID)
	assert.Equal(t, int64(0), bill.TotalAmount)
	assert.Contains(t, bill.Notes, "3 covers")

	_, err = SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host:ana"})
	require.ErrorIs(t, err, ErrFloorTableOccupied)
}

func TestClearTable_EmptyOpenOnly(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, table, _ := seedFloorBizAndTables(t)

	bill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.NoError(t, err)

	cleared, err := ClearTable(ClearTableInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.NoError(t, err)
	require.Equal(t, BillStatusVoided, cleared.Status)
	assert.Nil(t, cleared.SettledAt)
	require.NotNil(t, cleared.ClosedAt)
	var clearEvent BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ? AND event_type = ?", cleared.ID, BillHistoryEventBillVoided).First(&clearEvent).Error)
	assert.Equal(t, "Cleared from Live View", clearEvent.Reason)

	// Re-seat then put money on the check — clear must refuse.
	bill2, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.NoError(t, err)
	require.NoError(t, db.Model(bill2).Updates(map[string]interface{}{
		"total_amount": int64(1500),
		"subtotal":     int64(1500),
		"items":        `[{"id":"x","name":"Steak","price":15,"quantity":1,"subtotal":15}]`,
	}).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, created_at) VALUES (?,?,?,?,?,?,?)`,
		"item-1", bill2.ID, "Steak", 15.0, 1, 15.0, time.Now(),
	).Error)

	_, err = ClearTable(ClearTableInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.ErrorIs(t, err, ErrFloorSettleRequired)
	_ = bill
}

func TestTransferActiveBill_MovesOpenCheck(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, source, target := seedFloorBizAndTables(t)

	bill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)

	moved, err := TransferActiveBill(TransferBillInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)
	assert.Equal(t, target.ID, moved.TableID)
	assert.Equal(t, bill.ID, moved.ID)

	_, err = loadActiveBillForTableTx(db, source.ID)
	require.ErrorIs(t, err, ErrNoActiveBill)
	active, err := loadActiveBillForTableTx(db, target.ID)
	require.NoError(t, err)
	assert.Equal(t, bill.ID, active.ID)
}

func TestMergeTableChecks_MovesItemsAndFreesSource(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, source, target := seedFloorBizAndTables(t)

	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)
	tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
	require.NoError(t, err)

	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"src-item", srcBill.ID, "Empanada", 5.0, 2, 10.0, "menu_item", time.Now(),
	).Error)
	require.NoError(t, db.Model(srcBill).Updates(map[string]interface{}{
		"subtotal": int64(1000), "total_amount": int64(1100), "tax_amount": int64(100),
		"items": `[{"id":"src-item","name":"Empanada","price":5,"quantity":2,"subtotal":10,"item_type":"menu_item"}]`,
	}).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"tgt-item", tgtBill.ID, "Wine", 12.0, 1, 12.0, "menu_item", time.Now(),
	).Error)
	require.NoError(t, db.Model(tgtBill).Updates(map[string]interface{}{
		"subtotal": int64(1200), "total_amount": int64(1320), "tax_amount": int64(120),
		"items": `[{"id":"tgt-item","name":"Wine","price":12,"quantity":1,"subtotal":12,"item_type":"menu_item"}]`,
	}).Error)

	mergedTarget, mergedSource, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)
	require.NotNil(t, mergedTarget)
	require.NotNil(t, mergedSource)
	assert.Equal(t, BillStatusVoided, mergedSource.Status)
	assert.Nil(t, mergedSource.SettledAt)
	require.NotNil(t, mergedSource.ClosedAt)
	assert.Equal(t, BillStatusOpen, mergedTarget.Status)
	var sourceEvent BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ? AND event_type = ?", mergedSource.ID, BillHistoryEventBillVoided).First(&sourceEvent).Error)
	assert.Equal(t, "Merged into another table from Live View", sourceEvent.Reason)
	assert.Equal(t, int64(2200), mergedTarget.Subtotal) // 10 + 12 dollars → cents
	assert.Equal(t, int64(2420), mergedTarget.TotalAmount)

	_, err = loadActiveBillForTableTx(db, source.ID)
	require.ErrorIs(t, err, ErrNoActiveBill)
}

func TestMergeTableChecks_TargetEmptyTransfers(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, source, target := seedFloorBizAndTables(t)
	_, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)

	targetBill, sourceBill, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)
	require.NotNil(t, targetBill)
	assert.Nil(t, sourceBill)
	assert.Equal(t, target.ID, targetBill.TableID)
}
