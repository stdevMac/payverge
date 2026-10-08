package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayEvictedAfterIdleButIDsStayMonotonic(t *testing.T) {
	hub := &Hub{replayIdleTTL: 20 * time.Millisecond}
	_, _, cancel := hub.SubscribeWithReplayTopic(1, 0)
	hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.created", Timestamp: time.Now()})
	first := hub.ReplayAfter(1, 0)
	require.Len(t, first, 1)
	firstID := first[0].ID
	cancel()

	require.Len(t, hub.ReplayAfter(1, 0), 1, "ring stays until the idle TTL")
	require.Eventually(t, func() bool {
		return len(hub.ReplayAfter(1, 0)) == 0
	}, time.Second, 5*time.Millisecond)
	// The evicted event cannot be replayed, so a client that last saw an
	// older ID must resync; one that saw the latest ID is current.
	assert.True(t, hub.HasGapAfter(1, firstID-1), "evicted events are a gap")
	assert.False(t, hub.HasGapAfter(1, firstID), "caught-up client has no gap")

	hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.updated", Timestamp: time.Now()})
	again := hub.ReplayAfter(1, 0)
	require.Len(t, again, 1)
	assert.Greater(t, again[0].ID, firstID)
}

func TestReplayNotEvictedWhileSubscribed(t *testing.T) {
	ttl := 20 * time.Millisecond
	hub := &Hub{replayIdleTTL: ttl}
	_, _, cancel := hub.SubscribeWithReplayTopic(1, 0)
	defer cancel()
	hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.created", Timestamp: time.Now()})

	time.Sleep(3 * ttl)
	require.Len(t, hub.ReplayAfter(1, 0), 1)
}

func TestResubscribeCancelsReplayEviction(t *testing.T) {
	ttl := 20 * time.Millisecond
	hub := &Hub{replayIdleTTL: ttl}
	_, _, cancel := hub.SubscribeWithReplayTopic(1, 0)
	hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.created", Timestamp: time.Now()})
	cancel()

	_, _, cancelAgain := hub.SubscribeWithReplayTopic(1, 0)
	defer cancelAgain()
	time.Sleep(3 * ttl)
	require.Len(t, hub.ReplayAfter(1, 0), 1)
}

func TestPublishWithoutSubscribersSchedulesReplayEviction(t *testing.T) {
	hub := &Hub{replayIdleTTL: 20 * time.Millisecond}
	hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.created", Timestamp: time.Now()})
	require.Len(t, hub.ReplayAfter(1, 0), 1)
	require.Eventually(t, func() bool {
		return len(hub.ReplayAfter(1, 0)) == 0
	}, time.Second, 5*time.Millisecond)
}

func TestPublishReplayRingTrimReleasesBackingArray(t *testing.T) {
	hub := &Hub{replayIdleTTL: time.Hour}
	t.Cleanup(func() {
		hub.mu.Lock()
		hub.cancelReplayEvictionLocked(1)
		hub.mu.Unlock()
	})

	for i := 0; i < replayBufferSize+10; i++ {
		hub.Publish(BusinessEvent{BusinessID: 1, Type: "order.updated", Timestamp: time.Now()})
	}

	hub.mu.Lock()
	defer hub.mu.Unlock()
	buf := hub.replay[1]
	require.Len(t, buf, replayBufferSize)
	require.Equal(t, replayBufferSize, cap(buf))
}
