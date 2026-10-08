// Package boundedcache is a concurrency-safe, generic TTL + max-entries LRU
// cache with Get/Set/Invalidate/InvalidatePrefix. It bounds both staleness
// (TTL) and memory (maxEntries with LRU eviction).
package boundedcache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

type entry[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time
}

// Cache is safe for concurrent use. K must be comparable; prefix operations are
// only meaningful when K is a string-like key.
type Cache[K comparable, V any] struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	ll         *list.List          // front = most recently used
	items      map[K]*list.Element // K -> *list.Element holding *entry[K,V]
}

// New returns a cache holding at most maxEntries live items, each valid for ttl.
func New[K comparable, V any](maxEntries int, ttl time.Duration) *Cache[K, V] {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &Cache[K, V]{
		maxEntries: maxEntries,
		ttl:        ttl,
		ll:         list.New(),
		items:      make(map[K]*list.Element),
	}
}

// Get returns the value and true if present and not expired. Expired entries
// are evicted lazily on read.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	e := el.Value.(*entry[K, V])
	if time.Now().After(e.expiresAt) {
		c.removeElement(el)
		var zero V
		return zero, false
	}
	c.ll.MoveToFront(el)
	return e.value, true
}

// Set inserts or replaces key, refreshing its TTL and marking it most-recently
// used, evicting the LRU entry if over capacity.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp := time.Now().Add(c.ttl)
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry[K, V])
		e.value, e.expiresAt = value, exp
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&entry[K, V]{key: key, value: value, expiresAt: exp})
	c.items[key] = el
	for c.ll.Len() > c.maxEntries {
		c.removeElement(c.ll.Back())
	}
}

// Invalidate removes a single key.
func (c *Cache[K, V]) Invalidate(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

// InvalidatePrefix removes every key whose string form starts with prefix.
// Only meaningful for string keys; for non-string K the conversion is a no-op
// match and nothing is removed.
func (c *Cache[K, V]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, el := range c.items {
		if ks, ok := any(k).(string); ok && strings.HasPrefix(ks, prefix) {
			c.removeElement(el)
		}
	}
}

// InvalidateFunc removes every entry whose value matches. It scans the whole
// cache, so it suits rare operator writes, not request paths.
func (c *Cache[K, V]) InvalidateFunc(match func(V) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, el := range c.items {
		if match(el.Value.(*entry[K, V]).value) {
			c.removeElement(el)
		}
	}
}

// SetAt inserts or replaces key with an explicit expiry time. Used by tests
// to seed entries that are already expired (expiresAt in the past). Production
// code should use Set, which derives expiry from the cache TTL.
func (c *Cache[K, V]) SetAt(key K, value V, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry[K, V])
		e.value, e.expiresAt = value, expiresAt
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&entry[K, V]{key: key, value: value, expiresAt: expiresAt})
	c.items[key] = el
	for c.ll.Len() > c.maxEntries {
		c.removeElement(c.ll.Back())
	}
}

// EvictExpired removes all entries whose TTL has elapsed. Call this on writes
// to bound the cache to live keys (reap-on-write pattern).
func (c *Cache[K, V]) EvictExpired() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, el := range c.items {
		if now.After(el.Value.(*entry[K, V]).expiresAt) {
			c.ll.Remove(el)
			delete(c.items, k)
		}
	}
}

// Len reports the number of entries currently held (including not-yet-evicted
// expired ones). Intended for tests/metrics.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

func (c *Cache[K, V]) removeElement(el *list.Element) {
	c.ll.Remove(el)
	delete(c.items, el.Value.(*entry[K, V]).key)
}
