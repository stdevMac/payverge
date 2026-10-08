package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateReservationAssignsWaitlistPositionsSequentially(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "waitlist-update-pos")
	createTestTable(t, db, business.ID, "Only", 2)
	configureReservationSettings(t, business.ID, nil)

	fullAt := nextDayAt(19, 0)
	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Filler Guest",
		CustomerPhone:   "5550000001",
		CustomerEmail:   "filler@example.com",
		PartySize:       2,
		ReservationTime: fullAt.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	createAt := func(name, phone, email string, hour, minute int) uint {
		t.Helper()
		created, createErr := service.CreateReservation(business.ID, CreateReservationInput{
			CustomerName:    name,
			CustomerPhone:   phone,
			CustomerEmail:   email,
			PartySize:       2,
			ReservationTime: nextDayAt(hour, minute).Format(time.RFC3339),
			Source:          "staff",
		}, "host", false)
		require.NoError(t, createErr)
		require.NotNil(t, created.TableID)
		require.Equal(t, "confirmed", created.Status)
		return created.ID
	}

	firstID := createAt("Ada Guest", "5550000002", "ada@example.com", 12, 0)
	secondID := createAt("Bea Guest", "5550000003", "bea@example.com", 15, 0)

	full := fullAt.Format(time.RFC3339)
	move := func(id uint) *int {
		t.Helper()
		updated, _, updateErr := service.UpdateReservation(business.ID, id, UpdateReservationInput{
			ReservationTime: &full,
			ClearTable:      true,
		}, "host")
		require.NoError(t, updateErr)
		require.Equal(t, "waitlist", updated.Status)
		require.Nil(t, updated.TableID)
		require.NotNil(t, updated.WaitlistPosition)
		return updated.WaitlistPosition
	}

	assert.Equal(t, 1, *move(firstID))
	assert.Equal(t, 2, *move(secondID))
}
