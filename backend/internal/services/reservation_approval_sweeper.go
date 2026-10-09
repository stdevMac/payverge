package services

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	"gorm.io/gorm"
)

// ReservationApprovalSweeper auto-declines pending (manual-approval)
// reservation requests whose response deadline passed, so guests are never
// left hanging on a request the business will no longer answer in time.
type ReservationApprovalSweeper struct{}

// NewReservationApprovalSweeper creates a new approval sweeper.
func NewReservationApprovalSweeper() *ReservationApprovalSweeper {
	return &ReservationApprovalSweeper{}
}

// ProcessExpiredApprovalRequests declines every pending request whose
// ReservationApprovalDeadline has passed and returns how many were declined.
// Safe to run concurrently with itself and with operator actions: each row is
// claimed via a status-guarded UPDATE before any side effects run.
func (s *ReservationApprovalSweeper) ProcessExpiredApprovalRequests() (int, error) {
	db := database.GetDB()
	now := time.Now().UTC()

	// Candidate set is bounded: pending rows whose 24h cap OR 2h-before mark
	// passed (SQL prefilter, index-friendly); the exact deadline formula —
	// including the created+30m floor — is re-checked in Go.
	var candidates []database.TableReservation
	if err := expiredApprovalCandidatesQuery(db, now).Find(&candidates).Error; err != nil {
		return 0, fmt.Errorf("load pending approval candidates: %w", err)
	}

	declined := 0
	for i := range candidates {
		res := &candidates[i]
		if now.Before(ReservationApprovalDeadline(res.CreatedAt, res.ReservationTime)) {
			continue
		}

		// Claim: status-guarded update so an overlapping sweeper pass (or a
		// just-now operator approve/decline) can never double-decline or
		// double-email the same request.
		claim := db.Model(&database.TableReservation{}).
			Where("id = ? AND status = ?", res.ID, "pending").
			Updates(map[string]any{
				"status":              "cancelled",
				"cancelled_at":        now,
				"cancelled_by":        "system",
				"cancellation_reason": database.ReservationReasonApprovalTimeout,
				"updated_at":          now,
			})
		if claim.Error != nil {
			log.Printf("approval sweeper: claim failed for reservation %d: %v", res.ID, claim.Error)
			continue
		}
		if claim.RowsAffected != 1 {
			// Lost the race — another pass or an operator already moved it.
			continue
		}
		declined++

		if err := database.CreateReservationStatusHistory(&database.ReservationStatusHistory{
			ReservationID: res.ID,
			Status:        "cancelled",
			TableID:       res.TableID,
			Notes:         "token:auto_declined_no_response",
			ChangedBy:     "system",
		}); err != nil {
			log.Printf("approval sweeper: failed to write status history for reservation %d: %v", res.ID, err)
		}

		if err := operational_alerts.NewService(db).ResolveAlertForResource(
			context.Background(),
			res.BusinessID,
			database.OperationalAlertResourceTypeReservation,
			res.ID,
			operational_alerts.Actor{Name: "system"},
			"cancelled",
		); err != nil {
			log.Printf("approval sweeper: failed to resolve operational alert for reservation %d: %v", res.ID, err)
		}

		events.GetHub().PublishJSON(res.BusinessID, "reservation.updated", map[string]any{
			"id":                  res.ID,
			"business_id":         res.BusinessID,
			"status":              "cancelled",
			"cancellation_reason": database.ReservationReasonApprovalTimeout,
		})

		s.notifyDeclined(res, now)
	}

	if declined > 0 {
		log.Printf("approval sweeper: auto-declined %d expired reservation request(s)", declined)
	}
	return declined, nil
}

// ProcessApprovalReminders emails the operator (business contact email) once
// per pending manual-approval request that crossed the halfway point of its
// approval window without being actioned, and returns how many were reminded.
// The approval_reminder_sent_at stamp doubles as the claim: a guarded UPDATE
// sets it before the email goes out, so overlapping passes never double-email.
// Requests approved/declined before their halfway mark never generate email.
func (s *ReservationApprovalSweeper) ProcessApprovalReminders() (int, error) {
	db := database.GetDB()
	now := time.Now().UTC()

	var candidates []database.TableReservation
	if err := approvalReminderCandidatesQuery(db, now).Find(&candidates).Error; err != nil {
		return 0, fmt.Errorf("load approval reminder candidates: %w", err)
	}

	reminded := 0
	for i := range candidates {
		res := &candidates[i]
		if now.Before(ReservationApprovalReminderTime(res.CreatedAt, res.ReservationTime)) {
			continue
		}
		deadline := ReservationApprovalDeadline(res.CreatedAt, res.ReservationTime)
		if !now.Before(deadline) {
			// Past the deadline entirely — the decline pass owns this row.
			continue
		}

		// Claim: stamp-guarded update so an overlapping pass (or an operator
		// action racing this one) can never double-email the same request.
		claim := db.Model(&database.TableReservation{}).
			Where("id = ? AND status = ? AND approval_reminder_sent_at IS NULL", res.ID, "pending").
			Updates(map[string]any{
				"approval_reminder_sent_at": now,
				"updated_at":                now,
			})
		if claim.Error != nil {
			log.Printf("approval sweeper: reminder claim failed for reservation %d: %v", res.ID, claim.Error)
			continue
		}
		if claim.RowsAffected != 1 {
			continue
		}
		reminded++

		s.notifyOperatorPending(res, deadline)
	}

	if reminded > 0 {
		log.Printf("approval sweeper: sent %d unactioned-request operator reminder(s)", reminded)
	}
	return reminded, nil
}

// approvalReminderCandidatesQuery builds the bounded SQL prefilter for
// reminder candidates: pending, not waitlist-assigned, never reminded, and at
// least 15 minutes old — the earliest any reminder can be due (half of the
// 30-minute floor window). The exact halfway-point and still-before-deadline
// checks run in Go.
func approvalReminderCandidatesQuery(db *gorm.DB, now time.Time) *gorm.DB {
	return db.
		Model(&database.TableReservation{}).
		Where("status = ?", "pending").
		Where("assigned_at IS NULL").
		Where("approval_reminder_sent_at IS NULL").
		Where("created_at < ?", now.Add(-15*time.Minute)).
		Order("created_at ASC").
		Limit(500)
}

// notifyOperatorPending sends the business the "still unactioned" nudge.
// Best-effort: a failed or skipped send never un-claims the stamp (matching
// the decline email's at-most-once posture).
func (s *ReservationApprovalSweeper) notifyOperatorPending(res *database.TableReservation, deadline time.Time) {
	if emails.EmailServerInstance == nil {
		return
	}

	business, err := database.GetBusinessByID(res.BusinessID)
	if err != nil {
		log.Printf("approval sweeper: failed to load business %d for reminder email: %v", res.BusinessID, err)
		return
	}
	if business.Email == "" {
		return
	}

	language := business.DefaultLanguage
	if language == "" {
		language = "en"
	}
	reservationDate, reservationTime := FormatReservationEmailDateTime(res.ReservationTime, business.Timezone, language)
	respondByDate, respondByTime := FormatReservationEmailDateTime(deadline, business.Timezone, language)
	dashboardURL := fmt.Sprintf("%s/business/%d/dashboard?tab=reservations",
		strings.TrimSuffix(strings.TrimSpace(config.FrontendBaseURL()), "/"), res.BusinessID)

	// Stamped like the decline email: a booking made through the public form
	// keeps its follow-ups in the guest-booking lane, so anonymous pending
	// requests cannot spend the operator's budget (or the venue address's
	// per-recipient allowance that staff and wallet notices share). A
	// staff-entered booking's reminder is ordinary operator mail.
	if err := emails.EmailServerInstance.ForReservationFollowUp(res.BusinessID, res.ID, res.CreatedBy).SendReservationApprovalReminderEmail(
		[]string{business.Email},
		business.OwnerName,
		business.Name,
		res.CustomerName,
		res.PartySize,
		reservationDate,
		reservationTime,
		respondByDate+" "+respondByTime,
		res.SpecialRequests,
		dashboardURL,
		language,
	); err != nil {
		log.Printf("approval sweeper: failed to send reminder email for reservation %d: %v", res.ID, err)
	}
}

// expiredApprovalCandidatesQuery builds the bounded SQL prefilter for
// expired-approval candidates. Rows with assigned_at set are excluded: the
// waitlist "assign" transition flips waitlist → pending with assigned_at
// stamped, and those are staff-driven seating steps, not approval requests.
// Oldest-first ordering makes backlog draining deterministic across passes.
// Kept as its own function so its access shape can be asserted with a DryRun
// session in tests.
func expiredApprovalCandidatesQuery(db *gorm.DB, now time.Time) *gorm.DB {
	return db.
		Model(&database.TableReservation{}).
		Where("status = ?", "pending").
		Where("assigned_at IS NULL").
		Where("created_at < ? OR reservation_time < ?", now.Add(-24*time.Hour), now.Add(2*time.Hour)).
		Order("created_at ASC").
		Limit(500)
}

// shouldEmailDeclinedGuest reports whether the auto-decline email should be
// sent: only when the guest left an email address AND the reservation time
// hasn't already passed. Past-dated rows (e.g. stale legacy pending requests
// swept long after the fact) are still declined, but emailing the guest
// "we couldn't accommodate you" after the date is worse than silence.
// Intentionally checked BEFORE the EmailServerInstance nil-check so tests can
// assert the skip without a live email server.
func shouldEmailDeclinedGuest(res *database.TableReservation, now time.Time) bool {
	if res.CustomerEmail == "" {
		return false
	}
	return !res.ReservationTime.Before(now)
}

// notifyDeclined sends the guest the declined email. The machine reason
// ("approval_timeout") stays internal — guests get the template's generic
// copy via an empty reason. Skipping the email never skips the decline.
func (s *ReservationApprovalSweeper) notifyDeclined(res *database.TableReservation, now time.Time) {
	if !shouldEmailDeclinedGuest(res, now) {
		return
	}
	if emails.EmailServerInstance == nil {
		return
	}

	business, err := database.GetBusinessByID(res.BusinessID)
	if err != nil {
		log.Printf("approval sweeper: failed to load business %d for declined email: %v", res.BusinessID, err)
		return
	}

	language := business.DefaultLanguage
	if language == "" {
		language = "en"
	}
	reservationDate, reservationTime := FormatReservationEmailDateTime(res.ReservationTime, business.Timezone, language)

	if err := emails.EmailServerInstance.ForReservationFollowUp(res.BusinessID, res.ID, res.CreatedBy).SendReservationDeclinedEmail(
		[]string{res.CustomerEmail},
		res.CustomerName,
		business.Name,
		reservationDate,
		reservationTime,
		"", // machine reason stays internal; guests get the generic copy
		approvalSweeperBusinessPageURL(business),
		business.Phone,
		language,
	); err != nil {
		log.Printf("approval sweeper: failed to send declined email for reservation %d: %v", res.ID, err)
	}
}

// approvalSweeperBusinessPageURL builds the guest-facing business page link
// (the frontend /b/[customUrl] route) for the declined email's "see other
// times" CTA. Mirrors businessPublicPageURL in internal/server (which the
// services package cannot import). Returns "" when the business has no
// custom URL.
func approvalSweeperBusinessPageURL(business *database.Business) string {
	if business == nil {
		return ""
	}
	slug := strings.TrimSpace(business.CustomURL)
	base := strings.TrimSuffix(strings.TrimSpace(config.FrontendBaseURL()), "/")
	if slug == "" || base == "" {
		return ""
	}
	return fmt.Sprintf("%s/b/%s", base, url.PathEscape(slug))
}
