package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Overlapping scheduler passes (or a slow pass racing the next one) must not
// email the same guest twice: the reminder flag is claimed atomically BEFORE
// sending, so only one pass can win a given reservation.
func TestReservationReminderClaimIsExclusive(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := createTestHospitalityBusiness(t, db, "reminder-claim")

	reservation := database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Reminder Guest",
		CustomerEmail:    "reminder@example.com",
		PartySize:        2,
		ReservationTime:  time.Now().Add(24 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "REMIND000001",
	}
	require.NoError(t, db.Create(&reservation).Error)

	service := NewReservationReminderService()

	claimed, err := service.claimReminder(reservation.ID)
	require.NoError(t, err)
	assert.True(t, claimed, "first claim must win")

	claimedAgain, err := service.claimReminder(reservation.ID)
	require.NoError(t, err)
	assert.False(t, claimedAgain, "second claim must lose — reminder already sent")

	// A failed send releases the claim so the next pass retries.
	require.NoError(t, service.releaseReminderClaim(reservation.ID))
	reclaimed, err := service.claimReminder(reservation.ID)
	require.NoError(t, err)
	assert.True(t, reclaimed, "released claim must be claimable again")
}

func TestReservationReminderProcessMarksReminderSent(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := createTestHospitalityBusiness(t, db, "reminder-process")
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.SendReminderEmail = true
		settings.ReminderHoursBefore = 24
	})

	reservation := database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Window Guest",
		CustomerEmail:    "window@example.com",
		PartySize:        2,
		ReservationTime:  time.Now().Add(24 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "REMIND000002",
	}
	require.NoError(t, db.Create(&reservation).Error)

	// nil email server: send is a no-op success, the claim must stick.
	service := &ReservationReminderService{emailServer: nil}
	processed, sent, errCount := service.processBusinessReservations(business)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, errCount)

	var updated database.TableReservation
	require.NoError(t, db.First(&updated, reservation.ID).Error)
	assert.True(t, updated.ReminderSent)

	// Second pass finds nothing — no duplicate email.
	processed, sent, errCount = service.processBusinessReservations(business)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 0, errCount)
}

// TestReservationReminderCatchUpAfterDowntime is the downtime regression: the
// old predicate was an exact targetTime±30min tile matched to an hourly cron, so
// one missed run silently dropped that hour's reminders forever. The predicate
// must be catch-up-safe: remind whenever reminder_sent=false AND the reservation
// is still in the future within the lead window ("due or overdue, but not past").
func TestReservationReminderCatchUpAfterDowntime(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := createTestHospitalityBusiness(t, db, "reminder-catchup")
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.SendReminderEmail = true
		settings.ReminderHoursBefore = 24
	})

	mkRes := func(code string, at time.Time, alreadySent bool) database.TableReservation {
		r := database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     "Guest " + code,
			CustomerEmail:    code + "@example.com",
			PartySize:        2,
			ReservationTime:  at,
			Duration:         90,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: code,
			ReminderSent:     alreadySent,
		}
		require.NoError(t, db.Create(&r).Error)
		return r
	}

	now := time.Now()
	// The 23.5h-24.5h "exact window" for this booking elapsed during downtime:
	// it is now only 10h out, but still in the future — it MUST be reminded.
	overdue := mkRes("CATCHUP00001", now.Add(10*time.Hour), false)
	// Already reminded — must not be resent.
	sent := mkRes("CATCHUP00002", now.Add(10*time.Hour), true)
	// In the past — must never be reminded.
	past := mkRes("CATCHUP00003", now.Add(-1*time.Hour), false)
	// Beyond the lead window — not yet due.
	future := mkRes("CATCHUP00004", now.Add(48*time.Hour), false)

	service := &ReservationReminderService{emailServer: nil} // send = no-op success
	processed, sentCount, errCount := service.processBusinessReservations(business)
	assert.Equal(t, 1, processed, "only the overdue-but-future reservation is due")
	assert.Equal(t, 1, sentCount)
	assert.Equal(t, 0, errCount)

	var r database.TableReservation
	require.NoError(t, db.First(&r, overdue.ID).Error)
	assert.True(t, r.ReminderSent, "downtime-elapsed window must still remind")
	r = database.TableReservation{}
	require.NoError(t, db.First(&r, past.ID).Error)
	assert.False(t, r.ReminderSent, "past reservations must never be reminded")
	r = database.TableReservation{}
	require.NoError(t, db.First(&r, future.ID).Error)
	assert.False(t, r.ReminderSent, "not-yet-due reservations must wait")
	_ = sent

	// Second pass: nothing new — no double send.
	processed, sentCount, errCount = service.processBusinessReservations(business)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 0, sentCount)
	assert.Equal(t, 0, errCount)
}
