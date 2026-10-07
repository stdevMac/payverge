package services

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeProvider returns scripted responses in order. Each call to Generate pops
// the next scripted response. Used by TestRunDirectorLoop_* to drive the loop
// deterministically without a real LLM provider.
type fakeProvider struct {
	scripted []*llm.Response
	calls    int
}

func (f *fakeProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	if f.calls >= len(f.scripted) {
		return nil, fmt.Errorf("fakeProvider: no more scripted responses (call #%d)", f.calls+1)
	}
	resp := f.scripted[f.calls]
	f.calls++
	return resp, nil
}

func newFunctionCallResponse(name string, args map[string]any) *llm.Response {
	return &llm.Response{ToolCalls: []llm.ToolCall{{ID: "call_" + name, Name: name, Args: args}}}
}

func newTextResponse(text string) *llm.Response {
	return &llm.Response{Text: text}
}

func newServiceTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
		&database.DirectorToolCall{},
	))
	return database.GetDBWrapper()
}

func createTestBusinessForService(t *testing.T, db *database.DB, name string) database.Business {
	t.Helper()
	business := database.Business{
		Name:           name,
		SettlementAddr: "settlement-" + name,
		TippingAddr:    "tipping-" + name,
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)
	return business
}

func assertHasEvent(t *testing.T, events []SinkEvent, eventType string) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == eventType {
			return
		}
	}
	t.Fatalf("expected event of type %q, got %d events: %+v", eventType, len(events), events)
}

func TestRunDirectorLoop_FunctionCallThenJSON(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "Loop Test Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "loop test", "en")
	require.NoError(t, err)

	registry := director_tools.NewRegistry()
	registry.Register(&director_tools.BusinessProfileTool{})

	finalJSON, err := json.Marshal(DirectorStructuredResponse{
		Summary:   "Loop completed",
		Diagnosis: "All good",
		Evidence:  []string{"Used profile tool"},
		ActionPlan: []DirectorAction{
			{
				Title:       "Review analytics",
				Description: "Look at weekly trends",
				DeepLink:    "/business/1/dashboard?tab=analytics",
				Priority:    "high",
			},
		},
		ExpectedImpact: "Improved focus",
		FollowUps:      []string{"What's next?"},
	})
	require.NoError(t, err)

	fake := &fakeProvider{
		scripted: []*llm.Response{
			newFunctionCallResponse("get_business_profile", map[string]any{}),
			newTextResponse(string(finalJSON)), // tool-allowed probe
			newTextResponse(string(finalJSON)), // schema finalization
		},
	}

	sink := &bufferSink{}
	env := director_tools.ToolEnv{
		BusinessID: business.ID,
		ThreadID:   thread.ID,
		Locale:     "en",
		DB:         db,
		Analytics:  analytics.NewAnalyticsService(db),
	}

	cfg := LoopConfig{
		Registry:      registry,
		Sink:          sink,
		Env:           env,
		Model:         "gemini-test",
		MaxIterations: 6,
		WallClock:     5 * time.Second,
		Provider:      fake,
		SystemPrompt:  "You are a test assistant.",
		UserPrompt:    "Test question",
	}

	res, err := RunDirectorLoop(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, "Loop completed", res.Final.Summary)
	assert.Equal(t, "gemini-test", res.Model)
	assert.Len(t, res.ToolCallIDs, 1, "expected one tool call persisted")
	assert.Equal(t, 3, fake.calls, "probe-with-tool + tool-result turn + schema finalization")

	assertHasEvent(t, sink.events, "tool.call.started")
	assertHasEvent(t, sink.events, "tool.call.completed")

	var rows []database.DirectorToolCall
	err = database.GetDB().Where("thread_id = ?", thread.ID).Order("id asc").Find(&rows).Error
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "get_business_profile", rows[0].ToolName)
	assert.True(t, rows[0].Success)
}

func TestRunDirectorLoop_UnknownToolReturnsError(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "Unknown Tool Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "unknown tool test", "en")
	require.NoError(t, err)

	registry := director_tools.NewRegistry()
	registry.Register(&director_tools.BusinessProfileTool{})

	finalJSON := `{"summary":"recovered","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`

	fake := &fakeProvider{
		scripted: []*llm.Response{
			newFunctionCallResponse("does_not_exist", map[string]any{}),
			newTextResponse(finalJSON), // tool-allowed probe after unknown tool
			newTextResponse(finalJSON), // schema finalization
		},
	}

	sink := &bufferSink{}
	env := director_tools.ToolEnv{
		BusinessID: business.ID,
		ThreadID:   thread.ID,
		Locale:     "en",
		DB:         db,
		Analytics:  analytics.NewAnalyticsService(db),
	}

	cfg := LoopConfig{
		Registry:      registry,
		Sink:          sink,
		Env:           env,
		Model:         "gemini-test",
		MaxIterations: 6,
		WallClock:     5 * time.Second,
		Provider:      fake,
		SystemPrompt:  "test",
		UserPrompt:    "test",
	}

	res, err := RunDirectorLoop(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "recovered", res.Final.Summary)
	assert.Empty(t, res.ToolCallIDs, "unknown tools should not be persisted")
}

func TestRunDirectorLoop_MaxIterationsExceeded(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "Max Iter Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "max iter test", "en")
	require.NoError(t, err)

	registry := director_tools.NewRegistry()
	registry.Register(&director_tools.BusinessProfileTool{})

	fake := &fakeProvider{
		scripted: []*llm.Response{
			newFunctionCallResponse("get_business_profile", map[string]any{}),
			newFunctionCallResponse("get_business_profile", map[string]any{}),
			newFunctionCallResponse("get_business_profile", map[string]any{}),
		},
	}

	sink := &bufferSink{}
	env := director_tools.ToolEnv{
		BusinessID: business.ID,
		ThreadID:   thread.ID,
		Locale:     "en",
		DB:         db,
		Analytics:  analytics.NewAnalyticsService(db),
	}

	cfg := LoopConfig{
		Registry:      registry,
		Sink:          sink,
		Env:           env,
		Model:         "gemini-test",
		MaxIterations: 2,
		WallClock:     5 * time.Second,
		Provider:      fake,
		SystemPrompt:  "test",
		UserPrompt:    "test",
	}

	_, err = RunDirectorLoop(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MaxIterations")
}

// TestDirectorLoopSingleRoundTripPerIteration is an access-shape guard: a single
// text response must drive exactly two Generate calls: the tool-allowed probe
// plus the strict-schema finalization turn.
func TestDirectorLoopSingleRoundTripPerIteration(t *testing.T) {
	final := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	fake := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(final), // tool-allowed probe returns text (no tool calls)
		newTextResponse(final), // schema finalization
	}}
	reg := director_tools.NewRegistry()

	_, err := RunDirectorLoop(context.Background(), LoopConfig{
		Registry:     reg,
		Provider:     fake,
		Model:        "test-model",
		SystemPrompt: "sys",
		UserPrompt:   "hi",
	})
	if err != nil {
		t.Fatalf("RunDirectorLoop: %v", err)
	}
	if fake.calls != 2 {
		t.Fatalf("expected exactly 2 Generate calls (probe + schema finalization), got %d", fake.calls)
	}
}

// capturingDirectorProvider scripts responses in order and records every
// request it received, so tests can assert what the loop sent back.
type capturingDirectorProvider struct {
	scripted []*llm.Response
	requests []llm.GenerateRequest
	calls    int
}

func (c *capturingDirectorProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	c.requests = append(c.requests, req)
	if c.calls >= len(c.scripted) {
		return nil, fmt.Errorf("capturingDirectorProvider: no more scripted responses (call #%d)", c.calls+1)
	}
	resp := c.scripted[c.calls]
	c.calls++
	return resp, nil
}

func TestRunDirectorLoop_SynthesizesEmptyToolCallID(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "Empty ID Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "empty id test", "en")
	require.NoError(t, err)

	registry := director_tools.NewRegistry()
	registry.Register(&director_tools.BusinessProfileTool{})

	finalJSON := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	// First response: a tool call with an EMPTY ID (as a non-Gemini fallback might return).
	cap := &capturingDirectorProvider{scripted: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "", Name: "get_business_profile", Args: map[string]any{}}}},
		newTextResponse(finalJSON), // tool-allowed probe after tool execution
		newTextResponse(finalJSON), // schema finalization
	}}

	env := director_tools.ToolEnv{
		BusinessID: business.ID,
		ThreadID:   thread.ID,
		Locale:     "en",
		DB:         db,
		Analytics:  analytics.NewAnalyticsService(db),
	}

	res, err := RunDirectorLoop(context.Background(), LoopConfig{
		Registry:     registry,
		Sink:         &bufferSink{},
		Env:          env,
		Model:        "test-model",
		Provider:     cap,
		SystemPrompt: "sys",
		UserPrompt:   "hi",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, cap.requests, 3, "expected three Generate calls (tool-call + probe + schema finalization)")

	// On the 2nd request, the history must contain the assistant tool-call turn
	// with a synthesized (non-empty) ID, and a tool result whose ToolCallID matches.
	msgs := cap.requests[1].Messages
	var asstID, toolID string
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) > 0 {
			asstID = m.ToolCalls[0].ID
		}
		if m.Role == llm.RoleTool {
			toolID = m.ToolCallID
		}
	}
	require.NotEmpty(t, asstID, "assistant tool-call ID should be synthesized, not empty")
	require.Equal(t, asstID, toolID, "tool result ToolCallID must match the assistant tool-call ID")
}

func TestRunDirectorLoop_SeedsPriorTurns(t *testing.T) {
	db := newServiceTestDB(t)
	reg := director_tools.NewRegistry()
	final := `{"summary":"ok","diagnosis":"d","evidence":[],"actions":[],"expected_impact":"i","follow_ups":[]}`

	var captured llm.GenerateRequest
	prov := &capturingProvider{resp: newTextResponse(final), capture: &captured}

	cfg := LoopConfig{
		Registry:     reg,
		Provider:     prov,
		Env:          director_tools.ToolEnv{BusinessID: 1, ThreadID: 1, DB: db},
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "current question",
		PriorTurns: []llm.Message{
			{Role: llm.RoleUser, Text: "q1"},
			{Role: llm.RoleAssistant, Text: "a1"},
		},
	}
	_, err := RunDirectorLoop(context.Background(), cfg)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(captured.Messages), 3)
	assert.Equal(t, "q1", captured.Messages[0].Text)
	assert.Equal(t, "a1", captured.Messages[1].Text)
	assert.Equal(t, "current question", captured.Messages[2].Text)
}

// capturingProvider records the most recent GenerateRequest (overwritten on
// every call) and returns a fixed response each time. Use it when only the
// first/last request's Messages matter; for per-call inspection across the
// loop's probe + schema-finalization turns, use capturingDirectorProvider.
type capturingProvider struct {
	resp    *llm.Response
	capture *llm.GenerateRequest
}

func (c *capturingProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	*c.capture = req
	return c.resp, nil
}
