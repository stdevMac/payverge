package main

import (
	"os"
	"strings"
	"testing"
)

// TestShiftCreateRouteWiresIdempotencyMiddleware is the L5-26 / decision #10
// source gate: POST /businesses/:id/shifts must go through middleware.Idempotency
// so a lost-response retry with the same Idempotency-Key replays the cached 201
// instead of double-creating the shift. Mirrors the alt-payment wiring style.
func TestShiftCreateRouteWiresIdempotencyMiddleware(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	sourceText := string(source)

	const routeStart = `protectedRoutes.POST("/businesses/:id/shifts"`
	start := strings.Index(sourceText, routeStart)
	if start == -1 {
		t.Fatalf("shift create route is missing: %s", routeStart)
	}
	// Bound the block to the next route registration so we only inspect this line's args.
	end := strings.Index(sourceText[start:], "\n")
	if end == -1 {
		end = len(sourceText) - start
	}
	block := sourceText[start : start+end]

	const wantEndpoint = `middleware.Idempotency(database.GetDB(), "POST /businesses/:id/shifts")`
	if !strings.Contains(block, wantEndpoint) {
		t.Fatalf("shift create route must wire %s; got: %s", wantEndpoint, block)
	}
	if !strings.Contains(block, "scheduleHandler.CreateShift") {
		t.Fatalf("shift create route must still call scheduleHandler.CreateShift; got: %s", block)
	}
}
