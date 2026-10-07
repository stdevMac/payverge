package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestCacheControlBreakpointOnSystemMessage(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:      "test",
		Model:        "google/gemini-2.5-flash",
		System:       "STATIC PREFIX: you are a waiter at Fixture Bistro.",
		Messages:     []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
		CacheControl: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	msgs := gotBody["messages"].([]any)
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" {
		t.Fatalf("first message not system: %v", sys)
	}
	parts, ok := sys["content"].([]any)
	if !ok {
		t.Fatalf("system content not an array when CacheControl set: %T", sys["content"])
	}
	part := parts[0].(map[string]any)
	if part["type"] != "text" || part["text"] != "STATIC PREFIX: you are a waiter at Fixture Bistro." {
		t.Fatalf("system text part = %v", part)
	}
	cc, ok := part["cache_control"].(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("cache_control = %v", part["cache_control"])
	}
}

func TestNoCacheControlKeepsStringSystem(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.5-flash",
		System:   "plain system",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sys := gotBody["messages"].([]any)[0].(map[string]any)
	if _, isStr := sys["content"].(string); !isStr {
		t.Fatalf("system content should stay a plain string without CacheControl, got %T", sys["content"])
	}
}

func TestCachedTokensParsed(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"google/gemini-2.5-flash","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1200,"completion_tokens":40,"total_tokens":1240,"prompt_tokens_details":{"cached_tokens":1024}}}`))
	})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Usage.CachedTokens != 1024 {
		t.Fatalf("CachedTokens = %d, want 1024", resp.Usage.CachedTokens)
	}
	if resp.Usage.PromptTokens != 1200 {
		t.Fatalf("PromptTokens = %d, want 1200", resp.Usage.PromptTokens)
	}
}
