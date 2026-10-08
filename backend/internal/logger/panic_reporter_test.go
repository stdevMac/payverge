package logger

import (
	"sync"
	"testing"
)

// TestSetPanicReporterReceivesRecoveredPanic asserts SafeGoNamed routes a
// recovered panic to the installed reporter (the seam main wires to Sentry).
func TestSetPanicReporterReceivesRecoveredPanic(t *testing.T) {
	var mu sync.Mutex
	var gotName string
	var gotR interface{}
	done := make(chan struct{})

	prev := panicReporter.Load()
	SetPanicReporter(func(name string, r interface{}, _ []byte) {
		mu.Lock()
		gotName, gotR = name, r
		mu.Unlock()
		close(done)
	})
	t.Cleanup(func() { panicReporter.Store(prev) })

	SafeGoNamed("test-worker", func() { panic("boom") })
	<-done

	mu.Lock()
	defer mu.Unlock()
	if gotName != "test-worker" || gotR != "boom" {
		t.Fatalf("reporter got (%q,%v), want (test-worker,boom)", gotName, gotR)
	}
}
