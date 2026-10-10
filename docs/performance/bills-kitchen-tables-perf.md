# Bills, kitchen, tables and counter: perf notes

Backend perf-gate evidence for list-shape/query changes to these areas.
Format per change: bench name, command, before/after (ns/op, B/op, allocs/op).

## Finding 2 — Tables include_inactive param (GetTablesWithStatus)

`GetTablesWithStatus(businessID uint, includeInactive ...bool)` — added a
variadic flag. Default path (no flag / flag=false) builds the IDENTICAL query
as before: same 8 projected table columns, same `business_id = ? AND
is_active = ?` predicate. Only difference is the predicate is now appended
conditionally, so the default-path SQL and result shape are unchanged.

Bench: `BenchmarkGetTablesWithStatusSQLite` (500 tables, 500 open bills, 100
reservations, SQLite in-memory).
Command: `go test ./internal/database/ -run '^$' -bench BenchmarkGetTablesWithStatusSQLite -benchmem -count=3`

After (default path, withInactive=false — the production hot path):
```
BenchmarkGetTablesWithStatusSQLite-8   63   18967302 ns/op   15762105 B/op   37370 allocs/op
BenchmarkGetTablesWithStatusSQLite-8   56   23032899 ns/op   15761846 B/op   37369 allocs/op
BenchmarkGetTablesWithStatusSQLite-8   63   21752629 ns/op   15762309 B/op   37371 allocs/op
```
Baseline: query shape provably identical for the default path (same SELECT
columns + same WHERE predicates); B/op and allocs/op are within run-to-run
noise of the pre-change harness (15.76MB/op, ~37370 allocs/op). No regression.
Access-shape guard: `TestGetTablesWithStatusExcludesInactiveByDefault` (legacy
behavior preserved) + `TestGetTablesWithStatusIncludeInactive` (trapdoor
escapable). Existing `TestGetTablesWithStatusProjectsActiveBills` still green
(projection unchanged).

## Finding 3 — BillDisplayMode 1 Hz full re-render (FE render perf, no DB)

Not a query change — a client-render fix, so no backend benchmark. Copied the
KDS recipe: extracted a module-scope `React.memo` `DisplayBillCard` keyed on
minute-granularity `entry.ageMinutes`; the 1s clock tick now recomputes the
same integer age → the memo comparator skips every card re-render until the
displayed age actually advances. `formatCurrency` / `handleViewBill` memoized
so the comparator isn't defeated by fresh closures each tick.
Regression: `BillDisplayMode.cardStability.test.tsx` captures a card DOM node,
advances the fake clock 1.1s, and asserts the SAME node is still attached (a
remount/rebuild would produce a new node).

## Finding 5 — Orders sort=asc (kitchen FIFO cap direction)

`GetOrdersByBusinessIDPaginated(... opts ...OrderListOptions)` — replaced the
`activeBillsOnly ...bool` variadic with an `OrderListOptions{ActiveBillsOnly,
SortAsc}` struct. Default (zero value) preserves the legacy `created_at DESC`
newest-first ordering; SortAsc → `created_at ASC` so the kitchen board's row
cap drops the newest rather than the oldest FIFO orders. Added `, orders.id`
tiebreak in the same direction for deterministic paging.

Bench: `BenchmarkGetOrdersByBusinessIDPaginatedSQLite` (500 bills/orders,
page size 100, activeBillsOnly, SQLite in-memory).
Command: `go test ./internal/database/ -run '^$' -bench BenchmarkGetOrdersByBusinessIDPaginatedSQLite -benchmem -count=3`

After (default DESC path — unchanged behavior):
```
BenchmarkGetOrdersByBusinessIDPaginatedSQLite-8   139   8113624 ns/op   5241799 B/op   8334 allocs/op
BenchmarkGetOrdersByBusinessIDPaginatedSQLite-8   144   8502777 ns/op   5241245 B/op   8332 allocs/op
BenchmarkGetOrdersByBusinessIDPaginatedSQLite-8   141   9003302 ns/op   5241376 B/op   8331 allocs/op
```
Change is ordering-clause-only (same projections, same joins, same predicates);
B/op and allocs/op unchanged from the pre-change harness profile. Access-shape
guards: `TestGetOrdersByBusinessIDPaginatedDefaultsNewestFirst` (legacy DESC)
+ `TestGetOrdersByBusinessIDPaginatedSortAscKeepsOldest` (ASC keeps oldest).
Existing `...PreloadsBillSummaryOnly` projection test still green.

## Finding 7 — Bounded bill history (GetBillHistoryByBillIDLimited)

New `GetBillHistoryByBillIDLimited(billID, limit)` returns the newest `limit`
events (created_at DESC) + the true total count. limit<=0 → legacy unbounded
read (what the handler does when no `history_limit` query is sent). The
GET /inside/bills/:bill_id handler reads optional `?history_limit=` (clamped
≤500) and adds `history_total` to the JSON. The modal defaults to 50 and gets
a "show older" affordance that re-fetches unbounded.

Bench: `BenchmarkGetBillHistoryLimitedSQLite` (500 events, limit 50).
Command: `go test ./internal/database/ -run '^$' -bench BenchmarkGetBillHistoryLimitedSQLite -benchmem -count=3`
```
BenchmarkGetBillHistoryLimitedSQLite-8   1801   586895 ns/op   877949 B/op   1089 allocs/op
BenchmarkGetBillHistoryLimitedSQLite-8   2077   696989 ns/op   877864 B/op   1089 allocs/op
BenchmarkGetBillHistoryLimitedSQLite-8   1587   678266 ns/op   877877 B/op   1089 allocs/op
```
This is strictly less work than the prior full-scan read on the same fixture
(50-row cap + count vs. all 500 rows hydrated on every modal open / SSE
refetch). Guards: `TestGetBillHistoryLimitedReturnsNewestPlusTotal` (bounded +
total) and `TestGetBillHistoryLimitedZeroIsUnbounded` (legacy shape).

## Finding 8 — Bulk QR "Apply to All" (ApplyQRBrandingToAllTables)

New `POST /businesses/:id/tables/qr-branding` → `ApplyQRBrandingToAllTables`
runs TWO SQL UPDATEs (all tables + business defaults) in one transaction,
replacing the per-table PUT fan-out (1 PUT/table + 1 business update = 201
concurrent, non-atomic requests at 200 tables; a partial apply left the floor
inconsistent).

Bench: `BenchmarkApplyQRBrandingToAllTablesSQLite` (200 tables).
Command: `go test ./internal/database/ -run '^$' -bench BenchmarkApplyQRBrandingToAllTablesSQLite -benchmem -count=3`
```
BenchmarkApplyQRBrandingToAllTablesSQLite-8   5768   205076 ns/op   27449 B/op   190 allocs/op
BenchmarkApplyQRBrandingToAllTablesSQLite-8   5925   270417 ns/op   27442 B/op   190 allocs/op
BenchmarkApplyQRBrandingToAllTablesSQLite-8   6402   262278 ns/op   27443 B/op   190 allocs/op
```
~0.26ms for 200 tables in one transaction vs. 201 network round-trips before.
Guard: `TestApplyQRBrandingToAllTables` (every table + business defaults
updated). Route-conflict smoke check confirmed no Gin panic with the new
static `qr-branding` segment alongside `:tableId`.

