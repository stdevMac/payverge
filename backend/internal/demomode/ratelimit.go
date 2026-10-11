package demomode

import (
	"net"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"golang.org/x/time/rate"
)

// writeLimiter is a per-client token bucket for state-changing requests
// while DEMO_MODE is on: DEMO_WRITE_RATE_PER_MIN per minute with the same
// burst. It sits on top of the normal per-route limits, which stay in force.
//
// Memory stays bounded against a visitor rotating source addresses: IPv6
// clients share one bucket per /64 (one host's usual allocation), buckets are
// dropped as soon as they are back to full, and at maxBuckets live buckets a
// new client is refused until older ones refill and are swept.
type writeLimiter struct {
	mu         sync.Mutex
	perMin     int
	buckets    map[string]*bucket
	lastSweep  time.Time
	now        func() time.Time
	maxBuckets int
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// bucketIdle is how long a bucket takes to refill completely (burst perMin at
// perMin per minute). A bucket idle that long is indistinguishable from a new
// one, so dropping it loses no state.
const bucketIdle = time.Minute

// defaultMaxBuckets caps live buckets (~200 B each: about 10 MB).
const defaultMaxBuckets = 50_000

func newWriteLimiter() *writeLimiter {
	return &writeLimiter{buckets: map[string]*bucket{}, now: time.Now, maxBuckets: defaultMaxBuckets}
}

// limiterKey groups IPv6 clients by /64; IPv4 and unparsable values are used
// as is.
func limiterKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() != nil {
		return ip
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func (w *writeLimiter) sweep(now time.Time) {
	for k, b := range w.buckets {
		if now.Sub(b.seen) > bucketIdle {
			delete(w.buckets, k)
		}
	}
	w.lastSweep = now
}

func (w *writeLimiter) allow(ip string) bool {
	perMin := config.DemoWriteRatePerMinute()
	now := w.now()
	key := limiterKey(ip)
	w.mu.Lock()
	defer w.mu.Unlock()
	if perMin != w.perMin {
		// The budget changed (tests): start every bucket over.
		w.perMin = perMin
		w.buckets = map[string]*bucket{}
	}
	if now.Sub(w.lastSweep) > bucketIdle {
		w.sweep(now)
	}
	b, ok := w.buckets[key]
	if !ok {
		if w.maxBuckets > 0 && len(w.buckets) >= w.maxBuckets {
			w.sweep(now)
			if len(w.buckets) >= w.maxBuckets {
				// Fail closed: more distinct clients wrote within one refill
				// window than the cap allows.
				return false
			}
		}
		b = &bucket{lim: rate.NewLimiter(rate.Limit(float64(perMin)/60.0), perMin)}
		w.buckets[key] = b
	}
	b.seen = now
	return b.lim.AllowN(now, 1)
}
