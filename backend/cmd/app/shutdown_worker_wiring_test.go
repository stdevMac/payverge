package main

import (
	"os"
	"strings"
	"testing"
)

// TestShutdownStopsBackgroundWorkers asserts graceful shutdown drains the
// workers that own goroutines with in-flight DB or object-store work, rather
// than only cancelling their context and racing the process exit.
func TestShutdownStopsBackgroundWorkers(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	shutdown := strings.Index(body, "signal.Notify(quit")
	if shutdown < 0 {
		t.Fatal("main.go has no signal.Notify(quit, ...) shutdown block")
	}
	tail := body[shutdown:]
	for _, call := range []string{
		"spaceScanWorker.Stop()",
		"menuExtractionWorker.Stop()",
	} {
		if !strings.Contains(tail, call) {
			t.Errorf("graceful shutdown does not call %s", call)
		}
	}
	for _, discarded := range []string{
		"_ = services.StartSpaceScanWorker(",
		"menuExtractionWorker := services.NewMenuExtractionWorker(",
	} {
		if strings.Contains(body, discarded) {
			t.Errorf("main.go discards or shadows the worker handle: %s", discarded)
		}
	}
}
