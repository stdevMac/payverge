package services

import (
	"fmt"
	"log"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

// ReservationReminderService handles sending reminder emails for upcoming reservations
type ReservationReminderService struct {
	emailServer *emails.EmailServer
}

// NewReservationReminderService creates a new reservation reminder service
func NewReservationReminderService() *ReservationReminderService {
	return &ReservationReminderService{
		emailServer: emails.EmailServerInstance,
	}
}

// ProcessUpcomingReservations checks for upcoming reservations and sends reminders
// This should be called periodically (e.g., every hour) by a cron job
func (s *ReservationReminderService) ProcessUpcomingReservations() error {
	log.Println("Starting reservation reminder processing...")

	// Get all businesses with reservations enabled
	businesses, err := s.getBusinessesWithReservationsEnabled()
	if err != nil {
		return fmt.Errorf("failed to get businesses: %w", err)
	}

	totalProcessed := 0
	totalSent := 0
	totalErrors := 0

	for _, business := range businesses {
		processed, sent, errors := s.processBusinessReservations(business)
		totalProcessed += processed
		totalSent += sent
		totalErrors += errors
	}

	log.Printf("Reservation reminder processing complete. Processed: %d, Sent: %d, Errors: %d",
		totalProcessed, totalSent, totalErrors)

	return nil
}

// processBusinessReservations processes reservations for a single business
func (s *ReservationReminderService) processBusinessReservations(business *database.Business) (processed, sent, errors int) {
	// Get reservation settings for this business. Use the pure-read variant: this
	// is an hourly background sweep, so it must never persist normalized defaults
	// back on read (the persist variant GetReservationSettings would issue a
	// hidden UPDATE per business, per hour). The eligibility JOIN already proved
	// the settings row exists.
	settings, err := database.GetReservationSettingsForRead(business.ID)
	if err != nil {
		log.Printf("Failed to get reservation settings for business %d: %v", business.ID, err)
		return 0, 0, 1
	}

	// Skip if reminder emails are disabled
	if !settings.SendReminderEmail {
		return 0, 0, 0
	}

	// Calculate the time window for reminders
	reminderHours := settings.ReminderHoursBefore
	if reminderHours <= 0 {
		reminderHours = 24 // Default to 24 hours
	}

	// Catch-up-safe reminder window: remind any un-reminded, confirmed booking
	// that is DUE OR OVERDUE but still in the future — i.e. reservation_time in
	// (now, now + lead]. The previous predicate was an exact targetTime ± 30min
	// tile matched to the hourly cron, so a single missed run (deploy, crash,
	// host downtime) silently dropped that hour's reminders forever. This shape
	// self-heals across any downtime and cannot double-send: the reminder_sent
	// claim is CASed before sending (claimReminder) and only released on a
	// failed send.
	now := time.Now()
	startWindow := now
	endWindow := now.Add(time.Duration(reminderHours) * time.Hour)

	// Get reservations in this time window that haven't had reminders sent
	reservations, err := s.getReservationsNeedingReminders(business.ID, startWindow, endWindow)
	if err != nil {
		log.Printf("Failed to get reservations for business %d: %v", business.ID, err)
		return 0, 0, 1
	}

	processed = len(reservations)

	for _, reservation := range reservations {
		// Claim the reminder BEFORE sending so an overlapping scheduler pass
		// (or a retry after a slow pass) can never email the same guest twice.
		claimed, err := s.claimReminder(reservation.ID)
		if err != nil {
			log.Printf("Failed to claim reminder for reservation %d: %v", reservation.ID, err)
			errors++
			continue
		}
		if !claimed {
			// Another pass already owns this reminder.
			continue
		}
		if err := s.sendReminderEmail(business, &reservation, settings); err != nil {
			log.Printf("Failed to send reminder for reservation %d: %v", reservation.ID, err)
			errors++
			// Release the claim so the next pass retries; if the release
			// itself fails we deliberately leave the claim in place — a
			// missed reminder beats a duplicate one.
			if releaseErr := s.releaseReminderClaim(reservation.ID); releaseErr != nil {
				log.Printf("Failed to release reminder claim for reservation %d: %v", reservation.ID, releaseErr)
			}
		} else {
			sent++
		}
	}

	return processed, sent, errors
}

// getBusinessesWithReservationsEnabled returns all businesses with reservations enabled.
//
// The projection is deliberately narrow: this hourly sweep only needs the scalar
// fields sendReminderEmail reads off each Business (id for the per-business
// reservation/settings lookups, plus name/phone/timezone/default_language and the
// embedded address columns for the reminder email). A SELECT * here would hydrate
// the full ~115-column row — onboarding_state JSON, welcome_message/about_story
// text, every stripe_*/ai_* blob — for every reservation-enabled business, every
// hour, only to discard it. Keep this list in sync with sendReminderEmail.
func (s *ReservationReminderService) getBusinessesWithReservationsEnabled() ([]*database.Business, error) {
	var businesses []*database.Business

	// Query businesses that have reservation settings enabled
	err := database.GetDB().
		Select(
			"businesses.id",
			"businesses.name",
			"businesses.phone",
			"businesses.timezone",
			"businesses.default_language",
			// Embedded BusinessAddress columns consumed by FormatBusinessAddress.
			"businesses.street",
			"businesses.city",
			"businesses.state",
			"businesses.postal_code",
			"businesses.country",
		).
		Joins("JOIN reservation_settings ON reservation_settings.business_id = businesses.id").
		Where("reservation_settings.enabled = ? AND reservation_settings.send_reminder_email = ?", true, true).
		Find(&businesses).Error

	if err != nil {
		return nil, err
	}

	return businesses, nil
}

// getReservationsNeedingReminders gets reservations that need reminder emails
func (s *ReservationReminderService) getReservationsNeedingReminders(
	businessID uint,
	startTime, endTime time.Time,
) ([]database.TableReservation, error) {
	var reservations []database.TableReservation

	err := database.GetDB().
		Preload("Table").
		Where("business_id = ?", businessID).
		// Half-open (start, end]: strictly future (never remind a past booking),
		// due-or-overdue within the lead window (catch-up-safe across downtime).
		Where("reservation_time > ? AND reservation_time <= ?", startTime, endTime).
		// Only remind CONFIRMED bookings. A manual-approval business's request sits
		// at "pending" until the operator approves it (and may still be declined),
		// so a guest "see you tomorrow" reminder for a pending row is wrong; auto-
		// approval bookings are already "confirmed", so this covers both modes.
		Where("status = ?", "confirmed").
		Where("reminder_sent = ?", false).
		Where("customer_email != ?", ""). // Only get reservations with email
		Find(&reservations).Error

	if err != nil {
		return nil, err
	}

	return reservations, nil
}

// sendReminderEmail sends a reminder email for a reservation
func (s *ReservationReminderService) sendReminderEmail(
	business *database.Business,
	reservation *database.TableReservation,
	settings *database.ReservationSettings,
) error {
	// Skip if no customer email
	if reservation.CustomerEmail == "" || s.emailServer == nil {
		return nil
	}

	// Prefer language stored at booking time; fall back to business default.
	language := ResolveReservationEmailLanguage(reservation, business)

	// Format date and time in the email's language
	reservationDate, reservationTime := FormatReservationEmailDateTime(reservation.ReservationTime, business.Timezone, language)

	// Build business address
	businessAddress := FormatBusinessAddress(business)

	// Generate URLs (carry booking language into confirmation/cancel pages)
	detailsURL := BuildReservationDetailsURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)
	cancellationURL := BuildReservationCancellationURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)

	// Get table name
	tableName := "Table"
	if reservation.Table.ID != 0 {
		tableName = reservation.Table.Name
	}

	// Send the reminder email
	// Stamped with the reservation so it is attributed (bounces) and counted
	// against the business's tenant outbound email budget; a guest-made
	// booking's reminder stays in the guest-booking lane.
	return s.emailServer.ForReservationFollowUp(business.ID, reservation.ID, reservation.CreatedBy).SendReservationReminderEmail(
		[]string{reservation.CustomerEmail},
		reservation.CustomerName,
		business.Name,
		businessAddress,
		reservationDate,
		reservationTime,
		reservation.PartySize,
		tableName,
		reservation.SpecialRequests,
		detailsURL,
		cancellationURL,
		business.Phone,
		language,
	)
}

// claimReminder atomically flips reminder_sent false→true and reports whether
// this caller won the claim. Losing the claim means another pass already sent
// (or is sending) the reminder.
func (s *ReservationReminderService) claimReminder(reservationID uint) (bool, error) {
	result := database.GetDB().
		Model(&database.TableReservation{}).
		Where("id = ? AND reminder_sent = ?", reservationID, false).
		Update("reminder_sent", true)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// releaseReminderClaim returns a claimed reminder to the pool after a failed
// send so a later pass can retry it.
func (s *ReservationReminderService) releaseReminderClaim(reservationID uint) error {
	return database.GetDB().
		Model(&database.TableReservation{}).
		Where("id = ?", reservationID).
		Update("reminder_sent", false).
		Error
}
