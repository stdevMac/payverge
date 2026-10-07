package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterPerIPHourlyCap: the same client IP may create at most 5 accounts
// per hour; the 6th attempt gets 429 RATE_LIMITED. This is the durable hourly
// cap behind the per-minute authLimiter middleware — it reuses the exact
// sliding-window RateLimiter the resend-verification flow uses.
func TestRegisterPerIPHourlyCap(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)

	for i := 0; i < 5; i++ {
		w, c := postJSON(t, map[string]string{
			"email":    fmt.Sprintf("burst%d@example.com", i),
			"password": "password123",
			"name":     "Burst",
		})
		h.Register(c)
		require.Equalf(t, http.StatusCreated, w.Code, "registration %d within the cap must succeed", i+1)
	}

	w, c := postJSON(t, map[string]string{
		"email":    "burst-over@example.com",
		"password": "password123",
		"name":     "Burst",
	})
	h.Register(c)

	require.Equal(t, http.StatusTooManyRequests, w.Code, "6th registration from the same IP within the hour must be throttled")
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "RATE_LIMITED", body["code"])
}

func TestRegisterPerIPHourlyCapCanBeRaisedForHarnesses(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)
	h.SetRegistrationRateLimit(6)

	for i := 0; i < 6; i++ {
		w, c := postJSON(t, map[string]string{
			"email":    fmt.Sprintf("raised-burst%d@example.com", i),
			"password": "password123",
			"name":     "Raised Burst",
		})
		h.Register(c)
		require.Equalf(t, http.StatusCreated, w.Code, "registration %d within raised cap must succeed", i+1)
	}
}

// Failed attempts (here: duplicate-email 409s) must not consume the hourly
// creation budget — only successful account creations count (P3).
func TestRegisterHourlyCapIgnoresFailedAttempts(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)

	w0, c0 := postJSON(t, map[string]string{
		"email": "dup@example.com", "password": "password123", "name": "Dup",
	})
	h.Register(c0)
	require.Equal(t, http.StatusCreated, w0.Code)

	for i := 0; i < 10; i++ {
		w, c := postJSON(t, map[string]string{
			"email": "dup@example.com", "password": "password123", "name": "Dup",
		})
		h.Register(c)
		require.Equal(t, http.StatusConflict, w.Code)
	}

	// 1 of 5 consumed — four more fresh signups must still pass.
	for i := 0; i < 4; i++ {
		w, c := postJSON(t, map[string]string{
			"email":    fmt.Sprintf("fresh%d@example.com", i),
			"password": "password123",
			"name":     "Fresh",
		})
		h.Register(c)
		require.Equalf(t, http.StatusCreated, w.Code, "fresh signup %d blocked by failed-attempt quota burn", i)
	}
}

// The hourly cap buckets IPv6 clients per /64: rotating the low 64 bits (one
// subscriber's delegation) must not mint a fresh budget for each address.
func TestRegisterPerIPHourlyCap_BucketsIPv6PerSlash64(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)

	register := func(remoteAddr, email string) int {
		w, c := postJSON(t, map[string]string{"email": email, "password": "password123", "name": "Six"})
		c.Request.RemoteAddr = remoteAddr
		h.Register(c)
		return w.Code
	}

	for i := 0; i < 5; i++ {
		require.Equalf(t, http.StatusCreated, register(fmt.Sprintf("[2001:db8:aa:bb::%x]:443", i+1), fmt.Sprintf("v6-%d@example.com", i)),
			"registration %d within the cap must succeed", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, register("[2001:db8:aa:bb:dead:beef:0:9]:443", "v6-over@example.com"),
		"another address in the same /64 shares the budget")
	assert.Equal(t, http.StatusCreated, register("[2001:db8:aa:cc::1]:443", "v6-other@example.com"),
		"a different /64 has its own budget")
}
