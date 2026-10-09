package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func BenchmarkGenerateSuccessPath(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"google/gemini-2.0-flash-001","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`))
	}))
	defer srv.Close()
	var observed int
	p, err := New(Config{APIKey: "k", BaseURL: srv.URL}, WithObserver(func(llm.CallInfo) { observed++ }))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	req := llm.GenerateRequest{
		Model:     "google/gemini-2.0-flash-001",
		Feature:   "waiter",
		MaxTokens: 1024,
		Messages:  []llm.Message{{Role: llm.RoleUser, Text: "hello"}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Generate(context.Background(), req); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
	b.StopTimer()
	if observed != b.N {
		b.Fatalf("observer fired %d times, want %d", observed, b.N)
	}
}
