package events

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// SSE per-permission topic scoping (SSE-RBAC).
//
// The whole-business SSE stream (GET /businesses/:id/events) used to deliver
// EVERY business event type to any caller that merely passed the membership
// (checkBusinessAccess) check. That bypassed the per-permission RBAC that the
// equivalent polling routes enforce: a Kitchen/Host/Server role denied
// financial:read / reservations:read on its REST endpoints still received the
// realtime payment.received (amounts), reservation.* (guest PII + notes), etc.
//
// The fix scopes each subscriber's hub topic set to the event types its
// effective RBAC permissions actually allow it to see, using the hub's existing
// permission-agnostic SubscribeWithReplayTopic. An owner / financial:read role
// resolves to "all permissions" and therefore still gets the full firehose it
// gets today.
//
// The permission resolution itself lives in the server package (canonical RBAC),
// but events cannot import server (import cycle, see sse_handler.go). main.go,
// which imports both, injects the resolver via SetPermissionResolver at startup.

// eventTypePermissions maps each business event type published to the hub to the
// single RBAC permission a subscriber must hold to receive it. Types are matched
// exactly; prefix families (e.g. "alert.") are handled in requiredPermission.
//
// Keep this in sync with the publish sites (grep `GetHub().Publish`) — the
// sync is now enforced both ways by sse_permissions_completeness_test.go, so
// drift fails `go test ./internal/events/`. The original families:
//   - payment.received          carries settled amounts        -> financial:read
//   - reservation.*             carries guest PII + notes       -> reservations:read
//   - delivery.*                carries customer name/phone/addr -> delivery:dispatch:read
//   - bill.*                    bill lifecycle                  -> bills:read
//   - order.*                   order lifecycle (kitchen)       -> orders:read
//   - alert.*                   operational alerts              -> alerts:read
var eventTypePermissions = map[string]string{
	"payment.received": "financial:read",
	// A guest payment REQUEST awaiting operator confirmation. Carries the
	// requested amount, so it is scoped like payment.received — but it is a
	// distinct type on purpose: the money has NOT been recorded yet, and
	// frontends that only understand payment.received safely ignore it.
	"payment.pending": "financial:read",
	// An operator cancelled or rejected a pending request. Carries only bill,
	// request, and terminal status identifiers, but remains financial workflow
	// state and therefore follows the same visibility boundary.
	"payment.request.resolved": "financial:read",

	// On-chain refund lifecycle (Wave 4 crypto refunds). These are money-MOVEMENT
	// frames — a refund reduces settled revenue — so their VISIBILITY is scoped
	// like payment.received/payment.pending on financial:read, NOT on the crypto-
	// refund ACTION permissions (refunds:crypto:request / refunds:crypto:approve),
	// which gate who may initiate/approve a refund rather than who may watch the
	// money move. Payloads are ids-only + status + tx_hash (a refetch nudge); the
	// durable refund record stays behind the authorized refund-list read. Without
	// these entries the completeness guard (TestEveryPublishedEventTypeIsPermission-
	// Mapped) fails and scoped financial subscribers would never receive the frame.
	"crypto_refund.submitted": "financial:read",
	"crypto_refund.confirmed": "financial:read",

	"reservation.new":     "reservations:read",
	"reservation.updated": "reservations:read",

	"delivery.created":   "delivery:dispatch:read",
	"delivery.updated":   "delivery:dispatch:read",
	"delivery.cancelled": "delivery:dispatch:read",

	"bill.created":       "bills:read",
	"bill.closed":        "bills:read",
	"bill.updated":       "bills:read",
	"bill.stuck":         "bills:read",
	"bill.abandoned":     "bills:read", // Task 16 lifecycle sweeper — same visibility as other bill status frames
	"bill.split.updated": "bills:read",

	"order.created":   "orders:read",
	"order.updated":   "orders:read",
	"order.cancelled": "orders:read",

	"print.bill_available":    "print:bill",
	"print.receipt_available": "print:receipt",
	"print.kitchen_available": "orders:kitchen",

	// Staff scheduling (Slice 2). All staff hold schedule:read (row-scoped), so
	// these are safe to fan out business-wide; the payload carries only ids +
	// times, never money or another person's pay.
	"schedule.published": "schedule:read",
	"shift.assigned":     "schedule:read",
	"shift.updated":      "schedule:read",

	// Availability & time-off (Slice 3). A new request is only of interest to
	// approvers (managers/owners), so timeoff.requested is gated on
	// schedule:approve. A decision notifies the requester, and all staff hold
	// schedule:self, so timeoff.decided fans out business-wide — its payload
	// carries only ids + the decision status, never money or PII (the requester
	// filters client-side; per the §6 per-business fan-out caveat).
	"timeoff.requested": "schedule:approve",
	"timeoff.decided":   "schedule:self",

	// Time clock (Slice 4). A clock-out or manager correction (reject/edit/
	// approve/manual-entry) changes the manager review queue, so timeclock.entry
	// is gated on timeclock:manage — the same permission that reads the queue. The
	// payload carries only ids + status (a refetch nudge), never money or another
	// person's hours; the durable rows stay behind the authorized timesheet reads.
	"timeclock.entry": "timeclock:manage",

	// Staff coverage (Slice 5). Open-shift claims and accepted swaps feed the
	// manager approval queue, so they are gated on schedule:approve. The swap
	// request (eligible coworkers may pick it up) and the decision outcomes
	// (which notify the requester/claimant — all staff hold schedule:self) fan
	// out business-wide; payloads carry only ids + status, never money/PII (the
	// recipient filters client-side, per the §6 per-business fan-out caveat).
	"openshift.claimed":    "schedule:approve",
	"openshift.decided":    "schedule:self",
	"shift.swap.requested": "schedule:self",
	"shift.swap.accepted":  "schedule:approve",
	"shift.swap.decided":   "schedule:self",

	// Staff chat (Slice 7). EVERY staff role holds chat:read, so chat.message /
	// chat.announcement fan out business-wide — which is precisely why their
	// frames carry IDS ONLY (channel_id+message_id / announcement_id) and NEVER
	// the message text or a DM body. The hub fans per-business and chat:read is
	// universal, so any body in the frame would leak a DM to every staff member;
	// the body stays behind the authorized ListMessages read (CanReadChannel) and
	// the SSE frame is only a refetch nudge. See the publish sites in
	// internal/handlers/chat.go.
	"chat.message":      "chat:read",
	"chat.announcement": "chat:read",
	// The ack nudge only matters to whoever can read the confirmation roster
	// (GetAnnouncementAcks is chat:announce-gated) — same gate here so servers
	// aren't churned by frames they can do nothing with. Ids only, as always.
	"chat.announcement_ack": "chat:announce",

	// Staff inbox nudge (NotifyStaff chokepoint). Payload is ids-only
	// ({staff_ids:[...]}); recipients filter client-side. Gated on chat:read
	// because every staff role holds it (see the chat block above) — the frame
	// must reach ALL staff, and the durable content stays behind the
	// per-staff-authenticated inbox endpoints.
	"notification.new": "chat:read",

	// Process-local access-revocation control frame. Ids-only payload
	// ({staff_id}); the SSE handler turns a match into a terminal
	// session_revoked error and does NOT forward it as a domain event.
	// Gated on business:read (every staff role holds it) so topic-scoped
	// streams still receive the eject signal. Hub.Publish also force-delivers
	// this type regardless of topic set (belt-and-suspenders).
	"auth.session_revoked": "business:read",

	// Spaces phone-scan progress (Spaces & Tables). Operators with tables:read
	// receive progress for sessions they started; payload is ids + status only
	// (no raw tokens or upload bytes).
	"space.scan.updated": "tables:read",
}

// allBusinessEventTypes is the canonical list of event types the whole-business
// SSE stream can deliver. A subscriber is subscribed only to the subset of these
// that its permissions allow. Keeping the list explicit (rather than passing no
// topics and filtering on send) means the hub never even queues a disallowed
// frame into a low-privilege subscriber's bounded channel.
var allBusinessEventTypes = func() []string {
	types := make([]string, 0, len(eventTypePermissions)+1)
	for t := range eventTypePermissions {
		types = append(types, t)
	}
	// alert.* is a prefix family; enumerate the concrete names published.
	types = append(types,
		"alert.created", "alert.claimed", "alert.resolved",
		"alert.dismissed", "alert.reopened", "alert.updated",
	)
	return types
}()

// requiredPermission returns the RBAC permission a subscriber must hold to
// receive the given event type. Unknown/unmapped types deny-by-default behind
// financial:read so a newly-added event type can never silently leak to a
// low-privilege role before this map is updated.
func requiredPermission(eventType string) string {
	if perm, ok := eventTypePermissions[eventType]; ok {
		return perm
	}
	if strings.HasPrefix(eventType, "alert.") {
		return "alerts:read"
	}
	return "financial:read"
}

// PermissionChecker evaluates one concrete permission using the canonical RBAC
// semantics supplied by the server package. Keeping the check concrete is
// security-critical: a raw wildcard grant such as financial:* cannot be safely
// flattened into an "effective permission list" when financial:read is also
// explicitly denied.
type PermissionChecker = func(required string) bool

// PermissionResolver resolves a request-scoped concrete permission checker,
// plus whether the caller holds ALL permissions for the business (owner /
// platform admin). The resolver loads grants and denies once; the returned
// checker can then evaluate every SSE topic without further database queries.
// When allAccess is true the SSE handler subscribes to the whole-business stream
// unchanged (no topic scoping), preserving current owner behavior byte-for-byte.
//
// It is injected by main.go (SetPermissionResolver) because the canonical RBAC
// logic lives in the server package, which events cannot import.
type PermissionResolver func(c *gin.Context) (checker PermissionChecker, allAccess bool)

var permissionResolver PermissionResolver

// SetPermissionResolver wires the server-side RBAC resolver into the events
// package. Called once at startup from main.go.
func SetPermissionResolver(r PermissionResolver) {
	permissionResolver = r
}

// allowedEventTopics returns the subset of business event types the caller is
// permitted to receive, plus allAccess=true when the caller holds all
// permissions (owner/admin) and should get the un-scoped whole-business stream.
//
// If no resolver is wired (e.g. a unit test that exercises only the stream
// plumbing), it fails OPEN to all-access to preserve the legacy behavior — the
// route-level RoleBasedAccessMiddleware is the hard gate; this is defense in
// depth on top of it.
func allowedEventTopics(c *gin.Context) (topics []string, allAccess bool) {
	if permissionResolver == nil {
		return nil, true
	}
	checker, all := permissionResolver(c)
	if all {
		return nil, true
	}
	if checker == nil {
		return nil, false
	}
	allowed := make([]string, 0, len(allBusinessEventTypes))
	for _, et := range allBusinessEventTypes {
		if checker(requiredPermission(et)) {
			allowed = append(allowed, et)
		}
	}
	return allowed, false
}

// hasPermission mirrors the server RBAC match semantics (exact, "cat:*" wildcard,
// and "*:*" super-wildcard) so resolved permission lists are interpreted
// identically on both sides of the injection boundary.
func hasPermission(perms []string, required string) bool {
	for _, p := range perms {
		if p == required {
			return true
		}
		if p == "*:*" {
			return true
		}
		if strings.HasSuffix(p, ":*") {
			category := strings.TrimSuffix(p, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}
	}
	return false
}
