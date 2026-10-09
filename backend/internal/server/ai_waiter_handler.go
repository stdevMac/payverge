package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/pii"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

var (
	errAIWaiterBusinessUnavailable = errors.New("business not found")
	errAIWaiterInvalidMode         = errors.New("invalid mode")
	errAIWaiterTableRequired       = errors.New("table code is required for ordering mode")
	errAIWaiterTableNotFound       = errors.New("table not found")
	// Narrow test seam for deterministic inventory read failures. Production
	// always uses the canonical batched inventory-grounding query.
	loadUnrecommendableMenuItemIDs = database.UnrecommendableMenuItemIDs
	newAIWaiterResponseID          = func(conversationID uint) (string, error) {
		var entropy [16]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return "", err
		}
		return fmt.Sprintf("waiter-%d-%s", conversationID, hex.EncodeToString(entropy[:])), nil
	}
)

func aiWaiterModelWire(parts []gin.H, response assistantcontract.Response) gin.H {
	return gin.H{"role": "model", "parts": parts, "response_v2": response}
}

// Payload caps for POST /api/v1/ai-waiter/:businessId. These bound the
// per-request token cost we forward to the model on the business's behalf
// and constrain the prompt-injection surface. BusinessRateLimit(20) handles
// request frequency; these caps handle request size.
const (
	maxAIWaiterHistoryLen     = 40
	maxAIWaiterMessageBytes   = 4096
	maxAIWaiterBillContextLen = 2048
	maxAIWaiterModeLen        = 32
	maxAIWaiterMessagesFetch  = 100
)

type AIWaiterRequest struct {
	Language     string                   `json:"language"`
	SessionToken string                   `json:"session_token"` // explicit token; the HttpOnly cookie is used when empty
	TableCode    string                   `json:"table_code"`
	Mode         string                   `json:"mode"`         // ordering, concierge
	BillContext  string                   `json:"bill_context"` // e.g. "Previous orders: 2x Burger, 1x Coke (Total: $45)"
	History      []services.WaiterMessage `json:"history" binding:"required"`
}

type aiWaiterHistoryMessage struct {
	ID         uint                        `json:"id,omitempty"`
	Role       string                      `json:"role"`
	Content    string                      `json:"content"`
	CreatedAt  int64                       `json:"created_at"`
	ResponseV2 *assistantcontract.Response `json:"response_v2,omitempty"`
}

func legacyAiWaiterResponseV2(messageID uint, content, locale string, degraded bool) assistantcontract.Response {
	content = pii.Redact(content)
	if strings.TrimSpace(content) == "" {
		content = services.ClarifyItemMessage(locale)
		degraded = true
	}
	response := assistantcontract.NewResponse(fmt.Sprintf("waiter-message-%d", messageID), content)
	response.Answer.Format = assistantcontract.FormatPlainText
	if degraded {
		response.Status = assistantcontract.StatusDegraded
	}
	if err := assistantcontract.Validate(response); err != nil {
		response = assistantcontract.NewResponse(fmt.Sprintf("waiter-message-%d", messageID), services.ClarifyItemMessage(locale))
		response.Answer.Format = assistantcontract.FormatPlainText
		response.Status = assistantcontract.StatusDegraded
	}
	return response
}

// restoreAiWaiterResponseV2 accepts only a fully valid persisted V2 object.
// Legacy rows become non-clickable plain text. Malformed or contract-invalid
// structured data degrades the same readable text and never restores its
// model-owned actions, links, sources, or tool calls.
func restoreAiWaiterResponseV2(message database.AiWaiterMessage, locale string) assistantcontract.Response {
	raw := strings.TrimSpace(message.StructuredResponse)
	isLegacy := raw == "" || raw == "{}"
	if !isLegacy {
		var response assistantcontract.Response
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&response); err == nil {
			var trailing any
			if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) && response.Version == 2 {
				if err := assistantcontract.Validate(response); err == nil {
					return response
				}
			}
		}
		log.Printf("WARNING: ignored invalid AI waiter structured response for message %d", message.ID)
	}
	return legacyAiWaiterResponseV2(message.ID, message.Content, locale, !isLegacy)
}

func canonicalizeAiWaiterResponseV2(response assistantcontract.Response) (assistantcontract.Response, string, error) {
	if err := assistantcontract.Validate(response); err != nil {
		return assistantcontract.Response{}, "", err
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return assistantcontract.Response{}, "", err
	}
	normalized, err := database.NormalizeAiWaiterStructuredResponse(string(raw))
	if err != nil {
		return assistantcontract.Response{}, "", err
	}
	var canonical assistantcontract.Response
	decoder := json.NewDecoder(strings.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&canonical); err != nil {
		return assistantcontract.Response{}, "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return assistantcontract.Response{}, "", fmt.Errorf("canonical response has trailing JSON")
	}
	if err := assistantcontract.Validate(canonical); err != nil {
		return assistantcontract.Response{}, "", err
	}
	return canonical, normalized, nil
}

func waiterLegacyPartsFromV2(response assistantcontract.Response) ([]gin.H, string) {
	entityNames := make(map[string]string, len(response.Entities))
	for _, entity := range response.Entities {
		entityNames[entity.ID] = entity.DisplayName
	}
	approvedCalls := make([]llm.ToolCall, 0, len(response.Actions))
	for _, action := range response.Actions {
		if action.Type != "add_cart_item" || action.State != "ready" || action.Target.Quantity == nil {
			continue
		}
		itemName := entityNames[action.Target.Kind+":"+action.Target.ID]
		if strings.TrimSpace(itemName) == "" {
			continue
		}
		args := map[string]any{
			"item_type": action.Target.Kind,
			"item_name": itemName,
			"quantity":  *action.Target.Quantity,
		}
		switch action.Target.Kind {
		case waiterMenuEntityTypeMenuItem:
			args["menu_item_id"] = action.Target.ID
		case waiterMenuEntityTypeBundle:
			args["bundle_id"] = action.Target.ID
		default:
			continue
		}
		if action.Target.Notes != nil && *action.Target.Notes != "" {
			args["notes"] = *action.Target.Notes
		}
		approvedCalls = append(approvedCalls, llm.ToolCall{Name: "add_to_cart", Args: args})
	}
	parts := make([]gin.H, 0, len(approvedCalls)+1)
	if response.Answer.Content != "" {
		parts = append(parts, gin.H{"text": response.Answer.Content})
	}
	toolParts, toolCallsJSON := waiterToolRepresentations(approvedCalls)
	return append(parts, toolParts...), toolCallsJSON
}

func persistAndPublishAiWaiterV2(conv *database.AiWaiterConversation, response assistantcontract.Response, locale string) (uint, []gin.H, assistantcontract.Response, error) {
	canonical, structuredJSON, err := canonicalizeAiWaiterResponseV2(response)
	if err != nil {
		fallback := assistantcontract.NewResponse(response.ResponseID, services.ClarifyItemMessage(locale))
		fallback.Answer.Format = assistantcontract.FormatPlainText
		fallback.Status = assistantcontract.StatusDegraded
		canonical, structuredJSON, err = canonicalizeAiWaiterResponseV2(fallback)
		if err != nil {
			return 0, nil, assistantcontract.Response{}, err
		}
	}
	parts, toolCallsJSON := waiterLegacyPartsFromV2(canonical)
	if conv == nil || conv.ID == 0 {
		return 0, parts, canonical, nil
	}
	id, createdAt, err := database.SaveAiWaiterMessageReturningIDV2(
		conv.ID, "assistant", canonical.Answer.Content, toolCallsJSON, structuredJSON,
	)
	if err != nil {
		return 0, parts, canonical, err
	}
	publishAiWaiterMessageV2(conv, id, "assistant", canonical.Answer.Content, createdAt, &canonical)
	return id, parts, canonical, nil
}

// buildAuthoritativeWaiterHistory reconstructs the model-facing conversation
// history from the server-persisted transcript rather than the client's
// (forgeable) req.History. Only role + content from the DB reach the model, so a
// guest cannot inject fake assistant turns. fallbackUserMsg is appended only when
// the DB's newest turn isn't the current user message (i.e. persisting it failed
// upstream), so the model still sees the question being asked.
func buildAuthoritativeWaiterHistory(convID uint, fallbackUserMsg string) []services.WaiterMessage {
	var history []services.WaiterMessage
	if convID != 0 {
		msgs, err := database.GetRecentAiWaiterMessages(convID, maxAIWaiterHistoryLen)
		if err != nil {
			log.Printf("WARNING: failed to load AI waiter history for conversation %d: %v", convID, err)
		} else {
			for _, m := range msgs {
				if strings.TrimSpace(m.Content) == "" {
					continue // skip tool-only / empty turns
				}
				history = append(history, services.WaiterMessage{
					ID:        m.ID,
					Role:      m.Role,
					Content:   m.Content,
					CreatedAt: m.CreatedAt.Unix(),
				})
			}
		}
	}
	// Guarantee the current user turn is present even if its persist failed.
	if fallbackUserMsg != "" {
		if n := len(history); n == 0 || history[n-1].Role != "user" {
			history = append(history, services.WaiterMessage{Role: "user", Content: fallbackUserMsg})
		}
	}
	return history
}

func normalizeAIWaiterMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "ordering":
		return "ordering", nil
	case "concierge":
		return "concierge", nil
	default:
		return "", errAIWaiterInvalidMode
	}
}

// validateAIWaiterPublicScope normalizes the mode, enforces the ordering-mode
// table requirement, and resolves+scopes the table to the business in ONE
// lookup. It returns the resolved *database.Table so callers can reuse it
// (e.g. for server-side bill context) instead of fetching the same row again
// (L11). The table is nil for concierge mode / when no table code is supplied.
func validateAIWaiterPublicScope(business *database.Business, mode, tableCode string) (string, string, *database.Table, error) {
	if business == nil || !business.IsActive {
		return "", "", nil, errAIWaiterBusinessUnavailable
	}

	normalizedMode, err := normalizeAIWaiterMode(mode)
	if err != nil {
		return "", "", nil, err
	}

	trimmedTableCode := strings.TrimSpace(tableCode)
	if normalizedMode == "ordering" && trimmedTableCode == "" {
		return "", "", nil, errAIWaiterTableRequired
	}
	if trimmedTableCode == "" {
		return normalizedMode, "", nil, nil
	}

	table, err := database.GetTableByCode(trimmedTableCode)
	if err != nil || table == nil || table.BusinessID != business.ID {
		return "", "", nil, errAIWaiterTableNotFound
	}

	return normalizedMode, trimmedTableCode, table, nil
}

// respondIfAIWaiterBusinessLocked answers the guest AI Waiter surfaces for a
// locked venue. A suspended (is_active=false) business is indistinguishable
// from a missing one, matching every other public loader; an administrator-
// closed venue still resolves and gets 403 business_unavailable.
func respondIfAIWaiterBusinessLocked(c *gin.Context, business *database.Business) bool {
	if business == nil || !business.IsActive {
		respondAIWaiterPublicScopeError(c, errAIWaiterBusinessUnavailable)
		return true
	}
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently available",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return true
	}
	return false
}

func respondAIWaiterPublicScopeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errAIWaiterBusinessUnavailable):
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found", "code": ErrCodeBusinessNotFound})
	case errors.Is(err, errAIWaiterInvalidMode):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mode"})
	case errors.Is(err, errAIWaiterTableRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required for ordering mode"})
	case errors.Is(err, errAIWaiterTableNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid AI Waiter scope"})
	}
}

// HandleAIWaiter handles AI-powered concierge requests for guests
// POST /api/v1/ai-waiter/:businessId
func HandleAIWaiter(c *gin.Context) {
	telemetryStarted := time.Now()
	telemetryEvent := llm.AITelemetryEvent{
		Feature: "waiter", Surface: "waiter", ContractVersion: "v2",
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none", Language: "en",
		SchemaOutcome: "none", LanguageOutcome: "none",
	}
	var terminalLanguageOutcome string
	defer func() {
		telemetryEvent.LatencyMs = aiWaiterTelemetryMilliseconds(time.Since(telemetryStarted))
		if telemetryEvent.TimeToFirstMs > telemetryEvent.LatencyMs {
			telemetryEvent.LatencyMs = telemetryEvent.TimeToFirstMs
		}
		if telemetryEvent.Outcome == "" {
			telemetryEvent.Outcome = "error"
		}
		llm.EmitTelemetry(telemetryEvent)
	}()

	// Public route — no auth middleware resolves the business for us,
	// so we look it up here. Accepts either the numeric DB id or the
	// public business_id slug; the guest-facing AI Waiter URLs use the
	// slug (see /b/<custom-url> deep-links into the table page).
	businessIdentifier := utils.BusinessIdentifierFromParam(c, "businessId")
	if businessIdentifier == "" {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	resolvedBusiness, err := database.GetBusinessByIdOrBusinessId(businessIdentifier)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			telemetryEvent.Outcome = "invalid"
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found", "code": ErrCodeBusinessNotFound})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		}
		return
	}
	telemetryEvent.BusinessID = resolvedBusiness.ID
	businessID := uint64(resolvedBusiness.ID)

	var req AIWaiterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Stable product copy (no gin Key: dumps). history is required.
		telemetryEvent.Outcome = "invalid"
		RespondBindError(c, err)
		return
	}

	// Enforce payload caps before any downstream work (DB lookups, model call)
	// so oversize requests never reach the AI provider or consume query budget.
	if len(req.History) > maxAIWaiterHistoryLen {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "AI waiter history exceeds maximum allowed length",
			"code":  "payload_too_large",
		})
		return
	}
	for _, msg := range req.History {
		if len(msg.Content) > maxAIWaiterMessageBytes {
			telemetryEvent.Outcome = "invalid"
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "AI waiter message content exceeds maximum size",
				"code":  "payload_too_large",
			})
			return
		}
	}
	if len(req.BillContext) > maxAIWaiterBillContextLen {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "AI waiter bill context exceeds maximum size",
			"code":  "payload_too_large",
		})
		return
	}
	if len(req.Mode) > maxAIWaiterModeLen {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "AI waiter mode exceeds maximum length",
			"code":  "payload_too_large",
		})
		return
	}

	// 1. Use the already resolved business and fetch menu context.
	business := resolvedBusiness
	if respondIfAIWaiterBusinessLocked(c, business) {
		telemetryEvent.Outcome = "blocked"
		return
	}

	// Check if AI Waiter is enabled for this business
	if !business.AiSettings.AiEnabled {
		telemetryEvent.Outcome = "blocked"
		c.JSON(http.StatusForbidden, gin.H{"error": "AI Waiter is not enabled for this business"})
		return
	}

	operatorTest := isAIWaiterOperatorTestRequest(c)
	if !operatorTest && database.IsAiWaiterOperatorTestTableCode(req.TableCode) {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown session", "code": ErrCodeSessionUnknown})
		return
	}

	var scopeTable *database.Table
	if operatorTest {
		// Dashboard sandbox: force concierge + reserved table code. Never accept
		// client-supplied table/mode that could escape the isolation namespace.
		req.Mode = aiWaiterOperatorTestMode
		req.TableCode = database.AiWaiterOperatorTestTableCode
		if business == nil || !business.IsActive {
			telemetryEvent.Outcome = "invalid"
			respondAIWaiterPublicScopeError(c, errAIWaiterBusinessUnavailable)
			return
		}
	} else {
		req.Mode, req.TableCode, scopeTable, err = validateAIWaiterPublicScope(business, req.Mode, req.TableCode)
		if err != nil {
			telemetryEvent.Outcome = "invalid"
			respondAIWaiterPublicScopeError(c, err)
			return
		}
	}

	_, categories, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Menu not found for this business"})
		return
	}

	// 1.5 Handle Conversation Persistence. The guest UI sends the token as
	// session_token, falling back to the HttpOnly pv_ai_waiter_session cookie so the guest UI can
	// authenticate the session without holding a replayable bearer token in
	// JS-readable storage (localStorage/sessionStorage), which any XSS could
	// dump and replay to impersonate the guest's session.
	//
	// Operator sandbox never falls back to the guest cookie — that would let a
	// dashboard probe hitch onto (or overwrite semantics of) a real guest chat.
	var conv *database.AiWaiterConversation
	explicitToken := req.SessionToken
	var sessionToken string
	if operatorTest {
		sessionToken = strings.TrimSpace(explicitToken)
	} else {
		sessionToken = resolveAiWaiterSessionToken(c, explicitToken)
	}
	if sessionToken == "" {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown session", "code": ErrCodeSessionUnknown})
		return
	}
	found, ok := database.FindAiWaiterConversation(sessionToken, uint(businessID), req.Mode, req.TableCode)
	if !ok {
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown session", "code": ErrCodeSessionUnknown})
		return
	}
	conv = found
	if operatorTest != database.IsAiWaiterOperatorTestTableCode(conv.TableCode) {
		// Public guest path must not serve sandbox rows; sandbox path must not
		// accept real guest conversations.
		telemetryEvent.Outcome = "invalid"
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown session", "code": ErrCodeSessionUnknown})
		return
	}
	if aiWaiterSessionExpired(conv) {
		telemetryEvent.Outcome = "blocked"
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session expired", "code": "session_expired"})
		return
	}

	// Resolve the guest-facing language up front so both the paused (takeover)
	// ack and the downstream menu grounding use the same locale. A requested
	// locale that isn't a supported guest locale falls back to the business
	// default. Persist a locale change before the new user turn so Live Monitor
	// no longer freezes the conversation at its first-session language.
	businessDefaultLanguage, requestedLanguage, nextLanguageOutcome := resolveAIWaiterRequestLanguage(business, req.Language, &telemetryEvent)
	terminalLanguageOutcome = nextLanguageOutcome
	syncAiWaiterConversationLocale(conv, requestedLanguage)

	convID := conv.ID
	responseID, err := newAIWaiterResponseID(convID)
	if err != nil {
		log.Printf("ERROR: failed to mint AI waiter response id for conversation %d", convID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI assistant is temporarily unavailable"})
		return
	}

	// The latest guest turn (PII redacted at ingest) is persisted only once the
	// turn is admitted: the budget checks below run first, so a client
	// hammering an over-budget session cannot keep growing ai_waiter_messages
	// (SEC H-ai-cost). A paused conversation still records it for the staff
	// member who is answering, once the per-session and per-client caps admit it.
	var pendingUserTurn *string
	if len(req.History) > 0 {
		if lastMsg := req.History[len(req.History)-1]; lastMsg.Role == "user" {
			content := lastMsg.Content
			pendingUserTurn = &content
		}
	}
	saveUserTurn := func() {
		if pendingUserTurn == nil {
			return
		}
		if err := database.SaveAiWaiterMessage(convID, "user", pii.Redact(*pendingUserTurn), ""); err != nil {
			log.Printf("WARNING: failed to save AI waiter message for conversation %d: %v", convID, err)
		} else if !operatorTest {
			// Operator sandbox must not burn the guest daily message quota.
			database.IncrementDailyAiWaiterCount(uint(businessID), time.Now())
		}
	}

	// Before honoring a pause, apply the same stale-claim release semantics as
	// the dashboard sweep — but for THIS conversation only. The sweep runs only
	// when staff open the conversations list, so without this a guest whose
	// claimer walked away would get the "human is assisting you" ack forever.
	// A pause with NO claim (claimed_at NULL — deliberate manual operator
	// pause) is left intact. Single conditional UPDATE; almost always a no-op.
	if conv.IsPaused && conv.ClaimedAt != nil {
		idleCutoff := time.Now().Add(-aiClaimIdleTTL)
		res := database.GetDB().Model(&database.AiWaiterConversation{}).
			Where("id = ? AND claimed_at IS NOT NULL AND claimed_at < ?", conv.ID, idleCutoff).
			Updates(map[string]interface{}{
				"claimed_by_staff_id": nil, "claimed_by_name": "", "claimed_by_role": "",
				"claimed_at": nil, "is_paused": false,
			})
		if res.Error != nil {
			log.Printf("WARNING: failed to release stale claim for conversation %d: %v", conv.ID, res.Error)
		} else if res.RowsAffected > 0 {
			// Refresh the in-memory row so the paused branch below is skipped and
			// the AI answers again.
			conv.IsPaused = false
			conv.ClaimedByStaffID = nil
			conv.ClaimedByName = ""
			conv.ClaimedByRole = ""
			conv.ClaimedAt = nil
			// The guest is back with the AI — the takeover alert is moot.
			resolveAITakeoverAlertQuietly(c.Request.Context(), conv.BusinessID, conv.ID,
				operational_alerts.Actor{Name: "system"})
		}
	}

	var pendingTurns int64
	if pendingUserTurn != nil {
		pendingTurns = 1
	}

	// Check if AI is paused (Takeover mode)
	if conv.IsPaused {
		// No model call happens here, but every admitted turn still writes a
		// row and can raise a takeover alert (event row + SSE fan-out), so hold
		// paused turns to a per-session row cap and to the same per-network /
		// per-device daily ceilings as AI turns. The per-business AI message
		// count and the USD ceilings are not checked: a human is answering.
		if database.CountAiWaiterMessages(convID)+pendingTurns >= maxPausedAIWaiterMessagesPerSession ||
			(!operatorTest && !takeGuestAIQuota(c)) {
			telemetryEvent.Outcome = "blocked"
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "AI assistant is busy, please try again later", "code": "over_budget"})
			return
		}
		saveUserTurn()
		telemetryEvent.Outcome = "blocked"
		// A paused conversation only has a human answering while a claim is
		// actively held. Same semantics as the claim handlers: a claim idle
		// past the TTL counts as unclaimed. Owner claims write a fresh
		// claimed_at with a NULL claimed_by_staff_id, so the freshness of
		// claimed_at — not the staff id — is what marks a claim as held.
		// If nobody holds the claim, this guest message would land in a void —
		// raise an urgent takeover alert.
		claimActive := conv.ClaimedAt != nil &&
			conv.ClaimedAt.After(time.Now().Add(-aiClaimIdleTTL))
		if !claimActive {
			// Detach from the request context: a guest disconnect mid-request
			// is exactly the case where the operator still needs the signal.
			if err := operational_alerts.NewService(database.GetDB()).
				CreateAITakeoverAlert(context.WithoutCancel(c.Request.Context()), conv.BusinessID, int64(conv.ID)); err != nil {
				log.Printf("WARNING: failed to create AI takeover alert for conversation %d: %v", conv.ID, err)
			}
		}

		// While paused, do NOT echo the latest assistant DB row back to the guest.
		// The staff reply already reaches the guest client over the SSE channel;
		// re-emitting it here (with no message id) made the client append it as a
		// duplicate bubble and replay pre-pause AI answers. Instead return a
		// localized, idempotent "a human is assisting you" acknowledgement that the
		// guest client renders as an ephemeral system notice (not a chat bubble),
		// so it never duplicates the SSE-delivered staff reply. (R3-AI-1, R3-AI-4)
		humanMessage := services.WaiterHumanAssisting(requestedLanguage)
		response := assistantcontract.NewResponse(responseID, humanMessage)
		response.Answer.Format = assistantcontract.FormatPlainText
		response.Status = assistantcontract.StatusBlocked
		applyAIWaiterResponseTelemetry(&telemetryEvent, response, terminalLanguageOutcome, time.Since(telemetryStarted))
		out := aiWaiterModelWire([]gin.H{
			{
				"text": humanMessage,
			},
		}, response)
		out["human_ack"] = true
		out["is_paused"] = true
		c.JSON(http.StatusOK, out)
		return
	}

	// Budget enforcement BEFORE the turn is persisted (per-session +
	// per-business daily message count counting the pending turn, the
	// business's guest-scope daily USD ceiling plus the instance-wide one, and
	// the per-network / per-device daily caps; 429 over_budget). The guest
	// scope is separate from the owner's, so guests can never exhaust the
	// owner's own AI tools. Operator sandbox skips the guest daily message
	// quota and the per-client caps; it still honors the per-session cap and
	// the USD ceilings (its model calls are charged to the guest scope).
	overBudget := false
	if operatorTest {
		overBudget = database.CountAiWaiterMessages(convID)+pendingTurns >= maxAIWaiterMessagesPerSession ||
			guestAIOverDollarBudget(uint(businessID))
	} else {
		overBudget = aiWaiterOverBudget(convID, uint(businessID), pendingTurns) ||
			guestAIOverDollarBudget(uint(businessID)) ||
			!takeGuestAIQuota(c)
	}
	if overBudget {
		telemetryEvent.Outcome = "blocked"
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "AI assistant is busy, please try again later", "code": "over_budget"})
		return
	}
	saveUserTurn()

	// 2. Prepare Menu Context (with translations if applicable). The guest-facing
	// language (requestedLanguage / businessDefaultLanguage) was resolved above,
	// before the paused-takeover branch, so the takeover ack and the menu
	// grounding share one locale.
	displayCategories := categories
	if requestedLanguage != businessDefaultLanguage {
		displayCategories, _ = applyTranslationsToMenu(business.ID, categories, requestedLanguage)
	}

	activeOffers, activeBundles := getActivePromotionsForBusiness(business)
	displayOffers := activeOffers
	displayBundles := activeBundles
	if requestedLanguage != businessDefaultLanguage {
		missingOfferTranslations := false
		missingBundleTranslations := false
		displayOffers, missingOfferTranslations = applyTranslationsToOffers(activeOffers, requestedLanguage)
		displayBundles, missingBundleTranslations = applyTranslationsToBundles(activeBundles, requestedLanguage)
		if missingOfferTranslations || missingBundleTranslations {
			schedulePromotionTranslationBackfill(business.ID, businessDefaultLanguage, requestedLanguage, activeOffers, activeBundles)
		}
	}

	// Build one canonical, localized menu identity after the existing batched
	// translation, promotion, stock, hours and ordering-toggle resolution. The
	// same snapshot owns prompt data, allergen facts and cart validation; no
	// downstream path independently reconstructs menu identity from raw slices.
	soldOutItemIDs, herr := loadUnrecommendableMenuItemIDs(business.ID)
	if herr != nil {
		log.Printf("AI waiter stock-grounding failed closed (business=%d): %v", business.ID, herr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Menu context is unavailable"})
		return
	}
	businessOpen := services.BusinessOpenAt(database.GetDB(), business, time.Now())
	menuSnapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: requestedLanguage, Mode: req.Mode, BusinessOpen: businessOpen,
		Categories: displayCategories, Offers: displayOffers, Bundles: displayBundles,
		SourceCategories: categories, SourceBundles: activeBundles,
		SoldOutItemIDs: soldOutItemIDs, TrustedImageHost: s3.PublicHost(),
	})
	if err != nil {
		log.Printf("AI waiter menu snapshot failed (business=%d, mode=%q, locale=%q): %v", business.ID, req.Mode, requestedLanguage, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Menu context is unavailable"})
		return
	}
	// 3. Prepare Business Context
	resvSettings, _ := database.GetReservationSettings(uint(businessID))
	reservationContext := "Reservations are DISABLED. Guests cannot book tables through the AI."
	if resvSettings != nil && resvSettings.Enabled && database.IsBusinessOperational(business) {
		reservationContext = fmt.Sprintf("Reservations are ENABLED. Guests can book for parties of %d to %d people. Please encourage them to use the 'Reserve a Table' button on the page.", resvSettings.MinPartySize, resvSettings.MaxPartySize)
	}

	var deliveryContext string
	if deliveryService := GetDeliveryService(); deliveryService != nil {
		delSettings, _ := deliveryService.GetDeliverySettings(uint(businessID))
		if delSettings != nil && !database.IsBusinessOperational(business) {
			copy := *delSettings
			copy.InHouseDeliveryEnabled = false
			delSettings = &copy
		}
		deliveryContext = buildDeliveryContext(delSettings)
	}

	businessAddress := fmt.Sprintf("%s, %s, %s %s, %s",
		business.Address.Street, business.Address.City, business.Address.State, business.Address.PostalCode, business.Address.Country)

	// Deprecation: client-supplied bill_context is ignored; bill context is now
	// server-computed from the table's open bill (see Task 11).
	if strings.TrimSpace(req.BillContext) != "" {
		log.Printf("DEPRECATION: client-supplied bill_context ignored (business=%d); bill context is server-computed", businessID)
	}

	// Build server bill context from the table's open bill (ordering mode only).
	// Bill prices are rendered in the business's own currency so the assistant
	// never echoes a hardcoded "$" to a non-USD operator (audit ai-waiter-hardcoded-currency).
	serverBillContext := ""
	// billMoney carries the same open bill as a number so the finalizer can
	// split it or add a tip without asking the model to do arithmetic (issue
	// 943). TotalAmount is the DB wire shape: int64 cents.
	billMoney := waiterBillMoney{}
	if req.Mode == "ordering" && scopeTable != nil {
		if bill, items, berr := database.GetOpenBillSummaryAndItemsByTableID(scopeTable.ID); berr == nil {
			currency := resolveBusinessCurrency(business)
			serverBillContext = buildServerBillContext(bill, items, currency)
			if bill != nil {
				billMoney = waiterBillMoney{TotalCents: bill.TotalAmount, Currency: currency}
			}
		}
	}

	// 4. Allergen intent interception BEFORE the LLM call
	var lastUserMsg string
	if len(req.History) > 0 {
		last := req.History[len(req.History)-1]
		if last.Role == "user" {
			lastUserMsg = last.Content
		}
	}

	// Guardrails on the inbound guest message. The classifier's strict flag
	// (resolveGuardrailStrict) decides the failure mode: fail-closed in production,
	// fail-open in dev. Classify never returns an error — strict mode encodes a
	// classifier failure as Allowed=false, so the block below covers both modes.
	if lastUserMsg != "" {
		gctx, gcancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		verdict, gerr := aiWaiterClassifier.Classify(gctx, guardrails.ClassifyRequest{
			Surface: guardrails.SurfaceAIWaiter, BusinessID: uint(businessID), Locale: requestedLanguage, Text: lastUserMsg,
		})
		gcancel()
		if gerr == nil && !verdict.Allowed {
			telemetryEvent.Outcome = "blocked"
			msg := services.WaiterOffTopicRedirect(requestedLanguage)
			if verdict.Category == "abuse" {
				msg = services.WaiterAbuseDecline(requestedLanguage)
			}
			response := assistantcontract.NewResponse(responseID, msg)
			response.Answer.Format = assistantcontract.FormatPlainText
			response.Status = assistantcontract.StatusBlocked
			applyAIWaiterResponseTelemetry(&telemetryEvent, response, terminalLanguageOutcome, time.Since(telemetryStarted))
			messageID, parts, response, saveErr := persistAndPublishAiWaiterV2(conv, response, requestedLanguage)
			if saveErr != nil {
				log.Printf("WARNING: failed to save AI waiter blocked response for conversation %d: %v", convID, saveErr)
			}
			out := aiWaiterModelWire(parts, response)
			if messageID != 0 {
				out["id"] = messageID
			}
			c.JSON(http.StatusOK, out)
			return
		}
	}

	orderingBrowseOnly := req.Mode == "ordering" && !menuSnapshot.OrderingOpen
	visitFacts := waiterVisitFactsFromContext(business, resvSettings, serverBillContext)
	if lastUserMsg != "" && !waiterNeedsModel(requestedLanguage, lastUserMsg, menuSnapshot) {
		finalized := finalizeWaiterForHandler(WaiterFinalizeInput{
			ResponseID:         responseID,
			Locale:             requestedLanguage,
			Mode:               req.Mode,
			UserMessage:        lastUserMsg,
			Snapshot:           menuSnapshot,
			OrderingBrowseOnly: orderingBrowseOnly,
			Visit:              visitFacts,
			Bill:               billMoney,
		})
		applyAIWaiterResponseTelemetry(&telemetryEvent, finalized, terminalLanguageOutcome, time.Since(telemetryStarted))
		messageID, parts, finalized, saveErr := persistAndPublishAiWaiterV2(conv, finalized, requestedLanguage)
		if saveErr != nil {
			log.Printf("WARNING: failed to save AI waiter deterministic response for conversation %d: %v", convID, saveErr)
		}
		out := aiWaiterModelWire(parts, finalized)
		if messageID != 0 {
			out["id"] = messageID
		}
		c.JSON(http.StatusOK, out)
		return
	}

	// 5. Call AI Service
	ai := GetAIService()
	if ai == nil {
		log.Printf("AI waiter service unset (business=%d) — answering from the menu snapshot", business.ID)
		finalized := finalizeWaiterForHandler(WaiterFinalizeInput{
			ResponseID:         responseID,
			Locale:             requestedLanguage,
			Mode:               req.Mode,
			UserMessage:        lastUserMsg,
			Snapshot:           menuSnapshot,
			OrderingBrowseOnly: orderingBrowseOnly,
			Visit:              visitFacts,
			Bill:               billMoney,
		})
		applyAIWaiterResponseTelemetry(&telemetryEvent, finalized, terminalLanguageOutcome, time.Since(telemetryStarted))
		messageID, parts, finalized, saveErr := persistAndPublishAiWaiterV2(conv, finalized, requestedLanguage)
		if saveErr != nil {
			log.Printf("WARNING: failed to save AI waiter fallback response for conversation %d: %v", convID, saveErr)
		}
		out := aiWaiterModelWire(parts, finalized)
		if messageID != 0 {
			out["id"] = messageID
		}
		c.JSON(http.StatusOK, out)
		return
	}

	// Use defaults if not set
	aiName := business.AiSettings.AiName
	if aiName == "" {
		aiName = "Sage"
	}

	modelCtx, modelCancel := context.WithTimeout(c.Request.Context(), llm.FeatureTimeout("waiter"))
	defer modelCancel()

	resp, err := ai.ChatWithWaiter(modelCtx, services.WaiterChatParams{
		AIName:              aiName,
		AIPriority:          business.AiSettings.AiPriority,
		SpecialInstructions: business.AiSettings.SpecialInstructions,
		BillContext:         serverBillContext,
		BusinessName:        business.Name,
		BusinessDescription: business.Description,
		BusinessAddress:     businessAddress,
		ReservationContext:  reservationContext,
		DeliveryContext:     deliveryContext,
		MenuData:            menuSnapshot.PromptMenuJSON(),
		OffersData:          menuSnapshot.PromptOffersJSON(),
		BundlesData:         menuSnapshot.PromptBundlesJSON(),
		Language:            requestedLanguage,
		Mode:                req.Mode,
		// Authoritative history from the server-persisted transcript — NEVER the
		// client's req.History, which a guest can forge (e.g. fake "assistant"
		// turns that grant discounts or override policy). The current user
		// message was just persisted above, so DB history already includes it;
		// lastUserMsg is only a fallback if that persist failed.
		History:    buildAuthoritativeWaiterHistory(convID, lastUserMsg),
		BusinessID: business.ID,
	})
	if err != nil || resp == nil {
		if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(modelCtx.Err(), context.DeadlineExceeded)) {
			telemetryEvent.Outcome = "timeout"
		}
		log.Printf("AI waiter chat failed (business=%d, mode=%q): %v — answering from the menu snapshot", businessID, req.Mode, err)
		finalized := finalizeWaiterForHandler(WaiterFinalizeInput{
			ResponseID:         responseID,
			Locale:             requestedLanguage,
			Mode:               req.Mode,
			UserMessage:        lastUserMsg,
			Snapshot:           menuSnapshot,
			OrderingBrowseOnly: orderingBrowseOnly,
			Visit:              visitFacts,
			Bill:               billMoney,
		})
		applyAIWaiterResponseTelemetry(&telemetryEvent, finalized, terminalLanguageOutcome, time.Since(telemetryStarted))
		messageID, parts, finalized, saveErr := persistAndPublishAiWaiterV2(conv, finalized, requestedLanguage)
		if saveErr != nil {
			log.Printf("WARNING: failed to save AI waiter fallback response for conversation %d: %v", convID, saveErr)
		}
		out := aiWaiterModelWire(parts, finalized)
		if messageID != 0 {
			out["id"] = messageID
		}
		c.JSON(http.StatusOK, out)
		return
	}
	telemetryEvent.ToolCalls = len(resp.ToolCalls)
	if telemetryEvent.ToolCalls > 64 {
		telemetryEvent.ToolCalls = 64
	}

	// 5. Validate tool calls BEFORE any wire/persistence representation.
	// Model output is semi-trusted: only menu-resolved cart calls may reach
	// the guest client or the saved ToolCalls column.
	validatedCalls, dropped := validateCartToolCalls(
		resp.ToolCalls,
		menuSnapshot.OrderableMenuCategories(),
		menuSnapshot.OrderableBundles(),
	)

	// Closed Mode / kitchen-off defense-in-depth: never wire add_to_cart when
	// the guest cannot order. The V2 finalizer owns this decision so the same
	// validated call can become a blocked explanation without ever becoming an
	// executable legacy part.
	finalized := finalizeWaiterForHandler(WaiterFinalizeInput{
		ResponseID:         responseID,
		Locale:             requestedLanguage,
		Mode:               req.Mode,
		UserMessage:        lastUserMsg,
		ModelText:          resp.Text,
		ValidatedCalls:     validatedCalls,
		Snapshot:           menuSnapshot,
		OrderingBrowseOnly: orderingBrowseOnly,
		Visit:              visitFacts,
		Bill:               billMoney,
	})
	if dropped > 0 && len(validatedCalls) == 0 && explicitWaiterCartIntent(requestedLanguage, lastUserMsg) &&
		!waiterVisitQuestionTurn(requestedLanguage, lastUserMsg, menuSnapshot) {
		finalized = finalizeRejectedWaiterCart(finalized.ResponseID, requestedLanguage, orderingBrowseOnly)
	}
	if dropped > 0 {
		telemetryEvent.ActionOutcome = "rejected"
	}
	applyAIWaiterResponseTelemetry(&telemetryEvent, finalized, terminalLanguageOutcome, time.Since(telemetryStarted))
	assistantText := finalized.Answer.Content
	if dropped > 0 {
		log.Printf("AI waiter dropped %d ungrounded tool call(s) (business=%d, response=%s)", dropped, businessID, finalized.ResponseID)
	}

	// Build the legacy {role,parts:[...]} shape the guest AiWaiter expects from
	// the already validated V2 answer and its approved compatibility calls.
	var parts []gin.H

	// Observability: an empty assistant text with no tool calls is the upstream
	// condition behind the guest-side empty-response retry prompt. Log it with
	// provider/locale context so the root cause is visible in prod. (audit A1)
	if assistantText == "" && len(finalized.Actions) == 0 {
		log.Printf("AI waiter empty response: no text and no tool calls (business=%d, mode=%q, lang=%q) — guest will see the retry prompt",
			businessID, req.Mode, requestedLanguage)
	}

	var assistantMessageID uint
	// Don't persist an empty assistant turn (no usable text and no tool calls):
	// a saved blank row would echo back as a blank bubble on every reconnect.
	if convID != 0 && (assistantText != "" || len(finalized.Actions) > 0) {
		var saveErr error
		assistantMessageID, parts, finalized, saveErr = persistAndPublishAiWaiterV2(conv, finalized, requestedLanguage)
		if saveErr != nil {
			log.Printf("WARNING: failed to save AI waiter message for conversation %d: %v", convID, saveErr)
		}
	} else {
		parts, _ = waiterLegacyPartsFromV2(finalized)
	}

	// Return the persisted assistant message id so the guest client can tag its
	// optimistic bubble and dedupe the SSE/poll echo of the same row by id,
	// instead of fragile content matching that doubles bubbles on a race. (audit E4)
	out := aiWaiterModelWire(parts, finalized)
	if assistantMessageID != 0 {
		out["id"] = assistantMessageID
	}
	c.JSON(http.StatusOK, out)
}

func aiWaiterTelemetryMilliseconds(duration time.Duration) int64 {
	const maximum = int64((30 * time.Minute) / time.Millisecond)
	milliseconds := duration.Milliseconds()
	if milliseconds < 0 {
		return 0
	}
	if milliseconds > maximum {
		return maximum
	}
	return milliseconds
}

func applyAIWaiterResponseTelemetry(event *llm.AITelemetryEvent, response assistantcontract.Response, languageOutcome string, firstContentElapsed ...time.Duration) {
	event.V2ShadowValid = false
	if err := assistantcontract.Validate(response); err != nil {
		event.SchemaOutcome = "dropped"
		event.Outcome = "invalid"
		return
	}
	event.V2ShadowValid = true
	event.SchemaOutcome = "verified"
	renderableText := aiWaiterTelemetryRenderableText(response)
	event.LanguageOutcome = llm.ValidatedResponseLanguageOutcome(event.Language, renderableText, languageOutcome)
	if strings.TrimSpace(renderableText) != "" && len(firstContentElapsed) > 0 {
		event.TimeToFirstMs = aiWaiterTelemetryFirstContentMilliseconds(firstContentElapsed[0])
	}
	if event.Outcome == "" {
		switch response.Status {
		case assistantcontract.StatusComplete:
			event.Outcome = "ok"
		case assistantcontract.StatusDegraded:
			event.Outcome = "fallback"
		case assistantcontract.StatusBlocked:
			event.Outcome = "blocked"
		case assistantcontract.StatusNeedsClarification:
			event.Outcome = "fallback"
		default:
			event.Outcome = "error"
		}
	}
	if event.ActionOutcome == "none" && len(response.Actions) > 0 {
		event.ActionOutcome = "offered"
	}
	if event.SourceOutcome == "none" && len(response.Sources) > 0 {
		event.SourceOutcome = "verified"
	}
	if event.EntityOutcome == "none" && len(response.Entities) > 0 {
		event.EntityOutcome = "verified"
	}
}

func aiWaiterTelemetryRenderableText(response assistantcontract.Response) string {
	parts := []string{response.Answer.Content}
	parts = append(parts, response.Steps...)
	for _, section := range response.Sections {
		parts = append(parts, section.Title, section.Answer)
		parts = append(parts, section.Steps...)
	}
	return strings.Join(parts, "\n")
}

func aiWaiterTelemetryFirstContentMilliseconds(duration time.Duration) int64 {
	milliseconds := aiWaiterTelemetryMilliseconds(duration)
	if duration > 0 && milliseconds == 0 {
		return 1
	}
	return milliseconds
}

// GetAiWaiterMessages retrieves conversation history for a specific session ID (Guest side)
// GET /api/v1/ai-waiter/:businessId/messages?session_token=...&since=...
func GetAiWaiterMessages(c *gin.Context) {
	businessIdentifier := utils.BusinessIdentifierFromParam(c, "businessId")
	if businessIdentifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	resolvedBusiness, err := database.GetBusinessByIdOrBusinessId(businessIdentifier)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found", "code": ErrCodeBusinessNotFound})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		}
		return
	}
	businessID := uint64(resolvedBusiness.ID)

	// The guest UI (AiWaiter.tsx) and the /stream endpoint send the token as
	// `session_token`, falling back to the HttpOnly pv_ai_waiter_session cookie (see
	// resolveAiWaiterSessionToken) so the token need not ride the query string
	// (which leaks into proxy/CDN logs) or JS-readable storage (XSS-stealable).
	sessionID := resolveAiWaiterSessionToken(c, c.Query("session_token"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session token is required"})
		return
	}

	sinceStr := c.Query("since") // ISO8601 or unix timestamp
	mode := c.Query("mode")
	if mode == "" {
		mode = "ordering"
	}
	language := strings.TrimSpace(c.Query("language"))
	if language == "" || !locales.IsGuestLocale(language) {
		language = strings.TrimSpace(resolvedBusiness.DefaultLanguage)
	}
	if language == "" || !locales.IsGuestLocale(language) {
		language = "en"
	}

	business := resolvedBusiness
	if respondIfAIWaiterBusinessLocked(c, business) {
		return
	}
	if !business.AiSettings.AiEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "AI Waiter is not enabled for this business"})
		return
	}

	db := database.GetDB()
	var conv database.AiWaiterConversation
	tableCode := c.Query("table_code")
	mode, tableCode, _, err = validateAIWaiterPublicScope(business, mode, tableCode)
	if err != nil {
		respondAIWaiterPublicScopeError(c, err)
		return
	}
	err = db.Where("session_id = ? AND business_id = ? AND mode = ? AND table_code = ?", sessionID, businessID, mode, tableCode).First(&conv).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown session", "code": ErrCodeSessionUnknown})
		return
	}
	if database.IsAiWaiterOperatorTestTableCode(conv.TableCode) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown session", "code": ErrCodeSessionUnknown})
		return
	}
	if aiWaiterSessionExpired(&conv) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session expired", "code": "session_expired"})
		return
	}

	var sinceTime *time.Time
	if sinceStr != "" {
		// Support simple unix timestamp or ISO date
		if unix, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			parsed := time.Unix(unix, 0)
			sinceTime = &parsed
		} else if parsedRFC3339, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			parsed := parsedRFC3339
			sinceTime = &parsed
		}
	}

	limit := maxAIWaiterMessagesFetch
	if requestedLimit, err := strconv.Atoi(c.Query("limit")); err == nil && requestedLimit > 0 && requestedLimit < limit {
		limit = requestedLimit
	}

	messages, err := database.GetAiWaiterMessagesForGuest(conv.ID, sinceTime, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch messages"})
		return
	}

	// Restore only validated server-persisted V2. Assistant rows written before
	// structured persistence are projected as safe plain text; user rows do not
	// carry an assistant response object.
	var result []aiWaiterHistoryMessage
	for _, m := range messages {
		item := aiWaiterHistoryMessage{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt.Unix(),
		}
		if m.Role == "assistant" {
			response := restoreAiWaiterResponseV2(m, language)
			item.Content = response.Answer.Content
			item.ResponseV2 = &response
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, result)
}

// resolveBusinessCurrency picks the customer-facing currency code for a
// business, preferring DisplayCurrency, then DefaultCurrency, then "USD". This
// mirrors the resolution used elsewhere (see business_handlers.go).
func resolveAIWaiterRequestLanguage(business *database.Business, requested string, telemetryEvent *llm.AITelemetryEvent) (businessDefault, resolved, languageOutcome string) {
	businessDefault = strings.TrimSpace(business.DefaultLanguage)
	if businessDefault == "" {
		businessDefault = "en"
	}
	resolved = strings.TrimSpace(requested)
	requestedTelemetryLanguage := resolved
	if resolved == "" || !locales.IsGuestLocale(resolved) {
		resolved = businessDefault
	}
	languageOutcome = "dropped"
	if locale, ok := locales.Lookup(resolved); ok {
		telemetryEvent.Language = locale.Canonical
		languageOutcome = "none"
		if requestedTelemetryLanguage != "" && requestedTelemetryLanguage != locale.Canonical {
			languageOutcome = "dropped"
		}
	}
	if rl := strings.TrimSpace(requested); rl != "" && rl != resolved {
		log.Printf("AI waiter locale fallback: requested=%q is not a guest locale, resolved=%q (business=%d, default=%q)",
			rl, resolved, business.ID, businessDefault)
	}
	return businessDefault, resolved, languageOutcome
}

func syncAiWaiterConversationLocale(conv *database.AiWaiterConversation, locale string) {
	if conv == nil {
		return
	}
	locale = strings.TrimSpace(locale)
	if locale == "" || conv.Language == locale {
		return
	}
	from := conv.Language
	if err := database.SaveAiWaiterMessage(conv.ID, "system", services.WaiterLanguageTransition(from, locale), ""); err != nil {
		log.Printf("WARNING: failed to persist AI waiter language transition for conversation %d: %v", conv.ID, err)
	}
	if err := database.UpdateAiWaiterConversationLanguage(conv.ID, locale); err != nil {
		log.Printf("WARNING: failed to update AI waiter conversation language for conversation %d: %v", conv.ID, err)
		return
	}
	conv.Language = locale
}

func resolveBusinessCurrency(business *database.Business) string {
	if business != nil {
		if c := strings.TrimSpace(business.DisplayCurrency); c != "" {
			return c
		}
		if c := strings.TrimSpace(business.DefaultCurrency); c != "" {
			return c
		}
	}
	return "USD"
}

// aiWaiterCurrencySymbol maps an ISO 4217 code to a display glyph, falling back
// to the uppercased code plus a space (e.g. "AED 50.00") so the assistant's
// bill context reads in the business's own currency instead of a hardcoded "$".
func aiWaiterCurrencySymbol(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY", "CNY":
		return "¥"
	case "CAD":
		return "C$"
	case "AUD":
		return "A$"
	case "":
		return "$"
	default:
		return strings.ToUpper(strings.TrimSpace(code)) + " "
	}
}

func waiterNeedsModel(locale, userMessage string, snapshot WaiterMenuSnapshot) bool {
	if services.WaiterJokePrompt(locale, userMessage) {
		return true
	}
	if !explicitWaiterCartIntent(locale, userMessage) {
		return false
	}
	intent, matched := classifyWaiterV2Intent(locale, userMessage, snapshot)
	if len(matched) == 1 && !matched[0].Available {
		return false
	}
	// A visit question that merely borrows cart vocabulary ("can I order at
	// 10:30pm?") is answered entirely from snapshot and visit facts, so calling
	// the model only creates the stray add_to_cart that issue 867 reported.
	if waiterInformationalIntent(intent) && len(matched) == 0 {
		return false
	}
	return true
}

func waiterVisitFactsFromContext(business *database.Business, resv *database.ReservationSettings, billSummary string) services.WaiterVisitFacts {
	facts := services.WaiterVisitFacts{
		HasOpenBill: strings.TrimSpace(billSummary) != "",
		BillSummary: strings.TrimSpace(billSummary),
	}
	if business == nil {
		return facts
	}
	hours, err := database.GetBusinessOperatingHours(business.ID)
	if err == nil {
		open, closeTime, closed, known := services.BuildWaiterHoursFacts(hours, business.Timezone, time.Now())
		facts.HoursKnown = known
		facts.TodayClosed = closed
		facts.TodayOpen = open
		facts.TodayClose = closeTime
	}
	if resv != nil && resv.Enabled && database.IsBusinessOperational(business) {
		facts.Reservations = true
		facts.PartyMin = resv.MinPartySize
		facts.PartyMax = resv.MaxPartySize
	}
	// #903: the guest waiter used to be handed buildDeliveryContext's output —
	// the model's English system-prompt fragment — and read it out verbatim.
	// Carry the venue's actual delivery configuration instead; the finalizer
	// turns it into guest copy in the guest's own language.
	if deliveryService := GetDeliveryService(); deliveryService != nil {
		delSettings, derr := deliveryService.GetDeliverySettings(business.ID)
		if derr != nil {
			// Leave DeliveryKnown false so the assistant defers to staff
			// instead of claiming the venue does or does not deliver.
			log.Printf("WARNING: failed to read delivery settings for business %d: %v", business.ID, derr)
		} else if delSettings != nil {
			// Both arms of the canonical public storefront gate, in the same
			// order: the waiter must never advertise a channel the storefront
			// itself hides.
			if !database.IsBusinessOperational(business) {
				// Keep the marketplace partners — they still take the order —
				// but do not advertise in-house checkout, whose quote/create
				// path refuses (403) for a business that is not operational.
				copySettings := *delSettings
				copySettings.InHouseDeliveryEnabled = false
				delSettings = &copySettings
			} else if delSettings.InHouseDeliveryEnabled && !deliveryService.InHouseDeliveryIsReachable(business.ID) {
				// No active zone can match an address (#714), so in-house
				// checkout cannot take the order. Quoting a delivery fee here
				// would be a priced promise the guest cannot redeem; the
				// marketplace partners stay.
				copySettings := *delSettings
				copySettings.InHouseDeliveryEnabled = false
				delSettings = &copySettings
			}
			facts.DeliveryKnown = true
			facts.DeliveryEnabled = delSettings.DeliveryEnabled
			facts.DeliveryInHouse = delSettings.InHouseDeliveryEnabled
			facts.DeliveryPartners = services.WaiterDeliveryPartnerNames(delSettings)
			if delSettings.InHouseDeliveryEnabled {
				// FlatDeliveryFee is int64 cents; format once here in the
				// venue's own currency, the way BillSummary already is.
				facts.DeliveryFeeLabel = fmt.Sprintf("%s%.2f",
					aiWaiterCurrencySymbol(resolveBusinessCurrency(business)),
					float64(delSettings.FlatDeliveryFee)/100.0)
			}
		}
	}
	return facts
}

func buildServerBillContext(bill *database.Bill, items []database.BillItem, currency string) string {
	if bill == nil || len(items) == 0 {
		return ""
	}
	sym := aiWaiterCurrencySymbol(currency)
	var b strings.Builder
	for _, it := range items {
		// BillItem.Subtotal is a float64 already in dollars (no cents conversion).
		fmt.Fprintf(&b, "- %dx %s (%s%.2f)\n", it.Quantity, it.Name, sym, it.Subtotal)
	}
	// Bill.TotalAmount is int64 cents; divide to dollars.
	fmt.Fprintf(&b, "Total: %s%.2f", sym, float64(bill.TotalAmount)/100.0)
	return b.String()
}

// filterOffersByHiddenItems drops item-targeted offers that center on a hidden
// menu item (e.g. "$5 off the Steak Plate" while the steak is 86'd). Broad
// offers (applicable to "all" or a category) are left untouched.
func filterOffersByHiddenItems(offers []database.Offer, hidden map[string]bool) []database.Offer {
	if len(hidden) == 0 {
		return offers
	}
	out := make([]database.Offer, 0, len(offers))
	for _, offer := range offers {
		if offer.ApplicableTo == "item" && offer.TargetID != nil && hidden[strings.TrimSpace(*offer.TargetID)] {
			continue
		}
		out = append(out, offer)
	}
	return out
}

// filterBundlesByHiddenItems drops any bundle that contains a hidden menu item,
// since the bundle as a whole can no longer be fulfilled.
func filterBundlesByHiddenItems(bundles []database.Bundle, hidden map[string]bool) []database.Bundle {
	if len(hidden) == 0 {
		return bundles
	}
	out := make([]database.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		var refs []database.BundleItemRef
		if err := json.Unmarshal([]byte(bundle.Items), &refs); err != nil {
			// Unparseable bundle payload: keep it rather than silently hide a
			// promotion on a data quirk unrelated to stock.
			out = append(out, bundle)
			continue
		}
		blocked := false
		for _, ref := range refs {
			if hidden[strings.TrimSpace(ref.MenuItemID)] {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, bundle)
		}
	}
	return out
}

// buildDeliveryContext renders the AI Waiter system-prompt fragment for the
// current delivery posture. Three states match the public-page gate:
//   - in-house available -> direct order CTA
//   - partner-only       -> "see partners" CTA
//   - off                -> empty (caller should not mention delivery)
func buildDeliveryContext(settings *database.DeliverySettings) string {
	if settings == nil || !settings.DeliveryEnabled {
		return ""
	}
	if settings.InHouseDeliveryEnabled {
		return fmt.Sprintf(
			"Delivery is ENABLED. We offer delivery with a flat fee of %.2f (Free for orders over %.2f). "+
				"Please encourage them to use the 'Order Delivery' button on the page.",
			float64(settings.FlatDeliveryFee)/100.0,
			float64(settings.FreeDeliveryMinimum)/100.0,
		)
	}
	// Partner-only path. Don't promise live quotes; point them at partners.
	if settings.ThirdPartyEnabled {
		return "Delivery is available through our delivery partners. " +
			"Direct guests to the delivery section on this page where they can pick a partner."
	}
	return ""
}
