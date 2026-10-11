package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedApprovalSweeperReservation creates a reservation with a backdated
// created_at. GORM autofills CreatedAt on Create, so the backdate is applied
// via UpdateColumn afterwards (UpdateColumn also leaves updated_at alone).
func seedApprovalSweeperReservation(
	t *testing.T,
	db *gorm.DB,
	businessID uint,
	status string,
	reservationTime, createdAt time.Time,
) *database.TableReservation {
	t.Helper()

	res := &database.TableReservation{
		BusinessID:       businessID,
		CustomerName:     "Sweep Guest",
		CustomerEmail:    "",
		PartySize:        2,
		ReservationTime:  reservationTime,
		Status:           status,
		Source:           "customer",
		CreatedBy:        "customer",
		ConfirmationCode: fmt.Sprintf("SWP%d", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(res).Error)
	require.NoError(t, db.Model(res).UpdateColumn("created_at", createdAt).Error)
	res.CreatedAt = createdAt
	return res
}

func TestApprovalSweeperDeclinesPastDeadlineOnly(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "approval-sweep")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.ApprovalMode = database.ReservationApprovalManual
	})

	now := time.Now().UTC()
	nextWeek := now.AddDate(0, 0, 7)

	// Created 25h ago → the created+24h cap passed an hour ago.
	expired := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-25*time.Hour))
	// Created 1h ago → deadline is still ~23h out.
	fresh := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-1*time.Hour))

	sweeper := NewReservationApprovalSweeper()
	declined, err := sweeper.ProcessExpiredApprovalRequests()
	require.NoError(t, err)
	require.Equal(t, 1, declined)

	var got database.TableReservation
	require.NoError(t, db.First(&got, expired.ID).Error)
	require.Equal(t, "cancelled", got.Status)
	require.Equal(t, "system", got.CancelledBy)
	require.Equal(t, database.ReservationReasonApprovalTimeout, got.CancellationReason)
	require.NotNil(t, got.CancelledAt)

	var history []database.ReservationStatusHistory
	require.NoError(t, db.Where("reservation_id = ?", expired.ID).Find(&history).Error)
	require.Len(t, history, 1)
	require.Equal(t, "cancelled", history[0].Status)
	require.Equal(t, "system", history[0].ChangedBy)

	var freshGot database.TableReservation
	require.NoError(t, db.First(&freshGot, fresh.ID).Error)
	require.Equal(t, "pending", freshGot.Status)
	require.Empty(t, freshGot.CancellationReason)

	// Second pass is idempotent: the claim already flipped the row out of
	// "pending", so nothing is double-declined (or double-emailed).
	declinedAgain, err := sweeper.ProcessExpiredApprovalRequests()
	require.NoError(t, err)
	require.Equal(t, 0, declinedAgain)

	require.NoError(t, db.Where("reservation_id = ?", expired.ID).Find(&history).Error)
	require.Len(t, history, 1)
}

// Past-dated legacy rows (e.g. stale pending requests swept on first deploy)
// must still be declined, but the guest must NOT get a months-late
// "we couldn't accommodate you" email.
func TestApprovalSweeperDeclinesPastDatedWithoutEmail(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "approval-sweep-pastdate")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.ApprovalMode = database.ReservationApprovalManual
	})

	now := time.Now().UTC()
	// Reservation time already passed (months ago), still pending, guest left an email.
	stale := seedApprovalSweeperReservation(t, db, business.ID, "pending", now.AddDate(0, -2, 0), now.AddDate(0, -2, -1))
	require.NoError(t, db.Model(stale).UpdateColumn("customer_email", "guest@example.com").Error)
	stale.CustomerEmail = "guest@example.com"

	sweeper := NewReservationApprovalSweeper()
	declined, err := sweeper.ProcessExpiredApprovalRequests()
	require.NoError(t, err)
	require.Equal(t, 1, declined)

	var got database.TableReservation
	require.NoError(t, db.First(&got, stale.ID).Error)
	require.Equal(t, "cancelled", got.Status)
	require.Equal(t, database.ReservationReasonApprovalTimeout, got.CancellationReason)

	// No email attempt: the past-date guard short-circuits before any email
	// plumbing (including the EmailServerInstance nil-check), and the pure
	// predicate the sweeper uses says "don't email".
	require.False(t, shouldEmailDeclinedGuest(&got, time.Now().UTC()),
		"past-dated reservations must not trigger a late decline email")
}

func TestShouldEmailDeclinedGuest(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(48 * time.Hour)
	past := now.Add(-48 * time.Hour)

	require.True(t, shouldEmailDeclinedGuest(&database.TableReservation{CustomerEmail: "g@x.com", ReservationTime: future}, now))
	require.False(t, shouldEmailDeclinedGuest(&database.TableReservation{CustomerEmail: "", ReservationTime: future}, now),
		"no email address → no email")
	require.False(t, shouldEmailDeclinedGuest(&database.TableReservation{CustomerEmail: "g@x.com", ReservationTime: past}, now),
		"reservation already in the past → declining is cleanup, emailing is not")
}

// Waitlist→pending rows produced by the "assign" transition carry assigned_at;
// they are staff-driven seating steps, not approval requests — the sweeper
// must leave them alone.
func TestApprovalSweeperSkipsWaitlistAssignedRows(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "approval-sweep-assigned")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.ApprovalMode = database.ReservationApprovalManual
	})

	now := time.Now().UTC()
	assigned := seedApprovalSweeperReservation(t, db, business.ID, "pending", now.AddDate(0, 0, 7), now.Add(-25*time.Hour))
	require.NoError(t, db.Model(assigned).UpdateColumn("assigned_at", now.Add(-1*time.Hour)).Error)

	sweeper := NewReservationApprovalSweeper()
	declined, err := sweeper.ProcessExpiredApprovalRequests()
	require.NoError(t, err)
	require.Equal(t, 0, declined)

	var got database.TableReservation
	require.NoError(t, db.First(&got, assigned.ID).Error)
	require.Equal(t, "pending", got.Status)
	require.Empty(t, got.CancellationReason)
	require.Nil(t, got.CancelledAt)
}

// Access-shape guard (per the repo perf gate): the candidates prefilter must
// stay a bounded, index-friendly query — status filter, assigned_at IS NULL,
// the time-bound OR, oldest-first ordering, and a LIMIT.
func TestApprovalSweeperCandidatesQueryShape(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)

	now := time.Now().UTC()
	var rows []database.TableReservation
	stmt := expiredApprovalCandidatesQuery(db.Session(&gorm.Session{DryRun: true}), now).
		Find(&rows).Statement

	sql := strings.ToLower(strings.NewReplacer("`", "", `"`, "").Replace(stmt.SQL.String()))

	require.Contains(t, sql, "status = ?", "must filter to pending rows")
	require.Contains(t, sql, "assigned_at is null", "must exclude waitlist-assigned (staff-driven) rows")
	require.Contains(t, sql, "created_at < ? or reservation_time < ?", "must keep the bounded time prefilter")
	require.Contains(t, sql, "order by created_at", "must drain the backlog oldest-first")
	require.Contains(t, sql, "limit", "must stay bounded")
	require.Contains(t, stmt.Vars, "pending")
}

// The reminder pass nudges the operator about pending requests that crossed
// the halfway point of their approval window — and only those. Already
// stamped, not-yet-due, assigned, non-pending, and already-expired rows are
// all left alone.
func TestApprovalReminderStampsDueRowsOnly(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "approval-remind")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.ApprovalMode = database.ReservationApprovalManual
	})

	now := time.Now().UTC()
	nextWeek := now.AddDate(0, 0, 7)

	// Created 13h ago, far-future booking: 24h window, reminder due at 12h.
	due := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-13*time.Hour))
	// Created 1h ago: reminder not due until 12h.
	notDue := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-1*time.Hour))
	// Due, but already reminded.
	stamped := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-13*time.Hour))
	require.NoError(t, db.Model(stamped).UpdateColumn("approval_reminder_sent_at", now.Add(-30*time.Minute)).Error)
	// Due, but waitlist-assigned (staff-driven seating step, not a request).
	assigned := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-13*time.Hour))
	require.NoError(t, db.Model(assigned).UpdateColumn("assigned_at", now.Add(-1*time.Hour)).Error)
	// Past its deadline entirely: belongs to the decline pass, not a nudge.
	expired := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-25*time.Hour))
	// Already actioned.
	confirmed := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", nextWeek, now.Add(-13*time.Hour))

	sweeper := NewReservationApprovalSweeper()
	reminded, err := sweeper.ProcessApprovalReminders()
	require.NoError(t, err)
	require.Equal(t, 1, reminded)

	var got database.TableReservation
	require.NoError(t, db.First(&got, due.ID).Error)
	require.NotNil(t, got.ApprovalReminderSentAt, "due row must be stamped")
	require.Equal(t, "pending", got.Status, "reminder must not touch status")

	for _, untouched := range []*database.TableReservation{notDue, assigned, expired, confirmed} {
		var row database.TableReservation
		require.NoError(t, db.First(&row, untouched.ID).Error)
		require.Nil(t, row.ApprovalReminderSentAt, "row %d must not be stamped", untouched.ID)
	}

	// Second pass is a no-op: the stamp is the claim.
	remindedAgain, err := sweeper.ProcessApprovalReminders()
	require.NoError(t, err)
	require.Equal(t, 0, remindedAgain)
}

// Same-day bookings get proportionally earlier nudges: a 6h window (deadline
// = reservation-2h) is due at 3h.
func TestApprovalReminderShortWindowDueProportionally(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "approval-remind-short")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.ApprovalMode = database.ReservationApprovalManual
	})

	now := time.Now().UTC()
	// Created 4h ago for a booking 4h from now: window = created → reservation-2h
	// = 6h, halfway at 3h → due an hour ago.
	res := seedApprovalSweeperReservation(t, db, business.ID, "pending", now.Add(4*time.Hour), now.Add(-4*time.Hour))

	sweeper := NewReservationApprovalSweeper()
	reminded, err := sweeper.ProcessApprovalReminders()
	require.NoError(t, err)
	require.Equal(t, 1, reminded)

	var got database.TableReservation
	require.NoError(t, db.First(&got, res.ID).Error)
	require.NotNil(t, got.ApprovalReminderSentAt)
}

// Access-shape guard for the reminder prefilter: bounded, index-friendly,
// unsent-only.
func TestApprovalReminderCandidatesQueryShape(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)

	now := time.Now().UTC()
	var rows []database.TableReservation
	stmt := approvalReminderCandidatesQuery(db.Session(&gorm.Session{DryRun: true}), now).
		Find(&rows).Statement

	sql := strings.ToLower(strings.NewReplacer("`", "", `"`, "").Replace(stmt.SQL.String()))

	require.Contains(t, sql, "status = ?", "must filter to pending rows")
	require.Contains(t, sql, "assigned_at is null", "must exclude waitlist-assigned (staff-driven) rows")
	require.Contains(t, sql, "approval_reminder_sent_at is null", "must exclude already-reminded rows")
	require.Contains(t, sql, "created_at < ?", "must keep the bounded age prefilter")
	require.Contains(t, sql, "order by created_at", "must drain oldest-first")
	require.Contains(t, sql, "limit", "must stay bounded")
	require.Contains(t, stmt.Vars, "pending")
}

func TestApprovalSweeperSkipsNonPending(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}))

	business := createTestHospitalityBusiness(t, db, "approval-sweep-confirmed")
	configureReservationSettings(t, business.ID, func(s *database.ReservationSettings) {
		s.ApprovalMode = database.ReservationApprovalManual
	})

	now := time.Now().UTC()
	confirmed := seedApprovalSweeperReservation(t, db, business.ID, "confirmed", now.AddDate(0, 0, 7), now.Add(-25*time.Hour))

	sweeper := NewReservationApprovalSweeper()
	declined, err := sweeper.ProcessExpiredApprovalRequests()
	require.NoError(t, err)
	require.Equal(t, 0, declined)

	var got database.TableReservation
	require.NoError(t, db.First(&got, confirmed.ID).Error)
	require.Equal(t, "confirmed", got.Status)
	require.Empty(t, got.CancellationReason)
	require.Nil(t, got.CancelledAt)
}
