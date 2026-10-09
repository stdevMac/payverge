// Package aiwaiterevents is an in-process pub/sub hub for guest AI-waiter
// conversation events, scoped by the authenticated conversation ID. The HTTP
// handler first resolves the server-issued session token plus business/table
// scope to a conversation, then subscribes by that immutable database ID.
// This lets HandleAIWaiter (assistant reply) and PostAiReply (staff takeover) push
// updates the moment a row is persisted, replacing the guest's 3s poll.
package aiwaiterevents

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
)

// ErrLimit is returned when a conversation or the process is already at its
// live AI-waiter stream cap. Callers should answer 429 and let the guest poll.
var ErrLimit = errors.New("ai waiter event connection limit reached")

// subscriberBufferSize bounds per-subscriber backlog. A guest chat turn
// produces at most a couple of events; 16 is generous and a slow/stalled
// consumer drops rather than blocking the publisher.
const subscriberBufferSize = 16

// MessageEvent is the payload pushed to a guest SSE stream when a new
// assistant (or staff-takeover) message is persisted. It mirrors the fields
// services.WaiterMessage exposes to the transcript so the frontend can append
// without re-fetching.
type MessageEvent struct {
	ID         uint                        `json:"id"`
	Role       string                      `json:"role"`
	Content    string                      `json:"content"`
	CreatedAt  int64                       `json:"createdAt"`
	ResponseV2 *assistantcontract.Response `json:"response_v2,omitempty"`
}

type subscriber struct {
	ch             chan MessageEvent
	conversationID uint
}

// Hub fans MessageEvents to subscribers keyed by authenticated conversation ID.
// maxPerConversation, maxPerIP and maxTotal cap SubscribeLimited only.
// Subscribe stays uncapped for in-process listeners.
type Hub struct {
	mu                 sync.RWMutex
	subscribers        map[uint][]*subscriber
	perIP              map[string]int
	total              int
	maxPerConversation int
	maxPerIP           int
	maxTotal           int
}

var (
	globalHub *Hub
	hubOnce   sync.Once
)

// Production SubscribeLimited caps. Guests on shared venue Wi-Fi present one
// client IP, so the per-IP cap matches the public table SSE hub (60) and the
// per-conversation cap is what bounds a single guest.
const (
	DefaultMaxPerConversation = 4
	DefaultMaxPerIP           = 60
	DefaultMaxTotal           = 2000
)

// NewHub builds an empty hub with the production connection caps.
func NewHub() *Hub {
	return NewHubWithLimits(DefaultMaxPerConversation, DefaultMaxPerIP, DefaultMaxTotal)
}

// NewHubWithLimits builds an empty hub whose SubscribeLimited caps are the
// given per-conversation, per-IP and total limits.
func NewHubWithLimits(perConversation, perIP, total int) *Hub {
	return &Hub{
		subscribers:        make(map[uint][]*subscriber),
		perIP:              make(map[string]int),
		maxPerConversation: perConversation,
		maxPerIP:           perIP,
		maxTotal:           total,
	}
}

// GetHub returns the process-wide singleton used by handlers.
func GetHub() *Hub {
	hubOnce.Do(func() { globalHub = NewHub() })
	return globalHub
}

// SetHubForTest installs h as the process-wide hub and returns a function
// that restores the previous hub. Test-only: production code must use GetHub.
func SetHubForTest(h *Hub) (restore func()) {
	previous := GetHub()
	globalHub = h
	restore = func() { globalHub = previous }
	return restore
}

// Subscribe registers a reader for one resolved conversation and returns a
// buffered read channel plus a cancel func. Cancel deliberately does NOT close the
// channel: Publish copies the subscriber slice under the lock and sends
// OUTSIDE it, so closing here could let a concurrent Publish send on a
// closed channel and panic the process (same discipline as
// internal/events.Hub). The SSE handler terminates on its request context,
// and the now-unreferenced channel is reclaimed by GC.
//
// ErrLimit is returned, with a nil channel and a nil cancel, when this
// conversation or the process is already at its live-stream cap. A limit of
// zero or less means that cap is disabled.
func (h *Hub) Subscribe(conversationID uint) (<-chan MessageEvent, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers == nil {
		h.subscribers = make(map[uint][]*subscriber)
	}
	if (h.maxPerConversation > 0 && len(h.subscribers[conversationID]) >= h.maxPerConversation) ||
		(h.maxTotal > 0 && h.total >= h.maxTotal) {
		return nil, nil, ErrLimit
	}
	sub := &subscriber{ch: make(chan MessageEvent, subscriberBufferSize), conversationID: conversationID}
	h.subscribers[conversationID] = append(h.subscribers[conversationID], sub)
	h.total++

	var once sync.Once
	cancel := func() {
		// Idempotent: a second call must not free another slot.
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			subs := h.subscribers[conversationID]
			removed := false
			for i, s := range subs {
				if s == sub {
					h.subscribers[conversationID] = append(subs[:i], subs[i+1:]...)
					removed = true
					break
				}
			}
			if removed {
				h.total--
			}
			if len(h.subscribers[conversationID]) == 0 {
				delete(h.subscribers, conversationID)
			}
		})
	}
	return sub.ch, cancel, nil
}

// SubscribeLimited registers a reader for one conversation under the hub caps.
// It rejects (ok=false) when that conversation already has maxPerConversation
// subscribers, when clientIP (unless empty) already has maxPerIP subscribers,
// or when the hub is at maxTotal. The cancel func is idempotent.
func (h *Hub) SubscribeLimited(conversationID uint, clientIP string) (<-chan MessageEvent, func(), bool) {
	sub := &subscriber{ch: make(chan MessageEvent, subscriberBufferSize), conversationID: conversationID}

	h.mu.Lock()
	if h.subscribers == nil {
		h.subscribers = make(map[uint][]*subscriber)
	}
	if h.perIP == nil {
		h.perIP = make(map[string]int)
	}
	if len(h.subscribers[conversationID]) >= h.maxPerConversation ||
		(clientIP != "" && h.perIP[clientIP] >= h.maxPerIP) ||
		h.total >= h.maxTotal {
		h.mu.Unlock()
		return nil, nil, false
	}
	h.subscribers[conversationID] = append(h.subscribers[conversationID], sub)
	h.total++
	if clientIP != "" {
		h.perIP[clientIP]++
	}
	h.mu.Unlock()

	// Cancel does not close the channel: Publish sends outside the lock.
	// sync.Once keeps a second cancel from decrementing the counters again.
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			subs := h.subscribers[conversationID]
			for i, s := range subs {
				if s != sub {
					continue
				}
				h.subscribers[conversationID] = append(subs[:i], subs[i+1:]...)
				h.total--
				if clientIP != "" {
					h.perIP[clientIP]--
					if h.perIP[clientIP] == 0 {
						delete(h.perIP, clientIP)
					}
				}
				break
			}
			if len(h.subscribers[conversationID]) == 0 {
				delete(h.subscribers, conversationID)
			}
		})
	}
	return sub.ch, cancel, true
}

// PublishMessage delivers an event to every subscriber of the conversation.
// Non-blocking: a full subscriber buffer drops the event (the frontend's
// poll fallback and on-open hydrate cover any dropped frame).
func (h *Hub) PublishMessage(conversationID uint, event MessageEvent) {
	if conversationID == 0 {
		return
	}
	if event.Role != "assistant" {
		event.ResponseV2 = nil
	} else if event.ResponseV2 != nil {
		if err := assistantcontract.Validate(*event.ResponseV2); err != nil {
			event.ResponseV2 = nil
		} else {
			event.Content = event.ResponseV2.Answer.Content
		}
	}
	h.mu.RLock()
	subs := append([]*subscriber(nil), h.subscribers[conversationID]...)
	h.mu.RUnlock()

	for _, sub := range subs {
		cloned := cloneMessageEvent(event)
		select {
		case sub.ch <- cloned:
		default:
		}
	}
}

// cloneMessageEvent prevents the publisher or one subscriber from mutating
// the structured slices/pointers observed by another subscriber. The contract
// is JSON-safe by definition; a marshal failure drops only the structured
// attachment while retaining the legacy event fields.
func cloneMessageEvent(event MessageEvent) MessageEvent {
	cloned := event
	if event.ResponseV2 == nil {
		return cloned
	}
	raw, err := json.Marshal(event.ResponseV2)
	if err != nil {
		cloned.ResponseV2 = nil
		return cloned
	}
	var response assistantcontract.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		cloned.ResponseV2 = nil
		return cloned
	}
	cloned.ResponseV2 = &response
	return cloned
}
