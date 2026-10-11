package openrouter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func newRawServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestObserverInvokedOnceOnSuccess(t *testing.T) {
	var infos []llm.CallInfo
	srv := newRawServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"openai/gpt-4o-mini","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`))
	})
	p, err := New(Config{APIKey: "k", BaseURL: srv.URL}, WithObserver(func(ci llm.CallInfo) {
		infos = append(infos, ci)
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = p.Generate(context.Background(), llm.GenerateRequest{
		Model:     "google/gemini-2.0-flash-001",
		Feature:   "waiter",
		Fallbacks: []string{"openai/gpt-4o-mini"},
		Messages:  []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("observer calls=%d, want 1", len(infos))
	}
	ci := infos[0]
	if ci.Feature != "waiter" || ci.Model != "google/gemini-2.0-flash-001" {
		t.Fatalf("feature/model = %q/%q", ci.Feature, ci.Model)
	}
	if ci.ServedModel != "openai/gpt-4o-mini" {
		t.Fatalf("served model = %q (want fallback)", ci.ServedModel)
	}
	if ci.InputTokens != 10 || ci.OutputTokens != 4 {
		t.Fatalf("tokens = %d/%d", ci.InputTokens, ci.OutputTokens)
	}
	if ci.Err != nil || ci.Latency <= 0 {
		t.Fatalf("err/latency = %v/%v", ci.Err, ci.Latency)
	}
}

func TestObserverInvokedOnceOnErrorAfterRetries(t *testing.T) {
	var calls int32
	var infos []llm.CallInfo
	srv := newRawServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	p, err := New(Config{APIKey: "k", BaseURL: srv.URL}, WithObserver(func(ci llm.CallInfo) {
		infos = append(infos, ci)
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	installFakeRetry(p)
	_, err = p.Generate(context.Background(), llm.GenerateRequest{
		Model:    "google/gemini-2.0-flash-001",
		Feature:  "director",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrUpstream) {
		t.Fatalf("err=%v, want ErrUpstream", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("http calls=%d, want 3", calls)
	}
	if len(infos) != 1 {
		t.Fatalf("observer calls=%d, want 1", len(infos))
	}
	if infos[0].Feature != "director" || !errors.Is(infos[0].Err, llm.ErrUpstream) {
		t.Fatalf("callinfo = %+v", infos[0])
	}
}

func TestNilObserverNeverPanics(t *testing.T) {
	srv := newRawServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	p, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}
