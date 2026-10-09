// Package dailyquota is a bounded, in-memory, per-UTC-day event counter for
// abuse ceilings on anonymous AI surfaces (per-IP, per-device, per-email ...).
//
// It is process-local: counts reset on restart and are not shared between
// replicas, so it is a cheap first line in front of the durable USD ledger
// (internal/llm BudgetStore), never a billing control. Memory is bounded by the
// LRU cap; under a key flood the least-recently-used counters are evicted,
// which can only ever reset a counter to zero (fail-open for that key), never
// block an unrelated one.
package dailyquota

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/boundedcache"
)

// keyTTL outlives one UTC day so a counter cannot expire mid-day; day-stamped
// keys from earlier days age out by TTL or LRU.
const keyTTL = 26 * time.Hour

// Limit is one ceiling an event is counted against.
type Limit struct {
	// Key identifies the subject ("waiter-ip:203.0.113.7"). Empty skips it.
	Key string
	// Max is the number of events allowed per UTC day. <= 0 disables it.
	Max int64
}

// Counter is safe for concurrent use.
type Counter struct {
	mu    sync.Mutex
	cache *boundedcache.Cache[string, int64]
	now   func() time.Time
}

// New returns a counter tracking at most maxKeys (subject, day) pairs.
func New(maxKeys int) *Counter {
	return &Counter{cache: boundedcache.New[string, int64](maxKeys, keyTTL), now: time.Now}
}

func (c *Counter) dayKey(key string) string {
	return c.now().UTC().Format("2006-01-02") + "|" + key
}

// Take records one event against every limit if, and only if, every limit
// still has room (all-or-nothing, so a request refused by one ceiling never
// burns another). It returns false and the first exhausted key otherwise.
func (c *Counter) Take(limits ...Limit) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	type slot struct {
		key  string
		next int64
	}
	slots := make([]slot, 0, len(limits))
	for _, l := range limits {
		if l.Key == "" || l.Max <= 0 {
			continue
		}
		k := c.dayKey(l.Key)
		n, _ := c.cache.Get(k)
		if n >= l.Max {
			return false, l.Key
		}
		slots = append(slots, slot{key: k, next: n + 1})
	}
	for _, s := range slots {
		c.cache.Set(s.key, s.next)
	}
	return true, ""
}

// Count returns today's count for key.
func (c *Counter) Count(key string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, _ := c.cache.Get(c.dayKey(key))
	return n
}

// Reset forgets every counter (tests).
func (c *Counter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.InvalidatePrefix("")
}

// SetClock replaces the time source (tests).
func (c *Counter) SetClock(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// NetworkKey normalizes a client IP into the unit one subscriber controls:
// the address itself for IPv4, the /64 prefix for IPv6 (a single host is
// routinely handed a whole /64, so per-address keys would be free to rotate).
// Unparseable input is returned trimmed; empty stays empty.
func NetworkKey(ip string) string {
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
