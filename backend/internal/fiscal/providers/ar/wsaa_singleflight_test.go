package ar

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestWSAAClient returns a WSAAClient with a real cert+key but an injected
// fetch function so tests never touch the network.
func newTestWSAAClient(t *testing.T) *WSAAClient {
	t.Helper()
	cert, key := newSelfSignedCert(t)
	c := NewWSAAClient("https://wsaa.test.afip.gov.ar/ws/services/LoginCms", cert, key)
	return c
}

// TestAuthenticateSingleFlight asserts a cache-miss storm triggers exactly one
// underlying SOAP fetch, not one per caller, and does not serialize callers
// behind the mutex for the full round-trip.
func TestAuthenticateSingleFlight(t *testing.T) {
	var fetches int32
	c := newTestWSAAClient(t)
	c.fetch = func(ctx context.Context) (*WSAACredentials, error) {
		atomic.AddInt32(&fetches, 1)
		time.Sleep(50 * time.Millisecond) // simulate slow SOAP
		return &WSAACredentials{ExpiresAt: time.Now().Add(time.Hour)}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = c.Authenticate(context.Background()) }()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&fetches); got != 1 {
		t.Fatalf("expected exactly 1 underlying fetch, got %d", got)
	}
}

// TestAuthenticateCallerCancelDoesNotFailCoalescedCaller proves a cancelled
// leader does not abort the shared login. The fetch blocks until release and
// fails if its own context is done. Caller 1 must return context.Canceled
// while the fetch is still in flight; caller 2 then receives the credentials,
// and the fetch runs once.
func TestAuthenticateCallerCancelDoesNotFailCoalescedCaller(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var fetches atomic.Int32
	var startedOnce sync.Once

	c := newTestWSAAClient(t)
	c.fetch = func(ctx context.Context) (*WSAACredentials, error) {
		fetches.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return &WSAACredentials{Token: "tok", Sign: "sig", ExpiresAt: time.Now().Add(time.Hour)}, nil
		}
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	err1Ch := make(chan error, 1)
	go func() {
		_, err := c.Authenticate(ctx1)
		err1Ch <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch did not start")
	}

	res2Ch := make(chan struct {
		creds *WSAACredentials
		err   error
	}, 1)
	go func() {
		creds, err := c.Authenticate(context.Background())
		res2Ch <- struct {
			creds *WSAACredentials
			err   error
		}{creds, err}
	}()

	cancel1()

	select {
	case err := <-err1Ch:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("caller 1: got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("caller 1 did not return promptly after cancel")
	}

	close(release)

	select {
	case got := <-res2Ch:
		if got.err != nil {
			t.Fatalf("caller 2: %v", got.err)
		}
		if got.creds == nil || got.creds.Token == "" || got.creds.Sign == "" {
			t.Fatalf("caller 2: expected valid credentials, got %+v", got.creds)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller 2 did not return")
	}

	if n := fetches.Load(); n != 1 {
		t.Fatalf("fetch ran %d times, want 1", n)
	}
}

// TestAuthenticateCachedTokenSkipsFetch proves a valid cached token returns
// immediately without any fetch invocation.
func TestAuthenticateCachedTokenSkipsFetch(t *testing.T) {
	var fetches int32
	c := newTestWSAAClient(t)
	c.fetch = func(ctx context.Context) (*WSAACredentials, error) {
		atomic.AddInt32(&fetches, 1)
		return &WSAACredentials{ExpiresAt: time.Now().Add(time.Hour)}, nil
	}

	// Warm the cache with one call.
	if _, err := c.Authenticate(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if got := atomic.LoadInt32(&fetches); got != 1 {
		t.Fatalf("expected 1 fetch to warm cache, got %d", got)
	}

	// Subsequent calls must return from cache without any fetch.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = c.Authenticate(context.Background()) }()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&fetches); got != 1 {
		t.Fatalf("expected cached token to skip fetch (still 1), got %d", got)
	}
}
