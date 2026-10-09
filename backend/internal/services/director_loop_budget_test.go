package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/require"
)

type slowProvider struct{ delay time.Duration }

func (s *slowProvider) Generate(ctx context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	select {
	case <-time.After(s.delay):
		return newTextResponse(`{"summary":"late","diagnosis":"","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type directorBudgetKeyProvider struct {
	requests []llm.GenerateRequest
}

func (p *directorBudgetKeyProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	p.requests = append(p.requests, req)
	return newTextResponse(`{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`), nil
}

func TestRunDirectorLoop_UsesToolEnvBusinessIDForEveryGeneration(t *testing.T) {
	p := &directorBudgetKeyProvider{}
	_, err := RunDirectorLoop(context.Background(), LoopConfig{
		Registry: director_tools.NewRegistry(), Provider: p, Model: "test", SystemPrompt: "sys", UserPrompt: "hi",
		Env: director_tools.ToolEnv{BusinessID: 91},
	})
	require.NoError(t, err)
	require.NotEmpty(t, p.requests)
	for i, req := range p.requests {
		require.Equal(t, uint(91), req.BusinessID, "generation %d", i)
	}
}

type scriptedThenSlow struct {
	first  *llm.Response
	slow   *slowProvider
	called bool
}

func (s *scriptedThenSlow) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	if !s.called {
		s.called = true
		return s.first, nil
	}
	return s.slow.Generate(ctx, req)
}

type instantFakeTool struct{ name string }

func (f *instantFakeTool) Name() string               { return f.name }
func (f *instantFakeTool) HumanLabel(_ string) string { return f.name }
func (f *instantFakeTool) Description() string        { return "fake tool that does " + f.name }
func (f *instantFakeTool) Schema() *llm.JSONSchema    { return &llm.JSONSchema{Type: llm.TypeObject} }
func (f *instantFakeTool) Run(_ context.Context, _ map[string]any, _ director_tools.ToolEnv) (director_tools.ToolResult, error) {
	return director_tools.ToolResult{Summary: "tuesday traffic is 18% below the weekly average", Data: map[string]any{}}, nil
}

func TestRunDirectorLoop_PerIterationTimeout(t *testing.T) {
	reg := director_tools.NewRegistry()
	start := time.Now()
	_, err := RunDirectorLoop(context.Background(), LoopConfig{
		Registry:            reg,
		Provider:            &slowProvider{delay: 5 * time.Second},
		Model:               "test",
		SystemPrompt:        "sys",
		UserPrompt:          "hi",
		WallClock:           35 * time.Second,
		PerIterationTimeout: 200 * time.Millisecond,
	})
	require.Error(t, err)
	require.Less(t, time.Since(start), 3*time.Second, "per-iteration timeout must fire well before wall clock")
}

func TestRunDirectorLoop_WallClockReturnsPartialSentinel(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "Timeout Partial Restaurant")
	reg := director_tools.NewRegistry()
	reg.Register(&instantFakeTool{name: "get_business_profile"})

	prov := &scriptedThenSlow{
		first: newFunctionCallResponse("get_business_profile", map[string]any{}),
		slow:  &slowProvider{delay: 5 * time.Second},
	}

	_, err := RunDirectorLoop(context.Background(), LoopConfig{
		Registry:            reg,
		Provider:            prov,
		Model:               "test",
		SystemPrompt:        "sys",
		UserPrompt:          "why are tuesdays slow",
		WallClock:           2 * time.Second,
		PerIterationTimeout: 30 * time.Second,
		Env: director_tools.ToolEnv{
			BusinessID: business.ID,
			ThreadID:   0,
			Locale:     "en",
			DB:         db,
		},
	})
	require.Error(t, err)
	var timedOut *DirectorLoopTimeoutError
	require.True(t, errors.As(err, &timedOut), "expected DirectorLoopTimeoutError, got %T: %v", err, err)
	require.NotEmpty(t, timedOut.CompletedSummaries, "timeout should carry completed tool summaries")
}

func BenchmarkRunDirectorLoop_NoToolFinalization(b *testing.B) {
	final := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	reg := director_tools.NewRegistry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fake := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
		_, err := RunDirectorLoop(context.Background(), LoopConfig{
			Registry: reg, Provider: fake, Model: "test", SystemPrompt: "sys", UserPrompt: "hi",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
