package llm

import (
	"math"
	"testing"
)

const floatEps = 1e-9

func TestRunningTotalUSDDoesNotReset(t *testing.T) {
	r := NewDailyCostRollup()
	r.Add(7, "waiter", 0.10)
	r.Add(7, "director", 0.05)
	r.Add(9, "waiter", 1.00)

	if got := r.RunningTotalUSD(7); math.Abs(got-0.15) > floatEps {
		t.Fatalf("biz 7 running total = %v, want 0.15", got)
	}
	if got := r.RunningTotalUSD(9); math.Abs(got-1.00) > floatEps {
		t.Fatalf("biz 9 running total = %v, want 1.00", got)
	}
	// Reading must NOT reset — a second read returns the same value.
	if got := r.RunningTotalUSD(7); math.Abs(got-0.15) > floatEps {
		t.Fatalf("biz 7 running total after re-read = %v, want 0.15 (read must not reset)", got)
	}
	if got := r.RunningTotalUSD(404); got != 0 {
		t.Fatalf("unknown biz running total = %v, want 0", got)
	}
}
