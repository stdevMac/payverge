package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// AskDirectorStream is the SSE counterpart of AskDirector. It drives the
// same Ask loop as POST /ai/director/ask but pipes each tool.call.started
// and tool.call.completed event out as a Server-Sent Event so the dashboard
// can render a live tool-trace while the model is still thinking. The final
// response.complete event carries the assistant message ID, latency, model
// name, thread ID, the request-supplied nonce, and the list of persisted
// tool-call IDs the client can use to back-link the trace.
//
// POST /api/v1/inside/businesses/:id/ai/director/ask/stream
func AskDirectorStream(c *gin.Context) {
	if !ensureOwnerToken(c) {
		return
	}

	businessIDStr := c.Param("id")
	businessID64, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	businessID := uint(businessID64)
	if !ensureDirectorFeatureAvailable(c, businessID) {
		return
	}
	// Asking the Director is the LLM-only part of the console (threads,
	// briefing, insights and the apply/undo rail stay data-driven), so with no
	// provider it answers 503 ai_not_configured instead of a canned reply.
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}

	service := GetDirectorConsoleService()
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Director console service unavailable"})
		return
	}

	var req struct {
		Message    string `json:"message" binding:"required"`
		ThreadID   *uint  `json:"thread_id"`
		Locale     string `json:"locale"`
		ActiveTab  string `json:"active_tab"`
		Nonce      string `json:"nonce"`
		Regenerate bool   `json:"regenerate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	// X-Accel-Buffering=no tells Caddy (and nginx) to flush our response as
	// soon as we write it. Without it, intermediate buffering can hold the
	// stream until the loop completes and defeats the whole point of SSE.
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)
	streamWriter := newSSEStreamWriter(c.Writer, flusher)
	emit := streamWriter.Emit

	sink := &sseSink{emit: emit}

	// Heartbeat comment every 5 s keeps the connection warm through any
	// reverse proxy idle timeout while the model is still generating.
	// stop waits for the goroutine so a tick cannot write the gin writer
	// after this handler returns (gin recycles the context).
	stopHeartbeat := startSSEHeartbeat(5*time.Second, func() { streamWriter.Comment("ping") })
	defer stopHeartbeat()

	res, err := service.AskStreaming(c.Request.Context(), services.DirectorAskRequest{
		BusinessID: businessID,
		Message:    req.Message,
		ThreadID:   req.ThreadID,
		Locale:     req.Locale,
		ActiveTab:  req.ActiveTab,
		Regenerate: req.Regenerate,
	}, sink)
	if err != nil {
		logger.Logger.Errorf("director console stream: AskStreaming failed (business=%d, thread=%v): %v", businessID, req.ThreadID, err)
		// FIND-060: log provider/stack detail server-side; SSE clients get product copy only.
		emit("error", map[string]any{"code": "ask_failed", "message": "Director could not complete this request"})
		return
	}

	emit("response.complete", map[string]any{
		"message_id":    res.AssistantMessage.ID,
		"latency_ms":    res.Usage.LatencyMs,
		"model":         res.Usage.Model,
		"thread_id":     res.Thread.ID,
		"nonce":         req.Nonce,
		"tool_call_ids": res.ToolCallIDs,
		"response":      res.Response,
		// Staged write proposals (Phase 5 proposal card). The dashboard only
		// talks to this streaming endpoint, so without this field proposals
		// would exist solely on the non-streaming /ask response.
		"proposed_actions": res.ProposedActions,
	})
}

// sseSink fans services.Sink events out as SSE event lines via the captured
// emit closure. The handler keeps the closure private so we don't have to
// thread the gin.ResponseWriter through the streaming code path.
type sseSink struct {
	emit func(event string, payload any)
}

// Emit forwards a single Sink event onto the SSE wire as an
// "event: <type>\ndata: <payload>\n\n" frame.
func (s *sseSink) Emit(e services.SinkEvent) {
	s.emit(e.Type, e.Payload)
}

type sseStreamWriter struct {
	mu      sync.Mutex
	writer  io.Writer
	flusher http.Flusher
}

func newSSEStreamWriter(writer io.Writer, flusher http.Flusher) *sseStreamWriter {
	return &sseStreamWriter{writer: writer, flusher: flusher}
}

func (w *sseStreamWriter) Emit(event string, payload any) {
	data, _ := json.Marshal(payload)

	w.mu.Lock()
	defer w.mu.Unlock()

	fmt.Fprintf(w.writer, "event: %s\ndata: %s\n\n", event, data)
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

func (w *sseStreamWriter) Comment(comment string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	fmt.Fprintf(w.writer, ": %s\n\n", comment)
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

// startSSEHeartbeat runs ping on interval until the returned stop is called.
// stop closes the loop and waits for the goroutine to finish, so ping cannot
// run after stop returns. stop is idempotent.
func startSSEHeartbeat(interval time.Duration, ping func()) (stop func()) {
	stopCh := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	logger.SafeGo(func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ping()
			case <-stopCh:
				return
			}
		}
	})
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stopCh)
			wg.Wait()
		})
	}
}
