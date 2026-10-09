package agents

// Test doubles shared by the ops-assistant tests.

import (
	"context"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

type assistantCapturingProvider struct {
	requests []llm.GenerateRequest
	answer   string
}

func (p *assistantCapturingProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	p.requests = append(p.requests, req)
	answer := p.answer
	if answer == "" {
		answer = `{"answer":"Claro, podés usar el formulario de contacto.","steps":[],"actions":[],"follow_ups":[]}`
	}
	return &llm.Response{Text: answer}, nil
}
