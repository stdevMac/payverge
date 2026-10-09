package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDirectorConsoleService is a DirectorConsoleServiceAPI fake used by the
// SSE handler test. AskStreaming emits a scripted sequence of Sink events and
// then returns a deterministic DirectorAskResult — the handler should
// translate the script into SSE frames followed by response.complete.
type stubDirectorConsoleService struct {
	events []services.SinkEvent
	result *services.DirectorAskResult
	err    error
	gotReq services.DirectorAskRequest
}

func (s *stubDirectorConsoleService) Ask(_ context.Context, req services.DirectorAskRequest) (*services.DirectorAskResult, error) {
	s.gotReq = req
	return s.result, s.err
}

func (s *stubDirectorConsoleService) AskStreaming(_ context.Context, req services.DirectorAskRequest, sink services.Sink) (*services.DirectorAskResult, error) {
	s.gotReq = req
	for _, ev := range s.events {
		sink.Emit(ev)
	}
	return s.result, s.err
}

func (s *stubDirectorConsoleService) ListThreads(_ uint) ([]services.DirectorThreadDTO, error) {
	return nil, nil
}

func (s *stubDirectorConsoleService) ListThreadsPaged(_ uint, _ services.ListThreadsOptions) (services.ListThreadsResult, error) {
	return services.ListThreadsResult{}, nil
}

func (s *stubDirectorConsoleService) ListThreadMessages(_ uint, _ uint, _ int) ([]services.DirectorMessageDTO, error) {
	return nil, nil
}

func (s *stubDirectorConsoleService) SubmitFeedback(_ uint, _ uint, _ database.DirectorFeedbackVote) (*services.DirectorMessageDTO, error) {
	return nil, nil
}

// withStubDirectorService swaps the package-level service for the duration of
// the test and restores the previous value afterwards. Without restoring we'd
// leak the stub into sibling tests that share the binary's process memory.
func withStubDirectorService(t *testing.T, stub DirectorConsoleServiceAPI) {
	t.Helper()
	prev := directorConsoleService
	directorConsoleService = stub
	t.Cleanup(func() {
		directorConsoleService = prev
	})
}

// parseSSEEvents pulls out the named events from an SSE response body. Each
// returned entry preserves the order events appeared in the stream so tests
// can assert on the full sequence, not just presence. Heartbeat comment lines
// (": ping") are filtered out because they don't carry semantic value.
type sseFrame struct {
	Event string
	Data  string
}

func parseSSEEvents(t *testing.T, body string) []sseFrame {
	t.Helper()
	var frames []sseFrame
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if strings.HasPrefix(block, ":") {
			continue
		}
		var name, data string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				if data != "" {
					data += "\n"
				}
				data += strings.TrimPrefix(line, "data: ")
			}
		}
		if name != "" {
			frames = append(frames, sseFrame{Event: name, Data: data})
		}
	}
	return frames
}

func TestSSEStreamWriterSerializesConcurrentWrites(t *testing.T) {
	var buffer bytes.Buffer
	writer := newSSEStreamWriter(&buffer, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			writer.Emit("tool.call.started", map[string]any{"index": i})
		}(i)
		go func() {
			defer wg.Done()
			writer.Comment("ping")
		}()
	}
	wg.Wait()

	body := buffer.String()
	frames := parseSSEEvents(t, body)
	require.Len(t, frames, 50)
	assert.Equal(t, 50, strings.Count(body, ": ping\n\n"))
	for _, frame := range frames {
		assert.Equal(t, "tool.call.started", frame.Event)
		assert.True(t, strings.HasPrefix(frame.Data, `{"index":`), "event frame should remain intact: %q", frame.Data)
	}
}

func TestAskDirectorStream_EmitsToolEventsThenResponseComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "director-stream-happy")

	stub := &stubDirectorConsoleService{
		events: []services.SinkEvent{
			{
				Type: "tool.call.started",
				Payload: map[string]any{
					"name":        "get_revenue_summary",
					"human_label": "Pulling revenue summary",
					"args":        map[string]any{"period": "week"},
				},
			},
			{
				Type: "tool.call.completed",
				Payload: map[string]any{
					"name":        "get_revenue_summary",
					"summary":     "Weekly revenue: $4,200",
					"duration_ms": 123,
					"success":     true,
				},
			},
		},
		result: &services.DirectorAskResult{
			Thread: database.DirectorConsoleThread{
				BusinessID: business.ID,
			},
			AssistantMessage: database.DirectorConsoleMessage{
				BusinessID: business.ID,
				Role:       database.DirectorMessageRoleAssistant,
				Content:    "Stub summary",
			},
			Response: services.DirectorStructuredResponse{
				Summary: "Stub summary",
			},
			Usage: services.DirectorUsage{
				Model:     "stub-model",
				LatencyMs: 42,
			},
			ToolCallIDs: []uint{11, 12},
			ProposedActions: []director_actions.ProposedAction{
				{
					ID:    "pa_stream1",
					Kind:  director_actions.KindAdjustPrices,
					Title: "Raise dessert prices 5%",
					Preview: director_actions.ActionPreview{
						AffectedCount: 3,
						Summary:       "3 items change",
					},
					MenuVersion: 9,
				},
			},
		},
	}
	// Assign deterministic IDs so we can assert on them downstream.
	stub.result.Thread.ID = 777
	stub.result.AssistantMessage.ID = 888

	withStubDirectorService(t, stub)
	// Asking the Director needs a configured LLM provider (503 otherwise).
	config.SetAIProviderConfiguredForTesting(t, true)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.POST("/businesses/:id/ai/director/ask/stream", AskDirectorStream)

	reqBody := mustJSON(t, map[string]any{
		"message":    "How is revenue trending this week?",
		"locale":     "en",
		"active_tab": "analytics",
		"nonce":      "nonce-abc",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/businesses/%d/ai/director/ask/stream", business.ID), bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-transform", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))

	frames := parseSSEEvents(t, w.Body.String())
	require.GreaterOrEqual(t, len(frames), 3, "expected at least tool.call.started, tool.call.completed, response.complete; got %d frames: %s", len(frames), w.Body.String())

	// Event order must be deterministic so the frontend can rely on it to
	// drive the tool-trace timeline.
	assert.Equal(t, "tool.call.started", frames[0].Event)
	assert.Equal(t, "tool.call.completed", frames[1].Event)
	assert.Equal(t, "response.complete", frames[len(frames)-1].Event)

	// tool.call.started payload carries the human label so the UI can render
	// "Pulling revenue summary" without a second lookup.
	var startedPayload map[string]any
	require.NoError(t, json.Unmarshal([]byte(frames[0].Data), &startedPayload))
	assert.Equal(t, "get_revenue_summary", startedPayload["name"])
	assert.Equal(t, "Pulling revenue summary", startedPayload["human_label"])

	// response.complete carries the assistant message ID, thread ID, model,
	// latency, nonce, and the persisted tool-call IDs.
	var completePayload map[string]any
	require.NoError(t, json.Unmarshal([]byte(frames[len(frames)-1].Data), &completePayload))
	assert.EqualValues(t, 888, completePayload["message_id"])
	assert.EqualValues(t, 777, completePayload["thread_id"])
	assert.Equal(t, "stub-model", completePayload["model"])
	assert.EqualValues(t, 42, completePayload["latency_ms"])
	assert.Equal(t, "nonce-abc", completePayload["nonce"])
	ids, ok := completePayload["tool_call_ids"].([]any)
	require.True(t, ok, "tool_call_ids should serialize as a JSON array")
	require.Len(t, ids, 2)
	assert.EqualValues(t, 11, ids[0])
	assert.EqualValues(t, 12, ids[1])

	// response.complete must also carry the staged write proposals — the
	// dashboard talks to this endpoint (not the non-streaming /ask), so this
	// is the only live path for the Phase 5 proposal card.
	proposals, ok := completePayload["proposed_actions"].([]any)
	require.True(t, ok, "proposed_actions should serialize as a JSON array")
	require.Len(t, proposals, 1)
	proposal, ok := proposals[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pa_stream1", proposal["id"])
	assert.Equal(t, string(director_actions.KindAdjustPrices), proposal["kind"])
	assert.Equal(t, "Raise dessert prices 5%", proposal["title"])
	assert.EqualValues(t, 9, proposal["menu_version"])

	// The stub captured the incoming request — confirm the handler forwarded
	// the body fields (apart from nonce, which only lives in the response).
	assert.Equal(t, business.ID, stub.gotReq.BusinessID)
	assert.Equal(t, "How is revenue trending this week?", stub.gotReq.Message)
	assert.Equal(t, "analytics", stub.gotReq.ActiveTab)
	assert.Equal(t, "en", stub.gotReq.Locale)
}

func TestAskDirectorStream_RejectsStaffToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "director-stream-staff")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Staff token must be rejected before we ever reach the service.
		c.Set("token_type", "staff")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.POST("/businesses/:id/ai/director/ask/stream", AskDirectorStream)

	reqBody := mustJSON(t, map[string]any{"message": "hello"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/businesses/%d/ai/director/ask/stream", business.ID), bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
