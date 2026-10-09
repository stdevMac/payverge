package bench_test

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/events"
)

// BenchmarkSSEBroadcast measures the cost of a single events.Hub.Publish
// fanned out across many subscribers for one business. All 5000 subscribers
// register for businessID=1 so each Publish iterates the full slice — this
// is the worst-case fanout we care about (peak kitchen with many open SSE
// connections). The channels are buffered (64), and Publish is non-blocking,
// so the bench measures slice iteration + non-blocking select cost rather
// than scheduler latency. No DB required.
func BenchmarkSSEBroadcast(b *testing.B) {
	hub := events.GetHub()
	const subs = 5000
	cancels := make([]func(), 0, subs)
	for i := 0; i < subs; i++ {
		_, _, cancel := hub.SubscribeWithReplayTopic(uint(1), 0)
		cancels = append(cancels, cancel)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	payload, _ := json.Marshal(map[string]string{"order_id": "abc"})
	event := events.BusinessEvent{
		BusinessID: 1,
		Type:       "order.created",
		Data:       payload,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Publish(event)
	}
}

// BenchmarkSSEBroadcast_LowFanout is the realistic-load comparison point: a
// single business with only 10 concurrent SSE subscribers. Pair with
// BenchmarkSSEBroadcast to compare per-event cost across fanout sizes.
func BenchmarkSSEBroadcast_LowFanout(b *testing.B) {
	hub := events.GetHub()
	const subs = 10
	cancels := make([]func(), 0, subs)
	for i := 0; i < subs; i++ {
		_, _, cancel := hub.SubscribeWithReplayTopic(uint(2), 0)
		cancels = append(cancels, cancel)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	payload, _ := json.Marshal(map[string]string{"order_id": "abc"})
	event := events.BusinessEvent{
		BusinessID: 2,
		Type:       "order.created",
		Data:       payload,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Publish(event)
	}
}
