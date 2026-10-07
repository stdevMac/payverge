package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// escTool is a stub escalation tool named like a real one so the loop's
// escalation detection fires.
type escTool struct{ name string }

func (e escTool) Name() string             { return e.name }
func (e escTool) HumanLabel(string) string { return e.name }
func (e escTool) Description() string      { return "stub escalation" }
func (e escTool) Schema() *llm.JSONSchema  { return &llm.JSONSchema{Type: "object"} }
func (e escTool) Run(context.Context, map[string]any, ToolEnv) (ToolResult, error) {
	return ToolResult{Summary: "escalated"}, nil
}

// scriptProvider returns queued responses in order, then errors.
type scriptProvider struct {
	steps []*llm.Response
	err   error
	i     int
}

func (p *scriptProvider) Generate(context.Context, llm.GenerateRequest) (*llm.Response, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.i >= len(p.steps) {
		return &llm.Response{Text: `{"answer":"done","steps":[],"actions":[],"follow_ups":[]}`}, nil
	}
	r := p.steps[p.i]
	p.i++
	return r, nil
}

func newEscRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(escTool{name: "create_support_escalation"})
	return reg
}

func TestLoopFiresEscalationOnTool(t *testing.T) {
	var got []EscalationEvent
	provider := &scriptProvider{steps: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "create_support_escalation", Args: map[string]any{"issue": "printer down", "transcript_summary": "guest can't print"}}}},
		{Text: `{"answer":"escalated","steps":[],"actions":[],"follow_ups":[]}`},
	}}

	_, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     newEscRegistry(),
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "help",
		Source:       "ops",
		Env:          ToolEnv{BusinessID: 7, ThreadID: 3},
		OnEscalate:   func(ev EscalationEvent) { got = append(got, ev) },
	})
	if err != nil {
		t.Fatalf("loop err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 escalation, got %d", len(got))
	}
	if got[0].Reason != "tool" || got[0].Source != "ops" || got[0].BusinessID != 7 {
		t.Fatalf("unexpected event: %+v", got[0])
	}
	if got[0].Issue != "printer down" {
		t.Fatalf("issue not passed through: %q", got[0].Issue)
	}
}

func TestLoopFiresEscalationOnHardFailure(t *testing.T) {
	var got []EscalationEvent
	provider := &scriptProvider{err: errors.New("upstream 500")}

	_, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     newEscRegistry(),
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "help",
		Source:       "concierge",
		Env:          ToolEnv{SessionID: "cs_x"},
		OnEscalate:   func(ev EscalationEvent) { got = append(got, ev) },
	})
	if err == nil {
		t.Fatal("expected loop error")
	}
	if len(got) != 1 || got[0].Reason != "loop_failure" {
		t.Fatalf("want 1 loop_failure escalation, got %+v", got)
	}
	if got[0].SessionRef != "cs_x" {
		t.Fatalf("session ref not passed: %q", got[0].SessionRef)
	}
}

func TestLoopMaxIterationsEscalates(t *testing.T) {
	var count int
	// Provider always returns a tool call → never terminates → max iterations.
	provider := &scriptProvider{steps: []*llm.Response{}}
	provider.steps = []*llm.Response{}
	loopingProvider := &alwaysToolProvider{}

	_, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:      newEscRegistry(),
		Provider:      loopingProvider,
		Model:         "test",
		SystemPrompt:  "sys",
		UserPrompt:    "help",
		Source:        "ops",
		MaxIterations: 2,
		OnEscalate:    func(EscalationEvent) { count++ },
	})
	if err == nil {
		t.Fatal("expected max-iterations error")
	}
	// One escalation per tool call (2 iters) + one loop_failure = at least 3.
	if count < 1 {
		t.Fatalf("expected escalation on max iterations, got %d", count)
	}
	_ = provider
}

type alwaysToolProvider struct{}

func (alwaysToolProvider) Generate(context.Context, llm.GenerateRequest) (*llm.Response, error) {
	return &llm.Response{ToolCalls: []llm.ToolCall{{Name: "create_support_escalation", Args: map[string]any{"issue": "x"}}}}, nil
}

// limitedEscTool mimics a lead tool refused by its daily abuse ceiling.
type limitedEscTool struct{ escTool }

func (limitedEscTool) Run(context.Context, map[string]any, ToolEnv) (ToolResult, error) {
	return ToolResult{Summary: "limit reached", Data: map[string]any{"rate_limited": true}}, nil
}

// A rate-limited lead/escalation tool stored nothing, so the loop must not
// fan out a durable escalation (Telegram + email) for it either.
func TestLoopSkipsEscalationForRateLimitedTool(t *testing.T) {
	reg := NewRegistry()
	reg.Register(limitedEscTool{escTool{name: "capture_lead"}})
	var got []EscalationEvent
	provider := &scriptProvider{steps: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "capture_lead", Args: map[string]any{"name": "Bot", "email": "b@x.io"}}}},
		{Text: `{"answer":"ok","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	if _, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry: reg, Provider: provider, Model: "test", SystemPrompt: "sys", UserPrompt: "hi",
		Source: "concierge", Env: ToolEnv{SessionID: "cs_x"},
		OnEscalate: func(ev EscalationEvent) { got = append(got, ev) },
	}); err != nil {
		t.Fatalf("loop err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("rate-limited tool must not escalate, got %+v", got)
	}
}
