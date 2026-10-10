> Short commit SHAs and branch names in this log refer to pre-release private history. They do not resolve in the public repository.

# AI cost and abuse hardening before the open-source release, 2026-10-03

> Point-in-time evidence. Migration numbers cited here (0002xx) predate the
> squash into `backend/schema/genesis/current_schema.sql`; that schema now
> lives in the genesis baseline, and numbered migrations restart at 000001.

This is the Backend Performance Gate record for the pre-release AI cost and
abuse hardening: the AI cost budget, guest AI message limits, Google Places
caching and the concierge lead flood. The baseline is the pre-release main
branch at `e26f72b6d`, exported to a scratch
directory and benchmarked from there. Before and after runs were interleaved
on one machine (Apple M3, 8 cores).

The machine was shared with other builds during these runs (load average
5.7 to 6.3), so latency figures vary a lot between runs. The B/op, allocs/op
and upstream-calls/op figures are deterministic, and the conclusions below
rest on them.

## 1. Budget reservation path (Postgres)

`BenchmarkCallBudgetReserveFinalize` in `backend/internal/llm/budget_bench_test.go`
measures one guarded provider call: a reserve against the durable ledger
followed by a finalize. It runs on the real migrations 000132, 000140 and
000225 against Postgres 15.

```bash
cd backend && go test -c -o /tmp/llm.test ./internal/llm/
cd internal/llm && TEST_DATABASE_URL=postgres://... /tmp/llm.test \
  -test.run '^$' -test.bench BenchmarkCallBudgetReserveFinalize \
  -test.benchmem -test.count=1 -test.benchtime=500x   # x3, interleaved
```

| Variant | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| before, business scope only | 7.90 ms, 3.66 ms, 7.35 ms | 105,000 | 1,204 |
| after, business scope only | 3.31 ms, 3.22 ms, 10.15 ms | 116,800 | 1,230 |
| after, business + global scope (production shape) | 5.92 ms, 5.57 ms, 8.74 ms | 183,300 | 1,979 |

Reading the numbers:

- **Single-scope path:** reserving now carries the multi-scope
  bookkeeping (a `parent_reservation_id` column and family reconcile).
  This costs about 11% more bytes and 26 more allocations per call.
  The latency difference is inside the noise.
- **Production shape:** this path also holds against the instance-wide
  `global` row in the same transaction, so each call writes a second ledger
  row and a child reservation. Best case it adds about 2.3 ms per guarded
  LLM call. An LLM call itself takes 0.5 to 5 s, so the overhead is under 1%
  of the request.
- **Concurrency limit:** every reservation in the instance locks the one
  global row, which serializes reservations. See the backlog note in the
  hand-off.

## 2. Guest message path (AI waiter)

`BenchmarkHandleAIWaiterTableFetch` in `backend/internal/server/ai_waiter_handler_test.go`
runs one table-scoped guest turn end to end through `HandleAIWaiter`, using
SQLite and a stub provider. The after tree includes the following:

- The budget checks now run before the user turn is persisted, and the
  session cap uses pending-turn arithmetic.
- A guest-scope USD gate check is added.
- The per-network and per-device `dailyquota.Take` calls are added.

```bash
cd backend/internal/server && /tmp/server.test -test.run '^$' \
  -test.bench 'BenchmarkHandleAIWaiterTableFetch$' -test.benchmem \
  -test.count=1 -test.benchtime=2000x   # x6, interleaved (order reversed for runs 4-6)
```

| Tree | ns/op (6 runs) | median | B/op | allocs/op |
|---|---|---|---|---|
| before | 2.05 ms*, 0.726, 0.652, 0.693, 0.681, 0.850 | 0.710 ms | ~468,500 | 4,547 |
| after | 1.27 ms*, 0.657, 0.685, 0.711, 0.721, 1.005 | 0.716 ms | ~468,500 | 4,553 |

\* The first run of each tree is a cold start.

The change is latency-neutral and memory-neutral. The +6 allocations per turn
come from the quota `Take` and the device-cookie read. The benchmark raises
`AI_WAITER_DAILY_MESSAGES_PER_IP` so every iteration reaches the model, and the
quota check still runs on every iteration.

The regression tests below were checked against the baseline tree with a
temporary probe:

- The baseline persisted all 5 turns that the budget refused, and let a
  session reach 65 stored messages against a cap of 60.
- After the change, a refused turn writes nothing.

## 3. Google Places reads

`BenchmarkPlacesDetailsRead` in `backend/internal/server/places_cache_test.go`
measures one storefront render's Places reads, which is one details call
plus one reviews call. The `uncached` variant is the pre-change behaviour,
and the `cached` variant is the decorator that `SetGooglePlacesService` now
installs. The upstream is a stub, so ns/op is the decorator's own overhead.
The real cost of the uncached path is a billed Place Details request that
takes 100 to 300 ms.

| Variant | ns/op | B/op | allocs/op | upstream calls per render |
|---|---|---|---|---|
| uncached (before) | 131 to 152 | 331 to 339 | 4 | 2.000 |
| cached (after) | 238 to 260 | 72 | 4 | 0.0000004 (one miss per key per 12 h) |

The end-to-end evidence comes from the handler regression test.
`TestGetPublicBusinessGoogleRoutes_ServedFromCache` makes 20 public storefront
renders, varying `?language=` each time, and then makes the owner dashboard
reads:

- **Baseline** (probe against the exported tree): 40 upstream calls (20
  reviews and 20 details).
- **After:** 2 upstream calls.

## 4. Regression and gate commands

The hermetic env prefix from `CLAUDE.md` applies throughout.

```bash
cd backend
go build ./... && go build -tags whatsapp ./...
go vet ./internal/llm ./internal/guardrails ./cmd/app ./internal/agents/... \
  ./internal/services ./internal/server ./internal/signedid ./internal/dailyquota
go test -short -p 2 -count=1 ./internal/llm ./internal/guardrails ./cmd/app \
  ./internal/agents/... ./internal/services ./internal/server \
  ./internal/signedid ./internal/dailyquota
cd ../frontend && npx jest src/components/concierge src/lib/storefront/serverData.test.ts
```

## 5. Not measured

- **Postgres-backed reservation tests** (`budget_scopes_test.go`): these need
  `TEST_DATABASE_URL`. They ran green against a throwaway Postgres 15
  container during this work. Without that variable set they skip.
- **Multi-replica behaviour**: the in-memory quota counters, the Places cache
  and the concierge lead counters are per process. N replicas multiply each
  ceiling by N, while the durable USD ledger stays the shared backstop.

## 6. Fix round 1 (review findings F1 to F7)

Load average on the shared machine was 21 to 33 during these runs, so
latency moved by 2x to 3x between identical runs. The B/op and allocs/op
figures are deterministic and are what the conclusions rest on. Raw output
was kept in the session scratchpad.

### 6.1 Cost-gate pre-check and reservation (F1, F2)

F1 adds a guest-pool scope (business 0, `guest_pool`; named `public` before
the round-2 fix below) inside `global` that guest calls also reserve against,
so they can no longer spend the owner reserve. That adds a third ledger row to every guest reservation and a third
read to the guest pre-check. Two optimizations pay for most of it:

- `AICostGate.OverBudget` reads every scope it checks in one
  `SELECT ... WHERE usage_date = ? AND ((business_id, feature_scope) OR ...)`
  through `BudgetStore.CommittedSpendScopes`, instead of one query per scope.
- `BudgetStore.Reserve` bumps `reserved_micro_usd` on all locked daily rows
  with one `UPDATE ... WHERE id IN ?`, instead of one `Save` per row.

`BenchmarkAICostGateOverBudget` and `BenchmarkCallBudgetReserveFinalize`
(`backend/internal/llm/budget_bench_test.go`), Postgres 15. The before tree
is `9b382b40f` (this change before the fix round) with the new benchmark
file added. The table shows the final interleaved set, three runs per binary:

```bash
cd backend && go test -c -o $SP/llm-after.test ./internal/llm/
cd internal/llm && TEST_DATABASE_URL=postgres://... $SP/llm-after.test \
  -test.run '^$' \
  -test.bench 'BenchmarkAICostGateOverBudget|BenchmarkCallBudgetReserveFinalize/(business_plus_global|guest_plus_pool)' \
  -test.benchmem -test.count=1 -test.benchtime=500x   # x3, interleaved with the before binary
```

| Benchmark | Before | After |
|---|---|---|
| Gate, guest + global (2 scopes) | 215 allocs, 14.5 KB, 490 to 1,190 us | 114 allocs (-47%), 10.6 KB (-27%), 409 to 514 us |
| Gate, guest + guest pool + global (new production shape) | n/a | 128 allocs, 11.8 KB, 314 to 499 us |
| Reserve+finalize, owner: business + global | 1,978 allocs, 183.6 KB, 8.5 to 17.6 ms | 1,843 allocs (-7%), 169.1 KB (-8%), 4.9 to 17.1 ms |
| Reserve+finalize, guest: business + guest pool + global | 1,978 allocs, 183.6 KB (was 2 scopes) | 2,506 allocs, 226.4 KB, 11.4 to 24.2 ms |

Reading the numbers:

- **Pre-check:** the guest gate now checks three scopes for fewer
  allocations than it used to spend on two.
- **Owner reservation:** slightly cheaper than before, from the batched
  `UPDATE`.
- **Guest reservation:** it now locks and writes a third ledger row and a
  third child reservation, which costs about +27% allocations. Before the
  `UPDATE` batching the same shape cost 2,722 allocs and 248.8 KB.
  Against a guarded LLM call of 0.5 to 5 s this overhead stays under 1%.
- **Concurrency:** every guest reservation in the instance now locks two
  business-0 rows (`guest_pool` and `global`), so those reservations
  serialize on them. This is the same class of limit as section 1 and is
  carried in the hand-off backlog.

**Round-2 fix (F1R): per-venue guest share, concierge out of the pool.** The
first layout let one venue's guests ($5) plus the concierge ($5) fill the
shared $10 sub-cap and refuse guest AI at every other venue. Now the pool is
guest-only, each venue's guest scope is capped at min(per-business, pool / 4)
($2.50 by default), and the concierge keeps its own $5 slice inside `global`.
`TestBudgetStore_DefaultCapsOneVenuePlusConciergeCannotCloseOtherVenues`
pins the default numbers. The guest reservation shape is unchanged (three
rows), and a concierge reservation now locks two rows instead of three.
Rerun with the same command (benchmark names renamed to `guest_plus_pool*`),
local Postgres 14, `-test.count=3 -test.benchtime=500x`:

| Benchmark | Round 1 | Round 2 |
|---|---|---|
| Gate, guest + guest pool + global | 128 allocs, 11.8 KB | 128 allocs, 11.8 KB, 50 to 56 us |
| Reserve+finalize, owner: business + global | 1,843 allocs, 169.1 KB | 1,842 allocs, 168.9 KB, 1.2 to 2.6 ms |
| Reserve+finalize, guest: business + guest pool + global | 2,506 allocs, 226.4 KB | 2,507 allocs, 226.5 KB, 3.8 to 4.5 ms |

Allocation and byte counts are flat. The latency columns are not comparable
across rounds, because round 1 ran on a different Postgres instance.

### 6.2 Paused (taken-over) guest turn (F4)

The paused branch of `HandleAIWaiter` now counts the session's rows (the same
indexed `COUNT` the AI branch already runs) and takes the in-memory per-client
quota before it persists the turn. `BenchmarkHandleAIWaiterPausedTurn`
(`backend/internal/server/ai_waiter_paused_bench_test.go`) measures one guest
turn into a conversation whose claim is held, so no model call and no alert:

```bash
cd backend && go test -c -ldflags='-s -w' -o $SP/server-after.test ./internal/server/
cd internal/server && $SP/server-after.test -test.run '^$' \
  -test.bench 'BenchmarkHandleAIWaiterPausedTurn$' -test.benchmem \
  -test.count=1 -test.benchtime=2000x   # x6, interleaved with the before binary
```

The before tree is `7cfac30ea` with the benchmark file added. Six runs per
tree were taken: rounds 1 to 3 ran before-then-after, and rounds 4 to 6 ran
after-then-before. Load average was about 36 throughout.

| Tree | ns/op (6 runs) | min | median | B/op | allocs/op |
|---|---|---|---|---|---|
| before | 1.70, 0.90, 0.79, 3.08, 1.24, 3.64 ms | 0.79 ms | 1.47 ms | ~167,800 | 2,175 |
| after | 2.43, 0.75, 4.44, 3.73, 0.95, 4.47 ms | 0.75 ms | 3.08 ms | ~171,400 | 2,224 |

The deterministic cost is +49 allocations and about 3.6 KB per paused turn
(+2.2%). That comes from the extra `COUNT` and the quota `Take`. Latency
under this load does not resolve the change:

- The fastest runs of the two trees are within 5% of each other.
- Single runs of the same binary vary by 5x.

The medians differ, but the run-to-run spread is wider than that difference.
The added work is the same indexed `COUNT` the AI branch runs on every turn,
so a quiet-machine rerun is listed as not measured below.

### 6.3 Not hot-path or not measured

- **F3 (concierge tool-path leads keyed on client IP):** this passes one more
  string through `ToolEnv`. The per-IP `dailyquota.Take` already ran on the
  HTTP lead path, so nothing new was added to a hot path.
- **F5 (WhatsApp per-sender daily ceiling):** this is one in-memory
  `dailyquota.Take` (a SHA-256 of the JID and an LRU lookup) per inbound
  message, in front of a model call. It is not benchmarked.
- **F7 (Ops prompt business name):** this sanitizes a string of at most 120
  runes once per Ops request. It is not benchmarked.
- **Quiet-machine latency rerun:** no run in this round could resolve
  latency. The disk was also full for part of the round, which delayed the
  runs. The allocation deltas above are the evidence.
