# Stream 1 (Analytics + Overview + Cash Register) — perf gate records

Benchmarks captured on the isolated worktree branch. SQLite microbenchmarks,
`-benchmem -count=3`.

## Fix A — Payment History server pagination

Command:
```
go test ./internal/server/ -bench 'BenchmarkLoadPaymentHistoryItems' -benchmem -run '^$' -count=3
```

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkLoadPaymentHistoryItemsSQLite` (full 500-row window, legacy) | ~12.0M | 717,663 | 17,446 |
| `BenchmarkLoadPaymentHistoryItemsPaginatedSQLite` (page of 20) | ~3.9–10.8M | 43,138 | 927 |

Result: the page the client renders drops from 500 rows to 20 → **~16x fewer
bytes/op, ~18x fewer allocs/op**. The COUNT still scans the window (bounded by
the ≤366-day range clamp), so latency improves less than allocations, but the
serialized payload and client-side work shrink proportionally to page_size.

Access-shape: `TestLoadPaymentHistoryItemsFilteredAccessShape` asserts the
projected join is preserved on the paginated path (no `SELECT *` on bills/tables).

## Fix 6 — Live-bills LIMIT + capped flag; item-analytics limit

Command:
```
go test ./internal/handlers/ -bench 'BenchmarkAnalyticsLiveBillsSQLite' -benchmem -run '^$' -count=3
```

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkAnalyticsLiveBillsSQLite` (250 seeded, capped to 200) | ~3.7–4.9M | ~1.30M | ~14,674 |

`GetActiveBillSummariesByBusinessID` now bounds at DefaultActiveBillSummaryLimit
(200, newest-first) with an over-fetch-by-one capped detector; the handler echoes
`capped`/`limit`. In this 250-row fixture the cap shaves ~20%; the real guarantee
is O(limit) instead of O(open-bills) — a business with thousands of open bills now
returns 200, not thousands, every 10s poll. Regression:
`TestGetActiveBillSummariesBoundedCapsAndFlags` + `TestAnalyticsLiveBillsBoundsPayload`.
Item analytics (`GetItemAnalytics`) now defaults limit=100 (was 0/unbounded);
GetPopularItems bounds at SQL. Briefing OpenBills switched to the COUNT query so
the summary cap can't undercount it.

## Fix 14 — Unassigned-cash list endpoint (bounded)

Command:
```
go test ./internal/database/ -bench 'BenchmarkListUnassignedCashAlternativePayments' -benchmem -run '^$' -count=3
```

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkListUnassignedCashAlternativePayments` (500 unassigned, page of 50) | ~0.8–1.0M | ~57,500 | ~1,285 |

New `GET /cash-register/unassigned/list` returns the individual unassigned cash
tenders that make up the summary total, bounded (limit clamped <=100, default 50)
+ offset + total. Payload is O(limit), not O(unassigned tenders). Regression:
`TestListUnassignedCashAlternativePaymentsMatchesSummary` (list agrees with the
summary count, bounded + offset) and `TestListUnassignedCashAlternativePaymentsExcludesAssigned`
(assigned tenders excluded, mirrors the summary's crm.id IS NULL predicate).
Assign-to-session action was NOT added (no cheap existing service capability that
scopes it here) — list-only, deferred.
