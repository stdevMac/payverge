package server

import (
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// staffLoginPerClientMaxFailures is how many wrong login codes one client
// (IPv4 address or IPv6 /64) may submit for one staff email inside
// database.StaffLoginLockoutWindow. It is deliberately well below
// database.StaffLoginMaxFailedAttempts: a single stranger is stopped here,
// with a 429 that only affects their own client, long before their failures
// could reach the identity-wide lockout that would lock the real staff member
// out. The identity-wide lockout (database.StaffService.ReserveLoginAttempt,
// a 4x higher per-email ceiling) still bounds attackers who rotate addresses.
const staffLoginPerClientMaxFailures = 5

type staffLoginFailureBucket struct {
	start time.Time
	count int
}

// staffLoginClientGuard is an in-memory, per-process (client, email) failure
// counter. Restarts reset it; the identity-wide DB lockout is the durable bound.
type staffLoginClientGuard struct {
	mu      sync.Mutex
	window  time.Duration
	limit   int
	buckets map[string]staffLoginFailureBucket
}

var staffLoginGuard = newStaffLoginClientGuard(database.StaffLoginLockoutWindow, staffLoginPerClientMaxFailures)

// staffLoginCodeSendGuard caps login-code emails at 3 per normalized address
// per hour. reserve is atomic, so concurrent requests cannot exceed the cap.
var staffLoginCodeSendGuard = newStaffLoginClientGuard(time.Hour, 3)

func newStaffLoginClientGuard(window time.Duration, limit int) *staffLoginClientGuard {
	return &staffLoginClientGuard{window: window, limit: limit, buckets: map[string]staffLoginFailureBucket{}}
}

// staffLoginGuardKey normalizes the email (trim + lowercase) like the staff
// lookup does, so case or whitespace variants of one address share a bucket
// and cannot each spend a fresh per-client allowance against the identity-wide
// lockout. VerifyLoginCode already normalizes req.Email; this keeps the key
// safe for any future caller that does not.
func staffLoginGuardKey(clientKey, email string) string {
	return clientKey + "|" + normalizeStaffEmailAddress(email)
}

// reserve atomically claims one verification attempt for the (client, email)
// pair before the code is compared. Check and increment happen under one lock,
// so concurrent requests from one client cannot exceed the limit. Returns
// false once the pair has used up its attempts in the current window.
func (g *staffLoginClientGuard) reserve(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if len(g.buckets) > 10000 {
		for k, b := range g.buckets {
			if now.Sub(b.start) >= g.window {
				delete(g.buckets, k)
			}
		}
	}
	b, ok := g.buckets[key]
	if !ok || now.Sub(b.start) >= g.window {
		b = staffLoginFailureBucket{start: now}
	}
	if b.count >= g.limit {
		return false
	}
	b.count++
	g.buckets[key] = b
	return true
}

// reset clears the pair after a successful login.
func (g *staffLoginClientGuard) reset(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.buckets, key)
}
