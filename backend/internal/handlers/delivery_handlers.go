package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"
	serverhelpers "github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

// driverPhoneAllowed matches operator phone shapes: optional +, digits, and
// common separators. Letters (e.g. "abc") are rejected (L3-42).
var driverPhoneAllowed = regexp.MustCompile(`^[+]?[\d\s()\-]{7,20}$`)

// isValidDriverPhone requires 7–15 digits and only phone-like punctuation.
func isValidDriverPhone(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || !driverPhoneAllowed.MatchString(s) {
		return false
	}
	digits := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return digits >= 7 && digits <= 15
}

type DeliveryHandler struct {
	deliveryService *services.DeliveryService
}

func NewDeliveryHandler(deliveryService *services.DeliveryService) *DeliveryHandler {
	return &DeliveryHandler{
		deliveryService: deliveryService,
	}
}

// sharedDeliveryService lets package-level handlers (orders.go) reach the
// fully-wired delivery service (with notification manager) that main.go
// constructs. Set once at startup via SetSharedDeliveryService.
var sharedDeliveryService *services.DeliveryService

// SetSharedDeliveryService wires the delivery service into the package-level
// order-status handler. Called once from main.go after service construction.
func SetSharedDeliveryService(svc *services.DeliveryService) { sharedDeliveryService = svc }

func deliveryActor(c *gin.Context) string {
	if staffID, exists := c.Get("staff_id"); exists {
		return "staff:" + strconv.FormatUint(uint64(staffID.(uint)), 10)
	}
	if address, exists := c.Get("address"); exists {
		return "owner:" + address.(string)
	}
	return "system"
}

// deliveryClaimActor builds the claim-lock principal for dispatch. Owners
// (no staff_role) and managers may steal; front-line may only hold their own.
func deliveryClaimActor(c *gin.Context) services.ClaimActor {
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
	// Steal permission mirrors AI waiter: owner principal or manager role.
	if actor.Role == "owner" || actor.Role == string(database.StaffRoleManager) {
		actor.CanSteal = true
	}
	return actor
}

// respondDeliveryClaimHeld writes a 409 that names the current holder so the
// blocked operator knows who to ask.
func respondDeliveryClaimHeld(c *gin.Context, err error) bool {
	var held *services.DeliveryClaimHeldError
	if !errors.As(err, &held) {
		return false
	}
	var msg string
	if held.ClaimedByName != "" {
		msg = fmt.Sprintf("This delivery is claimed by %s", held.ClaimedByName)
	} else {
		msg = "Claim this delivery before making changes"
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

// requireDeliveryClaim gates a mutating dispatch action on holding a fresh claim.
func (h *DeliveryHandler) requireDeliveryClaim(c *gin.Context, businessID, deliveryID uint) bool {
	err := h.deliveryService.AssertDeliveryClaimHeld(businessID, deliveryID, deliveryClaimActor(c))
	if err == nil {
		return true
	}
	if respondDeliveryClaimHeld(c, err) {
		return false
	}
	if strings.Contains(err.Error(), "not found") {
		serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "Delivery order not found")
		return false
	}
	log.Printf("delivery claim assert failed (business=%d delivery=%d): %v", businessID, deliveryID, err)
	serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
	return false
}

func deliveryOperationalAlertActor(c *gin.Context) operational_alerts.Actor {
	return operational_alerts.Actor{Name: deliveryActor(c)}
}

func deliveryStatusAddressesAlert(status database.DeliveryStatus) bool {
	return status != database.DeliveryStatusPending
}

func deliveryCreateErrorMessage(err error) string {
	message := err.Error()
	for _, prefix := range []string{
		services.ErrDeliveryValidation.Error() + ": ",
		services.ErrDeliveryScopedNotFound.Error() + ": ",
	} {
		message = strings.TrimPrefix(message, prefix)
	}
	return message
}

// deliverySettingsValidationMessageMarkers are the known-safe, operator-facing
// message shapes produced by services.(*DeliveryService).validateDeliverySettings.
// That validator returns bare errors (no ErrDeliveryValidation sentinel), so
// the handler allowlists its message shapes to keep returning them as 400s
// while every other (potentially raw DB/GORM) error collapses to a generic
// 500 (DEL-LEAK-6). The proper fix is wrapping the validator's errors in
// ErrDeliveryValidation at the service layer; this stays handler-side because
// internal/services is owned by a separate workstream.
var deliverySettingsValidationMessageMarkers = []string{
	"cannot be negative",
	"invalid payment_mode",
	"invalid delivery_zones",
	"invalid external_partner_links",
	"name required",
	"requires at least one",
	"priority must be >= 1",
	"duplicate active zone priority",
	"estimated_time must be > 0",
}

func isDeliverySettingsValidationMessage(err error) bool {
	msg := err.Error()
	for _, marker := range deliverySettingsValidationMessageMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// deliveryValidationCode maps known delivery validation messages to stable
// FE-localizable codes. English error text is kept for logs/API consumers.
func deliveryValidationCode(err error) string {
	if err == nil {
		return serverhelpers.ErrCodeInvalidInput
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "delivery fee cannot be negative"),
		strings.Contains(msg, "fees and minimums cannot be negative"),
		strings.Contains(msg, "fee and minimum cannot be negative"),
		strings.Contains(msg, "timing and capacity values cannot be negative"):
		return serverhelpers.ErrCodeDeliveryFeeNegative
	case strings.Contains(msg, "driver tip cannot be negative"):
		return serverhelpers.ErrCodeDeliveryTipNegative
	case strings.Contains(msg, "invalid payment_mode"):
		return serverhelpers.ErrCodeDeliveryPaymentModeInvalid
	case strings.Contains(msg, "invalid delivery_zones"):
		return serverhelpers.ErrCodeDeliveryZoneInvalid
	case strings.Contains(msg, "invalid external_partner_links"):
		return serverhelpers.ErrCodeDeliveryPartnerLinksInvalid
	case strings.Contains(msg, "name required"):
		return serverhelpers.ErrCodeDeliveryZoneNameRequired
	case strings.Contains(msg, "requires at least one"):
		return serverhelpers.ErrCodeDeliveryRequiresZoneOrPartner
	case strings.Contains(msg, "priority must be"):
		return serverhelpers.ErrCodeDeliveryZonePriorityInvalid
	case strings.Contains(msg, "duplicate active zone priority"):
		return serverhelpers.ErrCodeDeliveryZonePriorityDuplicate
	case strings.Contains(msg, "estimated_time must"):
		return serverhelpers.ErrCodeDeliveryZoneEstimatedTimeInvalid
	case strings.Contains(msg, "delivery_start_time"), strings.Contains(msg, "delivery_end_time"):
		return serverhelpers.ErrCodeDeliveryHoursInvalid
	default:
		return serverhelpers.ErrCodeInvalidInput
	}
}

func deliveryBusinessIDFromRoute(c *gin.Context) (uint, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return 0, false
	}
	return business.ID, true
}

// CreateDeliveryOrder creates a new delivery order
func (h *DeliveryHandler) CreateDeliveryOrder(c *gin.Context) {
	if _, exists := c.Get("token_type"); !exists {
		serverhelpers.RespondWithError(c, http.StatusUnauthorized, serverhelpers.ErrCodeTokenInvalid, "Authentication required")
		return
	}

	var req services.CreateDeliveryOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	businessIDParam := c.Param("id")
	if businessIDParam == "" {
		businessIDParam = c.Param("business_id")
	}

	// Validate business ID from URL matches request
	var businessID uint
	if c.Param("id") != "" {
		var ok bool
		businessID, ok = deliveryBusinessIDFromRoute(c)
		if !ok {
			return
		}
	} else {
		parsedBusinessID, err := strconv.ParseUint(businessIDParam, 10, 32)
		if err != nil {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid business ID")
			return
		}
		businessID = uint(parsedBusinessID)
	}
	req.BusinessID = businessID

	deliveryOrder, err := h.deliveryService.CreateDeliveryOrder(req)
	if err != nil {
		log.Printf("Failed to create delivery order: %v", err)
		switch {
		case errors.Is(err, services.ErrDeliveryValidation):
			serverhelpers.RespondWithError(c, http.StatusBadRequest, deliveryValidationCode(err), deliveryCreateErrorMessage(err))
		case errors.Is(err, services.ErrDeliveryScopedNotFound):
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, deliveryCreateErrorMessage(err))
		default:
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	events.GetHub().PublishJSON(deliveryOrder.BusinessID, "delivery.created", deliveryOrder)
	if err := operational_alerts.NewService(database.GetDB()).CreateDeliveryNewAlert(c.Request.Context(), *deliveryOrder); err != nil {
		log.Printf("failed to create delivery operational alert: business_id=%d delivery_id=%d error=%v", deliveryOrder.BusinessID, deliveryOrder.ID, err)
	}
	c.JSON(http.StatusCreated, deliveryOrder)
}

// GetDeliveryOrder retrieves a delivery order by ID
func (h *DeliveryHandler) GetDeliveryOrder(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}

	deliveryOrder, err := h.deliveryService.GetDeliveryOrderByBusiness(businessID, uint(deliveryID))
	if err != nil {
		log.Printf("Delivery order not found (ID: %d): %v", deliveryID, err)
		serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "Delivery order not found")
		return
	}

	c.JSON(http.StatusOK, deliveryOrder)
}

// GetBusinessDeliveries retrieves paginated deliveries for a business.
// Query params: limit (default 100, max 500), offset (default 0),
// status (optional filter), since (optional ISO timestamp; updated_at filter).
func (h *DeliveryHandler) GetBusinessDeliveries(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	params := services.DeliveryListParams{}

	if limitStr := c.Query("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v <= 0 {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid limit parameter")
			return
		}
		params.Limit = v
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid offset parameter")
			return
		}
		params.Offset = v
	}

	// status accepts a single value (legacy) or a comma-separated list for the
	// dispatch board's active/terminal windows. Empty keeps the unfiltered shape.
	if statusStr := c.Query("status"); statusStr != "" {
		parts := strings.Split(statusStr, ",")
		statuses := make([]database.DeliveryStatus, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			statuses = append(statuses, database.DeliveryStatus(part))
		}
		if len(statuses) == 1 {
			s := statuses[0]
			params.Status = &s
		} else if len(statuses) > 1 {
			params.Statuses = statuses
		}
	}

	if sinceStr := c.Query("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid since parameter; expected RFC3339 timestamp")
			return
		}
		params.Since = &t
	}

	// Lazy idle-TTL sweep: abandoned claims return to the pool so the list
	// response is truthful (mirrors AI waiter GetAiConversations).
	if err := h.deliveryService.SweepStaleDeliveryClaims(businessID); err != nil {
		log.Printf("delivery claim sweep failed for business %d: %v", businessID, err)
		// non-fatal — list still serves
	}

	result, err := h.deliveryService.GetBusinessDeliveries(businessID, params)
	if err != nil {
		if errors.Is(err, services.ErrUnknownDeliveryStatusFilter) {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid status parameter")
			return
		}
		log.Printf("Failed to get deliveries for business %d: %v", businessID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	c.JSON(http.StatusOK, result)
}

// PatchDeliveryOrder applies a scoped operator edit (contact/address/
// instructions/ETA) to a non-terminal delivery. Terminal orders reject with
// 409. All fields are optional; omitted fields are left unchanged.
func (h *DeliveryHandler) PatchDeliveryOrder(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}
	if !h.requireDeliveryClaim(c, businessID, uint(deliveryID)) {
		return
	}

	var req struct {
		CustomerName          *string    `json:"customer_name"`
		CustomerPhone         *string    `json:"customer_phone"`
		CustomerEmail         *string    `json:"customer_email"`
		Street                *string    `json:"street"`
		Apartment             *string    `json:"apartment"`
		City                  *string    `json:"city"`
		State                 *string    `json:"state"`
		PostalCode            *string    `json:"postal_code"`
		Country               *string    `json:"country"`
		FormattedAddress      *string    `json:"formatted_address"`
		DeliveryInstructions  *string    `json:"delivery_instructions"`
		ContactlessDelivery   *bool      `json:"contactless_delivery"`
		LeaveAtDoor           *bool      `json:"leave_at_door"`
		EstimatedDeliveryTime *time.Time `json:"estimated_delivery_time"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	// Minimal validation: if phone is supplied it must be non-empty after trim.
	if req.CustomerPhone != nil && strings.TrimSpace(*req.CustomerPhone) == "" {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "customer_phone cannot be empty")
		return
	}
	if req.CustomerName != nil && strings.TrimSpace(*req.CustomerName) == "" {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "customer_name cannot be empty")
		return
	}

	changedBy := "operator"
	if v, exists := c.Get("address"); exists {
		if s, ok := v.(string); ok && s != "" {
			changedBy = s
		}
	}
	if v, exists := c.Get("staff_id"); exists {
		changedBy = fmt.Sprintf("staff:%v", v)
	}

	updated, err := h.deliveryService.UpdateDeliveryOrderContact(businessID, uint(deliveryID), services.DeliveryOrderContactPatch{
		CustomerName:          req.CustomerName,
		CustomerPhone:         req.CustomerPhone,
		CustomerEmail:         req.CustomerEmail,
		Street:                req.Street,
		Apartment:             req.Apartment,
		City:                  req.City,
		State:                 req.State,
		PostalCode:            req.PostalCode,
		Country:               req.Country,
		FormattedAddress:      req.FormattedAddress,
		DeliveryInstructions:  req.DeliveryInstructions,
		ContactlessDelivery:   req.ContactlessDelivery,
		LeaveAtDoor:           req.LeaveAtDoor,
		EstimatedDeliveryTime: req.EstimatedDeliveryTime,
		ChangedBy:             changedBy,
	})
	if err != nil {
		if errors.Is(err, services.ErrDeliveryOrderNotEditable) {
			serverhelpers.RespondWithError(c, http.StatusConflict, serverhelpers.ErrCodeInvalidInput, "Delivery is terminal and cannot be edited")
			return
		}
		if strings.Contains(err.Error(), "not found") {
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "Delivery order not found")
			return
		}
		log.Printf("Failed to patch delivery (ID: %d): %v", deliveryID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	_ = h.deliveryService.TouchDeliveryClaim(businessID, uint(deliveryID))
	events.GetHub().PublishJSON(businessID, "delivery.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// UpdateDeliveryStatus updates the status of a delivery order
func (h *DeliveryHandler) UpdateDeliveryStatus(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}
	if !h.requireDeliveryClaim(c, businessID, uint(deliveryID)) {
		return
	}

	var req struct {
		Status   database.DeliveryStatus `json:"status" binding:"required"`
		Location *database.Location      `json:"location"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	changedBy := deliveryActor(c)

	if err := h.deliveryService.UpdateDeliveryStatusByBusiness(businessID, uint(deliveryID), req.Status, req.Location, changedBy); err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidDeliveryStatus):
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery status")
		case errors.Is(err, services.ErrDeliveryStatusTerminal):
			// The delivery is delivered/cancelled/failed — a final state that can't
			// change. 409 Conflict distinguishes "already done" from a bad request.
			serverhelpers.RespondWithError(c, http.StatusConflict, serverhelpers.ErrCodeConflict, "Delivery is already in a final state and cannot change")
		case errors.Is(err, services.ErrInvalidDeliveryTransition):
			// The requested edge is not allowed by the delivery state machine
			// (backward move, or an advance that would bypass payment). 400 with a
			// distinct message so the operator sees why the move was rejected.
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "That status change is not allowed for this delivery")
		default:
			log.Printf("Failed to update delivery status (ID: %d): %v", deliveryID, err)
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	_ = h.deliveryService.TouchDeliveryClaim(businessID, uint(deliveryID))
	// cancelled/failed route through the lifecycle helper, which already
	// publishes delivery.cancelled (payload status carries failed vs
	// cancelled) — publishing again here duplicated toasts. The operational
	// alert still needs resolving.
	if req.Status == database.DeliveryStatusCancelled || req.Status == database.DeliveryStatusFailed {
		h.resolveDeliveryAlert(c, businessID, uint(deliveryID))
	} else {
		h.publishDeliveryEvent(c, businessID, uint(deliveryID), "delivery.updated")
	}
	c.JSON(http.StatusOK, gin.H{"message": "Delivery status updated successfully"})
}

// AssignDriver assigns a driver to a delivery order
func (h *DeliveryHandler) AssignDriver(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}
	if !h.requireDeliveryClaim(c, businessID, uint(deliveryID)) {
		return
	}

	var req struct {
		DriverID uint `json:"driver_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	if err := h.deliveryService.AssignDriverByBusiness(businessID, uint(deliveryID), req.DriverID); err != nil {
		switch {
		case errors.Is(err, services.ErrDeliveryAlreadyAssigned):
			serverhelpers.RespondWithError(c, http.StatusConflict, serverhelpers.ErrCodeConflict, "Delivery already has an assigned driver")
		case errors.Is(err, services.ErrDeliveryStatusTerminal):
			serverhelpers.RespondWithError(c, http.StatusConflict, serverhelpers.ErrCodeConflict, "Delivery is already in a final state and cannot change")
		case errors.Is(err, services.ErrInvalidDeliveryTransition):
			// Same state-machine rejections as UpdateDeliveryStatus (confirmed
			// unpaid prepay, pending+online skip-Accept, backward edges).
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "That status change is not allowed for this delivery")
		default:
			log.Printf("Failed to assign driver to delivery %d: %v", deliveryID, err)
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	_ = h.deliveryService.TouchDeliveryClaim(businessID, uint(deliveryID))
	h.publishDeliveryEvent(c, businessID, uint(deliveryID), "delivery.updated")
	c.JSON(http.StatusOK, gin.H{"message": "Driver assigned successfully"})
}

// CancelDeliveryOrder cancels a delivery order
func (h *DeliveryHandler) CancelDeliveryOrder(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}
	if !h.requireDeliveryClaim(c, businessID, uint(deliveryID)) {
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	if err := h.deliveryService.CancelDeliveryOrderByBusiness(businessID, uint(deliveryID), req.Reason, deliveryActor(c)); err != nil {
		switch {
		case errors.Is(err, services.ErrDeliveryStatusTerminal):
			// Cancelling an already-terminal delivery is a conflict, not a server
			// error. (The lifecycle path is idempotent for already-cancelled, but a
			// delivered/failed order surfaces this sentinel.)
			serverhelpers.RespondWithError(c, http.StatusConflict, serverhelpers.ErrCodeConflict, "Delivery is already in a final state and cannot be cancelled")
		case errors.Is(err, services.ErrInvalidDeliveryTransition):
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "This delivery cannot be cancelled")
		default:
			log.Printf("Failed to cancel delivery order %d: %v", deliveryID, err)
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	// The lifecycle helper (cancelDeliveryLinked) already published
	// delivery.cancelled at the state change — emitting again here doubled
	// the event. Only the operational alert remains for the handler.
	h.resolveDeliveryAlert(c, businessID, uint(deliveryID))
	c.JSON(http.StatusOK, gin.H{"message": "Delivery order cancelled successfully"})
}

// ClaimDeliveryOrder assigns the delivery to the acting operator. Pass
// {"steal":true} to take over a fresh claim (managers/owners only); that path
// writes an audit row and notifies the previous holder.
// POST /api/v1/inside/businesses/:id/deliveries/:delivery_id/claim
func (h *DeliveryHandler) ClaimDeliveryOrder(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}
	var req struct {
		Steal bool `json:"steal"`
	}
	// Empty body is fine (plain claim).
	_ = c.ShouldBindJSON(&req)

	actor := deliveryClaimActor(c)
	order, prevStaffID, err := h.deliveryService.ClaimDeliveryOrder(businessID, uint(deliveryID), actor, req.Steal)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrDeliveryClaimStealRequired):
			// Surface who holds it so the UI can prompt for an explicit steal.
			var current database.DeliveryOrder
			_ = database.GetDB().Select("claimed_by_staff_id", "claimed_by_name", "claimed_by_role").
				Where("id = ? AND business_id = ?", deliveryID, businessID).First(&current).Error
			c.JSON(http.StatusConflict, gin.H{
				"error":               "This delivery is claimed by another operator — confirm steal to take over",
				"code":                "claim_steal_required",
				"claimed_by_staff_id": current.ClaimedByStaffID,
				"claimed_by_name":     current.ClaimedByName,
				"claimed_by_role":     current.ClaimedByRole,
			})
		case errors.Is(err, services.ErrDeliveryClaimForbidden):
			if respondDeliveryClaimHeld(c, err) {
				return
			}
			// Fall through with a held-shaped response when not typed.
			c.JSON(http.StatusConflict, gin.H{
				"error": "You cannot take a claim held by another operator",
				"code":  "claim_forbidden",
			})
		case respondDeliveryClaimHeld(c, err):
			return
		case strings.Contains(err.Error(), "not found"):
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "Delivery order not found")
		default:
			log.Printf("Failed to claim delivery %d: %v", deliveryID, err)
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	if prevStaffID != nil && *prevStaffID != 0 {
		resource := "delivery"
		if order != nil && order.DeliveryNumber != "" {
			resource = "delivery " + order.DeliveryNumber
		}
		notifyStaff(database.GetDBWrapper(), businessID, []uint{*prevStaffID}, "claim.stolen",
			services.PushKeyClaimStolen,
			services.PushArgs{StealerName: actor.Name, Resource: resource},
			fmt.Sprintf("/business/%d/delivery", businessID),
		)
	}

	events.GetHub().PublishJSON(businessID, "delivery.updated", order)
	c.JSON(http.StatusOK, gin.H{
		"claimed_by_staff_id": order.ClaimedByStaffID,
		"claimed_by_name":     order.ClaimedByName,
		"claimed_by_role":     order.ClaimedByRole,
		"claimed_at":          order.ClaimedAt,
	})
}

// ReleaseDeliveryOrder clears the dispatch claim.
// POST /api/v1/inside/businesses/:id/deliveries/:delivery_id/release
func (h *DeliveryHandler) ReleaseDeliveryOrder(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid delivery ID")
		return
	}
	if err := h.deliveryService.ReleaseDeliveryOrder(businessID, uint(deliveryID), deliveryClaimActor(c)); err != nil {
		if respondDeliveryClaimHeld(c, err) {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "Delivery order not found")
			return
		}
		log.Printf("Failed to release delivery claim %d: %v", deliveryID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}
	if order, err := h.deliveryService.GetDeliveryOrderByBusiness(businessID, uint(deliveryID)); err == nil {
		events.GetHub().PublishJSON(businessID, "delivery.updated", order)
	}
	c.Status(http.StatusOK)
}

func (h *DeliveryHandler) publishDeliveryEvent(c *gin.Context, businessID, deliveryID uint, eventType string) {
	deliveryOrder, err := h.deliveryService.GetDeliveryOrderByBusiness(businessID, deliveryID)
	if err != nil {
		log.Printf("Failed to load delivery %d for %s event: %v", deliveryID, eventType, err)
		return
	}
	events.GetHub().PublishJSON(deliveryOrder.BusinessID, eventType, deliveryOrder)
	if deliveryStatusAddressesAlert(deliveryOrder.Status) {
		h.resolveDeliveryAlertForOrder(c, deliveryOrder)
	}
}

// resolveDeliveryAlert resolves the delivery's operational alert without
// publishing any SSE event — used on paths where the delivery lifecycle
// helper already emitted the state-change event (cancel/failed).
func (h *DeliveryHandler) resolveDeliveryAlert(c *gin.Context, businessID, deliveryID uint) {
	deliveryOrder, err := h.deliveryService.GetDeliveryOrderByBusiness(businessID, deliveryID)
	if err != nil {
		log.Printf("Failed to load delivery %d for alert resolution: %v", deliveryID, err)
		return
	}
	h.resolveDeliveryAlertForOrder(c, deliveryOrder)
}

func (h *DeliveryHandler) resolveDeliveryAlertForOrder(c *gin.Context, deliveryOrder *database.DeliveryOrder) {
	if err := operational_alerts.NewService(database.GetDB()).ResolveAlertForResource(
		c.Request.Context(),
		deliveryOrder.BusinessID,
		database.OperationalAlertResourceTypeDelivery,
		deliveryOrder.ID,
		deliveryOperationalAlertActor(c),
		string(deliveryOrder.Status),
	); err != nil {
		log.Printf("failed to resolve delivery operational alert: business_id=%d delivery_id=%d status=%s error=%v", deliveryOrder.BusinessID, deliveryOrder.ID, deliveryOrder.Status, err)
	}
}

// Driver Management Handlers

// createDriverInput is the explicit allow-list of fields a caller may set when
// creating a driver. Using a dedicated struct instead of the full
// database.DeliveryDriver prevents mass-assignment of protected server-managed
// fields (Status, CurrentDeliveryID, TotalDeliveries, CompletedDeliveries,
// AverageRating, TotalEarnings, IsActive, LastActiveAt).
type createDriverInput struct {
	Name          string               `json:"name" binding:"required"`
	Phone         string               `json:"phone" binding:"required"`
	Email         string               `json:"email"`
	LicenseNumber string               `json:"license_number"`
	VehicleType   database.VehicleType `json:"vehicle_type"`
	VehiclePlate  string               `json:"vehicle_plate"`
	IsAvailable   bool                 `json:"is_available"`
	// StaffID links the driver to a staff member. Validated server-side to
	// ensure the staff record belongs to the same business (cross-tenant guard).
	StaffID *uint `json:"staff_id"`
}

// CreateDriver creates a new delivery driver
func (h *DeliveryHandler) CreateDriver(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	var input createDriverInput
	if err := c.ShouldBindJSON(&input); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	if !isValidDriverPhone(input.Phone) {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "invalid phone")
		return
	}

	if !input.VehicleType.IsValid() {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "invalid vehicle_type")
		return
	}

	// Cross-tenant guard: if a staff_id is supplied, verify the staff record
	// belongs to this business before associating it. A staff_id from a
	// different business would otherwise preload that business's staff PII in
	// the response (disclosure via the Staff relationship).
	if input.StaffID != nil {
		var count int64
		if err := database.GetDB().Model(&database.Staff{}).
			Where("id = ? AND business_id = ?", *input.StaffID, businessID).
			Count(&count).Error; err != nil || count == 0 {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "staff not found for this business")
			return
		}
	}

	driver := database.DeliveryDriver{
		BusinessID:    businessID,
		Name:          input.Name,
		Phone:         input.Phone,
		Email:         input.Email,
		LicenseNumber: input.LicenseNumber,
		VehicleType:   input.VehicleType,
		VehiclePlate:  input.VehiclePlate,
		IsAvailable:   input.IsAvailable,
		StaffID:       input.StaffID,
	}

	if err := h.deliveryService.CreateDriver(&driver); err != nil {
		log.Printf("Failed to create driver for business %d: %v", businessID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	c.JSON(http.StatusCreated, driver)
}

// GetBusinessDrivers retrieves all drivers for a business
func (h *DeliveryHandler) GetBusinessDrivers(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	drivers, err := h.deliveryService.GetBusinessDrivers(businessID)
	if err != nil {
		log.Printf("Failed to get drivers for business %d: %v", businessID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	c.JSON(http.StatusOK, drivers)
}

// ListDriverPerformance returns the per-driver scorecard leaderboard for a business.
func (h *DeliveryHandler) ListDriverPerformance(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	performance, err := h.deliveryService.ListDriverPerformance(businessID)
	if err != nil {
		log.Printf("Failed to list driver performance for business %d: %v", businessID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load driver performance")
		return
	}

	c.JSON(http.StatusOK, gin.H{"drivers": performance})
}

// GetDriverPerformance returns the scorecard for a single driver, scoped to the business.
func (h *DeliveryHandler) GetDriverPerformance(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}
	driverID, err := strconv.ParseUint(c.Param("driver_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid driver ID")
		return
	}

	dto, err := h.deliveryService.GetDriverPerformance(businessID, uint(driverID))
	if err != nil {
		if strings.Contains(err.Error(), "driver not found") {
			serverhelpers.RespondWithError(c, http.StatusNotFound, "", "Driver not found")
			return
		}
		log.Printf("Failed to load driver %d performance: %v", driverID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load driver performance")
		return
	}

	c.JSON(http.StatusOK, dto)
}

// GetAvailableDrivers retrieves available drivers for a business
func (h *DeliveryHandler) GetAvailableDrivers(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	drivers, err := h.deliveryService.GetAvailableDrivers(businessID)
	if err != nil {
		log.Printf("Failed to get available drivers for business %d: %v", businessID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	c.JSON(http.StatusOK, drivers)
}

// UpdateDriver updates a driver's information
func (h *DeliveryHandler) UpdateDriver(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	driverID, err := strconv.ParseUint(c.Param("driver_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid driver ID")
		return
	}

	// Pointer fields give partial-update semantics: absent fields are left
	// untouched, while present-but-empty strings are applied as-is (e.g.
	// clearing vehicle_plate). The edit form sends every field; the
	// availability toggle sends only is_available.
	var updates struct {
		Name          *string `json:"name"`
		Phone         *string `json:"phone"`
		Email         *string `json:"email"`
		LicenseNumber *string `json:"license_number"`
		VehicleType   *string `json:"vehicle_type"`
		VehiclePlate  *string `json:"vehicle_plate"`
		IsAvailable   *bool   `json:"is_available"`
	}
	if err := c.ShouldBindJSON(&updates); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	// Build the map from present fields only — GORM's map-based Updates
	// applies empty strings as-is (struct-based Updates would skip them).
	updateMap := map[string]interface{}{}
	if updates.Name != nil {
		if strings.TrimSpace(*updates.Name) == "" {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "name is required")
			return
		}
		updateMap["name"] = *updates.Name
	}
	if updates.Phone != nil {
		if strings.TrimSpace(*updates.Phone) == "" {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "phone is required")
			return
		}
		if !isValidDriverPhone(*updates.Phone) {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "invalid phone")
			return
		}
		updateMap["phone"] = *updates.Phone
	}
	if updates.Email != nil {
		updateMap["email"] = *updates.Email
	}
	if updates.LicenseNumber != nil {
		updateMap["license_number"] = *updates.LicenseNumber
	}
	if updates.VehicleType != nil {
		if !database.VehicleType(*updates.VehicleType).IsValid() {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "invalid vehicle_type")
			return
		}
		updateMap["vehicle_type"] = *updates.VehicleType
	}
	if updates.VehiclePlate != nil {
		updateMap["vehicle_plate"] = *updates.VehiclePlate
	}
	if updates.IsAvailable != nil {
		updateMap["is_available"] = *updates.IsAvailable
		// Keep the driver's `status` column coherent with the availability toggle
		// so the roster chip doesn't show "offline" for a driver who is in fact
		// assignable. Assignment itself keys off is_available (see
		// GetAvailableDrivers/AssignDriver), so this is display coherence only.
		// A driver currently carrying a delivery stays "busy" — never demote a
		// mid-delivery driver to online/offline. The conditional WHERE (status !=
		// busy) is applied at the service layer.
		if *updates.IsAvailable {
			updateMap["status"] = database.DriverStatusOnline
		} else {
			updateMap["status"] = database.DriverStatusOffline
		}
	}
	if len(updateMap) == 0 {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "no fields to update")
		return
	}

	if err := h.deliveryService.UpdateDriverByBusiness(businessID, uint(driverID), updateMap); err != nil {
		log.Printf("Failed to update driver %d: %v", driverID, err)
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeNotFound, "Driver not found")
			return
		}
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	driver, err := h.deliveryService.GetDriverByBusiness(businessID, uint(driverID))
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load updated driver")
		return
	}
	c.JSON(http.StatusOK, driver)
}

// DeleteDriver deletes a driver
func (h *DeliveryHandler) DeleteDriver(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	driverID, err := strconv.ParseUint(c.Param("driver_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid driver ID")
		return
	}

	if err := h.deliveryService.DeleteDriverByBusiness(businessID, uint(driverID)); err != nil {
		log.Printf("Failed to delete driver %d: %v", driverID, err)
		switch {
		case strings.Contains(strings.ToLower(err.Error()), "not found"):
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeNotFound, "Driver not found")
		case strings.Contains(strings.ToLower(err.Error()), "active delivery"):
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Cannot delete driver with active delivery")
		default:
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Driver deleted successfully"})
}

// NOTE (DEL-ROUTE-4): the never-routed "Driver App" handlers
// (UpdateDriverLocation / UpdateDriverStatus / GetDriverActiveDelivery) were
// deleted — they authorized purely on the raw :driver_id path param with no
// business scoping, a latent cross-tenant IDOR had they ever been wired. The
// UpdateDriverLocation service method was deleted with them (dead once its
// handler was gone); UpdateDriverStatus / GetDriverActiveDelivery remain for a
// future driver-app slice. Any new handlers must scope through the business
// (GetDriverByBusiness pattern) plus a driver-identity check before being
// routed.

// Delivery Settings Handlers

// GetDeliverySettings retrieves delivery settings for a business
func (h *DeliveryHandler) GetDeliverySettings(c *gin.Context) {
	// Try both parameter names (business_id for public routes, id for protected routes)
	businessIDStr := c.Param("business_id")
	isPublicRoute := businessIDStr != ""
	if businessIDStr == "" {
		businessIDStr = c.Param("id")
	}

	var businessID uint
	if !isPublicRoute {
		var ok bool
		businessID, ok = deliveryBusinessIDFromRoute(c)
		if !ok {
			return
		}
	} else {
		parsedBusinessID, err := strconv.ParseUint(businessIDStr, 10, 32)
		if err != nil {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid business ID")
			return
		}
		businessID = uint(parsedBusinessID)
	}

	settings, err := h.deliveryService.GetDeliverySettingsDTO(businessID, isPublicRoute)
	if err != nil {
		log.Printf("Failed to get delivery settings for business %d: %v", businessID, err)
		if errors.Is(err, services.ErrDeliveryScopedNotFound) {
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "business not found")
			return
		}
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateDeliverySettings updates delivery settings for a business
func (h *DeliveryHandler) UpdateDeliverySettings(c *gin.Context) {
	businessID, ok := deliveryBusinessIDFromRoute(c)
	if !ok {
		return
	}

	var input services.UpdateDeliverySettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	if input.ExternalPartnerLinks != nil {
		// #829: delivery partners are couriers — reservation platforms
		// (opentable, resy) are rejected here but stay valid for reservations.
		normalizedLinks, err := serverhelpers.NormalizeAndValidateExternalPartnerLinksForKind(*input.ExternalPartnerLinks, serverhelpers.ExternalPartnerKindDelivery)
		if err != nil {
			serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid external partner links")
			return
		}
		input.ExternalPartnerLinks = &normalizedLinks
		// Public demo: partner CTAs render on the storefront. Other delivery
		// fields stay editable, and a form that re-sends the stored links passes.
		if config.DemoModeEnabled() {
			current, loadErr := h.deliveryService.GetDeliverySettings(businessID)
			if loadErr != nil {
				log.Printf("Failed to load delivery settings for business %d: %v", businessID, loadErr)
				serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
				return
			}
			kind := serverhelpers.ExternalPartnerKindDelivery
			changed, cmpErr := serverhelpers.ExternalPartnerLinksChanged([]byte(current.ExternalPartnerLinks), normalizedLinks, &kind)
			if cmpErr != nil {
				log.Printf("demo delivery partner link compare failed for business %d: %v", businessID, cmpErr)
			}
			if cmpErr != nil || changed {
				demomode.Refuse(c, demomode.KindStorefront, "changing the venue's delivery partner links")
				return
			}
		}
	}

	settings, err := h.deliveryService.UpdateDeliverySettingsDTO(businessID, input)
	if err != nil {
		log.Printf("Failed to update delivery settings for business %d: %v", businessID, err)
		switch {
		case errors.Is(err, services.ErrDeliveryScopedNotFound):
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "business not found")
		case errors.Is(err, services.ErrDeliveryValidation):
			serverhelpers.RespondWithError(c, http.StatusBadRequest, deliveryValidationCode(err), deliveryCreateErrorMessage(err))
		case isDeliverySettingsValidationMessage(err):
			// validateDeliverySettings returns bare (non-sentinel) errors whose
			// text is operator-facing; forward the known-safe shapes as 400 with
			// stable codes for FE localization.
			serverhelpers.RespondWithError(c, http.StatusBadRequest, deliveryValidationCode(err), err.Error())
		default:
			// Non-sentinel errors may contain raw DB/GORM detail — log server-side
			// (above) and return a generic message so internal state never
			// reaches the caller (DEL-LEAK-6).
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	c.JSON(http.StatusOK, settings)
}

// requirePublicDeliveryAcceptingOrders enforces the public delivery storefront
// visibility gate plus the administrator lifecycle lock used by guest dine-in
// order/bill creation. Suspended/hidden businesses stay 404 (do not leak that
// the venue exists); closed businesses return 403 business_unavailable so
// guests cannot create delivery traffic after closure.
func requirePublicDeliveryAcceptingOrders(c *gin.Context, businessID uint) bool {
	business, err := database.GetBusinessByID(businessID)
	if err != nil || business == nil {
		serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "business not found")
		return false
	}
	if !business.IsActive || !business.BusinessPageEnabled {
		serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "business not found")
		return false
	}
	if !database.IsBusinessOperational(business) {
		// Mirror CreateGuestOrder / guest bill create: guest-facing copy + stable code.
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return false
	}
	return true
}

func (h *DeliveryHandler) QuoteDelivery(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("business_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	if !requirePublicDeliveryAcceptingOrders(c, uint(businessID)) {
		return
	}

	var request services.DeliveryQuoteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}

	quote, err := h.deliveryService.QuoteDelivery(uint(businessID), request)
	if err != nil {
		if errors.Is(err, services.ErrDeliveryScopedNotFound) {
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "business not found")
			return
		}
		// Non-sentinel errors may contain raw DB/GORM detail — log server-side
		// and return a generic message so internal state never reaches the caller.
		log.Printf("QuoteDelivery: business_id=%d error=%v", businessID, err)
		serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		return
	}

	c.JSON(http.StatusOK, quote)
}

func (h *DeliveryHandler) GuestDeliveryCheckout(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("business_id"), 10, 32)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	if !requirePublicDeliveryAcceptingOrders(c, uint(businessID)) {
		return
	}

	var request services.GuestDeliveryCheckoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		serverhelpers.RespondBindError(c, err)
		return
	}
	// Idempotency key from the guest client's retry-safe header. Same id ⇒
	// replay of the original checkout instead of a second order.
	request.ClientRequestID = strings.TrimSpace(c.GetHeader("X-Request-Id"))

	checkout, err := h.deliveryService.GuestDeliveryCheckout(uint(businessID), request)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrDeliveryScopedNotFound):
			serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "business not found")
		case errors.Is(err, services.ErrDeliveryValidation):
			// User-facing validation error: strip the sentinel prefix and return
			// the safe human-readable message + stable code for FE localization.
			serverhelpers.RespondWithError(c, http.StatusBadRequest, deliveryValidationCode(err), deliveryCreateErrorMessage(err))
		default:
			// Non-sentinel errors may contain raw DB/GORM detail — log server-side
			// and return a generic message so internal state never reaches the caller.
			log.Printf("GuestDeliveryCheckout: business_id=%d error=%v", businessID, err)
			serverhelpers.RespondWithError(c, http.StatusInternalServerError, "", "Failed to process delivery operation")
		}
		return
	}

	if checkout.Duplicate {
		// Replay of an already-committed checkout: no new rows were created,
		// so no events/alerts/notifications fire again. 200 (not 201) + the
		// duplicate flag mirrors the dine-in replay contract (B-9).
		c.JSON(http.StatusOK, checkout)
		return
	}

	delivery := checkout.DeliveryOrder
	events.GetHub().PublishJSON(delivery.BusinessID, "delivery.created", delivery)
	if checkout.Order != nil {
		if herr := database.HydrateOrderSnapshotForWire(checkout.Order); herr != nil {
			log.Printf("delivery order.created snapshot hydrate failed: order_id=%d error=%v", checkout.Order.ID, herr)
			checkout.Order.Items = ""
		}
		events.GetHub().PublishJSON(delivery.BusinessID, "order.created", checkout.Order)
	}
	if err := operational_alerts.NewService(database.GetDB()).CreateDeliveryNewAlert(c.Request.Context(), *delivery); err != nil {
		log.Printf("failed to create delivery operational alert: business_id=%d delivery_id=%d error=%v", delivery.BusinessID, delivery.ID, err)
	}
	// X-6: delivery orders get the same operator notifications as dine-in.
	notifyGuestDeliveryOrderCreated(checkout)

	c.JSON(http.StatusCreated, checkout)
}

// notifyGuestDeliveryOrderCreated mirrors the dine-in order.created web-push
// side effect for guest delivery checkouts (X-6). Called after the checkout
// transaction has committed; every failure is logged, never fatal.
//
// DELIV-NOTIF-2 (resolved): the Telegram order.created outbox row is no longer
// enqueued here. services.GuestDeliveryCheckout writes it INSIDE the checkout
// transaction (transactional outbox — see enqueueDeliveryTelegramOrderCreatedTx
// in delivery_v1.go), so a crash after the commit can never lose the operator
// notification, and the ON CONFLICT (business_id, plugin_name, event_type,
// event_id) dedup keyed on "order:<id>" guarantees at most one send.
func notifyGuestDeliveryOrderCreated(checkout *services.GuestDeliveryCheckoutDTO) {
	if checkout == nil || checkout.Order == nil {
		return
	}
	order := *checkout.Order

	if wp := serverhelpers.GetWebPushService(); wp != nil {
		logger.SafeGo(func() {
			if err := wp.SendLocalizedPushToBusiness(order.BusinessID, services.PushKeyNewDeliveryOrder,
				services.PushArgs{OrderNumber: order.OrderNumber}, ""); err != nil {
				log.Printf("delivery checkout web push failed: business_id=%d order_id=%d error=%v",
					order.BusinessID, order.ID, err)
			}
		})
	}
}

// Public Handlers (for guests/customers)

// TrackDelivery allows customers to track their delivery by delivery number
func (h *DeliveryHandler) TrackDelivery(c *gin.Context) {
	deliveryNumber := c.Param("delivery_number")
	if deliveryNumber == "" {
		serverhelpers.RespondWithError(c, http.StatusBadRequest, serverhelpers.ErrCodeInvalidInput, "Delivery number is required")
		return
	}

	deliveryOrder, err := h.deliveryService.GetDeliveryOrderByNumber(deliveryNumber)
	if err != nil {
		serverhelpers.RespondWithError(c, http.StatusNotFound, serverhelpers.ErrCodeBusinessNotFound, "Delivery not found")
		return
	}

	// Load bill summary for payment-state fields (best-effort; not fatal).
	// settlement_addr and tipping_addr are included so the guest pay page can
	// pass them to PaymentSection for crypto payment flows.
	// public_token is the bill's guest capability (/guest/bill/:bill_token). The
	// guest pay page needs it to mount PaymentSection, but only while the order
	// is awaiting online payment; it is withheld otherwise (M-track) so a
	// leaked or guessed delivery number does not hand out a long-lived bill
	// capability for the whole life of the order.
	var bill database.Bill
	billLoaded := false
	if err := database.GetDB().Select("id", "bill_number", "public_token", "status", "total_amount", "paid_amount", "settlement_addr", "tipping_addr").
		First(&bill, deliveryOrder.BillID).Error; err == nil {
		billLoaded = true
	}

	// Derive the RESOLVED payment mode, not the raw stored value. The stored
	// PaymentMode is "" for businesses that never explicitly saved one;
	// EffectiveDeliveryPaymentMode resolves "" via online-payment availability
	// so the effective mode is always "online" or "cash_on_delivery". Using the
	// raw stored "" causes awaitingPayment to be false on prepay orders, hiding
	// the Pay button and letting them expire. The narrow resolver (projected
	// settings + settlement-address reads) keeps this polled public endpoint
	// off the full settings DTO (business aggregate + zones). Skipped for
	// terminal deliveries: terminal cards never render payment UI.
	paymentMode := ""
	if !deliveryOrder.Status.IsTerminal() {
		paymentMode = h.deliveryService.EffectiveDeliveryPaymentMode(deliveryOrder.BusinessID)
	}

	// awaitingPayment: the guest must pay online and the bill is not yet paid.
	awaitingPayment := deliveryOrder.Status == database.DeliveryStatusConfirmed &&
		paymentMode == string(database.DeliveryPaymentOnline) &&
		billLoaded && bill.Status != database.BillStatusPaid

	// Return limited information for public tracking
	driverInfo := gin.H{
		"name":  "",
		"phone": "",
	}
	if deliveryOrder.Driver != nil {
		driverInfo["name"] = deliveryOrder.Driver.Name
		driverInfo["phone"] = deliveryOrder.Driver.Phone
	}

	var billSummary gin.H
	if billLoaded {
		billSummary = gin.H{
			"id":                 bill.ID,
			"bill_number":        bill.BillNumber,
			"status":             bill.Status,
			"total_amount":       float64(bill.TotalAmount) / 100.0,
			"paid_amount":        float64(bill.PaidAmount) / 100.0,
			"paid":               bill.Status == database.BillStatusPaid,
			"settlement_address": bill.SettlementAddr,
			"tipping_address":    bill.TippingAddr,
		}
		if awaitingPayment {
			billSummary["public_token"] = bill.PublicToken
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"delivery_number":         deliveryOrder.DeliveryNumber,
		"status":                  deliveryOrder.Status,
		"estimated_delivery_time": deliveryOrder.EstimatedDeliveryTime,
		"current_location":        deliveryOrder.CurrentLocation,
		"driver":                  driverInfo,
		"business_name":           deliveryOrder.Business.Name,
		"business_custom_url":     deliveryOrder.Business.CustomURL,
		"external_tracking_url":   deliveryOrder.ExternalTrackingURL,
		"updated_at":              deliveryOrder.UpdatedAt,
		// Payment-state fields for the guest tracking and pay pages.
		"business_id":         deliveryOrder.BusinessID,
		"payment_mode":        paymentMode,
		"awaiting_payment":    awaitingPayment,
		"payment_expires_at":  deliveryOrder.PaymentExpiresAt,
		"cancellation_reason": deliveryOrder.CancellationReason,
		"bill":                billSummary,
		// Currency fields consumed by PaymentSection on the guest pay page.
		"default_currency": deliveryOrder.Business.DefaultCurrency,
		"display_currency": deliveryOrder.Business.DisplayCurrency,
		// Venue IANA TZ for ETA / timestamp formatting on the guest track page.
		"timezone":               deliveryOrder.Business.Timezone,
		"external_partner_links": h.deliveryService.PublicPartnerLinks(deliveryOrder.BusinessID),
	})
}
