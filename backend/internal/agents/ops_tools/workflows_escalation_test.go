package ops_tools_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_tools"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// failingProvider always errors so email delivery fails deterministically.
type failingProvider struct{}

func (failingProvider) Send(context.Context, emails.EmailMessage) error {
	return errors.New("smtp down")
}

// passingProvider always succeeds so the tool reports email_sent=true.
type passingProvider struct{}

func (passingProvider) Send(context.Context, emails.EmailMessage) error { return nil }

// withAdminInbox points emails.AdminsEmails at a test inbox for one test:
// there is no default ADMIN_EMAILS inbox any more.
func withAdminInbox(t *testing.T, inbox ...string) {
	t.Helper()
	original := emails.AdminsEmails
	emails.AdminsEmails = inbox
	t.Cleanup(func() { emails.AdminsEmails = original })
}

func passingEmailServer(t *testing.T) *emails.EmailServer {
	t.Helper()
	withAdminInbox(t, "ops@example.org")
	srv, err := emails.NewEmailServer(passingProvider{}, "noreply@test.payverge.io", "updates@test.payverge.io", "../../../email/templates")
	if err != nil {
		t.Fatalf("build email server: %v", err)
	}
	return srv
}

func failingEmailServer(t *testing.T) *emails.EmailServer {
	t.Helper()
	withAdminInbox(t, "ops@example.org")
	srv, err := emails.NewEmailServer(failingProvider{}, "noreply@test.payverge.io", "updates@test.payverge.io", "../../../email/templates")
	if err != nil {
		t.Fatalf("build email server: %v", err)
	}
	return srv
}

// The escalation tool must NOT fail when email is unavailable — a tool error
// blocks OnEscalate, which is exactly the moment the durable row + Telegram
// fallback matter most.
func TestSupportEscalationSucceedsWithoutEmailServer(t *testing.T) {
	tool := &ops_tools.SupportEscalationTool{}
	res, err := tool.Run(context.Background(), map[string]any{"issue": "printer down"}, agents.ToolEnv{
		BusinessID: 7, CanWrite: true, EmailServer: nil,
	})
	if err != nil {
		t.Fatalf("tool must succeed when email is unavailable, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "email notification failed") {
		t.Fatalf("summary should note the email failure, got %q", res.Summary)
	}
	if res.Data["email_sent"] != false {
		t.Fatalf("data should flag email_sent=false, got %#v", res.Data)
	}
}

func TestSupportEscalationSucceedsWhenEmailSendFails(t *testing.T) {
	tool := &ops_tools.SupportEscalationTool{}
	res, err := tool.Run(context.Background(), map[string]any{"issue": "printer down", "transcript_summary": "guest can't print"}, agents.ToolEnv{
		BusinessID: 7, CanWrite: true, EmailServer: failingEmailServer(t),
	})
	if err != nil {
		t.Fatalf("tool must succeed when the email send errors, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "email notification failed") {
		t.Fatalf("summary should note the email failure, got %q", res.Summary)
	}
}

// With ADMIN_EMAILS unset the escalation is still recorded, flagged
// email_sent=false so the Escalator's own fallback (Telegram, durable row)
// takes over — and nothing is mailed to a hard-coded upstream inbox.
func TestSupportEscalationWithoutAdminInboxRecordsEmailNotSent(t *testing.T) {
	srv := passingEmailServer(t)
	withAdminInbox(t) // ADMIN_EMAILS unset
	tool := &ops_tools.SupportEscalationTool{}
	res, err := tool.Run(context.Background(), map[string]any{"issue": "printer down"}, agents.ToolEnv{
		BusinessID: 7, CanWrite: true, EmailServer: srv,
	})
	if err != nil {
		t.Fatalf("tool must succeed without an admin inbox, got: %v", err)
	}
	if res.Data["email_sent"] != false {
		t.Fatalf("data should flag email_sent=false without ADMIN_EMAILS, got %#v", res.Data)
	}
}

// scriptedProvider returns queued responses in order.
type scriptedProvider struct {
	steps []*llm.Response
	i     int
}

func (p *scriptedProvider) Generate(context.Context, llm.GenerateRequest) (*llm.Response, error) {
	if p.i >= len(p.steps) {
		return &llm.Response{Text: `{"answer":"done","steps":[],"actions":[],"follow_ups":[]}`}, nil
	}
	r := p.steps[p.i]
	p.i++
	return r, nil
}

// End-to-end: EmailServer nil → the tool still succeeds → the loop completes
// and OnEscalate fires, so the durable escalation row is persisted.
func TestLoopFiresEscalationWhenEmailDeliveryUnavailable(t *testing.T) {
	reg := agents.NewRegistry()
	reg.Register(&ops_tools.SupportEscalationTool{})

	provider := &scriptedProvider{steps: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "create_support_escalation", Args: map[string]any{"issue": "printer down"}}}},
		{Text: `{"answer":"escalated","steps":[],"actions":[],"follow_ups":[]}`},
	}}

	var got []agents.EscalationEvent
	_, err := agents.RunAgentLoop(context.Background(), agents.LoopConfig{
		Registry:     reg,
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "help",
		Source:       "ops",
		Env:          agents.ToolEnv{BusinessID: 7, ThreadID: 3, CanWrite: true, EmailServer: nil},
		OnEscalate:   func(ev agents.EscalationEvent) { got = append(got, ev) },
	})
	if err != nil {
		t.Fatalf("loop must complete when the escalation email fails, got: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 escalation despite email being down, got %d", len(got))
	}
	if got[0].Reason != "tool" || got[0].ToolName != "create_support_escalation" {
		t.Fatalf("unexpected event: %+v", got[0])
	}
	if got[0].EmailAlreadySent {
		t.Fatalf("email was down, EmailAlreadySent must be false so Escalate is the fallback sender")
	}
}

// When the tool's inline email SUCCEEDS, the escalation event must carry
// EmailAlreadySent=true so the Escalator skips its own email — one incident, one
// email (dedup interleaving: tool-succeeds + OnEscalate).
func TestLoopEscalationFlagsEmailAlreadySentWhenToolEmailSucceeds(t *testing.T) {
	reg := agents.NewRegistry()
	reg.Register(&ops_tools.SupportEscalationTool{})

	provider := &scriptedProvider{steps: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "create_support_escalation", Args: map[string]any{"issue": "printer down"}}}},
		{Text: `{"answer":"escalated","steps":[],"actions":[],"follow_ups":[]}`},
	}}

	var got []agents.EscalationEvent
	_, err := agents.RunAgentLoop(context.Background(), agents.LoopConfig{
		Registry:     reg,
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "help",
		Source:       "ops",
		Env:          agents.ToolEnv{BusinessID: 7, ThreadID: 3, CanWrite: true, EmailServer: passingEmailServer(t)},
		OnEscalate:   func(ev agents.EscalationEvent) { got = append(got, ev) },
	})
	if err != nil {
		t.Fatalf("loop must complete, got: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 escalation, got %d", len(got))
	}
	if !got[0].EmailAlreadySent {
		t.Fatalf("tool email succeeded, EmailAlreadySent must be true so Escalate does not double-email")
	}
}
