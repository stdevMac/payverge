package server

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

func getReservationService() *services.ReservationService {
	return services.NewReservationService(database.GetDB())
}

// reservationEventView marshals a reservation for the dashboard SSE channel in a
// single pass, omitting the embedded Business aggregate (Stripe IDs, settlement/
// tipping wallets, billing state). The shadow Business field (same Go field name,
// shallower embed depth) wins the JSON "business" key and, being a nil
// *struct{} with omitempty, is omitted entirely. TableReservation has no custom
// MarshalJSON, so the embedded-pointer + field-shadow projection is exact, and
// the value is marshaled exactly once by Hub.PublishJSON. (SSE-03)
type reservationEventView struct {
	*database.TableReservation
	Business *struct{} `json:"business,omitempty"`
}

// reservationPublicView is the guest-safe projection for the unauthenticated
// public mutation responses (create/cancel): it additionally omits internal
// staff notes, the status history, and the assigned table association. The
// nested Table carries table_code / qr_code — working credentials for
// /guest/table/:code — which must never leave the public reservation APIs
// (#531). Like the event view it is a single-pass field-shadow projection,
// marshaled once by c.JSON. (SSE-03)
type reservationPublicView struct {
	*database.TableReservation
	Business      *struct{} `json:"business,omitempty"`
	Notes         *struct{} `json:"notes,omitempty"`
	StatusHistory *struct{} `json:"status_history,omitempty"`
	Table         *struct{} `json:"table,omitempty"`
}

// reservationEventPayload returns the SSE view of a reservation for the
// authenticated business dashboard channel (reservation.new/updated). Staff
// fields (notes, table, status history) stay, but the embedded Business
// aggregate (Stripe IDs, settlement/tipping wallets, billing state) must not
// fan out to every dashboard subscriber regardless of role.
func reservationEventPayload(r *database.TableReservation) any {
	if r == nil {
		return nil
	}
	return reservationEventView{TableReservation: r}
}

// publicReservationPayload returns a guest-safe view of a reservation for the
// unauthenticated public mutation responses (create/cancel). The raw
// TableReservation embeds the full Business (operator Stripe IDs, wallet/
// settlement/tipping addresses, contact email, internal billing state) and
// carries staff-internal Notes, a status history, and the assigned Table
// (table_code / qr_code guest QR credentials) — none of which any guest
// reservation page reads. The shadow-struct view strips those keys.
func publicReservationPayload(r *database.TableReservation) any {
	if r == nil {
		return nil
	}
	return reservationPublicView{TableReservation: r}
}

func getReservationActor(c *gin.Context) string {
	if staffID, exists := c.Get("staff_id"); exists {
		return "staff:" + strconv.FormatUint(uint64(staffID.(uint)), 10)
	}
	if tokenType, exists := c.Get("token_type"); exists {
		if tokenTypeStr, ok := tokenType.(string); ok && tokenTypeStr != "" {
			return tokenTypeStr
		}
	}
	if address, exists := c.Get("address"); exists {
		return "owner:" + address.(string)
	}
	return "system"
}

// reservationAlertRoute picks alert type + priority: pending requests are
// urgent until actioned (they have an approval deadline); auto-confirmed
// bookings are only urgent when the party arrives within 2 hours.
func reservationAlertRoute(reservation *database.TableReservation, now time.Time) (database.OperationalAlertType, database.OperationalAlertPriority) {
	if reservation.Status == "pending" {
		return database.OperationalAlertTypeReservationApproval, database.OperationalAlertPriorityUrgent
	}
	if reservation.ReservationTime.Sub(now) <= 2*time.Hour {
		return database.OperationalAlertTypeReservationNew, database.OperationalAlertPriorityUrgent
	}
	return database.OperationalAlertTypeReservationNew, database.OperationalAlertPriorityNormal
}

// createReservationOperationalAlert raises an operational alert for a
// GUEST-created reservation. Staff-created bookings must not call this — the
// staff member typed the reservation themselves, there is nothing to react to.
func createReservationOperationalAlert(c *gin.Context, reservation *database.TableReservation) {
	if reservation == nil {
		return
	}
	alertType, priority := reservationAlertRoute(reservation, time.Now())
	if err := operational_alerts.NewService(database.GetDB()).CreateReservationAlert(c.Request.Context(), *reservation, alertType, priority); err != nil {
		log.Printf("failed to create reservation operational alert: business_id=%d reservation_id=%d alert_type=%s error=%v", reservation.BusinessID, reservation.ID, alertType, err)
	}
}

func resolveReservationOperationalAlert(c *gin.Context, reservation *database.TableReservation) {
	if reservation == nil {
		return
	}
	switch reservation.Status {
	case "confirmed", "seated", "completed", "cancelled", "no_show":
		actor := operational_alerts.Actor{Name: getReservationActor(c)}
		if err := operational_alerts.NewService(database.GetDB()).ResolveAlertForResource(
			c.Request.Context(),
			reservation.BusinessID,
			database.OperationalAlertResourceTypeReservation,
			reservation.ID,
			actor,
			reservation.Status,
		); err != nil {
			log.Printf("failed to resolve reservation operational alert: business_id=%d reservation_id=%d status=%s error=%v", reservation.BusinessID, reservation.ID, reservation.Status, err)
		}
	}
}

func getReservationBusinessID(c *gin.Context) (uint, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return 0, false
	}
	return business.ID, true
}

// GetReservationSettings retrieves reservation settings for a business
func GetReservationSettings(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	settings, err := getReservationService().GetSettings(businessID)
	if err != nil {
		// FIND-060: never surface GORM/driver text to operators.
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load reservation settings")
		return
	}

	c.JSON(http.StatusOK, settings)
}

// GetReservationTableOptions returns the operator-only table selection model:
// reservation conflicts plus current floor occupancy. Keeping this endpoint
// distinct from the public availability API prevents bill identifiers from
// leaking to guests.
func GetReservationTableOptions(c *gin.Context) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	invalid := func(message string) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":  "invalid_reservation_table_options",
			"error": message,
		})
	}
	reservationTime, err := time.Parse(time.RFC3339, c.Query("reservation_time"))
	if err != nil {
		invalid("reservation_time must be RFC3339")
		return
	}
	duration, err := strconv.Atoi(c.Query("duration"))
	if err != nil || duration <= 0 {
		invalid("duration must be a positive integer")
		return
	}
	partySize, err := strconv.Atoi(c.Query("party_size"))
	if err != nil || partySize <= 0 {
		invalid("party_size must be a positive integer")
		return
	}
	var excludeReservationID uint
	if raw := strings.TrimSpace(c.Query("exclude_reservation_id")); raw != "" {
		parsed, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil || parsed == 0 {
			invalid("exclude_reservation_id must be a positive integer")
			return
		}
		excludeReservationID = uint(parsed)
	}

	options, err := getReservationService().GetTableOptions(
		business.ID,
		reservationTime,
		duration,
		partySize,
		excludeReservationID,
	)
	if err != nil {
		invalid(LogAndClientSafeErrorMessage("reservation table options", err))
		return
	}
	alertService := operational_alerts.NewService(database.GetDB())
	for _, option := range options {
		var syncErr error
		if option.OccupancyState == database.TableOccupancyStaleOccupied &&
			option.ActiveBillID != nil && option.ActiveBillOpenedAt != nil {
			syncErr = alertService.CreateStaleOccupiedTableAlert(
				c.Request.Context(),
				business.ID,
				option.ID,
				option.Name,
				*option.ActiveBillID,
				*option.ActiveBillOpenedAt,
			)
		} else {
			syncErr = alertService.ResolveStaleOccupiedTableAlert(
				c.Request.Context(),
				business.ID,
				option.ID,
			)
		}
		if syncErr != nil {
			log.Printf(
				"failed to synchronize stale table alert: business_id=%d table_id=%d error=%v",
				business.ID,
				option.ID,
				syncErr,
			)
		}
	}
	now := time.Now().UTC()
	nearTerm := !reservationTime.Before(now) &&
		!reservationTime.After(now.Add(services.ReservationNearTermWindow))
	timezone := business.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	c.JSON(http.StatusOK, gin.H{
		"business_timezone": timezone,
		"near_term":         nearTerm,
		"tables":            options,
	})
}

// UpdateReservationSettings updates reservation settings for a business
func UpdateReservationSettings(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	var input services.UpdateReservationSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondBindError(c, err)
		return
	}

	if input.ExternalPartnerLinks != nil {
		normalizedLinks, err := NormalizeAndValidateExternalPartnerLinks(*input.ExternalPartnerLinks)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		input.ExternalPartnerLinks = &normalizedLinks
		// Public demo: booking CTAs render on the storefront. Other reservation
		// fields stay editable, and a form that re-sends the stored links passes.
		if config.DemoModeEnabled() {
			current, loadErr := getReservationService().GetSettings(businessID)
			if loadErr != nil {
				RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load reservation settings")
				return
			}
			changed, cmpErr := ExternalPartnerLinksChanged(current.ExternalPartnerLinks, normalizedLinks, nil)
			if cmpErr != nil {
				log.Printf("demo reservation partner link compare failed for business %d: %v", businessID, cmpErr)
			}
			if cmpErr != nil || changed {
				demomode.Refuse(c, demomode.KindStorefront, "changing the venue's reservation booking links")
				return
			}
		}
	}

	updatedSettings, err := getReservationService().UpdateSettings(businessID, input)
	if err != nil {
		var validationErr *services.ReservationSettingsValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
			return
		}
		// Get/update/serialize failures are server faults. Log the driver text
		// and return a fixed message so SQL never reaches the client (CO-1).
		log.Printf("reservation settings update failed for business %d: %v", businessID, err)
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not update reservation settings")
		return
	}
	invalidateReservationAvailability(businessID)

	c.JSON(http.StatusOK, updatedSettings)
}

// CreateReservation creates a new table reservation
func CreateReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	var input services.CreateReservationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondBindError(c, err)
		return
	}

	reservation, err := getReservationService().CreateReservation(businessID, input, getReservationActor(c), false)
	if err != nil {
		respondReservationWriteError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)

	// Push SSE event for new reservation. No operational alert: the staff
	// member created this booking themselves, there is nothing to react to.
	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.new", reservationEventPayload(reservation))

	if wp := GetWebPushService(); wp != nil {
		logger.SafeGo(func() {
			_ = wp.SendLocalizedPushToBusiness(reservation.BusinessID, services.PushKeyNewReservation, services.PushArgs{CustomerName: reservation.CustomerName}, "")
		})
	}

	c.JSON(http.StatusCreated, reservation)
}

// reservationBusinessLocation returns the IANA location for a business's
// configured timezone, falling back to UTC when it is unset or unparseable.
// Reservation date-only filters ("Today", start_date/end_date) must anchor on
// the business's local calendar day: parsing "2026-07-03" as UTC midnight west
// of UTC (e.g. Buenos Aires, UTC-3) shifts the window three hours early, so
// tonight's 21:00-local dinner bookings fall out of the "Today" range entirely.
func reservationBusinessLocation(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// GetReservations retrieves reservations for a business
func GetReservations(c *gin.Context) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	businessID := business.ID
	// M3: do not UPDATE on every polled list GET. Stale claims are treated as
	// free by Assert/Claim paths via idle TTL; SweepStaleReservationClaims
	// remains available for optional background use.
	loc := reservationBusinessLocation(business.Timezone)

	// Parse query parameters
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	status := c.Query("status")
	// Optional free-text search across customer name/phone/email. Absent or
	// whitespace-only keeps the legacy unfiltered list shape (BE-first deploy).
	searchQ := c.Query("q")
	if _, err := database.ParseReservationStatusFilter(status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var startDate, endDate time.Time

	if startDateStr != "" {
		// Anchor at business-local midnight, not UTC midnight, so the day
		// boundary matches the operator's calendar (R3-RS-1).
		parsedStartDate, err := time.ParseInLocation("2006-01-02", startDateStr, loc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format. Use YYYY-MM-DD"})
			return
		}
		startDate = parsedStartDate
	} else {
		// Default to the start of today in the business timezone.
		nowLocal := time.Now().In(loc)
		startDate = time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	}

	if endDateStr != "" {
		parsedEndDate, err := time.ParseInLocation("2006-01-02", endDateStr, loc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format. Use YYYY-MM-DD"})
			return
		}
		endDate = parsedEndDate
		// Set to end of day (in business-local time).
		endDate = endDate.Add(24 * time.Hour).Add(-1 * time.Second)
	} else {
		// Default window must cover the whole bookable horizon: guests can
		// book up to max_advance_days out, and anything past the default end
		// would be invisible in every dashboard view until it drifted into
		// range.
		maxAdvanceDays := 0
		if settings, settingsErr := database.GetReservationSettings(businessID); settingsErr == nil {
			maxAdvanceDays = settings.MaxAdvanceDays
		}
		endDate = defaultReservationListEnd(startDate, maxAdvanceDays)
	}

	// Normalize the (business-local) bounds to UTC before the query. The
	// boundary instants are already correct — this only fixes the storage
	// format: reservation_time is persisted in UTC, and comparing it against a
	// timezone-offset-carrying bound breaks the DB's BETWEEN comparison.
	startDate = startDate.UTC()
	endDate = endDate.UTC()

	result, err := database.GetReservationsByBusinessIDPaginatedWithSearch(businessID, startDate, endDate, status, searchQ, parseDatabasePagination(c))
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not list reservations")
		return
	}

	response := gin.H{
		"reservations": result.Data,
		"total":        result.Total,
		"page":         result.Page,
		"page_size":    result.PageSize,
		"total_pages":  result.TotalPages,
	}

	// Wave 4: the approval queue (status=pending) shows guest history context.
	// One grouped query over the page's emails — never per-row.
	if status == "pending" && len(result.Data) > 0 {
		emails := make([]string, 0, len(result.Data))
		for _, reservation := range result.Data {
			emails = append(emails, reservation.CustomerEmail)
		}
		if history, historyErr := database.GetReservationGuestHistory(businessID, emails); historyErr == nil {
			response["guest_history"] = history
		}
		// On error we omit the sidecar rather than failing the list — the
		// queue still renders without context.
	}

	c.JSON(http.StatusOK, response)
}

// defaultReservationListEnd returns the end of the default (undated) list
// window: at least 30 days, stretched to max_advance_days+1 when guests can
// book further out than that.
func defaultReservationListEnd(start time.Time, maxAdvanceDays int) time.Time {
	horizon := 30
	if maxAdvanceDays+1 > horizon {
		horizon = maxAdvanceDays + 1
	}
	return start.AddDate(0, 0, horizon)
}

// GetReservation retrieves a single reservation by ID
func GetReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	reservation, err := database.GetReservationByBusinessAndID(businessID, uint(reservationID))
	if err != nil {
		respondReservationLookupError(c, businessID, uint(reservationID), err)
		return
	}

	c.JSON(http.StatusOK, reservation)
}

// respondReservationLookupError maps a GetReservationByBusinessAndID failure:
// a missing row is 404, any other (DB/driver) failure is logged and returned as
// a generic 500 so a database outage is not reported as "not found".
func respondReservationLookupError(c *gin.Context, businessID, reservationID uint, err error) {
	if errors.Is(err, database.ErrReservationNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found"})
		return
	}
	log.Printf("reservation lookup failed (business=%d reservation=%d): %v", businessID, reservationID, err)
	RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load reservation")
}

// respondReservationTransitionError maps a reservation transition error to the
// right HTTP status: a concurrent status change (the sweeper or another actor
// moved the reservation while this request was in flight) is a 409 so the client
// re-reads and retries rather than treating it as a bad request; domain
// validation messages pass through as 400 with a stable code so the FE can
// localize (L1-4); low-level DB/driver failures collapse to a generic 400 so
// SQLSTATE / relation / constraint names never cross the API boundary (CO-1 / B-15).
func respondReservationTransitionError(c *gin.Context, err error) {
	if errors.Is(err, database.ErrReservationStatusConflict) {
		c.JSON(http.StatusConflict, gin.H{
			"code":  "reservation_status_conflict",
			"error": "This reservation was just updated elsewhere. Refresh and try again.",
		})
		return
	}
	if errors.Is(err, database.ErrReservationSlotUnavailable) {
		c.JSON(http.StatusConflict, gin.H{
			"code":  "reservation_slot_unavailable",
			"error": "This reservation slot is no longer available. Pick another time.",
		})
		return
	}
	if errors.Is(err, database.ErrWaitlistFull) {
		c.JSON(http.StatusConflict, gin.H{
			"code":  "reservation_waitlist_full",
			"error": database.ErrWaitlistFull.Error(),
		})
		return
	}
	// Domain validation (unsupported transition, capacity, cancellation window,
	// …) is safe product copy. DB constraint/driver failures are not.
	msg := LogAndClientSafeErrorMessage("reservation transition", err)
	if code := reservationDomainErrorCode(msg); code != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "error": msg})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}

// reservationErrCodeEmailRequired is the wire code for an online booking that
// arrives without a deliverable guest email. The storefront localizes it from
// apiErrors.json; without a code the guest only ever saw the English sentence.
const reservationErrCodeEmailRequired = "reservation_email_required"

// respondGuestReservationEmailRequired rejects a public booking that has no
// reachable email, with the stable code the guest storefront localizes.
func respondGuestReservationEmailRequired(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": "a valid email address is required to book online",
		"code":  reservationErrCodeEmailRequired,
	})
}

// reservationDomainErrorCode maps known English domain messages (produced by
// ReservationService) onto stable wire codes. The FE localizes via apiErrors.json.
// Unknown messages return "" so callers fall back to the English body only —
// never invent a code for driver/SQL noise that LogAndClientSafeErrorMessage
// already collapsed to a generic string.
func reservationDomainErrorCode(msg string) string {
	m := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.Contains(m, "too soon to book") || (strings.Contains(m, "at least") && strings.Contains(m, "minutes in advance")):
		return "reservation_min_advance"
	case strings.Contains(m, "in the past"):
		return "reservation_in_past"
	case strings.Contains(m, "days in advance"):
		return "reservation_max_advance"
	case strings.Contains(m, "closed for the selected date") || strings.Contains(m, "business is closed"):
		return "reservation_business_closed"
	case strings.Contains(m, "operating hours") || strings.Contains(m, "too close to closing") || strings.Contains(m, "outside_operating"):
		return "reservation_outside_hours"
	case strings.Contains(m, "party size"):
		return "reservation_party_size"
	case strings.Contains(m, "slot capacity") || strings.Contains(m, "covers_limit") || strings.Contains(m, "capacity has been reached"):
		return "reservation_covers_limit"
	case strings.Contains(m, "no tables are available") || strings.Contains(m, "no available table") || strings.Contains(m, "selected table is not available") || strings.Contains(m, "no availability"):
		return "reservation_no_tables"
	case strings.Contains(m, "no longer available"):
		return "reservation_slot_unavailable"
	case strings.Contains(m, "cannot be cancelled online"):
		return "reservation_cancel_never_open"
	case strings.Contains(m, "cancellation window") || strings.Contains(m, "can no longer be cancelled") || strings.Contains(m, "cancellations are not allowed"):
		return "reservation_cancel_window"
	case strings.Contains(m, "cannot seat") || strings.Contains(m, "cannot mark no-show"):
		return "reservation_arrival_window"
	case strings.Contains(m, "unsupported reservation transition"):
		return "reservation_invalid_transition"
	case strings.Contains(m, "duration must"):
		return "reservation_invalid_duration"
	case strings.Contains(m, "duration cannot be set"):
		return "duration_not_allowed"
	case strings.Contains(m, "invalid reservation_time"):
		return "reservation_invalid_time"
	case strings.Contains(m, "email address is required"):
		return reservationErrCodeEmailRequired
	default:
		return ""
	}
}

func respondReservationWriteError(c *gin.Context, err error) {
	if errors.Is(err, database.ErrFloorLiveKitchenTickets) || errors.Is(err, database.ErrFloorPendingOrders) {
		// Live View POST /seat (reservation branch) and reservation check-in
		// share walk-in's unfinished-service 409 codes.
		respondFloorError(c, err)
		return
	}
	if errors.Is(err, services.ErrGuestDurationNotAllowed) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":  "duration_not_allowed",
			"error": err.Error(),
		})
		return
	}
	var fieldErr *services.ReservationFieldError
	if errors.As(err, &fieldErr) {
		code := "reservation_field_invalid"
		details := gin.H{"field": fieldErr.Field, "reason": fieldErr.Reason}
		if fieldErr.Reason == services.ReservationFieldReasonTooLong {
			code = "reservation_field_too_long"
			details["max_length"] = fieldErr.Max
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    code,
			"error":   fieldErr.Error(),
			"details": details,
		})
		return
	}
	var occupied *database.ReservationTableOccupiedError
	if errors.As(err, &occupied) {
		c.JSON(http.StatusConflict, gin.H{
			"code":  "reservation_table_occupied",
			"error": "The selected table still has an active bill",
			"details": gin.H{
				"table_id":              occupied.TableID,
				"occupancy_state":       occupied.State,
				"active_bill_id":        occupied.ActiveBillID,
				"active_bill_opened_at": occupied.OpenedAt,
			},
		})
		return
	}
	respondReservationTransitionError(c, err)
}

// UpdateReservation updates an existing reservation
func UpdateReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	// L4-8: require a fresh claim before mutating.
	if !requireReservationClaim(c, businessID, uint(reservationID)) {
		return
	}

	var input services.UpdateReservationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondBindError(c, err)
		return
	}

	if _, err := database.GetReservationByBusinessAndID(businessID, uint(reservationID)); err != nil {
		respondReservationLookupError(c, businessID, uint(reservationID), err)
		return
	}

	// The service returns the pre-update snapshot from the same row read it
	// mutates; using it (instead of our own earlier read) closes the race
	// where two concurrent updates both saw status=pending and both sent the
	// approval-outcome email. The service additionally guards the write with a
	// status precondition, so a transition racing the auto-decline sweeper now
	// fails as a 409 conflict instead of clobbering the swept state.
	updatedReservation, prior, err := getReservationService().UpdateReservation(businessID, uint(reservationID), input, getReservationActor(c))
	if err != nil {
		respondReservationWriteError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)
	previousReservationTime := prior.ReservationTime
	previousPartySize := prior.PartySize
	previousStatus := prior.Status

	// Push SSE event for reservation updated
	events.GetHub().PublishJSON(updatedReservation.BusinessID, "reservation.updated", reservationEventPayload(updatedReservation))
	resolveReservationOperationalAlert(c, updatedReservation)

	// Approval outcomes (pending → confirmed/cancelled) get their own dedicated
	// guest email; that branch supersedes the generic update email so the guest
	// receives exactly one email per transition.
	approvalKind := approvalOutcomeEmailKind(previousStatus, updatedReservation)
	if approvalKind == "confirmed" || approvalKind == "declined" {
		business, err := database.GetBusinessByID(updatedReservation.BusinessID)
		if err != nil {
			log.Printf("Failed to get business for reservation %s email: %v", approvalKind, err)
		} else {
			reservationApprovalEmailHook(approvalKind, updatedReservation, business)
		}
	} else if input.Status != nil && *input.Status == "no_show" && previousStatus != "no_show" {
		// Send no-show email if status changed to no_show
		business, err := database.GetBusinessByID(updatedReservation.BusinessID)
		if err == nil && updatedReservation.CustomerEmail != "" {
			sendReservationNoShowEmail(updatedReservation, business)
		}
	} else if updatedReservation.CustomerEmail != "" {
		// Send update email only when material fields changed AND status is
		// not cancelled/no_show (those have their own dedicated emails).
		statusIsTerminal := updatedReservation.Status == "cancelled" || updatedReservation.Status == "no_show"
		materialChange := !previousReservationTime.Equal(updatedReservation.ReservationTime) ||
			previousPartySize != updatedReservation.PartySize
		if !statusIsTerminal && materialChange {
			business, err := database.GetBusinessByID(updatedReservation.BusinessID)
			if err != nil {
				log.Printf("Failed to get business for reservation update email: %v", err)
			} else {
				sendReservationUpdatedEmail(updatedReservation, business)
			}
		}
	}

	c.JSON(http.StatusOK, updatedReservation)
}

// CancelReservation cancels a reservation
func CancelReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	// L4-8: require a fresh claim before mutating.
	if !requireReservationClaim(c, businessID, uint(reservationID)) {
		return
	}

	// Get reservation details before cancellation
	reservation, err := database.GetReservationByBusinessAndID(businessID, uint(reservationID))
	if err != nil {
		respondReservationLookupError(c, businessID, uint(reservationID), err)
		return
	}

	// Get business details for email
	business, err := database.GetBusinessByID(reservation.BusinessID)
	if err != nil {
		log.Printf("Failed to get business for cancellation email: %v", err)
	}

	// Capture the pre-cancel status BEFORE the transition: cancelling a
	// still-pending request is a decline, not a cancellation of a confirmed
	// booking, and earns the declined email instead of the generic one.
	previousStatus := reservation.Status

	// Cancel the reservation
	updatedReservation, err := getReservationService().TransitionReservation(businessID, uint(reservationID), "cancel", services.ReservationTransitionInput{
		Reason: database.ReservationReasonCancelledByBusiness,
	}, getReservationActor(c))
	if err != nil {
		respondReservationTransitionError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)

	// Exactly one guest email per outcome: declined for pending requests,
	// the generic cancellation email for everything else.
	switch approvalOutcomeEmailKind(previousStatus, updatedReservation) {
	case "declined":
		reservationApprovalEmailHook("declined", updatedReservation, business)
	case "cancelled":
		if business != nil {
			sendReservationCancellationEmail(updatedReservation, business)
		}
	}

	events.GetHub().PublishJSON(updatedReservation.BusinessID, "reservation.updated", reservationEventPayload(updatedReservation))
	resolveReservationOperationalAlert(c, updatedReservation)
	c.JSON(http.StatusOK, gin.H{"message": "Reservation cancelled successfully"})
}

func AssignReservationTable(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}
	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	if !requireReservationClaim(c, businessID, uint(reservationID)) {
		return
	}

	var input services.ReservationTransitionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondBindError(c, err)
		return
	}

	reservation, err := getReservationService().TransitionReservation(businessID, uint(reservationID), "assign_table", input, getReservationActor(c))
	if err != nil {
		respondReservationWriteError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)

	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.updated", reservationEventPayload(reservation))
	resolveReservationOperationalAlert(c, reservation)
	c.JSON(http.StatusOK, reservation)
}

func CheckInReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}
	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	if !requireReservationClaim(c, businessID, uint(reservationID)) {
		return
	}

	var input services.ReservationTransitionInput
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		RespondBindError(c, err)
		return
	}

	reservation, err := getReservationService().TransitionReservation(businessID, uint(reservationID), "check_in", input, getReservationActor(c))
	if err != nil {
		respondReservationWriteError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)

	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.updated", reservationEventPayload(reservation))
	resolveReservationOperationalAlert(c, reservation)
	c.JSON(http.StatusOK, reservation)
}

func MarkReservationNoShow(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}
	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	if !requireReservationClaim(c, businessID, uint(reservationID)) {
		return
	}

	var input services.ReservationTransitionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondBindError(c, err)
		return
	}

	reservation, err := getReservationService().TransitionReservation(businessID, uint(reservationID), "no_show", input, getReservationActor(c))
	if err != nil {
		respondReservationTransitionError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)

	if business, businessErr := database.GetBusinessByID(reservation.BusinessID); businessErr == nil && reservation.CustomerEmail != "" {
		sendReservationNoShowEmail(reservation, business)
	}

	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.updated", reservationEventPayload(reservation))
	resolveReservationOperationalAlert(c, reservation)
	c.JSON(http.StatusOK, reservation)
}

func PromoteWaitlistReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}
	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}

	if !requireReservationClaim(c, businessID, uint(reservationID)) {
		return
	}

	var input services.ReservationTransitionInput
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		RespondBindError(c, err)
		return
	}

	reservation, err := getReservationService().TransitionReservation(businessID, uint(reservationID), "promote_waitlist", input, getReservationActor(c))
	if err != nil {
		respondReservationWriteError(c, err)
		return
	}
	invalidateReservationAvailability(businessID)

	// Waitlist create only sent a pending/waitlist email; promotion is the
	// first true confirmation — mirror the pending→confirmed approval path.
	if reservation.CustomerEmail != "" {
		if business, bErr := database.GetBusinessByID(reservation.BusinessID); bErr == nil && business != nil {
			settings, sErr := database.GetReservationSettings(reservation.BusinessID)
			if sErr == nil && settings != nil && settings.SendConfirmationEmail {
				reservationApprovalEmailHook("confirmed", reservation, business)
			}
		}
	}

	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.updated", reservationEventPayload(reservation))
	resolveReservationOperationalAlert(c, reservation)
	c.JSON(http.StatusOK, reservation)
}

// GetUpcomingReservations retrieves upcoming reservations
func GetUpcomingReservations(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 10
	}

	reservations, err := database.GetUpcomingReservations(businessID, limit)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load upcoming reservations")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reservations": reservations,
		"total":        len(reservations),
	})
}

// parseReservationDayBounds interprets inclusive YYYY-MM-DD calendar days in
// the business-local timezone and returns UTC instants for reservation_time
// BETWEEN queries. End is the last second of the local end day.
func parseReservationDayBounds(startStr, endStr string, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	startDate, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endDate, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endDate = endDate.Add(24 * time.Hour).Add(-1 * time.Second)
	return startDate.UTC(), endDate.UTC(), nil
}

// GetReservationStats retrieves reservation statistics
func GetReservationStats(c *gin.Context) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	loc := reservationBusinessLocation(business.Timezone)
	nowLocal := time.Now().In(loc)

	// Match the list default: today-forward in the business timezone.
	// The previous -30d→today window made "20 reservations" appear over an
	// empty today-forward list when clients omitted dates on one of the two
	// endpoints.
	startDateStr := c.DefaultQuery("start_date", nowLocal.Format("2006-01-02"))
	endDateStr := c.DefaultQuery("end_date", nowLocal.AddDate(0, 0, 365).Format("2006-01-02"))

	startDate, endDate, err := parseReservationDayBounds(startDateStr, endDateStr, loc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date or end_date format. Use YYYY-MM-DD"})
		return
	}

	stats, err := database.GetReservationStats(business.ID, startDate, endDate)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load reservation stats")
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetPublicReservationSettings retrieves reservation settings by custom URL (public route)
func GetPublicReservationSettings(c *gin.Context) {
	customUrl := c.Param("customUrl")

	// Get public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customUrl)
	if err != nil {
		respondPublicBusinessLookupError(c, err)
		return
	}

	settings, err := getReservationService().GetPublicSettingsForBusiness(business)
	if err != nil {
		// Public/unauthenticated route — log the detail server-side but return a
		// generic message so a raw DB error can't leak schema details to guests
		// (matches respondPublicBusinessLookupError's generic-message convention).
		log.Printf("[Reservations] GetPublicSettings failed for business %d: %v", business.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load reservation settings"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// CreatePublicReservation creates a reservation from a guest (public route)
func CreatePublicReservation(c *gin.Context) {
	customUrl := c.Param("customUrl")

	// Get public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customUrl)
	if err != nil {
		respondPublicBusinessLookupError(c, err)
		return
	}

	settings, err := database.GetReservationSettingsForRead(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get reservation settings"})
		return
	}

	if !settings.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "Reservations are not enabled for this business"})
		return
	}

	// Match sibling guest routes (orders, delivery, promo): a suspended or
	// closed business must not accept new online bookings. Staff APIs already
	// return 403 via RequireOperationalBusiness.
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting reservations",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	var input services.CreateReservationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondBindError(c, err)
		return
	}

	// Guest bookings require a reachable email: it is the only channel for
	// approval results, confirmations, cancellations and reminders. Staff
	// bookings (walk-ins/phone) stay email-optional. ParseAddress alone is
	// not enough — john@gmail is RFC 5322-legal and never delivers.
	canonical, err := services.ParseGuestReservationEmail(input.CustomerEmail)
	if err != nil {
		respondGuestReservationEmailRequired(c)
		return
	}
	input.CustomerEmail = canonical

	reservation, err := getReservationService().CreateReservation(business.ID, input, "customer", true)
	if err != nil {
		// Occupancy conflict + domain validation pass through; DB failures
		// collapse via the shared sanitizer (CO-1).
		respondReservationWriteError(c, err)
		return
	}
	invalidateReservationAvailability(business.ID)

	// Push SSE event for new public reservation
	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.new", reservationEventPayload(reservation))
	createReservationOperationalAlert(c, reservation)

	if wp := GetWebPushService(); wp != nil {
		logger.SafeGo(func() {
			_ = wp.SendLocalizedPushToBusiness(reservation.BusinessID, services.PushKeyNewReservation, services.PushArgs{CustomerName: reservation.CustomerName}, "")
		})
	}

	if settings.SendConfirmationEmail && reservation.CustomerEmail != "" && emails.EmailServerInstance != nil {
		tableName := ""
		if reservation.TableID != nil {
			if t, err := database.GetTableByID(*reservation.TableID); err == nil {
				tableName = t.Name
			}
		}
		language := services.ResolveReservationEmailLanguage(reservation, business)
		detailsURL := services.BuildReservationDetailsURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)
		// Guests no longer get a confirm link — approval belongs to the
		// business. Pending/waitlist requests instead tell the guest when
		// they'll hear back by (never a false "confirmed" email).
		cancelURL := services.BuildReservationCancellationURL(config.FrontendBaseURL(), reservation.ConfirmationCode, language)
		reservationDate, reservationTime := services.FormatReservationEmailDateTime(reservation.ReservationTime, business.Timezone, language)
		approvalDeadline := services.ReservationApprovalDeadline(reservation.CreatedAt, reservation.ReservationTime)
		respondByDate, respondByTime := services.FormatReservationEmailDateTime(approvalDeadline, business.Timezone, language)
		respondByFull := respondByDate + " " + respondByTime
		emailKind := reservationGuestCreateEmailKind(reservation.Status)

		// The address was typed by an anonymous guest, so the send is stamped
		// as a booking-form request: it counts against the business's tenant
		// outbound email budget plus the stricter per-recipient and per-business
		// booking-confirmation caps. Over a cap the booking still stands; only
		// the email is skipped (and logged). The guest's own free text (name,
		// phone, special requests) is never rendered into this mail — the
		// senders drop it; the venue sees it on the dashboard.
		guestMailer := emails.EmailServerInstance.ForReservationRequest(reservation.BusinessID, reservation.ID)
		logger.SafeGo(func() {
			businessAddress := services.FormatBusinessAddress(business)
			var emailErr error
			switch emailKind {
			case "pending":
				emailErr = guestMailer.SendReservationPendingEmail(
					[]string{reservation.CustomerEmail},
					reservation.CustomerName,
					business.Name,
					businessAddress,
					reservationDate,
					reservationTime,
					reservation.PartySize,
					tableName,
					reservation.SpecialRequests,
					respondByFull,
					cancelURL,
					reservation.CustomerPhone,
					language,
				)
			default:
				emailErr = guestMailer.SendReservationConfirmationEmail(
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
			}
			if emailErr != nil {
				logReservationGuestEmail(emailKind, reservation.CustomerEmail, emailErr)
			}
		})
	}

	if reservation.Status == "pending" {
		deadline := services.ReservationApprovalDeadline(reservation.CreatedAt, reservation.ReservationTime).UTC().Format(time.RFC3339)
		// Guest-safe public view plus the approval deadline for pending requests.
		// The embedded reservationPublicView still wins the business/notes/
		// status_history keys (shallower depth) and is marshaled once. (SSE-03)
		c.JSON(http.StatusCreated, struct {
			reservationPublicView
			ApprovalDeadline string `json:"approval_deadline"`
		}{
			reservationPublicView: reservationPublicView{TableReservation: reservation},
			ApprovalDeadline:      deadline,
		})
		return
	}
	c.JSON(http.StatusCreated, publicReservationPayload(reservation))
}

func GetPublicReservationByCode(c *gin.Context) {
	confirmationCode := c.Param("confirmationCode")
	details, err := getReservationService().GetPublicReservationDetailsByCode(confirmationCode)
	if err != nil {
		if errors.Is(err, database.ErrReservationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found"})
			return
		}
		// Same chokepoint hygiene as respondReservationTransitionError: domain
		// validation (e.g. load failures mapped to product copy) passes through;
		// raw driver text does not (CO-1).
		c.JSON(http.StatusBadRequest, gin.H{"error": LogAndClientSafeErrorMessage("public reservation lookup", err)})
		return
	}

	c.JSON(http.StatusOK, details)
}

func CancelPublicReservation(c *gin.Context) {
	confirmationCode := c.Param("confirmationCode")
	reservation, changed, err := getReservationService().CancelReservationByCode(confirmationCode, "customer")
	if err != nil {
		if errors.Is(err, database.ErrReservationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found"})
			return
		}
		// Guest cancel ends in TransitionReservation → same DB write path as
		// operator cancel; must not echo SQLSTATE / constraint names (CO-1).
		c.JSON(http.StatusBadRequest, gin.H{"error": LogAndClientSafeErrorMessage("public reservation cancel", err)})
		return
	}

	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.updated", reservationEventPayload(reservation))
	resolveReservationOperationalAlert(c, reservation)
	invalidateReservationAvailability(reservation.BusinessID)

	message := "Reservation cancelled successfully"
	if !changed {
		message = "Reservation was already cancelled"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":             message,
		"reservation":         publicReservationPayload(reservation),
		"business_name":       reservation.Business.Name,
		"business_custom_url": reservation.Business.CustomURL,
		"business_timezone":   reservation.Business.Timezone,
	})
}

// GetReservationAvailability returns available time slots for a given date (public route)
func GetReservationAvailability(c *gin.Context) {
	customUrl := c.Param("customUrl")
	dateStr := c.Query("date")
	partySizeStr := c.DefaultQuery("party_size", "2")

	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date parameter is required (format: YYYY-MM-DD)"})
		return
	}

	// Get public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customUrl)
	if err != nil {
		respondPublicBusinessLookupError(c, err)
		return
	}

	// Get business timezone from database
	businessTZ := business.Timezone
	if businessTZ == "" {
		businessTZ = "UTC" // Fallback to UTC if not set
	}

	loc, err := time.LoadLocation(businessTZ)
	if err != nil {
		loc = time.UTC
	}

	requestedDate, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD"})
		return
	}

	partySize, err := strconv.Atoi(partySizeStr)
	if err != nil {
		partySize = 2
	}

	availability, err := reservationAvailabilityFromCache(business, requestedDate, dateStr, partySize)
	if err != nil {
		if errors.Is(err, services.ErrReservationsDisabled) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Reservations are not enabled"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": LogAndClientSafeErrorMessage("reservation availability", err)})
		return
	}

	c.JSON(http.StatusOK, availability)
}

// reservationClaimActor builds the claim-lock principal for reservation floor
// actions. Owners (no staff_role) and managers may steal; front-line may only
// hold their own. L4-8.
func reservationClaimActor(c *gin.Context) services.ClaimActor {
	actor := services.ClaimActor{Role: "owner", Name: "Owner"}
	if v, ok := c.Get("staff_id"); ok {
		if id, ok := v.(uint); ok {
			sid := id
			actor.StaffID = &sid
		}
	}
	if v, ok := c.Get("staff_name"); ok {
		if s, ok := v.(string); ok && s != "" {
			actor.Name = s
		}
	}
	if v, ok := c.Get("staff_role"); ok {
		if s, ok := v.(string); ok && s != "" {
			actor.Role = s
		}
	}
	if actor.Role == "owner" || actor.Role == string(database.StaffRoleManager) {
		actor.CanSteal = true
	}
	return actor
}

func respondReservationClaimHeld(c *gin.Context, err error) bool {
	var held *services.ReservationClaimHeldError
	if !errors.As(err, &held) {
		return false
	}
	msg := "Claim this reservation before making changes"
	if held.ClaimedByName != "" {
		msg = fmt.Sprintf("This reservation is claimed by %s", held.ClaimedByName)
	}
	c.JSON(http.StatusConflict, gin.H{
		"error":               msg,
		"code":                "claim_held",
		"claimed_by_staff_id": held.ClaimedByStaffID,
		"claimed_by_name":     held.ClaimedByName,
		"claimed_by_role":     held.ClaimedByRole,
	})
	return true
}

// requireReservationClaim gates mutating operator actions (L4-8).
func requireReservationClaim(c *gin.Context, businessID, reservationID uint) bool {
	err := getReservationService().AssertReservationClaimHeld(businessID, reservationID, reservationClaimActor(c))
	if err == nil {
		return true
	}
	if respondReservationClaimHeld(c, err) {
		return false
	}
	if strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found"})
		return false
	}
	log.Printf("reservation claim assert failed (business=%d reservation=%d): %v", businessID, reservationID, err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify reservation claim"})
	return false
}

// ClaimReservation assigns exclusive operator lock on a reservation.
// POST /inside/businesses/:id/reservations/:reservationId/claim
// Body optional: {"steal":true} for managers/owners taking over.
func ClaimReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}
	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}
	var req struct {
		Steal bool `json:"steal"`
	}
	_ = c.ShouldBindJSON(&req)

	actor := reservationClaimActor(c)
	reservation, prevStaffID, err := getReservationService().ClaimReservation(
		businessID, uint(reservationID), actor, req.Steal,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrReservationClaimStealRequired):
			var current database.TableReservation
			_ = database.GetDB().Select("claimed_by_staff_id", "claimed_by_name", "claimed_by_role").
				Where("id = ? AND business_id = ?", reservationID, businessID).First(&current).Error
			c.JSON(http.StatusConflict, gin.H{
				"error":               "This reservation is claimed by another operator — confirm steal to take over",
				"code":                "claim_steal_required",
				"claimed_by_staff_id": current.ClaimedByStaffID,
				"claimed_by_name":     current.ClaimedByName,
				"claimed_by_role":     current.ClaimedByRole,
			})
		case errors.Is(err, services.ErrReservationClaimForbidden):
			if respondReservationClaimHeld(c, err) {
				return
			}
			c.JSON(http.StatusConflict, gin.H{
				"error": "You cannot take a claim held by another operator",
				"code":  "claim_forbidden",
			})
		case respondReservationClaimHeld(c, err):
			return
		case strings.Contains(err.Error(), "not found"):
			c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found"})
		default:
			log.Printf("Failed to claim reservation %d: %v", reservationID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to claim reservation"})
		}
		return
	}

	_ = prevStaffID // notify path reserved for push (delivery-style); audit already written on steal
	events.GetHub().PublishJSON(businessID, "reservation.updated", reservationEventPayload(reservation))
	c.JSON(http.StatusOK, gin.H{
		"claimed_by_staff_id": reservation.ClaimedByStaffID,
		"claimed_by_name":     reservation.ClaimedByName,
		"claimed_by_role":     reservation.ClaimedByRole,
		"claimed_at":          reservation.ClaimedAt,
	})
}

// ReleaseReservation clears the claim (self or force by manager/owner).
// POST /inside/businesses/:id/reservations/:reservationId/release
// Body optional: {"self_only":true} for automatic post-mutate release — never
// force-clears another operator's fresh claim (mirrors claim steal flag).
func ReleaseReservation(c *gin.Context) {
	businessID, ok := getReservationBusinessID(c)
	if !ok {
		return
	}
	reservationID, err := strconv.ParseUint(c.Param("reservationId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reservation ID"})
		return
	}
	var req struct {
		SelfOnly bool `json:"self_only"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := getReservationService().ReleaseReservation(
		businessID, uint(reservationID), reservationClaimActor(c), req.SelfOnly,
	); err != nil {
		if respondReservationClaimHeld(c, err) {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found"})
			return
		}
		log.Printf("Failed to release reservation claim %d: %v", reservationID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to release reservation claim"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
