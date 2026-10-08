package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHubSubscribeLimited_PerBusinessCap asserts that once a business reaches
// its concurrent-connection ceiling, further subscriptions are rejected
// (ok=false, nil channel, nil cancel) until an existing one is released. This
// is the held-connection DoS guard: rate limiters count arrivals, the hub must
// count concurrently-held streams.
func TestHubSubscribeLimited_PerBusinessCap(t *testing.T) {
	hub := newTestHub()

	const businessID uint = 42
	cap := 3
	hub.maxPerBusiness = cap
	hub.maxPerIP = 1000 // not under test here

	var cancels []func()
	for i := 0; i < cap; i++ {
		_, _, cancel, ok := hub.SubscribeLimited(businessID, 0, "10.0.0.1", nil)
		require.True(t, ok, "subscription %d within cap must be accepted", i)
		require.NotNil(t, cancel)
		cancels = append(cancels, cancel)
	}

	// One past the cap is rejected with no resources handed out.
	ch, replayed, cancel, ok := hub.SubscribeLimited(businessID, 0, "10.0.0.1", nil)
	assert.False(t, ok, "subscription past the per-business cap must be rejected")
	assert.Nil(t, ch)
	assert.Nil(t, replayed)
	assert.Nil(t, cancel)

	// Counter reflects exactly the cap, no leak from the rejected attempt.
	assert.Equal(t, cap, hub.businessConnCount(businessID))

	// Releasing one frees a slot so the next subscribe succeeds.
	cancels[0]()
	assert.Equal(t, cap-1, hub.businessConnCount(businessID))

	_, _, cancel2, ok2 := hub.SubscribeLimited(businessID, 0, "10.0.0.1", nil)
	require.True(t, ok2, "after release a new subscription must be accepted")
	require.NotNil(t, cancel2)
	cancels = append(cancels[1:], cancel2)

	// Drain everything and confirm the counter returns to zero (no leak).
	for _, c := range cancels {
		c()
	}
	assert.Equal(t, 0, hub.businessConnCount(businessID))
	assert.Equal(t, 0, hub.ipConnCount("10.0.0.1"))
}

// TestHubSubscribeLimited_PerIPCap asserts the per-IP ceiling is enforced
// independently of the per-business one: a single attacker IP cannot hold more
// than maxPerIP streams even across different businesses, and double-cancel is
// safe (count is not driven negative).
func TestHubSubscribeLimited_PerIPCap(t *testing.T) {
	hub := newTestHub()

	hub.maxPerBusiness = 1000 // not under test here
	hub.maxPerIP = 2

	const ip = "203.0.113.7"

	// Two different businesses, same IP, fills the per-IP budget.
	_, _, cancelA, okA := hub.SubscribeLimited(1, 0, ip, nil)
	require.True(t, okA)
	_, _, cancelB, okB := hub.SubscribeLimited(2, 0, ip, nil)
	require.True(t, okB)
	assert.Equal(t, 2, hub.ipConnCount(ip))

	// Third from the same IP (to a fresh business well under its own cap) is
	// rejected: the per-IP ceiling binds.
	_, _, cancelC, okC := hub.SubscribeLimited(3, 0, ip, nil)
	assert.False(t, okC, "subscription past the per-IP cap must be rejected")
	assert.Nil(t, cancelC)
	assert.Equal(t, 2, hub.ipConnCount(ip))

	// A different IP is unaffected.
	_, _, cancelD, okD := hub.SubscribeLimited(1, 0, "198.51.100.9", nil)
	require.True(t, okD, "a different IP must not be blocked by another IP's budget")

	// Releasing frees the per-IP slot; double-cancel must not underflow.
	cancelA()
	cancelA()
	assert.Equal(t, 1, hub.ipConnCount(ip))

	cancelB()
	cancelD()
	assert.Equal(t, 0, hub.ipConnCount(ip))
	assert.Equal(t, 0, hub.ipConnCount("198.51.100.9"))
}

// TestHubSubscribeLimited_EmptyIPSkipsIPCap asserts that an empty client IP
// (unknown source) is exempt from the per-IP ceiling but still subject to the
// per-business one, so the per-business guard never silently disappears.
func TestHubSubscribeLimited_EmptyIPSkipsIPCap(t *testing.T) {
	hub := newTestHub()
	hub.maxPerBusiness = 5
	hub.maxPerIP = 1

	// Many connections with an empty IP succeed (IP cap not applied) up to the
	// per-business cap.
	var cancels []func()
	for i := 0; i < 5; i++ {
		_, _, cancel, ok := hub.SubscribeLimited(7, 0, "", nil)
		require.True(t, ok, "empty-IP subscription %d must bypass the per-IP cap", i)
		cancels = append(cancels, cancel)
	}
	assert.Equal(t, 0, hub.ipConnCount(""))
	assert.Equal(t, 5, hub.businessConnCount(7))

	// But the per-business cap still binds.
	_, _, _, ok := hub.SubscribeLimited(7, 0, "", nil)
	assert.False(t, ok, "per-business cap must still apply to empty-IP connections")

	for _, c := range cancels {
		c()
	}
	assert.Equal(t, 0, hub.businessConnCount(7))
}

func newTestHub() *Hub {
	return &Hub{
		subscribers:    make(map[uint][]*subscriber),
		replay:         make(map[uint][]BusinessEvent),
		businessConns:  make(map[uint]int),
		ipConns:        make(map[string]int),
		maxPerBusiness: defaultMaxSubscribersPerBusiness,
		maxPerIP:       defaultMaxSubscribersPerIP,

		guestBusinessConns:  make(map[uint]int),
		guestIPConns:        make(map[string]int),
		maxGuestPerBusiness: defaultMaxGuestSubscribersPerBusiness,
		maxGuestPerIP:       defaultMaxGuestSubscribersPerIP,
	}
}

// M-sse: public guest streams are counted in their own pool. Filling the guest
// pool for a business must not consume a single authenticated slot, and
// filling the authenticated pool must not block guests either.
func TestHubSubscribeGuestLimited_SeparatePoolFromAuthenticated(t *testing.T) {
	hub := newTestHub()
	hub.maxPerBusiness = 2
	hub.maxPerIP = 1000
	hub.maxGuestPerBusiness = 2
	hub.maxGuestPerIP = 1000

	var reasons []string
	orig := recordRejectedSubscription
	recordRejectedSubscription = func(reason string) { reasons = append(reasons, reason) }
	t.Cleanup(func() { recordRejectedSubscription = orig })

	const biz uint = 77
	var cancels []func()
	for i := 0; i < 2; i++ {
		_, _, cancel, ok := hub.SubscribeGuestLimited(biz, 0, "203.0.113.1", []string{"bill.split.updated"})
		require.True(t, ok, "guest %d within guest cap", i)
		cancels = append(cancels, cancel)
	}
	_, _, _, ok := hub.SubscribeGuestLimited(biz, 0, "203.0.113.2", []string{"bill.split.updated"})
	assert.False(t, ok, "guest pool is full")
	assert.Equal(t, []string{"guest_business"}, reasons)

	// Authenticated capacity is untouched by the guest flood.
	assert.Equal(t, 0, hub.businessConnCount(biz))
	for i := 0; i < 2; i++ {
		_, _, cancel, ok := hub.SubscribeLimited(biz, 0, "198.51.100.1", nil)
		require.True(t, ok, "authenticated stream %d must still be accepted", i)
		cancels = append(cancels, cancel)
	}
	_, _, _, ok = hub.SubscribeLimited(biz, 0, "198.51.100.1", nil)
	assert.False(t, ok, "authenticated pool has its own ceiling")

	// Releasing a guest frees a guest slot, not an authenticated one.
	cancels[0]()
	cancels[0]() // double cancel never underflows
	_, _, cancelGuest, ok := hub.SubscribeGuestLimited(biz, 0, "203.0.113.3", nil)
	require.True(t, ok)
	_, _, _, ok = hub.SubscribeLimited(biz, 0, "198.51.100.1", nil)
	assert.False(t, ok)

	cancelGuest()
	for _, c := range cancels[1:] {
		c()
	}
	assert.Equal(t, 0, hub.businessConnCount(biz))
	hub.mu.RLock()
	assert.Empty(t, hub.guestBusinessConns)
	assert.Empty(t, hub.guestIPConns)
	hub.mu.RUnlock()
}

func TestHubSubscribeGuestLimited_PerIPCap(t *testing.T) {
	hub := newTestHub()
	hub.SetGuestConnectionLimits(1000, 2)

	ip := "2001:db8:5::/64"
	_, _, c1, ok1 := hub.SubscribeGuestLimited(1, 0, ip, nil)
	_, _, c2, ok2 := hub.SubscribeGuestLimited(2, 0, ip, nil)
	require.True(t, ok1)
	require.True(t, ok2)
	_, _, _, ok3 := hub.SubscribeGuestLimited(3, 0, ip, nil)
	assert.False(t, ok3, "guest per-IP cap spans businesses")
	assert.Equal(t, 0, hub.ipConnCount(ip), "guest streams never count against the authenticated per-IP cap")
	c1()
	c2()
}
