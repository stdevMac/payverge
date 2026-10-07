package server

// guest_analytics.go — P2-19: guest-surface funnel instrumentation.
//
// GetTableByCodePublic is a HOT public route (CLAUDE.md § Backend Performance
// Gate). The rules here:
//   - ZERO extra SQL on the request path (in-memory throttle only),
//   - zero synchronous I/O (emit runs under logger.SafeGo),
//   - O(1) throttle check under one mutex, no allocation on the deny path.
//
// Throttle: one guest_table_scanned per table per 15 minutes, per replica.
// The activation question is "is this QR scanned at all / roughly how often",
// not per-pageview telemetry — guests reload /t/{code} constantly while
// seated. Per-replica state means multi-replica deployments mildly over-count;
// that is an accepted trade for keeping shared state off a public hot path.

import (
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// guestAnalyticsTrackHook is metrics.TrackEvent behind a package var so tests
// can intercept guest funnel events without a live PostHog client.
var guestAnalyticsTrackHook = metrics.TrackEvent

const guestScanThrottleTTL = 15 * time.Minute

// guestScanThrottleMaxEntries bounds throttle memory. When exceeded, expired
// entries are pruned; if still over (pathological churn) the map resets —
// worst case is a few duplicate analytics events, never unbounded memory.
const guestScanThrottleMaxEntries = 4096

type scanThrottle struct {
	mu   sync.Mutex
	seen map[uint]time.Time
}

var guestScanThrottle = &scanThrottle{seen: make(map[uint]time.Time)}

// Allow reports whether a scan event for tableID should be emitted now and
// records the emission when allowed.
func (st *scanThrottle) Allow(tableID uint, now time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if last, ok := st.seen[tableID]; ok && now.Sub(last) < guestScanThrottleTTL {
		return false
	}
	if len(st.seen) >= guestScanThrottleMaxEntries {
		for id, ts := range st.seen {
			if now.Sub(ts) >= guestScanThrottleTTL {
				delete(st.seen, id)
			}
		}
		if len(st.seen) >= guestScanThrottleMaxEntries {
			st.seen = make(map[uint]time.Time)
		}
	}
	st.seen[tableID] = now
	return true
}

// resetGuestScanThrottleForTest clears throttle state between tests.
func resetGuestScanThrottleForTest() {
	guestScanThrottle.mu.Lock()
	defer guestScanThrottle.mu.Unlock()
	guestScanThrottle.seen = make(map[uint]time.Time)
}

// emitGuestTableScanned fires the throttled guest_table_scanned product event.
// The business-scoped identifier contains no owner or guest identity.
func emitGuestTableScanned(businessIDString string, tableID uint) {
	if businessIDString == "" {
		return
	}
	if !guestScanThrottle.Allow(tableID, time.Now()) {
		return
	}
	logger.SafeGo(func() {
		_ = guestAnalyticsTrackHook(businessIDString, "guest_table_scanned", map[string]interface{}{
			"source": "qr",
		})
	})
}

// emitGuestOrderPlaced fires the guest_order_placed funnel event. Unthrottled:
// order creation is rare relative to scans and is the terminal guest-funnel
// signal. Async + zero extra SQL, like the scan emit.
func emitGuestOrderPlaced(businessIDString string, itemCount int) {
	if businessIDString == "" {
		return
	}
	logger.SafeGo(func() {
		_ = guestAnalyticsTrackHook(businessIDString, "guest_order_placed", map[string]interface{}{
			"item_count": itemCount,
		})
	})
}
