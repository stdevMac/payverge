package logger

import (
	"strings"
	"testing"
	"time"
)

func TestSafeGoNamed_reportsPanicToReporter(t *testing.T) {
	reported := make(chan string, 1)
	prev := panicReporter.Load()
	SetPanicReporter(func(name string, r interface{}, stack []byte) { reported <- name })
	t.Cleanup(func() { panicReporter.Store(prev) })

	SafeGoNamed("test-worker", func() { panic("boom") })

	select {
	case got := <-reported:
		if !strings.Contains(got, "test-worker") {
			t.Fatalf("expected reporter to receive worker name, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SafeGoNamed did not report within timeout")
	}
}

func TestSafeGo_reportsBackgroundGoroutineName(t *testing.T) {
	reported := make(chan string, 1)
	prev := panicReporter.Load()
	SetPanicReporter(func(name string, r interface{}, stack []byte) { reported <- name })
	t.Cleanup(func() { panicReporter.Store(prev) })

	SafeGo(func() { panic("boom") })

	select {
	case got := <-reported:
		if got != "background goroutine" {
			t.Fatalf("expected SafeGo to delegate with default name, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SafeGo did not report within timeout")
	}
}

func TestSafeTick_reportsPanicToReporter(t *testing.T) {
	reported := make(chan string, 1)
	prev := panicReporter.Load()
	SetPanicReporter(func(name string, r interface{}, stack []byte) { reported <- name })
	t.Cleanup(func() { panicReporter.Store(prev) })

	SafeTick("tick-worker", func() { panic("nope") })

	select {
	case got := <-reported:
		if !strings.Contains(got, "tick-worker") {
			t.Fatalf("expected reporter to receive tick name, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SafeTick did not report")
	}
}
