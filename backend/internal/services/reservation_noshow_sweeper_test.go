package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestNoShowSweeperAgesPastGraceConfirmedOnly(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "noshow-sweep")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.NoShowGraceMinutes = 15
	})

	now := time.Now().UTC()
	expired := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", now.Add(-4*time.Hour), now.Add(-28*time.Hour))
	late := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", now.Add(-5*time.Minute), now.Add(-2*time.Hour))
	future := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", now.Add(2*time.Hour), now.Add(-1*time.Hour))
	pending := seedApprovalSweeperReservation(t, db, business.ID, "pending", now.Add(-4*time.Hour), now.Add(-5*time.Hour))

	sweeper := NewReservationNoShowSweeper()
	aged, err := sweeper.ProcessExpiredConfirmedArrivals()
	require.NoError(t, err)
	require.Equal(t, 1, aged)

	var got database.TableReservation
	require.NoError(t, db.First(&got, expired.ID).Error)
	require.Equal(t, "no_show", got.Status)
	require.Equal(t, "system", got.CancelledBy)
	require.Equal(t, database.ReservationReasonNoShowTimeout, got.CancellationReason)
	require.NotNil(t, got.CancelledAt)

	var history []database.ReservationStatusHistory
	require.NoError(t, db.Where("reservation_id = ?", expired.ID).Find(&history).Error)
	require.Len(t, history, 1)
	require.Equal(t, "no_show", history[0].Status)
	require.Equal(t, "system", history[0].ChangedBy)

	var stillLate database.TableReservation
	require.NoError(t, db.First(&stillLate, late.ID).Error)
	require.Equal(t, "confirmed", stillLate.Status)

	var stillFuture database.TableReservation
	require.NoError(t, db.First(&stillFuture, future.ID).Error)
	require.Equal(t, "confirmed", stillFuture.Status)

	var stillPending database.TableReservation
	require.NoError(t, db.First(&stillPending, pending.ID).Error)
	require.Equal(t, "pending", stillPending.Status)

	agedAgain, err := sweeper.ProcessExpiredConfirmedArrivals()
	require.NoError(t, err)
	require.Equal(t, 0, agedAgain)
}

func TestNoShowSweeperSkipsConfirmedWhenTableHasOpenBill(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "noshow-open-check")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.NoShowGraceMinutes = 15
	})
	table := createTestTable(t, db, business.ID, "T-OPEN", 4)

	now := time.Now().UTC()
	dining := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", now.Add(-4*time.Hour), now.Add(-28*time.Hour))
	require.NoError(t, db.Model(dining).Update("table_id", table.ID).Error)
	tableID := table.ID
	dining.TableID = &tableID

	empty := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", now.Add(-4*time.Hour), now.Add(-28*time.Hour))

	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "B-OPEN-NOSHOW",
		Status:         database.BillStatusOpen,
		TotalAmount:    4200,
		SettlementAddr: "0xsettlement",
		TippingAddr:    "0xtipping",
	}).Error)

	sweeper := NewReservationNoShowSweeper()
	aged, err := sweeper.ProcessExpiredConfirmedArrivals()
	require.NoError(t, err)
	require.Equal(t, 1, aged)

	var stillDining database.TableReservation
	require.NoError(t, db.First(&stillDining, dining.ID).Error)
	require.Equal(t, "confirmed", stillDining.Status)

	var freed database.TableReservation
	require.NoError(t, db.First(&freed, empty.ID).Error)
	require.Equal(t, "no_show", freed.Status)
}

func TestShouldEmailAutoNoShowSkipsStaleSlots(t *testing.T) {
	now := time.Now().UTC()
	stale := &database.TableReservation{
		CustomerEmail:    "guest@example.com",
		ReservationTime:  now.Add(-5 * time.Hour),
		ConfirmationCode: fmt.Sprintf("STALE%d", now.UnixNano()),
	}
	require.False(t, shouldEmailAutoNoShow(stale, now, 15))

	recent := &database.TableReservation{
		CustomerEmail:   "guest@example.com",
		ReservationTime: now.Add(-20 * time.Minute),
	}
	require.True(t, shouldEmailAutoNoShow(recent, now, 15))

	noEmail := &database.TableReservation{
		ReservationTime: now.Add(-20 * time.Minute),
	}
	require.False(t, shouldEmailAutoNoShow(noEmail, now, 15))
}
