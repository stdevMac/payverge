package auth

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConsumeStaffState_ExactlyOnceUnderConcurrency guards against the TOCTOU
// window that existed when GoogleStaffCallback read the state under RLock,
// released, then re-acquired the write lock to delete it. Two concurrent
// callbacks carrying the same (captured) state token could both pass validation
// between the unlock and the delete, replaying the OAuth state. A correct
// consume checks-and-deletes atomically, so exactly one caller may win.
func TestConsumeStaffState_ExactlyOnceUnderConcurrency(t *testing.T) {
	const goroutines = 64
	h := &AuthHandler{
		staffStateStore: map[string]StaffOAuthState{
			"state-token": {ExpiresAt: time.Now().Add(time.Hour), Redirect: "https://payverge.io/staff/login"},
		},
	}

	var wins int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			if data, ok := h.consumeStaffState("state-token"); ok {
				atomic.AddInt64(&wins, 1)
				if data.Redirect != "https://payverge.io/staff/login" {
					t.Errorf("winner got wrong redirect: %q", data.Redirect)
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt64(&wins); got != 1 {
		t.Fatalf("expected exactly 1 successful consume, got %d (OAuth state replay possible)", got)
	}
}

func TestConsumeStaffState_RejectsExpiredAndRemovesIt(t *testing.T) {
	h := &AuthHandler{
		staffStateStore: map[string]StaffOAuthState{
			"expired": {ExpiresAt: time.Now().Add(-time.Minute), Redirect: "x"},
		},
	}

	if _, ok := h.consumeStaffState("expired"); ok {
		t.Fatal("consumeStaffState accepted an expired state")
	}
	h.staffStateMu.RLock()
	_, present := h.staffStateStore["expired"]
	h.staffStateMu.RUnlock()
	if present {
		t.Fatal("expired state was not removed on consume")
	}
}

func TestConsumeStaffState_MissingReturnsFalse(t *testing.T) {
	h := &AuthHandler{staffStateStore: make(map[string]StaffOAuthState)}
	if _, ok := h.consumeStaffState("nope"); ok {
		t.Fatal("consumeStaffState returned ok for a missing state")
	}
}

func TestSanitizeStaffRedirect_rejectsEvilSubdomain(t *testing.T) {
	os.Setenv("PUBLIC_URL", "https://payverge.io")
	defer os.Unsetenv("PUBLIC_URL")

	result := sanitizeStaffRedirect("https://payverge.io.evil.com/steal")
	if result == "https://payverge.io.evil.com/steal" {
		t.Fatal("sanitizeStaffRedirect accepted a spoofed subdomain")
	}
	expected := "https://payverge.io/staff/login"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestSanitizeStaffRedirect_acceptsLegitimate(t *testing.T) {
	os.Setenv("PUBLIC_URL", "https://payverge.io")
	defer os.Unsetenv("PUBLIC_URL")

	result := sanitizeStaffRedirect("https://payverge.io/dashboard")
	if result != "https://payverge.io/dashboard" {
		t.Fatalf("expected legitimate URL to pass, got %q", result)
	}
}

func TestSanitizeStaffRedirect_rejectsJavascriptProtocol(t *testing.T) {
	os.Setenv("PUBLIC_URL", "https://payverge.io")
	defer os.Unsetenv("PUBLIC_URL")

	result := sanitizeStaffRedirect("javascript:alert(1)")
	expected := "https://payverge.io/staff/login"
	if result != expected {
		t.Fatalf("expected fallback %q, got %q", expected, result)
	}
}

func TestSanitizeStaffRedirect_handlesEmpty(t *testing.T) {
	os.Setenv("PUBLIC_URL", "https://payverge.io")
	defer os.Unsetenv("PUBLIC_URL")

	result := sanitizeStaffRedirect("")
	expected := "https://payverge.io/staff/login"
	if result != expected {
		t.Fatalf("expected fallback %q, got %q", expected, result)
	}
}
