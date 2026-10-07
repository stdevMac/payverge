package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/pii"
)

// SinkEvent mirrors Director Console streaming events.
type SinkEvent struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// Sink receives loop progress.
type Sink interface {
	Emit(SinkEvent)
}

type bufferSink struct {
	events []SinkEvent
}

func (b *bufferSink) Emit(ev SinkEvent) { b.events = append(b.events, ev) }

// EscalationEvent carries a deterministic escalation raised either by an
// escalation/lead tool firing or by a hard loop failure (max iterations /
// provider error). It carries a Source label so the surface is recorded with
// the escalation.
type EscalationEvent struct {
	Source       string // "ops"
	BusinessID   uint
	SessionRef   string
	ThreadID     uint
	Issue        string
	Transcript   string
	ContactEmail string
	Reason       string // "tool" | "loop_failure"
	ToolName     string
	// EmailAlreadySent is true when the raising path already delivered the admin
	// email (the SupportEscalationTool sends synchronously inside the loop). The
	// Escalator then skips its own email so one incident isn't double-sent, while
	// still persisting the row and firing the Telegram fallback. False for tool
	// escalations whose email failed (Escalate becomes the fallback sender) and
	// for loop-failure / lead escalations that never email inline.
	EmailAlreadySent bool
}

// Escalator receives EscalationEvents. Implemented by internal/escalation.Service.
type Escalator interface {
	Escalate(EscalationEvent)
}

// escalationToolNames are the tools whose success should also raise a
// deterministic escalation (durable row + Telegram/email), on top of whatever
// the tool itself does.
var escalationToolNames = map[string]bool{
	"escalate_to_sales":         true,
	"capture_lead":              true,
	"create_support_escalation": true,
}

// LoopConfig drives RunAgentLoop.
type LoopConfig struct {
	Registry            *Registry
	Sink                Sink
	Env                 ToolEnv
	Model               string
	Fallbacks           []string
	MaxIterations       int
	WallClock           time.Duration
	PerIterationTimeout time.Duration
	Provider            llm.Provider
	SystemPrompt        string
	UserPrompt          string
	PriorTurns          []llm.Message
	Feature             string
	PersistToolCalls    func(*database.OpsAssistantToolCall) error
	// OnEscalate, when set, is fired deterministically when an escalation/lead
	// tool succeeds and when the loop fails hard, so a broken bot still notifies
	// the owner. Source labels the surface ("ops").
	OnEscalate func(EscalationEvent)
	Source     string
}

// ToolEvidence records the trusted result of a successful tool execution.
// Data is expected to contain JSON-shaped values, matching ToolResult.Data.
type ToolEvidence struct {
	Name    string
	Summary string
	Data    map[string]any
}

func snapshotToolEvidenceData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	return cloneToolEvidenceValue(reflect.ValueOf(data)).Interface().(map[string]any)
}

func cloneToolEvidenceValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneToolEvidenceValue(value.Elem())
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneToolEvidenceValue(iter.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneToolEvidenceValue(value.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneToolEvidenceValue(value.Index(i)))
		}
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneToolEvidenceValue(value.Elem()))
		return out
	default:
		return value
	}
}

// LoopResult is returned on success. When finalization fails after trusted tool
// evidence was captured, it is also returned alongside the error so callers do
// not lose that evidence.
type LoopResult struct {
	Final                StructuredResponse
	ToolCallIDs          []uint
	ToolEvidence         []ToolEvidence
	StructuredRepairUsed bool
	CommunicationSent    bool
	Model                string
	LatencyMs            int64
	TotalTokens          int
}

func agentTemperature() *float32 {
	t := float32(0.2)
	return &t
}

func agentResponseSchema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"answer":     {Type: llm.TypeString},
			"steps":      {Type: llm.TypeArray, Items: &llm.JSONSchema{Type: llm.TypeString}},
			"follow_ups": {Type: llm.TypeArray, Items: &llm.JSONSchema{Type: llm.TypeString}},
			"actions": {
				Type: llm.TypeArray,
				Items: &llm.JSONSchema{
					Type: llm.TypeObject,
					Properties: map[string]*llm.JSONSchema{
						"label":           {Type: llm.TypeString},
						"href":            {Type: llm.TypeString},
						"kind":            {Type: llm.TypeString},
						"disabled":        {Type: llm.TypeBoolean},
						"disabled_reason": {Type: llm.TypeString},
					},
					Required: []string{"label", "href", "kind"},
				},
			},
			"workflow": {
				Type: llm.TypeObject,
				Properties: map[string]*llm.JSONSchema{
					"id":         {Type: llm.TypeString},
					"step_index": {Type: llm.TypeInteger},
					"step_total": {Type: llm.TypeInteger},
				},
			},
		},
		Required: []string{"answer", "steps", "actions", "follow_ups"},
	}
}

func parseAgentJSON(raw string) (StructuredResponse, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	var out StructuredResponse
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return StructuredResponse{}, err
	}
	if strings.TrimSpace(out.Answer) == "" {
		return StructuredResponse{}, fmt.Errorf("empty answer")
	}
	if out.Steps == nil {
		out.Steps = []string{}
	}
	if out.Actions == nil {
		out.Actions = []ActionLink{}
	}
	if out.FollowUps == nil {
		out.FollowUps = []string{}
	}
	return out, nil
}

// coerceAgentResponse tries to turn a first-pass model response into a
// StructuredResponse WITHOUT a second schema-reformat call. It returns ok=false
// (caller should fall back to the schema call) when the text is empty or looks
// like a malformed structured-JSON attempt — those need the reformat pass.
//
//  1. Already-structured JSON (fenced or raw) -> parsed directly.
//  2. Plain-text prose -> wrapped as {answer: text}.
//  3. Empty / malformed JSON attempt -> ok=false, fall back to schema call.
func coerceAgentResponse(raw string) (StructuredResponse, bool) {
	if parsed, err := parseAgentJSON(raw); err == nil {
		return parsed, true
	}

	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return StructuredResponse{}, false
	}
	// A leading brace/bracket means the model was attempting structured JSON but
	// parseAgentJSON already rejected it — treat as malformed and let the
	// schema-reformat call repair it rather than surfacing raw braces as prose.
	if clean[0] == '{' || clean[0] == '[' {
		return StructuredResponse{}, false
	}
	// Mixed output: prose followed by a fenced JSON copy of the structured
	// payload. Prefer the parsed payload; failing that, strip the fence so raw
	// JSON never reaches the user as chat text.
	if block, rest, found := splitTrailingFencedJSON(clean); found {
		if parsed, err := parseAgentJSON(block); err == nil {
			return parsed, true
		}
		clean = strings.TrimSpace(rest)
		if clean == "" {
			return StructuredResponse{}, false
		}
	}
	// Gemini occasionally serializes the requested response schema as YAML-like
	// text after a prose answer. Do not wrap that whole payload as user-facing
	// prose; send it through the existing schema-repair call instead.
	if looksLikeYAMLLikeAgentResponse(clean) {
		return StructuredResponse{}, false
	}
	return StructuredResponse{
		Answer:    clean,
		Steps:     []string{},
		Actions:   []ActionLink{},
		FollowUps: []string{},
	}, true
}

func looksLikeYAMLLikeAgentResponse(text string) bool {
	hasActions := false
	hasActionField := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		switch {
		case strings.HasPrefix(line, "follow_ups:"),
			strings.HasPrefix(line, "follow-ups:"),
			strings.HasPrefix(line, "workflow:"):
			return true
		case strings.HasPrefix(line, "actions:"):
			hasActions = true
		case strings.HasPrefix(line, "kind:"),
			strings.HasPrefix(line, "label:"),
			strings.HasPrefix(line, "href:"),
			strings.HasPrefix(line, "path:"):
			hasActionField = true
		}
	}
	return hasActions && hasActionField
}

// splitTrailingFencedJSON detects a ```json (or bare ```) fence whose body
// starts with '{' inside otherwise-prose model output. Returns the fence body,
// the text outside the fence, and whether a fence was found.
func splitTrailingFencedJSON(text string) (block, rest string, found bool) {
	idx := strings.LastIndex(text, "```json")
	fenceLen := len("```json")
	if idx < 0 {
		idx = strings.LastIndex(text, "```")
		fenceLen = 3
		if idx < 0 {
			return "", "", false
		}
	}
	body := text[idx+fenceLen:]
	if end := strings.Index(body, "```"); end >= 0 {
		rest = text[:idx] + body[end+3:]
		body = body[:end]
	} else {
		rest = text[:idx]
	}
	body = strings.TrimSpace(body)
	if body == "" || body[0] != '{' {
		return "", "", false
	}
	return body, rest, true
}

func pillLocale(locale string) string {
	normalized := strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	return strings.SplitN(normalized, "-", 2)[0]
}

// RunAgentLoop executes the Ops Assistant tool-calling loop.
func RunAgentLoop(ctx context.Context, cfg LoopConfig) (*LoopResult, error) {
	if cfg.Registry == nil {
		return nil, fmt.Errorf("agent loop: nil registry")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("agent loop: nil provider")
	}
	if cfg.Sink == nil {
		cfg.Sink = &bufferSink{}
	}
	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 6
	}
	wallClock := cfg.WallClock
	if wallClock <= 0 {
		wallClock = 30 * time.Second
	}
	perIter := cfg.PerIterationTimeout
	if perIter <= 0 {
		perIter = 12 * time.Second
	}
	temp := agentTemperature()

	loopCtx, cancel := context.WithTimeout(ctx, wallClock)
	defer cancel()

	tools := cfg.Registry.Declarations()
	history := make([]llm.Message, 0, len(cfg.PriorTurns)+1)
	history = append(history, cfg.PriorTurns...)
	history = append(history, llm.Message{Role: llm.RoleUser, Text: cfg.UserPrompt})

	toolCallIDs := make([]uint, 0)
	toolEvidence := make([]ToolEvidence, 0)
	finalizationRequired := false
	communicationSent := false
	totalTokens := 0
	started := time.Now()
	loopResult := func(final StructuredResponse, structuredRepairUsed bool) *LoopResult {
		return &LoopResult{
			Final:                final,
			ToolCallIDs:          toolCallIDs,
			ToolEvidence:         toolEvidence,
			StructuredRepairUsed: structuredRepairUsed,
			CommunicationSent:    communicationSent,
			Model:                cfg.Model,
			LatencyMs:            time.Since(started).Milliseconds(),
			TotalTokens:          totalTokens,
		}
	}

	for i := 0; i < maxIter; i++ {
		if loopCtx.Err() != nil {
			if len(toolEvidence) > 0 {
				return loopResult(StructuredResponse{}, false), loopCtx.Err()
			}
			return nil, loopCtx.Err()
		}
		iterCtx, iterCancel := context.WithTimeout(loopCtx, perIter)
		resp, err := cfg.Provider.Generate(iterCtx, llm.GenerateRequest{
			Model:       cfg.Model,
			Fallbacks:   cfg.Fallbacks,
			System:      cfg.SystemPrompt,
			Messages:    history,
			Tools:       tools,
			Feature:     cfg.Feature,
			MaxTokens:   2048,
			Temperature: temp,
			BusinessID:  cfg.Env.BusinessID,
		})
		iterCancel()
		if resp != nil {
			totalTokens += resp.Usage.TotalTokens
		}
		if err != nil {
			cfg.Sink.Emit(SinkEvent{Type: "error", Payload: map[string]any{"error": err.Error()}})
			fireLoopFailureEscalation(cfg, err)
			if len(toolEvidence) > 0 {
				return loopResult(StructuredResponse{}, false), err
			}
			return nil, err
		}

		if len(resp.ToolCalls) == 0 {
			// Fast path: the tool-free answer (the most common case for both bots)
			// can usually be used as-is — the model already emits schema JSON, or a
			// plain-text answer we can wrap — so we avoid a second Generate call that
			// would ~double tokens and latency. Post-tool terminal turns require valid
			// schema JSON; plain prose there goes through the bounded schema call.
			if !finalizationRequired {
				if parsed, ok := coerceAgentResponse(resp.Text); ok {
					return loopResult(parsed, false), nil
				}
			} else if parsed, parseErr := parseAgentJSON(resp.Text); parseErr == nil {
				return loopResult(parsed, false), nil
			}

			finalText := strings.TrimSpace(resp.Text)
			finalCtx, finalCancel := context.WithTimeout(loopCtx, perIter)
			schemaResp, schemaErr := cfg.Provider.Generate(finalCtx, llm.GenerateRequest{
				Model:          cfg.Model,
				Fallbacks:      cfg.Fallbacks,
				System:         cfg.SystemPrompt,
				Messages:       history,
				ResponseSchema: agentResponseSchema(),
				Feature:        cfg.Feature,
				MaxTokens:      2048,
				Temperature:    temp,
				BusinessID:     cfg.Env.BusinessID,
			})
			finalCancel()
			if schemaResp != nil {
				totalTokens += schemaResp.Usage.TotalTokens
			}
			if schemaErr != nil {
				cfg.Sink.Emit(SinkEvent{Type: "error", Payload: map[string]any{"error": schemaErr.Error()}})
				fireLoopFailureEscalation(cfg, schemaErr)
				err := fmt.Errorf("agent loop: structured repair: %w", schemaErr)
				if len(toolEvidence) > 0 {
					return loopResult(StructuredResponse{}, true), err
				}
				return nil, err
			}
			if schemaResp != nil && strings.TrimSpace(schemaResp.Text) != "" {
				finalText = strings.TrimSpace(schemaResp.Text)
			}
			parsed, parseErr := parseAgentJSON(finalText)
			if parseErr != nil {
				if len(toolEvidence) > 0 {
					return loopResult(StructuredResponse{}, true), parseErr
				}
				return nil, parseErr
			}
			return loopResult(parsed, true), nil
		}

		finalizationRequired = true
		calls := resp.ToolCalls
		for idx := range calls {
			if calls[idx].ID == "" {
				calls[idx].ID = fmt.Sprintf("call_%d_%d", i, idx)
			}
		}
		history = append(history, llm.Message{Role: llm.RoleAssistant, ToolCalls: calls})

		for _, fc := range calls {
			tool, ok := cfg.Registry.Get(fc.Name)
			args := fc.Args
			if args == nil {
				args = map[string]any{}
			}
			if !ok {
				history = append(history, llm.Message{
					Role: llm.RoleTool, ToolCallID: fc.ID, ToolName: fc.Name,
					Text: `{"error":"unknown_tool"}`,
				})
				continue
			}

			cfg.Sink.Emit(SinkEvent{
				Type: "tool.call.started",
				Payload: map[string]any{
					"name":        fc.Name,
					"human_label": tool.HumanLabel(pillLocale(cfg.Env.Locale)),
				},
			})

			callStarted := time.Now()
			result, runErr := tool.Run(loopCtx, args, cfg.Env)
			durationMs := int(time.Since(callStarted).Milliseconds())
			if runErr == nil {
				toolEvidence = append(toolEvidence, ToolEvidence{
					Name:    fc.Name,
					Summary: result.Summary,
					Data:    snapshotToolEvidenceData(result.Data),
				})
			}
			if attempted, sent := communicationToolResult(fc.Name, result, runErr); attempted {
				communicationSent = sent
			}

			// Deterministic escalation: a successful escalate/lead tool also
			// raises a durable escalation (row + Telegram/email) so the owner is
			// notified even if the model's prose forgets to mention it.
			// A tool refused by its abuse ceiling (rate_limited) stored nothing
			// and must not fan out an escalation either, or the lead-flood cap
			// would only move the flood from the leads table to Telegram/email.
			rateLimited, _ := result.Data["rate_limited"].(bool)
			if runErr == nil && !rateLimited && cfg.OnEscalate != nil && escalationToolNames[fc.Name] {
				// If the tool already delivered the admin email (email_sent=true in
				// its result), signal the Escalator to skip its own email so the
				// incident is not double-sent. A failed/absent inline email leaves
				// this false and the Escalator becomes the single fallback sender.
				emailAlreadySent, _ := result.Data["email_sent"].(bool)
				cfg.OnEscalate(EscalationEvent{
					Source:           cfg.Source,
					BusinessID:       cfg.Env.BusinessID,
					SessionRef:       cfg.Env.SessionID,
					ThreadID:         cfg.Env.ThreadID,
					Issue:            escalationIssueFromArgs(fc.Name, args),
					Transcript:       pii.Redact(strings.TrimSpace(argString(args, "transcript_summary"))),
					ContactEmail:     strings.TrimSpace(argString(args, "email")),
					Reason:           "tool",
					ToolName:         fc.Name,
					EmailAlreadySent: emailAlreadySent,
				})
			}

			if cfg.PersistToolCalls != nil {
				argsJSON, _ := json.Marshal(args)
				rec := &database.OpsAssistantToolCall{
					ThreadID:      cfg.Env.ThreadID,
					BusinessID:    cfg.Env.BusinessID,
					ToolName:      fc.Name,
					ArgumentsJSON: string(argsJSON),
					ResultSummary: result.Summary,
					Status:        "completed",
				}
				if runErr != nil {
					rec.Status = "error"
					rec.ResultSummary = runErr.Error()
				}
				if saveErr := cfg.PersistToolCalls(rec); saveErr == nil {
					toolCallIDs = append(toolCallIDs, rec.ID)
				}
			}

			cfg.Sink.Emit(SinkEvent{
				Type: "tool.call.completed",
				Payload: map[string]any{
					"name": fc.Name, "summary": result.Summary, "duration_ms": durationMs,
				},
			})

			payload := map[string]any{"summary": result.Summary, "data": result.Data}
			if runErr != nil {
				payload = map[string]any{"error": runErr.Error()}
			}
			b, _ := json.Marshal(payload)
			history = append(history, llm.Message{
				Role: llm.RoleTool, ToolCallID: fc.ID, ToolName: fc.Name, Text: string(b),
			})
		}
	}
	err := fmt.Errorf("agent loop: max iterations exceeded")
	fireLoopFailureEscalation(cfg, err)
	if len(toolEvidence) > 0 {
		return loopResult(StructuredResponse{}, false), err
	}
	return nil, err
}

// fireLoopFailureEscalation raises an escalation when the loop fails hard so a
// broken bot still reaches the owner. Best-effort and nil-safe.
//
// context.Canceled is NOT a hard failure: it means the caller went away (guest
// closed the widget / client disconnected), so it is logged at info and never
// escalated. context.DeadlineExceeded keeps escalating — the loop's own
// wall-clock budget blowing is a real failure worth an owner ping.
//
// The user prompt doubles as the escalation transcript and is redacted with the
// same pii.Redact used when ops persists the message.
func fireLoopFailureEscalation(cfg LoopConfig, cause error) {
	if cfg.OnEscalate == nil {
		return
	}
	if errors.Is(cause, context.Canceled) {
		logger.Logger.Infof("agent loop: canceled by caller (source=%s business=%d), skipping escalation: %v",
			cfg.Source, cfg.Env.BusinessID, cause)
		return
	}
	cfg.OnEscalate(EscalationEvent{
		Source:     cfg.Source,
		BusinessID: cfg.Env.BusinessID,
		SessionRef: cfg.Env.SessionID,
		ThreadID:   cfg.Env.ThreadID,
		Issue:      "Assistant failed to answer: " + cause.Error(),
		Transcript: pii.Redact(cfg.UserPrompt),
		Reason:     "loop_failure",
	})
}

// argString reads a string arg, tolerating non-string values.
func argString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// escalationIssueFromArgs builds a non-empty Issue for lead/escalation tools.
// capture_lead / escalate_to_sales schemas use name+intent, not "issue" — without
// this fallback every lead handoff wrote escalations with a blank issue.
func escalationIssueFromArgs(toolName string, args map[string]any) string {
	if issue := strings.TrimSpace(argString(args, "issue")); issue != "" {
		return issue
	}
	name := strings.TrimSpace(argString(args, "name"))
	intent := strings.TrimSpace(argString(args, "intent"))
	switch {
	case name != "" && intent != "":
		return name + " — " + intent
	case name != "":
		return name
	case intent != "":
		return intent
	default:
		return "Handoff via " + toolName
	}
}

func communicationToolResult(toolName string, result ToolResult, runErr error) (bool, bool) {
	switch toolName {
	case "capture_lead", "escalate_to_sales":
		sent, _ := result.Data["email_sent"].(bool)
		return true, runErr == nil && sent
	case "submit_contact_message":
		ok, _ := result.Data["ok"].(bool)
		return true, runErr == nil && ok
	default:
		return false, false
	}
}
