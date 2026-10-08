package database

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLocation_CachesPointer(t *testing.T) {
	first := ResolveLocation("America/Argentina/Buenos_Aires")
	second := ResolveLocation("America/Argentina/Buenos_Aires")
	assert.Same(t, first, second, "second lookup must reuse the cached *time.Location")
}

func TestResolveLocation_UnknownNegativeCachedAsUTC(t *testing.T) {
	loc := ResolveLocation("Not/ARealZone")
	assert.Equal(t, time.UTC, loc)
	cached, ok := businessLocationCache.Load("Not/ARealZone")
	require.True(t, ok, "unknown zone must be negative-cached")
	assert.Equal(t, time.UTC, cached.(*time.Location))
}

// TestResolveLocation_NegativeCacheIsBounded guards the memory side of the
// decision-14 cache. Business.Timezone is a free-text operator-writable column,
// so every distinct junk value that reaches ResolveLocation would otherwise
// pin one map entry for the life of the process with nothing to evict it. The
// cache must stop growing at the cap while still resolving correctly — an
// uncached lookup is slower, never wrong.
func TestResolveLocation_NegativeCacheIsBounded(t *testing.T) {
	businessLocationCache = sync.Map{}
	atomic.StoreInt64(&businessLocationCacheLen, 0)
	t.Cleanup(func() {
		businessLocationCache = sync.Map{}
		atomic.StoreInt64(&businessLocationCacheLen, 0)
	})

	for i := 0; i < maxCachedLocations*3; i++ {
		assert.Equal(t, time.UTC, ResolveLocation(fmt.Sprintf("Junk/Zone-%d", i)))
	}

	stored := 0
	businessLocationCache.Range(func(_, _ any) bool {
		stored++
		return true
	})
	assert.LessOrEqual(t, stored, maxCachedLocations,
		"cache must stop admitting entries at the cap")

	// Correctness survives a full cache: real zones still resolve.
	assert.Equal(t, "Asia/Dubai", ResolveLocation("Asia/Dubai").String())
}

func TestResolveBusinessLocation_EmptyFallsBackToUTC(t *testing.T) {
	assert.Equal(t, time.UTC, ResolveBusinessLocation(nil))
	assert.Equal(t, time.UTC, ResolveBusinessLocation(&Business{Timezone: ""}))
}

// TestLocalizedTimestampExpr_CachesSQL is the decision-14 access-shape gate for
// timezone-conversion caching: building the SQLite/Postgres localized
// timestamp expression must not re-allocate a new SQL string on every call
// for the same (dialect, column, zone, offset).
func TestLocalizedTimestampExpr_CachesSQL(t *testing.T) {
	loc := ResolveLocation("America/Argentina/Buenos_Aires")
	require.NotEqual(t, time.UTC, loc)

	a := LocalizedTimestampExpr("sqlite", "created_at", loc)
	b := LocalizedTimestampExpr("sqlite", "created_at", loc)
	assert.Equal(t, a, b)
	// Pointer identity of the cached string header is not guaranteed after
	// interning, so assert content + that a second build hits the cache map.
	_, offset := time.Now().In(loc).Zone()
	key := localizedExprCacheKey("sqlite", "created_at", loc.String(), offset)
	cached, ok := localizedSQLCache.Load(key)
	require.True(t, ok, "expression must be stored in localizedSQLCache")
	assert.Equal(t, a, cached.(string))
	assert.Contains(t, a, "datetime(created_at")
	assert.True(t, strings.Contains(a, "seconds") || strings.Contains(a, "second"),
		"sqlite expr should shift by fixed offset: %s", a)

	// UTC / nil must stay the bare column (byte-identical hot path).
	assert.Equal(t, "created_at", LocalizedTimestampExpr("sqlite", "created_at", time.UTC))
	assert.Equal(t, "created_at", LocalizedTimestampExpr("postgres", "created_at", nil))
}

func TestLocalizedTimestampExpr_PostgresUsesATTimeZone(t *testing.T) {
	loc := ResolveLocation("Asia/Dubai")
	expr := LocalizedTimestampExpr("postgres", "created_at", loc)
	assert.Contains(t, expr, "AT TIME ZONE")
	assert.Contains(t, expr, "Asia/Dubai")
}
