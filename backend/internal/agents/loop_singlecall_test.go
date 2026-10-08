package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// countingProvider records how many times Generate is called and returns queued
// responses in order (last response repeats once exhausted).
type countingProvider struct {
	steps []*llm.Response
	calls int
}

func (p *countingProvider) Generate(context.Context, llm.GenerateRequest) (*llm.Response, error) {
	p.calls++
	idx := p.calls - 1
	if idx >= len(p.steps) {
		idx = len(p.steps) - 1
	}
	return p.steps[idx], nil
}

func runToolFreeLoop(t *testing.T, provider llm.Provider) *LoopResult {
	t.Helper()
	res, err := RunAgentLoop(context.Background(), LoopConfig{
		Registry:     NewRegistry(),
		Provider:     provider,
		Model:        "test",
		SystemPrompt: "sys",
		UserPrompt:   "hi",
	})
	if err != nil {
		t.Fatalf("loop err: %v", err)
	}
	return res
}

// A tool-free response that is already valid schema JSON must be used as-is with
// exactly ONE model call (no schema-reformat round trip).
func TestToolFreeStructuredJSONSingleCall(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: `{"answer":"Bills live under the Serve tab.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 1 {
		t.Fatalf("want 1 provider call, got %d", p.calls)
	}
	if res.Final.Answer != "Bills live under the Serve tab." {
		t.Fatalf("unexpected answer: %q", res.Final.Answer)
	}
}

// A tool-free plain-text response must be wrapped without a second model call.
func TestToolFreePlainTextWrapsSingleCall(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: "You can reset your PIN from the Team tab."},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 1 {
		t.Fatalf("want 1 provider call (plain-text wrap), got %d", p.calls)
	}
	if res.Final.Answer != "You can reset your PIN from the Team tab." {
		t.Fatalf("plain text not wrapped: %q", res.Final.Answer)
	}
	if res.Final.Steps == nil || res.Final.Actions == nil || res.Final.FollowUps == nil {
		t.Fatalf("wrapped response left nil slices: %+v", res.Final)
	}
}

// A fenced-JSON first response is also used directly (single call).
func TestToolFreeFencedJSONSingleCall(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: "```json\n{\"answer\":\"Fenced answer\",\"steps\":[],\"actions\":[],\"follow_ups\":[]}\n```"},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 1 {
		t.Fatalf("want 1 provider call for fenced JSON, got %d", p.calls)
	}
	if res.Final.Answer != "Fenced answer" {
		t.Fatalf("unexpected answer: %q", res.Final.Answer)
	}
}

// A malformed JSON attempt (leading brace, unparseable) must fall back to the
// schema-reformat call (two model calls) and use the repaired output.
func TestToolFreeMalformedFallsBackToSchemaCall(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: `{"answer":"broken",`}, // malformed JSON -> triggers fallback
		{Text: `{"answer":"Repaired answer.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 2 {
		t.Fatalf("want 2 provider calls (malformed -> schema reformat), got %d", p.calls)
	}
	if res.Final.Answer != "Repaired answer." {
		t.Fatalf("unexpected answer after fallback: %q", res.Final.Answer)
	}
}

// Gemini can occasionally emit the requested response shape as YAML-like text
// after a prose answer. That is an attempted structured response, not prose:
// it must take the schema-repair path so action metadata never reaches chat.
func TestToolFreeYAMLLikeMetadataFallsBackToSchemaCall(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: "Payverge ofrece dos planes principales.\n\nactions:\n\nkind: navigate\nlabel: Ver precios path: /pricing follow_ups:\n\n¿Qué incluye el plan Operaciones?"},
		{Text: `{"answer":"Payverge ofrece dos planes principales.","steps":[],"actions":[{"label":"Ver precios","href":"/pricing","kind":"navigate"}],"follow_ups":["¿Qué incluye el plan Operaciones?"]}`},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 2 {
		t.Fatalf("want 2 provider calls (YAML-like metadata -> schema reformat), got %d", p.calls)
	}
	if strings.Contains(res.Final.Answer, "actions:") || strings.Contains(res.Final.Answer, "follow_ups:") {
		t.Fatalf("structured metadata leaked into answer: %q", res.Final.Answer)
	}
	if len(res.Final.Actions) != 1 || res.Final.Actions[0].Href != "/pricing" {
		t.Fatalf("repaired action missing: %#v", res.Final.Actions)
	}
	if len(res.Final.FollowUps) != 1 {
		t.Fatalf("repaired follow-up missing: %#v", res.Final.FollowUps)
	}
}

// An empty first response must also fall back to the schema-reformat call.
func TestToolFreeEmptyFallsBackToSchemaCall(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: "   "}, // empty/whitespace -> fallback
		{Text: `{"answer":"Recovered answer.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 2 {
		t.Fatalf("want 2 provider calls (empty -> schema reformat), got %d", p.calls)
	}
	if res.Final.Answer != "Recovered answer." {
		t.Fatalf("unexpected answer after fallback: %q", res.Final.Answer)
	}
}

// Prose followed by a fenced JSON copy of the structured payload (a common
// model failure seen in prod) must surface the PARSED payload — never the raw
// fenced JSON as chat text — and still take a single model call.
func TestToolFreeProseWithTrailingFencedJSON(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: "¡Claro que sí! Te ayudamos a migrar.\n\n```json\n{\"answer\":\"¡Claro que sí! Te ayudamos a migrar.\",\"steps\":[],\"actions\":[],\"follow_ups\":[\"Quiero ver una demo\"]}\n```"},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 1 {
		t.Fatalf("want 1 provider call, got %d", p.calls)
	}
	if res.Final.Answer != "¡Claro que sí! Te ayudamos a migrar." {
		t.Fatalf("unexpected answer: %q", res.Final.Answer)
	}
	if strings.Contains(res.Final.Answer, "```") || strings.Contains(res.Final.Answer, "{") {
		t.Fatalf("raw JSON leaked into answer: %q", res.Final.Answer)
	}
	if len(res.Final.FollowUps) != 1 {
		t.Fatalf("structured payload not used: %+v", res.Final)
	}
}

// Prose with an unparseable fenced JSON block keeps the prose and strips the
// fence, so raw braces never reach the user.
func TestToolFreeProseWithMalformedFenceStripsFence(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: "Here is how migration works.\n\n```json\n{\"answer\": broken\n```"},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 1 {
		t.Fatalf("want 1 provider call, got %d", p.calls)
	}
	if res.Final.Answer != "Here is how migration works." {
		t.Fatalf("unexpected answer: %q", res.Final.Answer)
	}
}

// A tool-free structured response that carries actions must reach LoopResult
// with those actions intact — coerceAgentResponse / parseAgentJSON must not drop
// them on the single-call fast path.
func TestToolFreeStructuredJSONPreservesActions(t *testing.T) {
	p := &countingProvider{steps: []*llm.Response{
		{Text: `{"answer":"Open the menu.","steps":[],"actions":[{"label":"Edit menu","href":"/business/2/dashboard?tab=menu","kind":"navigate","disabled":false}],"follow_ups":[]}`},
	}}
	res := runToolFreeLoop(t, p)
	if p.calls != 1 {
		t.Fatalf("want 1 provider call, got %d", p.calls)
	}
	if len(res.Final.Actions) != 1 {
		t.Fatalf("actions dropped: %#v", res.Final.Actions)
	}
	if res.Final.Actions[0].Href != "/business/2/dashboard?tab=menu" {
		t.Fatalf("action href=%q", res.Final.Actions[0].Href)
	}
	if res.Final.Actions[0].Kind != "navigate" {
		t.Fatalf("action kind=%q", res.Final.Actions[0].Kind)
	}
}
