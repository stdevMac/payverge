package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/metrics"
)

const (
	subscriberBufferSize = 64
	replayBufferSize     = 256

	// defaultReplayIdleTTL is how long a business keeps its replay ring after
	// the last subscriber leaves, or after a publish nobody is listening to.
	// The ring is a reconnect buffer; retaining one per business forever leaked
	// every venue that had ever published. A zero Hub.replayIdleTTL uses this.
	defaultReplayIdleTTL = 10 * time.Minute

	// defaultMaxSubscribersPerBusiness caps concurrently-HELD SSE streams for a
	// single business. Rate limiters and the reverse proxy count arrivals, not
	// held connections, so without this ceiling an attacker who can reach a
	// stream (e.g. the public split SSE, reusing one valid bill_number) can open
	// streams up to the rate-limit budget and hold them open, accumulating
	// goroutines + 64-slot channels without bound and making Publish O(N) worse.
	// A legitimate business serves a bounded set of operator dashboards + active
	// guest split bills; this is generous for that and still bounds the abuse.
	defaultMaxSubscribersPerBusiness = 200

	// defaultMaxSubscribersPerIP caps concurrently-held SSE streams from a single
	// client IP across ALL businesses, so one source cannot accumulate held
	// connections by fanning out across businesses/bills. A normal guest holds
	// one split stream per open tab; operators a handful of dashboards.
	defaultMaxSubscribersPerIP = 30

	// defaultMaxGuestSubscribersPerBusiness / defaultMaxGuestSubscribersPerIP
	// cap the public (unauthenticated) guest streams, which are counted in a
	// pool separate from authenticated operator streams (M-sse). Guests can
	// exhaust only their own pool, so an anonymous flood against one venue can
	// never take the kitchen display or dashboard streams offline.
	defaultMaxGuestSubscribersPerBusiness = 150
	defaultMaxGuestSubscribersPerIP       = 10
)

// BusinessEvent represents an event pushed to subscribers.
type BusinessEvent struct {
	ID         uint64          `json:"id"`
	BusinessID uint            `json:"business_id"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data"`
	Timestamp  time.Time       `json:"timestamp"`
}

// subscriber is an internal channel+metadata for a single SSE client.
type subscriber struct {
	ch         chan BusinessEvent
	businessID uint
	// topics is an allowed-event-type set. A nil/empty set means "accept ALL
	// types" (the legacy whole-business behavior). When non-empty, Publish skips
	// the send for any event whose Type is not a member, so unrelated event
	// types never enter this subscriber's bounded channel and therefore cannot
	// starve/evict the types it actually cares about.
	//
	// IMMUTABILITY INVARIANT: topics is assigned exactly once at subscribe time
	// (in newSubscriber) and is NEVER mutated afterward. This is what makes it
	// safe for Publish to read it lock-free, outside h.mu, while it fans events
	// out. Do not mutate this map after construction.
	topics map[string]struct{}

	// filter, when set, is a per-subscriber predicate Publish applies before
	// enqueueing, so events this stream would discard never take buffer slots.
	// Same immutability invariant as topics; it is called outside h.mu.
	filter func(BusinessEvent) bool

	// dropped, when non-nil (cap 1), receives a non-blocking signal whenever
	// Publish drops an event for this subscriber because its buffer is full,
	// so the stream can resynchronize instead of silently missing a frame.
	dropped chan struct{}
}

// matches reports whether an event of the given type should be delivered to this
// subscriber. An empty/nil topic set matches everything (legacy behavior).
// Reads the immutable topics map; safe to call lock-free (see invariant above).
func (s *subscriber) matches(eventType string) bool {
	if len(s.topics) == 0 {
		return true
	}
	_, ok := s.topics[eventType]
	return ok
}

// newSubscriber builds a subscriber with its (immutable) topic set. An empty
// types list yields a nil topic set, preserving the all-types legacy behavior.
func newSubscriber(businessID uint, types []string) *subscriber {
	var topics map[string]struct{}
	if len(types) > 0 {
		topics = make(map[string]struct{}, len(types))
		for _, t := range types {
			topics[t] = struct{}{}
		}
	}
	return &subscriber{
		ch:         make(chan BusinessEvent, subscriberBufferSize),
		businessID: businessID,
		topics:     topics,
	}
}

// Hub is a thread-safe in-process pub/sub hub for business events.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[uint][]*subscriber
	replay      map[uint][]BusinessEvent
	// nextID is per-business so a public tenant-scoped SSE stream cannot
	// leak the platform-wide event counter (#536).
	// Entries are never deleted. Clients resume with Last-Event-ID, so the
	// sequence must stay monotonic for the life of the process; one uint64
	// per business is negligible next to the replay ring eviction drops.
	nextID map[uint]uint64
	// replayIdleTTL bounds how long an idle business keeps its replay ring.
	// Zero means defaultReplayIdleTTL.
	replayIdleTTL time.Duration
	// replayEvict drops h.replay[id] after the idle TTL. Nil until the first
	// schedule; GetHub initializes it.
	replayEvict map[uint]*time.Timer

	// Concurrent-connection ceilings. businessConns/ipConns track currently-held
	// (not arrived) subscriptions; they are incremented under h.mu at subscribe
	// time and decremented by the returned cancel. maxPerBusiness/maxPerIP are
	// the caps a SubscribeLimited call is rejected against. A zero cap means
	// "uncapped" (SubscribeWithReplayTopic never touches these counters).
	businessConns  map[uint]int
	ipConns        map[string]int
	maxPerBusiness int
	maxPerIP       int

	// Guest pool (SubscribeGuestLimited): the same ceilings, counted separately
	// so unauthenticated streams never consume authenticated capacity.
	guestBusinessConns  map[uint]int
	guestIPConns        map[string]int
	maxGuestPerBusiness int
	maxGuestPerIP       int

	// epoch identifies this process's replay ring. The in-memory ring is lost on
	// restart, so an event id embeds the epoch ("<epoch>:<seq>"). A client
	// resuming with a Last-Event-ID from a previous epoch cannot be served from
	// this ring — the SSE handler detects the mismatch and tells it to resync
	// rather than silently missing everything published before the restart.
	epoch string
}

var (
	globalHub *Hub
	hubOnce   sync.Once
)

var recordDroppedBusinessEvent = func(event BusinessEvent) {
	metrics.SSEDroppedEvents.WithLabelValues(event.Type).Inc()
}

// recordRejectedSubscription counts SSE subscriptions refused by a
// concurrent-connection ceiling. reason is "business" or "ip", prefixed with
// "guest_" for the guest pool. Overridable in tests.
var recordRejectedSubscription = func(reason string) {
	metrics.SSERejectedSubscriptions.WithLabelValues(reason).Inc()
}

// GetHub returns the singleton Hub instance.
func GetHub() *Hub {
	hubOnce.Do(func() {
		globalHub = &Hub{
			subscribers:    make(map[uint][]*subscriber),
			replay:         make(map[uint][]BusinessEvent),
			replayEvict:    make(map[uint]*time.Timer),
			businessConns:  make(map[uint]int),
			ipConns:        make(map[string]int),
			maxPerBusiness: envInt("SSE_MAX_SUBSCRIBERS_PER_BUSINESS", defaultMaxSubscribersPerBusiness),
			maxPerIP:       envInt("SSE_MAX_SUBSCRIBERS_PER_IP", defaultMaxSubscribersPerIP),
			epoch:          newHubEpoch(),

			guestBusinessConns:  make(map[uint]int),
			guestIPConns:        make(map[string]int),
			maxGuestPerBusiness: envInt("SSE_MAX_GUEST_SUBSCRIBERS_PER_BUSINESS", defaultMaxGuestSubscribersPerBusiness),
			maxGuestPerIP:       envInt("SSE_MAX_GUEST_SUBSCRIBERS_PER_IP", defaultMaxGuestSubscribersPerIP),
		}
	})
	return globalHub
}

// newHubEpoch returns a short random identifier for this process's replay ring.
// Falls back to the startup nanotime if the CSPRNG is unavailable so the epoch
// is never empty (an empty epoch would defeat the stale-resume detection).
func newHubEpoch() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// Epoch returns this process's replay-ring identifier (see Hub.epoch).
func (h *Hub) Epoch() string {
	return h.epoch
}

// HasGapAfter reports whether the retained ring for a business can no longer
// serve a resume from lastID: the oldest retained event is newer than lastID+1,
// so the events between them were evicted (256-entry overflow) and cannot be
// replayed. An empty ring is a gap only when events newer than lastID were
// published and then dropped by the idle-TTL eviction (nextID is kept, so it
// still knows the latest ID); otherwise the live channel covers the client.
func (h *Hub) HasGapAfter(businessID uint, lastID uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	buf := h.replay[businessID]
	if len(buf) == 0 {
		return lastID < h.nextID[businessID]
	}
	return buf[0].ID > lastID+1
}

// envInt reads a positive integer from the environment, falling back to def for
// an unset, empty, non-numeric, or non-positive value.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// SubscribeWithReplayTopic atomically registers a subscriber and returns the
// live channel plus a snapshot of buffered events newer than lastID. Both the
// live channel (via Publish's per-subscriber topic check) and the returned
// replay snapshot are limited to the given event types. Passing no types makes
// the topic set empty, which is the whole-business stream: the subscriber
// receives every event type. Events published after registration go to the
// live channel, which avoids replay/live duplicates on reconnect. The channel
// is buffered (64) and slow consumers have events dropped via non-blocking send.
func (h *Hub) SubscribeWithReplayTopic(businessID uint, lastID uint64, types ...string) (<-chan BusinessEvent, []BusinessEvent, func()) {
	sub := newSubscriber(businessID, types)

	h.mu.Lock()
	if h.subscribers == nil {
		h.subscribers = make(map[uint][]*subscriber)
	}
	h.subscribers[businessID] = append(h.subscribers[businessID], sub)
	h.cancelReplayEvictionLocked(businessID)
	replayed := h.replayAfterLocked(businessID, lastID)
	h.mu.Unlock()

	// Scope the replay snapshot to the subscriber's topics. An empty topic set
	// (sub.matches always true) leaves the snapshot untouched, so the
	// whole-business path returns exactly what replayAfterLocked produced.
	if len(sub.topics) > 0 {
		filtered := replayed[:0]
		for _, ev := range replayed {
			if sub.matches(ev.Type) {
				filtered = append(filtered, ev)
			}
		}
		replayed = filtered
	}

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subscribers[businessID]
		for i, s := range subs {
			if s == sub {
				h.subscribers[businessID] = append(subs[:i], subs[i+1:]...)
				// Deliberately do NOT close(sub.ch). Publish copies the subscriber
				// slice under the lock and then sends OUTSIDE the lock; closing here
				// would let a concurrent Publish send on a closed channel and panic
				// (which, from a non-HTTP publisher like the stuck-bill watchdog,
				// crashes the process). The consumer (SSE handler) terminates on its
				// request context, not on a channel close, and the now-unreferenced
				// buffered channel is reclaimed by GC.
				break
			}
		}
		if len(h.subscribers[businessID]) == 0 {
			delete(h.subscribers, businessID)
			h.scheduleReplayEvictionLocked(businessID)
		}
	}

	return sub.ch, replayed, cancel
}

// SubscribeLimited is SubscribeWithReplayTopic guarded by the hub's per-business
// AND per-IP concurrent-connection ceilings. It atomically checks both caps and
// registers the subscriber under a single lock, so two simultaneous over-cap
// arrivals cannot both slip past. On rejection it returns ok=false with a nil
// channel, nil replay, and nil cancel — the HTTP handler should then return 429
// WITHOUT having opened a stream. On success the returned cancel releases BOTH
// counters exactly once (double-cancel is safe and never underflows the count).
//
// SubscribeLimited is for AUTHENTICATED (operator/staff) streams. Public guest
// streams must use SubscribeGuestLimited, whose pool is counted separately.
//
// An empty clientIP exempts the call from the per-IP cap (the per-business cap
// still applies) so an unknown/unparseable source can't disable the guard for
// everyone else by sharing the empty key.
func (h *Hub) SubscribeLimited(businessID uint, lastID uint64, clientIP string, types []string) (<-chan BusinessEvent, []BusinessEvent, func(), bool) {
	return h.subscribeInPool(false, businessID, lastID, clientIP, types)
}

// SubscribeGuestLimited is SubscribeLimited for public, unauthenticated guest
// streams (M-sse). It enforces the guest per-business / per-IP ceilings and
// counts held connections in the guest pool only, so guests can never consume
// the capacity reserved for authenticated operator streams. Rejections are
// recorded with reasons "guest_business" / "guest_ip".
func (h *Hub) SubscribeGuestLimited(businessID uint, lastID uint64, clientIP string, types []string) (<-chan BusinessEvent, []BusinessEvent, func(), bool) {
	return h.subscribeInPool(true, businessID, lastID, clientIP, types)
}

// poolLocked returns the counters and caps of the staff or guest pool,
// allocating the counter maps on first use. Caller holds h.mu.
func (h *Hub) poolLocked(guest bool) (businessConns map[uint]int, ipConns map[string]int, maxPerBusiness, maxPerIP int, reasonPrefix string) {
	if guest {
		if h.guestBusinessConns == nil {
			h.guestBusinessConns = make(map[uint]int)
		}
		if h.guestIPConns == nil {
			h.guestIPConns = make(map[string]int)
		}
		return h.guestBusinessConns, h.guestIPConns, h.maxGuestPerBusiness, h.maxGuestPerIP, "guest_"
	}
	if h.businessConns == nil {
		h.businessConns = make(map[uint]int)
	}
	if h.ipConns == nil {
		h.ipConns = make(map[string]int)
	}
	return h.businessConns, h.ipConns, h.maxPerBusiness, h.maxPerIP, ""
}

// SubscribeGuestFiltered is SubscribeGuestLimited with a per-subscriber event
// filter applied before enqueue (and to the replay snapshot), plus a drop
// channel that is signalled when this subscriber's buffer overflows. Use it
// for a guest stream that only cares about a subset of one business's events
// (one bill), so a burst for other bills can never evict its own frames.
// filter must be safe to call concurrently and must not block.
func (h *Hub) SubscribeGuestFiltered(businessID uint, lastID uint64, clientIP string, types []string, filter func(BusinessEvent) bool) (<-chan BusinessEvent, []BusinessEvent, <-chan struct{}, func(), bool) {
	sub := newSubscriber(businessID, types)
	sub.filter = filter
	sub.dropped = make(chan struct{}, 1)
	ch, replayed, cancel, ok := h.subscribeSubInPool(true, sub, lastID, clientIP)
	if !ok {
		return nil, nil, nil, nil, false
	}
	return ch, replayed, sub.dropped, cancel, true
}

// accepts reports whether Publish should enqueue event for this subscriber.
func (s *subscriber) accepts(event BusinessEvent) bool {
	// Control frame auth.session_revoked always delivers so same-instance
	// staff streams can eject even when their topic set is narrow.
	if event.Type != EventStaffAccessRevoked && !s.matches(event.Type) {
		return false
	}
	return s.filter == nil || s.filter(event)
}

func (h *Hub) subscribeInPool(guest bool, businessID uint, lastID uint64, clientIP string, types []string) (<-chan BusinessEvent, []BusinessEvent, func(), bool) {
	return h.subscribeSubInPool(guest, newSubscriber(businessID, types), lastID, clientIP)
}

func (h *Hub) subscribeSubInPool(guest bool, sub *subscriber, lastID uint64, clientIP string) (<-chan BusinessEvent, []BusinessEvent, func(), bool) {
	businessID := sub.businessID

	h.mu.Lock()
	if h.subscribers == nil {
		h.subscribers = make(map[uint][]*subscriber)
	}
	businessConns, ipConns, maxPerBusiness, maxPerIP, reasonPrefix := h.poolLocked(guest)

	if maxPerBusiness > 0 && businessConns[businessID] >= maxPerBusiness {
		h.mu.Unlock()
		recordRejectedSubscription(reasonPrefix + "business")
		return nil, nil, nil, false
	}
	if clientIP != "" && maxPerIP > 0 && ipConns[clientIP] >= maxPerIP {
		h.mu.Unlock()
		recordRejectedSubscription(reasonPrefix + "ip")
		return nil, nil, nil, false
	}

	h.subscribers[businessID] = append(h.subscribers[businessID], sub)
	h.cancelReplayEvictionLocked(businessID)
	businessConns[businessID]++
	if clientIP != "" {
		ipConns[clientIP]++
	}
	replayed := h.replayAfterLocked(businessID, lastID)
	h.mu.Unlock()

	// Scope the replay snapshot to the subscriber's topics and filter (mirrors
	// SubscribeWithReplayTopic; an empty topic set leaves it untouched).
	if len(sub.topics) > 0 || sub.filter != nil {
		filtered := replayed[:0]
		for _, ev := range replayed {
			if sub.matches(ev.Type) && (sub.filter == nil || sub.filter(ev)) {
				filtered = append(filtered, ev)
			}
		}
		replayed = filtered
	}

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			subs := h.subscribers[businessID]
			for i, s := range subs {
				if s == sub {
					// Deliberately do NOT close(sub.ch); see SubscribeWithReplayTopic.
					h.subscribers[businessID] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
			if len(h.subscribers[businessID]) == 0 {
				delete(h.subscribers, businessID)
				h.scheduleReplayEvictionLocked(businessID)
			}
			businessConns, ipConns, _, _, _ := h.poolLocked(guest)
			if businessConns[businessID] > 0 {
				businessConns[businessID]--
			}
			if businessConns[businessID] == 0 {
				delete(businessConns, businessID)
			}
			if clientIP != "" {
				if ipConns[clientIP] > 0 {
					ipConns[clientIP]--
				}
				if ipConns[clientIP] == 0 {
					delete(ipConns, clientIP)
				}
			}
		})
	}

	return sub.ch, replayed, cancel, true
}

// SetGuestConnectionLimits overrides the guest-pool ceilings (see
// SubscribeGuestLimited). A non-positive value leaves the corresponding cap
// unchanged.
func (h *Hub) SetGuestConnectionLimits(maxPerBusiness, maxPerIP int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if maxPerBusiness > 0 {
		h.maxGuestPerBusiness = maxPerBusiness
	}
	if maxPerIP > 0 {
		h.maxGuestPerIP = maxPerIP
	}
}

// businessConnCount returns the current held-connection count for a business.
// Test/observability helper; takes the read lock.
func (h *Hub) businessConnCount(businessID uint) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.businessConns[businessID]
}

// ipConnCount returns the current held-connection count for a client IP.
// Test/observability helper; takes the read lock.
func (h *Hub) ipConnCount(clientIP string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ipConns[clientIP]
}

// Publish sends an event to all subscribers of the given business.
// Non-blocking: if a subscriber's channel is full the event is dropped.
func (h *Hub) Publish(event BusinessEvent) {
	// Strip guest bearer capabilities before the replay ring and the fan-out.
	event.Data = scrubBusinessEventData(event.Type, event.Data)
	h.mu.Lock()
	if h.replay == nil {
		h.replay = make(map[uint][]BusinessEvent)
	}
	if h.nextID == nil {
		h.nextID = make(map[uint]uint64)
	}
	if event.ID == 0 {
		h.nextID[event.BusinessID]++
		event.ID = h.nextID[event.BusinessID]
	} else if event.ID > h.nextID[event.BusinessID] {
		h.nextID[event.BusinessID] = event.ID
	}
	buf := append(h.replay[event.BusinessID], event)
	if len(buf) > replayBufferSize {
		// Copy the tail so the oversized backing array (still holding evicted
		// events) can be garbage-collected.
		trimmed := make([]BusinessEvent, replayBufferSize)
		copy(trimmed, buf[len(buf)-replayBufferSize:])
		buf = trimmed
	}
	h.replay[event.BusinessID] = buf
	// A business that only ever publishes with nobody listening is reclaimed
	// too. Do not restart a timer that is already pending: every publish would
	// otherwise keep the ring alive forever.
	if len(h.subscribers[event.BusinessID]) == 0 && h.replayEvict[event.BusinessID] == nil {
		h.scheduleReplayEvictionLocked(event.BusinessID)
	}
	subs := append([]*subscriber(nil), h.subscribers[event.BusinessID]...)
	h.mu.Unlock()

	for _, sub := range subs {
		// Topic filter: a topic-scoped subscriber never sees event types it
		// didn't subscribe to, so an unrelated burst can't fill its bounded
		// channel and evict the types it does want. Reading sub.topics here is
		// lock-free; it is immutable after subscribe (see subscriber.topics).
		// A per-subscriber filter (SubscribeGuestFiltered) is applied the same
		// way, before enqueue.
		if !sub.accepts(event) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			recordDroppedBusinessEvent(event)
			if sub.dropped != nil {
				select {
				case sub.dropped <- struct{}{}:
				default:
				}
			}
		}
	}
}

// scheduleReplayEvictionLocked arms a timer that drops the business's replay
// ring after the idle TTL once nobody is listening. Caller holds h.mu.
// A business that still has subscribers is left alone. An existing timer is
// stopped and replaced so the idle window restarts from now.
func (h *Hub) scheduleReplayEvictionLocked(businessID uint) {
	if len(h.subscribers[businessID]) > 0 {
		return
	}
	if h.replayEvict == nil {
		h.replayEvict = make(map[uint]*time.Timer)
	}
	if existing := h.replayEvict[businessID]; existing != nil {
		existing.Stop()
	}
	ttl := h.replayIdleTTL
	if ttl <= 0 {
		ttl = defaultReplayIdleTTL
	}
	var t *time.Timer
	t = time.AfterFunc(ttl, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		// A timer that was stopped or replaced after it fired, but before it
		// got the lock, must not act.
		if h.replayEvict[businessID] != t {
			return
		}
		delete(h.replayEvict, businessID)
		// nextID stays. Clients resume with Last-Event-ID, so event IDs must
		// remain monotonic for the life of the process.
		if len(h.subscribers[businessID]) == 0 {
			delete(h.replay, businessID)
		}
	})
	h.replayEvict[businessID] = t
}

// cancelReplayEvictionLocked stops a pending replay eviction. Caller holds h.mu.
func (h *Hub) cancelReplayEvictionLocked(businessID uint) {
	if h.replayEvict == nil {
		return
	}
	if t := h.replayEvict[businessID]; t != nil {
		t.Stop()
		delete(h.replayEvict, businessID)
	}
}

// ReplayAfter returns buffered events for a business with IDs greater than lastID.
func (h *Hub) ReplayAfter(businessID uint, lastID uint64) []BusinessEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.replayAfterLocked(businessID, lastID)
}

func (h *Hub) replayAfterLocked(businessID uint, lastID uint64) []BusinessEvent {
	events := h.replay[businessID]
	replayed := make([]BusinessEvent, 0, len(events))
	for _, event := range events {
		if event.ID > lastID {
			replayed = append(replayed, event)
		}
	}
	return replayed
}

// PublishJSON is a convenience wrapper that marshals data to JSON and publishes.
func (h *Hub) PublishJSON(businessID uint, eventType string, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.Publish(BusinessEvent{
		BusinessID: businessID,
		Type:       eventType,
		Data:       raw,
		Timestamp:  time.Now(),
	})
}
