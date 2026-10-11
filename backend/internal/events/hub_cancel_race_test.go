package events

import (
	"sync"
	"testing"
)

// TestHub_PublishDuringCancelDoesNotPanic stresses the race between Publish
// (which copies the subscriber slice under lock, then sends outside the lock)
// and a subscriber cancelling. If cancel closes the subscriber channel, a
// concurrent Publish can send on a closed channel and panic (a closed channel
// makes the send case "ready", so the select's default does not save it).
// Background publishers (e.g. the stuck-bill watchdog) are not behind Gin's
// recovery, so that panic crashes the process. This test must complete without
// panicking.
func TestHub_PublishDuringCancelDoesNotPanic(t *testing.T) {
	hub := &Hub{}

	const businessID = 99
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Publisher: tight loop fanning out to the business's subscribers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			hub.Publish(BusinessEvent{BusinessID: businessID, Type: "tick"})
		}
	}()

	// A few long-lived subscribers so the publish fan-out loop is non-trivial,
	// widening the copy→send window for the churning subscribers below.
	var longLived []func()
	for i := 0; i < 8; i++ {
		_, _, cancel := hub.SubscribeWithReplayTopic(businessID, 0)
		longLived = append(longLived, cancel)
	}

	// Rapidly subscribe then cancel, racing the publisher's send loop.
	for i := 0; i < 5000; i++ {
		_, _, cancel := hub.SubscribeWithReplayTopic(businessID, 0)
		cancel()
	}

	close(stop)
	wg.Wait()
	for _, cancel := range longLived {
		cancel()
	}
}
