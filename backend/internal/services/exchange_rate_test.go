package services

import (
	"errors"
	"testing"
)

// TestRunRateFetchTickRecoversPanic is the regression guard for the periodic
// exchange-rate loop. The loop runs in its own goroutine and previously called
// FetchLatestRates() directly inside `for range ticker.C`; a single panic deep
// in the fetch (e.g. a driver/GORM panic) would unwind the loop and permanently
// freeze rate refresh at stale values until the next process restart — with no
// self-heal. runRateFetchTick must contain a panicking tick (via logger.SafeTick)
// so the loop survives to the next tick. If the SafeTick wrapping is removed,
// the panic escapes here and the test fails.
func TestRunRateFetchTickRecoversPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped runRateFetchTick — periodic loop would die and freeze rates; got: %v", r)
		}
	}()

	runRateFetchTick(func() error { panic("simulated driver panic inside FetchLatestRates") })
}

// TestRunRateFetchTickHandlesError verifies the ordinary error path: a returned
// error is logged and swallowed (the loop keeps running), never propagated.
func TestRunRateFetchTickHandlesError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a returned error must not panic the tick; got: %v", r)
		}
	}()

	runRateFetchTick(func() error { return errors.New("upstream 503") })
}
