package openrouter

import (
	"context"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestObserverReceivesEstimatedCostAndBusinessID(t *testing.T) {
	var got llm.CallInfo
	srv := newRawServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"google/gemini-2.5-flash","choices":[{"message":{"content":"ok"}}],` +
			`"usage":{"prompt_tokens":1200000,"completion_tokens":200000,"total_tokens":1400000,` +
			`"prompt_tokens_details":{"cached_tokens":1024000}}}`))
	})
	p, err := New(Config{APIKey: "k", BaseURL: srv.URL}, WithObserver(func(ci llm.CallInfo) { got = ci }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = p.Generate(context.Background(), llm.GenerateRequest{
		Model:      "google/gemini-2.5-flash",
		Feature:    "waiter",
		BusinessID: 42,
		Messages:   []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.BusinessID != 42 {
		t.Fatalf("CallInfo.BusinessID = %d, want 42 (GenerateRequest.BusinessID not propagated in retry.go)", got.BusinessID)
	}
	if got.EstimatedCostUSD <= 0 {
		t.Fatalf("CallInfo.EstimatedCostUSD = %v, want > 0 (AnnotateCost not called before observer fired)", got.EstimatedCostUSD)
	}
	if got.EstimatedCostUSD < 0.62 || got.EstimatedCostUSD > 0.64 {
		t.Fatalf("CallInfo.EstimatedCostUSD = %v, want ~0.6296 (cache-discounted)", got.EstimatedCostUSD)
	}
}
