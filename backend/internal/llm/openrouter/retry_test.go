package openrouter

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func installFakeRetry(p *Provider) (*int32, *[]time.Duration) {
	var sleeps int32
	durs := &[]time.Duration{}
	p.sleep = func(ctx context.Context, d time.Duration) error {
		atomic.AddInt32(&sleeps, 1)
		*durs = append(*durs, d)
		return nil
	}
	p.randFloat = func() float64 { return 1.0 }
	return &sleeps, durs
}

func TestRetryRateLimitedThenSuccess(t *testing.T) {
	var calls int32
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	sleeps, _ := installFakeRetry(p)
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("text=%q", resp.Text)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("http calls=%d, want 2", calls)
	}
	if atomic.LoadInt32(sleeps) != 1 {
		t.Fatalf("sleeps=%d, want 1", *sleeps)
	}
}

func TestRetryUpstream5xxExhausts(t *testing.T) {
	var calls int32
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	sleeps, durs := installFakeRetry(p)
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrUpstream) {
		t.Fatalf("err=%v, want ErrUpstream", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("http calls=%d, want 3", calls)
	}
	if atomic.LoadInt32(sleeps) != 2 {
		t.Fatalf("sleeps=%d, want 2", *sleeps)
	}
	want := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond}
	for i, d := range want {
		if (*durs)[i] != d {
			t.Fatalf("sleep[%d]=%v, want %v", i, (*durs)[i], d)
		}
	}
}

func TestRetryAuthNeverRetries(t *testing.T) {
	var calls int32
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"no key"}}`))
	})
	sleeps, _ := installFakeRetry(p)
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrAuth) {
		t.Fatalf("err=%v, want ErrAuth", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("http calls=%d, want 1 (no retry on auth)", calls)
	}
	if atomic.LoadInt32(sleeps) != 0 {
		t.Fatalf("sleeps=%d, want 0", *sleeps)
	}
}

func TestRetry4xxNeverRetries(t *testing.T) {
	var calls int32
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad request"}}`))
	})
	sleeps, _ := installFakeRetry(p)
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrUpstream) {
		t.Fatalf("err=%v, want ErrUpstream", err)
	}
	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("expected 400 APIError, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("http calls=%d, want 1 (no retry on 4xx)", calls)
	}
	if atomic.LoadInt32(sleeps) != 0 {
		t.Fatalf("sleeps=%d, want 0", *sleeps)
	}
}

func TestRetryHonorsContextDeadline(t *testing.T) {
	var calls int32
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	var sleeps int32
	p.sleep = func(ctx context.Context, d time.Duration) error {
		atomic.AddInt32(&sleeps, 1)
		return context.DeadlineExceeded
	}
	p.randFloat = func() float64 { return 1.0 }
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrUpstream) {
		t.Fatalf("err=%v, want ErrUpstream (last attempt's error)", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("http calls=%d, want 1 (ctx cancelled before retry)", calls)
	}
	if atomic.LoadInt32(&sleeps) != 1 {
		t.Fatalf("sleeps=%d, want 1", sleeps)
	}
}

type failingFinalizeBudget struct {
	finalizeErr error
}

func (b *failingFinalizeBudget) ReserveForRequest(context.Context, llm.GenerateRequest) (string, bool, error) {
	return "reservation-1", true, nil
}

func (b *failingFinalizeBudget) FinalizeRequest(context.Context, string, *llm.Response, string) error {
	return b.finalizeErr
}

func (b *failingFinalizeBudget) ReleaseRequest(context.Context, string) error { return nil }

func TestGenerate_SurfacesFinalizeFailureAfterProviderSpend(t *testing.T) {
	want := errors.New("ledger unavailable")
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"google/gemini-2.5-flash","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	})
	p.budget = &failingFinalizeBudget{finalizeErr: want}

	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature: "test",
		Model:   "google/gemini-2.5-flash", BusinessID: 7,
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hello"}},
	})
	if !errors.Is(err, want) {
		t.Fatalf("Generate error = %v, want finalize error", err)
	}
	if resp == nil || resp.Text != "ok" {
		t.Fatalf("provider response should remain available for telemetry/reconciliation: %+v", resp)
	}
}

func TestBudgetAccountingContextSurvivesRequestCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	accountingCtx, cancelAccounting := budgetAccountingContext(requestCtx)
	defer cancelAccounting()

	if err := accountingCtx.Err(); err != nil {
		t.Fatalf("accounting context inherited request cancellation: %v", err)
	}
	if _, ok := accountingCtx.Deadline(); !ok {
		t.Fatal("detached accounting context must remain bounded by a deadline")
	}
}
