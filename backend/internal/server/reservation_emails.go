package server

// Guest-facing reservation lifecycle emails (cancelled / no-show / updated /
// approval outcomes). Split from reservation_handlers.go to keep that file
// focused on HTTP handling; everything here is the email side of those
// handlers' transitions.

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// logReservationGuestEmail records a guest reservation email attempt without
// writing the raw address to stdout. Used by the public create path and the
// lifecycle senders.
func logReservationGuestEmail(kind, email string, err error) {
	redacted := logger.RedactEmail(email)
	if err != nil {
		logger.Logger.WithError(err).Warnf("Failed to send reservation %s email to %s", kind, redacted)
		return
	}
	logger.Logger.Infof("Reservation %s email sent to %s", kind, redacted)
}

// reservationGuestCreateEmailKind decides which guest email a public create
// earns. Waitlist and pending both get the pending/waitlist template — never
// a false "confirmed" message. Confirmed bookings get confirmation.
func reservationGuestCreateEmailKind(status string) string {
	switch strings.TrimSpace(status) {
	case "pending", "waitlist":
		return "pending"
	default:
		return "confirmation"
	}
}

// sendReservationCancellationEmail sends cancellation email to customer
func sendReservationCancellationEmail(reservation *database.TableReservation, business *database.Business) {
	if emails.EmailServerInstance == nil {
		return
	}

	// Dispatch off the request path: the cancel handler must not block its HTTP
	// response on a Postmark round-trip (up to the provider's 30s timeout if the
	// API stalls). Mirrors the async CreatePublicReservation confirmation send.
	// The reservation/business pointers are only read here, concurrently with the
	// handler's read-only c.JSON serialization, so the capture is race-free.
	logger.SafeGo(func() {
		language := services.ResolveReservationEmailLanguage(reservation, business)

		reservationDate, reservationTime := services.FormatReservationEmailDateTime(reservation.ReservationTime, business.Timezone, language)

		err := emails.EmailServerInstance.ForReservation(business.ID, reservation.ID).SendReservationCancelledEmail(
			[]string{reservation.CustomerEmail},
			reservation.CustomerName,
			business.Name,
			reservationDate,
			reservationTime,
			business.Phone,
			language,
		)
		logReservationGuestEmail("cancellation", reservation.CustomerEmail, err)
	})
}

// sendReservationNoShowEmail sends no-show notification email to customer
func sendReservationNoShowEmail(reservation *database.TableReservation, business *database.Business) {
	if emails.EmailServerInstance == nil {
		return
	}

	language := services.ResolveReservationEmailLanguage(reservation, business)

	reservationDate, reservationTime := services.FormatReservationEmailDateTime(reservation.ReservationTime, business.Timezone, language)

	// Dedicated no-show template (not cancellation reuse) so inbox copy is accurate.
	// Dispatched off the request path (see sendReservationCancellationEmail).
	logger.SafeGo(func() {
		err := emails.EmailServerInstance.ForReservation(business.ID, reservation.ID).SendReservationNoShowEmail(
			[]string{reservation.CustomerEmail},
			reservation.CustomerName,
			business.Name,
			reservationDate,
			reservationTime,
			business.Phone,
			language,
		)
		logReservationGuestEmail("no-show", reservation.CustomerEmail, err)
	})
}

// sendReservationUpdatedEmail sends an update notification to the guest.
// Called only when material fields (reservation_time / party_size) changed
// AND status is not terminal (cancelled / no_show have their own emails).
// Mirrors the formatting pattern used by CreatePublicReservation.
func sendReservationUpdatedEmail(reservation *database.TableReservation, business *database.Business) {
	if emails.EmailServerInstance == nil {
		return
	}

	// Dispatched off the request path (see sendReservationCancellationEmail): the
	// update handler must not block its HTTP response on the GetTableByID lookup
	// + the Postmark round-trip. Captured pointers are read-only here.
	logger.SafeGo(func() {
		language := services.ResolveReservationEmailLanguage(reservation, business)

		reservationDate, reservationTime := services.FormatReservationEmailDateTime(reservation.ReservationTime, business.Timezone, language)
		businessAddress := services.FormatBusinessAddress(business)
		detailsURL := services.BuildReservationDetailsURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)
		cancelURL := services.BuildReservationCancellationURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)

		tableName := ""
		if reservation.TableID != nil {
			if t, err := database.GetTableByID(*reservation.TableID); err == nil {
				tableName = t.Name
			}
		}

		err := emails.EmailServerInstance.ForReservation(business.ID, reservation.ID).SendReservationUpdatedEmail(
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
			cancelURL,
			reservation.CustomerPhone,
			language,
		)
		logReservationGuestEmail("updated", reservation.CustomerEmail, err)
	})
}

// reservationApprovalEmailHook is a test seam: production wires it to the
// real email sends; tests can swap it to capture (kind, reservation, business).
var reservationApprovalEmailHook = sendReservationApprovalOutcomeEmail

// approvalOutcomeEmailKind decides which guest email a status change earns.
// "" means none. Exactly one email per outcome:
//   - pending → confirmed: the standard confirmation email ("confirmed")
//   - pending → cancelled: the declined email ("declined"), NOT the generic
//     cancelled one — the guest never had a confirmed booking to cancel
//   - anything else → cancelled: the generic cancelled email ("cancelled")
func approvalOutcomeEmailKind(prevStatus string, reservation *database.TableReservation) string {
	if reservation == nil || reservation.CustomerEmail == "" {
		return ""
	}
	if prevStatus == "pending" && reservation.Status == "confirmed" {
		return "confirmed"
	}
	if prevStatus == "pending" && reservation.Status == "cancelled" {
		return "declined"
	}
	if reservation.Status == "cancelled" {
		return "cancelled"
	}
	return ""
}

// businessPublicPageURL builds the guest-facing business page link (the
// frontend /b/[customUrl] route) for the declined email's "see other times"
// CTA. Returns "" when the business has no custom URL.
func businessPublicPageURL(business *database.Business) string {
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

// sendReservationApprovalOutcomeEmail sends the guest email earned by an
// approval decision: "confirmed" mirrors the CreatePublicReservation
// confirmation send; "declined" uses the dedicated declined template.
// Dispatched off the request path (see sendReservationCancellationEmail).
func sendReservationApprovalOutcomeEmail(kind string, reservation *database.TableReservation, business *database.Business) {
	if emails.EmailServerInstance == nil || business == nil || reservation == nil {
		return
	}
	language := services.ResolveReservationEmailLanguage(reservation, business)
	reservationDate, reservationTime := services.FormatReservationEmailDateTime(reservation.ReservationTime, business.Timezone, language)
	logger.SafeGo(func() {
		var err error
		switch kind {
		case "confirmed":
			tableName := ""
			if reservation.TableID != nil {
				if t, tErr := database.GetTableByID(*reservation.TableID); tErr == nil {
					tableName = t.Name
				}
			}
			detailsURL := services.BuildReservationDetailsURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)
			cancelURL := services.BuildReservationCancellationURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)
			err = emails.EmailServerInstance.ForReservation(business.ID, reservation.ID).SendReservationConfirmationEmail(
				[]string{reservation.CustomerEmail},
				reservation.CustomerName,
				business.Name,
				services.FormatBusinessAddress(business),
				reservationDate,
				reservationTime,
				reservation.PartySize,
				tableName,
				reservation.SpecialRequests,
				detailsURL,
				cancelURL,
				reservation.CustomerPhone,
				language,
			)
		case "declined":
			reason := strings.TrimSpace(reservation.CancellationReason)
			// Never surface machine values as a human-readable reason: the
			// approval-timeout sweeper reserves ReservationReasonApprovalTimeout,
			// and the DELETE handler stamps its own "Cancelled by business" default.
			if reason == database.ReservationReasonApprovalTimeout || reason == database.ReservationReasonCancelledByBusiness {
				reason = ""
			}
			err = emails.EmailServerInstance.ForReservation(business.ID, reservation.ID).SendReservationDeclinedEmail(
				[]string{reservation.CustomerEmail},
				reservation.CustomerName,
				business.Name,
				reservationDate,
				reservationTime,
				reason,
				businessPublicPageURL(business),
				business.Phone,
				language,
			)
		}
		logReservationGuestEmail(kind, reservation.CustomerEmail, err)
	})
}
