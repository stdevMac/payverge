package logger

import "testing"

func init() {
	if Logger == nil {
		InitLogger()
	}
}

func TestSafeTick_runsFunction(t *testing.T) {
	ran := false
	SafeTick("test-loop", func() { ran = true })
	if !ran {
		t.Fatal("SafeTick did not execute the function")
	}
}

func TestSafeTick_recoversAndReturns(t *testing.T) {
	// A panicking tick must NOT propagate (which would unwind a worker loop
	// and crash the whole process). SafeTick must recover, log, and return
	// normally so the caller's for-loop survives to the next iteration.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SafeTick let the panic propagate: %v", r)
		}
	}()
	SafeTick("test-loop", func() { panic("boom") })
	// Reaching here means the panic was contained.
}

func TestSafeTick_loopSurvivesPanickingIteration(t *testing.T) {
	// Models a worker loop: a panic on one iteration must not stop later ones.
	iterations := 0
	for i := 0; i < 3; i++ {
		SafeTick("test-loop", func() {
			iterations++
			if i == 1 {
				panic("transient bad tick")
			}
		})
	}
	if iterations != 3 {
		t.Fatalf("expected 3 iterations to run despite panic on #2, got %d", iterations)
	}
}
