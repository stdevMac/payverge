package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type CoverageHandler struct{ db *database.DB }

func NewCoverageHandler(db *database.DB) *CoverageHandler { return &CoverageHandler{db: db} }

func itoa(u uint) string { return strconv.FormatUint(uint64(u), 10) }

// callerStaffID returns the acting staff id (0 for an owner authenticating by
// wallet, who has no staff row).
func callerStaffID(c *gin.Context) uint {
	// Reuse the canonical context reader (handles uint/uint64/int/int64); an
	// owner with no staff row resolves to 0, which coverage intentionally allows
	// (audited via callerActor).
	id, _ := staffIDFromContext(c)
	return id
}

// callerActor is the audit/SSE actor label.
func callerActor(c *gin.Context) string {
	if id := callerStaffID(c); id != 0 {
		return "staff:" + itoa(id)
	}
	if a, ok := c.Get("address"); ok {
		if s, ok := a.(string); ok {
			return "owner:" + s
		}
	}
	return "system"
}

// callerCanApprove is true for business owners (wallet or email/password),
// managers, or a custom schedule:approve grant — mirrors the default holders of
// schedule:approve.
//
// Hybrid auth admits owners on two token_type values:
//   - "web3" — wallet SIWE session
//   - "user" — email/password (or OAuth) owner session, after business.UserID match
//
// Only checking "web3" left email owners 403'd on coverage history / approve
// paths while staff managers still worked (local admin@local.test).
func callerCanApprove(c *gin.Context) bool {
	if t, _ := c.Get("token_type"); t == "web3" || t == "user" {
		return true // business owner (wallet or email)
	}
	if r, _ := c.Get("staff_role"); r == "manager" {
		return true
	}
	if raw, ok := c.Get("staff_custom_permissions"); ok {
		if s, _ := raw.(string); s != "" {
			var perms []string
			if json.Unmarshal([]byte(s), &perms) == nil {
				for _, p := range perms {
					if p == "schedule:approve" {
						return true
					}
				}
			}
		}
	}
	return false
}

// coverageError maps service sentinels to HTTP responses.
func coverageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrCoverageNotFound):
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Coverage record not found")
	case errors.Is(err, database.ErrShiftNotOpen):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Shift is not open")
	case errors.Is(err, database.ErrNotEligible):
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "You are not eligible for this position")
	case errors.Is(err, database.ErrSelfCoverage):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot cover your own shift")
	case errors.Is(err, database.ErrAlreadyClaimed):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "You already claimed this shift")
	case errors.Is(err, database.ErrNotOwner):
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "You can only cancel your own request")
	case errors.Is(err, database.ErrCoverageConflict):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "That request changed status — refresh and retry")
	case errors.Is(err, database.ErrCoverageExpired):
		server.RespondWithError(c, http.StatusConflict, "coverage_expired", "Coverage request has expired")
	default:
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Coverage operation failed")
	}
}

func (h *CoverageHandler) ClaimOpenShift(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	shiftID, ok := parseParamUint(c, "shiftId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can claim shifts")
		return
	}
	claim, err := h.db.ClaimOpenShift(businessID, shiftID, staffID, callerActor(c))
	if err != nil {
		coverageError(c, err)
		return
	}
	events.GetHub().PublishJSON(businessID, "openshift.claimed", gin.H{"claim_id": claim.ID, "shift_id": shiftID, "claiming_staff_id": staffID})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": claim})
}

type swapRequestDTO struct {
	Kind          string `json:"kind"`
	Target        string `json:"target"`
	TargetStaffID *uint  `json:"target_staff_id"`
}

func (h *CoverageHandler) RequestSwap(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	shiftID, ok := parseParamUint(c, "shiftId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can request swaps")
		return
	}
	var in swapRequestDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.Kind != database.SwapKindSwap && in.Kind != database.SwapKindGiveup {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "kind must be swap or giveup")
		return
	}
	if in.Target == database.SwapTargetSpecific && in.TargetStaffID == nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "target_staff_id required for specific target")
		return
	}
	sw, err := h.db.RequestSwap(businessID, shiftID, staffID, in.Kind, in.Target, in.TargetStaffID, callerActor(c))
	if err != nil {
		coverageError(c, err)
		return
	}
	events.GetHub().PublishJSON(businessID, "shift.swap.requested", gin.H{"swap_id": sw.ID, "shift_id": shiftID, "kind": sw.Kind, "status": sw.Status})
	var offerTargets []uint
	if in.Target == database.SwapTargetSpecific && in.TargetStaffID != nil {
		offerTargets = []uint{*in.TargetStaffID}
	} else {
		offerTargets, _ = h.db.ListEligibleStaffIDsForShift(businessID, shiftID)
	}
	notifyStaff(h.db, businessID, excludeStaffID(offerTargets, staffID), "shift.swap.requested",
		services.PushKeyCoverageOffer, services.PushArgs{}, "/staff/home?tab=coverage")
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": sw})
}

func (h *CoverageHandler) AcceptSwap(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	swapID, ok := parseParamUint(c, "swapId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can accept swaps")
		return
	}
	sw, err := h.db.AcceptSwap(businessID, swapID, staffID, callerActor(c))
	if err != nil {
		coverageError(c, err)
		return
	}
	events.GetHub().PublishJSON(businessID, "shift.swap.accepted", gin.H{"swap_id": sw.ID, "shift_id": sw.ShiftID, "status": sw.Status})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sw})
}

type decisionDTO struct {
	Kind     string `json:"kind"`     // swap | open_claim
	Decision string `json:"decision"` // approve | deny
}

// Decide is the polymorphic coverage-approval endpoint (locked route
// /swaps/:swapId/decision). The body `kind` selects swap vs open_claim; :swapId
// is the swap id or claim id respectively.
func (h *CoverageHandler) Decide(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	reqID, ok := parseParamUint(c, "swapId")
	if !ok {
		return
	}
	var in decisionDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.Decision != "approve" && in.Decision != "deny" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "decision must be approve or deny")
		return
	}
	approve := in.Decision == "approve"
	decider := callerStaffID(c) // 0 for owner; audited via callerActor
	switch in.Kind {
	case database.SwapKindSwap:
		sw, err := h.db.DecideSwap(businessID, reqID, decider, approve, callerActor(c))
		if err != nil {
			coverageError(c, err)
			return
		}
		events.GetHub().PublishJSON(businessID, "shift.swap.decided", gin.H{"swap_id": sw.ID, "shift_id": sw.ShiftID, "status": sw.Status})
		h.notifyDecided(businessID, "shift.swap.decided", sw.ID, sw.RequestingStaffID, sw.Status)
		notifyStaff(h.db, businessID, []uint{sw.RequestingStaffID}, "shift.swap.decided",
			services.PushKeyCoverageDecided, services.PushArgs{}, "/staff/home?tab=coverage")
		c.JSON(http.StatusOK, gin.H{"success": true, "data": sw})
	case "open_claim":
		cl, err := h.db.DecideClaim(businessID, reqID, decider, approve, callerActor(c))
		if err != nil {
			coverageError(c, err)
			return
		}
		events.GetHub().PublishJSON(businessID, "openshift.decided", gin.H{"claim_id": cl.ID, "shift_id": cl.ShiftID, "status": cl.Status})
		h.notifyDecided(businessID, "openshift.decided", cl.ID, cl.ClaimingStaffID, cl.Status)
		notifyStaff(h.db, businessID, []uint{cl.ClaimingStaffID}, "openshift.decided",
			services.PushKeyCoverageDecided, services.PushArgs{}, "/staff/home?tab=coverage")
		c.JSON(http.StatusOK, gin.H{"success": true, "data": cl})
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "kind must be swap or open_claim")
	}
}

type cancelDTO struct {
	Kind string `json:"kind"` // swap | giveup | open_claim
}

// Cancel lets a staffer retract their OWN pending coverage request — a swap/giveup
// they offered or an open-shift claim they filed. Polymorphic like Decide: the body
// `kind` selects the state machine, :requestId is the swap id or claim id. It reuses
// the *.decided SSE events for invalidation — a cancelled request must leave both
// the manager pending queue (ApprovalsPanel) and the staffer's "my requests" list.
func (h *CoverageHandler) Cancel(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	reqID, ok := parseParamUint(c, "requestId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can cancel requests")
		return
	}
	var in cancelDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	switch in.Kind {
	case database.SwapKindSwap, database.SwapKindGiveup:
		sw, err := h.db.CancelSwap(businessID, reqID, staffID, callerActor(c))
		if err != nil {
			coverageError(c, err)
			return
		}
		events.GetHub().PublishJSON(businessID, "shift.swap.decided", gin.H{"swap_id": sw.ID, "shift_id": sw.ShiftID, "status": sw.Status})
		c.JSON(http.StatusOK, gin.H{"success": true, "data": sw})
	case "open_claim":
		cl, err := h.db.WithdrawClaim(businessID, reqID, staffID, callerActor(c))
		if err != nil {
			coverageError(c, err)
			return
		}
		events.GetHub().PublishJSON(businessID, "openshift.decided", gin.H{"claim_id": cl.ID, "shift_id": cl.ShiftID, "status": cl.Status})
		c.JSON(http.StatusOK, gin.H{"success": true, "data": cl})
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "kind must be swap, giveup, or open_claim")
	}
}

func (h *CoverageHandler) ListOpen(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	res, err := h.db.ListOpenCoverage(businessID, callerStaffID(c), callerCanApprove(c))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list coverage")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

func (h *CoverageHandler) ListMine(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": &database.MyCoverageResult{Claims: []database.OpenShiftClaim{}, Swaps: []database.ShiftSwapRequest{}}})
		return
	}
	res, err := h.db.ListMyCoverage(businessID, staffID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list coverage")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// History returns the business's resolved coverage events (approved/denied/
// cancelled swaps + approved/denied/withdrawn claims), newest resolution first.
// A manager surface: gated schedule:read at the route, and here restricted to
// approvers (owner/manager/schedule:approve) — a non-approver gets 403, which the
// operator History panel treats as "stay hidden". ?limit caps the page (default
// 50; the service clamps to its own bound). Money-free.
func (h *CoverageHandler) History(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	if !callerCanApprove(c) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Approver access required")
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.db.ListCoverageHistory(businessID, limit)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list coverage history")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// notifyDecided enqueues a best-effort operator Telegram nudge for a resolved
// coverage request. Gated so businesses without a connected Telegram config (or
// with the event disabled) never accrue dead outbox rows, and enqueue errors are
// logged rather than silently discarded.
func (h *CoverageHandler) notifyDecided(businessID uint, eventType string, eventID, recipientStaffID uint, status string) {
	if !services.ShouldEnqueueTelegramNotification(businessID, services.PluginEventCoverageDecided) {
		return
	}
	if _, _, err := services.EnqueuePluginNotification(services.PluginNotificationEvent{
		BusinessID: businessID,
		EventType:  services.PluginEventCoverageDecided,
		EventID:    eventType + ":" + itoa(eventID),
		Payload:    map[string]interface{}{"recipient_staff_id": recipientStaffID, "status": status},
		CreatedAt:  time.Now().UTC(),
	}, "telegram"); err != nil {
		log.Printf("Failed to enqueue Telegram coverage.decided notification for business_id=%d event=%s id=%d: %v", businessID, eventType, eventID, err)
	}
}

// excludeStaffID returns ids with the given staff id removed (the requester is
// never notified of their own coverage offer).
func excludeStaffID(ids []uint, drop uint) []uint {
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}
