package agents

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// A guest closing the widget cancels the request context; that is not a broken
// bot and must not raise a loop_failure escalation (row + Telegram + email).
func TestLoopFailureSkipsEscalationOnContextCanceled(t *testing.T) {
	var got []EscalationEvent
	provider := &scriptProvider{err: fmt.Errorf("provider: %w", context.Canceled)}

	_, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     newEscRegistry(),
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "help",
		Source:       "concierge",
		OnEscalate:   func(ev EscalationEvent) { got = append(got, ev) },
	})
	if err == nil {
		t.Fatal("expected loop error")
	}
	if len(got) != 0 {
		t.Fatalf("client cancellation must not escalate, got %+v", got)
	}
}

// DeadlineExceeded means the loop blew its own wall-clock budget — that IS a
// real failure and must keep escalating.
func TestLoopFailureStillEscalatesOnDeadlineExceeded(t *testing.T) {
	var got []EscalationEvent
	provider := &scriptProvider{err: fmt.Errorf("provider: %w", context.DeadlineExceeded)}

	_, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     newEscRegistry(),
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "help",
		Source:       "ops",
		OnEscalate:   func(ev EscalationEvent) { got = append(got, ev) },
	})
	if err == nil {
		t.Fatal("expected loop error")
	}
	if len(got) != 1 || got[0].Reason != "loop_failure" {
		t.Fatalf("deadline exceeded must escalate, got %+v", got)
	}
}

// The raw user prompt goes into the escalation row/Telegram/email as the
// transcript; it must pass through pii.Redact first, matching how ops persists
// the same message.
func TestLoopFailureEscalationRedactsUserPrompt(t *testing.T) {
	var got []EscalationEvent
	provider := &scriptProvider{err: fmt.Errorf("upstream 500")}

	_, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     newEscRegistry(),
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "reach me at bob@example.com or call +12345678901 please",
		Source:       "ops",
		OnEscalate:   func(ev EscalationEvent) { got = append(got, ev) },
	})
	if err == nil {
		t.Fatal("expected loop error")
	}
	if len(got) != 1 {
		t.Fatalf("want 1 escalation, got %d", len(got))
	}
	tr := got[0].Transcript
	if strings.Contains(tr, "bob@example.com") || strings.Contains(tr, "+12345678901") {
		t.Fatalf("transcript leaked PII: %q", tr)
	}
	if !strings.Contains(tr, "[redacted-email]") || !strings.Contains(tr, "[redacted-phone]") {
		t.Fatalf("transcript not redacted: %q", tr)
	}
}
