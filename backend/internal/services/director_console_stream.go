package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"
)

// pillLocale collapses a request locale to the base language subtag the tool
// HumanLabel switches expect ("es", "fr", "ar", …). The request locale arrives
// canonicalized (e.g. "es-AR") or as a prompt family ("es_ar"); neither matches
// a tool's `case "es"` / `case "es", "es_ar"` branch, so Argentine operators
// were shown the English loading pill mid-stream. The Spanish pill copy is
// neutral Spanish and reads fine for es-AR, so collapsing the region away is
// safe.
func pillLocale(locale string) string {
	normalized := strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	return strings.SplitN(normalized, "-", 2)[0]
}

// SinkEvent is the unit of progress emitted by the Director Console loop.
// Type is one of "tool.call.started" | "tool.call.completed" | "error".
// Payload carries the type-specific fields the SSE handler forwards to the
// client (and the in-memory bufferSink keeps for the non-streaming Ask path).
type SinkEvent struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// Sink is the destination for loop progress events. The Phase 3 SSE handler
// plugs in a streaming implementation; the non-streaming Ask path uses
// bufferSink.
type Sink interface {
	Emit(SinkEvent)
}

// bufferSink collects events in memory — used by the non-streaming Ask path
// when the caller doesn't care about per-step progress.
type bufferSink struct {
	events []SinkEvent
}

// Emit appends an event to the buffer.
func (b *bufferSink) Emit(ev SinkEvent) {
	b.events = append(b.events, ev)
}

// LoopConfig bundles every input the function-calling driver needs. Set
// MaxIterations to 0 to fall back to the default (6). Set WallClock to 0 to
// fall back to the default (35s). Set PerIterationTimeout to 0 to fall back to
// the default (12s).
type LoopConfig struct {
	Registry            *director_tools.Registry
	Sink                Sink
	Env                 director_tools.ToolEnv
	Model               string
	MaxIterations       int
	WallClock           time.Duration
	PerIterationTimeout time.Duration
	Provider            llm.Provider
	Fallbacks           []string
	SystemPrompt        string
	UserPrompt          string
	PriorTurns          []llm.Message
}

// LoopResult is what RunDirectorLoop returns on success.
type LoopResult struct {
	Final       DirectorStructuredResponse
	ToolCallIDs []uint
	ProposalIDs []uint
	Model       string
	LatencyMs   int64
}

// DirectorLoopTimeoutError signals the wall clock was exhausted mid-loop. It
// carries the summaries of any tool calls that completed before the deadline so
// the caller can build an honest partial-progress answer instead of the generic
// fallback. This is NOT a hard provider error (those still return a plain error).
type DirectorLoopTimeoutError struct {
	CompletedSummaries []string
}

func (e *DirectorLoopTimeoutError) Error() string {
	return fmt.Sprintf("director loop timed out after %d completed tool calls", len(e.CompletedSummaries))
}

// directorTemperature returns the intentional low-variance setting for the
// director loop (audit P0-3: synthesis quality favors determinism).
func directorTemperature() *float32 {
	t := float32(0.2)
	return &t
}

const (
	defaultDirectorLoopMaxIterations   = 6
	defaultDirectorLoopWallClock       = 35 * time.Second
	defaultDirectorPerIterationTimeout = 12 * time.Second
)

// RunDirectorLoop drives the function-calling conversation up to
// cfg.MaxIterations turns under cfg.WallClock. Each turn either returns text
// (final JSON, parsed into DirectorStructuredResponse) or a batch of tool
// calls; tool calls are executed sequentially, persisted to director_tool_calls,
// and their results fed back to the model as tool-role messages.
func RunDirectorLoop(ctx context.Context, cfg LoopConfig) (*LoopResult, error) {
	if cfg.Registry == nil {
		return nil, fmt.Errorf("director loop: nil registry")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("director loop: nil provider")
	}
	if cfg.Sink == nil {
		cfg.Sink = &bufferSink{}
	}

	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultDirectorLoopMaxIterations
	}
	wallClock := cfg.WallClock
	if wallClock <= 0 {
		wallClock = defaultDirectorLoopWallClock
	}
	perIter := cfg.PerIterationTimeout
	if perIter <= 0 {
		perIter = defaultDirectorPerIterationTimeout
	}

	loopCtx, cancel := context.WithTimeout(ctx, wallClock)
	defer cancel()

	tools := cfg.Registry.Declarations()

	history := make([]llm.Message, 0, len(cfg.PriorTurns)+1)
	history = append(history, cfg.PriorTurns...)
	history = append(history, llm.Message{Role: llm.RoleUser, Text: cfg.UserPrompt})

	toolCallIDs := make([]uint, 0)
	proposalIDs := make([]uint, 0)
	completedSummaries := make([]string, 0)
	started := time.Now()

	for i := 0; i < maxIter; i++ {
		// Honor the wall clock before spending another iteration. If we already
		// gathered tool results, hand them back as a partial-progress sentinel.
		if loopCtx.Err() != nil {
			return nil, &DirectorLoopTimeoutError{CompletedSummaries: completedSummaries}
		}

		iterCtx, iterCancel := context.WithTimeout(loopCtx, perIter)
		resp, err := cfg.Provider.Generate(iterCtx, llm.GenerateRequest{
			Model:       cfg.Model,
			Fallbacks:   cfg.Fallbacks,
			System:      cfg.SystemPrompt,
			Messages:    history,
			Tools:       tools,
			Feature:     "director",
			MaxTokens:   4096,
			Temperature: directorTemperature(),
			BusinessID:  cfg.Env.BusinessID,
		})
		iterCancel()
		if err != nil {
			// Distinguish wall-clock exhaustion (partial sentinel) from a hard
			// provider error. If the parent wall clock is done AND we have
			// completed tool work, surface the partial sentinel.
			if loopCtx.Err() != nil && len(completedSummaries) > 0 {
				return nil, &DirectorLoopTimeoutError{CompletedSummaries: completedSummaries}
			}
			cfg.Sink.Emit(SinkEvent{
				Type:    "error",
				Payload: map[string]any{"iteration": i, "error": err.Error()},
			})
			return nil, fmt.Errorf("director loop: generate content (iter %d): %w", i, err)
		}

		text, calls := resp.Text, resp.ToolCalls

		if len(calls) == 0 {
			// The model produced no tool calls. Re-request the answer with a
			// strict schema and no tools so the JSON adheres deterministically.
			// Reuse this turn's text if the schema call fails (defense-in-depth).
			finalText := strings.TrimSpace(text)
			finalCtx, finalCancel := context.WithTimeout(loopCtx, perIter)
			schemaResp, schemaErr := cfg.Provider.Generate(finalCtx, llm.GenerateRequest{
				Model:          cfg.Model,
				Fallbacks:      cfg.Fallbacks,
				System:         cfg.SystemPrompt,
				Messages:       history,
				ResponseSchema: directorResponseSchema(),
				Feature:        "director",
				MaxTokens:      4096,
				Temperature:    directorTemperature(),
				BusinessID:     cfg.Env.BusinessID,
			})
			finalCancel()
			if schemaErr == nil && strings.TrimSpace(schemaResp.Text) != "" {
				finalText = strings.TrimSpace(schemaResp.Text)
			}
			if finalText == "" {
				return nil, fmt.Errorf("director loop: empty text response on iter %d", i)
			}
			parsed, parseErr := parseDirectorJSON(finalText)
			if parseErr != nil {
				return nil, fmt.Errorf("director loop: parse final JSON (iter %d): %w", i, parseErr)
			}
			return &LoopResult{
				Final:       parsed,
				ToolCallIDs: toolCallIDs,
				ProposalIDs: proposalIDs,
				Model:       cfg.Model,
				LatencyMs:   time.Since(started).Milliseconds(),
			}, nil
		}

		// Some providers/fallback models omit tool-call IDs. Synthesize stable
		// ones so the assistant tool_calls and their matching tool results pair
		// up correctly for strict OpenAI-protocol upstreams.
		for idx := range calls {
			if calls[idx].ID == "" {
				calls[idx].ID = fmt.Sprintf("call_%d_%d", i, idx)
			}
		}

		// Capture the model's tool-call turn in history so the model sees its
		// own request alongside the response parts on the next iteration.
		history = append(history, llm.Message{Role: llm.RoleAssistant, ToolCalls: calls})

		// Execute every requested call sequentially and stage their responses
		// for the next set of tool-role turns.
		toolMsgs := make([]llm.Message, 0, len(calls))
		for _, fc := range calls {
			tool, ok := cfg.Registry.Get(fc.Name)
			if !ok {
				toolMsgs = append(toolMsgs, llm.Message{
					Role:       llm.RoleTool,
					ToolCallID: fc.ID,
					ToolName:   fc.Name,
					Text:       `{"error":"unknown_tool"}`,
				})
				continue
			}

			args := fc.Args
			if args == nil {
				args = map[string]any{}
			}

			cfg.Sink.Emit(SinkEvent{
				Type: "tool.call.started",
				Payload: map[string]any{
					"name":        fc.Name,
					"human_label": tool.HumanLabel(pillLocale(cfg.Env.Locale)),
					"args":        args,
				},
			})

			callStarted := time.Now()
			result, runErr := tool.Run(loopCtx, args, cfg.Env)
			duration := time.Since(callStarted)
			durationMs := int(duration.Milliseconds())

			argsJSON, _ := json.Marshal(args)
			record := &database.DirectorToolCall{
				ThreadID:   cfg.Env.ThreadID,
				BusinessID: cfg.Env.BusinessID,
				ToolName:   fc.Name,
				ArgsJSON:   string(argsJSON),
				Summary:    result.Summary,
				DurationMs: durationMs,
				Success:    runErr == nil,
			}
			if runErr != nil {
				record.Error = runErr.Error()
			}
			if saveErr := database.SaveDirectorToolCall(record); saveErr == nil {
				toolCallIDs = append(toolCallIDs, record.ID)
			}

			if runErr == nil && result.ProposalID != 0 {
				proposalIDs = append(proposalIDs, result.ProposalID)
			}

			cfg.Sink.Emit(SinkEvent{
				Type: "tool.call.completed",
				Payload: map[string]any{
					"name":        fc.Name,
					"summary":     result.Summary,
					"duration_ms": durationMs,
					"success":     runErr == nil,
				},
			})

			if runErr == nil && strings.TrimSpace(result.Summary) != "" {
				completedSummaries = append(completedSummaries, result.Summary)
			}

			respPayload := map[string]any{}
			if runErr != nil {
				respPayload["error"] = runErr.Error()
			} else if result.Data != nil {
				respPayload = result.Data
			}
			payloadJSON, _ := json.Marshal(respPayload)

			toolMsgs = append(toolMsgs, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: fc.ID,
				ToolName:   fc.Name,
				Text:       string(payloadJSON),
			})
		}

		history = append(history, toolMsgs...)
	}

	return nil, fmt.Errorf("director loop exceeded MaxIterations=%d", maxIter)
}

// directorResponseSchema is the strict JSON schema mirroring
// DirectorStructuredResponse, set on the finalization turn so the model returns
// schema-valid JSON directly (audit §4-D). parseDirectorJSON stays as
// defense-in-depth for providers/fallbacks that ignore the schema.
func directorResponseSchema() *llm.JSONSchema {
	str := &llm.JSONSchema{Type: llm.TypeString}
	strArr := &llm.JSONSchema{Type: llm.TypeArray, Items: &llm.JSONSchema{Type: llm.TypeString}}
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"summary":         str,
			"diagnosis":       str,
			"evidence":        strArr,
			"expected_impact": str,
			"follow_ups":      strArr,
			"actions": {
				Type: llm.TypeArray,
				Items: &llm.JSONSchema{
					Type: llm.TypeObject,
					Properties: map[string]*llm.JSONSchema{
						"title":       {Type: llm.TypeString},
						"description": {Type: llm.TypeString},
						"deep_link":   {Type: llm.TypeString},
						"priority":    {Type: llm.TypeString, Enum: []string{"high", "medium", "low"}},
					},
					Required: []string{"title", "description", "deep_link", "priority"},
				},
			},
		},
		Required: []string{"summary", "diagnosis", "evidence", "actions", "expected_impact", "follow_ups"},
	}
}
