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

func TestReservationHoldCutoffIsUTC(t *testing.T) {
	loc := time.FixedZone("UTC+4", 4*3600)
	now := time.Date(2026, 8, 20, 7, 32, 0, 0, loc)
	cutoff := reservationHoldCutoff(^uint(0), now)
	require.Equal(t, time.UTC, cutoff.Location())
	require.True(t, cutoff.Equal(time.Date(2026, 8, 20, 3, 17, 0, 0, time.UTC)))
}

func setupHostStandTableStatusDB(t testing.TB) uint {
	t.Helper()
	dsn := fmt.Sprintf("file:host-stand-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &TableReservation{}, &Staff{}, &Order{}))
	require.NoError(t, db.Exec(`CREATE TABLE bill_items (
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
	)`).Error)

	business := &Business{
		BusinessId:     fmt.Sprintf("host-stand-%d", time.Now().UnixNano()),
		Name:           "Host Stand Cafe",
		OwnerAddress:   "0xHostStandOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)
	return business.ID
}

// TestGetTablesWithStatusMarksUpcomingReservationReserved locks the host-stand
// rule: a confirmed booking later tonight must not read as Available.
func TestGetTablesWithStatusMarksUpcomingReservationReserved(t *testing.T) {
	businessID := setupHostStandTableStatusDB(t)

	table := Table{
		BusinessID: businessID,
		Name:       "Table 2",
		TableCode:  "HOST-T02",
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, db.Create(&table).Error)

	tableID := table.ID
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       businessID,
		TableID:          &tableID,
		CustomerName:     "Demo Reservation",
		PartySize:        4,
		ReservationTime:  time.Now().UTC().Add(3 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		ConfirmationCode: "HOST-RES-1",
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "reserved", rows[0]["status"])
	assert.Equal(t, 1, rows[0]["reservations_count"])
}

// TestGetTablesWithStatusProjectsServerName locks Live View server attribution:
// an open bill created by staff must expose active_bill_server_name (and stay occupied).
func TestGetTablesWithStatusProjectsServerName(t *testing.T) {
	businessID := setupHostStandTableStatusDB(t)

	staff := Staff{
		BusinessID: businessID,
		Email:      "server@example.com",
		Name:       "Alex Server",
		Role:       StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner",
	}
	require.NoError(t, db.Create(&staff).Error)

	table := Table{
		BusinessID: businessID,
		Name:       "Table 1",
		TableCode:  "HOST-T01",
		Capacity:   2,
		IsActive:   true,
	}
	require.NoError(t, db.Create(&table).Error)

	staffID := staff.ID
	require.NoError(t, db.Create(&Bill{
		BusinessID:       businessID,
		TableID:          table.ID,
		BillNumber:       "B-HOST-1",
		Status:           BillStatusOpen,
		TotalAmount:      40855,
		CreatedByStaffID: &staffID,
		SettlementAddr:   "0x1111111111111111111111111111111111111111",
		TippingAddr:      "0x2222222222222222222222222222222222222222",
		CreatedAt:        time.Now().Add(-2 * time.Hour),
		UpdatedAt:        time.Now().Add(-4 * time.Minute),
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "occupied", rows[0]["status"])
	assert.Equal(t, "Alex Server", rows[0]["active_bill_server_name"])
}

func TestGetTablesWithStatusLateWithinGraceIsReserved(t *testing.T) {
	businessID := setupHostStandTableStatusDB(t)

	table := Table{
		BusinessID: businessID,
		Name:       "Table 4",
		TableCode:  "HOST-T04",
		Capacity:   6,
		IsActive:   true,
	}
	require.NoError(t, db.Create(&table).Error)

	tableID := table.ID
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       businessID,
		TableID:          &tableID,
		CustomerName:     "Demo Reservation",
		PartySize:        4,
		ReservationTime:  time.Now().UTC().Add(-10 * time.Minute),
		Duration:         120,
		Status:           "confirmed",
		ConfirmationCode: "HOST-RES-LATE",
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "reserved", rows[0]["status"])
	assert.Equal(t, 1, rows[0]["reservations_count"])
}

func TestGetTablesWithStatusPastGraceIsAvailable(t *testing.T) {
	businessID := setupHostStandTableStatusDB(t)

	table := Table{
		BusinessID: businessID,
		Name:       "Table 4",
		TableCode:  "HOST-T04B",
		Capacity:   6,
		IsActive:   true,
	}
	require.NoError(t, db.Create(&table).Error)

	tableID := table.ID
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       businessID,
		TableID:          &tableID,
		CustomerName:     "Demo Reservation",
		PartySize:        4,
		ReservationTime:  time.Now().UTC().Add(-4 * time.Hour),
		Duration:         120,
		Status:           "confirmed",
		ConfirmationCode: "HOST-RES-EXPIRED",
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "available", rows[0]["status"])
	assert.Equal(t, 0, rows[0]["reservations_count"])
}

// #704: ticket 1123 / bill 1132 closed $0 / T5 Available. A terminal check
// that still owns live kitchen work must keep the table occupied.
func TestGetTablesWithStatus_AbandonedBillWithLiveKitchenIsOccupied(t *testing.T) {
	businessID := setupHostStandTableStatusDB(t)

	table := Table{
		BusinessID: businessID,
		Name:       "Table 5",
		TableCode:  "HOST-T05",
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, db.Create(&table).Error)

	bill := Bill{
		BusinessID:  businessID,
		TableID:     table.ID,
		BillNumber:  "1132",
		Status:      BillStatusClosed,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(&bill).Error)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  businessID,
		OrderNumber: "G86-36604192",
		Status:      OrderStatusInKitchen,
		Items:       "[]",
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "occupied", rows[0]["status"],
		"closed $0 check with an in_kitchen ticket must not read as Available")
	require.NotNil(t, rows[0]["seated_at"],
		"occupying unfinished check must project seated_at for Live View")
	require.NotNil(t, rows[0]["last_seen"])
}

// #704 T1 / pending 1129+1125 on abandoned 1139: occupancy that only joins
// kitchen-live statuses leaves the table Available and hides Liberar.
func TestGetTablesWithStatus_AbandonedBillWithPendingOrdersIsOccupied(t *testing.T) {
	businessID := setupHostStandTableStatusDB(t)

	table := Table{
		BusinessID: businessID,
		Name:       "Table 1",
		TableCode:  "HOST-T01-PEND",
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, db.Create(&table).Error)

	bill := Bill{
		BusinessID:  businessID,
		TableID:     table.ID,
		BillNumber:  "1139",
		Status:      BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(&bill).Error)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  businessID,
		OrderNumber: "G86-1129",
		Status:      OrderStatusPending,
		Items:       "[]",
	}).Error)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  businessID,
		OrderNumber: "G86-1125",
		Status:      OrderStatusPending,
		Items:       "[]",
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "occupied", rows[0]["status"],
		"abandoned check with pending guest sends must not read as Available")
}
