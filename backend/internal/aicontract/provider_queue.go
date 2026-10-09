package aicontract

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// ScriptedProvider is a strict ordered fake llm.Provider for hermetic scenarios.
type ScriptedProvider struct {
	mu    sync.Mutex
	steps []ProviderStep
	calls int
	// LastRequests records every GenerateRequest for post-asserts (ZDR etc.).
	LastRequests []llm.GenerateRequest
}

// NewScriptedProvider builds a provider that fails on unexpected calls.
func NewScriptedProvider(steps []ProviderStep) *ScriptedProvider {
	cp := make([]ProviderStep, len(steps))
	copy(cp, steps)
	return &ScriptedProvider{steps: cp}
}

// Generate consumes the next scripted step and validates the request shape.
func (p *ScriptedProvider) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := llm.ApplyFeaturePrivacy(&req); err != nil {
		return nil, err
	}
	if err := req.ValidatePrivacy(); err != nil {
		return nil, err
	}
	p.LastRequests = append(p.LastRequests, req)
	p.calls++

	if len(p.steps) == 0 {
		return nil, fmt.Errorf("aicontract: unexpected provider call #%d feature=%q (script exhausted)", p.calls, req.Feature)
	}
	step := p.steps[0]
	p.steps = p.steps[1:]

	if step.Feature != "" && step.Feature != req.Feature {
		return nil, fmt.Errorf("aicontract: feature want %q got %q", step.Feature, req.Feature)
	}
	if step.PrivacyClass != "" && string(req.PrivacyClass) != step.PrivacyClass {
		return nil, fmt.Errorf("aicontract: privacy_class want %q got %q", step.PrivacyClass, req.PrivacyClass)
	}
	if step.RequireZDR != nil {
		if got := req.RequiresZDR(); got != *step.RequireZDR {
			return nil, fmt.Errorf("aicontract: require_zdr want %v got %v", *step.RequireZDR, got)
		}
	}
	if step.Model != "" && step.Model != req.Model {
		return nil, fmt.Errorf("aicontract: model want %q got %q", step.Model, req.Model)
	}
	if step.HasTools != nil {
		has := len(req.Tools) > 0
		if has != *step.HasTools {
			return nil, fmt.Errorf("aicontract: has_tools want %v got %v", *step.HasTools, has)
		}
	}
	if step.HasSchema != nil {
		has := req.ResponseSchema != nil || req.JSONMode
		if has != *step.HasSchema {
			return nil, fmt.Errorf("aicontract: has_schema want %v got %v", *step.HasSchema, has)
		}
	}
	for _, mark := range step.PromptMarks {
		if !strings.Contains(req.System, mark) && !messagesContain(req.Messages, mark) {
			return nil, fmt.Errorf("aicontract: missing prompt mark %q", mark)
		}
	}

	if step.Error != "" {
		return nil, fmt.Errorf("%s", step.Error)
	}

	resp := &llm.Response{
		Text:  step.ResponseText,
		Model: step.ServedModel,
	}
	if resp.Model == "" {
		resp.Model = req.Model
	}
	for _, tc := range step.ToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			Name: tc.Name,
			Args: tc.Args,
		})
	}
	return resp, nil
}

// Remaining returns unconsumed script steps (must be 0 at end of scenario).
func (p *ScriptedProvider) Remaining() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.steps)
}

// CallCount returns how many Generate calls were made.
func (p *ScriptedProvider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func messagesContain(msgs []llm.Message, mark string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Text, mark) {
			return true
		}
	}
	return false
}
