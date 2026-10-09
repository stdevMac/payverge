package events

import (
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kitchenPerms mirrors the RBAC StaffRoleKitchen permission set for the event
// types relevant to the SSE stream. Kitchen legitimately needs orders/bills/
// dispatch/alerts but is denied financial:read (payment amounts) and
// reservations:read (guest PII).
var kitchenPerms = []string{
	"business:read", "menu:read",
	"orders:read", "orders:write", "orders:status", "orders:kitchen",
	"bills:read",
	"staff:read",
	"delivery:dispatch:read",
	"overview:read",
	"alerts:read", "alerts:claim", "alerts:resolve",
}

// hostPerms mirrors StaffRoleHost: reservations + bills (read) + orders (read),
// but NO financial:read and NO delivery:dispatch:read.
var hostPerms = []string{
	"business:read", "menu:read",
	"tables:read", "tables:write",
	"bills:read",
	"reservations:read", "reservations:write", "reservations:create", "reservations:delete",
	"orders:read",
	"staff:read",
	"overview:read",
	"alerts:read", "alerts:claim", "alerts:resolve",
}

func restorePermissionResolver(t *testing.T) {
	t.Helper()
	prev := permissionResolver
	t.Cleanup(func() { permissionResolver = prev })
}

func checkerWithPermissions(perms []string) PermissionChecker {
	return func(required string) bool { return hasPermission(perms, required) }
}

// TestAllowedEventTopics_KitchenExcludesFinancialAndPII is the load-bearing
// security assertion for SSE-RBAC: a Kitchen role must NOT be subscribed to
// payment.received (amounts) or reservation.* (guest PII), while still being
// subscribed to the order/bill/delivery/alert types it legitimately needs.
func TestAllowedEventTopics_KitchenExcludesFinancialAndPII(t *testing.T) {
	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions(kitchenPerms), false
	})

	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess, "kitchen role must not resolve to all-access")

	got := map[string]struct{}{}
	for _, t := range topics {
		got[t] = struct{}{}
	}

	// MUST NOT receive financial amounts.
	assert.NotContains(t, got, "payment.received", "kitchen must NOT receive payment.received (amounts)")
	// MUST NOT receive reservation PII.
	assert.NotContains(t, got, "reservation.new")
	assert.NotContains(t, got, "reservation.updated")
	assert.NotContains(t, got, "reservation.created")

	// MUST still receive the order/bill/delivery/alert types it needs.
	for _, want := range []string{
		"order.created", "order.updated", "order.cancelled",
		"bill.created", "bill.closed", "bill.updated", "bill.stuck", "bill.split.updated",
		"delivery.created", "delivery.updated", "delivery.cancelled",
		"alert.created", "alert.updated",
	} {
		assert.Contains(t, got, want, "kitchen must still receive %s", want)
	}
}

// TestAllowedEventTopics_HostExcludesFinancialAndDelivery confirms a Host role
// (has reservations:read, lacks financial:read and delivery:dispatch:read) is
// scoped to reservations/bills/orders/alerts and denied payment + delivery PII.
func TestAllowedEventTopics_HostExcludesFinancialAndDelivery(t *testing.T) {
	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions(hostPerms), false
	})

	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess)

	got := map[string]struct{}{}
	for _, t := range topics {
		got[t] = struct{}{}
	}

	assert.NotContains(t, got, "payment.received", "host must NOT receive payment.received")
	assert.NotContains(t, got, "delivery.created", "host (no dispatch:read) must NOT receive delivery PII")
	assert.NotContains(t, got, "delivery.updated")

	// Host legitimately sees reservations + bills + orders.
	for _, want := range []string{
		"reservation.new", "reservation.updated",
		"bill.created", "order.created", "alert.created",
	} {
		assert.Contains(t, got, want, "host must still receive %s", want)
	}
}

// TestAllowedEventTopics_OwnerGetsAllAccess confirms a caller resolving to
// all-access keeps the un-scoped whole-business stream (allAccess=true, nil
// topics), preserving current owner/manager-with-financial behavior.
func TestAllowedEventTopics_OwnerGetsAllAccess(t *testing.T) {
	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return nil, true
	})

	topics, allAccess := allowedEventTopics(&gin.Context{})
	assert.True(t, allAccess)
	assert.Nil(t, topics)
}

// TestAllowedEventTopics_FinancialReadSeesPayments confirms a role that holds
// financial:read (e.g. manager) is subscribed to payment.received.
func TestAllowedEventTopics_FinancialReadSeesPayments(t *testing.T) {
	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions([]string{"financial:read", "reservations:read", "bills:read", "orders:read", "delivery:dispatch:read", "alerts:read"}), false
	})

	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess)
	assert.Contains(t, topics, "payment.received")
	assert.Contains(t, topics, "reservation.new")
}

// TestAllowedEventTopics_ConcreteExactDenyOverridesWildcard locks the event-side
// half of the explicit-deny contract. The server resolver evaluates each
// concrete required permission; therefore an exact financial:read deny removes
// every payment/refund topic even when financial:* remains a valid grant.
func TestAllowedEventTopics_ConcreteExactDenyOverridesWildcard(t *testing.T) {
	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		grants := []string{"financial:*", "bills:read"}
		denies := []string{"financial:read"}
		return func(required string) bool {
			return hasPermission(grants, required) && !hasPermission(denies, required)
		}, false
	})

	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess)
	assert.NotContains(t, topics, "payment.received")
	assert.NotContains(t, topics, "payment.pending")
	assert.NotContains(t, topics, "crypto_refund.submitted")
	assert.NotContains(t, topics, "crypto_refund.confirmed")
	assert.Contains(t, topics, "bill.updated", "an unrelated allowed topic must remain subscribed")
}

// TestRequiredPermission_UnknownTypeDeniesByDefault locks in the deny-by-default
// posture: an event type not in the map (a future addition) requires
// financial:read so it can never silently leak to a low-privilege role.
func TestRequiredPermission_UnknownTypeDeniesByDefault(t *testing.T) {
	assert.Equal(t, "financial:read", requiredPermission("some.future.sensitive.event"))
	assert.Equal(t, "alerts:read", requiredPermission("alert.something_new"))
	assert.Equal(t, "financial:read", requiredPermission("payment.received"))
	assert.Equal(t, "reservations:read", requiredPermission("reservation.new"))
}

func TestPrintWakeEventsUseKindSpecificPermissions(t *testing.T) {
	assert.Equal(t, "print:bill", requiredPermission("print.bill_available"))
	assert.Equal(t, "print:receipt", requiredPermission("print.receipt_available"))
	assert.Equal(t, "orders:kitchen", requiredPermission("print.kitchen_available"))

	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions([]string{"print:bill"}), false
	})
	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess)
	assert.Contains(t, topics, "print.bill_available")
	assert.NotContains(t, topics, "print.receipt_available")
	assert.NotContains(t, topics, "print.kitchen_available")
}

// TestAllowedEventTopics_ChatGatedOnChatRead locks the Slice-7 chat event
// scoping: a chat:read holder is subscribed to chat.message + chat.announcement,
// while a role lacking chat:read receives neither. The frames are content-free
// (ids only) by design — see the publish-site assertion in the handlers package —
// because chat:read is universal and the hub fans per-business, so a body would
// leak a DM to every staff member.
func TestAllowedEventTopics_ChatGatedOnChatRead(t *testing.T) {
	assert.Equal(t, "chat:read", requiredPermission("chat.message"))
	assert.Equal(t, "chat:read", requiredPermission("chat.announcement"))

	restorePermissionResolver(t)

	// A chat:read holder gets both chat topics.
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions([]string{"chat:read", "overview:read"}), false
	})
	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess)
	got := map[string]struct{}{}
	for _, et := range topics {
		got[et] = struct{}{}
	}
	assert.Contains(t, got, "chat.message")
	assert.Contains(t, got, "chat.announcement")

	// A role WITHOUT chat:read receives neither chat topic.
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions([]string{"orders:read", "bills:read"}), false
	})
	topics, _ = allowedEventTopics(&gin.Context{})
	got = map[string]struct{}{}
	for _, et := range topics {
		got[et] = struct{}{}
	}
	assert.NotContains(t, got, "chat.message", "no chat:read -> no chat.message")
	assert.NotContains(t, got, "chat.announcement", "no chat:read -> no chat.announcement")
}

// TestAllowedEventTopics_AckGatedOnAnnounce locks the ack-nudge scoping: only a
// chat:announce holder (who can read the confirmation roster) is subscribed to
// chat.announcement_ack; a plain chat:read holder is not.
func TestAllowedEventTopics_AckGatedOnAnnounce(t *testing.T) {
	assert.Equal(t, "chat:announce", requiredPermission("chat.announcement_ack"))

	restorePermissionResolver(t)

	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions([]string{"chat:read", "chat:announce"}), false
	})
	topics, _ := allowedEventTopics(&gin.Context{})
	got := map[string]struct{}{}
	for _, et := range topics {
		got[et] = struct{}{}
	}
	assert.Contains(t, got, "chat.announcement_ack")

	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions([]string{"chat:read"}), false
	})
	topics, _ = allowedEventTopics(&gin.Context{})
	got = map[string]struct{}{}
	for _, et := range topics {
		got[et] = struct{}{}
	}
	assert.NotContains(t, got, "chat.announcement_ack", "no chat:announce -> no ack nudge")
}

// TestSSEStream_KitchenSubscriberNeverReceivesFinancialOrPII is an end-to-end
// hub assertion: a kitchen-scoped topic subscription (the exact topic set the
// SSE handler computes for a kitchen role) must drop payment.received and
// reservation.* on publish, while delivering order/bill events. This proves the
// fix at the channel level, not just the topic-list level.
func TestSSEStream_KitchenSubscriberNeverReceivesFinancialOrPII(t *testing.T) {
	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions(kitchenPerms), false
	})

	topics, allAccess := allowedEventTopics(&gin.Context{})
	require.False(t, allAccess)

	hub := &Hub{subscribers: make(map[uint][]*subscriber)}
	const bizID = 99
	ch, _, cancel := hub.SubscribeWithReplayTopic(bizID, 0, topics...)
	defer cancel()

	// Publish a representative mix: two sensitive, two allowed.
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "payment.received", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "reservation.new", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.created", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.updated", Timestamp: time.Now()})

	// Only the two allowed types should have entered the channel.
	require.Len(t, ch, 2, "kitchen subscriber must only receive its allowed types")
	received := []string{(<-ch).Type, (<-ch).Type}
	sort.Strings(received)
	assert.Equal(t, []string{"bill.updated", "order.created"}, received)
}

// TestCoverageEventPermissions locks the Slice-5 coverage event → permission
// mapping: claim/accept events feed the manager approval queue (schedule:approve)
// while request/decision events fan out to the staff who own the request
// (schedule:self). A low-priv role must never receive the manager-scoped events.
func TestCoverageEventPermissions(t *testing.T) {
	require.Equal(t, "schedule:approve", requiredPermission("openshift.claimed"))
	require.Equal(t, "schedule:self", requiredPermission("openshift.decided"))
	require.Equal(t, "schedule:self", requiredPermission("shift.swap.requested"))
	require.Equal(t, "schedule:approve", requiredPermission("shift.swap.accepted"))
	require.Equal(t, "schedule:self", requiredPermission("shift.swap.decided"))
}
