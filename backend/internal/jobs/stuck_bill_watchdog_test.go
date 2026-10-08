package jobs

import (
	"testing"
	"time"
)

// TestStuckBillWatchdog_loopSurvivesPanickingTick proves the watchdog's real
// run() loop does NOT crash the process when a tick panics.
//
// A nil *gorm.DB makes RunOnce panic (nil-pointer deref in db.Table(...)).
// Before the SafeTick wrapping, that panic would unwind the watchdog goroutine
// — and an unrecovered panic in any goroutine terminates the ENTIRE process
// (all tenants). With logger.SafeTick wrapping both the initial sweep and the
// per-tick sweep, the panic is recovered + logged and the loop stays alive, so
// Start()/Stop() complete normally instead of taking the process down.
func TestStuckBillWatchdog_loopSurvivesPanickingTick(t *testing.T) {
	// nil db => every RunOnce panics. A short interval forces a panicking tick
	// (not just the initial sweep) to exercise the per-tick SafeTick too.
	w := NewStuckBillWatchdog(nil, StuckBillWatchdogConfig{Interval: 10 * time.Millisecond})

	w.Start()
	// Let several ticks fire and panic; the loop must keep running.
	time.Sleep(60 * time.Millisecond)

	// Stop() blocks on the loop's done channel. If the goroutine had crashed
	// (panic propagated) the process would already be gone; reaching a clean
	// Stop() here proves the loop survived the panicking ticks.
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
		// Loop was alive and shut down cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog loop did not stop cleanly after panicking ticks (loop likely died)")
	}
}
