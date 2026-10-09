package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

type capturedRequest struct {
	path    string
	headers http.Header
	body    map[string]any
}

func newCompatServer(t *testing.T) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.headers = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","model":"llama3.1:8b","choices":[{"finish_reason":"stop","message":{"content":"hola"}}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func compatRequest() llm.GenerateRequest {
	return llm.GenerateRequest{
		Model:        "llama3.1:8b",
		Fallbacks:    []string{"qwen2.5:7b"},
		System:       "You are a waiter.",
		CacheControl: true,
		Messages:     []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
		Feature:      "waiter", // RequireZDR=true on OpenRouter
		ImageConfig:  &llm.ImageConfig{AspectRatio: "1:1"},
	}
}

func TestNewRequiresKeyOnlyForOpenRouter(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("OpenRouter without a key must error")
	}
	if _, err := New(Config{OpenAICompatible: true}); err == nil {
		t.Fatal("OpenAI-compatible mode without a base URL must error")
	}
	if _, err := New(Config{OpenAICompatible: true, BaseURL: "http://ollama:11434/v1"}); err != nil {
		t.Fatalf("OpenAI-compatible mode must not require a key: %v", err)
	}
}

func TestCompatModeUsesBaseURLAndOmitsOpenRouterOnlyFields(t *testing.T) {
	srv, got := newCompatServer(t)
	p, err := New(Config{BaseURL: srv.URL + "/v1/", OpenAICompatible: true, Referer: "https://pos.example.com", Title: "Bistro"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := p.Generate(context.Background(), compatRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "hola" || resp.Model != "llama3.1:8b" {
		t.Fatalf("resp = %+v", resp)
	}
	if got.path != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", got.path)
	}
	if h := got.headers.Get("Authorization"); h != "" {
		t.Fatalf("keyless compat mode must not send Authorization, got %q", h)
	}
	if got.headers.Get("HTTP-Referer") != "" || got.headers.Get("X-Title") != "" {
		t.Fatalf("OpenRouter ranking headers leaked to a third-party endpoint: %v", got.headers)
	}
	for _, field := range []string{"models", "provider", "image_config"} {
		if _, ok := got.body[field]; ok {
			t.Fatalf("compat body must omit %q: %v", field, got.body)
		}
	}
	msgs := got.body["messages"].([]any)
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" {
		t.Fatalf("first message = %v", sys)
	}
	if _, isString := sys["content"].(string); !isString {
		t.Fatalf("compat system content must be a plain string (no cache_control parts), got %T", sys["content"])
	}
}

func TestCompatModeSendsBearerWhenKeySet(t *testing.T) {
	srv, got := newCompatServer(t)
	p, err := New(Config{BaseURL: srv.URL, APIKey: "litellm-key", OpenAICompatible: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Generate(context.Background(), compatRequest()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if h := got.headers.Get("Authorization"); h != "Bearer litellm-key" {
		t.Fatalf("Authorization = %q", h)
	}
}

func TestOpenRouterModeKeepsDialect(t *testing.T) {
	srv, got := newCompatServer(t)
	p, err := New(Config{BaseURL: srv.URL, APIKey: "sk-or", Referer: "https://pos.example.com", Title: "Bistro"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Generate(context.Background(), compatRequest()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.headers.Get("Authorization") != "Bearer sk-or" || got.headers.Get("HTTP-Referer") != "https://pos.example.com" || got.headers.Get("X-Title") != "Bistro" {
		t.Fatalf("headers = %v", got.headers)
	}
	for _, field := range []string{"models", "provider", "image_config"} {
		if _, ok := got.body[field]; !ok {
			t.Fatalf("OpenRouter body must keep %q: %v", field, got.body)
		}
	}
}

// Hosted vendors echo a dated snapshot of the requested model. The budget
// ledger must price it as the requested (priced) id, not latch the
// unpriced-served-model shutdown; any other served id stays verbatim.
func TestCompatReportedModelBillsSnapshotsAsRequested(t *testing.T) {
	cases := []struct{ requested, served, want string }{
		{"gpt-4o-mini", "gpt-4o-mini-2024-07-18", "gpt-4o-mini"},
		{"claude-3-5-sonnet", "claude-3-5-sonnet-20241022", "claude-3-5-sonnet"},
		{"mistral-large", "mistral-large-2411", "mistral-large"},
		{"llama3.1:8b", "llama3.1:8b", "llama3.1:8b"},
		{"llama3.1:8b", "", ""},
		// Not a snapshot of the requested id: kept, so it fails closed.
		{"gpt-4o", "gpt-4o-mini-2024-07-18", "gpt-4o-mini-2024-07-18"},
		{"gpt-4o-mini", "gpt-4o", "gpt-4o"},
		{"gpt-4o", "gpt-4o-audio-preview", "gpt-4o-audio-preview"},
		{"", "gpt-4o-2024-08-06", "gpt-4o-2024-08-06"},
	}
	for _, c := range cases {
		if got := compatReportedModel(c.requested, c.served); got != c.want {
			t.Errorf("compatReportedModel(%q, %q) = %q, want %q", c.requested, c.served, got, c.want)
		}
	}
}

func TestCompatModeReportsRequestedModelForVendorSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-2","model":"gpt-4o-mini-2024-07-18","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
	}))
	t.Cleanup(srv.Close)
	req := llm.GenerateRequest{Model: "gpt-4o-mini", Feature: "waiter", Messages: []llm.Message{{Role: llm.RoleUser, Text: "hi"}}}

	compat, err := New(Config{BaseURL: srv.URL + "/v1", OpenAICompatible: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := compat.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Fatalf("compat served model = %q, want the requested id", resp.Model)
	}

	// OpenRouter mode keeps the served id verbatim (its own pricing contract).
	or, err := New(Config{APIKey: "k", BaseURL: srv.URL + "/v1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err = or.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "gpt-4o-mini-2024-07-18" {
		t.Fatalf("OpenRouter served model = %q, want verbatim", resp.Model)
	}
}
