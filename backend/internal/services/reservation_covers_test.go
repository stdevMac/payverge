package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestReservationRecheckSlotCapacityTxEnforcesMaxCovers proves the in-transaction
// recheck, not the outside-transaction availability read. A confirmed 3-cover
// booking is inserted directly. The context cache is then filled with an empty
// conflict set that would allow another party of 2; the recheck must ignore
// that cache, refuse party 2 against MaxCoversPerSlot=4, and allow party 1.
func TestReservationRecheckSlotCapacityTxEnforcesMaxCovers(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-covers-recheck")
	createTestTable(t, db, business.ID, "Cover A", 4)
	createTestTable(t, db, business.ID, "Cover B", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.MaxCoversPerSlot = 4
		settings.AutoAssignTables = false
		settings.AllowWaitlist = false
		settings.ServiceBufferMinutes = 0
		settings.DefaultDuration = 90
	})

	slot := nextDayAt(18, 0)
	require.NoError(t, db.Create(&database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Already booked",
		PartySize:        3,
		ReservationTime:  slot.UTC(),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "COVERS-EXISTING",
	}).Error)

	ctx, err := service.loadAvailabilityContext(business.ID)
	require.NoError(t, err)
	ctx.conflictCache = &reservationConflictSet{
		all:     nil,
		byTable: map[uint][]database.TableReservation{},
	}
	ctx.conflictCacheFrom = slot.Add(-48 * time.Hour)
	ctx.conflictCacheTo = slot.Add(48 * time.Hour)

	err = db.Transaction(func(tx *gorm.DB) error {
		return service.recheckSlotCapacityTx(tx, ctx, slot, 90, 2, nil, 0)
	})
	require.ErrorIs(t, err, database.ErrReservationSlotUnavailable)

	err = db.Transaction(func(tx *gorm.DB) error {
		return service.recheckSlotCapacityTx(tx, ctx, slot, 90, 1, nil, 0)
	})
	require.NoError(t, err)
}
