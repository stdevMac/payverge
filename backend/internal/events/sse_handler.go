package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// sseNoAllowedTopicsSentinel is a topic string no publisher ever emits. It is
// subscribed on behalf of a permission-scoped caller that resolves to zero
// allowed event types, so the subscription matches nothing (empty stream) rather
// than falling through the hub's "zero topics == match all" firehose behavior.
const sseNoAllowedTopicsSentinel = "__no_allowed_topics__"

// EventStaffAccessRevoked is a process-local control frame published after a
// staff access change so same-instance SSE streams can eject immediately.
// Not a domain event for the frontend — the handler turns it into a terminal
// session_revoked error for the matching staff_id only.
const EventStaffAccessRevoked = "auth.session_revoked"

// sseHeartbeatInterval is the keepalive / authz re-check period. Overridable in
// tests so heartbeat-driven revocation can be asserted without waiting 15s.
var sseHeartbeatInterval = 15 * time.Second

// sseStaffAuthzFailureLimit is how many consecutive transient authz-check
// errors a staff stream tolerates before it closes so the client reconnects.
// A confirmed revocation still ejects on the first check.
const sseStaffAuthzFailureLimit = 3

// SSEHandler streams Server-Sent Events for a business.
// GET /businesses/:id/events
func SSEHandler(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Admin lifecycle guard: a suspended or closed business gets no stream.
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check business"})
		return
	}
	if code, message, locked := database.BusinessLockDenial(business); locked {
		writeSSETerminalError(c, code, message)
		return
	}

	// Ownership check is duplicated here to avoid an import cycle with server.
	if !checkBusinessAccess(c, business) {
		writeSSETerminalError(c, "access_denied", "Access denied")
		return
	}

	// Staff streams re-check authz_version on connect (and every heartbeat).
	// Guests/owners have no staff_id and skip this gate.
	streamingStaffID, isStaffStream := extractContextUint(mustGet(c, "staff_id"))
	tokenAuthzVersion := 0
	if isStaffStream {
		if v, ok := extractContextInt(mustGet(c, "staff_authz_version")); ok {
			tokenAuthzVersion = v
		} else if v, ok := extractContextInt(mustGet(c, "authz_version")); ok {
			tokenAuthzVersion = v
		}
		ok, err := staffSSEAuthzStillValid(streamingStaffID, tokenAuthzVersion)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "Authorization check unavailable",
				"code":  "authz_check_unavailable",
			})
			return
		}
		if !ok {
			writeSSETerminalError(c, "session_revoked", "Session has been revoked")
			return
		}
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Resume point: prefer the standard Last-Event-ID header (sent by native
	// EventSource auto-reconnect), falling back to a last_event_id query param
	// for clients that close + recreate the EventSource on manual reconnect
	// (they cannot set the header). This bounds replay to genuinely-missed
	// events instead of replaying the whole ring buffer.
	resumeRaw := c.GetHeader("Last-Event-ID")
	if resumeRaw == "" {
		resumeRaw = c.Query("last_event_id")
	}
	resumeEpoch, lastEventID := parseResumeToken(resumeRaw)

	// Restart-epoch gap detection. A resume token references this process's
	// in-memory replay ring by epoch; if it carries a DIFFERENT epoch (the ring
	// was rebuilt by a restart) or the ring can no longer serve the requested seq
	// (256-entry overflow), replay cannot recover the missed events. Signal the
	// client to resync (sync.reset) and start it fresh at the live head instead of
	// silently delivering a partial/empty replay. An empty resume epoch is a
	// legacy/first-connect client and is left on the normal path.
	needsReset := false
	if lastEventID > 0 {
		if resumeEpoch != "" && resumeEpoch != GetHub().Epoch() {
			needsReset = true
		} else if GetHub().HasGapAfter(uint(businessID), lastEventID) {
			needsReset = true
		}
	}
	if needsReset {
		lastEventID = 0
	}

	// Per-permission topic scoping (SSE-RBAC): resolve the subset of business
	// event types this caller's RBAC permissions actually allow it to receive,
	// then subscribe ONLY to those topics. An owner / financial:read role
	// resolves to allAccess and keeps the un-scoped whole-business stream
	// (byte-for-byte current behavior). A low-privilege role (Kitchen/Host/
	// Server) never even has disallowed types — payment.received (amounts),
	// reservation.* (guest PII) — queued into its channel. See sse_permissions.go.
	topics, allAccess := allowedEventTopics(c)

	// Subscribe to the hub. X-9: a fresh connection (no Last-Event-ID header
	// or last_event_id param) starts at the live head — replaying the ring
	// buffer would deliver up to 256 stale events to every new tab. Clients
	// reconnecting WITH a resume point still get bounded replay of
	// genuinely-missed events. Kitchen + dashboard refetch on connect, so a
	// fresh stream losing history is by design.
	hub := GetHub()
	var (
		ch       <-chan BusinessEvent
		replayed []BusinessEvent
		cancel   func()
		ok       bool
	)
	// Always use SubscribeLimited so per-business / per-IP caps apply to owners
	// and staff alike (unlimited Subscribe was a DoS vector for allAccess paths).
	clientIP := middleware.ClientRateLimitKey(c)
	switch {
	case allAccess:
		ch, replayed, cancel, ok = hub.SubscribeLimited(uint(businessID), lastEventID, clientIP, nil)
	default:
		// Permission-scoped: only the allowed topics enter the channel/replay.
		// Fail CLOSED on an empty allowed set: a role whose permissions map to zero
		// event types must get an EMPTY stream, not the whole-business firehose.
		// SubscribeLimited with empty topics matches every type, so substitute a
		// sentinel topic no publisher ever emits.
		if len(topics) == 0 {
			topics = []string{sseNoAllowedTopicsSentinel}
		}
		ch, replayed, cancel, ok = hub.SubscribeLimited(uint(businessID), lastEventID, clientIP, topics)
	}
	if !ok {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many concurrent event streams",
			"code":  "sse_connection_limit",
		})
		return
	}
	// X-9 fresh-connection rule: lastID 0 would otherwise replay the whole ring.
	if lastEventID == 0 {
		replayed = nil
	}
	defer cancel()

	// Resume failed across a restart/eviction: tell the client to discard its
	// stale Last-Event-ID and full-heal (refetch domain state) before it starts
	// consuming live frames. Written ahead of `connected` so the client resyncs
	// first. Old clients without a sync.reset listener simply drop this frame.
	if needsReset {
		writeSSE(c, "sync.reset", map[string]interface{}{
			"epoch":  GetHub().Epoch(),
			"reason": "resume_unavailable",
		})
	}

	for _, event := range replayed {
		writeBusinessSSE(c, event)
	}

	// Send initial connected event
	writeSSE(c, "connected", map[string]interface{}{
		"business_id": businessID,
		"epoch":       GetHub().Epoch(),
		"message":     "SSE connection established",
	})
	c.Writer.Flush()

	// Keepalive ticker. Kept well under any reverse-proxy idle/read timeout
	// (Caddy's API transport read_timeout is 60s) so the stream is never torn
	// down for "inactivity" between real events. Also the authz_version re-check
	// cadence for staff streams (cross-replica revocation bound).
	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	ctx := c.Request.Context()
	staffAuthzFailures := 0

	for {
		select {
		case <-ctx.Done():
			log.Printf("SSE client disconnected for business %d", businessID)
			return

		case event, ok := <-ch:
			if !ok {
				return
			}
			// Process-local access-revocation control frame: eject matching staff
			// streams immediately; never forward as a domain event.
			if event.Type == EventStaffAccessRevoked {
				if isStaffStream && staffAccessRevokedMatches(event, streamingStaffID) {
					writeSSETerminalError(c, "session_revoked", "Session has been revoked")
					return
				}
				continue
			}
			writeBusinessSSE(c, event)
			c.Writer.Flush()

		case <-ticker.C:
			closeAfterPing := false
			if isStaffStream {
				stillValid, authzErr := staffSSEAuthzStillValid(streamingStaffID, tokenAuthzVersion)
				if authzErr != nil {
					staffAuthzFailures++
					log.Printf("SSE staff authz check unavailable for staff %d business %d (consecutive %d): %v", streamingStaffID, businessID, staffAuthzFailures, authzErr)
					// Keep the keepalive so one blip does not look like a dead
					// socket. After the limit, close with no revoked frame so
					// the client reconnects instead of treating it as logout.
					closeAfterPing = staffAuthzFailures >= sseStaffAuthzFailureLimit
				} else {
					staffAuthzFailures = 0
					if !stillValid {
						writeSSETerminalError(c, "session_revoked", "Session has been revoked")
						return
					}
				}
			}
			writeSSE(c, "ping", map[string]string{"time": time.Now().UTC().Format(time.RFC3339)})
			c.Writer.Flush()
			if closeAfterPing {
				return
			}
		}
	}
}

// staffSSEAuthzStillValid is the lean per-heartbeat / on-connect check: one
// Select of (authz_version, is_active) by primary key. A missing staff row is
// a confirmed revocation (false, nil). Any other error is transient and must
// not be treated as revocation. Access-shape tests pin the single-query,
// non-SELECT-* contract.
func staffSSEAuthzStillValid(staffID uint, tokenAuthzVersion int) (bool, error) {
	if staffID == 0 {
		return false, nil
	}
	var row struct {
		AuthzVersion int  `gorm:"column:authz_version"`
		IsActive     bool `gorm:"column:is_active"`
	}
	err := database.GetDB().
		Model(&database.Staff{}).
		Select("authz_version", "is_active").
		Where("id = ?", staffID).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !row.IsActive {
		return false, nil
	}
	live := row.AuthzVersion
	if live <= 0 {
		live = 1
	}
	return live == tokenAuthzVersion, nil
}

// PublishStaffAccessRevoked notifies same-instance SSE subscribers that a staff
// member's access was revoked. Multi-replica sub-second eviction via
// LISTEN/NOTIFY is DEFERRED — see RevokeStaffAccess docs; the 15s heartbeat
// authz_version re-check already guarantees cross-replica termination.
func PublishStaffAccessRevoked(businessID, staffID uint) {
	raw, err := json.Marshal(map[string]uint{"staff_id": staffID})
	if err != nil {
		return
	}
	GetHub().Publish(BusinessEvent{
		BusinessID: businessID,
		// Literal (== EventStaffAccessRevoked) on purpose: the SSE
		// permission-completeness scanner only records publish sites whose event
		// type is a quoted literal at the call. The constant is used everywhere
		// else (matching / eventTypePermissions), so this must stay in sync.
		Type:      "auth.session_revoked",
		Data:      raw,
		Timestamp: time.Now(),
	})
}

func staffAccessRevokedMatches(event BusinessEvent, staffID uint) bool {
	var payload struct {
		StaffID uint `json:"staff_id"`
	}
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return false
	}
	return payload.StaffID == staffID
}

// mustGet returns the context value or nil (never panics).
func mustGet(c *gin.Context, key string) any {
	v, _ := c.Get(key)
	return v
}

func extractContextInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case uint:
		return int(v), true
	case uint64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	default:
		return 0, false
	}
}

func writeSSE(c *gin.Context, eventType string, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	writeSSEFrame(c, 0, eventType, raw)
}

// writeSSETerminalError sets SSE headers (turning a would-be HTTP error into a
// 200 stream that EventSource can read), emits a single terminal `error` event
// carrying a machine-readable code + human message, flushes, and returns. The
// frontend listens for this frame to STOP reconnecting on a permanent gate
// denial — a bare HTTP 403 is invisible to EventSource and only triggers
// exponential-backoff reconnect hammering.
func writeSSETerminalError(c *gin.Context, code, message string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	writeSSE(c, "error", map[string]string{"code": code, "message": message})
	c.Writer.Flush()
}

func writeBusinessSSE(c *gin.Context, event BusinessEvent) {
	data := event.Data
	if event.Type == "bill.split.updated" {
		data = slimBillSplitUpdated(data)
	}

	// Event ids are epoch-prefixed ("<epoch>:<seq>") so a resume across a restart
	// is detectable (the epoch changes). EventSource treats the id as an opaque
	// string.
	idStr := businessEventID(event.ID)
	writeSSEFrameStr(c, idStr, event.Type, data)

	envelope := struct {
		ID         uint64          `json:"id"`
		BusinessID uint            `json:"business_id"`
		Type       string          `json:"type"`
		Data       json.RawMessage `json:"data"`
		Timestamp  string          `json:"timestamp,omitempty"`
	}{
		ID:         event.ID,
		BusinessID: event.BusinessID,
		Type:       event.Type,
		Data:       data,
	}
	if !event.Timestamp.IsZero() {
		envelope.Timestamp = event.Timestamp.UTC().Format(time.RFC3339Nano)
	}

	raw, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	writeSSEFrameStr(c, idStr, "", raw)
}

// businessEventID formats a hub sequence number as the epoch-prefixed SSE id.
func businessEventID(seq uint64) string {
	if seq == 0 {
		return ""
	}
	return GetHub().Epoch() + ":" + strconv.FormatUint(seq, 10)
}

// parseResumeToken splits a Last-Event-ID into its epoch prefix and sequence
// number. Accepts both the epoch-prefixed form ("<epoch>:<seq>") and the legacy
// bare-sequence form ("<seq>", from an old client or the pre-epoch backend), in
// which case the epoch is empty.
func parseResumeToken(raw string) (string, uint64) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0
	}
	if idx := strings.LastIndex(raw, ":"); idx >= 0 {
		epoch := raw[:idx]
		seq, err := strconv.ParseUint(raw[idx+1:], 10, 64)
		if err != nil {
			return "", 0
		}
		return epoch, seq
	}
	seq, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return "", 0
	}
	return "", seq
}

// slimBillSplitUpdated reduces a bill.split.updated payload to non-financial
// signal for the operator (bills:read) business stream: identifiers, split
// status, and participant/share COUNTS — never per-share amounts, tenders, or
// payer names (that detail is financial:read). The operator dashboard refetches
// full split state via REST when it needs the amounts. The guest split stream
// uses its own writer and is unaffected. On malformed input the original payload
// is returned unchanged (fail-open to the pre-existing behavior).
func slimBillSplitUpdated(raw json.RawMessage) json.RawMessage {
	var full struct {
		BillNumber string          `json:"bill_number"`
		Status     string          `json:"status"`
		UpdatedAt  json.RawMessage `json:"updated_at"`
		Shares     []struct {
			Status string `json:"status"`
		} `json:"shares"`
	}
	if err := json.Unmarshal(raw, &full); err != nil {
		return raw
	}

	settled, held := 0, 0
	for _, s := range full.Shares {
		switch s.Status {
		case "settled":
			settled++
		case "held":
			held++
		}
	}

	slim := map[string]interface{}{
		"bill_number":   full.BillNumber,
		"status":        full.Status,
		"share_count":   len(full.Shares),
		"settled_count": settled,
		"held_count":    held,
	}
	if len(full.UpdatedAt) > 0 && string(full.UpdatedAt) != "null" {
		slim["updated_at"] = full.UpdatedAt
	}

	out, err := json.Marshal(slim)
	if err != nil {
		return raw
	}
	return out
}

func writeSSEFrame(c *gin.Context, id uint64, eventType string, raw json.RawMessage) {
	idStr := ""
	if id > 0 {
		idStr = strconv.FormatUint(id, 10)
	}
	writeSSEFrameStr(c, idStr, eventType, raw)
}

func writeSSEFrameStr(c *gin.Context, id string, eventType string, raw json.RawMessage) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	if id != "" {
		_, _ = fmt.Fprintf(c.Writer, "id: %s\n", id)
	}
	if eventType != "" {
		_, _ = fmt.Fprintf(c.Writer, "event: %s\n", eventType)
	}
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(raw))
}

func extractContextUint(value any) (uint, bool) {
	switch v := value.(type) {
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
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return uint(parsed), true
	default:
		return 0, false
	}
}

// listedDemoForSSE mirrors server.listedDemoForAdmin. Duplicated here because
// events cannot import server (cycle). Keep both in lockstep.
func listedDemoForSSE(business *database.Business) bool {
	if business == nil {
		return false
	}
	return business.IsDemo || business.Kind == database.BusinessKindDemo
}

func sseHasPlatformAdminRole(c *gin.Context) bool {
	for _, key := range []string{"role", "user_role"} {
		if value, ok := c.Get(key); ok {
			if role, ok := value.(string); ok && role == "admin" {
				return true
			}
		}
	}
	return false
}

func sseContextUserID(c *gin.Context) (uint, bool) {
	uid, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	parsed, ok := extractContextUint(uid)
	if !ok || parsed == 0 {
		return 0, false
	}
	return parsed, true
}

// checkBusinessAccess matches server.CheckBusinessAccess: platform admins on
// listed demos, wallet/user/demo owners, and staff of this business. Live
// non-demo tenants stay owner/staff-only.
func checkBusinessAccess(c *gin.Context, business *database.Business) bool {
	if business == nil {
		return false
	}

	if sseHasPlatformAdminRole(c) && listedDemoForSSE(business) {
		return true
	}

	if addr, ok := c.Get("address"); ok {
		if a, ok := addr.(string); ok && business.OwnerAddress != "" &&
			strings.EqualFold(strings.TrimSpace(business.OwnerAddress), strings.TrimSpace(a)) {
			return true
		}
	}

	if uid, ok := sseContextUserID(c); ok {
		if business.UserID != nil && *business.UserID == uid {
			return true
		}
		if business.DemoOwnerUserID != nil && *business.DemoOwnerUserID == uid {
			return true
		}
	}

	if staffBizID, ok := c.Get("staff_business_id"); ok {
		if bizID, ok := extractContextUint(staffBizID); ok && bizID == business.ID {
			return true
		}
	}

	return false
}
