package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// UpdateReservationStatusGuardedTx must refuse to write when the row's status in
// the DB no longer matches the status the transition was planned against — the
// lost-update the auto-decline sweeper vs. operator-approve race would otherwise
// cause. It must succeed when the status is unchanged.
func TestUpdateReservationStatusGuarded(t *testing.T) {
	setupReservationPerfTestDB(t, nil)
	biz := helperReservationPerfBusiness(t)

	res := &TableReservation{
		BusinessID:       biz.ID,
		CustomerName:     "Race Guest",
		CustomerEmail:    "race@example.com",
		PartySize:        2,
		ReservationTime:  time.Now().Add(3 * time.Hour),
		Duration:         60,
		Status:           "pending",
		Source:           "staff",
		ConfirmationCode: "GUARD-1",
	}
	require.NoError(t, db.Create(res).Error)

	// Simulate the sweeper flipping the row to cancelled AFTER the caller read it
	// as pending: the guard (expecting "pending") must abort.
	require.NoError(t, db.Model(&TableReservation{}).Where("id = ?", res.ID).
		Update("status", "cancelled").Error)

	res.Status = "confirmed"
	res.ConfirmedAt = ptrTime(time.Now())
	err := db.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationStatusGuardedTx(tx, res, "pending", 0, ReservationWriteGuards{})
	})
	require.ErrorIs(t, err, ErrReservationStatusConflict, "must not clobber a concurrently-changed status")

	var reloaded TableReservation
	require.NoError(t, db.First(&reloaded, res.ID).Error)
	require.Equal(t, "cancelled", reloaded.Status, "swept status must survive; the racing confirm is rejected")

	// Happy path: when the status is still what we planned against, the write lands.
	res2 := &TableReservation{
		BusinessID:       biz.ID,
		CustomerName:     "Clean Guest",
		CustomerEmail:    "clean@example.com",
		PartySize:        4,
		ReservationTime:  time.Now().Add(4 * time.Hour),
		Duration:         60,
		Status:           "pending",
		Source:           "staff",
		ConfirmationCode: "GUARD-2",
	}
	require.NoError(t, db.Create(res2).Error)
	res2.Status = "confirmed"
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationStatusGuardedTx(tx, res2, "pending", 0, ReservationWriteGuards{})
	}))

	var reloaded2 TableReservation
	require.NoError(t, db.First(&reloaded2, res2.ID).Error)
	require.Equal(t, "confirmed", reloaded2.Status, "uncontended transition must persist")
}

func ptrTime(t time.Time) *time.Time { return &t }
