package database

import (
	"testing"
	"time"
)

func TestDailyAiWaiterCounterSeedsOnceThenIncrements(t *testing.T) {
	resetDailyAiWaiterCounterForTest()
	day := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	seeds := 0
	seed := func(businessID uint) int64 { seeds++; return 3 } // pretend 3 already today

	if got := dailyAiWaiterCount(7, day, seed); got != 3 {
		t.Fatalf("seed: got %d", got)
	}
	IncrementDailyAiWaiterCount(7, day)
	if got := dailyAiWaiterCount(7, day, seed); got != 4 {
		t.Fatalf("after increment: got %d", got)
	}
	if seeds != 1 {
		t.Fatalf("expected exactly one DB seed, got %d", seeds)
	}
	// next UTC day reseeds
	if got := dailyAiWaiterCount(7, day.AddDate(0, 0, 1), seed); got != 3 || seeds != 2 {
		t.Fatalf("next day reseed: got=%d seeds=%d", got, seeds)
	}
}
