> Short commit SHAs and branch names in this log refer to pre-release private history. They do not resolve in the public repository.

# Team, schedule and communication: perf notes

Base commit: 7feaee760 (pre-release main)

Backend perf gate applies to these list-shape/query changes:
- Fix 1: batch ack-summary endpoint (new query shape)
- Fix 6: timesheet review pagination + window (new params on ListForReview)

## Benchmarks

### Fix 1 — batch ack-summary endpoint (`GetAnnouncementAckSummaries`)

Replaces the per-announcement `AckRoster` poll (audit's one true HTTP N+1:
100 require_ack announcements = ~200 req/min forever, each mounting its own
`GET /announcements/:id/acks` on a 30s interval).

New method: `GetAnnouncementAckSummaries(businessID, ids, firstNamesLimit)` —
one call returns `{acked,total_eligible,first_acker_names}` per id in a SMALL
CONSTANT number of SELECTs (announcements + acked counts + first names +
per-audience-family totals), asserted by `TestGetAnnouncementAckSummariesAccessShape`
(≤6 SELECTs for 30 announcements; would be 30+ under N+1).

Baseline (no prior implementation existed — this is a net-new consolidation):
```
go test ./internal/database/ -run '^$' -bench BenchmarkGetAnnouncementAckSummaries -benchmem -count=3
BenchmarkGetAnnouncementAckSummaries-8   3112   559067 ns/op   95662 B/op   1596 allocs/op
BenchmarkGetAnnouncementAckSummaries-8   2281   566277 ns/op   95482 B/op   1596 allocs/op
BenchmarkGetAnnouncementAckSummaries-8   1980   548008 ns/op   95555 B/op   1596 allocs/op
```
50 announcements (mixed all/role/dept audiences) × 30 staff, ~555µs/op, one query
set. The "before" is N separate `GetAnnouncementAckStatus` roster calls (each its
own full LEFT-JOIN roster scan) fired every 30s per visible require_ack card; the
"after" is one bounded batch per feed refresh for visible cards only.

### Fix 6 — timesheet review pagination + window (`ListForReviewPaged`)

The manager review queue was `ListForReview` (fetches everything up to the silent
500 cap, no total). New BE-first `ListForReviewPaged(status, from, to, offset,
limit)` returns a bounded page + the TOTAL count (honest "showing X of N").
Access-shape test (`TestListForReviewPagedAccessShape`): a COUNT + one bounded
projected SELECT, never N+1, no `SELECT *`. Legacy `ListForReview` unchanged.

BEFORE (`BenchmarkListForReview`, 200 rows, all returned, no total):
```
BenchmarkListForReview-8   704   1695484 ns/op   268851 B/op   5218 allocs/op
BenchmarkListForReview-8   670   1804527 ns/op   268861 B/op   5218 allocs/op
BenchmarkListForReview-8   668   1799159 ns/op   268821 B/op   5218 allocs/op
```
AFTER (`BenchmarkListForReviewPaged`, 500 rows in DB, page of 50 + count):
```
BenchmarkListForReviewPaged-8   1155   1186051 ns/op   74063 B/op   1512 allocs/op
BenchmarkListForReviewPaged-8   1395   1077753 ns/op   74064 B/op   1512 allocs/op
BenchmarkListForReviewPaged-8   1137   1008809 ns/op   74061 B/op   1512 allocs/op
```
Even with 2.5× more rows in the DB (500 vs 200), the paged read materializes only
the 50-row page: ~3.6× fewer bytes/op (74KB vs 269KB) and ~3.5× fewer allocs
(1512 vs 5218). Commands: `go test ./internal/database/ -run '^$' -bench
'BenchmarkListForReview(Paged)?$' -benchmem -count=3`.
