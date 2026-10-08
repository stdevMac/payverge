package circuitbreaker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBreakerTripsAndRecovers(t *testing.T) {
	b := New(Config{Threshold: 3, OpenFor: 50 * time.Millisecond})
	boom := errors.New("upstream down")

	// 3 consecutive failures must trip the breaker.
	for i := 0; i < 3; i++ {
		_ = b.Do(func() error { return boom })
	}
	if err := b.Do(func() error { return nil }); !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen while breaker is open, got %v", err)
	}

	// After OpenFor elapses, a probe (half-open) is allowed; success closes it.
	time.Sleep(60 * time.Millisecond)
	if err := b.Do(func() error { return nil }); err != nil {
		t.Fatalf("expected half-open probe to be allowed and succeed, got %v", err)
	}
	if err := b.Do(func() error { return nil }); err != nil {
		t.Fatalf("expected closed breaker after successful probe, got %v", err)
	}
}

// TestBreakerHalfOpenConcurrentProbeFailureReopens locks in the fix for the
// half-open probe race: when two probes run concurrently in half-open and a
// SUCCESS finishes (clearing halfOpen) before a concurrent FAILURE records its
// result, the failure must still re-open the breaker. Pre-fix the failure saw
// halfOpen=false + failures<threshold and silently left the breaker closed.
func TestBreakerHalfOpenConcurrentProbeFailureReopens(t *testing.T) {
	boom := errors.New("upstream down")
	b := New(Config{Threshold: 5, OpenFor: 80 * time.Millisecond})

	// Trip it open (threshold consecutive failures).
	for i := 0; i < 5; i++ {
		_ = b.Do(func() error { return boom })
	}
	if err := b.Do(func() error { return nil }); err != ErrOpen {
		t.Fatalf("breaker should be open right after tripping; got %v", err)
	}

	// Let the open window elapse so the next callers enter half-open.
	time.Sleep(100 * time.Millisecond)

	var entered sync.WaitGroup
	entered.Add(2)
	releaseSuccess := make(chan struct{})
	releaseFail := make(chan struct{})
	successDone := make(chan struct{})
	failDone := make(chan struct{})

	// Both probes signal entry (so both capture probing=true) then block, so we
	// can deterministically order: success completes its result first, failure
	// second — the exact interleaving that defeated the pre-fix logic.
	go func() {
		_ = b.Do(func() error { entered.Done(); <-releaseSuccess; return nil })
		close(successDone)
	}()
	go func() {
		_ = b.Do(func() error { entered.Done(); <-releaseFail; return boom })
		close(failDone)
	}()

	entered.Wait()        // both probes are past the entry critical section
	close(releaseSuccess) // success records first -> would clear halfOpen
	<-successDone
	close(releaseFail) // failure records second -> must still re-open
	<-failDone

	if err := b.Do(func() error { return nil }); err != ErrOpen {
		t.Fatalf("a failed half-open probe must re-open the breaker even if a sibling success raced ahead; got %v", err)
	}
}
