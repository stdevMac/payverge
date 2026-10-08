package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupReservationOccupancyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Table{}, &Bill{}, &TableReservation{}))
	return db
}

func TestCreateReservationTxRechecksActiveBillAfterRecommendation(t *testing.T) {
	db := setupReservationOccupancyTestDB(t)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	const businessID uint = 72
	table := Table{BusinessID: businessID, TableCode: "occupancy-race", Name: "Race", Capacity: 4, IsActive: true}
	require.NoError(t, db.Create(&table).Error)
	require.NoError(t, db.Create(&Bill{
		BusinessID: businessID, TableID: table.ID, BillNumber: "late-active-bill",
		Status: BillStatusOpen, SettlementAddr: "settlement", TippingAddr: "tipping",
		CreatedAt: now.Add(-time.Hour),
	}).Error)

	tableID := table.ID
	reservation := &TableReservation{
		BusinessID: businessID, TableID: &tableID, CustomerName: "Race Guest",
		PartySize: 2, ReservationTime: now.Add(time.Hour), Duration: 120,
		Status: "confirmed", ConfirmationCode: "OCC-RACE",
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		return CreateReservationTx(tx, reservation, 15, ReservationWriteGuards{
			OccupancyCheckAt: &now, StaleAfter: 12 * time.Hour,
		})
	})
	require.ErrorIs(t, err, ErrReservationTableOccupied)
	var count int64
	require.NoError(t, db.Model(&TableReservation{}).Where("business_id = ?", businessID).Count(&count).Error)
	require.Zero(t, count)
}

func TestGetReservationTableOccupancySnapshot(t *testing.T) {
	db := setupReservationOccupancyTestDB(t)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	const businessID uint = 41

	createTable := func(name string) Table {
		table := Table{BusinessID: businessID, TableCode: "code-" + name, Name: name, Capacity: 4, IsActive: true}
		require.NoError(t, db.Create(&table).Error)
		return table
	}
	live := createTable("Live")
	stale := createTable("Stale")
	free := createTable("Free")
	otherBusiness := createTable("Other")
	otherBusiness.BusinessID = businessID + 1
	require.NoError(t, db.Save(&otherBusiness).Error)

	createBill := func(table Table, openedAt time.Time) {
		bill := Bill{
			BusinessID:     table.BusinessID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("bill-%d", table.ID),
			Status:         BillStatusOpen,
			SettlementAddr: "settlement",
			TippingAddr:    "tipping",
			CreatedAt:      openedAt,
		}
		require.NoError(t, db.Create(&bill).Error)
	}
	createBill(live, now.Add(-2*time.Hour))
	createBill(stale, now.Add(-13*time.Hour))
	createBill(otherBusiness, now.Add(-24*time.Hour))

	got, err := GetReservationTableOccupancySnapshot(
		db, businessID, []uint{live.ID, stale.ID, free.ID, otherBusiness.ID}, now, 12*time.Hour,
	)
	require.NoError(t, err)
	require.Equal(t, TableOccupancyOccupied, got[live.ID].State)
	require.Equal(t, TableOccupancyStaleOccupied, got[stale.ID].State)
	require.Equal(t, TableOccupancyAvailable, got[free.ID].State)
	require.Equal(t, TableOccupancyAvailable, got[otherBusiness.ID].State)
	require.NotNil(t, got[live.ID].ActiveBillID)
	require.Equal(t, 2*time.Hour, got[live.ID].ActiveBillAge)
}
