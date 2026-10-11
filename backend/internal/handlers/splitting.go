package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/guestsession"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/splitting"
	"gorm.io/gorm"
)

// MaxSplitPeople is the per-bill upper bound on splitters (IMP-16). Raised
// from 20 → 100 so private dining / banquet / hotel F&B parties don't get
// stranded mid-tap with an opaque error. Beyond 100 the UX stops making
// sense — those belong on the invoice flow, not the bill split flow.
const MaxSplitPeople = 100

const guestSplitSessionCookie = guestsession.CookieName

// SplittingHandler handles bill splitting requests
type SplittingHandler struct {
	db       *database.DB
	splitter *splitting.SplittingService
}

type publicSplitBillItem struct {
	ID         string  `json:"id"`
	MenuItemID string  `json:"menu_item_id,omitempty"`
	Name       string  `json:"name"`
	Price      float64 `json:"price"`
	Quantity   int     `json:"quantity"`
	Subtotal   float64 `json:"subtotal"`
	ItemType   string  `json:"item_type,omitempty"`
}

func (h *SplittingHandler) loadBillByNumber(c *gin.Context, requireOpen bool) (*database.Bill, []database.BillItem, bool) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return nil, nil, false
	}

	bill, items, err := database.GetPublicBillByToken(token)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return nil, nil, false
	}

	if requireOpen {
		if bill.Status != database.BillStatusOpen && bill.Status != database.BillStatusPartial {
			server.RespondWithError(c, http.StatusBadRequest, "split_not_open", "Bill is not open for splitting")
			return nil, nil, false
		}
		if bill.TotalAmount-bill.PaidAmount <= 0 {
			server.RespondWithError(c, http.StatusBadRequest, "split_not_open", "Bill has no remaining balance")
			return nil, nil, false
		}
	}

	return bill, items, true
}

// NewSplittingHandler creates a new splitting handler
func NewSplittingHandler(db *database.DB) *SplittingHandler {
	return &SplittingHandler{
		db:       db,
		splitter: splitting.NewSplittingService(db),
	}
}

func splitCentsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}

// The guest session cookie lives in internal/guestsession so the fiscal
// identity handler (internal/server) and payment rows share one session
// definition. These wrappers keep the split call sites on GetOrIssue and
// FromRequest unchanged.

func (h *SplittingHandler) getOrIssueGuestSession(c *gin.Context) string {
	return guestsession.GetOrIssue(c)
}

func getExistingGuestSession(c *gin.Context) (string, bool) {
	return guestsession.FromRequest(c)
}

func splitShareResponse(share *database.BillSplitShare) gin.H {
	if share == nil {
		return gin.H{}
	}
	return gin.H{
		"id":                share.ID,
		"display_name":      share.DisplayName,
		"mode":              share.Mode,
		"amount":            splitCentsToDollars(share.AmountCents),
		"amount_cents":      share.AmountCents,
		"tip_amount":        splitCentsToDollars(share.TipCents),
		"tip_cents":         share.TipCents,
		"status":            share.Status,
		"hold_expires_at":   share.HoldExpiresAt,
		"tender":            share.Tender,
		"settled_at":        share.SettledAt,
		"released_at":       share.ReleasedAt,
		"claimed_item_ids":  share.ClaimedItemIDs,
		"claimed_fractions": share.ClaimedFractions,
	}
}

func splitReceiptResponse(receipt *database.BillSplitShareReceipt) gin.H {
	items := make([]gin.H, 0, len(receipt.Items))
	for _, item := range receipt.Items {
		items = append(items, gin.H{
			"id":               item.ID,
			"name":             item.Name,
			"fraction":         item.Fraction,
			"quantity":         item.Quantity,
			"unit_price":       splitCentsToDollars(item.UnitPriceCents),
			"unit_price_cents": item.UnitPriceCents,
			"subtotal":         splitCentsToDollars(item.SubtotalCents),
			"subtotal_cents":   item.SubtotalCents,
		})
	}
	return gin.H{
		"share_id":          receipt.ShareID,
		"bill_number":       receipt.BillNumber,
		"display_name":      receipt.DisplayName,
		"mode":              receipt.Mode,
		"status":            receipt.Status,
		"tender":            receipt.Tender,
		"subtotal":          splitCentsToDollars(receipt.SubtotalCents),
		"subtotal_cents":    receipt.SubtotalCents,
		"tax":               splitCentsToDollars(receipt.TaxCents),
		"tax_cents":         receipt.TaxCents,
		"service_fee":       splitCentsToDollars(receipt.ServiceFeeCents),
		"service_fee_cents": receipt.ServiceFeeCents,
		"amount":            splitCentsToDollars(receipt.AmountCents),
		"amount_cents":      receipt.AmountCents,
		"tip_amount":        splitCentsToDollars(receipt.TipCents),
		"tip_cents":         receipt.TipCents,
		"grand_total":       splitCentsToDollars(receipt.GrandTotalCents),
		"grand_total_cents": receipt.GrandTotalCents,
		"items":             items,
		"settled_at":        receipt.SettledAt,
	}
}

func splitStateResponse(state *database.BillSplitState) gin.H {
	shares := make([]gin.H, 0, len(state.Shares))
	for _, share := range state.Shares {
		shares = append(shares, gin.H{
			"id":                share.ID,
			"display_name":      share.DisplayName,
			"mode":              share.Mode,
			"amount":            splitCentsToDollars(share.AmountCents),
			"amount_cents":      share.AmountCents,
			"tip_amount":        splitCentsToDollars(share.TipCents),
			"tip_cents":         share.TipCents,
			"status":            share.Status,
			"hold_expires_at":   share.HoldExpiresAt,
			"tender":            share.Tender,
			"settled_at":        share.SettledAt,
			"released_at":       share.ReleasedAt,
			"claimed_item_ids":  share.ClaimedItemIDs,
			"claimed_fractions": share.ClaimedFractions,
		})
	}
	return gin.H{
		"bill_number":      state.BillNumber,
		"status":           state.Status,
		"total_amount":     splitCentsToDollars(state.TotalCents),
		"total_cents":      state.TotalCents,
		"paid_amount":      splitCentsToDollars(state.PaidCents),
		"paid_cents":       state.PaidCents,
		"held_amount":      splitCentsToDollars(state.HeldCents),
		"held_cents":       state.HeldCents,
		"available_amount": splitCentsToDollars(state.AvailableCents),
		"available_cents":  state.AvailableCents,
		"updated_at":       state.UpdatedAt,
		"shares":           shares,
	}
}

func splitShareViewResponse(share database.BillSplitShareView) gin.H {
	return gin.H{
		"id":                share.ID,
		"display_name":      share.DisplayName,
		"mode":              share.Mode,
		"amount":            splitCentsToDollars(share.AmountCents),
		"amount_cents":      share.AmountCents,
		"tip_amount":        splitCentsToDollars(share.TipCents),
		"tip_cents":         share.TipCents,
		"status":            share.Status,
		"hold_expires_at":   share.HoldExpiresAt,
		"tender":            share.Tender,
		"settled_at":        share.SettledAt,
		"released_at":       share.ReleasedAt,
		"claimed_item_ids":  share.ClaimedItemIDs,
		"claimed_fractions": share.ClaimedFractions,
	}
}

func PublishGuestSplitState(state *database.BillSplitState) {
	if state == nil {
		return
	}
	events.GetHub().PublishJSON(state.BusinessID, "bill.split.updated", splitStateResponse(state))
}

func splitHandlerError(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case strings.Contains(err.Error(), "record not found"):
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill or split share not found")
	case errors.Is(err, database.ErrInvalidSplitMode),
		errors.Is(err, database.ErrInvalidSplitTender),
		errors.Is(err, database.ErrInvalidPaymentAmount),
		errors.Is(err, database.ErrInvalidTipAmount),
		errors.Is(err, database.ErrInvalidIdempotencyKey):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrSplitAmountUnavailable),
		errors.Is(err, database.ErrSplitItemUnavailable),
		errors.Is(err, database.ErrSplitReceiptUnavailable),
		errors.Is(err, database.ErrPaymentExceedsRemaining),
		errors.Is(err, database.ErrSplitHoldExpired),
		errors.Is(err, database.ErrSplitShareAlreadyFinal):
		// Guest-stable code for share/amount races (FE guestPaymentErrors mapper).
		server.RespondWithError(c, http.StatusConflict, "split_share_conflict", err.Error())
	case errors.Is(err, database.ErrBillNotPayable):
		server.RespondWithError(c, http.StatusConflict, "split_not_open", err.Error())
	case errors.Is(err, database.ErrPaymentTxHashConflict),
		errors.Is(err, database.ErrPaymentAlreadySettledForBill):
		server.RespondWithError(c, http.StatusConflict, "payment_failed", err.Error())
	case errors.Is(err, database.ErrSplitGuestMismatch):
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Split share belongs to another guest session")
	default:
		log.Printf("split handler error: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Operation failed")
	}
}

func parseSplitLastEventID(value string) uint64 {
	_, seq := parseSplitResumeToken(value)
	return seq
}

// parseSplitResumeToken accepts a bare sequence ("12") or the operator-style
// epoch-prefixed id ("<epoch>:<seq>"). A garbage token returns seq 0 so the
// stream can start fresh instead of dumping the whole ring (#535).
func parseSplitResumeToken(value string) (string, uint64) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		epoch := value[:idx]
		seq, err := strconv.ParseUint(value[idx+1:], 10, 64)
		if err != nil {
			return "", 0
		}
		return epoch, seq
	}
	seq, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "", 0
	}
	return "", seq
}

func splitResumeShouldSkipReplay(header, query string) bool {
	raw := strings.TrimSpace(header)
	if raw == "" {
		raw = strings.TrimSpace(query)
	}
	if raw == "" {
		return true
	}
	epoch, seq := parseSplitResumeToken(raw)
	if seq == 0 {
		return true
	}
	if epoch != "" && epoch != events.GetHub().Epoch() {
		return true
	}
	return false
}

func writeGuestSplitSSEFrame(c *gin.Context, id uint64, eventType string, raw json.RawMessage) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	if id > 0 {
		_, _ = fmt.Fprintf(c.Writer, "id: %d\n", id)
	}
	if eventType != "" {
		_, _ = fmt.Fprintf(c.Writer, "event: %s\n", eventType)
	}
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(raw))
}

func writeGuestSplitSSE(c *gin.Context, eventType string, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	writeGuestSplitSSEFrame(c, 0, eventType, raw)
}

func splitEventMatchesBill(event events.BusinessEvent, billNumber string) bool {
	if event.Type != "bill.split.updated" {
		return false
	}
	var payload struct {
		BillNumber string `json:"bill_number"`
	}
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return false
	}
	return payload.BillNumber == billNumber
}

func writeGuestSplitBusinessEvent(c *gin.Context, event events.BusinessEvent) {
	envelope := struct {
		ID        uint64          `json:"id"`
		Type      string          `json:"type"`
		Data      json.RawMessage `json:"data"`
		Timestamp string          `json:"timestamp,omitempty"`
	}{
		ID:   event.ID,
		Type: event.Type,
		Data: event.Data,
	}
	if !event.Timestamp.IsZero() {
		envelope.Timestamp = event.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	writeGuestSplitSSEFrame(c, event.ID, event.Type, raw)
}

// CalculateEqualSplit calculates equal split for a bill
// POST /api/v1/guest/bill/:bill_token/split/equal
func (h *SplittingHandler) CalculateEqualSplit(c *gin.Context) {
	var req splitting.EqualSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	bill, _, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}

	req.BillID = bill.ID

	// Validate number of people
	if req.NumPeople <= 0 || req.NumPeople > MaxSplitPeople {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Number of people must be between 1 and 100")
		return
	}

	result, err := h.splitter.CalculateEqualSplit(req.BillID, req.NumPeople, req.People)
	if err != nil {
		log.Printf("Failed to calculate equal split for bill %d: %v", req.BillID, err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Operation failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

// CalculateCustomSplit calculates custom split for a bill
// POST /api/v1/guest/bill/:bill_token/split/custom
func (h *SplittingHandler) CalculateCustomSplit(c *gin.Context) {
	var req splitting.CustomSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	bill, _, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}

	req.BillID = bill.ID

	// Validate amounts
	if len(req.Amounts) == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Amounts cannot be empty")
		return
	}

	if len(req.Amounts) > MaxSplitPeople {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot split between more than 100 people")
		return
	}

	result, err := h.splitter.CalculateCustomSplit(req.BillID, req.Amounts, req.People)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

// CalculateItemSplit calculates item-based split for a bill
// POST /api/v1/guest/bill/:bill_token/split/items
func (h *SplittingHandler) CalculateItemSplit(c *gin.Context) {
	var req splitting.ItemSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	bill, _, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}

	req.BillID = bill.ID

	// Validate item selections
	if len(req.ItemSelections) == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Item selections cannot be empty")
		return
	}

	if len(req.ItemSelections) > MaxSplitPeople {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot split between more than 100 people")
		return
	}

	result, err := h.splitter.CalculateItemSplit(req.BillID, req.ItemSelections, req.People)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

// GetBillSplitOptions returns available split options for a bill
// GET /api/v1/guest/bill/:bill_token/split/options
func (h *SplittingHandler) GetBillSplitOptions(c *gin.Context) {
	bill, items, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}

	// Calculate remaining amount to be paid
	remainingAmount := bill.TotalAmount - bill.PaidAmount

	// Only surface claimable (positive menu) lines for item-split. Offer
	// discount rows are auto-allocated into food bases server-side; exposing
	// them as claimable rows lets guests pick a zero/negative hold that 400s
	// and confuses total_items when promos are active.
	publicItems := make([]publicSplitBillItem, 0, len(items))
	for _, item := range items {
		subtotalCents := int64(math.Round(item.Subtotal * 100))
		if database.IsNonClaimableSplitBillItem(item.ItemType, subtotalCents) {
			continue
		}
		if subtotalCents <= 0 {
			continue
		}
		publicItems = append(publicItems, publicSplitBillItem{
			ID:         item.ID,
			MenuItemID: item.MenuItemID,
			Name:       item.Name,
			Price:      item.Price,
			Quantity:   item.Quantity,
			Subtotal:   item.Subtotal,
			ItemType:   item.ItemType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"bill": gin.H{
			"bill_number":        bill.BillNumber,
			"total_amount":       float64(bill.TotalAmount) / 100.0,
			"paid_amount":        float64(bill.PaidAmount) / 100.0,
			"remaining_amount":   float64(remainingAmount) / 100.0,
			"subtotal":           float64(bill.Subtotal) / 100.0,
			"tax_amount":         float64(bill.TaxAmount) / 100.0,
			"service_fee_amount": float64(bill.ServiceFeeAmount) / 100.0,
			"status":             bill.Status,
		},
		"items": publicItems,
		"split_options": gin.H{
			"equal": gin.H{
				"available":   true,
				"description": "Split the bill equally among all people",
				"min_people":  1,
				"max_people":  MaxSplitPeople,
			},
			"custom": gin.H{
				"available":   true,
				"description": "Specify custom amounts for each person",
				"min_people":  1,
				"max_people":  MaxSplitPeople,
			},
			"items": gin.H{
				"available":   len(publicItems) > 0,
				"description": "Split based on which items each person ordered",
				"min_people":  1,
				"max_people":  MaxSplitPeople,
				"total_items": len(publicItems),
			},
		},
	})
}

// GetSplitState returns the live guest-safe split state for a public bill.
// GET /api/v1/guest/bill/:bill_token/split/state
func (h *SplittingHandler) GetSplitState(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return
	}
	state, err := database.GetBillSplitStateByNumber(token, time.Now().UTC())
	if err != nil {
		splitHandlerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"state":   splitStateResponse(state),
	})
}

// GetSplitShareReceipt returns the payer's guest-safe receipt for one settled
// split share. The public token alone is not enough: the signed guest session
// cookie must match the share that paid.
// GET /api/v1/guest/bill/:bill_token/split/shares/:share_id/receipt
func (h *SplittingHandler) GetSplitShareReceipt(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return
	}
	shareID, err := strconv.ParseUint(strings.TrimSpace(c.Param("share_id")), 10, 32)
	if err != nil || shareID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid split share ID")
		return
	}
	guestSessionID, ok := getExistingGuestSession(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Split receipt belongs to another guest session")
		return
	}

	receipt, err := database.GetGuestBillSplitShareReceipt(token, uint(shareID), guestSessionID)
	if err != nil {
		splitHandlerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"receipt": splitReceiptResponse(receipt),
	})
}

// GetMySplitShares returns split shares owned by the current signed guest
// session so a no-login guest can recover a held or settled share after refresh.
// GET /api/v1/guest/bill/:bill_token/split/my-shares
func (h *SplittingHandler) GetMySplitShares(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return
	}
	guestSessionID, ok := getExistingGuestSession(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"shares":  []gin.H{},
		})
		return
	}

	shares, err := database.GetGuestBillSplitSharesByNumber(token, guestSessionID, time.Now().UTC())
	if err != nil {
		splitHandlerError(c, err)
		return
	}
	responseShares := make([]gin.H, 0, len(shares))
	for _, share := range shares {
		responseShares = append(responseShares, splitShareViewResponse(share))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"shares":  responseShares,
	})
}

// StreamSplitEvents streams guest-safe split updates for one public bill token.
// GET /api/v1/guest/bill/:bill_token/split/events
func (h *SplittingHandler) StreamSplitEvents(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return
	}

	state, err := database.GetBillSplitStateByNumber(token, time.Now().UTC())
	if err != nil {
		splitHandlerError(c, err)
		return
	}

	_ = h.getOrIssueGuestSession(c)

	headerID := c.GetHeader("Last-Event-ID")
	queryID := c.Query("last_event_id")
	lastEventID := parseSplitLastEventID(headerID)
	if lastEventID == 0 {
		lastEventID = parseSplitLastEventID(queryID)
	}
	skipReplay := splitResumeShouldSkipReplay(headerID, queryID)

	// Held-connection DoS guard: this public, unauthenticated stream has no
	// per-endpoint limiter — an attacker reusing one valid bill_number could open
	// streams up to the global rate-limit budget and HOLD them, accumulating
	// goroutines + 64-slot channels (and making Publish O(N) worse) without
	// bound. SubscribeLimited rejects once the per-business OR per-IP
	// concurrent-connection ceiling is hit. Reject BEFORE writing the 200 SSE
	// headers so we can return a real 429 the client (and any proxy) can see.
	//
	// Topic-scope to the only event type this guest SSE emits: subscribing to the
	// whole-business hub carried every type (orders, payments, deliveries,
	// reservations, alerts); an unrelated burst could fill the 64-slot channel
	// and silently evict this bill's own bill.split.updated frame. The
	// per-bill_number check below still runs, since one business can host many
	// split bills at once.
	//
	// Guest streams are counted in the hub's guest pool (M-sse), separate from
	// authenticated operator streams, and keyed per client with IPv6 bucketed
	// per /64, so an anonymous flood cannot take kitchen/dashboard streams
	// offline or dodge the per-IP cap by rotating addresses.
	//
	// The bill filter runs in Publish, before enqueue: a busy venue with many
	// split bills would otherwise fill this stream's buffer with other bills'
	// frames and drop this bill's own. If the buffer still overflows, the hub
	// signals `dropped` and the stream sends a fresh `sync.reset` snapshot.
	hub := events.GetHub()
	billNumber := state.BillNumber
	ch, replayed, dropped, cancel, ok := hub.SubscribeGuestFiltered(
		state.BusinessID, lastEventID, middleware.ClientRateLimitKey(c), []string{"bill.split.updated"},
		func(ev events.BusinessEvent) bool { return splitEventMatchesBill(ev, billNumber) },
	)
	if !ok {
		c.Header("Retry-After", "30")
		server.RespondWithError(c, http.StatusTooManyRequests, server.ErrCodeRateLimited,
			"Too many concurrent live connections; please retry shortly")
		return
	}
	defer cancel()
	// Same X-9 rule as the operator stream: a fresh connect (or an unusable
	// Last-Event-ID) must not dump the retained ring of other guests' names
	// and released holds before the authoritative `connected` snapshot (#535).
	if skipReplay {
		replayed = nil
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	for _, event := range replayed {
		if splitEventMatchesBill(event, state.BillNumber) {
			writeGuestSplitBusinessEvent(c, event)
		}
	}

	writeGuestSplitSSE(c, "connected", gin.H{
		"bill_number": state.BillNumber,
		"state":       splitStateResponse(state),
	})
	c.Writer.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			if splitEventMatchesBill(event, state.BillNumber) {
				writeGuestSplitBusinessEvent(c, event)
				c.Writer.Flush()
			}
		case <-dropped:
			// A frame for this bill was lost; resend the authoritative state so
			// the guest never acts on a stale split. Every bill.split.updated
			// frame is a full snapshot, so frames still queued from before the
			// drop are older than the state read below and must never be
			// written after the reset (a paid share would look available
			// again). Drain them first, and re-read if more arrive while the
			// state is being read.
			drainBusinessEvents(ch)
			var fresh *database.BillSplitState
			for attempt := 0; attempt < 3; attempt++ {
				var err error
				fresh, err = database.GetBillSplitStateByNumber(token, time.Now().UTC())
				if err != nil {
					// End the stream; the client reconnects and gets `connected`.
					return
				}
				// The last read keeps whatever arrived during it: dropping
				// those frames without a re-read could lose a newer state.
				if attempt == 2 || drainBusinessEvents(ch) == 0 {
					break
				}
			}
			writeGuestSplitSSE(c, "sync.reset", gin.H{
				"bill_number": fresh.BillNumber,
				"state":       splitStateResponse(fresh),
			})
			c.Writer.Flush()
		case <-ticker.C:
			writeGuestSplitSSE(c, "ping", gin.H{"time": time.Now().UTC().Format(time.RFC3339)})
			c.Writer.Flush()
		}
	}
}

// CreateSplitHold reserves a server-side share for a no-login guest session.
// POST /api/v1/guest/bill/:bill_token/split/holds
func (h *SplittingHandler) CreateSplitHold(c *gin.Context) {
	var req struct {
		Mode             string            `json:"mode" binding:"required"`
		Amount           string            `json:"amount"`
		CoverRemaining   bool              `json:"cover_remaining"`
		NumPeople        int               `json:"num_people"`
		SharesCovered    int               `json:"shares_covered"`
		DisplayName      string            `json:"display_name"`
		ClaimedItemIDs   []string          `json:"claimed_item_ids"`
		ClaimedFractions map[string]string `json:"claimed_fractions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	bill, _, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}

	mode := database.BillSplitMode(strings.ToLower(strings.TrimSpace(req.Mode)))
	var amountCents int64
	switch mode {
	case database.BillSplitModeEqual:
		if req.NumPeople <= 0 || req.NumPeople > MaxSplitPeople {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Number of people must be between 1 and 100")
			return
		}
		sharesCovered := req.SharesCovered
		if sharesCovered <= 0 {
			sharesCovered = 1
		}
		if sharesCovered > req.NumPeople {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Shares covered cannot exceed number of people")
			return
		}
		amountCents = 0
		req.SharesCovered = sharesCovered
	case database.BillSplitModeCustom:
		if req.CoverRemaining {
			amountCents = 0
		} else {
			var err error
			amountCents, err = parseDollarAmountToCents(req.Amount)
			if err != nil {
				server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
				return
			}
		}
	case database.BillSplitModeItems:
		if len(req.ClaimedItemIDs) == 0 && len(req.ClaimedFractions) == 0 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Claimed items are required for item split holds")
			return
		}
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid split mode")
		return
	}

	guestSessionID := h.getOrIssueGuestSession(c)
	share, state, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   guestSessionID,
		DisplayName:      req.DisplayName,
		Mode:             mode,
		CoverRemaining:   req.CoverRemaining,
		NumPeople:        req.NumPeople,
		SharesCovered:    req.SharesCovered,
		ClaimedItemIDs:   req.ClaimedItemIDs,
		ClaimedFractions: req.ClaimedFractions,
		AmountCents:      amountCents,
		IdempotencyKey:   c.GetHeader("X-Request-Id"),
		HoldTTL:          5 * time.Minute,
		Now:              time.Now().UTC(),
	})
	if err != nil {
		splitHandlerError(c, err)
		return
	}
	PublishGuestSplitState(state)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"share":   splitShareResponse(share),
		"state":   splitStateResponse(state),
	})
}

// ReleaseSplitShare releases a held split share owned by the signed guest
// session, freeing the amount/items for the rest of the table.
// POST /api/v1/guest/bill/:bill_token/split/shares/:share_id/release
func (h *SplittingHandler) ReleaseSplitShare(c *gin.Context) {
	bill, _, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}
	shareID, err := strconv.ParseUint(strings.TrimSpace(c.Param("share_id")), 10, 32)
	if err != nil || shareID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid split share ID")
		return
	}
	guestSessionID, ok := getExistingGuestSession(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Split share belongs to another guest session")
		return
	}

	share, state, err := database.ReleaseBillSplitShare(database.ReleaseBillSplitShareInput{
		ShareID:        uint(shareID),
		BillID:         bill.ID,
		GuestSessionID: guestSessionID,
		Now:            time.Now().UTC(),
	})
	if err != nil {
		splitHandlerError(c, err)
		return
	}
	PublishGuestSplitState(state)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"share":   splitShareResponse(share),
		"state":   splitStateResponse(state),
	})
}

// ValidateSplit validates a split calculation without saving
// POST /api/v1/guest/bill/:bill_token/split/validate
func (h *SplittingHandler) ValidateSplit(c *gin.Context) {
	var req struct {
		Method         string              `json:"method"`
		NumPeople      int                 `json:"num_people,omitempty"`
		Amounts        map[string]float64  `json:"amounts,omitempty"`
		ItemSelections map[string][]string `json:"item_selections,omitempty"`
		People         map[string]string   `json:"people,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	bill, _, ok := h.loadBillByNumber(c, true)
	if !ok {
		return
	}

	var result *splitting.SplitResult
	var err error

	switch req.Method {
	case "equal":
		if req.NumPeople <= 0 || req.NumPeople > MaxSplitPeople {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Number of people must be between 1 and 100")
			return
		}
		result, err = h.splitter.CalculateEqualSplit(bill.ID, req.NumPeople, req.People)
	case "custom":
		if len(req.Amounts) == 0 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Amounts are required for custom split")
			return
		}
		if len(req.Amounts) > MaxSplitPeople {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot split between more than 100 people")
			return
		}
		result, err = h.splitter.CalculateCustomSplit(bill.ID, req.Amounts, req.People)
	case "items":
		if len(req.ItemSelections) == 0 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Item selections are required for item split")
			return
		}
		if len(req.ItemSelections) > MaxSplitPeople {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot split between more than 100 people")
			return
		}
		result, err = h.splitter.CalculateItemSplit(bill.ID, req.ItemSelections, req.People)
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid split method. Must be 'equal', 'custom', or 'items'")
		return
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"code":    server.ErrCodeInvalidInput,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"valid":   true,
		"result":  result,
	})
}

// POST /api/v1/guest/bill/:bill_token/split/execute
func (h *SplittingHandler) ExecuteSplitPayment(c *gin.Context) {
	var req struct {
		ShareID         uint   `json:"share_id" binding:"required"`
		PaymentMethod   string `json:"payment_method" binding:"required"`
		TransactionHash string `json:"transaction_hash"`
		PayerAddress    string `json:"payer_address"`
		TipAmount       string `json:"tip_amount"`
		IdempotencyKey  string `json:"idempotency_key"`
		SourceChain     string `json:"source_chain"`
		SourceToken     string `json:"source_token"`
		LifiRouteID     string `json:"lifi_route_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	guestSessionID := h.getOrIssueGuestSession(c)
	idempotencyKey := strings.TrimSpace(c.GetHeader("X-Request-Id"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	}

	var tipCents int64
	if strings.TrimSpace(req.TipAmount) != "" {
		parsedTip, err := parseDollarAmountToCents(req.TipAmount)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
			return
		}
		tipCents = parsedTip
	}

	if splitExecuteTenderRequiresStaffConfirmation(req.PaymentMethod) {
		splitHandlerError(c, database.ErrInvalidSplitTender)
		return
	}

	if payment, existingShare, tender, err := h.confirmedSplitPaymentForExecute(
		c.Param("bill_token"),
		req.ShareID,
		guestSessionID,
		req.PaymentMethod,
		req.TransactionHash,
		tipCents,
	); err != nil {
		splitHandlerError(c, err)
		return
	} else if payment != nil {
		share, err := database.MarkBillSplitShareSettledByPayment(database.MarkBillSplitShareSettledByPaymentInput{
			ShareID:        req.ShareID,
			BillID:         payment.BillID,
			PaymentID:      payment.ID,
			TipCents:       tipCents,
			IdempotencyKey: idempotencyKey,
			Tender:         tender,
			Now:            time.Now().UTC(),
		})
		if err != nil {
			splitHandlerError(c, err)
			return
		}
		if state, stateErr := database.GetBillSplitStateByNumber(c.Param("bill_token"), time.Now().UTC()); stateErr == nil {
			PublishGuestSplitState(state)
		}
		bill, _, err := database.GetBillByIDLean(payment.BillID)
		if err != nil {
			splitHandlerError(c, err)
			return
		}
		remaining := bill.TotalAmount - bill.PaidAmount
		if remaining < 0 {
			remaining = 0
		}
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"applied":          existingShare.Status != database.BillSplitShareStatusSettled,
			"share":            splitShareResponse(share),
			"bill_status":      bill.Status,
			"remaining_amount": splitCentsToDollars(remaining),
			"remaining_cents":  remaining,
		})
		return
	}

	share, bill, applied, err := database.SettleBillSplitShare(database.SettleBillSplitShareInput{
		ShareID:        req.ShareID,
		GuestSessionID: guestSessionID,
		IdempotencyKey: idempotencyKey,
		Tender:         req.PaymentMethod,
		TxHash:         req.TransactionHash,
		PayerAddr:      req.PayerAddress,
		TipCents:       tipCents,
		SourceChain:    req.SourceChain,
		SourceToken:    req.SourceToken,
		LifiRouteID:    req.LifiRouteID,
		Now:            time.Now().UTC(),
	})
	if err != nil {
		splitHandlerError(c, err)
		return
	}
	if state, stateErr := database.GetBillSplitStateByNumber(c.Param("bill_token"), time.Now().UTC()); stateErr == nil {
		PublishGuestSplitState(state)
	}

	remaining := int64(0)
	billStatus := ""
	if bill != nil {
		remaining = bill.TotalAmount - bill.PaidAmount
		if remaining < 0 {
			remaining = 0
		}
		billStatus = string(bill.Status)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"applied":          applied,
		"share":            splitShareResponse(share),
		"bill_status":      billStatus,
		"remaining_amount": splitCentsToDollars(remaining),
		"remaining_cents":  remaining,
	})
}

func normalizeSplitExecuteTender(tender string) string {
	tender = strings.ToLower(strings.TrimSpace(tender))
	if tender == "cross_chain" {
		return "cross-chain"
	}
	if tender == "cashier" {
		return "cash"
	}
	return tender
}

func splitExecuteTenderRequiresStaffConfirmation(tender string) bool {
	switch normalizeSplitExecuteTender(tender) {
	case "cash", "card", "venmo", "other":
		return true
	default:
		return false
	}
}

func splitExecuteTenderRequiresConfirmedPayment(tender string) bool {
	switch normalizeSplitExecuteTender(tender) {
	case "crypto", "cross-chain", "plugin":
		return true
	default:
		return false
	}
}

func (h *SplittingHandler) confirmedSplitPaymentForExecute(
	token string,
	shareID uint,
	guestSessionID string,
	tender string,
	txHash string,
	tipCents int64,
) (*database.Payment, database.BillSplitShare, string, error) {
	normalizedTender := normalizeSplitExecuteTender(tender)
	if !splitExecuteTenderRequiresConfirmedPayment(normalizedTender) {
		return nil, database.BillSplitShare{}, normalizedTender, nil
	}

	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return nil, database.BillSplitShare{}, normalizedTender, database.ErrInvalidIdempotencyKey
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, database.BillSplitShare{}, normalizedTender, gorm.ErrRecordNotFound
	}

	var share database.BillSplitShare
	if err := database.GetDB().
		Model(&database.BillSplitShare{}).
		Select("bill_split_shares.id", "bill_split_shares.bill_id", "bill_split_shares.guest_session_id", "bill_split_shares.amount_cents", "bill_split_shares.status").
		Joins("JOIN bills ON bills.id = bill_split_shares.bill_id").
		Where("bill_split_shares.id = ? AND "+database.PublicBillTokenWhere, shareID, token).
		Take(&share).Error; err != nil {
		return nil, database.BillSplitShare{}, normalizedTender, err
	}
	if share.GuestSessionID != strings.TrimSpace(guestSessionID) {
		return nil, share, normalizedTender, database.ErrSplitGuestMismatch
	}

	payment, err := database.GetPaymentByTxHash(txHash)
	if err != nil {
		return nil, share, normalizedTender, database.ErrPaymentTxHashConflict
	}
	if payment.BillID != share.BillID ||
		payment.Status != database.PaymentStatusConfirmed ||
		payment.Amount != share.AmountCents ||
		payment.TipAmount != tipCents {
		return nil, share, normalizedTender, database.ErrPaymentTxHashConflict
	}

	return payment, share, normalizedTender, nil
}

// drainBusinessEvents discards every event already queued on ch without
// blocking and returns how many it dropped.
func drainBusinessEvents(ch <-chan events.BusinessEvent) int {
	n := 0
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return n
			}
			n++
		default:
			return n
		}
	}
}
