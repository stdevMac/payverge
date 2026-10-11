//go:build whatsapp

package services

import (
	"strconv"
	"testing"
	"time"
)

// TestWhatsAppAllowSender_PerSenderSlidingWindow locks in the per-sender rate
// limit that bounds inbound WhatsApp AI calls (cost-abuse / paywall-bypass fix).
func TestWhatsAppAllowSender_PerSenderSlidingWindow(t *testing.T) {
	wm := &WhatsAppManager{}

	sender := "1555000@s.whatsapp.net"
	for i := 0; i < whatsAppRateMax; i++ {
		if !wm.allowSender(sender) {
			t.Fatalf("expected message %d within window to be allowed", i+1)
		}
	}
	if wm.allowSender(sender) {
		t.Fatalf("expected message %d to be rate-limited", whatsAppRateMax+1)
	}

	// A different sender has its own independent budget.
	if !wm.allowSender("1555111@s.whatsapp.net") {
		t.Fatalf("expected a different sender to be allowed")
	}
}

// TestWhatsAppSweepOnce_EvictsIdleSenderKeys is the map-growth access-shape
// guard: allowSender prunes timestamps inside a sender's slice but never
// deletes the map key, so one entry per unique inbound JID would accumulate
// forever. sweepOnce must delete keys whose window has fully drained while
// keeping senders that are still active.
func TestWhatsAppSweepOnce_EvictsIdleSenderKeys(t *testing.T) {
	wm := &WhatsAppManager{}

	idle := "1555000@s.whatsapp.net"
	active := "1555111@s.whatsapp.net"

	// Both senders record one hit.
	if !wm.allowSender(idle) {
		t.Fatalf("expected idle sender first hit to be allowed")
	}
	if !wm.allowSender(active) {
		t.Fatalf("expected active sender first hit to be allowed")
	}

	// Back-date the idle sender's only timestamp so it falls outside the
	// window; leave the active sender's timestamp fresh.
	wm.rateMu.Lock()
	wm.rateHits[idle] = []time.Time{time.Now().Add(-2 * whatsAppRateWindow)}
	wm.rateMu.Unlock()

	wm.sweepOnce()

	wm.rateMu.Lock()
	_, idleStillPresent := wm.rateHits[idle]
	_, activeStillPresent := wm.rateHits[active]
	wm.rateMu.Unlock()

	if idleStillPresent {
		t.Fatalf("expected idle sender key to be evicted after sweep; map still has %q", idle)
	}
	if !activeStillPresent {
		t.Fatalf("expected active sender key to survive sweep")
	}
}

// TestWhatsAppSweepOnce_EmptyMapNoPanic guards the goroutine body launched by
// NewWhatsAppManager: sweepLoop calls sweepOnce on a ticker, and an empty (or
// freshly-initialized) map must be a safe no-op so the long-lived sweeper
// never crashes the process.
func TestWhatsAppSweepOnce_EmptyMapNoPanic(t *testing.T) {
	wm := &WhatsAppManager{rateHits: map[string][]time.Time{}}
	wm.sweepOnce() // must not panic
	if len(wm.rateHits) != 0 {
		t.Fatalf("expected empty map to stay empty, got %d keys", len(wm.rateHits))
	}

	// nil map is also safe (allowSender lazily initializes, sweepLoop may fire first).
	var nilWM WhatsAppManager
	nilWM.sweepOnce() // ranging a nil map is a no-op; must not panic
}

// BenchmarkWhatsAppAllowSenderChurn measures allowSender under high unique-JID
// churn (the leak scenario: every iteration is a brand-new sender). Use to
// record bytes/op and allocs/op before/after the sweep lands.
func BenchmarkWhatsAppAllowSenderChurn(b *testing.B) {
	wm := &WhatsAppManager{rateHits: map[string][]time.Time{}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender := "bench-" + strconv.Itoa(i) + "@s.whatsapp.net"
		wm.allowSender(sender)
	}
}

// BenchmarkWhatsAppSweptChurn measures allowSender + periodic sweepOnce under
// the same unique-JID churn and asserts the live map stays bounded — proving
// the eviction prevents unbounded growth. The sweep cadence (every 1024 iters)
// stands in for the production per-window ticker.
func BenchmarkWhatsAppSweptChurn(b *testing.B) {
	wm := &WhatsAppManager{rateHits: map[string][]time.Time{}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender := "bench-" + strconv.Itoa(i) + "@s.whatsapp.net"
		wm.allowSender(sender)
		if i%1024 == 0 {
			// Back-date everything so the sweep can fully drain idle keys.
			wm.rateMu.Lock()
			for s := range wm.rateHits {
				wm.rateHits[s] = []time.Time{time.Now().Add(-2 * whatsAppRateWindow)}
			}
			wm.rateMu.Unlock()
			wm.sweepOnce()
		}
	}
	b.StopTimer()

	// After the final sweep cycle the live map must be far smaller than b.N:
	// at most the senders added since the last sweep (< 1024), never one-per-iter.
	wm.rateMu.Lock()
	live := len(wm.rateHits)
	wm.rateMu.Unlock()
	if b.N > 2048 && live >= b.N {
		b.Fatalf("rateHits grew unbounded: %d live keys for %d iterations", live, b.N)
	}
}
