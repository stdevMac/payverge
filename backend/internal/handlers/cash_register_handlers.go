package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services/cashregister"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

type CashRegisterHandler struct {
	service *cashregister.Service
}

type openCashRegisterSessionRequest struct {
	OpeningFloat json.RawMessage `json:"opening_float"`
	OpeningNote  string          `json:"opening_note"`
}

type createCashRegisterMovementRequest struct {
	MovementType database.CashRegisterMovementType `json:"movement_type"`
	Amount       json.RawMessage                   `json:"amount"`
	Reason       string                            `json:"reason"`
	Note         string                            `json:"note"`
}

type closeCashRegisterSessionRequest struct {
	CountedCash json.RawMessage `json:"counted_cash"`
	ClosingNote string          `json:"closing_note"`
}

func NewCashRegisterHandler(db *gorm.DB) *CashRegisterHandler {
	return &CashRegisterHandler{service: cashregister.NewService(db)}
}

func (h *CashRegisterHandler) GetCurrent(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}

	snapshot, err := h.service.Current(c.Request.Context(), businessID)
	if err != nil {
		respondCashRegisterUnexpectedError(c, "current cash register session", err)
		return
	}

	payload := gin.H{
		"session":                 snapshot.Session,
		"unassigned_cash_total":   centsToDollars(snapshot.UnassignedTotalCents),
		"unassigned_cash_count":   snapshot.UnassignedCount,
		"suggested_opening_float": nil,
		"house_rail":              nil,
	}
	if snapshot.SuggestedOpeningFloatCents != nil {
		payload["suggested_opening_float"] = centsToDollars(*snapshot.SuggestedOpeningFloatCents)
	}
	if snapshot.Session != nil && snapshot.Session.Status == database.CashRegisterSessionStatusOpen {
		payload["house_rail"] = "cash"
	}

	c.JSON(http.StatusOK, payload)
}

func (h *CashRegisterHandler) OpenSession(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}

	var req openCashRegisterSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	openingFloatCents := int64(0)
	if len(req.OpeningFloat) > 0 && string(req.OpeningFloat) != "null" {
		parsed, err := parseCashRegisterDollarAmountToCents(req.OpeningFloat)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid opening_float")
			return
		}
		openingFloatCents = parsed
	}

	session, err := h.service.OpenSession(c.Request.Context(), cashregister.OpenSessionInput{
		BusinessID:        businessID,
		OpeningFloatCents: openingFloatCents,
		OpeningNote:       req.OpeningNote,
		Actor:             cashRegisterActorFromContext(c),
	})
	if err != nil {
		h.respondServiceError(c, "open cash register session", err)
		return
	}

	c.JSON(http.StatusCreated, session)
}

func (h *CashRegisterHandler) ListSessions(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	sessions, total, err := h.service.ListSessions(c.Request.Context(), businessID, limit, offset)
	if err != nil {
		respondCashRegisterUnexpectedError(c, "list cash register sessions", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions, "total": total})
}

func (h *CashRegisterHandler) GetSession(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}
	sessionID, ok := parseCashRegisterSessionID(c)
	if !ok {
		return
	}

	session, err := h.service.GetSession(c.Request.Context(), businessID, sessionID)
	if err != nil {
		h.respondServiceError(c, "get cash register session", err)
		return
	}

	c.JSON(http.StatusOK, cashRegisterSessionWithMovementsResponse(session))
}

func (h *CashRegisterHandler) CreateMovement(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}
	sessionID, ok := parseCashRegisterSessionID(c)
	if !ok {
		return
	}

	var req createCashRegisterMovementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	amountCents, err := parseCashRegisterDollarAmountToCents(req.Amount)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid amount")
		return
	}

	movement, session, err := h.service.CreateManualMovement(c.Request.Context(), cashregister.ManualMovementInput{
		BusinessID:   businessID,
		SessionID:    sessionID,
		MovementType: req.MovementType,
		AmountCents:  amountCents,
		Reason:       req.Reason,
		Note:         req.Note,
		Actor:        cashRegisterActorFromContext(c),
	})
	if err != nil {
		h.respondServiceError(c, "create cash register movement", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"movement": movement, "session": session})
}

func (h *CashRegisterHandler) CloseSession(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}
	sessionID, ok := parseCashRegisterSessionID(c)
	if !ok {
		return
	}

	var req closeCashRegisterSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	countedCashCents, err := parseCashRegisterDollarAmountToCents(req.CountedCash)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid counted_cash")
		return
	}

	session, err := h.service.CloseSession(c.Request.Context(), cashregister.CloseSessionInput{
		BusinessID:       businessID,
		SessionID:        sessionID,
		CountedCashCents: countedCashCents,
		ClosingNote:      req.ClosingNote,
		Actor:            cashRegisterActorFromContext(c),
	})
	if err != nil {
		h.respondServiceError(c, "close cash register session", err)
		return
	}

	c.JSON(http.StatusOK, session)
}

func (h *CashRegisterHandler) GetUnassigned(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}

	count, totalCents, err := h.service.UnassignedCash(c.Request.Context(), businessID)
	if err != nil {
		respondCashRegisterUnexpectedError(c, "get unassigned cash register cash", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"total": centsToDollars(totalCents), "count": count})
}

// ListUnassigned returns the individual unassigned cash tenders (bounded,
// paginated) that make up the unassigned-cash total, so the operator can inspect
// what needs assigning instead of seeing only a headline number.
func (h *CashRegisterHandler) ListUnassigned(c *gin.Context) {
	businessID, ok := h.loadBusinessID(c)
	if !ok {
		return
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	offset := 0
	if v := c.Query("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	items, total, err := h.service.ListUnassignedCash(c.Request.Context(), businessID, limit, offset)
	if err != nil {
		respondCashRegisterUnexpectedError(c, "list unassigned cash register cash", err)
		return
	}

	// Emit money as dollars (float64) per the wire contract, mirroring the
	// summary endpoint's centsToDollars conversion.
	type unassignedItemDTO struct {
		ID              uint    `json:"id"`
		BillID          uint    `json:"bill_id"`
		BillNumber      string  `json:"bill_number"`
		TableName       string  `json:"table_name"`
		ParticipantName string  `json:"participant_name"`
		Amount          float64 `json:"amount"`
		Status          string  `json:"status"`
		CreatedAt       string  `json:"created_at"`
	}
	out := make([]unassignedItemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, unassignedItemDTO{
			ID:              it.ID,
			BillID:          it.BillID,
			BillNumber:      it.BillNumber,
			TableName:       it.TableName,
			ParticipantName: it.ParticipantName,
			Amount:          centsToDollars(it.AmountCents),
			Status:          it.Status,
			CreatedAt:       it.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": out, "total": total})
}

func (h *CashRegisterHandler) loadBusinessID(c *gin.Context) (uint, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		if isCashRegisterBusinessNotFound(err) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return 0, false
		}
		respondCashRegisterUnexpectedError(c, "load cash register business", err)
		return 0, false
	}
	if !server.CheckBusinessAccess(c, business) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
		return 0, false
	}
	return business.ID, true
}

func isCashRegisterBusinessNotFound(err error) bool {
	if err == nil {
		return false
	}
	if database.IsRecordNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "business not found")
}

func (h *CashRegisterHandler) respondServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, cashregister.ErrInvalidAmount),
		errors.Is(err, cashregister.ErrInvalidMovementType),
		errors.Is(err, cashregister.ErrMissingReason):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, cashregister.ErrSessionNotFound):
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, err.Error())
	case errors.Is(err, cashregister.ErrSessionAlreadyOpen),
		errors.Is(err, cashregister.ErrSessionClosed):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, err.Error())
	default:
		respondCashRegisterUnexpectedError(c, operation, err)
	}
}

func parseCashRegisterSessionID(c *gin.Context) (uint, bool) {
	raw := strings.TrimSpace(c.Param("sessionId"))
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid sessionId")
		return 0, false
	}
	return uint(id), true
}

func parseCashRegisterDollarAmountToCents(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("amount is required")
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return parseDollarAmountToCents(str)
	}

	amountText := strings.TrimSpace(string(raw))
	if amountText == "" {
		return 0, fmt.Errorf("amount is required")
	}
	return parseDollarAmountToCents(amountText)
}

func cashRegisterSessionWithMovementsResponse(session *database.CashRegisterSession) gin.H {
	if session == nil {
		return gin.H{"movements": []database.CashRegisterMovement{}}
	}

	var payload map[string]any
	raw, err := json.Marshal(session)
	if err == nil {
		_ = json.Unmarshal(raw, &payload)
	}
	if payload == nil {
		payload = gin.H{}
	}
	payload["movements"] = session.Movements
	return payload
}

func respondCashRegisterUnexpectedError(c *gin.Context, operation string, err error) {
	logger.Logger.Errorf("cash-register: %s failed: %v", operation, err)
	server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Cash register operation failed")
}

func cashRegisterActorFromContext(c *gin.Context) database.CashRegisterActor {
	actor := database.CashRegisterActor{Label: "unknown"}
	if staffID := server.ExtractStaffIDFromContext(c); staffID != nil {
		actor.StaffID = staffID
		// Prefer the live staff name hydrated by staff auth middleware so
		// operators see "Maria Lopez", not opaque "staff:12" in the drawer.
		if name, ok := c.Get("staff_name"); ok {
			if s, ok := name.(string); ok {
				if label := strings.TrimSpace(s); label != "" {
					actor.Label = label
					return actor
				}
			}
		}
		var staff database.Staff
		if err := database.GetDB().Select("name", "email").First(&staff, *staffID).Error; err == nil {
			if label := strings.TrimSpace(staff.Name); label != "" {
				actor.Label = label
				return actor
			}
			if label := strings.TrimSpace(staff.Email); label != "" {
				actor.Label = label
				return actor
			}
		}
		actor.Label = fmt.Sprintf("staff:%d", *staffID)
		return actor
	}
	if raw, ok := c.Get("user_id"); ok {
		if id, ok := cashRegisterUintFromContext(raw); ok && id > 0 {
			actor.UserID = &id
			var user database.User
			if err := database.GetDB().Select("name", "email").First(&user, id).Error; err == nil {
				if label := strings.TrimSpace(user.Name); label != "" {
					actor.Label = label
					return actor
				}
				if label := strings.TrimSpace(user.Email); label != "" {
					actor.Label = label
					return actor
				}
			}
			actor.Label = fmt.Sprintf("user:%d", id)
			return actor
		}
	}
	if raw, ok := c.Get("address"); ok {
		actor.Label = fmt.Sprintf("%v", raw)
	}
	return actor
}

func cashRegisterUintFromContext(raw any) (uint, bool) {
	switch v := raw.(type) {
	case uint:
		return v, true
	case uint64:
		return uint(v), true
	case uint32:
		return uint(v), true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case int32:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case float64:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case float32:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case string:
		parsed, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0, false
		}
		return uint(parsed), true
	default:
		return 0, false
	}
}
