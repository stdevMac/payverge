package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(time.Hour, 3)

	for i := 1; i <= 3; i++ {
		if !rl.Allow("user@example.com") {
			t.Errorf("expected request %d to be allowed", i)
		}
	}
	if rl.Allow("user@example.com") {
		t.Error("expected 4th request to be denied")
	}
	if !rl.Allow("other@example.com") {
		t.Error("expected different key to be allowed")
	}
	if rl.Allow("") {
		t.Error("expected empty key to be denied")
	}
}

func TestRateLimiterWindowExpires(t *testing.T) {
	rl := NewRateLimiter(50*time.Millisecond, 1)
	if !rl.Allow("user@example.com") {
		t.Fatal("expected first request to be allowed")
	}
	if rl.Allow("user@example.com") {
		t.Fatal("expected second immediate request to be denied")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.Allow("user@example.com") {
		t.Error("expected request after window to be allowed again")
	}
}

func TestRateLimiterPeekDoesNotConsume(t *testing.T) {
	rl := NewRateLimiter(time.Hour, 1)
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Peek("10.0.0.1"), "Peek must never consume budget")
	}
	rl.Record("10.0.0.1")
	assert.False(t, rl.Peek("10.0.0.1"), "Record consumes; Peek then reports exhausted")
}

func TestRateLimiterBucketsIPv6ToSlash64(t *testing.T) {
	rl := NewRateLimiter(time.Hour, 2)
	assert.True(t, rl.Allow("2001:db8:abcd:12::1"))
	assert.True(t, rl.Allow("2001:db8:abcd:12::2"))
	// Same /64 — a third distinct host address shares the exhausted bucket.
	assert.False(t, rl.Allow("2001:db8:abcd:12:ffff::9"))
	// A different /64 gets its own bucket.
	assert.True(t, rl.Allow("2001:db8:abcd:13::1"))
	// IPv4 and non-IP keys (emails) are untouched by the bucketing.
	assert.True(t, rl.Allow("203.0.113.9"))
	assert.True(t, rl.Allow("someone@example.com"))
}
