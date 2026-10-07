package database

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// businessLocationCache memoizes time.LoadLocation results (including negative
// lookups, which are cached as UTC) so repeated analytics/report/scheduler
// calls don't re-read tzdata files for every business on every request.
//
// Keyed by IANA zone string (not business ID): when a business changes its
// timezone the new name is a new cache key; the old entry is harmless reuse for
// any other business still on that zone. No per-business invalidation hook is
// required.
var businessLocationCache sync.Map // map[string]*time.Location

// businessLocationCacheLen counts admitted entries so the cache can stop
// growing. Business.Timezone is operator-writable free text, so without a cap
// every distinct junk value ever written would pin an entry for the lifetime of
// the process with nothing to evict it. The IANA database is comfortably under
// this cap, so a real deployment never reaches it; past the cap lookups simply
// resolve uncached — slower, never wrong.
var businessLocationCacheLen int64

const maxCachedLocations = 1024

// storeLocation admits an entry only while the cache is under its cap.
func storeLocation(tz string, loc *time.Location) {
	if atomic.LoadInt64(&businessLocationCacheLen) >= maxCachedLocations {
		return
	}
	if _, loaded := businessLocationCache.LoadOrStore(tz, loc); !loaded {
		atomic.AddInt64(&businessLocationCacheLen, 1)
	}
}

// localizedSQLCache memoizes dialect-specific localized timestamp SQL fragments
// so hot analytics/AI GROUP BY paths do not rebuild fmt.Sprintf strings and
// re-sample the zone offset on every request. Key includes the current UTC
// offset so DST transitions naturally produce a new entry.
var localizedSQLCache sync.Map // map[string]string

// ResolveLocation returns the *time.Location for an IANA timezone string,
// falling back to time.UTC when the value is empty or unrecognised. Unknown
// zones are negative-cached to UTC so an invalid value doesn't retry-thrash the
// tzdata loader.
//
// This is the single source of truth for turning a stored Business.Timezone
// into a *time.Location across the analytics, services, and handlers packages,
// so day/period/bucket boundaries are computed in the business's own calendar
// rather than the server clock. (handlers keeps a thin local wrapper for the
// dashboard "today" path.)
func ResolveLocation(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	if cached, ok := businessLocationCache.Load(tz); ok {
		return cached.(*time.Location)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		storeLocation(tz, time.UTC) // negative cache
		return time.UTC
	}
	storeLocation(tz, loc)
	return loc
}

// ResolveBusinessLocation returns the configured *time.Location for a business,
// falling back to time.UTC for a nil business or an empty/invalid Timezone.
func ResolveBusinessLocation(b *Business) *time.Location {
	if b == nil {
		return time.UTC
	}
	return ResolveLocation(b.Timezone)
}

// sqlZoneName returns an IANA timezone name safe to inline into SQL. Names come
// from ResolveLocation / time.LoadLocation; anything outside the IANA character
// set (or an anonymous FixedZone) collapses to UTC as defense-in-depth.
func sqlZoneName(loc *time.Location) string {
	if loc == nil {
		return "UTC"
	}
	name := loc.String()
	if name == "" {
		return "UTC"
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '/', r == '_', r == '+', r == '-':
		default:
			return "UTC"
		}
	}
	return name
}

func localizedExprCacheKey(dialect, column, zone string, offsetSec int) string {
	// delimiter bytes that cannot appear in dialect/column/IANA names
	return dialect + "\x00" + column + "\x00" + zone + "\x00" + strconv.Itoa(offsetSec)
}

// LocalizedTimestampExpr wraps a timestamp column so date/hour extraction
// buckets in the business's local calendar. loc==UTC (or nil) returns the
// column unchanged. Postgres uses AT TIME ZONE (DST-correct); SQLite shifts by
// the zone's current fixed UTC offset via datetime() — DST-imperfect, matching
// the pre-existing analytics contract (tests use DST-free zones).
//
// Results are cached (decision-14): the same (dialect, column, zone, offset)
// rebuilds zero SQL strings after the first call on a hot path.
func LocalizedTimestampExpr(dialect, column string, loc *time.Location) string {
	if loc == nil || loc == time.UTC {
		return column
	}
	zone := sqlZoneName(loc)
	if zone == "UTC" && loc == time.UTC {
		return column
	}
	// Sample offset once for the cache key (SQLite expr embeds it; Postgres
	// ignores it but sharing the key keeps one code path).
	_, offset := time.Now().In(loc).Zone()
	key := localizedExprCacheKey(dialect, column, zone, offset)
	if cached, ok := localizedSQLCache.Load(key); ok {
		return cached.(string)
	}
	var expr string
	if dialect == "sqlite" {
		expr = fmt.Sprintf("datetime(%s, '%+d seconds')", column, offset)
	} else {
		expr = fmt.Sprintf("(%s)::timestamptz AT TIME ZONE '%s'", column, zone)
	}
	localizedSQLCache.Store(key, expr)
	return expr
}
