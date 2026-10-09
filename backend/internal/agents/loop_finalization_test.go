package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

type finalizationProvider struct {
	responses []*llm.Response
	errors    []error
	requests  []llm.GenerateRequest
}

func (p *finalizationProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	idx := len(p.requests)
	p.requests = append(p.requests, req)
	var response *llm.Response
	if idx < len(p.responses) {
		response = p.responses[idx]
	}
	var err error
	if idx < len(p.errors) {
		err = p.errors[idx]
	}
	return response, err
}

type finalizationTool struct {
	name   string
	result ToolResult
	err    error
	run    func(context.Context) (ToolResult, error)
}

func (t finalizationTool) Name() string             { return t.name }
func (t finalizationTool) HumanLabel(string) string { return t.name }
func (t finalizationTool) Description() string      { return "finalization test tool" }
func (t finalizationTool) Schema() *llm.JSONSchema  { return &llm.JSONSchema{Type: llm.TypeObject} }

func (t finalizationTool) Run(ctx context.Context, _ map[string]any, _ ToolEnv) (ToolResult, error) {
	if t.run != nil {
		return t.run(ctx)
	}
	return t.result, t.err
}

func runFinalizationLoop(t *testing.T, provider llm.Provider, tools ...finalizationTool) (*LoopResult, error) {
	return runFinalizationLoopContext(t, context.Background(), provider, tools...)
}

func runFinalizationLoopContext(t *testing.T, ctx context.Context, provider llm.Provider, tools ...finalizationTool) (*LoopResult, error) {
	t.Helper()
	registry := NewRegistry()
	for _, tool := range tools {
		registry.Register(tool)
	}
	return RunAgentLoop(ctx, LoopConfig{
		Registry:     registry,
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "Show me a demo",
	})
}

func TestLoopToolEvidenceSurvivesLaterProviderError(t *testing.T) {
	upstreamErr := errors.New("upstream unavailable")
	provider := &finalizationProvider{
		responses: []*llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
			nil,
		},
		errors: []error{nil, upstreamErr},
	}
	registry := NewRegistry()
	registry.Register(finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})
	sink := &bufferSink{}
	var escalations []EscalationEvent

	result, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     registry,
		Provider:     provider,
		Sink:         sink,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "Show me a demo",
		Source:       "concierge",
		OnEscalate:   func(event EscalationEvent) { escalations = append(escalations, event) },
	})

	require.ErrorIs(t, err, upstreamErr)
	require.NotNil(t, result)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.False(t, result.StructuredRepairUsed)
	require.Equal(t, "error", sink.events[len(sink.events)-1].Type)
	require.Len(t, escalations, 1)
	require.Equal(t, "loop_failure", escalations[0].Reason)
}

func TestLoopToolEvidenceCountsResponseUsageOnLaterProviderError(t *testing.T) {
	upstreamErr := errors.New("upstream unavailable")
	provider := &finalizationProvider{
		responses: []*llm.Response{
			{
				ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}},
				Usage:     llm.Usage{TotalTokens: 2},
			},
			{Text: "partial response", Usage: llm.Usage{TotalTokens: 5}},
		},
		errors: []error{nil, upstreamErr},
	}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})

	require.ErrorIs(t, err, upstreamErr)
	require.NotNil(t, result)
	require.Equal(t, 7, result.TotalTokens)
}

func TestLoopToolEvidenceSurvivesSingleMaxIteration(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
	}}
	registry := NewRegistry()
	registry.Register(finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})
	var escalations []EscalationEvent
	result, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:      registry,
		Provider:      provider,
		Model:         "test",
		SystemPrompt:  "sys",
		UserPrompt:    "Show me a demo",
		Source:        "concierge",
		MaxIterations: 1,
		OnEscalate:    func(event EscalationEvent) { escalations = append(escalations, event) },
	})

	require.EqualError(t, err, "agent loop: max iterations exceeded")
	require.NotNil(t, result)
	require.Len(t, result.ToolEvidence, 1)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.False(t, result.StructuredRepairUsed)
	require.Len(t, escalations, 1)
	require.Equal(t, "loop_failure", escalations[0].Reason)
}

func TestLoopToolEvidencePreservesOrderAcrossRepeatedMaxIterations(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
		{ToolCalls: []llm.ToolCall{{ID: "pricing-1", Name: "get_pricing_overview"}}},
	}}
	registry := NewRegistry()
	registry.Register(finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})
	registry.Register(finalizationTool{
		name: "get_pricing_overview",
		result: ToolResult{
			Summary: "Pricing ready",
			Data:    map[string]any{"href": "/pricing"},
		},
	})
	result, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:      registry,
		Provider:      provider,
		Model:         "test",
		SystemPrompt:  "sys",
		UserPrompt:    "Show me pricing and a demo",
		MaxIterations: 2,
	})

	require.EqualError(t, err, "agent loop: max iterations exceeded")
	require.NotNil(t, result)
	require.Len(t, result.ToolEvidence, 2)
	require.Equal(t, []string{"get_demo_link", "get_pricing_overview"}, []string{
		result.ToolEvidence[0].Name,
		result.ToolEvidence[1].Name,
	})
	require.False(t, result.StructuredRepairUsed)
}

func TestLoopToolEvidenceSurvivesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
	}}
	result, err := runFinalizationLoopContext(t, ctx, provider, finalizationTool{
		name: "get_demo_link",
		run: func(context.Context) (ToolResult, error) {
			cancel()
			return ToolResult{
				Summary: "Demo link ready",
				Data:    map[string]any{"href": "/book-demo"},
			}, nil
		},
	})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.False(t, result.StructuredRepairUsed)
}

func TestLoopPlainTextAfterToolUsesStructuredRepair(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
		{Text: "Book a demo when ready."},
		{Text: `{"answer":"Book a demo when ready.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})

	require.NoError(t, err)
	require.Len(t, provider.requests, 3)
	require.NotNil(t, provider.requests[2].ResponseSchema)
	require.Equal(t, "Book a demo when ready.", result.Final.Answer)
	require.Len(t, result.ToolEvidence, 1)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.Equal(t, "Demo link ready", result.ToolEvidence[0].Summary)
	require.Equal(t, "/book-demo", result.ToolEvidence[0].Data["href"])
	require.True(t, result.StructuredRepairUsed)
}

func TestLoopToolEvidenceIncludesSuccessfulResultsOnly(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{
			{ID: "demo-1", Name: "get_demo_link"},
			{ID: "pricing-1", Name: "get_pricing_overview"},
		}},
		{Text: `{"answer":"Here are the details.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	result, err := runFinalizationLoop(t, provider,
		finalizationTool{
			name: "get_demo_link",
			result: ToolResult{
				Summary: "Demo link ready",
				Data:    map[string]any{"href": "/book-demo"},
			},
		},
		finalizationTool{
			name:   "get_pricing_overview",
			result: ToolResult{Summary: "stale data", Data: map[string]any{"href": "/pricing"}},
			err:    errors.New("catalog unavailable"),
		},
	)

	require.NoError(t, err)
	require.Equal(t, []ToolEvidence{{
		Name:    "get_demo_link",
		Summary: "Demo link ready",
		Data:    map[string]any{"href": "/book-demo"},
	}}, result.ToolEvidence)
	require.False(t, result.StructuredRepairUsed)
}

func TestLoopToolEvidenceDeepSnapshotsToolData(t *testing.T) {
	labels := []string{"demo", "sales"}
	destinations := []map[string]any{{"href": "/book-demo"}}
	metadata := map[string]any{
		"labels":       labels,
		"destinations": destinations,
	}
	data := map[string]any{
		"href":     "/book-demo",
		"metadata": metadata,
	}
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
		{Text: `{"answer":"The demo link is ready.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    data,
		},
	})
	require.NoError(t, err)

	data["href"] = "/changed"
	metadata["new"] = true
	labels[0] = "changed"
	destinations[0]["href"] = "/changed"

	evidenceData := result.ToolEvidence[0].Data
	require.Equal(t, "/book-demo", evidenceData["href"])
	evidenceMetadata := evidenceData["metadata"].(map[string]any)
	require.NotContains(t, evidenceMetadata, "new")
	require.Equal(t, []string{"demo", "sales"}, evidenceMetadata["labels"])
	require.Equal(t, "/book-demo", evidenceMetadata["destinations"].([]map[string]any)[0]["href"])
}

func TestLoopToolEvidenceSnapshotsNilData(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
		{Text: `{"answer":"The demo link is ready.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name:   "get_demo_link",
		result: ToolResult{Summary: "Demo link ready"},
	})

	require.NoError(t, err)
	require.Nil(t, result.ToolEvidence[0].Data)
}

func TestLoopUnknownToolCallRequiresStructuredTerminalResponse(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "unknown-1", Name: "not_registered"}}},
		{Text: "I could not use that tool."},
		{Text: `{"answer":"I could not use that tool.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	result, err := runFinalizationLoop(t, provider)

	require.NoError(t, err)
	require.Len(t, provider.requests, 3)
	require.NotNil(t, provider.requests[2].ResponseSchema)
	require.Empty(t, result.ToolEvidence)
	require.True(t, result.StructuredRepairUsed)
}

func TestLoopMixedKnownAndUnknownToolCallsRequireStructuredTerminalResponse(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{
			{ID: "unknown-1", Name: "not_registered"},
			{ID: "demo-1", Name: "get_demo_link"},
		}},
		{Text: "The demo link is ready."},
		{Text: `{"answer":"The demo link is ready.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})

	require.NoError(t, err)
	require.Len(t, provider.requests, 3)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.True(t, result.StructuredRepairUsed)
}

func TestLoopToolEvidenceReturnsStructuredRepairProviderError(t *testing.T) {
	repairErr := errors.New("structured repair unavailable")
	provider := &finalizationProvider{
		responses: []*llm.Response{
			{
				ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}},
				Usage:     llm.Usage{TotalTokens: 2},
			},
			{Text: "Book a demo when ready.", Usage: llm.Usage{TotalTokens: 3}},
			{Text: "partial repair", Usage: llm.Usage{TotalTokens: 5}},
		},
		errors: []error{nil, nil, repairErr},
	}
	registry := NewRegistry()
	registry.Register(finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})
	sink := &bufferSink{}
	var escalations []EscalationEvent
	result, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     registry,
		Provider:     provider,
		Sink:         sink,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "Show me a demo",
		Source:       "concierge",
		OnEscalate:   func(event EscalationEvent) { escalations = append(escalations, event) },
	})

	require.ErrorIs(t, err, repairErr)
	require.NotNil(t, result)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.True(t, result.StructuredRepairUsed)
	require.Equal(t, 10, result.TotalTokens)
	require.Len(t, sink.events, 3)
	require.Equal(t, "error", sink.events[2].Type)
	require.Len(t, escalations, 1)
	require.Equal(t, "loop_failure", escalations[0].Reason)
}

func TestLoopToolEvidenceReturnsStructuredRepairCancellation(t *testing.T) {
	provider := &finalizationProvider{
		responses: []*llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
			{Text: "Book a demo when ready."},
			nil,
		},
		errors: []error{nil, nil, context.Canceled},
	}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.True(t, result.StructuredRepairUsed)
}

func TestLoopToolEvidenceSurvivesEmptyStructuredRepairResponse(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
		{Text: "Book a demo when ready."},
		{Text: "   "},
	}}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})

	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, provider.requests, 3)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.True(t, result.StructuredRepairUsed)
}

func TestLoopToolEvidenceSurvivesMalformedStructuredRepairResponse(t *testing.T) {
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "demo-1", Name: "get_demo_link"}}},
		{Text: "Book a demo when ready."},
		{Text: "still not structured JSON"},
	}}
	result, err := runFinalizationLoop(t, provider, finalizationTool{
		name: "get_demo_link",
		result: ToolResult{
			Summary: "Demo link ready",
			Data:    map[string]any{"href": "/book-demo"},
		},
	})

	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, "get_demo_link", result.ToolEvidence[0].Name)
	require.Equal(t, "/book-demo", result.ToolEvidence[0].Data["href"])
	require.True(t, result.StructuredRepairUsed)
}
