package services

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTranslateMenuItemSkipsUnchangedStrings is the §3.7 fix-1 regression: the
// job/batch translate path must NOT re-call the external translate API for a
// (entity, field, lang) whose stored translation already matches the current
// source text (OriginalText unchanged). At 1,200 items this is the difference
// between thousands of external calls per sync and near-zero on a no-op re-sync.
func TestTranslateMenuItemSkipsUnchangedStrings(t *testing.T) {
	setupTranslationServiceTestDB(t)

	var providerCalls int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&providerCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"traducido"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	const bizID = uint(42)
	const itemID = uint(7)

	// First pass: no existing translation → the API is called for name + desc.
	require.NoError(t, service.TranslateMenuItemToLanguages(bizID, itemID, "Burger", "Beef patty", []string{"es"}))
	firstPass := atomic.LoadInt64(&providerCalls)
	assert.Equal(t, int64(2), firstPass, "first pass translates name + description")

	// Second pass with the SAME source text: both rows already exist with matching
	// OriginalText, so no external call must be made.
	require.NoError(t, service.TranslateMenuItemToLanguages(bizID, itemID, "Burger", "Beef patty", []string{"es"}))
	assert.Equal(t, firstPass, atomic.LoadInt64(&providerCalls),
		"unchanged strings must not re-hit the external translate API")

	// Third pass with CHANGED name: only the name re-translates (desc still skips).
	require.NoError(t, service.TranslateMenuItemToLanguages(bizID, itemID, "Cheeseburger", "Beef patty", []string{"es"}))
	assert.Equal(t, firstPass+1, atomic.LoadInt64(&providerCalls),
		"a changed source string must re-translate; the unchanged one must still skip")
}

// TestTranslateCategorySkipsUnchangedStrings is the category-path equivalent.
func TestTranslateCategorySkipsUnchangedStrings(t *testing.T) {
	setupTranslationServiceTestDB(t)

	var providerCalls int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&providerCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"traducido"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	const bizID = uint(43)
	const catID = uint(3)

	require.NoError(t, service.TranslateCategoryToLanguages(bizID, catID, "Mains", "Hearty plates", []string{"es"}))
	firstPass := atomic.LoadInt64(&providerCalls)
	assert.Equal(t, int64(2), firstPass)

	require.NoError(t, service.TranslateCategoryToLanguages(bizID, catID, "Mains", "Hearty plates", []string{"es"}))
	assert.Equal(t, firstPass, atomic.LoadInt64(&providerCalls),
		"unchanged category strings must not re-hit the external translate API")
}

// BenchmarkTranslateMenuItem_NoOpResync measures the cost of re-syncing an item
// whose translation already exists with matching source text. With the fix-1
// unchanged-string skip this is a DB freshness read per (field, lang) and NO
// external call — the dominant cost in a real re-sync of an unchanged 1,200-item
// menu. A provider is wired but must never be hit; a hit fails the bench.
func BenchmarkTranslateMenuItem_NoOpResync(b *testing.B) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(&database.Translation{}); err != nil {
		b.Fatal(err)
	}

	var providerCalls int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&providerCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"traducido"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	const bizID = uint(99)
	const itemID = uint(1)
	// Prime a fresh translation once (this legitimately hits the provider twice).
	if err := service.TranslateMenuItemToLanguages(bizID, itemID, "Burger", "Beef patty", []string{"es"}); err != nil {
		b.Fatal(err)
	}
	primed := atomic.LoadInt64(&providerCalls)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := service.TranslateMenuItemToLanguages(bizID, itemID, "Burger", "Beef patty", []string{"es"}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if got := atomic.LoadInt64(&providerCalls); got != primed {
		b.Fatalf("no-op re-sync hit the external translate API %d extra times", got-primed)
	}
}
