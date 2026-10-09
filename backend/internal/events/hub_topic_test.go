package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubscribeTopic_NotStarvedByUnrelatedBurst is the load-bearing correctness
// test for SSE-01. A topic subscriber (the guest split SSE only ever needs
// "bill.split.updated") must NOT have its relevant frame evicted by an
// unrelated event burst. Before the topic filter, every event type for the
// business entered the 64-slot channel, so a flurry of order.updated could fill
// it and the subscriber's own bill.split.updated frame would be silently dropped
// by Publish's non-blocking send. With topic filtering, unrelated types never
// enter the channel, so the relevant frame always survives.
func TestSubscribeTopic_NotStarvedByUnrelatedBurst(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	const bizID = 42
	ch, _, cancel := hub.SubscribeWithReplayTopic(bizID, 0, "bill.split.updated")
	defer cancel()

	// Publish far more unrelated events than the buffer can hold. If these
	// entered the channel they would saturate it and evict the split frame.
	for i := 0; i < 200; i++ {
		hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.updated", Timestamp: time.Now()})
	}

	// The one frame the guest actually cares about.
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.split.updated", Timestamp: time.Now()})

	select {
	case got := <-ch:
		assert.Equal(t, "bill.split.updated", got.Type, "topic subscriber must only see its topic and the relevant frame must survive the burst")
	case <-time.After(2 * time.Second):
		t.Fatal("bill.split.updated frame was starved out by the unrelated burst")
	}

	// No further events should be queued: the unrelated burst was filtered out
	// entirely, so the channel holds only the single split frame.
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra event in topic channel: %s", extra.Type)
	default:
	}
}

// TestSubscribeTopic_DropsUnrelatedTypes asserts a topic subscriber receives
// ONLY its declared topic types; unrelated types are never delivered.
func TestSubscribeTopic_DropsUnrelatedTypes(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	const bizID = 7
	ch, _, cancel := hub.SubscribeWithReplayTopic(bizID, 0, "bill.split.updated")
	defer cancel()

	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "payment.received", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.split.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "reservation.created", Timestamp: time.Now()})

	require.Len(t, ch, 1, "only the single bill.split.updated event should have entered the channel")
	got := <-ch
	assert.Equal(t, "bill.split.updated", got.Type)
}

// TestSubscribeTopic_MultipleTypes confirms a subscriber may declare several
// topics and gets each of them, but nothing outside the set.
func TestSubscribeTopic_MultipleTypes(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	const bizID = 11
	ch, _, cancel := hub.SubscribeWithReplayTopic(bizID, 0, "bill.split.updated", "payment.received")
	defer cancel()

	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "payment.received", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.split.updated", Timestamp: time.Now()})

	require.Len(t, ch, 2)
	first := <-ch
	second := <-ch
	assert.Equal(t, "payment.received", first.Type)
	assert.Equal(t, "bill.split.updated", second.Type)
}

// TestSubscribe_StillReceivesAllTypes guards against regression: the existing
// un-topic'd Subscribe must keep delivering every event type for the business
// (operator dashboards depend on the whole-business stream).
func TestSubscribe_StillReceivesAllTypes(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	const bizID = 5
	ch, _, cancel := hub.SubscribeWithReplayTopic(bizID, 0)
	defer cancel()

	types := []string{"order.updated", "payment.received", "bill.split.updated", "reservation.created"}
	for _, typ := range types {
		hub.Publish(BusinessEvent{BusinessID: bizID, Type: typ, Timestamp: time.Now()})
	}

	require.Len(t, ch, len(types), "un-topic'd subscriber must receive all types")
	for _, want := range types {
		got := <-ch
		assert.Equal(t, want, got.Type)
	}
}

// TestSubscribeWithReplayTopic_FiltersReplay asserts the replay snapshot handed
// back at subscribe time is limited to the topic's buffered events, mirroring
// the live-path filter so the SSE handler never re-processes unrelated history.
func TestSubscribeWithReplayTopic_FiltersReplay(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	const bizID = 8
	// Pre-populate the replay buffer with a mix of types BEFORE subscribing.
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.split.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.split.updated", Timestamp: time.Now()})

	_, replayed, cancel := hub.SubscribeWithReplayTopic(bizID, 0, "bill.split.updated")
	defer cancel()

	require.Len(t, replayed, 2, "replay must be limited to the topic types")
	for _, ev := range replayed {
		assert.Equal(t, "bill.split.updated", ev.Type)
	}
}

// TestSubscribeWithReplay_StillReplaysAllTypes guards the un-topic'd replay path
// against regression.
func TestSubscribeWithReplay_StillReplaysAllTypes(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}

	const bizID = 9
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "order.updated", Timestamp: time.Now()})
	hub.Publish(BusinessEvent{BusinessID: bizID, Type: "bill.split.updated", Timestamp: time.Now()})

	_, replayed, cancel := hub.SubscribeWithReplayTopic(bizID, 0)
	defer cancel()

	require.Len(t, replayed, 2, "un-topic'd replay must include all buffered types")
}

// BenchmarkPublishMixed_Before measures the per-Publish cost when an un-topic'd
// subscriber is drained on the fly under a realistic mixed load where only a
// small fraction of events are the one the consumer actually wants. EVERY event
// is sent into the channel (and drained), so the subscriber pays for the full
// firehose.
func BenchmarkPublishMixed_Before(b *testing.B) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}
	recordDroppedBusinessEvent = func(event BusinessEvent) {}

	ch, _, cancel := hub.SubscribeWithReplayTopic(1, 0)
	defer cancel()

	// Drain in the background so the channel never saturates and we measure the
	// send cost rather than the drop branch.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-ch:
			}
		}
	}()

	types := []string{"order.updated", "order.updated", "order.updated", "payment.received", "bill.split.updated"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Publish(BusinessEvent{BusinessID: 1, Type: types[i%len(types)]})
	}
	b.StopTimer()
	close(stop)
}

// BenchmarkPublishMixed_After measures the same mixed load against a topic
// subscriber that only wants "bill.split.updated". Unrelated types skip the
// channel send entirely, so the filtered subscriber's drain cost (and channel
// pressure) drops to the ~1/5 of events it actually wants.
func BenchmarkPublishMixed_After(b *testing.B) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}
	recordDroppedBusinessEvent = func(event BusinessEvent) {}

	ch, _, cancel := hub.SubscribeWithReplayTopic(1, 0, "bill.split.updated")
	defer cancel()

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-ch:
			}
		}
	}()

	types := []string{"order.updated", "order.updated", "order.updated", "payment.received", "bill.split.updated"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Publish(BusinessEvent{BusinessID: 1, Type: types[i%len(types)]})
	}
	b.StopTimer()
	close(stop)
}
