package providers

import (
	"context"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

type countingFactory struct {
	mu     sync.Mutex
	builds int
}

func (f *countingFactory) Build(_ context.Context, _ *database.BusinessFiscalSettings) (fiscal.Provider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds++
	return stubProvider{}, nil
}

func (f *countingFactory) Builds() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.builds
}

// stubProvider satisfies fiscal.Provider without implementing every method; the
// zero-value embed delegates each method to the nil interface, which is fine for
// tests that never call those methods.
type stubProvider struct{ fiscal.Provider }

func TestCachingFactoryReusesProviderAcrossJobs(t *testing.T) {
	inner := &countingFactory{}
	cf := NewCachingFactory(inner)
	settings := &database.BusinessFiscalSettings{
		ID:                     7,
		CredentialsFingerprint: "fp-abc",
	}
	for i := 0; i < 25; i++ {
		if _, err := cf.Build(context.Background(), settings); err != nil {
			t.Fatalf("Build #%d: %v", i, err)
		}
	}
	if inner.Builds() != 1 {
		t.Fatalf("expected 1 inner build across 25 jobs, got %d", inner.Builds())
	}
}

func TestCachingFactoryRebuildsOnFingerprintChange(t *testing.T) {
	inner := &countingFactory{}
	cf := NewCachingFactory(inner)
	s := &database.BusinessFiscalSettings{ID: 7, CredentialsFingerprint: "fp-1"}
	_, _ = cf.Build(context.Background(), s)
	s.CredentialsFingerprint = "fp-2"
	_, _ = cf.Build(context.Background(), s)
	if inner.Builds() != 2 {
		t.Fatalf("expected rebuild on fingerprint change, got %d builds", inner.Builds())
	}
}

func TestCachingFactoryDifferentSettingsIDs(t *testing.T) {
	inner := &countingFactory{}
	cf := NewCachingFactory(inner)
	for id := uint(1); id <= 5; id++ {
		s := &database.BusinessFiscalSettings{ID: id, CredentialsFingerprint: "same-fp"}
		_, _ = cf.Build(context.Background(), s)
	}
	if inner.Builds() != 5 {
		t.Fatalf("expected 5 builds (one per settingsID), got %d", inner.Builds())
	}
	// Second round: all should be cached.
	for id := uint(1); id <= 5; id++ {
		s := &database.BusinessFiscalSettings{ID: id, CredentialsFingerprint: "same-fp"}
		_, _ = cf.Build(context.Background(), s)
	}
	if inner.Builds() != 5 {
		t.Fatalf("expected no new builds on second round, got %d total", inner.Builds())
	}
}

// TestCachingFactoryEmptyFingerprintNotCached ensures rows without a fingerprint
// (legacy/unconfigured) always delegate to the inner factory so stale creds are
// never served.
func TestCachingFactoryEmptyFingerprintNotCached(t *testing.T) {
	inner := &countingFactory{}
	cf := NewCachingFactory(inner)
	s := &database.BusinessFiscalSettings{ID: 1, CredentialsFingerprint: ""}
	for i := 0; i < 3; i++ {
		if _, err := cf.Build(context.Background(), s); err != nil {
			t.Fatalf("Build #%d: %v", i, err)
		}
	}
	if inner.Builds() != 3 {
		t.Fatalf("expected 3 builds for empty fingerprint (no caching), got %d", inner.Builds())
	}
}

func TestCachingFactoryNilSettings(t *testing.T) {
	cf := NewCachingFactory(&countingFactory{})
	_, err := cf.Build(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil settings, got nil")
	}
}

// TestCachingFactoryConcurrentBuilds confirms no data race when many goroutines
// call Build for the same (settingsID, fingerprint) simultaneously.
func TestCachingFactoryConcurrentBuilds(t *testing.T) {
	inner := &countingFactory{}
	cf := NewCachingFactory(inner)
	s := &database.BusinessFiscalSettings{ID: 42, CredentialsFingerprint: "fp-concurrent"}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, err := cf.Build(context.Background(), s); err != nil {
				t.Errorf("concurrent Build: %v", err)
			}
		}()
	}
	wg.Wait()
	// Under race detection, 1 or a small number of builds is acceptable due to
	// double-checked-locking: goroutines that both pass the read-lock check before
	// the winner stores may each call inner.Build. What must NOT happen is a race.
	got := inner.Builds()
	if got == 0 || got > goroutines {
		t.Fatalf("unexpected build count %d (want 1..%d)", got, goroutines)
	}
}

func BenchmarkCachingFactoryBuild(b *testing.B) {
	cf := NewCachingFactory(&countingFactory{})
	s := &database.BusinessFiscalSettings{ID: 1, CredentialsFingerprint: "fp"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = cf.Build(context.Background(), s)
	}
}
