package database

import (
	"fmt"
	"sync"
	"time"
)

// dailyAiWaiterCounters caches per-(business, UTC-day) message counts so the
// per-guest-turn budget check avoids a JOIN+COUNT over the business's whole day
// of ai_waiter_messages on every message. Keyed "businessID:YYYY-MM-DD"; an
// entry is seeded once via a DB COUNT then incremented in-memory. Stale prior-day
// entries are dropped lazily when a new day's key is first seeded.
var (
	dailyAiWaiterMu       sync.Mutex
	dailyAiWaiterCounters = make(map[string]int64)
)

func init() {
	// When tests call SetTestDB the in-memory counter is stale against the fresh
	// database — reset it so budget checks reseed from the new DB.
	RegisterOnDBChange(resetDailyAiWaiterCounterForTest)
}

func dailyAiWaiterKey(businessID uint, day time.Time) string {
	return fmt.Sprintf("%d:%s", businessID, day.UTC().Format("2006-01-02"))
}

// dailyAiWaiterCount returns today's count, seeding from `seed` (a DB COUNT) the
// first time a (business, day) key is seen. seed is called at most once per key.
func dailyAiWaiterCount(businessID uint, day time.Time, seed func(uint) int64) int64 {
	key := dailyAiWaiterKey(businessID, day)
	dailyAiWaiterMu.Lock()
	defer dailyAiWaiterMu.Unlock()
	if n, ok := dailyAiWaiterCounters[key]; ok {
		return n
	}
	// new day for this business: drop any stale keys for it to bound the map.
	prefix := fmt.Sprintf("%d:", businessID)
	for k := range dailyAiWaiterCounters {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(dailyAiWaiterCounters, k)
		}
	}
	n := seed(businessID)
	dailyAiWaiterCounters[key] = n
	return n
}

// IncrementDailyAiWaiterCount bumps the in-memory counter after a message is
// persisted. A no-op if the day was never seeded (next read will seed from DB).
func IncrementDailyAiWaiterCount(businessID uint, day time.Time) {
	key := dailyAiWaiterKey(businessID, day)
	dailyAiWaiterMu.Lock()
	if _, ok := dailyAiWaiterCounters[key]; ok {
		dailyAiWaiterCounters[key]++
	}
	dailyAiWaiterMu.Unlock()
}

// CountAiWaiterMessagesTodayCached returns the business's UTC-day message count,
// seeded once from CountAiWaiterMessagesSince and served from memory thereafter.
//
// Semantics note: the seed counts ALL ai_waiter_messages rows for the day, while
// IncrementDailyAiWaiterCount fires only on user turns. So after the first read
// the counter tracks (seed-snapshot + later user turns) and does not add later
// assistant rows. This is intentional and acceptable for an abuse/cost ceiling
// (the user turn is the cost driver) and keeps the gate close to the pre-cache
// all-rows count without a per-turn JOIN+COUNT.
func CountAiWaiterMessagesTodayCached(businessID uint, now time.Time) int64 {
	day := now.UTC().Truncate(24 * time.Hour)
	return dailyAiWaiterCount(businessID, day, func(id uint) int64 {
		return CountAiWaiterMessagesSince(id, day)
	})
}

func resetDailyAiWaiterCounterForTest() {
	dailyAiWaiterMu.Lock()
	dailyAiWaiterCounters = make(map[string]int64)
	dailyAiWaiterMu.Unlock()
}
