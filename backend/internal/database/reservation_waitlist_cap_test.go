package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWaitlistCapTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&TableReservation{}))
	return db
}

func TestGetNextWaitlistPositionTx_CapsAtMaxPerSlot(t *testing.T) {
	db := setupWaitlistCapTestDB(t)
	slot := time.Date(2026, 8, 15, 19, 0, 0, 0, time.UTC)
	const businessID uint = 9

	seed := func(n int, at time.Time, biz uint) {
		t.Helper()
		for i := 1; i <= n; i++ {
			pos := i
			require.NoError(t, db.Create(&TableReservation{
				BusinessID:       biz,
				CustomerName:     fmt.Sprintf("Guest %d-%d", biz, i),
				PartySize:        2,
				ReservationTime:  at,
				Duration:         90,
				Status:           "waitlist",
				ConfirmationCode: fmt.Sprintf("WL-%d-%d-%d", biz, at.Unix(), i),
				WaitlistPosition: &pos,
			}).Error)
		}
	}

	// A different slot and a different business must not consume this cap.
	seed(3, slot.Add(time.Hour), businessID)
	seed(3, slot, businessID+1)
	seed(199, slot, businessID)

	var got int
	err := db.Transaction(func(tx *gorm.DB) error {
		var txErr error
		got, txErr = GetNextWaitlistPositionTx(tx, businessID, slot)
		return txErr
	})
	require.NoError(t, err)
	require.Equal(t, 200, got)

	pos := 200
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       businessID,
		CustomerName:     "Guest 200",
		PartySize:        2,
		ReservationTime:  slot,
		Duration:         90,
		Status:           "waitlist",
		ConfirmationCode: "WL-200",
		WaitlistPosition: &pos,
	}).Error)

	err = db.Transaction(func(tx *gorm.DB) error {
		_, txErr := GetNextWaitlistPositionTx(tx, businessID, slot)
		return txErr
	})
	require.ErrorIs(t, err, ErrWaitlistFull)
}
