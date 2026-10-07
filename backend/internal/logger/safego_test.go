package logger

import (
	"sync"
	"testing"
	"time"
)

func init() {
	// Ensure Logger is initialised so SafeGo's recover path can log.
	if Logger == nil {
		InitLogger()
	}
}

func TestSafeGo_normalFunction(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	executed := false
	SafeGo(func() {
		defer wg.Done()
		executed = true
	})
	wg.Wait()
	if !executed {
		t.Fatal("SafeGo did not execute the function")
	}
}

func TestSafeGo_recoversFromPanic(t *testing.T) {
	done := make(chan struct{})
	SafeGo(func() {
		defer func() { close(done) }()
		panic("test panic")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SafeGo did not recover from panic within timeout")
	}
}
