package services

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchLatestRatesStampedeAndCooldown(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(20 * time.Millisecond)
		http.Error(w, "upstream down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	svc := NewExchangeRateService(nil)
	svc.coinbaseURL = srv.URL

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = svc.FetchLatestRates()
		}(i)
	}
	close(start)
	wg.Wait()

	if got := hits.Load(); got != 1 {
		t.Fatalf("concurrent FetchLatestRates hits = %d, want 1", got)
	}
	for i, err := range errs {
		if err == nil {
			t.Fatalf("concurrent call %d returned nil error", i)
		}
	}

	for i := 0; i < 10; i++ {
		err := svc.FetchLatestRates()
		if !errors.Is(err, ErrExchangeRateCooldown) {
			t.Fatalf("sequential call %d error = %v, want ErrExchangeRateCooldown", i, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("cooldown calls hit upstream, hits = %d", got)
	}

	svc.lastFailedNano.Store(time.Now().Add(-time.Hour).UnixNano())
	if err := svc.FetchLatestRates(); err == nil {
		t.Fatal("expected upstream error after cooldown elapsed")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("post-cooldown hits = %d, want 2", got)
	}
}

func TestExchangeRateZeroCooldownDoesNotSuppressFetch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "upstream down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	svc := &ExchangeRateService{
		coinbaseURL: srv.URL,
		httpClient:  &http.Client{Timeout: time.Second},
	}
	svc.lastFailedNano.Store(time.Now().UnixNano())

	err := svc.FetchLatestRates()
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if errors.Is(err, ErrExchangeRateCooldown) {
		t.Fatal("zero failureCooldown must not cool down")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}
