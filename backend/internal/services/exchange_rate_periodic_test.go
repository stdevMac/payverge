package services

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunPeriodicReturnsPromptlyAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var n atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		runPeriodic(ctx, time.Millisecond, func() error {
			n.Add(1)
			return nil
		})
	}()

	deadline := time.Now().Add(time.Second)
	for n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n.Load() == 0 {
		t.Fatal("fetch was not called")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPeriodic did not return within 1s of cancel")
	}
}

func TestRunPeriodicPreCancelledFetchesOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var n atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		runPeriodic(ctx, time.Millisecond, func() error {
			n.Add(1)
			return nil
		})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPeriodic did not return")
	}
	if got := n.Load(); got != 1 {
		t.Fatalf("fetch count = %d, want 1", got)
	}
}
