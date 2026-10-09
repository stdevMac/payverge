package server

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

const (
	maxAIWaiterMessagesPerSession = 60
	defaultAIWaiterDailyBudget    = 2000

	// maxPausedAIWaiterMessagesPerSession bounds a conversation a human has
	// taken over. It counts every row (guest, AI and staff), so it is twice
	// the AI cap: a guest whose AI session is full can still keep talking to
	// the staff member who took it over, but one session cannot grow
	// ai_waiter_messages (and takeover alert events) without limit.
	maxPausedAIWaiterMessagesPerSession = 2 * maxAIWaiterMessagesPerSession
)

func aiWaiterDailyBudget() int64 {
	if v := strings.TrimSpace(os.Getenv("AI_WAITER_DAILY_MESSAGE_BUDGET")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultAIWaiterDailyBudget
}

// aiWaiterOverBudget reports whether this conversation or business has exhausted
// its message budget once the not-yet-persisted turn(s) are counted (pending is
// 1 when the request carries a guest message that will be saved on admission,
// so the checks match the historical save-then-count semantics without
// writing first). The per-business daily count is served from an in-memory
// counter seeded once per (business, UTC day) from a DB COUNT — avoids a
// JOIN+COUNT on every guest turn.
func aiWaiterOverBudget(convID, businessID uint, pending int64) bool {
	if database.CountAiWaiterMessages(convID)+pending >= maxAIWaiterMessagesPerSession {
		return true
	}
	return database.CountAiWaiterMessagesTodayCached(businessID, time.Now())+pending >= aiWaiterDailyBudget()
}

const maxSessionsPerIPPerHour = 60

type ipSessionLimiter struct {
	mu     sync.Mutex
	window time.Time
	counts map[string]int
}

var sessionCreateLimiter = &ipSessionLimiter{counts: map[string]int{}}

// allowSessionCreate is a best-effort in-memory per-client cap on AI-waiter
// session creation (RT-3), keyed by middleware.ClientRateLimitKey (IPv4
// address or IPv6 /64). Resets hourly and on restart; not a substitute for
// edge rate limiting. Fails closed at the cap (429), accepting bounded
// overshoot.
func allowSessionCreate(clientKey string) bool {
	sessionCreateLimiter.mu.Lock()
	defer sessionCreateLimiter.mu.Unlock()
	hourStart := time.Now().UTC().Truncate(time.Hour)
	if sessionCreateLimiter.window.Before(hourStart) {
		sessionCreateLimiter.window = hourStart
		sessionCreateLimiter.counts = map[string]int{}
	}
	if sessionCreateLimiter.counts[clientKey] >= maxSessionsPerIPPerHour {
		return false
	}
	sessionCreateLimiter.counts[clientKey]++
	return true
}
