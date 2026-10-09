package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubPublishRecordsDroppedEvents(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	var dropped []BusinessEvent
	previousRecorder := recordDroppedBusinessEvent
	recordDroppedBusinessEvent = func(event BusinessEvent) {
		dropped = append(dropped, event)
	}
	t.Cleanup(func() {
		recordDroppedBusinessEvent = previousRecorder
	})

	ch, _, cancel := hub.SubscribeWithReplayTopic(42, 0)
	defer cancel()

	for i := 0; i < subscriberBufferSize; i++ {
		hub.Publish(BusinessEvent{BusinessID: 42, Type: "order.updated", Timestamp: time.Now()})
	}
	require.Len(t, ch, subscriberBufferSize)

	hub.Publish(BusinessEvent{BusinessID: 42, Type: "payment.received", Timestamp: time.Now()})

	require.Len(t, dropped, 1)
	assert.Equal(t, uint(42), dropped[0].BusinessID)
	assert.Equal(t, "payment.received", dropped[0].Type)
}

func TestHubPublishAssignsIDsAndReplaysEventsAfterLastID(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	hub.Publish(BusinessEvent{
		BusinessID: 42,
		Type:       "order.created",
		Data:       json.RawMessage(`{"order_id":1}`),
		Timestamp:  time.Now(),
	})
	hub.Publish(BusinessEvent{
		BusinessID: 42,
		Type:       "order.updated",
		Data:       json.RawMessage(`{"order_id":1}`),
		Timestamp:  time.Now(),
	})
	hub.Publish(BusinessEvent{
		BusinessID: 99,
		Type:       "order.created",
		Data:       json.RawMessage(`{"order_id":2}`),
		Timestamp:  time.Now(),
	})

	all := hub.ReplayAfter(42, 0)
	require.Len(t, all, 2)
	assert.NotZero(t, all[0].ID)
	assert.Greater(t, all[1].ID, all[0].ID)

	replayed := hub.ReplayAfter(42, all[0].ID)
	require.Len(t, replayed, 1)
	assert.Equal(t, "order.updated", replayed[0].Type)
}

func TestHubPublishIDsArePerBusiness(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	hub.Publish(BusinessEvent{BusinessID: 1, Type: "a", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: 1, Type: "b", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: 2, Type: "a", Timestamp: time.Now()})

	a := hub.ReplayAfter(1, 0)
	b := hub.ReplayAfter(2, 0)
	require.Len(t, a, 2)
	require.Len(t, b, 1)
	assert.Equal(t, uint64(1), a[0].ID)
	assert.Equal(t, uint64(2), a[1].ID)
	assert.Equal(t, uint64(1), b[0].ID, "public ids must not share a global counter (#536)")

	replayed := hub.ReplayAfter(1, 1)
	require.Len(t, replayed, 1)
	assert.Equal(t, "b", replayed[0].Type)
}

// TestHubPublishDoesNotBlockOnFullSubscriber asserts the hot fan-out path is
// non-blocking: once a subscriber's 64-slot buffer is full, further Publish
// calls must drop + count and return immediately rather than blocking the
// publisher (which would stall the whole event system behind one slow client).
func TestHubPublishDoesNotBlockOnFullSubscriber(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	var droppedCount int
	previousRecorder := recordDroppedBusinessEvent
	recordDroppedBusinessEvent = func(event BusinessEvent) { droppedCount++ }
	t.Cleanup(func() { recordDroppedBusinessEvent = previousRecorder })

	// Register a subscriber but never drain it.
	_, _, cancel := hub.SubscribeWithReplayTopic(7, 0)
	defer cancel()

	// Fill the buffer, then publish far past capacity. If Publish blocked on a
	// full channel this goroutine would never finish and the test would hang.
	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBufferSize+100; i++ {
			hub.Publish(BusinessEvent{BusinessID: 7, Type: "order.updated", Timestamp: time.Now()})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("hub.Publish blocked on a full subscriber buffer")
	}

	// Exactly the overflow beyond the buffer must have been dropped + counted.
	require.Equal(t, 100, droppedCount)
}

// BenchmarkPublishFullSubscriber measures the per-event cost of the fan-out
// hot path when the only subscriber's buffer is saturated (the worst case for
// backpressure: every Publish hits the drop+count branch). Captures bytes/op
// and allocs/op so the recorder hook and label lookup stay cheap.
func BenchmarkPublishFullSubscriber(b *testing.B) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}
	recordDroppedBusinessEvent = func(event BusinessEvent) {}

	_, _, cancel := hub.SubscribeWithReplayTopic(1, 0)
	defer cancel()
	// Saturate the buffer so every benchmarked Publish drops.
	for i := 0; i < subscriberBufferSize; i++ {
		hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.updated"})
	}

	evt := BusinessEvent{BusinessID: 1, Type: "order.updated"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Publish(evt)
	}
}
