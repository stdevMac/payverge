> Short commit SHAs in this log refer to pre-release private history. They do not resolve in the public repository.

# Backend Performance Benchmark Summary

## Authoritative guest checkout and orderability projection (2026-07-18)

### Coverage and access shape

- `TestGuestCheckoutConcurrentReplayPostgres` exercises concurrent idempotent
  retries, reuse of an existing active table bill, shared table-row locking with
  reservation writes, and rollback after a forced failure. The committed result
  is one bill and one order for a valid request; rejected/rolled-back requests
  leave neither partial record behind.
- `TestWebhookIngestQueryShape` remains within its existing completed-settlement
  budget of at most three bill SELECTs. Public table lifecycle signals reuse the
  bill already loaded by the payment path instead of adding a reload.
- Orderability is built in one batched projection for menu items and expanded
  bundle children; checkout revalidates the resolved lines inside the write
  transaction.

### Benchmark

`BenchmarkOrderabilityProjection` projects 500 menu items with inventory and
availability data in one service call. A one-iteration verification run on Apple
M3 / Go 1.25 measured approximately 15.7 ms/op, 18.7 MB/op, and 33,900
allocations/op. This is an absolute stress-shape measurement rather than a
before/after comparison; the regression tests additionally pin bounded query
count so item volume cannot reintroduce an N+1 database path.

### Commands

```text
go test -run '^$' -bench BenchmarkOrderabilityProjection -benchtime=1x -benchmem ./internal/services
go test -run '^TestWebhookIngestQueryShape$' ./internal/handlers
go test -run '^TestGuestCheckoutConcurrentReplayPostgres$' ./internal/services
```


## Checkout status poll — Stripe reconcile floor (Wave 2 Task 9, 2026-07-16)

### Benchmark
`BenchmarkGetPaymentStatusPending` in `internal/services/subscription_stripe_status_floor_test.go`.

### Regression tests
`TestGetPaymentStatus_ReconcileFloorPerSession` — 30-poll burst on one pending
session performs exactly ONE live Stripe reconcile call (10s per-session floor);
a second session gets its own window.

### Commands
`go test ./internal/services -run '^$' -bench BenchmarkGetPaymentStatusPending -benchmem -count=3`
(run twice: once at the pre-change commit — Task 8's HEAD — and once after)

### Before/after (median of 3, copied from /tmp/bench_status_before.txt / _after.txt)
| Run | ns/op | B/op | allocs/op | stripe-calls/op |
|---|---|---|---|---|
| before | 123410 | 18238 | 321 | 1.0 |
| after | 48350 | 9617 | 177 | 0.0000365 |

Route tier added the same task: POST /subscription/stripe/checkout behind
publicFormLimiter (5/min burst 2), GET /subscription/stripe/status/:session_id
behind CHECKOUT_STATUS_RATE_LIMIT_* (60/min burst 30 — sized above the success
page's 30x1s fast poll).

## Guest menu 86'd-item filter — Task P1.7 (2026-06-20)

### Benchmark
`BenchmarkBuildPublicGuestMenuResponse` in `internal/server/public_guest_menu_filter_test.go`.

### Regression tests
`TestBuildPublicGuestMenuResponse_DropsUnavailableItems` — proves 86'd items and
empty categories are removed from the guest response.
`TestBuildPublicGuestMenuResponse_MalformedJSONIsEmpty` — proves malformed stored
JSON returns an empty array rather than panicking.

### Setup
- Apple M3, Go 1.25, `goos: darwin goarch: arm64`
- 8-category × 12-item menu, 6 available + 6 unavailable per category (50% filtered)

### Why the trade-off is worth it
The old implementation (`return gin.H{"categories": menu.Categories}`) was a
zero-compute raw-string passthrough, but it served 86'd items to every guest. The
new implementation parses the stored JSON blob, filters out `is_available:false`
items, drops empty categories, and re-serializes once. The added marshal/unmarshal
cost is ~188 µs for a realistic 8×12 menu; the offsetting gain is a smaller wire
payload (50% item reduction in this benchmark) on the highest-traffic guest read.
The 5-second pricing-cache TTL (P1.6) means callers already feed a cached menu
object — this parse runs at most once per business per TTL window.

### After numbers (`-benchmem -count=3`)
| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkBuildPublicGuestMenuResponse (8 cats × 12 items, half filtered) | ~188,000 | 118,127 | 273 |

No "before" microbench exists because the prior impl had zero compute on this path
(raw string copy). The numbers above represent the absolute added parse+filter+serialize cost.

### Payload-size delta
8 categories × 12 items = 96 items total; 48 filtered out → ~50% reduction in
the `categories` array payload size for guest reads (exact bytes depend on item
field widths but structurally half the items are dropped).

### Commands
```
go test ./internal/server/ -run TestBuildPublicGuestMenuResponse -v
go test ./internal/server/ -run '^$' -bench BenchmarkBuildPublicGuestMenuResponse -benchmem -count=3 | tee /tmp/p1_7_after.txt
go test ./internal/server/ -count=1
```

---

## Plugin config secret encryption-at-rest — B1 (2026-06-18)

### Benchmark
`BenchmarkDecryptConfigSecrets`, `BenchmarkDecryptConfigSecrets_NoSecrets`,
`BenchmarkEncryptConfigSecrets` in `internal/security/config_secrets_test.go`.

### Why
B1 adds a generic encrypt-on-write / decrypt-on-read layer at the plugin-config
DB boundary (`EnableBusinessPlugin`, `UpdateBusinessPluginConfig`,
`GetBusinessPluginConfig`, `GetBusinessPluginConfigState`). Decrypt runs on every
plugin-config load — including payment charge/refund/webhook paths — so its
per-read cost is the relevant hot-path metric.

### Numbers (`-benchmem -count=3`)
| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| DecryptConfigSecrets (stripe-like, 2 secret fields) | ~1190 | 3328 | 16 |
| DecryptConfigSecrets_NoSecrets (telegram-like) | ~197 | 336 | 2 |
| EncryptConfigSecrets (write path) | ~1875 | 3888 | 23 |

~1.2 µs added per config read with secrets is negligible against the DB round-trip
the read already performs. `GetBusinessPaymentPluginConfigs` (guest payment listing
hot path) deliberately skips decrypt (gate-only), so it pays zero added cost.

### Commands
- `go test ./internal/security/ -run '^$' -bench 'ConfigSecrets' -benchmem -count=3`
- `go test ./internal/database/ -run 'EncryptsSecretsAtRest|LegacyPlaintext|Backfill' -count=1`
- `go test ./internal/{security,database,plugins/...,handlers,server,services} -count=1`
- `go test ./internal/{auth,middleware,security,database} -race -count=1`
- `go build ./...` · `go vet ./internal/{handlers,security,database}`

### Notes
Opt-in: inert (plaintext, byte-identical to today) until `PLUGIN_SECRET_KEY` is
provisioned. Decrypt always passes legacy plaintext through. No baseline "before"
microbenchmark exists because the prior behavior had no crypto on this path — the
numbers above are the absolute added cost, not a delta.

## BenchmarkGetDailySeries — Task 3 baseline (2026-05-29)

### Benchmark
`BenchmarkGetDailySeries` in `internal/analytics/service_benchmark_test.go`

### Setup
- SQLite in-memory, 2500 seeded bills spread over 7 days ending 2026-05-04
- `from = 2026-04-28`, `to = 2026-05-04` (covers all seeded days — measures real aggregation, not an empty scan)
- Apple M3, Go 1.25, `goos: darwin goarch: arm64`

### Results (`-count=3`)

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 72,348,861 | 164,864 | 654 |
| 2   | 73,549,514 | 164,477 | 653 |
| 3   | 74,944,611 | 160,885 | 651 |
| **median** | **73,549,514** | **164,477** | **653** |

Command run:
```
go test ./internal/analytics/ -bench BenchmarkGetDailySeries -benchmem -run '^$' -count=3
```

### Comparison — BenchmarkGetPaymentWindowSummary (`-count=3`)

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 72,937,917 | 164,091 | 544 |
| 2   | 72,909,414 | 162,688 | 544 |
| 3   | 73,997,297 | 164,029 | 544 |
| **median** | **72,937,917** | **164,029** | **544** |

### Analysis

`GetDailySeries` is within 1% of `GetPaymentWindowSummary` on ns/op and B/op. The +109 allocs/op delta is the 7-bucket slice/map enumeration in `enumerateDays` + the `byDay` map fill — not a SQL access shape difference. No index is needed: `GetDailySeries` reuses the exact recognized-payments scan (`recognizedPaymentBillAmountsSubquerySQL` + `recognizedEventsCTE`) that the existing summary/hourly aggregation already exercises; the only added work is a `GROUP BY day` reshaping of already-scanned rows, which SQLite handles via a single-pass scan.

### Index decision
**No new index.** The daily-series benchmark is not materially worse than the comparable summary benchmark (within 1% ns/op at the same 2500-bill seeded volume). The Task 2 access-shape regression test guards against future N+1 regression.

### Notes
- `setupAnalyticsBenchmarkDB` was updated with `b.Cleanup(func() { sqlDB.Close() })` to destroy the named `cache=shared` SQLite in-memory DB after each benchmark run. This is required for `-count=N > 1` correctness and fixes a latent fragility in all three pre-existing benchmarks (`BenchmarkGetPaymentWindowSummary`, `BenchmarkGetPeriodReport`, `BenchmarkGetDailySeries`).
- This is the baseline for the new additive timeseries endpoint; the Task 2 access-shape test in `internal/analytics/timeseries_test.go` guards against future N+1 regression on `GetDailySeries`.

## Gemini → OpenRouter migration — transport perf note (2026-06-03)

The LLM integration moved from the Google `genai` SDK to OpenRouter behind a
provider-neutral `internal/llm` interface. This is a **transport swap**, not a
change to call fan-out:

- Each `llm.Provider.Generate` issues exactly **one** HTTP round-trip per model
  turn. OpenRouter's `models[]` fallback array is resolved server-side in that
  single request, so configuring fallbacks does not add client round-trips.
- The director console loop still issues one `Generate` per iteration, bounded by
  `MaxIterations` (default 6) and `WallClock` (default 12s) — unchanged from the
  genai implementation. Regression guard: `TestDirectorLoopSingleRoundTripPerIteration`
  asserts one `Generate` call for a single-turn (text-only) response.
- AI-waiter, WhatsApp, menu extraction/wizard/generation, and image generation
  each issue a single `Generate` per call, identical to the prior per-call
  `GenerateContent` count.

No new N+1 patterns, preloads, or query-shape changes are introduced; this slice
does not touch the DB access path. Benchmarks were not re-baselined because the
change is a 1:1 transport substitution with no added round-trips or allocations
on the hot path beyond JSON (de)serialization equivalent to the prior SDK.

## AI Excellence Campaign — perf evidence (2026-06-06)

Consolidated from Lanes A/B/C/D/I/J per Backend Performance Gate item 6.

| Lane | Benchmark | Command | Evidence commit |
|---|---|---|---|
| A | LLM success-path `Generate` (retry+observer overhead) | `go test -bench=BenchmarkGenerate -benchmem -count=3 ./internal/llm/` | `e1ac17a1` |
| A | Guardrails inner-path micro-benchmarks | `go test -bench=. -benchmem -count=3 ./internal/guardrails/` | `7d61cf80` |
| B | Waiter prompt-assembly benchmark | `go test -bench=BenchmarkBuild -benchmem -count=3 ./internal/services/` | `42bdf1df` |
| C | Director loop access-shape benchmark | `go test -bench=BenchmarkDirector -benchmem -count=3 ./internal/services/` | `b342affb` |
| I | Waiter SSE access-shape + before/after benchmark | `go test -bench=BenchmarkSSE -benchmem -count=3 ./internal/server/` | `09ef489b` |
| J | 10-turn input-token caching-value model + waiter build-request benchmark | `go test -bench=BenchmarkCaching -benchmem -count=3 ./internal/services/` | `af7e63ce` |

Note: Concrete ns/op, B/op, and allocs/op per run were recorded in each lane's
private plan and are not reproduced here.

## Director Console v2 — Pillar 1 (grounding) perf evidence (2026-06-07)

Two new microbenchmarks gate the memory and trend-math additions. Both are pure
in-process paths (no new DB round-trips on the hot path); numbers below are
Apple M3 (arm64), `-benchmem -count=3`.

| Path | Benchmark | Command | ns/op | B/op | allocs/op |
|---|---|---|---|---|---|
| Conversation memory (compact ≤3 prior exchanges from 12 msgs) | `BenchmarkBuildPriorTurns` | `go test -run '^$' -bench BenchmarkBuildPriorTurns -benchmem -count=3 ./internal/services/` | ~745 | 2112 | 2 |
| Trend math (`compare_to_prior=true`: period + prior-window fetch + 2 delta maps) | `BenchmarkRevenueSummary_WithAndWithoutPrior` | `go test -run '^$' -bench BenchmarkRevenueSummary -benchmem -count=3 ./internal/services/director_tools/` | ~1145 | 1873 | 29 |

Access-shape notes:
- `buildPriorTurns` is allocation-bounded (one `make([]llm.Message, 0, maxTurns*2)`
  slice + backing array = 2 allocs) regardless of input size; the memory read in
  the service issues exactly one bounded `ListDirectorConsoleMessages` (limit 12)
  and only on the loop branch (guardrail-blocked asks skip it).
- `compare_to_prior` adds exactly one extra `GetPaymentWindowSummary` call and two
  result maps, and ONLY when the flag is set (guarded by `comparePriorRequested`);
  the no-flag path is unchanged (regression-tested by `TestRevenueSummaryTool_NoPriorWhenFlagOff`).
- The 3 analytics-backed director tools now resolve `env.Analytics` when their
  struct-field provider is nil (production wiring fix) — no added query shape,
  same single analytics call they always intended to make.

## Director Console v2 — Pillar 2 (write) perf evidence (2026-06-07)

Two microbenchmarks gate the proposal preview and apply write paths. Both run on
Apple M3 (arm64), `-benchmem -count=3`.

### BenchmarkComputePriceChange — pure compute, 50-item menu

Command:
```
cd backend && go test ./internal/services/director_actions/ -run '^$' -bench BenchmarkComputePriceChange -benchmem -count=3
```

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 4,138 | 14,987 | 123 |
| 2   | 4,483 | 14,987 | 123 |
| 3   | 4,356 | 14,987 | 123 |
| **median** | **4,356** | **14,987** | **123** |

The 123 allocs/op are the deep-copy of 50 items across 2 categories (item slices,
string fields, tag slices) — fixed cost per call regardless of B.N; B/op is
stable at 14,987 across all 3 runs.

### BenchmarkApplyPriceChange — full apply transaction, SQLite

Command:
```
cd backend && go test ./internal/services/ -run '^$' -bench BenchmarkApplyPriceChange -benchmem -count=3
```

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 105,425 | 56,542 | 595 |
| 2   | 110,087 | 56,540 | 595 |
| 3   | 106,906 | 56,541 | 595 |
| **median** | **106,906** | **56,541** | **595** |

Per-iteration setup (delete prior rows, re-seed menu at v1, re-seed pending
proposal) is excluded via `b.StopTimer()/b.StartTimer()`. The measured path is:
proposal load → menu load → recompute (pure compute) → version-checked menu write
→ audit insert → status update — all in one SQLite transaction.

### Access-shape notes

**Proposal preview** (propose tools only, no write):
- Exactly one `GetMenuByBusinessID` call (read the menu into `[]MenuCategory`).
- Pure compute (`ComputePriceChange` / `ComputeAvailabilityChange` / `ComputeContentEdit`) — no DB write; the proposal row insert is one `CREATE`.
- No N+1: menu is loaded once; compute iterates the in-memory slice.

**Apply** (`DirectorActionService.Apply`):
- One menu read: `GetMenuByBusinessID` (loads `*Menu` + deserialises `Categories` JSON once).
- One versioned write: `ApplyMenuCategoriesTx` with `WHERE business_id = ? AND is_active = true AND version = ?` predicate — the `version = ?` guard is the concurrent-edit safety check; 0 rows affected → `ErrMenuVersionConflict` (409).
- One audit insert: `tx.Create(&DirectorActionAudit{…})`.
- One status update: `tx.Model(&DirectorProposedAction{}).Where("id = ?", …).Update("status", applied)`.
- All four writes are inside a single `db.Transaction` — any failure rolls everything back.
- No per-item N+1 reload: categories are loaded once as a JSON blob and deserialized into the in-memory slice; individual items are mutated in-process, then the whole blob is re-serialized and written in one UPDATE.

**Regression guard:** `TestApply_AccessShape_SingleMenuLoadAndVersionedWrite` (in
`director_action_service_test.go`) asserts behaviorally: (1) one apply bumps
version by exactly 1 (v1→v2), and (2) a second apply of the same proposal fails
immediately with `ErrDirectorActionNotPending` — proving no duplicate or looped
write path exists.

## Stage 1 Task 1 — X-1 benchmark evidence (2026-06-12)

### Benchmark
`BenchmarkGetOrdersByBillIDSQLite` in `internal/database/orders_bill_lookup_perf_test.go`

### Setup
- Apple M3, Go 1.25, `goos: darwin goarch: arm64`
- SQLite in-memory, 25 orders on one bill

### Before (full Business preload)
Command: `go test ./internal/database -run '^$' -bench BenchmarkGetOrdersByBillIDSQLite -benchmem -count=3`

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 276,251 | 695,226 | 1,463 |
| 2   | 272,735 | 695,146 | 1,462 |
| 3   | 277,110 | 695,111 | 1,462 |
| **median** | **276,251** | **695,146** | **1,462** |

### After (narrow currency projection: id/default_currency/display_currency)
Command: `go test ./internal/database -run '^$' -bench BenchmarkGetOrdersByBillIDSQLite -benchmem -count=3`

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 210,788 | 669,214 | 993 |
| 2   | 201,589 | 669,203 | 993 |
| 3   | 213,941 | 669,192 | 993 |
| **median** | **210,788** | **669,203** | **993** |

### Analysis
Narrower projection is not worse — it is measurably better (23% fewer ns/op, 4% fewer B/op, 32% fewer allocs/op). No performance regression gate needed.


## Stage 2 orders sweep — perf evidence (X-12 / B-3 / B-4 / B-6 / X-2), 2026-06-12

Recorded by reviewer (remediation for the executor's skipped-benchmark deviation #8).
Comparison: pre-Stage-2 (488cc566, temp worktree) vs post-Stage-2 (f2a64ce6), SQLite
microbenchmarks, `-benchmem -count=3`, darwin/arm64.

### `BenchmarkGetOrdersByBusinessIDPaginatedSQLite` (./internal/database)
KDS/pending poll path. The 000077 `(business_id, status)` index is Postgres-only
(migration), so SQLite measures code-path overhead only — which did not change shape.

| | ns/op (range) | B/op (median) | allocs/op (median) |
|---|---|---|---|
| pre  | 3.15–5.41ms | 4,314,877 | 7,883 |
| post | 3.72–4.88ms | 4,315,470 | 7,888 |

Overlapping ns/op ranges; +0.01% B/op, +5 allocs — noise. No regression.

### `BenchmarkOrderCreate` (./internal/handlers)
Gained B-3 availability check + B-4 caps validation + B-6 idempotency plumbing.

| | ns/op (range) | B/op (median) | allocs/op (median) |
|---|---|---|---|
| pre  | 1.59–3.24ms | 145,660 | 826 |
| post | 3.13–4.21ms | 147,186 | 828 |

+1.0% B/op, +2 allocs for the validation layer; ns/op ranges overlap run-to-run
variance on this harness. Acceptable.

### `BenchmarkOrderStatusApprove` (./internal/handlers)
Gained the conditional status UPDATE (Stage 1) + nil-guarded print enqueue (X-2;
sharedPrintService nil in bench → early return). B/op on this bench scales with
iteration count (the bill grows per approval), so compare at matched N:
pre N=244 → 1,258,666 B/op / 6,938 allocs; post N=242 → 1,252,865 B/op / 6,903 allocs
(−0.5% B/op, −35 allocs). No regression.

### Deferred
Postgres-side validation of the 000077 index (EXPLAIN on the status-filtered poll)
happens at deploy observation; index is additive `IF NOT EXISTS` with a paired down.
No sweep benchmark exists yet (sweep is env-gated off in prod); add one if the print
worker is enabled.

---

## Embedded-`Business` JSON leak on guest endpoints (SEC-2 cont., 2026-06-15)

A live prod check on `GET /api/v1/business/:customUrl/menu` showed
`bundles[0].business` and `offers[0].business` being serialized as
full 94-field `Business` zero-value structs. Schema disclosure +
payload bloat today, and a single future `.Preload("Business")` on
any guest query would turn it into a Stripe-ID + owner-PII +
payout-address leak. Same risk class as SEC-2 (Bill/Table) which
was fixed via `MarshalJSON` safe-summary helpers — this batch extends
the same protection to the remaining guest-reachable embeds.

### Decision: OMIT (not safe-summary)

Grep evidence that no consumer needs the nested `business`:

- `frontend/src/api/business.ts:347-383` — `Offer` and `Bundle` types
  have **no** `business` field.
- `frontend/src/components/business/{BundlesManager,OffersManager}.tsx`
  — never reads `.business`.
- `promotion_translations.go:67,118,292-304` — accesses
  `offer.BusinessID` / `bundle.BusinessID`, never `offer.Business.*`.
- `pricing_cache.go:101` calls `GetActiveBundlesByBusinessID` /
  `GetActiveOffersByBusinessIDAt` (`bundles.go:16-20`, `offers.go:18-30`)
  — pure `db.Where(...).Find(&bundles)`, **no** `Preload("Business")`.

So omit (`json:"-"`) is the right call: simplest, kills bloat, and
the latent risk is closed. Kept the `gorm:"foreignKey:BusinessID"`
tag so server-side Go relations still work. Mirrors the SEC-2
Bill/Table MarshalJSON pattern conceptually (a serializer decision
on the embedding model, not a global on `Business`).

### Files touched

- `backend/internal/database/models.go` — flipped
  `Offer.Business`, `Bundle.Business`,
  `BusinessGalleryImage.Business`, `BusinessOperatingHours.Business`,
  `BusinessSpecialFeature.Business` from
  `json:"business,omitempty"` to `json:"-"`. (No `*Business` pointer;
  no global `Business.MarshalJSON`; no Bill/Table touches.)
- `backend/internal/database/models_json_business_embed_test.go`
  (new) — 5 model-level tests, one per fixed model.
- `backend/internal/database/models_json_menu_bench_test.go`
  (new) — `BenchmarkMenuResponseSerialization`.
- `backend/internal/server/public_business_helpers.go` — added
  `var menuDataForBusiness = services.MenuDataForBusiness` test
  seam (mirrors existing `getPublicBusinessByCustomURL` pattern).
- `backend/internal/server/guest_handlers.go` — 4 call sites
  switched from `services.MenuDataForBusiness` to the var seam.
- `backend/internal/server/public_business_regression_test.go` —
  new endpoint regression test; existing helper now also
  auto-migrates `Offer` and `Bundle`.

### Benchmark — guest menu payload (8 offers + 8 bundles, -benchmem -count=3, Apple M3)

`BenchmarkMenuResponseSerialization` (added in `models_json_menu_bench_test.go`),
`go test ./internal/database/ -run '^$' -bench BenchmarkMenuResponseSerialization -benchmem -count=3 -benchtime=2s`.

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| **Before** (`json:"business,omitempty"`, full struct) | ~58 000 | 52 437 | 82 |
| **After** (`json:"-"`) | ~10 900 | 6 456 | 34 |
| **Δ** | **−81%** | **−88%** | **−59%** |

(Before numbers captured by temporarily reverting
`models.go` to the pre-fix tree via `git show HEAD~2` of the
fix-commit, then re-running the bench. The test file was new
in the fix commit so it had to be back-ported, then restored.)

A clean perf win on a guest hot path, as expected: dropping the
zero-value Business struct from every Bundle/Offer is the dominant
saving in both bytes and allocs.

### Sibling-embed audit (Task 6)

Every model in `backend/internal/database/models.go` that embeds
`Business Business` (or `*Business`) was classified by:
1. Whether it's serialized on a public/guest route.
2. Whether `.Business` is preloaded there.
3. Whether a custom `MarshalJSON` (or DTO projection, or manual
   payload-builder) already drops the embed.

| Line | Model | Public/guest route? | Preloaded on guest? | Custom protection? | Verdict |
| --- | --- | --- | --- | --- | --- |
| 147 | `BusinessMilestoneEvent` | no (admin analytics) | n/a | n/a | SAFE (off-path) |
| 165 | `BusinessRevenueAggregate` | no (admin analytics) | n/a | n/a | SAFE (off-path) |
| 322 | `Counter` | no (operator-only) | n/a | n/a | SAFE (off-path) |
| **372** | **`BusinessGalleryImage`** | **yes** (`/business/:customUrl`, ~L1177) | **no** (`db.Find`) | **none** | **LEAK risk → FIXED (json:"-")** |
| **385** | **`BusinessOperatingHours`** | **yes** (`/business/:customUrl`, ~L1178) | **no** (`db.Find`) | **none** | **LEAK risk → FIXED (json:"-")** |
| **399** | **`BusinessSpecialFeature`** | **yes** (`/business/:customUrl`, ~L1179) | **no** (`db.Find`) | **none** | **LEAK risk → FIXED (json:"-")** |
| 411 | `Menu` | yes (`/business/:customUrl/menu`) | no | yes — `buildPublicGuestMenuResponse` projects to `{categories: ...}` | SAFE (projected) |
| 467 | `Table` | n/a (no direct guest response; in bills) | n/a | yes — `Table.MarshalJSON` (safe summary, omit when ID==0) | SAFE (SEC-2) |
| 496 | `ReservationSettings` | yes (`/business/:customUrl/reservations/settings`) | n/a | yes — `ReservationSettingsDTO` projection drops business | SAFE (projected) |
| 531 | `TableReservation` | yes (multiple guest reservation routes) | n/a | yes — `publicReservationPayload` marshals-then-deletes `business` | SAFE (manual) |
| 582 | `Bill` | yes (guest bill) | yes (when loaded) | yes — `Bill.MarshalJSON` (safe summary, omit when ID==0) | SAFE (SEC-2) |
| 712 | `WithdrawalHistory` | no (operator-only) | n/a | yes — custom MarshalJSON for $; no business override | SAFE (off-path) |
| 790 | `ManualLedgerEntry` | no (operator-only) | n/a | yes — custom MarshalJSON for $; no business override | SAFE (off-path) |
| 831 | `PayrollRun` | no (operator-only) | n/a | yes — custom MarshalJSON for $; no business override | SAFE (off-path) |
| 854 | `PayrollLineItem` | no (operator-only) | n/a | yes — custom MarshalJSON for $; no business override | SAFE (off-path) |
| 891 | `Staff` | no (operator-only) | n/a | none | SAFE (off-path) |
| 907 | `StaffInvitation` | no (operator-only) | n/a | none | SAFE (off-path) |
| 1063 | `ReferralRecord` | no (operator-only) | n/a | none | SAFE (off-path) |
| 1140 | `Order` | yes (`CreateGuestOrder` returns it) | yes (currency resolution path) | yes — `Order.MarshalJSON` nil-overrides `business` | SAFE (explicit drop) |
| **1229** | **`Offer`** | yes (multiple guest menu/promo paths) | no | none | **LEAK risk → FIXED (json:"-")** |
| **1249** | **`Bundle`** | yes (multiple guest menu paths) | no | none | **LEAK risk → FIXED (json:"-")** |
| 1289 | `RBACAuditLog` | no (operator-only) | n/a | none | SAFE (off-path) |
| 1387 | `StripeSubscriptionPayment` | no (operator-only) | n/a | n/a — already `*Business` (pointer) | SAFE (off-path) |
| 1439 | `ReportSchedule` | no (operator-only) | n/a | none | SAFE (off-path) |
| 1513 | `CustomerBusiness` | no (customer-JWT only) | n/a | none | SAFE (off-path) |
| 1607 | `CustomerCommunication` | no (customer-JWT only) | n/a | none | SAFE (off-path) |
| 1635 | `AiWaiterConversation` | yes (AI waiter endpoints) | n/a | yes — `[]services.WaiterMessage` projection | SAFE (projected) |
| 1667 | `DirectorConsoleThread` | no (owner-only dashboard) | n/a | none | SAFE (off-path) |
| 1710 | `DirectorConsoleMessage` | no (owner-only dashboard) | n/a | none | SAFE (off-path) |

**Models fixed in this batch:** 5 (Bundle, Offer, BusinessGalleryImage,
BusinessOperatingHours, BusinessSpecialFeature). All flipped to
`json:"-"` on the `Business` embed; the gorm tag is kept so server-
side Go can still read `bundle.Business.X` if a future change wants
it. No consumer in the codebase does today — confirmed by grep
across both backend and frontend.

**Models not touched** (SAFE verdicts): 24. All either off the guest
path, already protected by a custom `MarshalJSON` (SEC-2), already
projected to a DTO before JSON serialization, or already protected
by a manual marshals-then-deletes helper. I did not change operator
or admin serialization behavior — per guardrail.

**Models I could not conclusively classify** (none in this audit —
every embed fell into one of the above buckets after the
GET /business/:customUrl audit).

### RED → GREEN summary

**Task 1** — model-level RED on the populated-embed case
(`TestBundleMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields`,
`TestOfferMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields`).
The pre-fix JSON contained every sensitive key (the
`testsOutput` log shows the full 94-field Business under
`bundles[0].business` / `offers[0].business`); the post-fix JSON
contains no `business` key on either type. GREEN on the fix
commit.

**Task 5** — endpoint RED on the wire path. The new
`TestGetMenuByBusinessCustomUrl_OmitsEmbeddedBusinessOnBundlesAndOffers`
uses the `menuDataForBusiness` test seam to inject a hand-crafted
snapshot whose Offer and Bundle carry a populated `Business`. On
the pre-fix tree (HEAD~2) the test failed with every forbidden
value/key leaking into the response body. On the post-fix tree
(GREEN) the response body carries no `business` key on either
type, and the forbidden-value/keys asserts all pass.

**Task 6** — three additional model-level tests for the
storefront sub-resources: `TestBusinessGalleryImage…`,
`TestBusinessOperatingHours…`, `TestBusinessSpecialFeature…`. All
GREEN on the fix commit; would fail on the pre-fix tree
(redundant with the bundle/offer pattern but documents the audit
explicitly).

### Commands run

```bash
cd backend

# Build + vet
go build ./...
go vet ./...

# Targeted model tests
go test ./internal/database/ -run "TestBusinessEmbed|TestBusinessGalleryImageMarshalJSON|TestBusinessOperatingHoursMarshalJSON|TestBusinessSpecialFeatureMarshalJSON" -v

# Full database suite
go test ./internal/database/ -count=1

# Server suite (covers the new endpoint regression + refactored guest handlers)
go test ./internal/server/ -count=1

# Bench
go test ./internal/database/ -run '^$' -bench BenchmarkMenuResponseSerialization -benchmem -count=3 -benchtime=2s

# Adjacent compile/no-op gate
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

All green. No vendor churn (no `*Business` pointer flips, no
`go.mod` change). No SEC-2 follow-on work needed.

## Task H1 — Lean guest bill-number resolution (JSON-01 / PRELOAD-01 / OF-03), 2026-06-17

**Problem.** `database.GetBillByNumber` (`internal/database/business.go`) hydrates
the full bill aggregate: `Preload("Business").Preload("Table").Preload("Payments")`
(unbounded), then `loadBillManagedAlternativePayments` (a 2nd SELECT) and
`billItemsForBillSnapshot` (a 3rd SELECT against `bill_items`, with a legacy
`bill.Items` text-JSON snapshot fallback). The unauthenticated guest order poll
`GET /api/v1/guest/bill/:bill_number/orders` (`handlers.GetGuestOrdersByBillNumber`)
is polled every 3000ms per seated guest yet uses **only `bill.ID`**. The plugin
payment-status caller (`PluginHandlers.GetPluginPaymentStatus`) likewise discarded
all preloads and read only `bill.ID` + `bill.BusinessID`.

**Fix.** Added two lean single-query resolvers beside `GetPublicGuestBillByNumber`:
- `GetGuestBillIDByNumber(billNumber) (uint, error)` — `Select("bills.id")` only.
- `GetGuestBillScopeByNumber(billNumber) (id, businessID uint, err error)` —
  `Select("bills.id","bills.business_id")`.

Both `JOIN businesses ON businesses.id = bills.business_id AND businesses.is_active = ?`
so a bill on a deactivated business is no longer resolvable (correctness/security
tightening over the old path, which never checked `is_active`). No relation
hydration, no items snapshot, no alt-payment load. The shared `GetBillByNumber` /
`GetBillByID` signatures and behavior were left untouched — payment submit/verify
callers depend on the full aggregate.

**Call sites repointed.**
- `handlers.GetGuestOrdersByBillNumber` → `GetGuestBillIDByNumber` (uses `bill.ID` only).
- `PluginHandlers.GetPluginPaymentStatus` → `GetGuestBillScopeByNumber` (uses
  `bill.ID` + `bill.BusinessID` only).

**Call sites intentionally LEFT on `GetBillByNumber`** (need bill amounts/status,
not just id/business_id):
- `PluginHandlers.CreateGuestBillPayment` (`plugin_handlers.go` ~1084) — passes the
  full `*Bill` to `authoritativeGuestPluginChargeAmount` (reads `bill.Status`,
  `bill.TotalAmount`, `bill.PaidAmount`) and to `reserveGuestPaymentSplitShare`.
- The since-removed stablecoin plugin's quote handler — same:
  passed the full `*Bill` to `authoritativeGuestPluginChargeAmount`. Forcing the
  lean projection there would have broken payment-charge correctness.

### Benchmark deltas (SQLite microbench, `-benchmem -count=3`)

`BenchmarkGuestOrdersBillResolve_Before` (legacy `GetBillByNumber`, exercising the
JSON-snapshot fallback) vs `BenchmarkGuestOrdersBillResolve_After`
(`GetGuestBillIDByNumber`), seeded with 1 bill + 5 payments + a 64-item bill.Items
JSON snapshot:

| Benchmark | ns/op | B/op | allocs/op | SELECTs |
| --- | --- | --- | --- | --- |
| `_Before` (GetBillByNumber) | ~200,618–324,264 | ~235,974 | 1,297 | 3 (bill+preloads, alt-payments, bill_items snapshot) |
| `_After` (GetGuestBillIDByNumber) | ~8,255–9,047 | 16,447 | 63 | 1 (projected bills.id with JOIN) |

≈ **22x faster ns/op, ~14x fewer B/op, ~20x fewer allocs/op, 3 SELECTs → 1.**

### Access-shape regression tests (new)

`internal/database/business_bill_access_test.go` — a post-`gorm:query` callback
counts SELECTs and asserts the resolvers issue **exactly 1 query** (no preloads /
snapshot / alt-payment load): `TestGetGuestBillIDByNumber`,
`TestGetGuestBillScopeByNumber`, plus not-found and inactive-business
(`is_active=false` ⇒ error) cases for each.
`internal/handlers/guest_orders_test.go` — added
`TestGetGuestOrdersByBillNumber_InactiveBusiness` (404 for a deactivated
business's bill).

### Commands run

```bash
cd backend

# TDD red (undefined symbols), then green
go test ./internal/database -run TestGetGuestBill -v          # PASS (6 tests)
go test ./internal/handlers -run GetGuestOrdersByBillNumber -v # PASS (5 tests)

# Benchmarks
go test ./internal/database -run '^$' -bench BenchmarkGuestOrdersBillResolve -benchmem -count=3

# Targeted + full suites
go test ./internal/database ./internal/handlers -run 'GuestBill|GuestOrders|Plugin' -count=1
go test ./internal/database ./internal/handlers -count=1

# Adjacent compile/no-op gate
go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
```

All green.

---

## H2 — Project the delivery-detail read (stop Stripe-ID leak to dispatch staff) (PRELOAD-02)

### Problem

`DeliveryService.GetDeliveryOrder(id)` (`internal/services/delivery_service.go`)
issued **6 bare `Preload`s** — `Business`, `Bill`, `Order`, `Driver`, `Customer`,
`StatusHistory`. It backs the operator dispatch detail route
`GET /businesses/:id/deliveries/:delivery_id` (via `GetDeliveryOrderByBusiness`),
gated by `delivery:dispatch:read` — a permission held by **low-privilege
non-owner staff (server + kitchen roles)**.

`DeliveryOrder.MarshalJSON` (`internal/database/models_json.go` ~342) only
rewrites the money fields to dollars; it does **not** strip the embedded
`Business`, and `Business` has no `MarshalJSON` of its own. So the full ~94-field
`Business` row serialized to dispatch staff: `stripe_customer_id`,
`stripe_subscription_id`, `stripe_price_id`, `stripe_payment_method`, card
brand/last4, the `onboarding_state` jsonb blob, and every subscription/financial
column. Both a **data leak** and a wide-row **over-fetch** (plus the unused
Bill/Order/Customer hydration).

### Fix (behavior-preserving except removing the leak)

`GetDeliveryOrder` now projects relations instead of bare-loading them:

- **Business** → new `preloadDeliveryDispatchBusinessSummary` selecting
  `id, business_id, name, custom_url, phone, timezone, default_currency,
  display_currency`. EXCLUDES every `stripe_*` / card field and
  `onboarding_state`. Kept slightly wider than the tracking summary (adds
  phone/timezone) so any operator surface that reads those still works; never
  includes a sensitive column.
- **Customer** (PII) → new `preloadDeliveryCustomerSummary` selecting
  `id, name, phone, email`. Drops the full customer row (`password_hash`,
  reset/verification tokens are `json:"-"` but were still hydrated into memory).
- **Driver** → new `preloadDeliveryDispatchDriverSummary` selecting
  `id, business_id, name, phone, email`. Mirrors the tracking driver summary but
  **adds `email`**, which the operator `DispatchConsole` falls back to
  (`o.driver.name ?? o.driver.email`); the public tracking summary omits it.
- **StatusHistory** → kept bare (own audit rows; carries no cross-entity PII).
- **Bill / Order → DROPPED.**

`GetDeliveryOrderByNumber` (public guest tracking) was left untouched — it was
already correctly projected.

### Bill / Order keep-vs-drop decision: DROPPED — FE evidence

Conclusively proven the operator dispatch UI ignores both relations:

- The operator `DeliveryOrder` TypeScript interface
  (`frontend/src/api/delivery.ts` ~247–282) declares only `zone` and `driver` as
  embedded relations — **no `business`, `bill`, `order`, or `customer` field**.
  (The `bill?`/`business_name` fields live in the separate guest
  `PublicDeliveryTrackingDto`, served by the unrelated public tracking path.)
- `grep` over `frontend/src/components/business/delivery/**`,
  `PendingOrdersSection.tsx`, and the dashboard page found **no** read of
  `delivery.business`, `delivery.bill`, `delivery.order`, or `delivery.customer`.
  Dispatch components read only flat columns (`customer_name`, `customer_phone`,
  `delivery_address.city/country`, `delivery_fee`, `status`, `cutoff_at`) and the
  `driver` relation.
- The sibling **list** endpoint `GetBusinessDeliveries` — what the DispatchConsole
  actually renders — already preloads **only `Driver`**. Dropping Bill/Order from
  the detail read brings its shape into parity with the list shape rather than
  diverging from it.

### Benchmark deltas (SQLite microbench, `-benchmem -count=3`)

`BenchmarkGetDeliveryOrder` over one delivery with all relations
(Business+Bill+Order+Driver+Customer+1 StatusHistory):

| State | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before (6 bare Preloads) | ~264,504–314,929 | 184,552 | 1,588 |
| After (projected, Bill/Order dropped) | ~81,870–110,905 | 122,346 | 747 |

≈ **2.7–3.5x faster ns/op, −34% B/op, −53% allocs/op.**

### Access-shape regression test (new)

`internal/services/delivery_service_access_test.go` —
`TestGetDeliveryOrder_DoesNotLeakStripeFields` seeds a Business with non-empty
`StripeCustomerID/StripeSubscriptionID/StripePriceID/StripePaymentMethod/
StripeCardLast4` and a non-empty `OnboardingState`, then asserts each is empty on
`got.Business` while `Name`/`DefaultCurrency` survive, the projected `Driver.Name`
is present, and `Customer.PasswordHash` is never hydrated. Verified RED before the
fix (`Should be empty, but was cus_LEAKME`), GREEN after.

### Commands run

```bash
cd backend

# TDD red → green
go test ./internal/services -run TestGetDeliveryOrder_DoesNotLeakStripeFields -v  # RED, then PASS

# Benchmarks (before captured pre-fix, after captured post-fix)
go test ./internal/services -run '^$' -bench BenchmarkGetDeliveryOrder -benchmem -count=3

# Full delivery suite + adjacent compile/no-op gate
go test ./internal/services -run 'Delivery' -count=1
go test ./internal/services ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
go vet ./internal/services/
```

All green.

## H3 — Topic-filter the guest split-payment SSE so unrelated events can't drop split frames (SSE-01), 2026-06-17

### Root cause

`StreamSplitEvents` (`internal/handlers/splitting.go:619`) subscribed to the
**whole-business** event hub via `hub.Subscribe(state.BusinessID)` /
`hub.SubscribeWithReplay(...)`, so the guest's 64-slot channel received EVERY
event type for that restaurant (orders, payments, deliveries, reservations,
alerts) and filtered per-bill in-process via `splitEventMatchesBill` (which only
accepts `bill.split.updated`). `Hub.Publish` is non-blocking: on a full channel
it silently drops + counts (`recordDroppedBusinessEvent`). So an unrelated burst
(e.g. a flurry of `order.updated`) could saturate the channel and **evict the
guest's own `bill.split.updated` frame** — a paying guest silently misses the
split update on a public payment surface. Plus O(guests × events) wasted sends.

### Fix (Approach A — additive topic filter on the existing hub)

`internal/events/hub.go`:
- Added an immutable `topics map[string]struct{}` to `subscriber` (nil/empty ⇒
  accept ALL types, preserving legacy behavior) plus `newSubscriber` and a
  `(*subscriber).matches(eventType)` helper.
- Added `SubscribeTopic(businessID, types...)` and
  `SubscribeWithReplayTopic(businessID, lastID, types...)`. The legacy
  `Subscribe`/`SubscribeWithReplay` now delegate to the topic variants with NO
  types (empty set), so their behavior is byte-for-byte unchanged.
- **Publish skip happens after the subs slice is copied, outside `h.mu`**: the
  fan-out loop does `if !sub.matches(event.Type) { continue }` before the
  channel send, so unrelated types NEVER enter a topic subscriber's channel and
  therefore cannot starve the subscribed types. The `topics` map is set once at
  subscribe time and never mutated, so this lock-free read is safe (documented
  as an immutability invariant on the field).
- The replay snapshot is filtered to the topic set inside
  `SubscribeWithReplayTopic` (filter-in-place on the fresh slice
  `replayAfterLocked` returns — no aliasing of the replay buffer). `ReplayAfter`
  and the un-topic'd `replayAfterLocked` are untouched.
- The `cancel()` no-`close(sub.ch)` subtlety is preserved verbatim.

`internal/handlers/splitting.go`: repointed `StreamSplitEvents` to
`hub.SubscribeTopic(state.BusinessID, "bill.split.updated")` /
`hub.SubscribeWithReplayTopic(state.BusinessID, lastEventID, "bill.split.updated")`.
The in-process `splitEventMatchesBill` per-`bill_number` check is KEPT (one
business can host many split bills at once; the topic filter removes only the
cross-TYPE firehose, not the per-bill discrimination).

### Tests (new) — `internal/events/hub_topic_test.go`

- `TestSubscribeTopic_NotStarvedByUnrelatedBurst` (**load-bearing**): publish 200
  unrelated `order.updated` (> buffer 64), then ONE `bill.split.updated`; assert
  it is received within 2s and is the ONLY queued event. Verified RED first
  (`SubscribeTopic undefined`), GREEN after.
- `TestSubscribeTopic_DropsUnrelatedTypes` — topic subscriber gets only its type.
- `TestSubscribeTopic_MultipleTypes` — multi-topic set works, nothing outside it.
- `TestSubscribe_StillReceivesAllTypes` — un-topic'd `Subscribe` still gets all
  types (no regression).
- `TestSubscribeWithReplayTopic_FiltersReplay` — replay snapshot limited to topic.
- `TestSubscribeWithReplay_StillReplaysAllTypes` — un-topic'd replay unchanged.
Existing `hub_test.go`, `hub_cancel_race_test.go`, and the operator
`TestSSEHandler_*` tests all still pass.

### Benchmark deltas (`-benchmem -count=3`, Apple M3)

Mixed load where 4 of 5 events are unrelated (`order.updated`×3,
`payment.received`, `bill.split.updated`), subscriber drained on the fly:

| Bench | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkPublishMixed_Before` (un-topic'd Subscribe) | 122.3–133.1 | 168 | 1 |
| `BenchmarkPublishMixed_After` (topic subscriber) | 90.85–101.0 | 168 | 1 |

≈ **25% faster ns/op** — unrelated types skip the channel send. B/op/allocs are
identical (the 1 alloc is the replay-buffer append, which both paths still do;
the win is removed channel sends + zero starvation risk for the guest's frame).

### Commands run

```bash
cd backend
go test ./internal/events -run TestSubscribeTopic -v   # RED (undefined), then PASS
go test ./internal/events -race -count=1               # full suite + cancel-race, green
go test ./internal/handlers -run 'Split' -count=1      # StreamSplitEvents tests green
go test ./internal/events -run '^$' -bench BenchmarkPublishMixed -benchmem -count=3
go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
go vet ./internal/events/ ./internal/handlers/
```

All green.


## M2 — Reuse hydrated staff custom_permissions from context (MW-01), 2026-06-17

**Finding MW-01.** `hydrateLiveStaffContext` (`internal/server/middleware.go`)
already loads the full staff row via `StaffService.GetByID` — which includes
`CustomPermissions` — and stashed `staff_id/email/name/role/business_id/token_type`
into the gin context, but NOT the custom permissions. Then on every
permission-checked staff request `getStaffPermissions` → `getCustomPermissions`
issued a SECOND full-row `r.db.GetGorm().First(&staff, id)` solely to read the
`custom_permissions` JSON string. That is a redundant per-request staff SELECT on
every polled dashboard endpoint.

### Fix

- `middleware.go`: after the staff fetch, `c.Set("staff_custom_permissions", staff.CustomPermissions)`
  (pure stash of the raw JSON string; no parsing in middleware).
- `rbac.go`: `getCustomPermissions` now takes `(c *gin.Context, staffID interface{})`.
  - Hot path: read `c.Get("staff_custom_permissions")`. `exists==true` is
    authoritative — even a `""` value means "no custom perms" and issues ZERO
    queries. Type assertion guarded (`raw, ok := v.(string)`).
  - Fail-safe: only on context MISS (a caller that did not run
    hydrateLiveStaffContext) fall back to a NARROW query
    `r.db.GetGorm().Select("custom_permissions").First(&staff, id)` — never the
    full row.
  - JSON parse extracted to `parseCustomPermissions(raw string)`.
- Only caller updated: `getStaffPermissions` (the sole caller, verified via grep)
  now passes `c`.

Behavior (which permissions resolve) is identical; only the data source changes
(context vs second query). Full `internal/server` suite stays green.

### Tests (new) — `internal/server/rbac_custom_perms_test.go`

- `TestGetCustomPermissions_UsesContextNoQuery` (**load-bearing**): context has
  `staff_custom_permissions=["orders:write"]`; asserts perms returned AND a
  `gorm:query` Before-callback counter stays at **0**. Verified RED first
  (signature mismatch: `too many arguments in call to r.getCustomPermissions`),
  GREEN after.
- `TestGetCustomPermissions_EmptyContextValueNoQuery` — present-but-`""` value →
  nil, **0** queries (empty is authoritative, not a miss).
- `TestGetCustomPermissions_ContextMissFallsBackToNarrowQuery` — no context key,
  seeded staff row; asserts perms returned, **exactly 1** query, and the captured
  SQL (`db.Statement.SQL.String()` After-callback) contains `custom_permissions`
  but NOT `email`, `invited_by`, or `pin_hash` (narrow-projection proof — the
  full row is gone).

### Benchmark deltas (SQLite microbench, `-benchmem -count=3`, Apple M3)

| Bench | ns/op | B/op | allocs/op | queries/call |
| --- | --- | --- | --- | --- |
| Baseline (old full-row `First`, throwaway) | 13165–16172 | 8872 | 138 | 1 (full row) |
| `BenchmarkGetStaffPermissions_Before` (context-miss, narrow fallback) | 6294–7162 | ~9294 | 83 | 1 (custom_permissions only) |
| `BenchmarkGetStaffPermissions_After` (context-hit, pure parse) | 822.1–1012 | 1856 | 22 | **0** |

The After bench self-asserts zero queries (`b.Fatalf` if the `gorm:query` counter
is non-zero). Hot path is ~8x faster than the old baseline and ~6.6x faster than
the context-miss fallback, with zero DB round-trips. Even the fallback path is
~2x faster than the old full-row query thanks to the narrowed projection.

### Commands run

```bash
cd backend
# TDD red (signature mismatch), then green
go test ./internal/server -run 'CustomPermissions' -v
# Baseline (throwaway bench against old signature), then before/after
go test ./internal/server -run '^$' -bench 'BenchmarkGetStaffPermissions_(Before|After)' -benchmem -count=3
# Full server suite (auth-adjacent — nothing regresses)
go test ./internal/server -count=1
# Adjacent compile/no-op gate + build + vet
go test ./internal/handlers ./internal/database ./internal/services -run '^$' -count=1
go build ./...
go vet ./internal/server/
```

All green.

---

## M4 — Column-minimal Director memory prior-turns loader (JSON-02, 2026-06-17)

### Finding

`loadPriorTurns` (`internal/services/director_console_service.go:948`) called
`database.ListDirectorConsoleMessages(businessID, threadID, 12)`, which projects
the multi-KB `structured_response` text column. `buildPriorTurns` (the sole
consumer, `internal/services/director_memory.go:27`) reads ONLY `ID`, `Role`,
and `Content` — the blob was fetched, scanned into memory, and immediately
dropped on every Director Ask. The transcript path at line 386 (`ListThreadMessages`)
genuinely needs `structured_response` and was left untouched.

### Fix

- **New function** `ListDirectorConsoleMessageSummaries(businessID, threadID uint, limit int)` in
  `internal/database/director_console.go` — identical to `ListDirectorConsoleMessages`
  except `Select` covers only `id, thread_id, business_id, role, locale, content, created_at`
  (omits `structured_response`, `model_name`, `latency_ms`, `feedback_vote`, `feedback_at`).
  Same limit clamping (≤0 → 200, >500 → 500), same `Order("created_at DESC, id DESC")`,
  same ascending reversal.
- `loadPriorTurns` repointed to `ListDirectorConsoleMessageSummaries`.
- `ListDirectorConsoleMessages` and the transcript handler unchanged.

### Tests (new) — `internal/database/director_console_summaries_test.go`

- `TestListDirectorConsoleMessageSummaries_OmitsStructuredResponse` (**load-bearing**):
  seeds 6 messages with ~4KB `structured_response` blobs; calls
  `ListDirectorConsoleMessageSummaries`; asserts every row has `StructuredResponse == ""`
  while `ID`, `Role`, `Content` are populated and results are in ascending order.
  SQL-capture via `directorSQLRecorder.Trace` asserts generated SQL does NOT
  contain `"structured_response"`. Verified RED first (undefined function), GREEN after.
- `TestListDirectorConsoleMessages_IncludesStructuredResponse` — proves the full
  loader DOES SELECT `structured_response` (differential proof).
- `TestListDirectorConsoleMessageSummaries_AscendingOrder` — limit clamping +
  chronological order with limit < seeded count.

### Benchmark deltas (SQLite microbench, `-benchmem -count=3`, Apple M3)

| Bench | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkListDirectorConsoleMessages_Before` (full blob) | 72687–80545 | 155,839 | 387 |
| `BenchmarkListDirectorConsoleMessageSummaries_After` (no blob) | 48404–49765 | 113,052 | 291 |

B/op reduction: ~27% fewer bytes (−42.8 KB/op across 12 msgs), −96 allocs/op,
~33% faster per call. Gap scales with production blob size (assistant
`structured_response` blobs are typically 1–8KB each; 12 messages × ~4KB = ~48KB
avoided per Director Ask).

### Commands run

```bash
cd backend
# TDD red (undefined: ListDirectorConsoleMessageSummaries)
go test ./internal/database -run 'TestListDirectorConsoleMessageSummaries_OmitsStructuredResponse' -v
# Implement fix, then green
go test ./internal/database -run 'TestListDirectorConsoleMessageSummaries|TestListDirectorConsoleMessages_IncludesStructuredResponse|TestListDirectorConsoleMessageSummaries_AscendingOrder' -v
# Benchmarks
go test ./internal/database -run '^$' -bench 'BenchmarkListDirectorConsoleMessages_Before|BenchmarkListDirectorConsoleMessageSummaries_After' -benchmem -count=3
# Director regression suite
go test ./internal/services ./internal/database -run 'Director' -count=1
# Adjacent compile/no-op gate + build
go test ./internal/handlers ./internal/server -run '^$' -count=1
go build ./...
```

All green.

---

## M5 — Reservation reminder sweep: project business columns + remove write-on-read in settings load (SCHED-01)

**Finding:** The hourly `ReservationReminderService.ProcessUpcomingReservations` sweep was a `1 + 2N` query shape:
1. `getBusinessesWithReservationsEnabled` (`internal/services/reservation_reminder.go`) did `Find(&businesses)` with **no `Select`** → a `SELECT *` hydrating the full ~115-column `Business` row (`onboarding_state` JSON, `welcome_message`/`about_story` text, every `stripe_*`/`ai_*`/`design_*` blob) for every reservation-enabled business, every hour, to read ~10 scalars.
2. Per business, `processBusinessReservations` called `database.GetReservationSettings`, which on `needsUpdate` (from `normalizeReservationSettings`) does `db.Save(&settings)` — a **hidden UPDATE inside a read sweep**, per business, per hour (and a `db.Create` on not-found).

### Fix
- Added a narrow `.Select(...)` to `getBusinessesWithReservationsEnabled` (qualified `businesses.<col>` to disambiguate from the joined `reservation_settings`).
- Added `GetReservationSettingsForRead(businessID uint) (*ReservationSettings, error)` in `internal/database/reservations.go`: `First`s the row, returns in-memory `defaultReservationSettings` on not-found (no `db.Create`), normalizes in memory but **discards** `normalizeReservationSettings`' needsUpdate result (no `db.Save`).
- Repointed the sweep (`reservation_reminder.go`) from `GetReservationSettings` → `GetReservationSettingsForRead`. The persist variant `GetReservationSettings` is **untouched** and still used by the settings editor and all other callers (`ai_waiter_handler.go`, `whatsapp_manager.go`, `reservation_handlers.go` ×4, `reservation_service.go` ×4).

### Final Business projection column set + evidence
Every column the sweep reads off `*database.Business` (traced from `processBusinessReservations` + `sendReminderEmail` + `FormatBusinessAddress`):

| Column | Read at | Why |
| --- | --- | --- |
| `id` | `getReservationsNeedingReminders(business.ID)`, settings load, claim/release | scope key |
| `name` | `sendReminderEmail` → `business.Name` | email body |
| `phone` | `sendReminderEmail` → `business.Phone` | email body |
| `timezone` | `sendReminderEmail` → `FormatReservationEmailDateTime(.., business.Timezone, ..)` | date/time render |
| `default_language` | `sendReminderEmail` → `business.DefaultLanguage` | email language |
| `street`,`city`,`state`,`postal_code`,`country` | `FormatBusinessAddress(business)` → embedded `business.Address.*` | email address line |

**Dropped from the plan's starting set** (`business_id`, `owner_name`, `email`): none are read by the sweep — URLs come from `config.FrontendBaseURL()`, not `business.CustomURL`. **Added vs the plan's starting set:** the embedded `BusinessAddress` columns (`street`,`city`,`state`,`postal_code`,`country`) — the plan omitted them but `FormatBusinessAddress` reads them, so omitting them would have blanked the address line in reminder emails (correctness bug avoided).

### Tests
- `TestGetReservationSettingsForReadDoesNotPersistOnNeedsUpdate` (`internal/database/reservation_settings_read_test.go`) — **load-bearing**: seeds a row forced to `max_advance_days = 0` (raw UPDATE, because a struct-literal `0` is omitted in favor of the `default:30` column tag), registers `gorm:create`/`gorm:update` fail-fast callbacks scoped to `reservation_settings`, calls `GetReservationSettingsForRead`, asserts the returned settings are normalized in memory (`MaxAdvanceDays == 30`) AND the persisted row is still `0` (no write). Verified RED first ("read variant must not persist normalized values" + undefined func), GREEN after.
- `TestGetReservationSettingsForReadReturnsDefaultsWithoutCreate` — not-found returns in-memory defaults with `count == 0` rows created; write callbacks must not fire.
- `TestGetReservationSettingsPersistsOnNeedsUpdate` — contrast: the persist variant `GetReservationSettings` DOES write the normalized value back (editor path intact).
- `TestGetBusinessesWithReservationsEnabledProjectsLeanColumns` (`internal/services/reservation_reminder_projection_test.go`) — captures generated SQL via a `logger.Interface.Trace` recorder; asserts the `businesses` SELECT does NOT contain `select *`/`onboarding_state`/`welcome_message`/`about_story`, DOES contain every required column, AND functionally that heavy blobs (`WelcomeMessage`,`AboutStory`) are empty on the hydrated struct while name/phone/timezone/language/address are populated. Verified RED first (SELECT * dragged all ~115 columns incl. the blobs), GREEN after.

### Benchmark deltas (SQLite microbench, 50 businesses w/ heavy blobs + needsUpdate, `-benchmem -count=3`, Apple M3)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before (`SELECT *` + persist variant) | 2,121,408–3,161,974 | 1,308,524 | 17,515 |
| After (projection + `GetReservationSettingsForRead`) | 1,157,531–1,285,141 | 561,520 | 9,160 |

B/op **−57%** (−747 KB/op from dropping JSON/text blobs); allocs/op **−48%**; ~45–60% faster. **Query count:** settings load drops from `2N` (N SELECT + N hidden UPDATE) to `N` (N SELECT) — 50 hidden per-sweep UPDATEs eliminated; the business load drops from a full-row `SELECT *` to a 10-column projection.

### Commands run
```bash
cd backend
# TDD red
go test ./internal/database -run 'TestGetReservationSettingsForRead|TestGetReservationSettingsPersistsOnNeedsUpdate' -v   # undefined func
go test ./internal/services -run 'TestGetBusinessesWithReservationsEnabledProjectsLeanColumns' -v                         # SELECT * fails projection asserts
# Baseline benchmark (before code change)
go test ./internal/services -run '^$' -bench 'BenchmarkReminderSweepBusinessLoadSQLite' -benchmem -count=3
# Implement, then green
go test ./internal/database -run 'TestGetReservationSettingsForRead|TestGetReservationSettingsPersistsOnNeedsUpdate' -v
go test ./internal/services -run 'TestGetBusinessesWithReservationsEnabledProjectsLeanColumns' -v
go test ./internal/services -run '^$' -bench 'BenchmarkReminderSweepBusinessLoadSQLite' -benchmem -count=3
# Reservation regression suite + adjacent gate + build
go test ./internal/services ./internal/database -run 'Reservation' -count=1
go test ./internal/handlers ./internal/server -run '^$' -count=1
go build ./...
```

All green.

---

## M3 — Drop dead `resolveBill`; lean the payment-settle bill reload (OF-03, payment-caller portion)

**Files changed**
- `internal/handlers/payments.go` — deleted the dead `resolveBill`; repointed the two settle-path reloads (crypto + cross-chain) from `GetBillByID` → `GetBillByIDLean`.
- `internal/database/business.go` — `GetBillByIDLean` reduced to a single bill-row SELECT (no items-snapshot second query).
- `internal/database/bill_lean_reload_access_test.go` — new access-shape test + before/after benchmarks.

### Dead code removed
- `(*PaymentHandler).resolveBill` (was `payments.go:1148`) had the definition and **zero callers** (`grep -rn "resolveBill\b"` showed only the def). The live crypto/alternative submit+verify paths use `resolveBillPaymentSummary` (6-column projection) at 770/919/945. Deleted the function only.
- Helper audit: `resolveBill` called `database.GetBillByNumber`, `firstNonEmptyString`, `h.db.GetBill`, and referenced `errInvalidBillIdentifier`. **All four are shared** (`errInvalidBillIdentifier` + `firstNonEmptyString` are used by `resolveBillPaymentSummary`; `h.db.GetBill` used at 530/616). Nothing became exclusively-dead, so only `resolveBill` itself was removed.
- Dead-code guard = the compile gate: `go build ./...` and `go vet ./internal/handlers ./internal/database` are clean (a missed reference would not compile). No `deadcode` tool relied on.

### Reload decision: SAFE to lean — no consumer reads a preloaded relation
The reload at `payments.go` (crypto ~1546, cross-chain ~1730) fires ONLY on the final settling payment (`if applied && bill.Status == BillStatusPaid`) and feeds three consumers + the post-reload block. Every field read off the reloaded `bill`, traced in full:

| Consumer / post-reload site | Reads off `bill` | Relation needed? |
| --- | --- | --- |
| `recordCRMSettlementVisitForPaidBill(bill, src)` | `bill.Status`, `bill.ID` | none |
| `sendPaymentCompletionEmails(bill, bill.CRMCustomerID, ..)` | `bill.BusinessID`, `bill.ID`, `bill.TotalAmount`, `bill.BillNumber`, `bill.CRMCustomerID`; **re-fetches** business via `GetBusinessByID`, items via `GetBillItems`, customer/connection via own queries | none (does NOT touch `bill.Business`/`bill.Table`/`bill.Payments`/item snapshot) |
| `enqueueFiscalJobForPaidBill(bill, nil, src)` → `...WithAlternativePayment` | `bill.Status`, `bill.ID` | none |
| `flushPendingMilestones(bill.BusinessID)` | `bill.BusinessID` | none |
| `events.GetHub().PublishJSON(...)` | `bill.BusinessID`, `bill.ID` | none |
| `publishGuestSplitStateForBill(bill)` | `bill.BillNumber`, `bill.ID` (re-queries split state by number) | none |
| `createPaymentReceivedOperationalAlertForTxHash(...)` → `CreatePaymentReceivedAlertForResource(*bill,...)` | `bill.ID`, `bill.BillNumber`, `bill.BusinessID` | none |
| JSON response | `bill.BillNumber`, `bill.Status`, `bill.TotalAmount`, `bill.PaidAmount` | none |

All scalar. The reload still happens (fresh post-settlement columns are required because the in-memory bill is the stale lean payment-summary projection) — only the aggregate hydration (`Preload Business/Table/Payments` + `loadBillManagedAlternativePayments` + `billItemsForBillSnapshot`) is removed. CRM attribution, completion emails, and fiscal enqueue behavior unchanged.

### `GetBillByIDLean` change
Was: bill SELECT + `billItemsForBillSnapshot` (a second bill_items SELECT + legacy text-JSON fallback). Now: a single `db.First(&bill, id)` returning `(*Bill, nil, error)`. The `[]BillItem` slot is kept for signature stability; the only other caller (`splitting.go:954`) already discards items (`bill, _, err`) and reads only `bill.TotalAmount/PaidAmount/Status`, so dropping the second query is strictly better there too. No test referenced `GetBillByIDLean` before this slice.

### Tests
- `TestGetBillByIDLeanIsSingleQueryNoRelations` (`internal/database/bill_lean_reload_access_test.go`) — seeds a business carrying Stripe-style IDs + heavy `bill.Items` JSON blob + Table + 5 Payments + 1 managed alt-payment; registers a `gorm:query` post-callback counter; calls `GetBillByIDLean`. Asserts **exactly 1 query**, scalar columns populated (`ID/BusinessID/BillNumber/Status/TotalAmount/PaidAmount`), and `bill.Payments`/`bill.AlternativePayments`/`bill.Business.ID`/`bill.Table.ID`/returned-items all zero/empty. Verified RED first (2 queries + non-empty items), GREEN after.
- Dead-code proof = compile gate (stated above). Live resolver documented as `resolveBillPaymentSummary`.

### Benchmark deltas (SQLite microbench, fully-related paid bill, `-benchmem -count=3`)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before (`GetBillByID`, full hydration) | 214,862–303,315 | 252,504–252,613 | 1,488 |
| After (`GetBillByIDLean`, single SELECT) | 17,877–17,973 | 14,949–14,950 | 166 |

Latency **~12–16x faster**; B/op **−94%**; allocs/op **−89%**. **Query count:** settle reload drops from 4 queries (bill+preloads bundled, alt-payment load, items snapshot) to **1**.

### Commands run
```bash
cd backend
# TDD red
go test ./internal/database -run 'TestGetBillByIDLeanIsSingleQueryNoRelations' -count=1     # 2 queries + items → FAIL
# Baseline (before code change)
go test ./internal/database -run '^$' -bench 'BenchmarkSettleBillReload_Before' -benchmem -count=3
# Implement (delete resolveBill; lean GetBillByIDLean; repoint 2 reload sites), then:
go build ./... && go vet ./internal/handlers ./internal/database
go test ./internal/database -run 'TestGetBillByIDLeanIsSingleQueryNoRelations' -count=1     # GREEN
go test ./internal/database -run '^$' -bench 'BenchmarkSettleBillReload_After' -benchmem -count=3
# Money-path regression suite (232 tests) + adjacent compile gate
go test ./internal/handlers -run 'Payment|Bill|Crypto|Settle|Split' -count=1               # ok (232 tests)
go test ./internal/server ./internal/database ./internal/services -run '^$' -count=1
```

All green.

---

## Webhook ingest + guest checkout + table-status poll — lean projection sweep (2026-07-02)

### Scope
- PSP plugin webhook settlement (`internal/handlers/plugin_handlers.go`)
- Public plugin checkout bill/business loads (`internal/handlers/plugin_handlers.go`)
- Bill-handler RBAC business reloads (`internal/server/business_handlers.go`)
- Table-status polling payload (`internal/database/table.go`)

### What changed
- Added `GetBillBusinessIDByBillID` to resolve `bills.business_id` without hydrating the full bill aggregate.
- Added `GetGuestPluginCheckoutBillByNumber` and `GetGuestPluginCheckoutBusiness` so public plugin checkout no longer loads the full bill/business rows just to compute charge amount, return URLs, metadata, and currency.
- Repointed webhook settlement to use lean bill/business-id loads for:
  - business-id backfill from `bill_id`
  - PAY-1 cross-business ownership check
  - partial-refund alert bill lookup
  - pre-`ApplyConfirmedPayment` payable-state check
  - already-recorded payment reload
- Reused preloaded/projected bill business data for operator bill RBAC checks instead of re-fetching `GetBusinessByID` on every bill read/update/add-item/adjust/close path.
- Narrowed `GetTablesWithStatus` to project:
  - tables without `qr_code`
  - reservations without email/notes/special-requests blobs
  - existing active-bill narrow projection unchanged

### Why
These are hot paths:
- webhook settlement is externally triggered and latency-sensitive
- guest plugin checkout is on the public payment path
- table status is polled repeatedly by the operator UI
- operator bill handlers were doing redundant business reloads after `GetBillByID` had already preloaded the business auth fields they need

### Query-shape regression updates
- `TestWebhookIngestQueryShape` tightened from:
  - bills `<= 4` → `<= 3`
  - payments `<= 5` → `<= 3`
  - alternative_payments `<= 4` → `<= 2`
- `TestGetTablesWithStatusProjectsActiveBills` now also asserts the poll query does **not** select:
  - `tables.qr_code`
  - `table_reservations.customer_email`

### Benchmark deltas

#### `BenchmarkSettlePluginWebhook` (`./internal/handlers`, SQLite, `-benchmem -benchtime=5x`)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before | 15,951,525 | 683,419 | 4,078 |
| After | 7,935,975 | 683,089 | 4,088 |

Webhook settlement latency improved by **~50%**. Memory was effectively flat in this microbench because the remaining synchronous side effects still dominate allocations, but the redundant heavy bill loads were removed and the query-shape caps tightened materially.

#### `BenchmarkGetTablesWithStatusSQLite` (`./internal/database`, SQLite, `-benchmem -count=3`)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before | 43,317,705–87,342,510 | 15,393,037–15,394,061 | 46,001–46,005 |
| After | 70,348,425–150,599,174 | 14,510,314–14,511,294 | 35,627–35,634 |

This slice produced a clear **payload-allocation win** on the poll path:
- B/op improved by roughly **5.7%**
- allocs/op improved by roughly **22.5%**

Latency variance was noisy on the local SQLite run, but the result shape is still strictly narrower and the tests now lock the intended projection (`no qr_code`, `no reservation email blob`) so future polling work starts from a leaner baseline.

### Commands run
```bash
cd backend

# Webhook query-shape guard
go test ./internal/handlers -run TestWebhookIngestQueryShape -count=1

# Table-status projection guard + bill-handler smoke tests
go test ./internal/database -run TestGetTablesWithStatusProjectsActiveBills -count=1
go test ./internal/server -run 'TestGetBill|TestUpdateBill|TestAddBillItem|TestCloseBill' -count=1

# Before/after benchmarks
go test ./internal/handlers -run '^$' -bench BenchmarkSettlePluginWebhook -benchmem -benchtime=5x
go test ./internal/database -run '^$' -bench BenchmarkGetTablesWithStatusSQLite -benchmem -count=3
```

### Follow-ups still worth doing
- Move post-settlement Telegram / operational-alert / fiscal fan-out off the webhook request goroutine; that should unlock the next meaningful webhook latency reduction.
- Add a dedicated benchmark for `CreatePluginPayment`; the code path is now leaner, but it still lacks an explicit before/after perf harness.

---

## L-CRM — CRM customer aggregates, batched tier updates, list preload (UB-01 / UB-02 / UB-03 / N1-02 / PRELOAD-05)

**Files changed**
- `internal/crm/handlers.go` — `GetSegments` row-scan + Go loops → single SQL `SUM(CASE WHEN ...)` aggregate (UB-01); `PreviewLoyalty` full-row `Find` → `Pluck("total_spent")` projection (UB-02); `PutLoyalty` per-row UPDATE loop → projected read + Go bucketing + one batched `UPDATE ... WHERE id IN (?)` per distinct tier (UB-03 / N1-02).
- `internal/crm/service.go` — `GetBusinessCustomers` (list) drops `Preload("Customer.Preferences")`; detail reader `GetCustomerBusinessDetails` keeps it (PRELOAD-05).
- `internal/crm/l_crm_access_shape_test.go` — new access-shape tests + before/after benchmarks.

### UB-01 — `GetSegments` single SQL aggregate
Was: `Select("total_spent, visit_count, last_visit_at, first_visit_at").Find(&customers)` then two Go loops (one to sum/avg, one to bucket). Now: one `SUM(CASE WHEN ...)` aggregate. The day-boundary thresholds (`int(now.Sub(t)/day) > N`) are converted to absolute UTC timestamp cutoffs in Go (`daysSince > N ⟺ event ≤ now-(N+1)d`), so the SQL stays portable (no FILTER, no DB-specific date math) and runs on SQLite (tests) and Postgres (prod). avgSpend (for the VIP band) is inlined as a `COALESCE(AVG(total_spent),0)` scalar subquery so the whole computation is one query. The CASE nesting reproduces the original priority lapsed > atRisk > vip > new exactly; "regular" rows are not emitted.
- **UTC gotcha:** cutoffs must be `time.Now().UTC()`. SQLite stores datetimes as RFC3339 UTC text (`...Z`) and compares them lexicographically; a local-offset (`-0300`) bind param sorts wrong at day boundaries. `.UTC()` is semantically identical on Postgres (timestamptz) and fixes the SQLite text compare.

### UB-02 — `PreviewLoyalty` projection
Was: `Where(...).Find(&customers)` hydrating full `CustomerBusiness` rows (incl. `notes`/tags/json blobs) to read only `TotalSpent`. Now: `Model(...).Pluck("total_spent", &spends)`. Output (`total_customers` + `tier_distribution`) unchanged.

### UB-03 / N1-02 — `PutLoyalty` batched tier recompute
Was: `Find(&customers)` (full rows) + a per-row `UPDATE customer_businesses SET loyalty_tier=? WHERE id=?` inside a Go loop (the N+1 write). Now: project `id, total_spent, loyalty_tier`; compute each row's target tier in Go via the unchanged pure `computeTier`; collect changed-row IDs into a `map[tier][]id`; issue ONE `UPDATE ... WHERE id IN (?)` per distinct tier (bounded ~3–5).
- **Correctness preserved:** (a) `computeTier` is a pure function of `total_spent`, so each row lands in exactly one bucket — disjoint, no double-update; (b) sub-lowest-tier customers get `computeTier == ""` → bucketed under `""` → `loyalty_tier` reset to `""`, matching the original loop's reset; (c) skip-unchanged retained (`if tier != r.LoyaltyTier`), so `updated_at` on unchanged rows is untouched exactly as before.

### PRELOAD-05 — CRM customer LIST drops Preferences preload
`GetBusinessCustomers` (list cards) no longer `Preload("Customer.Preferences")` (the list renders identity + per-business loyalty/visit columns only). `GetCustomerBusinessDetails` (drawer) still preloads it.

### Tests (all RED first, then GREEN)
- `TestGetSegments_SingleAggregateQueryAndCounts` — seeded 6-row distribution hitting each band + a noise business; asserts the four counts AND that there is exactly 1 SELECT over customer_businesses and it uses a SQL aggregate (`sum(case`). RED before: 0 aggregate reads (Go loops over a projected scan).
- `TestGetSegments_MatchesLegacyGoLoop` — 11-row distribution incl. 30/90-day and exactly-91-day boundaries; recomputes the expectation with the verbatim original Go switch and asserts the handler matches band-for-band. RED before (lapsed/vip off by one) until the UTC cutoff fix.
- `TestPreviewLoyalty_ProjectsTotalSpentOnly` — asserts no `select *` and no `notes` column over customer_businesses. RED before: `SELECT * FROM customer_businesses`.
- `TestPutLoyalty_BatchedTierUpdatesAndCorrectness` — 7 customers across 3 tiers + a $0 reset case; asserts ≤3 customer_businesses UPDATEs AND final `loyalty_tier` per row matches the legacy per-row `computeTier`. RED before: 7 per-row UPDATEs.
- `TestPutLoyalty_ResetsToNoTierWhenNoLowestQualifies` — ladder starting at $200; a $50 customer resets to `""`, a $250 lands on Silver.
- `TestPutLoyalty_SkipsUnchangedRows` — all rows already on correct tier → 0 UPDATEs.
- `TestGetBusinessCustomers_ListDoesNotPreloadPreferences` — list issues 0 customer_preferences reads; detail issues exactly 1. RED before: list read `customer_preferences`.

### Proof (captured statements)
```
GetSegments SELECTs over customer_businesses: 1   (single SUM(CASE WHEN ...) aggregate)
PreviewLoyalty: SELECT `total_spent` FROM `customer_businesses` WHERE business_id = ? AND is_active = ?
PutLoyalty (7 customers, 3 tiers): UPDATE count = 3
  UPDATE `customer_businesses` SET `loyalty_tier`="Silver",... WHERE id IN (3,4)
  UPDATE `customer_businesses` SET `loyalty_tier`="Gold",...   WHERE id IN (5,6)
  UPDATE `customer_businesses` SET `loyalty_tier`="Bronze",... WHERE id IN (1,2,7)   # id 7 = $0 reset bucket
```

### Benchmark deltas (SQLite microbench, 200 customers, `-benchmem -count=3`)

| Benchmark | | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `BenchmarkGetSegments` (UB-01) | before | 306,606–364,132 | 113,531–113,552 | 2,433 |
| | after | 212,783–219,105 | 55,808–55,818 | 722 |
| `BenchmarkPutLoyaltyRecompute` (UB-03/N1-02) | before | 1,700,992–1,879,361 | 2,507,614–2,508,051 | 12,270–12,271 |
| | after | 420,370–423,747 | 229,670–229,713 | 3,939 |

GetSegments: latency ~−35%, B/op −51%, allocs −70%. PutLoyalty: latency ~−76%, B/op −91%, allocs −68%; the 200-row per-row UPDATE loop is now ≤3 batched UPDATEs.

### Commands run
```bash
cd backend
# TDD red (new file)
go test ./internal/crm -run 'TestGetSegments|TestPreviewLoyalty_ProjectsTotalSpentOnly|TestPutLoyalty_BatchedTierUpdatesAndCorrectness|TestPutLoyalty_ResetsToNoTierWhenNoLowestQualifies|TestPutLoyalty_SkipsUnchangedRows|TestGetBusinessCustomers_ListDoesNotPreloadPreferences' -count=1
# Baselines (before code change)
go test ./internal/crm -run '^$' -bench 'BenchmarkGetSegments|BenchmarkPutLoyaltyRecompute' -benchmem -count=3
# Implement four fixes, then:
go test ./internal/crm -run '^$' -bench 'BenchmarkGetSegments|BenchmarkPutLoyaltyRecompute' -benchmem -count=3
go test ./internal/crm ./internal/database -run 'CRM|Customer|Loyalty|Segment' -count=1
go test ./internal/crm -count=1
go test ./internal/handlers ./internal/server ./internal/services -run '^$' -count=1
go build ./... && go vet ./internal/crm
```

All green.

---

## L-ORDER-PRELOAD — `GetOrderByID` Business currency projection (PRELOAD-03)

**Files changed**
- `internal/database/business.go` — `GetOrderByID`: `Preload("Business")` → `Preload("Business", preloadOrderListBusinessCurrency)`. One-liner. Bill preload kept (see decision below).
- `internal/database/order_by_id_perf_test.go` — new access-shape test + before/after benchmarks.

### Bill keep/drop decision — KEEP
`order_cancel_handlers.go:174` reads `order.Bill.TableID` after a `GetOrderByID` call (guest cancel path — verifies the order belongs to the table before cancelling). Bill is a load-bearing preload. Projecting Bill via `preloadOrderListBillSummary` is viable (the helper includes `table_id`) but is outside PRELOAD-03 scope; it would be a separate task.

### Caller evidence — what callers read off `order.Business`
All `GetOrderByID` callers checked:
- `order_cancel_handlers.go:59,165` — reads `order.BusinessID` (scalar) only; no `order.Business.*` field access.
- `orders.go:483` — reads `order.BusinessID` only; response serializes via `Order.MarshalJSON`.
- `orders.go:565,594,615` — reads `order.BusinessID` only; serializes via `Order.MarshalJSON` + SSE.
- `delivery_payment.go:120` — just publishes the order to SSE via `eventsPublishOrderUpdated(order)`.
- `Order.MarshalJSON` (`models_json.go:236`) — calls `resolveBusinessCurrency(o.Currency, o.Business)` which reads `o.Business.DisplayCurrency` and `o.Business.DefaultCurrency` only.

`preloadOrderListBusinessCurrency` projects `id`, `default_currency`, `display_currency` — all three fields that `resolveBusinessCurrency` needs. Projection is complete; no caller reads any other Business field from an order returned by `GetOrderByID`.

Note: `enqueueTelegramOrderCreatedNotification` (which reads `order.Business.DisplayCurrency`/`DefaultCurrency`) is called only on the order-CREATE path where Business is manually assigned in memory (`order.Business = *business` at orders.go:347) — NOT from any `GetOrderByID` return path.

### Tests
- `TestGetOrderByIDUsesNarrowBusinessProjection` — seeds a business with `StripeCustomerID="cus_PRELOAD03LEAKTEST"`, calls `GetOrderByID`, asserts:
  - `order.Business.ID` non-zero (projection hydrated)
  - `order.Business.DefaultCurrency == "MXN"` / `DisplayCurrency == "MXN"` (currency fields populated)
  - `order.Business.StripeCustomerID` empty (sensitive column NOT hydrated)
  - `selectStarCount("businesses") == 0` (no `SELECT *` against businesses)
- RED before fix (StripeCustomerID leaked, SELECT * count = 1). GREEN after.

### Benchmark deltas (SQLite microbench, single fully-related order, `-benchmem -count=3`)

| Benchmark | | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `BenchmarkGetOrderByID_Before` | before | 113,152–117,315 | 64,812–64,832 | 883 |
| `BenchmarkGetOrderByID_After` | after | 43,659–43,762 | 42,885–42,886 | 420 |

Latency −62%, B/op −34%, allocs −52%. The narrow Business SELECT transfers fewer bytes and allocates fewer struct fields.

### Commands run
```bash
cd backend
# RED (new test file only, no fix yet)
go test ./internal/database -run 'TestGetOrderByIDUsesNarrowBusinessProjection' -count=1 -v
# Baseline
go test ./internal/database -run '^$' -bench 'BenchmarkGetOrderByID_Before' -benchmem -count=3
# Implement (business.go one-liner), then:
go test ./internal/database -run 'TestGetOrderByIDUsesNarrowBusinessProjection' -count=1 -v    # GREEN
go test ./internal/database -run '^$' -bench 'BenchmarkGetOrderByID_After' -benchmem -count=3
go test ./internal/database ./internal/handlers ./internal/server ./internal/services -run 'Order' -count=1
go test ./internal/database ./internal/handlers ./internal/server ./internal/services -run '^$' -count=1
go build ./...
```

All green.

## L-RESV-PRELOAD — project Business on reservation reads (PRELOAD-04)

### Finding
`createReservationBaseQuery` (`internal/database/reservations.go:279`) preloaded the full
~94-column `Business` row via `Preload("Business")` on all three reservation readers —
`GetReservationByID`, `GetReservationByBusinessAndID`, and the GUEST/public
`GetReservationByConfirmationCode`. Stripe IDs, the onboarding JSON blob, marketing
copy (description/welcome/about) etc. were hydrated despite no consumer reading them.

### Consumer audit (every `reservation.Business.X` read across the three readers)
Direct field reads off the embedded Business (grep across `internal/`, non-test):
- `reservation_handlers.go:928` — `reservation.Business.Name`  (cancel-by-code response `business_name`)
- `reservation_handlers.go:929` — `reservation.Business.CustomURL` (cancel-by-code response `business_custom_url`)
- `reservation_service.go:846`  — `reservation.Business.Timezone` (`reservationNotificationPayload`, Telegram status-change deep link)

Indirect / serialization:
- Operator detail `GetReservation` does `c.JSON(reservation)`; the FE `Reservation` interface
  (`frontend/src/api/reservations.ts:66`) declares **no** nested `business` object, so the embedded
  Business is unused dead weight on the operator wire.
- `reservationEventPayload` (`reservation_handlers.go:74`) and `publicReservationPayload`
  (`:38`) both `delete(m, "business")` before emitting — Business never fans out over SSE
  or the public response either way.
- The public details DTO (`buildPublicReservationDetails`) reads `ctx.business.*`
  (a separately loaded business from `loadAvailabilityContext`), NOT `reservation.Business`.
- Email hooks (`sendReservationCancellationEmail`/`...Updated`/`...NoShow`/approval-outcome)
  all take a separately loaded `*database.Business` via `GetBusinessByID`, not `reservation.Business`.

**Projected Business column set:** `id, name, custom_url, default_currency, display_currency, timezone`.
`id` for FK integrity; `name`/`custom_url` for the cancel-by-code response; `timezone` for the
Telegram notification; `default_currency`/`display_currency` as the money-rendering floor (no
current reader formats money off `reservation.Business`, but kept per the projection floor — cheap and safe).

### StatusHistory decision
- **Operator readers** (`GetReservationByID`, `GetReservationByBusinessAndID`): KEEP StatusHistory +
  StatusHistory.Table — the FE `Reservation.status_history` (`reservations.ts:98`) renders the timeline.
- **Guest reader** (`GetReservationByConfirmationCode`): DROP StatusHistory (new `createGuestReservationQuery`).
  Provably unread: `buildPublicReservationDetails` and `CancelReservationByCode`/`transitionReservation`
  never touch `reservation.StatusHistory` (no `.StatusHistory` read in services/handlers), and both the
  public details DTO and `publicReservationPayload` (which strips `status_history`) never surface it.

### Save-corruption check (cancel-by-code path)
`CancelReservationByCode` → `transitionReservation` → `UpdateReservation` → `db.Save(reservation)` Saves a
reservation whose Business is now projected. Verified GORM emits
`INSERT INTO businesses (...) ... ON CONFLICT DO NOTHING` for the association upsert — on the existing
Business PK it is a **no-op**, so the projected (partial) Business never overwrites real columns. This is
identical pre/post-fix and identical on Postgres (GORM's default association upsert is DO NOTHING).
Regression test: `TestUpdateReservationAfterProjectedReadKeepsBusinessIntact` asserts Stripe IDs /
onboarding blob / description survive the Save and the status mutation lands.

### Tests (access-shape, `internal/database/reservation_read_projection_test.go`)
- `TestGetReservationByConfirmationCodeProjectsBusiness` — projected Business (name/custom_url/currency/timezone
  populated, Stripe IDs + onboarding_state + description NOT hydrated, no `SELECT *` on businesses) AND no
  `reservation_status_histories` query (lean guest read).
- `TestGetReservationByIDProjectsBusinessAndKeepsStatusHistory` — same projection, but StatusHistory IS loaded.
- `TestGetReservationByBusinessAndIDProjectsBusiness` — same projection + StatusHistory loaded (scoped read).
- `TestUpdateReservationAfterProjectedReadKeepsBusinessIntact` — Save safety (above).
- RED first: pre-fix the full Business leaked Stripe IDs/onboarding/description and `SELECT *` on businesses.

### Benchmark deltas (SQLite microbench, fully-related reservation, `-benchmem -count=3`)

| Benchmark | | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `BenchmarkGetReservationByConfirmationCode` | before | 122,958–124,755 | 96,628–96,671 | 1054 |
| `BenchmarkGetReservationByConfirmationCode` | after  | 42,271–42,935  | 37,418–37,421 | 455 |
| `BenchmarkGetReservationByID` | before | 123,776–125,003 | 96,955–97,018 | 1055 |
| `BenchmarkGetReservationByID` | after  | 58,737–61,782  | 72,664–72,688 | 605 |

Guest reader: ns −66%, B/op −61%, allocs −57% (projected Business + dropped StatusHistory).
Operator reader: ns −51%, B/op −25%, allocs −43% (projected Business, timeline retained).

### Commands run
```bash
cd backend
go test ./internal/database -run 'TestGetReservationBy.*Projects' -count=1        # RED before, GREEN after
go test ./internal/database -run '^$' -bench 'BenchmarkGetReservationBy(ConfirmationCode|ID)$' -benchmem -count=3
go test ./internal/database ./internal/server ./internal/handlers ./internal/services -run 'Reservation' -count=1
go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
go build ./...
```
All green.

---

## L-PUBLIC-BIZ — project Business on storefront/QR loaders (OF-04)

Both unauthenticated guest loaders fetched the full ~94-column Business row:
- `GetBusinessByCustomURL` (`internal/database/business.go`) — public storefront page; `db.Where(...).First(&business)` → `SELECT *`.
- `GetActiveTableWithBusinessByCode` (`internal/database/business.go`) — QR/table-code loader returning `(*Table, *Business, error)`; `Joins("Business")` hydrated all Business columns.

Both leaked the private Stripe IDs / payment method and the `onboarding_state` blob onto guest paths and over-fetched. Fixed by a single shared projection slice `publicBusinessColumns` applied as `db.Select(publicBusinessColumns)` (custom-URL path) and `db.Joins("Business", db.Select(publicBusinessColumns))` (QR join). The `(*Table, *Business, error)` return shape and the table's own columns are unchanged.

### Derived Select column set + consumer evidence
The set is the UNION of everything every public consumer of BOTH loaders touches. Derived from reading the consumers, not guessed:

- **`publicBusinessProjection`** (`internal/server/public_business_helpers.go`) — the storefront response source of truth: id, name, logo, address (embedded street/city/state/postal_code/country), description, custom_url, phone, website, social_media, banner_images, show_reviews, google_reviews_enabled, google_place_id, google_business_name, google_review_link, google_business_url, default_currency, display_currency, default_language, design_settings (all `design_*`), welcome_message, about_story, show_welcome_message, show_about_story, show_gallery, show_operating_hours, show_special_features, ai_settings (`ai_ai_enabled`/`ai_ai_name`/`ai_ai_priority`/`ai_business_page_ai_enabled`), tax_rate, service_fee_rate, created_at, updated_at.
- **`buildPublicGuestBusinessResponse`** (`internal/server/public_guest_response_helpers.go`) — the QR/table guest response: adds business_id, timezone, kitchen_enabled, orders_enabled, crm_enabled, counter_enabled, source_language; reuses the public/ai fields above.
- **`IsAIWaiterAvailable` → `HasActiveSubscription` → `GetBusinessLockState`** (computed on the QR business, surfaced as `ai_available`, and used to reject orders on lapsed subs): subscription_plan, subscription_status, grace_period_ends_at, **subscription_end_date, cancel_at_period_end**, `ai_ai_enabled`. (subscription_end_date + cancel_at_period_end were ADDED in the OF-04 fix — see "Fail-open/fail-close gate fix" below.)
- **`guestOrderingEnabled`**: kitchen_enabled, orders_enabled.
- **`applyBusinessContentTranslations`** (custom-URL handler, mutates in place): description, welcome_message, about_story (already covered).
- **Gating**: `loadPublicBusinessByCustomURL` reads is_active + business_page_enabled; `GetActiveTableWithBusinessByCode` reads is_active.

Embedded-column gotcha (caught by the access-shape test, NOT guessed): GORM double-prefixes `AiSettings` — embeddedPrefix `ai_` + `Ai*` field names → columns are `ai_ai_enabled` / `ai_ai_name` / `ai_ai_priority`; only `BusinessPageAiEnabled` lacks the leading `Ai` → `ai_business_page_ai_enabled`. `DesignSettings` uses `design_*`; `BusinessAddress` has explicit unprefixed column tags. The standalone `First` errored (`no such column: ai_enabled`) and the join silently dropped them until the names were corrected — the test forced this.

### DROPPED private columns (proof: no consumer reads them)
- `stripe_customer_id`, `stripe_subscription_id`, `stripe_price_id`, `stripe_payment_method` — only read on billing/admin/owner paths (`GetBusinessTierInfo`, Stripe handlers); never in `publicBusinessProjection`, `buildPublicGuestBusinessResponse`, or any guest gate. `staffBusinessProjection` exposes only card brand/last4 (display), never IDs.
- `onboarding_state` — opaque wizard blob, only the authenticated dashboard reads it.
- `ai_special_instructions` — operator/AI-waiter-internal only (`business_handlers.go`, `ai_waiter_handler.go`, `whatsapp_manager.go`); never on a guest path.
- Plus all other unselected columns (owner_address/owner_name/email, settlement_addr/tipping_addr, billing bookkeeping `trial_ends_at`/`lockout_applied_at`/`yearly_fee`/`total_paid`/tx hashes, `closed_at`/`closed_reason`, `is_demo`, lat/long, counter_count/prefix, QR defaults, `stripe_card_*`) — none read by the two public loaders' consumers. NOTE: `subscription_end_date` and `cancel_at_period_end` are NOT in this dropped set — they ARE read by `GetBusinessLockState` and are projected (corrected below).

### Fail-open/fail-close gate fix (OF-04 follow-up — column omission)
The original `publicBusinessColumns` OMITTED two columns that `GetBusinessLockState` reads:
- `subscription_end_date` (`SubscriptionEndDate`)
- `cancel_at_period_end` (`CancelAtPeriodEnd`)

Both projected loaders reach `GetBusinessLockState` via `HasActiveSubscription`/`IsAIWaiterAvailable` (guest `ai_available` AND guest order acceptance at `guest_handlers.go:210,532`). With those columns zeroed by the projection:
- A lapsed-not-swept `subscription_status="active"` business (end date in the past) was mis-classified ACTIVE → **fail-open billing bypass** (guest ordering + AI wrongly enabled).
- A `cancel_at_period_end=true` business paid through a future period was mis-classified CANCELLED → AI/ordering wrongly DISABLED for a paying customer.

Fix: added both columns to `publicBusinessColumns` (next to subscription_plan/status/grace_period_ends_at) with a drift comment; added a drift-guard comment on `GetBusinessLockState`; corrected the stale `GetActiveTableWithBusinessByCode` comment. The full `GetBusinessLockState` column read-set was re-mapped line by line — the ONLY two omissions were these; all other reads (subscription_status, grace_period_ends_at) were already present. No additional consumer (publicBusinessProjection, buildPublicGuestBusinessResponse, guestOrderingEnabled, applyBusinessContentTranslations, IsAIPlanEnabled) had any further missing column.

The prior column-mention test guard was a TAUTOLOGY: `publicConsumedColumns` (test) was a hand-copy of `publicBusinessColumns` (prod) with the SAME blind spot, and the seeded business used `subscription_status="active"` with no end date, never exercising the date branches. `publicConsumedColumns` now includes the two columns too.

### Tests (access-shape, `internal/database/business_public_projection_test.go`)
- `TestGetBusinessByCustomURLProjectsPublicColumns` — every column in `publicConsumedColumns` present in the projected SELECT; Stripe IDs + payment method + onboarding_state NOT selected and NOT hydrated; representative public fields (name/welcome_message/about_story/currency/address.city/design.primary_color/ai.name/subscription_plan/kitchen+orders) populated; no `SELECT *` on businesses. RED before fix (SELECT * hydrated Stripe/onboarding).
- `TestGetActiveTableWithBusinessByCodeProjectsPublicColumns` — same projection assertions on the QR join AND the returned `*Table`'s own fields (name/table_code/capacity) intact. RED before fix.
- Both seed a fat business with non-empty Stripe IDs + onboarding blob + full public content.
- **`TestProjectedLoadersComputeIdenticalLockState`** (LOAD-BEARING, not a tautology) — proves the billing gate is correct THROUGH the projected loaders by comparing the lock state / `HasActiveSubscription` / `IsAIWaiterAvailable` computed off a FULL-ROW load (`GetBusinessByID`, `SELECT *`) vs both projected loaders (`GetBusinessByCustomURL`, `GetActiveTableWithBusinessByCode`). Because it asserts projected == full-row, it cannot share a blind spot with the column list. Scenarios: (1) `active` + past end date (lapsed-not-swept) → both must be `suspended`/inactive; (2) `cancelled` + `cancel_at_period_end` + future end date → both must be `active`; (3) `cancelled` + past end date → `cancelled`/inactive; (4) `active` + future end date → `active`. RED-without-fix evidence: removing the two columns makes scenarios (1) and (2) FAIL — projected computes `"active"` while full row computes `"suspended"` (fail-open), and projected `HasActiveSubscription=false` while full row `=true` (fail-close). GREEN with the columns added.
- Existing consumer-side guards still green: `TestPublicBusinessProjectionOmitsSensitiveFields`, `TestPublicBusinessProjectionStorefrontKeys`, `TestPublicBusinessProjectionKeysAreBlessed` (`internal/server/public_business_projection_test.go`).

### Benchmark deltas (SQLite microbench, fully-populated business, `-benchmem -count=3`)

| Benchmark | | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `BenchmarkGetBusinessByCustomURL` | before | 71,687–75,333 | 28,036–28,039 | 584 |
| `BenchmarkGetBusinessByCustomURL` | after  | 54,997–55,678 | 33,985–33,987 | 465 |
| `BenchmarkGetActiveTableWithBusinessByCode` | before | 170,716–172,068 | 137,261–137,275 | 1289 |
| `BenchmarkGetActiveTableWithBusinessByCode` | after  | 111,319–112,753 | 93,299–93,328 | 861 |

Custom-URL loader: ns −24%, allocs −20% (584→465). B/op rose slightly (named-column prepared statement vs `SELECT *`) — the load-bearing over-fetch metric (allocs) dropped and the private-column hydration is gone.
QR/table loader: ns −35%, B/op −32%, allocs −33% (1289→861).

### Commands run
```bash
cd backend
go test ./internal/database -run 'TestGetBusinessByCustomURLProjectsPublicColumns|TestGetActiveTableWithBusinessByCodeProjectsPublicColumns' -count=1   # RED before, GREEN after
go test ./internal/database -run 'TestProjectedLoadersComputeIdenticalLockState' -count=1   # RED without the two columns (fail-open/fail-close), GREEN with
go test ./internal/database -run '^$' -bench 'BenchmarkGetBusinessByCustomURL|BenchmarkGetActiveTableWithBusinessByCode' -benchmem -count=3
go test ./internal/database ./internal/server ./internal/handlers -run 'Business|Public|Storefront|Table|Guest|CustomURL|Lock|Subscription' -count=1
go test ./internal/database ./internal/server ./internal/handlers ./internal/services -run '^$' -count=1
go build ./...
go vet ./internal/database
```
All green.

---

## L-DIRECTOR-BILLS — count active bills instead of hydrating full rows (OF-05)

`GetActiveBillsByBusinessID` on the `*DB` receiver issued `SELECT *` against the
`bills` table and hydrated full Bill rows. Its sole caller in the Director path
(`internal/services/director_console_service.go:483`) immediately discarded the
rows and used only `len(activeBills)` to populate `Metrics.ActiveBills`. The other
two callers (`analytics.go:330` and `analytics.go:505`) already used the lean
`GetActiveBillSummariesByBusinessID` — confirmed via grep, so the full-row function
had exactly one caller and that caller needed only the count.

The misleading doc comment on `GetActiveBillsByBusinessID` claimed the function
returned "lean bill rows" — corrected to accurately describe that it returns full
Bill rows and to point to the two lean alternatives.

### Fix
- Added `GetActiveBillCountByBusinessID(businessID uint) (int64, error)` on `*DB`
  (`internal/database/db_config.go`) — issues `Model(&Bill{}).Where(...active status predicate...).Count(&n)`.
  Uses the identical `activeBillStatusStrings()` predicate as the full-row loader.
- Repointed `director_console_service.go:483` from `GetActiveBillsByBusinessID`
  (full-row slice) to `GetActiveBillCountByBusinessID` (int64). Changed
  `payload.Metrics.ActiveBills = len(activeBills)` → `= int(activeBillCount)`.
- Corrected the doc comment on `GetActiveBillsByBusinessID` (no behaviour change;
  the function is kept unchanged for any future row-consuming callers).

### Callers of GetActiveBillsByBusinessID (full audit)
| Caller | Uses rows? | Action |
| --- | --- | --- |
| `internal/services/director_console_service.go:483` | No — only `len(...)` | Repointed to count |
| (no other callers) | — | — |

`GetActiveBillSummariesByBusinessID` (the lean variant) is called by
`internal/handlers/analytics.go:330` and `analytics.go:505` — both already use
the correct lean path and are untouched.

### Benchmark deltas (SQLite microbench, 20 active bills, `-benchmem -count=3`)

| Benchmark | | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `BenchmarkGetActiveBillsByBusinessID_Before` | before | 122,323–130,666 | 171,722–171,728 | 711 |
| `BenchmarkGetActiveBillCountByBusinessID_After` | after | 6,586–6,660 | 7,568 | 52 |

ns/op: −95% (full-row scan → aggregate COUNT). B/op: −96%. allocs/op: −93% (711→52).

### Commands run
```bash
cd backend
go test ./internal/database -run 'TestGetActiveBillCountByBusinessID' -count=1   # RED before, GREEN after
go test ./internal/database -run '^$' -bench 'BenchmarkGetActiveBillsByBusinessID_Before|BenchmarkGetActiveBillCountByBusinessID_After' -benchmem -count=3
go test ./internal/database ./internal/services -run 'Director|Bill|ActiveBill' -count=1
go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
go build ./...
```
All green.

---

## L-DASH-ITEMS — Bound GetPopularItems for dashboard summary (DUP-02, 2026-06-17)

### Finding
`GetPopularItems` (previously `internal/analytics/service.go:287`) fetched and resolved
category/translated names for EVERY item in the menu regardless of how many the caller
needed. The two dashboard callers (`handlers/analytics.go:511` dashboard summary,
`director_console_service.go:478`) immediately sliced the result to 5 in Go after
fetching the whole result set. The report scheduler used only the top 3 (break-after-3
loop) but fetched everything. The SQL query already had `ORDER BY revenue DESC` — the
Go-side slice was redundant work.

### Fix
- Added `limit int` as the second parameter to `GetPopularItems(businessID uint, limit int, period string)`.
  When `limit > 0`, a `LIMIT ?` clause is appended to the final SELECT before the CTE executes,
  so only the top-N rows are fetched and passed to the category/name resolution loop.
  `limit <= 0` means unbounded (identical to the previous behaviour).
- Removed the post-fetch Go slice in `handlers/analytics.go:511` and `director_console_service.go:478`.
- Removed the `sort.SliceStable` in `tool_menu_top_items.go:Run` — the service now guarantees
  `ORDER BY revenue DESC` in the SQL result, so the tool's Go re-sort was redundant.
- Updated the `PopularItemsProvider` interface in `director_tools/tool_menu_top_items.go` to match.
- Updated the stub in `director_tools/helpers_test.go` to honour the limit parameter.
- Updated the `TestMenuTopItemsTool_Run` fixture to be pre-sorted by revenue DESC (reflecting
  the real SQL ordering guarantee).

### Callers and limit passed

| Caller | Limit passed | Reason |
| --- | --- | --- |
| `handlers/analytics.go` GetItemAnalytics | 0 (unbounded) | Full analytics page renders all items |
| `handlers/analytics.go` dashboardSummaryForBusiness | 5 | Dashboard renders only top 5 |
| `services/director_console_service.go` buildContext | 5 | Director context needs only top 5 |
| `services/report_scheduler.go` | 3 | Weekly email renders only top 3 |
| `director_tools/tool_menu_top_items.go` | limit from model args (1-50) | Tool passes model-requested count to SQL |
| `director_tools/tool_menu_underperformers.go` | 0 (unbounded) | Needs all items to compute median |

### Benchmark deltas (SQLite microbench, 50-item menu, `-benchmem -count=3`)

| Benchmark | Path | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| BenchmarkGetPopularItems_Before | limit=0, all 50 items | 620-678 us | 192-193 KB | 1121 |
| BenchmarkGetPopularItems_After  | limit=5, top 5 only  | 957-1036 us | 174 KB | 711-712 |

Allocs: -37% (1121->712). Memory: -10% (193KB->174KB). SQLite's in-memory CTE execution
means the latency number is dominated by query-planner overhead and does not reflect
production PostgreSQL behaviour — in Postgres the LIMIT is applied inside the CTE
before row transfer, yielding proportional latency gains (5 rows transferred vs. N).
The alloc/memory reduction is the deterministic, database-independent signal confirming
bounded work.

### Commands run
```bash
cd backend
go test ./internal/analytics -run TestGetPopularItemsLimitBoundsAtSQL -count=1
go test ./internal/analytics -run 'TestGetPopularItems' -count=1
go test ./internal/analytics ./internal/services/director_tools -run '^$' \
  -bench 'BenchmarkGetPopularItems' -benchmem -count=3
go test ./internal/database ./internal/handlers ./internal/server \
  -run 'Popular|Analytics|Dashboard|Item' -count=1
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
go build ./...
```
All green.

## L-DASH-SUMMARIES: batch + project business load in dashboard summaries (N1-01)

### Problem

`AnalyticsHandler.GetDashboardSummaries` (`internal/handlers/analytics.go`) looped over
`req.BusinessIDs` and ran a per-id full-row `h.db.GetGorm().First(&business, businessID)` —
an N+1 of full ~94-column `Business` rows, even though the loop body only consumes ownership,
the billing gate, and the timezone/id. For a multi-business operator dashboard refresh
(M businesses), that is M full-row SELECTs per request.

### Fix

Collect the deduped non-zero ids first (preserving the per-id `==0 → 400` check in request
order + the `seen` dedup), then ONE batched, projected read:
`Select(dashboardSummariesBusinessColumns).Where("id IN ?", ids).Find(&businesses)`, build an
`id → *Business` map, and loop over the deduped ids in order. Net business-load queries: N → 1.

Short-circuit semantics preserved byte-for-byte:
- `businessID == 0` → **400** `Invalid business id` (per id, in request order, before the batch load)
- absent from map → **404** `Business not found` (same as the old `First` error)
- `!CheckBusinessAccess` → **403** `Access denied`
- `!HasActiveSubscription` → **402** `subscription_inactive` / `This feature requires an active subscription`
- success → `dashboardSummaryForBusiness` keyed by `business.ID`

### Derived projection (UNION over consumers — completeness is the bug risk)

`dashboardSummariesBusinessColumns` = derived from every `business.X` field the per-id path reads:

| Column | Consumer that reads it (evidence) |
| --- | --- |
| `id` | PK; map key; `CheckBusinessAccess` staff path (`bizID == business.ID`); analytics calls `business.ID` |
| `owner_address` | `CheckBusinessAccess` → `CheckBusinessOwnership` (`business.OwnerAddress`) |
| `user_id` | `CheckBusinessOwnership` (`*business.UserID == parsedUID`) |
| `subscription_status` | `HasActiveSubscription` → `GetBusinessLockState` (`business.SubscriptionStatus`) |
| `grace_period_ends_at` | `GetBusinessLockState` (`business.GracePeriodEndsAt`) |
| `subscription_end_date` | `GetBusinessLockState` — lapsed-not-swept + cancel-through-period branches (**load-bearing**) |
| `cancel_at_period_end` | `GetBusinessLockState` — paid-through cancellation branch (**load-bearing**) |
| `timezone` | `dashboardSummaryForBusiness`/`buildTodayByStaff` → `resolveBusinessLocation` (`business.Timezone`) |
| `subscription_plan` | over-projected defensively (not read by `HasActiveSubscription`, but cheap + future-proof) |
| `is_active` | over-projected defensively |

Re-verified against source: `GetBusinessLockState` (`internal/database/business.go:3512`) reads exactly
`SubscriptionStatus`, `GracePeriodEndsAt`, `CancelAtPeriodEnd`, `SubscriptionEndDate` — all four present.
`CheckBusinessOwnership` (`internal/server/ownership.go:58`) reads `OwnerAddress`, `UserID`, `ID` — all present.
`resolveBusinessLocation` reads `Timezone` + `ID` — present. `dashboardSummaryForBusiness` reads only
`business.ID` + `business.Timezone` (grep of `business.` in analytics.go confirmed no other field).
Omitting `subscription_end_date`/`cancel_at_period_end` is the exact fail-open/fail-close class of bug a
sibling task shipped (OF-04); the parity test below guards it.

### Tests (RED → GREEN)

- `TestDashboardSummariesBatchesBusinessLoad` — seeds M=6 businesses, asserts the business load is
  exactly **1** `SELECT ... FROM businesses` AND **1** `WHERE id IN (...)` (was RED: 6 selects, 0 IN).
- `TestDashboardSummariesLockStateParity` — the **bug guard**: seeds a lapsed-not-swept
  (`status=active`, `subscription_end_date` past, no grace) business and a paid-through
  (`status=cancelled`, `cancel_at_period_end=true`, future end) business. Asserts a full-row
  oracle (`HasActiveSubscription` on the full row) matches the handler's HTTP gate: lapsed → **402
  subscription_inactive**, paid-through → **200**. Proves the projected columns reproduce the gate exactly.
- `TestDashboardSummariesShortCircuits` — preserves 400 (zero id) / 404 (missing) / 403 (not owned)
  + dedup (3× same id → single data entry).

### Benchmark deltas (`BenchmarkDashboardSummariesBusinessLoadSQLite`, M=20, `-benchmem -count=3`)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before (N First calls) | 21.77–21.90 ms | 10,285–10,290 KB | 43,699–43,708 |
| After (1 IN query)     | 20.22–20.67 ms | 9,779–9,784 KB   | 33,446–33,448 |

Business-load queries N→1; allocs −10,260 (~23%), bytes −0.51 MB (~5%), latency ~−6%. The benchmark
exercises the full handler (the per-business analytics sub-queries dominate raw latency), so the N→1
business-load win surfaces most cleanly in the deterministic alloc/byte deltas.

### Commands run
```bash
cd backend
go test ./internal/handlers -run 'TestDashboardSummaries' -count=1
go test ./internal/handlers -run '^$' -bench 'BenchmarkDashboardSummariesBusinessLoadSQLite' -benchmem -count=3
go test ./internal/handlers ./internal/server -run 'Dashboard|Summary|Analytics|Business' -count=1
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
go build ./...
```
All green.

---

## L-PAYROLL — batch staff validation in CreatePayrollRun (N1-04)

`internal/accounting/service.go` — `CreatePayrollRun` looped over line items and,
for each staff payee, issued a per-item `Where("id = ? AND business_id = ?").First(&staff)`
just to read `staff.Name` into `PayeeName` (N+1 over staff line items). Fix: a pre-pass
collects unique staff IDs, one `Select("id","name").Where("id IN ? AND business_id = ?").Find`
builds an id→name map, and the main loop validates/assigns from the map. Per-item error
strings (`line_items[%d].staff_id is required/invalid`) and first-failing-index order are
preserved (cross-business IDs fall out of the IN filter → "invalid").

### Tests (`internal/accounting/service_test.go`)
- `TestCreatePayrollRun_StaffValidationIssuesOneQuery` — load-bearing access-shape gate:
  counts SELECTs hitting the `staff` table via a `gorm:query` Before callback; K=5 staff
  items must produce **exactly 1** staff query. Proven RED without the fix (failed `5 != 1`).
- `TestCreatePayrollRun_InvalidStaffIDAtIndex` — unknown id → `line_items[1].staff_id is invalid`;
  cross-business id → `line_items[0].staff_id is invalid`; earlier `gross<=0` still fires before
  a later invalid staff id (order preserved).

### Benchmark deltas (`BenchmarkCreatePayrollRun`, K=10 staff items, `-benchmem -count=3`)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before (N+1 per-item First) | 262.6–282.7 µs | 204,692–204,763 | 1,834 |
| After (1 batched IN query)  | 144.0–146.9 µs | 184,511–184,521 | 709 |

Staff queries K→1; latency ~−47%, allocs −1,125 (~61%), bytes −20 KB (~10%).

### Commands run
```bash
cd backend
go test ./internal/accounting/ -run 'TestCreatePayrollRun' -count=1
go test ./internal/accounting/ -run '^$' -bench 'BenchmarkCreatePayrollRun$' -benchmem -count=3
go test ./internal/accounting/ -count=1
```
All green.

---

## L-ORDER-ALERT — batch order-alert resolution (N1-03)

`internal/database/order_auto_cancel.go` — `resolveOrderAlertsTx` looped over M
loaded `OperationalAlert` rows and, per alert, issued TWO statements:
`Where("id = ?").Updates(...)` + `Create(&OperationalAlertEvent{})`. With M=4
alerts that is 4 UPDATEs + 4 INSERTs (N+1 on both paths).

Fix (mirrors the sibling `cancelPendingOrdersForClosedBillTx`):
1. Collect alert IDs into a slice, issue ONE `Where("id IN ?").Updates(map...)` to
   batch-resolve all alerts (status=resolved, resolved_at=&now, last_event_at, updated_at).
2. Build a `[]OperationalAlertEvent` slice (one per alert) and issue ONE
   `tx.Create(&events)` (GORM batch insert). M UPDATEs + M INSERTs → 1 + 1.

### Test — `TestResolveOrderAlerts_BatchesUpdatesAndInserts` (access-shape regression, N1-03)

Seeds M=4 pending orders each with an open `OperationalAlert` on the same bill.
Registers `gorm:update` + `gorm:create` Before-callbacks (sync/atomic counters) to
count statements hitting `operational_alerts` and `operational_alert_events`.
Triggers via `CloseBill(bill.ID)` (the public entry point).

- **RED** (before fix): 4 UPDATEs to `operational_alerts`, 4 INSERTs to `operational_alert_events`.
- **GREEN** (after fix): exactly **1** UPDATE + **1** INSERT.

Correctness assertions: all 4 alerts have `Status=Resolved` and `ResolvedAt` non-nil;
a COUNT query confirms 4 `OperationalAlertEvent` rows exist (batch insert wrote all rows).

### Benchmark deltas (`BenchmarkResolveOrderAlerts`, M=4, `-benchmem -count=3`, Apple M3)

| State | ns/op (range) | B/op (median) | allocs/op (median) |
| --- | --- | --- | --- |
| Before (N+1: M UPDATEs + M INSERTs) | 435,384–508,711 | 425,508 | 1,810 |
| After (1 batched UPDATE + 1 batch INSERT) | 341,126–527,486 | 378,496 | 1,411 |

allocs/op: −399 (~22%); B/op: ~−11%; ns/op overlap is run-to-run SQLite variance
on DB setup overhead (each iteration opens a fresh in-memory DB — the measured
CloseBill path is the same). Statement count is the deterministic proof: 4+4 → 1+1.

### Commands run
```bash
cd backend
# RED then GREEN
go test ./internal/database/ -run TestResolveOrderAlerts_BatchesUpdatesAndInserts -v -count=1
# Baseline benchmark (before fix)
go test ./internal/database/ -run '^$' -bench BenchmarkResolveOrderAlerts -benchmem -count=3
# [applied fix to resolveOrderAlertsTx]
# After benchmark
go test ./internal/database/ -run '^$' -bench BenchmarkResolveOrderAlerts -benchmem -count=3
# Regression suite
go test ./internal/database/ -run 'AutoCancel|OrderAlert|CloseBill|VoidBill' -count=1
go test ./internal/database/ -count=1
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
go build ./...
```
All green.

## L-AIWAITER-PUB — publish from caller fields, drop re-query + race (SSE-02)

`publishAiWaiterMessage` re-queried the conversation's newest message
(`Select("id","role","content","created_at")...Order("created_at DESC, id DESC").First`)
just to fan it to the SSE hub. The two callers had already saved the exact
message microseconds earlier, so the read was wasted AND raced: a concurrent
newer message could be re-fetched and published instead of the saved one.

Fix: `publishAiWaiterMessage(conv, id, role, content, createdAt)` now publishes
directly with no DB read. `SaveAiWaiterStaffReply` and
`SaveAiWaiterMessageReturningID` now also return the created timestamp so the
two callers (staff-reply `PostAiReply`, guest `HandleAIWaiter`) can pass the
saved row's exact fields. `SaveAiWaiterMessage` wrapper updated to drop the
extra return.

### Tests
- **Zero-query access-shape test** `TestPublishAiWaiterMessage_IssuesNoQuery`
  (server): registers a `gorm:query` Before callback counter, subscribes to the
  hub, calls `publishAiWaiterMessage(conv, 42, "assistant", "hi", now)`, asserts
  the received event is id 42 / assistant / "hi" AND the query counter == 0.
  RED (temp probe against the old re-query signature): query count = **1** and
  it published the newer row (id=2), not the caller's. GREEN: query count = **0**.
- **Race-fix regression** `TestPublishAiWaiterMessage_NoRaceReQuery` (server):
  saves message A, then a newer message B for the same conversation, then
  publishes A's held fields and asserts the event carries A's id/content — not
  B's. (Under the old re-query the temp probe published B.) GREEN.
- Existing `TestPostAiReply_PublishesStreamEvent` (staff path end-to-end) and
  `BenchmarkAiWaiterStreamPublish` updated to the new signature; both green.

### Benchmark (BenchmarkAiWaiterStreamPublish, Apple M3, -benchmem -count=3)
| Metric     | Before (re-query) | After (caller fields) |
|------------|-------------------|-----------------------|
| ns/op      | ~7431–7632        | ~7.36–7.52            |
| B/op       | 5437              | 0                     |
| allocs/op  | 86                | 0                     |

Publish is now DB-free and allocation-free (~1000x faster, the SSE fan-out is
the only remaining cost).

### Commands
```
# RED probe (temp file, deleted after capture)
go test ./internal/server/ -run TestRedProbe_PublishReQueriesAndRaces -count=1 -v   # query=1, published newer B
# baseline benchmark (before, re-query)
go test ./internal/server/ -run '^$' -bench 'BenchmarkAiWaiterStreamPublish$' -benchmem -count=3
# after fix
go test ./internal/server/ -run 'TestPublishAiWaiterMessage_IssuesNoQuery|TestPublishAiWaiterMessage_NoRaceReQuery|TestPostAiReply_PublishesStreamEvent' -count=1
go test ./internal/server/ -run '^$' -bench 'BenchmarkAiWaiterStreamPublish$' -benchmem -count=3   # query-free
go test ./internal/server/ -run 'AIWaiter|AiWaiter|AiReply|Stream' -count=1
go test ./internal/database/ -run 'AiWaiter' -count=1
go build ./...
go vet ./internal/server/ ./internal/database/
```
All green. Full grep confirmed no callers of `publishAiWaiterMessage`,
`SaveAiWaiterStaffReply`, or `SaveAiWaiterMessageReturningID` beyond the two
publish sites + the in-file `SaveAiWaiterMessage` wrapper.

## L-RESV-EVENT — single-marshal SSE payload (SSE-03)

`internal/server/reservation_handlers.go` had two payload builders
(`reservationEventPayload`, `publicReservationPayload`) that each did a wasteful
`json.Marshal(r)` → `json.Unmarshal` into `map[string]any` → `delete(keys)` →
return map round-trip. The returned map was then marshaled AGAIN by
`Hub.PublishJSON` / `c.JSON` — three JSON passes where one suffices.

### Fix
Replaced the round-trip with field-shadowing view structs that embed
`*database.TableReservation` and shadow the `business` (and, for guests, `notes`
+ `status_history`) keys with `*struct{}` `omitempty` fields, then changed both
functions to return `any`. The value is now marshaled exactly once downstream.
`TableReservation` has **no custom MarshalJSON** (verified), so the
embedded-pointer + field-shadow projection is exact. The model and its tags were
NOT touched, and the raw `c.JSON(http.StatusOK, reservation)` REST responses
(lines ~340/439/516/600/628/660/688) are intentionally unchanged — they still
serialize the full reservation (including business when preloaded) byte-identical
to before. The one map-indexing call site (pending-reservation
`approval_deadline`) was adapted to an anonymous struct embedding
`reservationPublicView`.

### Shape / security test — RED→GREEN
- `TestNaiveReservationMarshalLeaksBusiness` proves the projection is
  load-bearing: a naive `json.Marshal(&r)` DOES carry a top-level `business` key
  and `cus_LEAKME`.
- `TestReservationEventPayloadShape` / `TestReservationPublicPayloadShape`
  (new, in `reservation_event_payload_test.go`) assert no top-level `business`
  key, no `cus_LEAKME`, while retaining `id`/`status`/`customer_name`/
  `confirmation_code`/`table` (+ `notes`/`status_history` for the event view,
  `party_size` for the public view; public view additionally drops
  `notes`/`status_history`).
- RED→GREEN: the existing `TestPublicReservationPayloadStripsOperatorData` /
  `TestReservationEventPayloadStripsEmbeddedBusiness` indexed the returned
  `map[string]any` directly; after the signature change to `any` they no longer
  COMPILED (RED), and pass once rewritten to marshal-then-inspect via the shared
  `topLevelKeys` helper (GREEN). The `TestReservationHandlersNeverPublishRawReservations`
  access-shape guard (every `PublishJSON` wraps `reservationEventPayload`) still
  holds. Note: the model's `status_history[].reservation.business` back-reference
  is a pre-existing zero-value serialization quirk (never hydrated in production —
  prod preloads `StatusHistory` + `StatusHistory.Table` only, not
  `StatusHistory.Reservation`); the shape tests assert top-level keys so they do
  not false-trip on it.

### Benchmark (before = old round-trip, after = single-marshal shadow view, `-benchmem -count=3`)

| Benchmark                       | Metric    | Before   | After  | Delta        |
|---------------------------------|-----------|----------|--------|--------------|
| BenchmarkReservationEventPayload  | ns/op     | ~105,225 | ~7,389 | ~14x faster  |
|                                 | B/op      | 63,349   | 6,810  | ~9x less     |
|                                 | allocs/op | 865      | 16     | ~54x fewer   |
| BenchmarkReservationPublicPayload | ns/op     | ~70,817  | ~3,985 | ~18x faster  |
|                                 | B/op      | 49,153   | 3,315  | ~15x less    |
|                                 | allocs/op | 563      | 9      | ~63x fewer   |

### Commands
```
# baseline benchmark (before, captured against old round-trip impl)
go test ./internal/server/ -run '^$' -bench 'BenchmarkReservation(Event|Public)Payload' -benchmem -count=3
# after fix
go test ./internal/server/ -run 'TestNaiveReservationMarshalLeaksBusiness|TestReservationEventPayloadShape|TestReservationPublicPayloadShape' -count=1
go test ./internal/server/ -run '^$' -bench 'BenchmarkReservation(Event|Public)Payload' -benchmem -count=3
go test ./internal/server/ -run 'Reservation' -count=1
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
go build ./...
go vet ./internal/server/
```
All green. The 9 SSE `PublishJSON(..., reservationEventPayload(x))` sites needed
no edits (`PublishJSON(uint, string, interface{})` accepts `any` and marshals
once). The raw `c.JSON(reservation)` REST responses are intentionally unchanged.

## L-PAGEVIEW — incremental session-summary upsert (DUP-01)

`internal/analytics/page_analytics.go` `updateSessionSummary` was called async
after EVERY `TrackPageView` and re-read ALL `page_views` rows for the session,
re-aggregated (pages/duration/first-last), ran a separate
`SELECT COUNT(*) FROM user_interactions`, then upserted the
`session_summaries` row — O(K^2) reads for K page views in a session.

Fix: changed the signature to `updateSessionSummary(pv *models.PageView)` and
replaced the body with ONE atomic incremental upsert per page view (no
page_views SELECT, no user_interactions COUNT). On INSERT it seeds
first_seen/last_seen/device_type/country from this page view and a
single-element `pages_visited`; on CONFLICT it bumps last_seen, does
`total_page_views + 1`, `total_duration + excluded.total_duration`, and appends
the page via a DIALECT-AWARE JSON append. The interaction counter moved into a
new `incrementSessionInteraction(*models.UserInteraction)` (its own atomic
upsert, run in `logger.SafeGo` from `TrackInteraction`). `GetRecentSessions`,
`GetAnalyticsSummary` (its own independent `COUNT(*)` over user_interactions),
and the migration schema are unchanged.

Dialect-aware `pages_visited` append (single statement → no read-modify-write race):
- sqlite: `pages_visited = json_insert(COALESCE(NULLIF(pages_visited,''),'[]'), '$[#]', ?)`  (bind: pv.Page)
- postgres: `pages_visited = (COALESCE(NULLIF(pages_visited,''),'[]')::jsonb || to_jsonb(?::text))::text`  (bind: pv.Page)

Access-shape test (TDD, RED→GREEN):
`TestUpdateSessionSummary_NoPageViewRescanOrInteractionCount` in
`internal/analytics/page_analytics_session_test.go` drives 5 page views in one
session and asserts ZERO `FROM page_views` SELECTs and ZERO
`COUNT(*) FROM user_interactions` across the 5 calls, plus correctness
(total_page_views=5, total_duration=150, pages_visited=["a","b","c","d","e"],
last_seen=5th ts, device_type/country from first page view).
- RED (old rescan impl, captured before editing): 1 `FROM page_views` scan AND
  1 `COUNT(*) FROM user_interactions` per call → 5 of each across 5 calls.
- GREEN (incremental impl): 0 and 0.
Supporting tests: `TestTrackInteraction_IncrementsCounter` (counter=3, last_seen
stays page-view-driven), `TestInteractionBeforePageView` (interaction creates
the row with total_interactions=1, total_page_views=0; a later page view
increments page views to 1 WITHOUT clobbering the interaction count),
`TestGetRecentSessions_ShapePreserved` (read-back shape unchanged).

Benchmark `BenchmarkUpdateSessionSummary` (SQLite in-memory, session pre-seeded
with N=20 page views, measuring one more update; `-benchmem -count=3`):
- BEFORE (old rescan impl, `BenchmarkUpdateSessionSummaryOld`):
  143919 ns/op  43277 B/op  1064 allocs/op
  190576 ns/op  79908 B/op  1967 allocs/op
  279466 ns/op 133382 B/op  2870 allocs/op
  (bytes/allocs CLIMB across counts — re-aggregation scales with accumulated rows: O(N).)
- AFTER (incremental impl):
  104838 ns/op   3699 B/op    50 allocs/op
  259012 ns/op   3698 B/op    50 allocs/op
  359296 ns/op   3698 B/op    50 allocs/op
  (bytes/allocs are FLAT at ~3.7k B/op and 50 allocs/op regardless of N — the
  decisive O(1) access-shape win. ns/op varies due to SQLite shared-cache
  in-memory contention across `-count=3`, not query shape.)

first_seen async-edge note: first_seen is now the first-PROCESSED page view's
timestamp rather than a recomputed MIN(timestamp). With strictly-ordered async
processing these are identical; only a rare async reorder could make first_seen
reflect a slightly later page view. Accepted to avoid the per-event rescan;
matches prior first-row semantics closely.

Postgres-branch caveat: the Postgres `pages_visited` append SQL
(`(... ::jsonb || to_jsonb(?::text))::text`) is dialect-branched and verified
only on the SQLite test path; the Postgres branch is constructed by reasoning —
recommend a Testcontainers/Postgres verification when Docker is healthy.

Commands run:
```
go test ./internal/analytics/ -run 'Session|PageView|Interaction' -count=1
go test ./internal/analytics/ ./internal/handlers/ -count=1
go build ./...
go vet ./internal/analytics/
go test ./internal/analytics/ -run '^$' -bench 'BenchmarkUpdateSessionSummary$' -benchmem -count=3
```
All green.

## L-ADMIN-ANALYTICS — collapse summary aggregates (DUP-03)

`GetAnalyticsSummary` in `internal/analytics/page_analytics.go` issued ~14 sequential
queries for one admin summary. Three groups hit the same table over the same range and
each collapsed into a single statement (net −5 queries). Numbers are byte-identical to
the prior implementation.

### Three collapses
1. **page_views scalar counts (2 → 1):**
   ```sql
   SELECT COUNT(*) AS total_page_views, COUNT(DISTINCT session_id) AS total_sessions
   FROM page_views WHERE timestamp >= ? AND timestamp < ?
   ```
2. **session_summaries scalar metrics (2 → 1)** — one conditional aggregate:
   ```sql
   SELECT COALESCE(AVG(total_duration), 0) AS avg_duration,
          COALESCE(SUM(CASE WHEN total_page_views = 1 THEN 1 ELSE 0 END), 0) AS bounced_sessions
   FROM session_summaries WHERE first_seen >= ? AND first_seen < ?
   ```
   Post-math preserved exactly: `AverageSessionTime = avg_duration`;
   `if TotalSessions > 0 { BounceRate = bounced/TotalSessions*100 }`.
3. **Conversion funnel (4 loops → 1)** — one conditional-distinct query (SELECT built
   dynamically from the steps slice, so it stays generic over its length):
   ```sql
   SELECT COUNT(DISTINCT CASE WHEN page LIKE ? THEN session_id END) AS step0,
          COUNT(DISTINCT CASE WHEN page LIKE ? THEN session_id END) AS step1,
          COUNT(DISTINCT CASE WHEN page LIKE ? THEN session_id END) AS step2,
          COUNT(DISTINCT CASE WHEN page LIKE ? THEN session_id END) AS step3
   FROM page_views WHERE timestamp >= ? AND timestamp < ?
   ```
   Bind order: the 4 `step.page+"%"` patterns, then startDate, endDate. Per-step Sessions
   and DropoffRate math (i==0 → 0; else `(prev-curr)/prev*100`) unchanged.
   `COUNT(DISTINCT CASE WHEN cond THEN x END)` is portable: both SQLite and Postgres make
   the CASE NULL when false and COUNT(DISTINCT) ignores NULLs — identical to the old
   per-step `COUNT(DISTINCT session_id) WHERE page LIKE ?`.

### Tests (internal/analytics/page_analytics_summary_test.go)
- **TestGetAnalyticsSummary_QueryShape** — RED 2/2/4 (page_views scalar / session_summaries
  avg+bounce / funnel statements) on the prior code; GREEN 1/1/1 after. Confirmed RED counts
  via a throwaway probe (2, 2, 4). Defensive checks: no standalone `total_page_views = 1`
  bounce query (outside the new CASE), no surviving per-step `page LIKE` funnel loop.
- **TestGetAnalyticsSummary_NumericEquality** — hand-computed expected values from a
  deterministic seed; PASSES against BOTH the old and new code (guards the conditional-
  aggregate rewrite against silent drift): TotalPageViews=9, TotalSessions=5,
  TotalInteractions=3, TotalConversions=2, AverageSessionTime=200, BounceRate=40,
  ConversionRate=40, funnel sessions [5,2,2,1], dropoff [0,60,0,50].

### Benchmark (BenchmarkGetAnalyticsSummary, SQLite, 200 sessions, -benchmem -count=3)
| | ns/op | B/op | allocs/op |
|---|---|---|---|
| before | ~567,000 | 54,929 | 920 |
| after  | ~470,000 | 41,459 | 638 |

~17% faster, ~24% less memory, 282 fewer allocs (5 fewer DB round-trips).

### Not collapsed (intentional)
user_interactions COUNT, conversion_events COUNT, getTopPages, getTopInteractions, the
device-breakdown and country-breakdown GROUP BYs, and getHourlyTraffic are left separate:
they hit different tables or use different GROUP BY keys and cannot portably merge (SQLite
has no GROUPING SETS). The plan's "≤4 queries" aspiration needs GROUPING SETS; the target
here is the three specific collapses (net −5). getHourlyTraffic still uses Postgres-only
`EXTRACT(HOUR ...)::INTEGER` and errors-then-swallows on SQLite (pre-existing, out of scope).

### Commands
```
go test ./internal/analytics/ -run 'TestGetAnalyticsSummary_QueryShape|TestGetAnalyticsSummary_NumericEquality' -count=1 -v
go test ./internal/analytics/ -run '^$' -bench 'BenchmarkGetAnalyticsSummary' -benchmem -count=3
go test ./internal/analytics/ -run 'Summary|Analytics|Funnel|PageView|Session|Interaction' -count=1
go test ./internal/analytics/ ./internal/handlers/ -count=1
go build ./...
go vet ./internal/analytics/
```
All green.

## L-AI-INSIGHTS — fold conversation counts in GetAiInsights (DUP-04)

### Shape test
`TestGetAiInsights_ConvCountsUseSingleQuery` in `internal/server/ai_dashboard_handler_test.go`.

- **RED** (before): 2 separate `count(*)` statements over `ai_waiter_conversations` — one bare COUNT for `totalConvs`, one filtered COUNT for `successfulUpsells`.
- **GREEN** (after): exactly 1 conditional-aggregate statement identified by `total_convs` or `sum(case when cart_items_added` in the SQL, replacing both COUNTs. Total round-trips 4 → 3.

### Numeric equality
Seed: 5 conversations, 2 with `cart_items_added > 0`. Assert: `total_conversations=5`, `upsell_success_rate=40.0`. All four existing `TestGetAiInsights_*` tests (ActiveAndCompleted, NoLeakAcrossBusinesses, TimeSeriesShape, ConvCountsUseSingleQuery) pass unchanged.

### Folded SQL
```sql
SELECT COUNT(*) AS total_convs,
       COALESCE(SUM(CASE WHEN cart_items_added > 0 THEN 1 ELSE 0 END), 0) AS successful_upsells
FROM `ai_waiter_conversations`
WHERE business_id = ?
```

### Benchmark
| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| Before (×1) | 448,765 | 152,677 | 2,950 |
| After run 1 (×3) | 577,816 | 149,229 | 2,927 |
| After run 2 (×3) | 454,083 | 149,226 | 2,927 |
| After run 3 (×3) | 458,768 | 149,242 | 2,927 |

Allocs drop by ~23/op (one fewer DB round-trip). Latency stable (SQLite microbenchmark; first-run anomaly in run 1 is typical SQLite warmup noise; runs 2–3 match baseline). Memory savings ~3.4 KB/op.

### Commands
```
go test ./internal/server/ -run 'TestGetAiInsights_ConvCountsUseSingleQuery' -count=1 -v
go test ./internal/server/ -run 'AiInsights|AiDashboard|Insights' -count=1
go test ./internal/server/ -run '^$' -bench 'BenchmarkGetAiInsights' -benchmem -count=3
go test ./internal/server/ -count=1
go build ./...
go vet ./internal/server/
```
All green.

## L-DIGEST — project digest-scheduler business columns (SCHED-02)

### Problem
`checkDigests()` in `internal/services/director_digest_scheduler.go` issued a full-row
`Find(&businesses)` (effectively `SELECT *`) over the `businesses` table for every 15-minute
scheduler tick, hydrating sensitive columns — `stripe_customer_id`, `stripe_subscription_id`,
`onboarding_state` — that the digest path never reads.

### Fix
Extracted `loadActiveAIProBusinesses()` returning a 9-column projection:

```go
s.db.
    Select("id", "business_id", "name", "owner_name", "email", "timezone", "default_language", "user_id", "owner_address").
    Where("is_active = ? AND subscription_plan = ?", true, database.SubscriptionPlanAIPro).
    Find(&businesses)
```

Consumer audit (why each column is needed):
- `id` — primary key, used everywhere
- `business_id` — `buildDirectorConsoleURL` (URL slug)
- `name` — `sendDigestEmail` businessName
- `owner_name` — `sendDigestEmail` ownerName
- `email` — recipient + `DetermineBusinessOwnerLanguage`
- `timezone` — `maybeDispatch` local-hour gate
- `default_language` — `DetermineBusinessOwnerLanguage` fallback
- `user_id` — `DetermineBusinessOwnerLanguage` → `GetUserByID`
- `owner_address` — `DetermineBusinessOwnerLanguage` → `GetUserByAddress`

### Tests (`internal/services/director_digest_projection_test.go`)

- **TestDigestProjection_FullRowContainsSecrets** — RED proof: full-row `Find` hydrates
  `StripeCustomerID="cus_LEAKME"`, `StripeSubscriptionID="sub_LEAKME"`, and OnboardingState
  sentinel. Passes before and after the fix (documents the dangerous pre-fix state).

- **TestDigestProjection_ShapeAndParity** — combined RED→GREEN shape + parity test:
  - RED phase: captures the SQL from a full-row `Find`, asserts it contains `SELECT *`
    (or an explicit secret column), confirming the pre-fix code exposes secrets.
  - GREEN phase: captures the SQL from the projected query, asserts it does NOT contain
    `stripe_customer_id`, `stripe_subscription_id`, `onboarding_state`, or `*`; and DOES
    contain all 9 audited column names.
  - Parity phase: all 9 fields are correctly populated; secrets are zero on the projected row;
    `buildDirectorConsoleURL`, ownerName/businessName derivations, and
    `DetermineBusinessOwnerLanguage` produce identical output from full vs projected row.

- **TestDigestProjection_LoadActiveAIProBusinesses** — exercises the production
  `loadActiveAIProBusinesses()` method: 9 fields populated, sentinel secrets zero in every
  returned row, SQL does not reference `stripe_customer_id` or `onboarding_state`.

### Benchmark (`BenchmarkDigestBusinessLoad`, SQLite, 200 AI Pro businesses, -benchmem -count=3)

| Variant | ns/op | B/op | allocs/op |
|---------|-------|------|-----------|
| FullRow (before) | ~4,180,000–4,210,000 | ~1,749,600 | ~35,400 |
| Projected (after) | ~511,000–513,000 | ~1,280,340 | ~5,304 |

**~8x faster, ~27% less memory, ~85% fewer allocations** (30,096 fewer allocs/op — 200 businesses × ~150 fields no longer decoded).

### Commands
```
go test ./internal/services/ -run 'TestDigestProjection' -count=1 -v
go test ./internal/services/ -run 'Digest' -count=1
go test ./internal/services/ -run '^$' -bench 'BenchmarkDigestBusinessLoad' -benchmem -count=3
go build ./...
go vet ./internal/services/
```
All green.

## L-SUBCHECK — project checker business columns; defer status index (SCHED-03)

`getManagedBusinesses()` in `internal/services/subscription_checker.go` loaded the FULL
`database.Business` row for the daily subscription lifecycle scan (`SELECT * ... WHERE
subscription_status IN (...)`), hydrating Stripe IDs, the `onboarding_state` JSON blob,
wallets, and ~150 fields when only ~12 billing columns are read. Replaced with a narrow
projection.

### Final query
```go
database.GetDB().
    Select(
        "id", "subscription_status", "email", "subscription_end_date",
        "cancel_at_period_end", "trial_ends_at", "default_language",
        "owner_name", "subscription_plan", "billing_cycle",
        "grace_period_ends_at", "grace_email_stage",
    ).
    Where("subscription_status IN (?)", statuses).
    Find(&businesses)
```

### Column → consumer audit (all 12 required; none dropped)
| Column | Consumer |
|--------|----------|
| `id` | PK; every targeted update keys on it |
| `subscription_status` | loop + all `should*` checks |
| `email` | reminder/grace/suspend emails + reminder gate |
| `subscription_end_date` | upcoming-reminder window, `shouldStartGrace`, period-end cancellation |
| `cancel_at_period_end` | loop period-end suspension branch |
| `trial_ends_at` | `shouldStartGrace` (trialing) |
| `default_language` | `normalizeBillingLanguage` in all emails |
| `owner_name` | all emails |
| `subscription_plan` | `subscriptionAmountLabel` |
| `billing_cycle` | `subscriptionAmountLabel` |
| `grace_period_ends_at` | `startGracePeriod` / `handleGracePeriod` |
| `grace_email_stage` | `startGracePeriod` / `handleGracePeriod` / `applySuspension` |

`lockout_applied_at` is WRITTEN by `applySuspension` but never READ in this path, so it is
intentionally NOT projected.

### Tests (`internal/services/subscription_checker_projection_test.go`)
- **TestGetManagedBusinessesProjectsBillingColumns** — shape. RED first: current full-row
  `Find` emits `SELECT *` (star count = 1, none of the 12 columns named). GREEN: SELECT names
  all 12 billing columns and never `stripe_customer_id` / `stripe_subscription_id` /
  `onboarding_state` / `*`.
- **TestGetManagedBusinessesParityBillingPopulatedSecretsZeroed** — parity. RED first:
  full-row load hydrates `cus_LEAKME`, `sub_LEAKME`, the onboarding blob, `Name`, `CustomURL`.
  GREEN: the 12 billing fields are populated and those secret/non-billing fields are ZERO
  (not hydrated).
- **TestGetManagedBusinessesWriteSafetyKeepsUnprojectedColumns** — load-bearing write-safety
  regression. Loads via the projected scan, then drives real lifecycle mutations
  (`startGracePeriod`, `applySuspension`) off the projected struct, re-reads the FULL row, and
  asserts (a) billing columns changed (`subscription_status` → grace_period/suspended,
  `grace_period_ends_at`, `grace_email_stage`, `lockout_applied_at`) AND (b) the
  secret/non-billing columns (`stripe_customer_id`, `stripe_subscription_id`,
  `onboarding_state`, `name`, `custom_url`) are STILL INTACT — proving projected-load +
  targeted `UpdateBusinessSubscriptionData` never zeroes unprojected data, and locking the
  invariant so a future refactor to `Save(&business)` can't silently wipe columns. (This test
  passes against the full-row load too — by design; it guards the write path, not the read
  shape.)

### Benchmark (`BenchmarkGetManagedBusinesses`, SQLite, 200 businesses, -benchmem -count=3)

| Variant | ns/op | B/op | allocs/op |
|---------|-------|------|-----------|
| FullRow (before) | ~4,313,000–4,868,000 | ~1,714,259 | ~36,527 |
| Projected (after) | ~605,000–613,000 | ~1,286,377 | ~5,448 |

**~7x faster, ~25% less memory, ~85% fewer allocations** (~31,000 fewer allocs/op).

### Index deferral (intentionally NOT added)
No `businesses(subscription_status)` index was added. The lifecycle scan runs once daily over
a small businesses table; an index is not justified at current scale. Revisit at higher tenant
count — likely as a partial index on the non-terminal billing statuses.

### Commands
```
go test ./internal/services/ -run 'TestGetManagedBusinesses' -count=1 -v
go test ./internal/services/ -run 'Subscription|Grace|Suspension|ManagedBusinesses' -count=1
go test ./internal/services/ -run '^$' -bench 'BenchmarkGetManagedBusinesses' -benchmem -count=3
go test ./internal/services/ -count=1
go build ./...
go vet ./internal/services/
```
All green.

---

## M1 — narrow Business projection in auth middleware (OF-01/MW-02) [SIGNED-OFF]

`HybridAuthenticationMiddleware` set `c.Set("_business", business)` from `GetBusinessByIdOrBusinessId`,
hydrating the full ~94-column Business row (jsonb `onboarding_state`, text blobs, all Stripe IDs) on
every authenticated business-scoped request. Added `GetBusinessAuthScopeByIdOrBusinessId` selecting only
`businessAuthScopeColumns` (id, business_id, name, owner_address, user_id, is_active, subscription_plan,
subscription_status, trial_ends_at, grace_period_ends_at, subscription_end_date, cancel_at_period_end,
lockout_applied_at) and repointed the 3 middleware call sites (middleware.go:452/487/531). Row resolution
rules are byte-identical to the full loader (numeric → PK, no is_active filter; slug → business_id + is_active=true).

### Consumer audit (exhaustive, read-only)
Every reader of the context `_business` value touches only a SUBSET of the 13 columns: the 3 middleware
paths (id/owner_address/user_id), printer handler (id), the billing gate `GetBusinessLockState` via
`businessFromContext` (subscription_status, grace_period_ends_at, cancel_at_period_end, subscription_end_date),
and `hasAIFeatureAccess` (subscription_plan). No field read off the context value is missing. No `.Save(_business)`
or whole-struct JSON marshal of the context value exists; all handler/polling readers re-fetch full rows by ID
(unaffected). DRIFT GUARD documented on `businessAuthScopeColumns` keeping it in sync with `GetBusinessLockState`.

### Tests (`internal/database/business_auth_scope_test.go`)
- `TestGetBusinessAuthScope_ProjectsAuthColumnsNotBlobs` — both numeric+slug paths: projected SELECT carries the
  13 auth columns, never stripe_*/onboarding_state/description blobs; full loader carries them (contrast/RED proof).
- `TestGetBusinessAuthScope_LockStateParity` — 11-state subscription matrix: `GetBusinessLockState(projected)` ==
  `GetBusinessLockState(full)`.

### Benchmark deltas (`-benchmem -count=3`, SQLite micro — Postgres jsonb/text row gains more)

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| Before (`GetBusinessByIdOrBusinessIdFull`) | 73,180–115,140 | 27,702–27,712 | 554 |
| After (`GetBusinessAuthScope`) | 14,602–17,897 | 10,756–10,768 | 142 |

~4.5x faster, bytes −61%, allocs −74% on every authenticated business-scoped request.

### Commands run
```bash
cd backend
go test ./internal/database/ -run 'TestGetBusinessAuthScope' -count=1
go test ./internal/database/ -run '^$' -bench 'BenchmarkGetBusinessByIdOrBusinessIdFull|BenchmarkGetBusinessAuthScope' -benchmem -count=3
go test ./internal/server -race -count=1   # all downstream consumers (subscription mw, auth, RBAC, printer, tier gates)
go test ./internal/database -race -count=1
go vet ./internal/server/ ./internal/database/
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```
All green. Human sign-off obtained before commit (core auth path). Deploy as its own release; watch auth/subscription regressions.

---

## CG — Coverage-gap audit verdicts (verify-then-fix, 2026-06-17)

Six subsystems the completeness critic flagged got a focused read-only access-shape pass. Verdicts:

### CG-1 `internal/database/inventory.go` — 🔧 Low (cold path)
Headline claim REFUTED: the two `Preload("InventoryItem")` (lines 367/440) are on COLD operator
inventory/activity tab reads, NOT the order hot path. The real deduction path
(`DeductApprovedOrderInventoryTx`, 868-976) is fully batched — constant ~6 reads regardless of order
size (recipes/items via `WHERE ... id IN (?)`, no per-line reload). Nested category→item→recipe loops
(534-576/653-699/801-936) are all map-driven, zero queries in-loop. **Only finding:** unprojected
`Preload("InventoryItem")` + `SELECT *` on the two cold reads (Business edge NOT hydrated → no leak).
Fix (optional, Low): projected preload closure. Not hot/benchmark-required.

### CG-2 `internal/events/hub.go` — 🔧 Medium (slow memory leak)
Lock discipline CLEAN (Publish copies the subscriber slice at hub.go:195 and sends outside `h.mu`),
backpressure CLEAN (non-blocking select/drop+metric), SSE handler CLEAN (ctx.Done exit, defer cancel).
**Finding:** `cancel()` deletes `h.subscribers[businessID]` on last-unsubscribe but NEVER prunes
`h.replay[businessID]` — the replay ring (≤256 events, ~25-128KB) is retained permanently per
ever-active business → O(businesses) monotonic heap growth over process lifetime. Fix: add
`delete(h.replay, businessID)` in cancel() when the subscriber list empties (already under `h.mu`).

### CG-3 payment webhook ingestion — 🔧 Medium-High
Idempotency is correct/cheap (indexed `WHERE tx_hash=?` + unique constraint), settlement txn uses
`FOR UPDATE` + SQL-aggregate milestones (no N+1 in the money mutation). **CG-3a (Med-High):**
`WebhookPaymentConfirmation` (payments.go:530) calls full `GetBillByID` (~6 queries incl.
Preload Business/Table/Payments + bill_items + alt_payments) but on the `confirmed` branch reads only
`bill.ID`, then reassigns to the lean txn bill — ~6 wasted queries per settlement webhook
(volume-exposed). Fix: use existing `GetBillByIDLean` on the confirmed branch. **CG-3b (Med):**
fiscal/alert/milestone enqueue runs synchronously in the request goroutine and re-reads the
just-created payment via `GetPaymentByTxHash`; defer to background + pass the payment ID through
(larger change — follow-up).

### CG-4 `internal/database/bill_split.go` expired-hold sweeper — 🔧 Medium
The release is already a single set-based UPDATE (good). **Finding:** after releasing, the loop
(1005-1015) rebuilds `BillSplitState` per expired bill for SSE broadcast — 2 queries/bill
(`loadProjectedBillForSplitTx` + `billSplitStateFromTx`), unbounded M, inside one 60s-ticker txn.
Fix: batch the state rebuild (bills `WHERE id IN (?)` + shares `WHERE bill_id IN (?)`, group in Go) +
bound the pluck with `LIMIT`. Optional partial index `(hold_expires_at) WHERE status='held'`.

### CG-5 `internal/database/payment_ledger.go` UNION — 🔧 Medium-High
The UNION assembly itself is well-bounded: 31-day range guard + 10k-row cap, every per-source scan
index-backed (composite indexes in performance_indexes.go). **Finding:** three callers pass
`time.Time{}` (year 0001) as startDate, bypassing the range guard → all-time scans. Worst:
`GetAdminStats` (admin/stats) fires 8 all-time GROUP BY scans across ALL businesses (summary/count
paths have NO range guard). milestone_tracker per-minute calls are all-time but business-scoped
(index-effective). Fix: add range enforcement to `getRecognizedPaymentSummary` +
`GetRecognizedPaymentBillCount`; pass a real epoch floor in milestone_tracker; pre-aggregate the
admin all-time totals (bigger — follow-up).

### CG-6 `internal/handlers/plugin_handlers.go` — 🔧 Medium (guest-facing N+1)
Loops at 1328 (in-memory scan over pre-loaded payments) and 1678 (map copy) CLEAN; `Preload("Plugin")`
at 160 is 1+1 not in a loop. **Finding:** `GetBusinessPaymentPlugins` (1027, GUEST bill-page load)
calls `guestPaymentPluginConfigured` per payment plugin, firing 2 queries each
(`IsPluginEnabledForBusiness` + `GetBusinessPluginConfig`) → up to ~12 extra queries when the data is
already in the initial JOIN. Fix: project `bp.config` into `GetBusinessPlugins` and check
enabled/config inline (drops the N+1).

**Disposition:** CG-2, CG-3a, CG-4, CG-6, and the bounded parts of CG-5 (range guards + milestone
epoch floor) + CG-1 are well-scoped and will be fixed as their own perf-gated commits. CG-3b
(background-worker deferral) and CG-5 admin all-time pre-aggregation are larger architectural changes
flagged as follow-ups.

---

## CG-2 — REVERSED to WON'T-FIX (replay prune defeats reconnect recovery)

The CG-2 audit recommended pruning `h.replay[businessID]` when the last subscriber unsubscribes.
Implemented (f38b2fdf) then REVERTED (72dc434d) after verifying the SSE reconnect path:

- `sse_handler.go:73` replays genuinely-missed events on reconnect via the `Last-Event-ID` header.
  Per project memory (sse_realtime_reliability), Caddy's `read_timeout` tears down SSE connections
  regularly, so reconnect-with-replay is a load-bearing reliability mechanism, not an edge case.
- Pruning replay the instant subscribers hit zero breaks this for SINGLE-subscriber businesses (the
  common operator-dashboard topology): SSE drop → 0 subscribers → replay wiped → on reconnect the
  operator misses every order/payment event published during the gap until the next poll.
- The "leak" is in fact BOUNDED — `h.replay` is keyed by businessID, ≤256 events each, so total
  memory is O(active tenants) (not unbounded over time) and resets on every deploy. Retaining a recent
  replay ring per active business is the intended design for reconnect recovery.

Net: the prune trades a bounded, working-as-intended memory cost for a user-facing realtime regression.
WON'T-FIX. If tenant count ever makes the bounded memory material, the correct fix is a TTL/grace prune
(retain replay for a few minutes after last-unsubscribe, sweep stale entries) that preserves
short-window reconnect replay — not an immediate prune.

---

## CG-3a — lean bill load on confirmed payment webhook

> Historical: `WebhookPaymentConfirmation`, its `TestWebhookPaymentConfirmation_*` tests and the
> `BenchmarkWebhookConfirmedBillLoad` benchmarks were deleted with the unrouted HMAC-only crypto
> webhook (3aacbe319, OSS hygiene A0.10). The evidence below no longer has a live target.

`WebhookPaymentConfirmation` (`internal/handlers/payments.go`) loaded the bill via the full
`h.db.GetBill` → `GetBillByID` aggregate (root SELECT + `Preload("Business")` + `Preload("Table")`
+ `Preload("Payments")` + `loadBillManagedAlternativePayments` + `billItemsForBillSnapshot` ≈ 5-6
queries) on EVERY webhook. On the `status == "confirmed"` settlement branch the pre-loaded bill is
used only for `bill.ID`, then immediately REASSIGNED by `database.ApplyConfirmedPayment` (which
re-loads the bill under `FOR UPDATE` internally) — so the whole heavy aggregate was wasted on the
volume-exposed, settlement-critical hot path. The non-confirmed branch falls through and serializes
the full bill in the response, so it legitimately needs the full load.

**Fix:** branch the load on the already-computed `status`. Confirmed path → single-query
`database.GetBillByIDLean(uint(billID))` (exist-check + carry ID). Non-confirmed path → unchanged
full `h.db.GetBill(uint(billID))`. The confirmed response is byte-identical: it serializes the bill
returned by `ApplyConfirmedPayment` (the lean txn bill), NOT the pre-loaded one, so dropping the
pre-load's preloads does not alter the response. 404 contract unchanged (same
`ErrCodeBusinessNotFound` / "Bill not found" on either loader error).

**Access-shape test** (`internal/handlers/payments_regression_test.go`,
`TestWebhookPaymentConfirmation_ConfirmedBranchSkipsHeavyBillPreload`): attaches a SQL-capturing
`gorm:query` callback, fires a CONFIRMED webhook for a seeded bill, and asserts the heavy aggregate
PRE-LOAD relations are GONE — no `FROM bill_items` and no `FROM alternative_payments` (quote-agnostic
match: SQLite backticks + Postgres double-quotes). `ApplyConfirmedPayment` + downstream
milestone/fiscal/alert touches (bills, payments, business_revenue_aggregates,
business_milestone_events, operational_alerts) are NOT asserted away — only the bill-aggregate
relations, which are the clean signal that the `GetBillByID` pre-load is gone.
- RED (current full `GetBill` pre-load): `bill_items` + `alternative_payments` SELECTs present → FAIL.
- GREEN (`GetBillByIDLean`): neither fires as part of the bill load → PASS.

Companion tests: `TestWebhookPaymentConfirmation_NonConfirmedBranchEchoesFullBill` (status "pending"
keeps the full `Business` preload, response echoes the Business-hydrated bill, bill stays unsettled)
and `TestWebhookPaymentConfirmation_MissingBillReturns404` (lean path preserves the 404 contract).
Existing idempotency / fiscal-enqueue / amount-magnitude / reject-path webhook tests pass unchanged.

**Benchmark** (`BenchmarkWebhookConfirmedBillLoad` vs `_Before`, SQLite microbench, `-benchmem
-count=3`), bill-load portion of the confirmed webhook (seeded bill + 5 payments + 1 alt-payment):

| | ns/op | B/op | allocs/op | queries |
|---|---|---|---|---|
| Before (`GetBillByID` full aggregate) | ~151,800 | ~207,300 | ~1,191 | ~5 (business + payments + bills + alt_payments + bill_items) |
| After (`GetBillByIDLean`) | ~16,500 | 11,816 | 162 | 1 (bills) |

≈ 9x faster, ≈ 17x fewer bytes, ≈ 7x fewer allocs, 4 fewer queries on the confirmed-webhook bill load.

**Commands:**
```
go test ./internal/handlers/ -run 'TestWebhookPaymentConfirmation_ConfirmedBranchSkipsHeavyBillPreload' -count=1   # RED on old code, GREEN after fix
go test ./internal/handlers/ -run 'Webhook' -count=1
go test ./internal/handlers/ -count=1
go test ./internal/handlers/ -run '^$' -bench 'BenchmarkWebhookConfirmedBillLoad' -benchmem -count=3
go build ./...
go vet ./internal/handlers/
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

## CG-4 — batch expired-hold sweeper state rebuild (N+1)

The 60s expired-hold split sweeper `ReleaseExpiredBillSplitSharesWithStates(now)`
(`internal/database/bill_split.go`, called from `cmd/app/main.go:1857`) plucked
distinct expired-held `bill_id`s, ran ONE set-based release UPDATE (kept as-is),
then rebuilt the SSE broadcast `[]*BillSplitState` with a per-bill loop calling
`loadProjectedBillForSplitTx` + `billSplitStateFromTx` once PER bill — 2·M reads
for M expired bills (an N+1 on the hot scheduler path).

### Fix
- Extracted the pure, no-DB tail of `billSplitStateFromTx` into
  `billSplitStateFromRows(bill Bill, rows []billSplitShareStateRow, now)` (the
  held/available cents math + struct build). `billSplitStateFromTx` keeps its
  single-bill query unchanged and now calls the helper; its 6 callers stay
  behavior-identical (pure refactor).
- Added `loadProjectedBillsForSplitTx(tx, billIDs)` — same projection column list
  as `loadProjectedBillForSplitTx` but one `WHERE id IN (...)` keyed into a
  `map[uint]Bill` (empty input → no query).
- Added `loadProjectedBillSplitStateRowsTx(tx, billIDs)` — one shares query with
  the same 15 columns, `WHERE bill_id IN ? AND status IN (held,settled)`,
  `ORDER BY bill_id ASC, created_at ASC, id ASC`, grouped into
  `map[uint][]billSplitShareStateRow`. The global ORDER BY preserves each group's
  per-bill `created_at ASC, id ASC` ordering that the single-bill path relies on
  (empty input → no query).
- Rewrote the sweeper loop: after the release UPDATE, batch-load bills + shares,
  then build states by iterating `billIDs` in the plucked order and calling
  `billSplitStateFromRows(billMap[id], sharesByBill[id], now)`.

Net query count for the state rebuild: **2 + 2·M → constant 4** (1 pluck +
1 release UPDATE + 1 bills-IN + 1 shares-IN), regardless of M.

### Missing-bill behavior refinement (deliberate, money-safe)
If a plucked `billID` is absent from the batched bill map (an anomalous share
referencing a vanished bill), the new loop SKIPS that state instead of aborting
the whole transaction (the old per-bill loop returned the not-found error and
rolled back). This cannot lose money: the release is unconditional and set-based
(it already committed in the single UPDATE), and the states are a best-effort SSE
notify. A vanished-but-referenced bill was already an integrity anomaly; the new
path degrades gracefully rather than wedging the 60s sweeper.

### LIMIT/IN-list chunking deferred
The pluck is intentionally NOT bounded with LIMIT in this task: bounding it would
require a new release-by-ID-set path (current `releaseExpiredBillSplitSharesTx`
filters by a single `*uint`, not a slice) and would change drain semantics.
Realistic M is small (active split sessions expiring within minutes). LIMIT +
IN-list chunking is an optional future hardening if the expired-hold backlog ever
grows pathological.

### Access-shape test (RED → GREEN)
`TestReleaseExpiredBillSplitSharesWithStatesBatchesStateRebuild`
(`internal/database/bill_split_test.go`). Seeds M=4 bills, each with an active
hold + a settled share + a directly-inserted EXPIRED held share (inserted last,
via raw `Create`, because any hold/settle/read call on the bill self-releases its
expired holds through the per-bill release path).

- RED (old per-bill loop): 4 per-bill `SELECT ... FROM bills WHERE bills.id = N`
  + 4 per-bill `SELECT ... FROM bill_split_shares WHERE bill_id = N` = 2·M state
  reads. New assertions failed (0 batched IN reads).
- GREEN (fix): exactly ONE `... FROM bills WHERE id IN (...)` and exactly ONE
  `... FROM bill_split_shares WHERE bill_id IN (...)`; zero per-bill `= N` reads;
  zero `SELECT *` on either table.

State parity: the test builds the expected `[]*BillSplitState` by replaying the
production set-based release inside a rolled-back side transaction and calling the
OLD per-bill helpers (`loadProjectedBillForSplitTx` + `billSplitStateFromTx`),
then deep-compares BillID, HeldCents, AvailableCents, PaidCents, TotalCents, and
the full `Shares` slice for each bill — plus a full-struct `require.Equal`. The
batched states are byte-identical to the per-bill output. `released` == M (4),
and each post-sweep state has Available 5500 (10000 − 3000 paid − 1500 held),
Held 1500, Paid 3000.

### Benchmark (`BenchmarkReleaseExpiredBillSplitSharesWithStates`, M=20, -benchmem -count=3)

| | ns/op | B/op | allocs/op |
|---|---|---|---|
| Before (per-bill 2·M loop) | ~1.81M | 684,910 | 7,107 |
| After (batched 4 queries)  | ~1.91M | 363,837 | 2,432 |

Bytes/op −47%, allocs/op −66%. (Wall time is dominated by the per-iteration
re-seed of M expired holds, shared by both arms, so ns/op looks flat; the sweep
itself drops from 2 + 2·M = 42 queries to a constant 4 and sheds ~4,700 allocs.)

### Commands
```
go test ./internal/database/ -run 'TestReleaseExpiredBillSplitSharesWithStatesBatchesStateRebuild' -count=1
go test ./internal/database/ -run '^$' -bench 'BenchmarkReleaseExpiredBillSplitSharesWithStates$' -benchmem -count=3
go test ./internal/database/ -run 'BillSplit|Split|Sweep|ExpiredHold' -count=1
go test ./internal/database/ -count=1
go build ./...
go vet ./internal/database/
```

## CG-6 — batch guest payment-plugin config (N+1)

**Finding (DUP/N+1):** `GetBusinessPaymentPlugins` (guest-facing, called on every bill-page load) ran `database.GetBusinessPlugins` once, then for each enabled+visible payment plugin called `guestPaymentPluginConfigured`, which fired TWO queries per plugin — `IsPluginEnabledForBusiness` (`SELECT COUNT(*) ... business_plugins`) + `GetBusinessPluginConfig` (`SELECT COALESCE(bp.config,'{}') ... p.name = ?`). With M visible payment plugins that is 1 + 2·M queries (e.g. 9 for 4 plugins, up to ~13 for ~6).

**Fix (loop only; `GetBusinessPlugins` untouched):**
- Extracted the config-decision logic into shared pure `evalPaymentPluginConfigEnabled(pluginName, config)` (explicit `config["enabled"]` bool/parseable-string wins; absent → default on, with a since-removed per-plugin default-off carve-out). `guestPaymentPluginConfigured` now calls it, so its single-call callers (e.g. `payments.go:275`) keep identical behavior — signature unchanged.
- Added `database.GetBusinessPaymentPluginConfigs(businessID, pluginNames)` — ONE query `SELECT p.name, COALESCE(bp.config,'{}') ... WHERE bp.business_id=? AND p.name IN ? AND bp.is_enabled=true AND p.is_active=true`, returns name→parsed-config; malformed config JSON is omitted+logged (matches old per-plugin config-error skip); empty names → no query.
- Rewrote the loop: collect candidate names (enabled + category=="payment" + `guestPaymentOptionVisible`), ONE batch config call, gate each via `evalPaymentPluginConfigEnabled`; absent-from-batch → skip. Net: GetBusinessPlugins (1) + batch (1) = **2 queries regardless of plugin count**.
- On batch-loader DB error → 500 (consistent with the existing GetBusinessPlugins error path), never a silent zero-options response.

**SECURITY — why `GetBusinessPlugins` was NOT modified:** `GetBusinessPaymentPlugins` returns the `bp` maps to the guest. `GetBusinessPlugins` deliberately projects `config_schema` but NOT `config` (which holds payment secrets / API keys) and has 5 consumers. Adding `config` to that shared projection would leak secrets to guests and widen the blast radius. The batch loader is a dedicated internal read whose maps are consumed in-memory for the enabled-gate only; the appended `bp` maps still carry NO `config` key (only `bp["is_enabled"]=true` is set, as before).

**Access-shape test (RED→GREEN):** `TestGetBusinessPaymentPluginsBatchesConfigLookup` (`internal/handlers/plugin_payment_plugins_perf_test.go`), SQL-recorder logger over 4 seeded enabled payment plugins.
- RED (before): 4× `SELECT COUNT(*) ... business_plugins`, 4× single-row `COALESCE(bp.config ...) p.name =`, 0 batch.
- GREEN (after): 0 COUNT(*) probes, 0 single-row config reads, exactly 1 batched `... p.name IN (...)` config query.
- BEHAVIOR PARITY: seeded `stripe {"enabled":false}` → excluded, `paypal {"enabled":"true"}` → included, `mercadopago` no-enabled-key → included, plus a non-guest-visible plugin → excluded; asserts exact ordered set `["mercadopago","paypal"]` (GetBusinessPlugins orders by category, display_name).
- SECURITY: asserts no returned plugin map has a `config` key AND the raw response body never contains the seeded `must_not_leak` secret marker.

**Benchmark `BenchmarkGetBusinessPaymentPluginsSQLite` (-benchmem -count=3, 4 enabled payment plugins):**
- Before: ~108761–109619 ns/op · 58836 B/op · 940 allocs/op
- After:  ~62185–63021 ns/op · 40115 B/op · 691 allocs/op
- Delta:  ~-43% ns/op, ~-32% B/op, ~-26% allocs/op — and now flat in plugin count (was 1+2·M queries).

**Commands:**
```
go test ./internal/handlers/ -run 'TestGetBusinessPaymentPluginsBatchesConfigLookup' -count=1   # RED then GREEN
go test ./internal/handlers/ -run '^$' -bench 'BenchmarkGetBusinessPaymentPluginsSQLite' -benchmem -count=3
go test ./internal/handlers/ -run 'Plugin|PaymentPlugin' -count=1
go test ./internal/handlers/ -count=1
go build ./...
go vet ./internal/handlers/ ./internal/database/
go test ./internal/database/ -run '^$' -count=1
```

---

## CG-5 — DEFERRED (range guard would break all-time correctness; real fix is pre-aggregation)

The CG-5 audit recommended adding range enforcement to `getRecognizedPaymentSummary` +
`GetRecognizedPaymentBillCount` and passing an epoch floor in milestone_tracker. On verification this
is NOT a safe quick fix:

- The existing guard in `getRecognizedPaymentEvents` (payment_ledger.go:156) ERRORS
  (`ErrRecognizedPaymentEventRangeTooLarge`) on ranges >31 days — it does not clamp. The summary/count
  callers pass `time.Time{}` INTENTIONALLY because they need cumulative ALL-TIME totals: `GetAdminStats`
  (lifetime platform revenue) and the milestone display (revenue/order thresholds like "$10k", "100th
  order"). Adding the error-guard to summary/count would make those callers ERROR — breaking the admin
  dashboard and milestone status. (Same lesson as CG-2: the audit's literal recommendation is harmful.)
- Re-measured severity: the milestone all-time scan is NOT per-minute. The 1-minute MilestoneScheduler
  runs `ProcessPendingMilestoneBatch` (event-driven claim path), not the scan. `GetBusinessMilestones`'
  all-time `GetRecognizedPaymentSummary`/`GetRecognizedPaymentBillCount` run only on-demand (operator
  milestone page) + one-off `BackfillMilestones` — business-scoped and index-backed (LOW frequency).
- The genuinely hot-relative-to-cost path is `GetAdminStats` (super-admin `/admin/stats`): 8 all-time
  GROUP BY scans across ALL businesses. But it is a low-volume super-admin endpoint and every scan is
  already index-backed. The correct fix is materialized/pre-aggregated lifetime totals (or an explicit
  product decision to bound the admin view to a rolling window) — a focused follow-up initiative, out of
  scope for this data-access-shape campaign and not a blind range-clamp.

DEFERRED. Follow-ups (new initiatives, not access-shape fixes): (1) pre-aggregate admin lifetime
revenue/bill-count (e.g. extend the existing `business_revenue_aggregates` to a platform rollup) so
`/admin/stats` reads a snapshot instead of all-time multi-scans; (2) consider having
`GetBusinessMilestones` read the existing `business_revenue_aggregates` rather than re-scanning the
recognized-payment UNION.

## CG-1 — projected InventoryItem preload on cold inventory reads

**Functions fixed**: `ListInventoryRecipesByBusinessID` (line ~364) and `ListInventoryMovementsByBusinessID` (line ~430) in `internal/database/inventory.go`.

**Finding**: Both cold operator reads (RBAC `inventory:read`, not the order hot path) issued `Preload("InventoryItem")` which issued `SELECT *` over `inventory_items`. No N+1 (GORM batches the preload into one `WHERE inventory_item_id IN (...)`), no Business leak. Pure over-fetch of unneeded columns on a cold admin path.

**Verification — field consumption before projecting**:

- **Movements (`ListInventoryMovementsByBusinessID`)**:
  - `frontend/src/components/business/InventoryManager.tsx` line ~1310: `movement.inventory_item?.name`
  - `frontend/src/components/business/InventoryManager.tsx` lines ~1334/1339: `movement.inventory_item.cost_per_unit`
  - **Projected to**: `id, name, cost_per_unit`
  - The audit's original suggestion of `id,name,sku,unit,category` was **incorrect** — it dropped `cost_per_unit`, which would blank the operator's cost column. Corrected here.

- **Recipes (`ListInventoryRecipesByBusinessID`)**:
  - Frontend (`InventoryManager.tsx`): only `recipe.inventory_item_id` (the FK field on the recipe row) is read — zero access to the nested `recipe.inventory_item?.field` object.
  - Backend (`GetInventorySummary`, line ~703): reads `recipe.InventoryItem.Name` as a label fallback when an item is absent/inactive in the active-item map (`itemByID`).
  - **Projected to**: `id, name`

**Access-shape tests** (new file `internal/database/inventory_preload_projection_test.go`):

- `TestInventoryMovementPreloadDropsSelectStar` — RED (SELECT * on inventory_items detected) → GREEN (projected `id,name,cost_per_unit`; FE-consumed `Name` and `CostPerUnit` hydrated; `reorder_threshold`/`is_active` not selected)
- `TestInventoryRecipePreloadDropsSelectStar` — RED (SELECT * on inventory_items detected) → GREEN (projected `id,name`; backend-consumed `Name` hydrated; `reorder_threshold`/`is_active`/`cost_per_unit` not selected)

**Benchmark**: Cold-path byte-only change — no ns/op microbenchmark taken. The over-fetched columns (`sku`, `category`, `current_quantity`, `reorder_threshold`, `is_active`, `created_at`, `updated_at`) are now excluded from the wire. The existing `BenchmarkDeductApprovedOrderInventoryTx` (hot order-deduction path) was not affected — it uses no preload.

**Gate**: `go test ./internal/database/ -run 'Inventory' -count=1` ✓ | `go test ./internal/database/ -count=1` ✓ | `go build ./...` ✓ | `go vet ./internal/database/` ✓

## B6 — partial (table_id, status) index for guest table lookups — DEFERRED (evidence-based no-op)

**Audit claim**: every guest QR scan resolves the active bill for a table via `WHERE table_id = ? AND status IN ('open','partial')`, and the existing `business_id`-leading `idx_bills_*` indexes can't serve it, so it "falls back to a scan". Proposed fix: add `CREATE INDEX idx_bills_table_active ON bills (table_id, status) WHERE status IN ('open','partial')`.

**Verification — the premise is false; the hot path is already optimally indexed**:

- **Migration `000059_active_bill_unique_per_table`** already created a partial **UNIQUE** index that is `table_id`-leading with the identical active-status predicate:
  ```sql
  CREATE UNIQUE INDEX IF NOT EXISTS idx_bills_active_per_table
      ON bills (table_id)
      WHERE table_id IS NOT NULL AND table_id <> 0 AND status IN ('open', 'partial');
  ```
  Because it is UNIQUE on `table_id` within the open/partial predicate, the lookup `WHERE table_id = ? AND status IN ('open','partial')` resolves to **at most one row via a direct index scan** — there is no seq-scan fallback in production.
- GORM's `gorm:"index"` on `Bill.TableID` additionally provides a plain `idx_bills_table_id` btree.
- **Postgres `EXPLAIN (ANALYZE)`** (testcontainer, ~6k seeded bills): with `idx_bills_active_per_table` alone the planner does an `Index Scan` at **cost 8.29**; adding the proposed `idx_bills_table_active` yields an **identical cost-8.29** index scan. No measurable read improvement. Wall-clock `BenchmarkActiveBillByTable` (3×) showed only noise (470–695µs/op before vs 461–1226µs/op after; identical B/op and allocs/op) — no speedup, none claimed.

**Decision**: NOT shipping `idx_bills_table_active`. It is **redundant** with the unique `idx_bills_active_per_table` for reads while adding write-amplification on the hot bills-write path (every order/payment touches a bill) for zero read benefit. Adding `status` as a second key column buys nothing because the unique index already guarantees a single matching row. Per the campaign's verify-then-fix discipline (same call made on the perf/data-ops CG-2/CG-5 findings), B6 is recorded here as an evidence-based no-op rather than shipped.

**If `idx_bills_active_per_table` is ever dropped** (e.g. during a future duplicate-active-bill reconciliation that temporarily removes the unique constraint), revisit adding a non-unique `(table_id) WHERE status IN ('open','partial')` index to preserve the index scan. Until then, no change.

**Gate**: no production code changed; `go build ./...` ✓ (tree restored to pre-B6 state).

## DI-1 — column-scoped business/design save writes (kill the editor lost-update race)

**Problem**: The Business Page editor's save batch fires `updateBusiness` and `updateBusinessDesignSettings` concurrently. Both handlers loaded the full row then persisted via full-row `db.Save(business)`, so whichever writer committed second clobbered the other's columns (e.g. a design-settings save would overwrite a concurrent description edit, and vice-versa) — silent data loss.

**Fix**: Two column-scoped GORM helpers in `backend/internal/database/business.go`, wired into the two handlers:
- `UpdateBusinessExceptDesign` → `db.Model(business).Select("*").Omit(businessDesignColumns...).Updates(business)` (used by `business_handlers.go` `UpdateBusiness`). `Select("*")` keeps zero-value persistence (false bools / empty strings) that the partial-patch handler depends on; `Omit(design_*)` leaves the 14 embedded `design_*` columns untouched.
- `UpdateBusinessDesignOnly` → `db.Model(business).Select(businessDesignColumns).Updates(business)` (used by `business_settings_handlers.go` `UpdateBusinessDesignSettings`). Writes only the 14 `design_*` columns.
- `businessDesignColumns` is the explicit shared list of the embedded `design_*` columns (BusinessDesignSettings, `embeddedPrefix:design_`).

**Access-shape regression tests** (`backend/internal/server/business_save_scoping_test.go`) proving the full-row clobber is gone:
- `TestUpdateBusinessExceptDesign_OmitsDesignColumns` — a non-design edit persists while a stale in-memory `design_primary_color` does NOT overwrite the DB value.
- `TestUpdateBusinessDesignOnly_WritesOnlyDesignColumns` — a design edit persists while a stale in-memory `description` does NOT overwrite the DB value.
- `TestUpdateBusinessExceptDesign_WritesZeroValues` — `false` bool and empty string still persist via the scoped write (the `Select("*")` guarantee).
- `TestSaveBatch_DesignAndBusinessEditsDoNotClobber` — deterministic simulation of the editor's concurrent save batch: two independent loads each persist a disjoint column via the scoped helpers; both edits survive.
- Pre-existing `TestUpdateBusiness_PreservesExistingFieldsOnPartialPatch` and `TestUpdateBusiness_UpdatesDefaultQRSettingsAndAllowsClearingCustomURL` still pass (the partial-patch contract is unchanged).

**Benchmark** — `BenchmarkUpdateBusinessExceptDesign` (SQLite microbenchmark, Apple M3, `-benchmem -count=3`):

- **Before**: full-row `db.Save(business)` (writes all ~110 columns including the `design_*` block).
- **After**: `Select("*").Omit(design...)` (writes all columns except the 14 `design_*`); design path uses `Select(design...)`.
- Numbers (after):
  ```
  BenchmarkUpdateBusinessExceptDesign-8    1357    771912 ns/op    95404 B/op    424 allocs/op
  BenchmarkUpdateBusinessExceptDesign-8    2006    812123 ns/op    95310 B/op    424 allocs/op
  BenchmarkUpdateBusinessExceptDesign-8    3370    748952 ns/op    95309 B/op    424 allocs/op
  ```
  The change is correctness-driven (eliminate the lost-update race), not a latency optimization — omitting 14 columns from the UPDATE is a wash on a single-row write. Allocs/op and B/op are stable across the 3 runs.

**Note (plan deviation)**: the plan's `BenchmarkUpdateBusinessExceptDesign` body called `setupStaffHandlerTestDBForTB(b)` but never created the `businesses` table — that helper's `*testing.B` path (`setupBenchmarkStaffHandlerDB`) only registers the DB + RBAC and does not auto-migrate. Added `gormDB.AutoMigrate(&database.Business{})` after the setup call, mirroring the existing `setupPaymentHistoryPerfDB` benchmark pattern (`payment_history_perf_test.go:26`). The plan's test import path `github.com/payverge/backend/internal/database` was also corrected to the real module path `payverge/internal/database` (now `github.com/stdevmac/payverge/backend/internal/database`).

**Commands run**:
```
go test ./internal/server/ -run 'TestUpdateBusiness(ExceptDesign|DesignOnly)' -count=1            ✓ (3 tests)
go test ./internal/server/ -run 'TestSaveBatch_DesignAndBusinessEditsDoNotClobber' -count=1        ✓
go test ./internal/server/ -run 'TestUpdateBusiness|TestSaveBatch' -count=1                        ✓ (incl. pre-existing partial-patch tests)
go test ./internal/server/ -bench BenchmarkUpdateBusinessExceptDesign -benchmem -run '^$' -count=3 ✓
go build ./...                                                                                      ✓
```

**Deferred (not Phase 1)**: the Google connect/remove handlers (`business_settings_handlers.go:433/463`) still use full-row `database.UpdateBusiness`; that path is single-user/non-concurrent, so converting it is optional hardening, left for a later slice.

---

### Fiscal mapper (Task 10)

**Change**: `MapIssueInputToWSFE` now splits the inclusive total into `NetCents`/`VATCents` using `fiscal.SplitInclusiveVAT`. Factura B and A apply 21% IVA; factura C emits zero VAT. Factura A additionally enforces a CUIT/CUIL recipient (DocTypeCode 80); any other doc type is rejected before the network call.

**Benchmark**: `BenchmarkMapIssueInputToWSFE` — factura_b mapping with DNI recipient, 12100 cents total.

**Command**:
```
go test ./internal/fiscal/providers/ar/ -bench BenchmarkMapIssueInputToWSFE -benchmem -count=3 -run '^$'
```

**Results** (Apple M3, arm64, Go 1.25):

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| 1   | 470   | 160  | 4         |
| 2   | 523   | 160  | 4         |
| 3   | 655   | 160  | 4         |

**Notes**: The VAT split (`fiscal.SplitInclusiveVAT`) adds one floating-point multiply + divide + round — negligible cost; all allocations are from the `WSFEPayload` pointer return and internal string builders in `digitString`. No database access. The mapper is called once per fiscal receipt issuance, so sub-microsecond latency is well within budget.

---

### Fiscal worker (Task 13)

**Change**: New `internal/fiscal/worker.go` (`FiscalWorker.ProcessDue`) drains the `fiscal_jobs` queue, mirroring `services.PluginNotificationWorker`: `ReclaimStaleFiscalJobs` → `ClaimDueFiscalJobs` (FOR UPDATE SKIP LOCKED on Postgres) → per-job processing → terminal/backoff transition + `fiscal_audit_event`. Single-job processing is delegated to `Service.ProcessClaimedJob` (new `internal/fiscal/service_jobs.go`): loads bill+settings+business via explicit column projections, resolves the per-business provider via the `ProviderFactory`, builds the `IssueInput` (receipt type from `resolveReceiptType`; VAT line via `SplitInclusiveVAT`; customer doc from the bill's `FiscalCustomer*` columns), dispatches on `job.Action` (`issue_receipt`/`credit_note`/`status_check`), and persists a `FiscalReceipt`. Error classification: a non-nil provider error is retryable by default unless it wraps the new `fiscal.ErrPermanent` sentinel; a result `Status` of `failed_retryable` retries; `rejected`/`failed_permanent` is terminal (recorded as `failed_permanent`). Retryable failures back off `min(2^(attempts-1)·1m, 6h)` until `attempts >= MaxAttempts`, then go terminal. Every transition clears `locked_at`/`locked_by`.

**Access-shape assertion**: `TestFiscalWorkerLoadUsesProjectionNoSelectStar` registers a GORM `gorm:query` callback that fails if the `bills` load renders `SELECT *`. The repository's `LoadJobContext` selects only the columns the pipeline reads (`billProjectionColumns`/`settingsProjectionColumns`/`businessProjectionColumns`); wide JSON/text bill columns (`items`, `notes`) are excluded. Each of bill/settings/business is fetched once by primary key (no N+1, no relation preloads).

**Benchmark**: `BenchmarkFiscalWorkerProcessDue` — N seeded paid bills + pending `issue_receipt` jobs, no-op authorized provider, claimed in one sweep, on in-memory SQLite (`SetMaxOpenConns(1)` so all reads/writes serialize on one connection — a worst-case for the per-op figure).

**Command**:
```
go test ./internal/fiscal/ -bench BenchmarkFiscalWorkerProcessDue -benchmem -count=3 -run '^$'
```

**Results** (Apple M3, arm64, Go 1.25):

| Run | ns/op  | B/op  | allocs/op |
|-----|--------|-------|-----------|
| 1   | 176461 | 77099 | 861       |
| 2   | 172535 | 77084 | 861       |
| 3   | 134936 | 77100 | 861       |

**Notes**: Per claimed job the worker does ~3 projected primary-key reads (bill/settings/business), one receipt insert + job-link update (one txn), one job-status update, and one audit insert — i.e. it is dominated by the 4 writes, not by `SELECT *` over wide rows. There is no pre-existing worker baseline (this is the first integration of the queue), so the numbers are the as-shipped baseline for regression tracking. New baseline, no regression. Delivery (Task 24), the real `IssueCreditNote` (Task 16), and `GetStatus` (Task 19) are dispatched to but intentionally left to their own tasks.

**Gate**: `go test ./internal/fiscal/...` ✓; `go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1` ✓; `go build ./...` ✓; `go vet ./internal/fiscal/...` ✓.

**Review follow-up (T13 review fixes)**: Two merge-blocking defects found in the T13 review were fixed:
- **C-1 (stranded retries)**: `ClaimDueFiscalJobs`/`ReclaimStaleFiscalJobs` only selected `status = pending`, but the worker parks transient failures in `failed_retryable` — so the entire backoff machinery was dead code (a job never retried automatically; only the manual operator-retry path reset it to `pending`). Both queries now claim `status IN (pending, failed_retryable)` (new `claimableFiscalJobStatuses`), matching the canonical `ClaimPluginNotificationDeliveries` (which claims `IN (pending, retry)`). A `failed_retryable` job is re-claimed only once `next_attempt_at` is due. Query shape change is `status = ?` → `status IN (?,?)`, still fully served by `idx_fiscal_jobs_claim (status, next_attempt_at)`; re-running `BenchmarkFiscalWorkerProcessDue` shows ~160055 ns/op / 77133 B/op / 861 allocs/op — within run-to-run variance of the baseline above. Regression guard: `TestFiscalWorkerRetriesFailedRetryableJobOnceDue` (a job is NOT claimed before `next_attempt_at`, IS claimed and retried-to-success after the backoff elapses).
- **C-2 (credit notes DOA)**: `processCreditNoteJob` built a `CreditNoteInput` with no `Settings`, so the real AR mapper rejected every credit note on CUIT/PoS validation and the job churned its whole retry budget. Now passes `Settings: settingsFromModel(jobCtx.Settings)`; `TestFiscalWorkerDispatchesCreditNote` asserts the input carries the issuer CUIT + point of sale.
- Minor: renamed the `cap` local in `backoffForAttempt` to `maxBackoff` (builtin shadow).

**Fiscal worker metrics (Task 28)**: Added three Prometheus series mirroring the plugin-notification worker — `payverge_fiscal_receipts_total{status}` (incremented per terminal/transition outcome in `applyOutcome`), `payverge_fiscal_job_attempts_total{action}` (per `processJob` call), `payverge_fiscal_jobs_queue_depth` (gauge, sampled once per `ProcessDue` from the new `database.CountDueFiscalJobs`). Perf: the gauge adds exactly one `COUNT(*)` per worker tick, using the same predicate as the claim (`status IN (pending,failed_retryable) AND next_attempt_at due AND locked_at IS NULL`) so it is fully served by `idx_fiscal_jobs_claim (status, next_attempt_at)` — one indexed count every `FISCAL_WORKER_INTERVAL_SECONDS` (default 10s), negligible. Counter increments are O(1) lock-free. Test `TestFiscalWorkerRecordsMetrics` asserts deltas via `prometheus/testutil`. go.mod gained one indirect dep (`kylelemons/godebug`, a testutil transitive).

**Receipt delivery (Task 24)**: An authorized `issue_receipt` job now delivers the receipt to the customer — render PDF + QR, upload both to the PROTECTED S3 bucket (`s3.UploadBytesProtected`, new), email the PDF as an attachment to the bill's CRM-customer email (new `SendFiscalReceiptEmail` + `fiscal_receipt` template in eng/es/es_ar, guest footer), and best-effort enqueue a `PrintJobKindReceipt`. Hooked in `processIssueJob` (service_jobs.go) after `persistReceiptResult` returns an authorized issue receipt; gated behind an injected `ReceiptDispatcher` (nil = disabled). Idempotent via the new `fiscal_receipts.delivered_at` column (mig 000087, model `FiscalReceipt.DeliveredAt`): `DeliverReceipt` short-circuits when non-nil and stamps it only AFTER a successful email send (or when there is no recipient), so a transient email failure leaves it nil for the next sweep to retry. Delivery is fully best-effort — a render/upload/email/print failure is logged and never fails the fiscal job.

**Perf**: Delivery only runs when a dispatcher is injected, so the worker benchmark (nil dispatcher) is unchanged: `BenchmarkFiscalWorkerProcessDue` ~257990–322897 ns/op / 77844–77970 B/op / 866 allocs/op (Apple M3, Go 1.25, `-count=2`) — within run-to-run variance of the baselines above (the small alloc delta vs. the original 861 is from the added `crm_customer_id` column in `billProjectionColumns`, a single extra projected scalar, not a new query). When delivery does run it adds exactly one extra read — the recipient lookup — which is a narrow projection (`SELECT email FROM customers WHERE id = ? LIMIT 1`, no `SELECT *`, no Preload of the wide customer row). Access-shape guard: `TestDeliverReceiptCustomerEmailUsesNarrowProjection`. The PDF render + S3 upload run after authorization, off the claim/issue latency path.

**Command**: `go test ./internal/fiscal/ -bench BenchmarkFiscalWorkerProcessDue -benchmem -count=2 -run '^$'`

**Tests**: `TestDeliverReceiptHappyPath` (exactly one email with PDF bytes + one print + upload + `DeliveredAt` set), `TestDeliverReceiptIdempotent` (a second pass over a delivered receipt does NOT re-send), `TestDeliverReceiptEmailFailureLeavesUndelivered` (email failure leaves `DeliveredAt` nil and does NOT fail the job), `TestDeliverReceiptNoCustomerEmail` (Consumidor Final still prints + stamps delivered, no email), `TestDeliverReceiptCustomerEmailUsesNarrowProjection`.

**Gate**: `go build ./...` ✓; `go test ./internal/fiscal/... ./internal/emails/ ./internal/s3/...` ✓; `go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1` ✓; `go vet ./internal/fiscal/...` ✓.

---

## AFIP/ARCA fiscal integration — T31 feature gate (31-task feature complete)

The dormant AR fiscal subsystem is now connected end-to-end: bill paid → enqueue →
env-gated worker → credential-aware provider → WSAA login + WSFE FECAESolicitar →
authorized FiscalReceipt (CAE + AFIP QR) → S3 + email/print delivery; refunds →
nota de crédito (full-reversal only); RetryReceipt → FECompConsultar status
reconciliation; operator FE (settings/credentials/validate/receipts/credit) +
guest/operator customer fiscal-identity capture.

**Gate (all green):**
- `go -C backend build ./...` ✓; `go -C backend vet ./internal/fiscal/... ./internal/server ./internal/handlers ./internal/emails ./internal/s3 ./internal/config ./internal/database` ✓.
- `go -C backend test ./internal/fiscal/... ./internal/server ./internal/handlers ./internal/emails ./internal/s3 ./internal/config ./internal/database -count=1` ✓.
- Frontend: `npx tsc --noEmit` ✓; jest fiscal/checkout suites 46/46 ✓; guest + operator locale parity ✓; production `npm run build` ✓.
- Benchmark: `BenchmarkFiscalWorkerProcessDue` ~160k ns/op / 861 allocs (nil-dispatcher path unchanged by delivery).

**Deploy (backend-first):** migrations 000086 (bill fiscal-customer cols) + 000087
(fiscal_receipts.delivered_at), both idempotent. `PLUGIN_SECRET_KEY` required in
prod (config validator rejects FISCAL_WORKER_ENABLED=true without it). Feature is
DARK by default: `FISCAL_WORKER_ENABLED=false` → jobs enqueue but never process
(loud startup warning). Gate prod enablement on the `afip_homo` build-tagged e2e
(`go test -tags afip_homo ...`) being green for a real test CUIT. See
docs/fiscal/argentina-afip.md.

**Accepted follow-ups (non-blocking):** (a) status_check retry-budget exhaustion
doesn't reconcile the receipt row (worker.applyOutcome would need attempts/max);
(b) AR mapper deterministic validation errors should wrap fiscal.ErrPermanent to
fail-fast; (c) receipt delivery is single-shot best-effort — no auto-retry sweep
(operator re-send available; a delivered_at-IS-NULL sweep is the future path);
(d) partial-refund credit notes are deferred to a manual partial nota de crédito.

---

## AFIP/ARCA fiscal — completeness follow-ups (T1–T11, all the above closed)

Every "accepted follow-up" above is now resolved, plus a duplicate-issue guard
surfaced during planning. Landed as 11 commits.

- **T1** FECompConsultar surfaces the full authorization tuple (CAE, number,
  CAE expiry, ImpTotal/MonId/MonCotiz/DocTipo/DocNro); ReceiptStatus enriched.
- **T6 / T5 (I-1)** AFIP `Resultado=R` rejections wrap `fiscal.ErrPermanent`
  (transient allowlist + empty/ambiguous Resultado stay retryable); all
  deterministic mapper/validation errors (CUIT, PoS, receipt type, recipient
  doc, factura_a-without-CUIT, currency, original-number, CAE) wrap ErrPermanent
  → fail-fast instead of churning the 5-attempt budget.
- **T4** duplicate-issue guard: on a non-permanent RequestCAE error the AR
  provider reconciles via FECompConsultar(N) and adopts a lost-response
  authorization (no new number); persist failure after authorization is bounded
  inline-retried then goes terminal `failed_permanent` (NOT retryable) with the
  CAE+number logged — re-issue can never duplicate the invoice. Single-worker
  assumption documented (multi-replica needs a per-(CUIT,PtoVta,CbteTipo) lock).
- **T2 (D-3)** status_check resolving to authorized backfills CAE/QR/number via
  `repository.BackfillAuthorizedReceipt` (narrow `Updates(map)`, only non-empty
  fields → never clobbers an existing CAE/QR; issued_at stamped only when NULL →
  never moves a legal issue date) then delivers.
- **T3 (SG-1)** retry-budget exhaustion reconciles the linked receipt row to
  failed_permanent (best-effort) so no orphaned non-terminal receipt remains.
- **T7** partial-amount notas de crédito: `CreditNoteInput.AmountCents` + mig
  000088 `fiscal_jobs.credit_amount_cents` (idempotent ADD COLUMN IF NOT EXISTS);
  per-refund idempotency key discriminator (`...:credit_note:<disc>`) so each
  partial refund gets its own NC; the refund handler now fires on partials too
  (fiscal amount = bill portion, tips excluded).
- **T8** delivery retry sweep: `repository.ListUndeliveredAuthorizedReceipts`
  (`status='authorized' AND delivered_at IS NULL AND issued_at < olderThan`,
  oldest-first, bounded) + `Service.SweepUndeliveredReceipts` run from the worker
  ProcessDue (5-min grace so it never races inline delivery). mig 000089 partial
  index `idx_fiscal_receipts_undelivered ON (issued_at) WHERE status='authorized'
  AND delivered_at IS NULL` keeps the scan selective. The sweep reuses
  `LoadJobContext` (narrow projections) via a synthetic job from each receipt.
- **T9** operator manual re-send/re-print: `Service.ResendReceipt` force-delivers
  past the delivered_at guard + audit event; `POST /businesses/:id/fiscal/
  receipts/:receiptId/resend` (RBAC fiscal:write); FE button on authorized rows.
- **T10 (D-2)** single canonical `fiscal.ResolveIssuableReceiptType`; worker
  `resolveReceiptType` and `policies.ArgentinaPolicy` both delegate so they can
  no longer diverge (RI-without-CUIT → factura_b in both, policy still warns).
- **T11** FE soft CUIT hint when responsable_inscripto lacks a CUIT/CUIL (never
  blocks submit).

**Access-shape:** the new repo reads are guarded — `BackfillAuthorizedReceipt`
and `ListUndeliveredAuthorizedReceipts` are narrow/bounded; the undelivered sweep
is bounded per tick + backed by a partial index, and reuses the existing
projection-based `LoadJobContext` rather than rehydrating wide rows. The
nil-dispatcher worker benchmark path is unchanged (the sweep no-ops without a
dispatcher).

**Gate (all green):** `go -C backend build ./... && go vet ./internal/fiscal/...
./internal/handlers/...` ✓; `go -C backend test ./internal/fiscal/...
./internal/handlers/... ./internal/server/... -count=1` ✓. Frontend (T9 button +
T11 hint): `npm run typecheck`, jest fiscal/identity suites, guest+operator
locale parity ✓.

**Deploy (backend-first):** migrations 000088 (fiscal_jobs.credit_amount_cents)
+ 000089 (undelivered partial index), both idempotent. No behavior change while
`FISCAL_WORKER_ENABLED=false` (still dark by default).

---

## Business Page Phase 2 — Task 1: project `timezone` on the guest hot path (PARITY-2)

**Finding PARITY-2:** the storefront Open/Closed badge and reservation slots must
compute in the venue's IANA timezone, but `publicBusinessProjection` (the shared
guest-hot-path projection) never emitted `timezone` — it lived only in
`staffBusinessProjection`. Added `"timezone": business.Timezone` to the
`publicBusinessProjection` `gin.H` literal in `public_business_helpers.go`. Both
public read paths pick it up because they call the same shared projection: the
storefront-by-custom-URL handler (`business_settings_handlers.go:1176`) and the
non-owner `GetBusiness` branch (`business_handlers.go:716`).

**Access-shape tests (TDD, failing-first):**
- `TestPublicBusinessProjectionStorefrontKeys` — now asserts `timezone` is
  present in the projection (added to the storefront-keys presence slice).
- `TestPublicBusinessProjectionEmitsTimezone` — new value-shape test: a business
  with `Timezone: "Asia/Dubai"` projects `timezone == "Asia/Dubai"`.
- `TestPublicBusinessProjectionKeysAreBlessed` — `timezone` blessed in
  `blessedPublicKeys` (non-sensitive; already exposed to staff via
  `staffBusinessProjection`, confirming it is safe for non-owners).

**Perf:** allocation-neutral — a single static key/value added to an existing
`gin.H` literal already built on this path. No new DB query, no new preload, no
migration; `business.Timezone` is already SELECTed on the public read path
(full-row load). Per the Backend Performance Gate, the rationale + access-shape
presence/value tests are the evidence; a micro-benchmark of an unchanged build
path would be misleading (no benchmark delta to report).

**Gate (all green):**
- `go test ./internal/server/ -run 'TestPublicBusinessProjection' -count=1` ✓
  (OmitsSensitiveFields, StorefrontKeys, KeysAreBlessed, EmitsTimezone all PASS).
- Adjacent compile/no-op gate:
  `go test ./internal/server ./internal/handlers ./internal/database -run '^$' -count=1` ✓ (clean).

**Deploy (backend-first):** additive + backward-compatible — must ship before the
storefront depends on the field, but introduces no breaking change.

---

## SEO-2 — public storefront-slug list endpoint for the sitemap (Phase 3 Reach, Task 5)

**Finding:** `sitemap.ts` runs unauthenticated at build/request time and had **no**
public surface to enumerate storefronts — the only public business surface is
per-`customUrl` reads; the list endpoints (`GET /businesses`,
`GET /customer/businesses`) are auth-gated. So the sitemap could not advertise
`/b/<slug>` pages at all (SEO discovery gap). Fix: a new narrowly-projected,
bounded, public endpoint returning only `{custom_url, updated_at}` for
`business_page_enabled AND is_active` rows with a non-empty slug — no sensitive
fields, mirroring `loadPublicBusinessByCustomURL`'s publish/active gate so the
sitemap and the page agree on what is public.

**Access shape (the dangerous-query regression guard):**
- `database.ListPublishedStorefrontSlugs(limit int) ([]StorefrontSlug, error)` —
  `db.Model(&Business{}).Select("custom_url", "updated_at")` (two-column
  projection, **no `SELECT *`, no `Preload`**), `Where("business_page_enabled = ?
  AND is_active = ? AND custom_url <> ''", true, true)`, `Order("updated_at
  DESC")`, `Limit(limit)`. Bounded result set regardless of table growth.
- `StorefrontSlug{CustomURL string; UpdatedAt time.Time}` — only the routable
  slug + last-modified time leave the boundary.
- Handler `server.ListPublishedStorefronts` caps the limit at **5000**
  (bounded; paginate if ever hit — flagged as a follow-up in the plan's Open
  questions). Returns `{"storefronts": [...], "count": n}`.
- Route: `publicRoutes.GET("/business/storefronts", ...)` registered **before**
  the `GET /business/:customUrl` wildcard. Gin v1.10.0's tree router resolves the
  static segment first — verified with a throwaway probe
  (`/business/storefronts` → static handler; `/business/some-cafe` → wildcard
  with `customUrl=some-cafe`; no registration panic). URL stays `/business/...`
  (NOT the `/businesses/storefronts` fallback), so Task 6's `sitemap.ts` fetches
  `${API_URL}/api/v1/business/storefronts`.

**Tests (access-shape, regression-first):**
- `TestListPublishedStorefrontSlugs` — only published+active rows returned
  (`alpha`), unpublished (`beta`)/inactive (`gamma`)/blank-slug (`""`) excluded,
  and `UpdatedAt` is projected (non-zero). Seeding caveat documented in-test:
  `IsActive` has `gorm:"default:true"`, so the inactive row is forced via a
  post-create `Update("is_active", false)`; each row needs a distinct
  `BusinessId` (uniqueIndex).
- `TestListPublishedStorefrontSlugsRespectsLimit` — `LIMIT` is honored (3 seeded,
  `limit=2` → 2 returned) so the read stays bounded.

**Perf (`-benchmem`, `-count=3`, 500-row seeded in-memory SQLite):**
`BenchmarkListPublishedStorefrontSlugs` — `~1.9–5.2 ms/op`, `162125 B/op`,
`4071 allocs/op` (bytes/allocs flat across runs; ns/op variance is in-memory
SQLite + macOS scheduling noise, not query-shape variance). This is a single
two-column projected, indexed-filtered, `LIMIT`-bounded read on a public route —
no preloads, no N+1, no full-row hydration. No prior baseline to delta against
(net-new endpoint); the bounded projection + cap are the perf guarantee. If
storefront count approaches 5000, paginate (follow-up).

**Commands run (all green):**
- `go test ./internal/database/ -run "Storefront" -count=1` ✓ (both tests PASS).
- `go test ./internal/database/ -run '^$' -bench 'BenchmarkListPublishedStorefrontSlugs' -benchmem -count=3` ✓.
- Adjacent compile/no-op gate:
  `go test ./internal/server ./internal/handlers ./internal/database -run '^$' -count=1` ✓ (clean).
- `go build ./...` ✓.

**Deploy (backend-first):** ship this endpoint **before** `sitemap.ts` fetches it
(Task 6). The FE degrades gracefully if the endpoint 404s, but BE goes first.
Additive + backward-compatible: new public route, no schema change, no migration.

---

## AI-waiter L11 — fetch table once in chat path (2026-06-20, Track C / menu-ai-remediation)

`HandleAIWaiter` resolved the QR table twice on every ordering-mode turn: once in
`validateAIWaiterPublicScope` (scope check) and again for the server-side bill
context. `validateAIWaiterPublicScope` now returns the resolved `*database.Table`
and the bill-context site reuses it, dropping one `GetTableByCode` SELECT per
chat turn.

**Access-shape regression test:** `TestHandleAIWaiter_FetchesTableOnce`
(internal/server/ai_waiter_handler_test.go) registers a `gorm:query` after-callback
counting SELECTs on the `tables` table during one `HandleAIWaiter` call and asserts
`<= 1` (was 2 before the fix).

**Benchmark:** `BenchmarkHandleAIWaiterTableFetch` (SQLite microbench, ordering-mode
turn; transcript truncated before the per-session cap with the timer paused).

```
cd backend && go test ./internal/server/ -run '^$' -bench BenchmarkHandleAIWaiterTableFetch -benchmem -count=3
```

| metric    | before (count=3)            | after (count=3)             |
|-----------|-----------------------------|-----------------------------|
| B/op      | 248710 / 249228 / 248911    | 240744 / 240882 / 240937    |
| allocs/op | 1931 / 1931 / 1931          | 1802 / 1802 / 1802          |

allocs/op: -129 (~6.7%), B/op: ~-8000 (~3.2%) — consistent with eliminating one
table SELECT + row hydration. ns/op is unreliable in this shared in-memory SQLite
microbench (SetMaxOpenConns(1) serializes all queries → high wall-time variance,
ranges overlap), so allocs/op + B/op are the load-bearing signal.

**Callers updated for the new 4-value signature:** `HandleAIWaiter`,
`GetAiWaiterMessages`, `CreateAIWaiterSession`. Gate: `go test ./internal/server
-run AIWaiter -count=1` ✓; `go test ./internal/server ./internal/database
./internal/services -run '^$' -count=1` ✓; `go build ./...` ✓.

## Phase 0 — reliability remediation reusable primitives (2026-06-20)

Five small, independently unit-tested packages under `internal/`, built foundations-first
for the backend reliability+perf remediation (2026-06-20 backend reliability/perf audit). **No hot path is
wired to them yet** — later phases (P1–P9) adopt them — so there is **no benchmark delta this
phase**; benchmarks land when callers adopt these primitives.

### Packages
- **`internal/logger`** (extended) — injectable panic-reporter seam: `SafeGo`/`SafeGoNamed`/`SafeTick`
  now forward recovered panics to an `atomic.Pointer`-held reporter (Sentry in prod, installed once
  from `cmd/app/main.go` after `sentry.Init`; the goroutine's real recover-site stack is attached to
  the Sentry event). The `logger` package intentionally does **not** import sentry-go. Race-safe under `-race`.
- **`internal/schedclaim`** — atomic claim-before-side-effect. `Claim(ctx, table, doneCol, keyCol, keyVal)`
  runs a **single** `UPDATE <table> SET <doneCol>=CURRENT_TIMESTAMP WHERE <keyCol>=? AND <doneCol> IS NULL`
  (no SELECT) and returns `claimed == (RowsAffected==1)`. Exactly-one-winner verified under 20-goroutine
  contention by `TestClaim_exactlyOneWinnerUnderContention` (also asserts zero attempt errors).
- **`internal/httpclientx`** — one process-wide pooled `*http.Client` (Timeout 30s) + `Do(ctx, req, deadline)`
  that applies a per-feature deadline and releases the deadline context exactly on `Body.Close()` (no ctx leak).
- **`internal/boundedcache`** — generic concurrency-safe TTL + max-entries LRU (`Get/Set/Invalidate/InvalidatePrefix/Len`),
  bounding both staleness and memory; replaces ad-hoc map+RWMutex caches in later phases.
- **`internal/auththrottle`** — DB-backed failed-attempt lockout with exponential backoff, keyed by
  `(principal, kind)`, persisted in new table `auth_attempts` (**migration 000091**, idempotent, deploy
  backend-first; mirrored in the AutoMigrate safety net). Backoff cap (`<<` shift) guards int64 overflow
  via `lockout <= 0 || lockout > MaxLockout → MaxLockout`. **Known limitation (acceptable at single-instance):**
  two concurrent `Record` calls for the same brand-new `(principal, kind)` can race on the unique index so
  one `Create` returns a unique-violation and that single increment is dropped; the row then exists for the
  next attempt and the 5-attempt margin absorbs it. An `ON CONFLICT DO UPDATE` upsert is a possible future
  hardening if multi-replica is ever adopted.

### Commands run
- Cross-package compile/no-op gate: `go test ./internal/logger ./internal/schedclaim ./internal/httpclientx ./internal/boundedcache ./internal/auththrottle ./internal/database ./cmd/... -run '^$' -count=1` ✓
- Race suites: `go test ./internal/logger ./internal/schedclaim ./internal/httpclientx ./internal/boundedcache ./internal/auththrottle -race -count=1` ✓ (schedclaim + auththrottle also green under `-count=3`)

---

## P1.6 — Pricing/storefront cache invalidation wired to write paths (2026-06-20)

### Finding
`InvalidatePricingCache`, `InvalidateBusinessCustomURL`, `InvalidatePublicGuestTableContext`, and
`InvalidatePublicGuestBusinessExtras` existed in `internal/services/pricing_cache.go` but were called
from **nowhere** in production. `getPricingSnapshot` caches menu+offers+bundles under `PricingCacheTTL`
(5 s); `PriceOrderInputsByBusinessID` prices every order from this snapshot. An operator who 86'd an
item or raised a price could be priced at the stale snapshot for up to 5 s.

### Benchmark
`BenchmarkInvalidatePricingCache` in `internal/services/pricing_cache_invalidation_test.go`

| Run | ns/op | B/op | allocs/op |
|-----|-------|------|-----------|
| Before (3 runs) | 73.15 / 74.31 / 73.48 | 112 | 1 |
| After  (3 runs) | 87.90 / 84.20 / 78.62 | 112 | 1 |

The ~10 ns variance is normal RW-mutex noise; B/op and allocs/op are identical. No regression.

### Access-shape regression tests
- `TestInvalidatePricingCache_DropsCachedSnapshot` (services) — unit-level: primes the cache and
  asserts `InvalidatePricingCache` evicts the entry (cache ENABLED, not disabled).
- `TestUpdateMenuItem_InvalidatesPricingCache` (server) — handler-level: primes cache, calls
  `UpdateMenuItem` handler via Gin test context with a new price; asserts `MenuDataForBusiness`
  returns the new price immediately (within the 5 s TTL window).
- `TestCreateOffer_InvalidatesPricingCache` (server) — handler-level: primes cache, calls
  `CreateOffer` handler; asserts the new offer appears in `MenuDataForBusiness` result.
- `TestInvalidateBusinessCustomURL_DropsCustomURLEntry` (services) — unit-level: primes
  `businessByCustomURL` cache and asserts eviction on `InvalidateBusinessCustomURL`.

### Write paths wired
**`internal/server/business_menu_core_handlers.go`** (`services` import added):
- `CreateMenu` — after `database.UpdateMenu` (update path) and after `database.CreateMenu` (create path)
- `AddMenuCategory` — after versioned/legacy DB write succeeds
- `UpdateMenuCategory` — after versioned/legacy DB write succeeds
- `DeleteMenuCategory` — after versioned/legacy DB write succeeds
- `AddMenuItem` — after versioned/legacy DB write succeeds
- `UpdateMenuItem` — after versioned/legacy DB write succeeds
- `DeleteMenuItem` — after versioned/legacy DB write succeeds

**`internal/server/menu_enhancements.go`** (services already imported):
- `CreateOffer`, `UpdateOffer`, `DeleteOffer` — after respective `database.*` calls
- `CreateBundle`, `UpdateBundle`, `DeleteBundle` — after respective `database.*` calls

**`internal/server/business_handlers.go`** (services already imported):
- `UpdateBusiness` — captures `oldCustomURL` before mutation; after `database.UpdateBusinessExceptDesign`
  calls `InvalidateBusinessCustomURL(oldCustomURL)` + `InvalidateBusinessCustomURL(newSlug)` if changed
  + `InvalidatePricingCache(business.ID)`.

### Commands run
```
# RED (before implementation):
go test ./internal/server/ -run 'TestUpdateMenuItem_InvalidatesPricingCache|TestCreateOffer_InvalidatesPricingCache' -v
# Both FAIL — stale snapshot served within TTL

# Baseline benchmark:
go test ./internal/services/ -run '^$' -bench BenchmarkInvalidatePricingCache -benchmem -count=3
# 73–74 ns/op, 112 B/op, 1 alloc/op

# GREEN (after implementation):
go test ./internal/server/ -run 'TestUpdateMenuItem_InvalidatesPricingCache|TestCreateOffer_InvalidatesPricingCache' -v
# PASS PASS

# After benchmark:
go test ./internal/services/ -run '^$' -bench BenchmarkInvalidatePricingCache -benchmem -count=3
# 78–88 ns/op, 112 B/op, 1 alloc/op — no regression

# Adjacent compile/no-op gate:
go test ./internal/server/ ./internal/services/ ./internal/handlers/ -run '^$' -count=1
# ok ok ok

# Full services suite:
go test ./internal/services/ -count=1
# ok
```

## Phase 2 — security & abuse hardening (2026-06-20)

Five security fixes; **no new migration** (the `auth_attempts` table ships in P0 migration `000091`).
Cross-replica limiter sharing remains documented-deferred (single-instance deployment).

### Fixes
- **Manager-PIN brute-force lockout (HIGH)** — `RequireManagerPIN` (`internal/server/manager_pin.go`) now
  consults the `auththrottle` primitive BEFORE bcrypt: a locked staff PIN is rejected with `429`/`RATE_LIMITED`
  even when the supplied PIN is correct. Principal is per-staff (`staff:<id>`), so one staff's lockout never
  locks another. `Record` on wrong PIN, `Clear` on success. Wired in `cmd/app/main.go` (Config 5 attempts /
  15m window / 1m→1h backoff) plus a dedicated `PaymentRateLimit` on the void/refund routes.
- **Email/password login account-lockout (MEDIUM ×2)** — `AuthHandler.Login` (`internal/auth/handlers.go`)
  gains a nil-safe `auththrottle` field: repeated wrong-password logins lock the account (429 even with the
  correct password once locked). Principal = normalized email, kind `password_login`; lock-check is fail-open
  on DB error (brute-force protection degrades rather than taking login down). Wired in main (same Config).
- **TRUSTED_PROXIES fail-loud in production (MEDIUM)** — new `server.TrustedProxiesConfigured`; main emits a
  production-only WARNING (non-fatal) when no trusted proxies are set, and the production compose file
  defaults `TRUSTED_PROXIES` to `172.16.0.0/12` (covers Caddy's container IP on the Docker bridge).
- **Payment limiter NAT-collapse (MEDIUM)** — `PaymentRateLimit` (`internal/middleware/rate_limiter.go`) now
  keys by `payment:biz:<bizID>:bill:<billID>` when a bill param is present (so two different bills behind one
  venue NAT/IP no longer throttle each other), with the original `payment:ip:<clientIP>` fallback preserved
  for no-bill routes (PayPal/Tron return). Covers `:bill_id` and `:bill_number` param forms.
- **`GetByDateRange` SQL-injection footgun (LOW)** — `internal/database/repository.go` validates the
  `dateField` column-name argument against an anchored identifier allowlist
  (`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`) and returns an error before any DB access. The one
  real caller passes the literal `"created_at"`.

### Commands run
- Race gate (shared throttle/middleware state): `go test ./internal/server ./internal/auth ./internal/middleware ./internal/database -race -count=1` ✓
- `go build ./...` ✓

---

## ReportScheduler atomic claim — Task P3.4 (2026-06-20)

### Benchmark
`BenchmarkClaimDueReportSchedule` in `internal/database/report_schedule_claim_test.go`.

### What changed
`ClaimDueReportSchedule` replaces the non-atomic `time.Since(LastSentAt) < 30m` guard with a single guarded
`UPDATE report_schedules SET last_sent_at=now, next_send_at=<next> WHERE id=? AND next_send_at<=? AND (last_sent_at IS NULL OR last_sent_at < ?)`.
`RowsAffected==1` means this caller won; zero means an overlapping tick or a second replica already stamped it.
The post-send `UpdateReportScheduleLastSent` call is removed — stamp happens at claim time.
Recurring re-arm: `calculateNextSendTime` is called inside the claim to compute the next window before the UPDATE fires.

### Setup
- Apple M3, Go 1.25, `goos: darwin goarch: arm64`
- SQLite in-memory microbench (one INSERT + one guarded UPDATE per iteration)

### Before numbers (`-benchmem -count=3`)
| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkClaimDueReportSchedule | ~37,199 | 18,810 | 215 |
| BenchmarkClaimDueReportSchedule | ~42,256 | 18,807 | 215 |
| BenchmarkClaimDueReportSchedule | ~36,908 | 18,806 | 215 |

### After numbers (`-benchmem -count=3`)
| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkClaimDueReportSchedule | ~37,075 | 18,808 | 215 |
| BenchmarkClaimDueReportSchedule | ~37,451 | 18,805 | 215 |
| BenchmarkClaimDueReportSchedule | ~52,157 | 18,806 | 216 |

Delta: essentially identical — the claim is still a single UPDATE; no regression.

### Commands run
- `go test ./internal/database/ -run TestClaimDueReportScheduleIsAtomic -race -count=1 -v` ✓
- `go test ./internal/database/ -race -count=1` ✓
- `go test ./internal/services/ -race -count=1` ✓
- `go build ./...` ✓

## Phase 3 — scheduler & worker durability (2026-06-20)

16 fixes wiring the P0 `schedclaim` (atomic claim-before-side-effect) and `safego` (Sentry panic
reporting) primitives into the schedulers/workers, plus assorted concurrency/durability fixes.
**4 new migrations: 000092–000095** (deploy backend-first). Race-gated across all touched packages.

### Fixes
- **P3.1** ExchangeRateService.lastFetched → `atomic.Int64` (race fix). **P3.2** fiscal-worker error-branch
  `CaptureException` (panic reporter already wired in P0).
- **P3.3/P3.4** GuestFeedback + Report schedulers claim-before-send (existing columns; report claim also
  atomically advances `next_send_at` so recurring schedules re-arm).
- **P3.5** DirectorDigest claim (`businesses.director_digest_last_sent_at`, **mig 000092**) — removed
  in-memory map dedup. **P3.6** Lifecycle campaigns per-column claim (`getting_started_/founder_feedback_email_sent_at`, **mig 000093**).
  **P3.7** InventoryAlert insert-claim dedup (`inventory_alert_logs` generated `alert_day` + unique index, **mig 000094**, Postgres-only generated col — tests are PG-container-backed).
  **P3.8** Fiscal undelivered-receipt sweep lease-claim (`fiscal_receipts.delivery_locked_at/_by`, **mig 000095**, 5-min stale reclaim so a crashed worker never strands a receipt).
- **P3.9** Fiscal: expired-credential short-circuit (failed-permanent, no SOAP call) + permanent classification
  for decrypt/empty-bundle/unsupported-provider (missing-creds stays retryable).
- **P3.10** Stablecoin-plugin reconciliation armed on env-OR-`AnyBusinessHasPluginEnabled` (closed the stuck-intent drift; plugin and scheduler since removed).
- **P3.11** WSAA token single-flight (`golang.org/x/sync/singleflight`, already vendored) — mutex no longer held
  across the ~20s SOAP login; 20 concurrent callers → 1 login.
- **P3.12** `UpdateBusinessColumns` narrow-map helper (lost-update already structurally prevented — subscription
  writes are partitioned via `UpdateBusinessSubscriptionData`).
- **P3.13 (+cleanup)** Stop()→Start() restart-safety: re-create `stopChan` under mutex on Start for 7 schedulers
  (the 5 named + MilestoneScheduler + SubscriptionChecker); removed dead `UpdateReportScheduleLastSent`.
- **P3.14** Webhook replay dedup: stripe/paypal/mercadopago require an extractable event id (missing → 400
  post-signature, never processed). **P3.15** print orphan-sweep initial run + documented `PRINT_RETRY_WORKER_ENABLED` (default off).

### Commands run
- `go build ./...` ✓
- Race sweep: `go test ./internal/services/... ./internal/database ./internal/fiscal/... ./internal/handlers ./internal/logger -race -count=1` ✓ (PG-container tests ran, not skipped)
- Migration sanity: 000091–000095 sequential, each with up+down.

## P4.3 AICostGate waiter path addition (2026-06-20)

P4.3 adds `AICostGate.OverBudget` (one mutexed map lookup, O(1), no allocation) to
the AI-waiter budget check alongside the existing message-count guard. No DB
round-trip added; no measurable latency delta expected on the hot path.

## Phase P4 — AI safety & cost (2026-06-21)

Goal: bound AI spend per business and bring the WhatsApp AI path to parity with the HTTP waiter's guardrail/safety pipeline, then enforce the dark/decommissioned state server-side. No new perf benchmarks (safety/cost, not a latency path); gate is correctness + `-race`.

Slices (each its own commit):
- **P4-A cost ceiling** — `4c9c0f9c` RunningTotalUSD reader on DailyCostRollup; `cc1ac3b0` `llm.AICostGate` (`NewAICostGate(rollup, capUSD)`, `OverBudget(businessID)` nil-safe); `ffee3e5c` HTTP waiter blocks with 429 `over_budget` when `AI_DAILY_BUDGET_USD`>0 and the business is over. Default cap 0 = disabled.
- **P4-B WhatsApp safety parity (H1)** — `7a8dda6b` `screenWhatsAppTurn` (shared guardrails.InputClassifier, 2s ceiling) + per-business daily message budget (`AI_WAITER_DAILY_MESSAGE_BUDGET`, default 2000) AND USD cost gate + PII redact before SaveAiWaiterMessage + allergen post-check + 20s FeatureTimeout replacing context.Background; `b711dbda` main wires `WithClassifier(aiInputClassifier).WithCostGate(aiCostGate)`. Over-budget = silent drop.
- **P4.7 guardrail fail-closed in prod** — `90afca51` `resolveGuardrailStrict(env, production)` in `cmd/app/guardrail_strict.go` (explicit GUARDRAIL_STRICT overrides, else defaults to productionMode); the shared GeminiClassifier's `strict` flag makes Classify return `{Allowed:false}` on failure, so both the HTTP waiter and WhatsApp screen branches block on classifier failure in prod while staying fail-open in dev. `a11d6fd7` corrected the now-stale "fail-open C2" comments on both paths.
- **De-flake** — `d890d675` gate the menu auto-translate `SafeGo` goroutines on `GetTranslationService() != nil` so they never read the package-global DB handle when translation is unconfigured (deterministic fix for the `TestCreateOffer_InvalidatesPricingCache` -race flake; replaced a fragile 1ms time.Sleep drain `bcb88d2b`). Also a prod micro-opt (skips a wasted goroutine + DB walk).
- **P4.8 WHATSAPP_ENABLED gate** — `7cfd9e83` construct manager / restore sessions / mount the 3 `/whatsapp/*` routes only when `WHATSAPP_ENABLED=true` (dark by default), matching the FE hide; `whatsAppManager` is nil when disabled and the shutdown + every server-side use is nil-guarded. Folds in the P4.6 wiring inside the enabled branch.

Phase gate (all green, 2026-06-21):
```
go build ./...                                                              # clean
go test ./cmd/app/... ./internal/llm/... ./internal/services/... -race -count=1   # ok (all pkgs)
go test ./internal/server/ -race -count=1                                   # ok 17s
```
Note: `./internal/server` + `./internal/services` are NOT reentrant under `-count>1` (shared global test DB via SetTestDB — pre-existing artifact); gate uses `-count=1`.

## Phase P5 — Outbound resilience & timeouts (2026-06-21)

Goal: no outbound dependency (Telegram, S3, PSPs, PostHog, DB ping) can wedge a request goroutine or the process forever; failures fail fast and degrade. Executed as 6 fully file-disjoint tasks in parallel isolated worktrees (P5.2–P5.7), cherry-picked back, then serial P5.1 (telegram + main.go). All `-race` green.

Slices:
- **P5.1 EXT-1 telegram** `375ca717` — shared `*tgbotapi.BotAPI` now built via `NewBotAPIWithClient(..., telegram.BoundedTelegramClient())` (15s); both the operator dispatcher and plugin worker inherit the ceiling (was http.DefaultClient, Timeout 0).
- **P5.2 EXT-2 s3** `1ad932cb` (+ polish `bb60463f`) — `boundedS3HTTPClient()` wired into InitS3 + InitS3Protected; 30s overall + bounded transport (dial 5s / TLS 5s / response-header 15s / idle 90s) for the third-party iDrive e2 endpoint.
- **P5.3 EXT-3 psp/translation ctx** `64b15463` — threaded `context.Context` (`http.NewRequestWithContext`) through stripe/paypal/mercadopago plugins + subscription_stripe + translation outbound helpers. Pure plumbing; money/refund semantics unchanged. (Background() at the call sites because the IntegrationPlugin interface carries no ctx — threading from handlers is a future interface-level task.)
- **P5.4 EXT-4 readiness** `8a2d5f6d` — readiness DB probe uses `PingContext` with a 2s deadline (fails fast on a hung/saturated pool); hermetic `slowConnector` test proves the deadline deterministically.
- **P5.5 EXT-5 circuit breaker** `9ea62882` (+ polish `bb60463f`) — new generic `internal/circuitbreaker` (opens after 5 consecutive transport failures, 30s window, half-open recovery) wrapped around the OpenRouter `http.Do`. Per-Provider, counts only transport failures (4xx/5xx handled by the retry layer).
- **P5.6 EXT-6 health rate-limit** `3851e112` — `/api/v1/health*` exempt from the global IP limiter (probes never throttled); non-health paths still limited.
- **P5.7 EXT-7 posthog** `01f46cf8` — PostHog init degrades (warn + disable) instead of `log.Fatal`; all capture/close paths nil-guarded so a disabled client is a quiet no-op.

Close-out gate (all green, 2026-06-21):
```
go build ./...   # clean
go test ./internal/s3/... ./internal/plugins/... ./internal/services/... ./internal/health/... \
  ./internal/circuitbreaker/... ./internal/llm/... ./internal/middleware/... ./internal/metrics/... \
  ./internal/telegram/... ./cmd/app/... -race -count=1   # ok (all packages)
```

## Phase P6 — Performance & caching (2026-06-21)

Goal: cut hot-path DB/CPU cost (guest menu/pricing, AI budget gate, bill detail, admin lists) and bound unbounded caches/queues, without changing correctness. Executed as an 8-task parallel wave in isolated worktrees (P6.2–P6.7,P6.9–P6.11), then serial P6.1/P6.8 (main.go) + P6.7 (guest files). All `-race` green; deploy backend-first (migration 000096).

Slices:
- **P6.1 fiscal Provider cache** — cache the credential-aware fiscal Provider per (settingsID, SHA-256(cert DER) fingerprint); rotation changes the fingerprint → fresh build, empty-fingerprint rows bypass; the cached Provider's WSAA client self-refreshes (never serves an expired token). Concurrency-safe; both fiscalService + worker share one cache.
- **P6.2 geo-IP dedup** — coalesce concurrent identical geo lookups + cap in-flight at 12 (atomic.Pointer[sync.Map] + semaphore); outbound via httpclientx with a 3s ceiling.
- **P6.3 webhook_events janitor + index** — `DeleteProcessedWebhookEventsBefore` (status='processed' only, batched) + `StartWebhookEventRetentionJanitor` (ctx-cancelled on shutdown, `WEBHOOK_EVENT_RETENTION_DAYS` default 60, 0=disabled) + **migration 000096** `(status, received_at)` index (idempotent).
- **P6.4 bill Business projection** — `GetBillByID`/`GetBillByNumber` project the preloaded Business to consumed columns only (incl. logo/timezone for the bill JSON serializer); ~35% fewer allocs on the bill-detail path. Access-shape test asserts no SELECT *.
- **P6.5 paginate stripe payments** — `GetStripeSubscriptionPaymentsPaged` (default 50 / max 200, `created_at DESC, id DESC`); admin handler reads limit/offset, response envelope unchanged.
- **P6.6 guest ETag + Cache-Control** — weak ETag (version+locale+updatedAt) → 304 on If-None-Match for guest menu (`max-age=5`) and live status (`no-cache`); collapses repeat polls.
- **P6.7 drop duplicated category array** — translated menu responses now emit a single translated `categories` (drops the redundant untranslated `parsed_categories`); FE fallback `parsed_categories || categories` verified across 5 read sites.
- **P6.8 verifiedTokenCache sweeper** — periodic expiry sweep (1-min) bounding the JWT verified-token cache; stop/done channels, shutdown-stopped, expired-only eviction.
- **P6.9 bound wizard history** — `GetRecentWizardMessages` (newest 20, returned chronological) replaces unbounded `GetWizardMessages` at both menu-AI call sites.
- **P6.10 AI-waiter daily counter** — per-(business, UTC-day) in-memory counter (seeded once from DB, incremented per user turn) replaces a per-turn JOIN+COUNT; both HTTP + WhatsApp budget gates switched.
- **P6.11 guest caches + single-flight + async exchange-rate** — pricing/customURL/table caches on boundedcache (5000-cap LRU+TTL) + singleflight (with in-flight double-check) coalescing bursts to one DB fetch + reap-on-write; staleness-triggered exchange-rate refresh runs async (callers keep last-good rate). Added `boundedcache.SetAt`/`EvictExpired`.

Close-out gate (all green, 2026-06-21):
```
go build ./...   # clean
go test ./internal/handlers/... ./internal/database/... ./internal/services/... ./internal/server/... \
  ./internal/fiscal/... ./internal/boundedcache/... ./cmd/app/... -race -count=1   # ok (all packages)
```

## Phase P7 — Migration & data-integrity infra (2026-06-21)

Goal: make a genesis/baseline-skipped DB converge to the same money-integrity schema as a fully-migrated one, stop AutoMigrate from flapping fiscal jsonb columns, and make migration failures recoverable. P7 adds NO new versioned migration (uses idempotent ensure-helpers); 000097 stays free. Executed as 4 file-disjoint TDD slices in parallel worktrees, then serial main.go wiring + a Docker-validated integration test.

Slices:
- **P7.1–P7.3 / P7.4 money invariants (MIG-1)** — `RunEnsureMoneyInvariants` recreates, idempotently (`CREATE UNIQUE INDEX IF NOT EXISTS` + a pg_constraint-guarded CHECK), the partial-unique active-bill index (`idx_bills_active_per_table`, predicate IDENTICAL to migration 000059) and the settled-payment unique index (`idx_bill_split_shares_payment_settled`, IDENTICAL to 000084), plus the `amount_cents > 0` CHECK. Wired in main.go right after `RunEnsureBillItemsSchema`, before versioned `RunMigrations`, non-fatal. This heals a baseline-skipped DB that golang-migrate force-baselines past (the same class of bug as the 2026-06-05 order_id silent-skip).
- **P7.5–P7.6 fiscal jsonb tags (MIG-2)** — added `;type:jsonb` (preserving `serializer:json`) to the 4 fiscal jsonb fields (BusinessFiscalSettings.ProviderConfig, FiscalReceipt.RawRequest/RawResponse, FiscalAuditEvent.Metadata) so AutoMigrate can't flap them to text; reflection test guards it.
- **P7.7–P7.8 dirty-state recovery (MIG-3)** — `dirtyMigrationError` names the dirty version and points operators to a real recovery runbook (`docs/runbooks/migration-dirty-recovery.md`); both dirty-state returns in migrate.go use it.
- **P7.9–P7.10 down-migration policy (MIG-4)** — a test requires every `.down.sql` to be real revert DDL or carry an explicit `-- IRREVERSIBLE` marker; 31 placeholder downs annotated (files with real DDL, incl 000096/000028, untouched).
- **P7.11 integration test (MIG-1 deliverable)** — `//go:build integration_postgres` Testcontainers test runs the real startup chain on a fresh postgres:15 and asserts all three invariants exist via pg catalogs. **Ran under Docker and PASSED**, reproducing the baseline-skip log and confirming the heal.

Close-out gate (all green, 2026-06-21): `go build ./...` clean; `go test ./internal/database/... ./cmd/app/... -race -count=1` ok; `go test -tags integration_postgres ./internal/database/ -run TestFreshDB_HasMoneyInvariants` PASS (Docker).

## P8 perf-gate (benchmark & perf-gate infra)

CI/test-tooling phase only — **no production query/queue/handler path was
changed**, so no before/after production delta applies. The deliverables are
(a) the new benchmark baselines, (b) a coverage guard so bench-ci can't silently
drop a tree, and (c) a real statistical perf gate replacing a fragile grep.

### P8.1 — bench-ci coverage guard (`861c7e60`)
`internal/perfgate/benchci_coverage_test.go` fails CI when any directory holding
a `*_perf_test.go` / `func Benchmark` is outside the `bench-ci` tree set. It
caught that bench-ci ran only 4 of 7 benchmark-bearing trees — ~60 hot-path
benches in `internal/services`, `internal/crm`, `internal/splitting` never ran
in CI. Added those three trees to the `Makefile` `bench-ci`/`bench` recipes.
Gate: `go test ./internal/perfgate/ -count=1` → ok.

### P8.2 / P8.3 — Postgres-backed read benches (Docker-gated)
- `BenchmarkGetOrdersByBusinessIDPaginatedParallelPostgres` (`88957437`) —
  concurrent throughput over the production 25-conn pool.
- `BenchmarkGetTablesWithStatusPostgres` (`d4ac9718`) — table-status dashboard poll.
Both use Testcontainers/Postgres and **skip under `-short`** (verified: clean
`ok` with no bench run when Docker absent). Baselines are captured on a
Docker-healthy host / nightly CI, not on this machine.

### P8.4 — PSP webhook ingest access-shape + bench (`6381f438`)
- `TestWebhookIngestQueryShape` locks `businesses SELECT * == 0` on the webhook
  settle path, protecting the P6.4 Business projection from regressing back to
  `SELECT *`. Gate: ok.
- `BenchmarkSettlePluginWebhook` first baseline (local SQLite, end-to-end
  handler, `-count=3` on Apple M3):
  ```
  BenchmarkSettlePluginWebhook-8   6790130 ns/op   17283079 B/op   42540 allocs/op
  BenchmarkSettlePluginWebhook-8   6286469 ns/op   16393912 B/op   40947 allocs/op
  BenchmarkSettlePluginWebhook-8   6114589 ns/op   17282618 B/op   42538 allocs/op
  ```
  (~6.1–6.8 ms/op; high B/op·allocs/op reflect the full per-op handler+DB setup,
  this is an access-shape guard not a micro-tuned path.)

### Newly-covered local bench baselines (P8.1 widened bench-ci)
Captured via the bounded `-short` bench-ci sanity run on Apple M3 (the trees that
previously never ran in CI), piped through `perf/bench/scripts/strip-log-noise.awk`:
```
BenchmarkPrintEnqueueReceipt-8                   299275 ns/op    172266 B/op    1924 allocs/op
BenchmarkGetBusinessCustomerSummary-8            122265 ns/op      8294 B/op      81 allocs/op
BenchmarkListBusinessCustomersWithSummary-8      544612 ns/op    194520 B/op    1831 allocs/op
BenchmarkGetCustomerBusinessConnectionsSQLite-8 2357991 ns/op   3815317 B/op   16732 allocs/op
BenchmarkGetSegments-8                           213635 ns/op     55918 B/op     728 allocs/op
BenchmarkPutLoyaltyRecompute-8                   411114 ns/op    229827 B/op    3945 allocs/op
BenchmarkCalculateEqualSplitSQLite-8              10703 ns/op     11024 B/op     118 allocs/op
BenchmarkCalculateItemSplitSQLite-8               30418 ns/op     22160 B/op     269 allocs/op
```

### P8.5 — benchstat-aware perf gate (`568ac253`)
Replaced the advisory `grep -E '\+1[5-9]\.|\+[2-9][0-9]\.' delta.txt` (time-only,
no significance check, brittle regex) with `perf/bench/scripts/gate.sh`:
- Parses `benchstat -format csv` output (real 7-column layout: name, old, CI,
  new, CI, `vs base`, `P`).
- Fails when ANY metric (sec/op **and** B/op **and** allocs/op) regresses
  `>= THRESHOLD_PCT` (default 15, env-tunable) **with** statistical significance
  `p < 0.05`. Skips `~` (no-change) and geomean/header rows; fail-closed if a
  p-value can't be parsed.
- `gate_test.sh` is the local regression guard — 4 fixtures: `clean.csv` (pass),
  `regress.csv` (time/B/op/allocs all regress → fail), `bop_regress.csv` (time
  `~` but B/op +30% significant → fail, proves not time-only), `nonsig.csv`
  (+40% delta but p≥0.05 → pass, proves significance required).
- Wired into the perf-bench workflow (since retired): Compare step now also emits
  `delta.csv`; Hard gate calls `gate.sh delta.csv` (skipped when no baseline).

### Commands run
```
go test ./internal/perfgate/ -count=1
go test ./internal/perfgate/... ./internal/database -run '^$' -count=1 && go vet ./internal/perfgate/...
bash perf/bench/scripts/gate_test.sh
go test -run=^$ -bench=. -benchmem -count=1 -short -timeout 7m ./internal/services/... ./internal/crm/... ./internal/splitting/... | awk -f perf/bench/scripts/strip-log-noise.awk
go test ./internal/handlers/ -run TestWebhookIngestQueryShape -count=1
go test ./internal/handlers/ -run=^$ -bench=BenchmarkSettlePluginWebhook -benchmem -count=3
```
All green. Migration numbers used: none (P8 is CI/bench tooling only).

## P9 hygiene & dead code

Pure hygiene — no DB migration, no query/queue/hot path touched (metric
`.Inc()` and request-time validation only), so no benchmark deltas apply.

### P9.1 — orphaned-CAE loud failure (`fd81c058`)
The worst fiscal failure (AFIP committed a CAE — a legal invoice number was
consumed — but the local `FiscalReceipt` row couldn't be persisted after the
bounded retry, so the job goes `failed_permanent` WITHOUT re-issuing) was a bare
stdlib `log.Printf` to stdout. Replaced with `logger.Logger.WithFields(...).Errorf`
structured error + a dedicated counter `payverge_fiscal_orphaned_cae_total`
(`prometheus.MustRegister`ed). Each increment is a page-the-operator,
reconcile-by-hand event. Terminal behavior unchanged. Tests:
`TestFiscalOrphanedCAETotalRegistered` (metric identity) +
`TestPersistReceiptResult_OrphanedCAE_IncrementsMetricAndGoesTerminal` (drives
`persistReceiptResult` with a forced-save-failure repo, asserts terminal status
+ exactly-one increment).

### P9.2 — untrack ~108MB build binaries (`6db5a027`)
`backend/payverge` (47MB) + `backend/payverge-backend` (56MB) were tracked at the
module root; a natural `go build -o payverge` re-stages a fresh blob. `git rm
--cached`ed both (kept on disk) + added `backend/.gitignore` (`/payverge`,
`/payverge-backend`, `/bin/`). Guard test `internal/hygiene/TestNoCommittedRootBinaries`
fails if either is ever re-tracked. History scrub (git-filter-repo to reclaim the
108MB) is deliberately left out-of-band.

### P9.3 — reject half-built cloudprnt transport (`e46ccb9c`)
`CreatePrinter` accepted `transport=cloudprnt` (token generated, validation
passed) but no CloudPRNT poll endpoint exists and `FormatterOutput.ESCPOS` is
never populated, so a cloudprnt printer black-holes every job at `status=routed`
forever. Allowlisted browser-only with a clear `"not yet available"` 400; the
unreachable token-gen call AND the now-unused `generateCloudPRNTToken` helper
(plus its `crypto/rand`/`encoding/base64` imports) were removed. Test
`TestPrinterHandlers_RejectsCloudPRNTUntilSprint2` (400, no row created);
existing transport-rejection + create/list tests still green.

### P9.4 — purge dead car/fleet/NFT/reward metrics (`e7a51386`)
Deleted 17 always-zero boilerplate metric vars inherited from a prior car-rental
template (`CarMetrics`, `FleetInvestmentAmount`, `FleetRentalDuration`,
`FinancialMetrics`, `UserMetrics`, `FleetOperations`, `CarOperations`,
`KYCOperations`, `NFTOperations`, `NFTResponseTime`, `RewardOperations`,
`RewardAmounts`, `RewardResponseTime`, `FleetMetrics`, `CarOperationDuration`,
`UserReferrals`, `ActiveUsers`) + their registrations — each verified 0 external
callers before deletion (`go build ./...` is the backstop). They exported
zero-value series that mislead restaurant/payment dashboards. Added the real
signal in their place: `payverge_payment_webhook_processing_failures_total{plugin,reason}`.
Kept the in-use `ActiveSubscribers`/`AuthOperations`/`SubscriptionOperations`/
`SubscriptionResponseTime`/`SubscribersByContact`/`UserOperations`/`UserResponseTime`.
Guard test `TestNoDeadCarFleetMetricsRegistered`.

### Integration de-flake (`585c1c2e`)
Surfaced during the P9 exit gate: `TestScheduleGeoLookupDeduplicatesInFlight`
(P6.2's test) read `calls` immediately after `wg.Wait()`, which only joins the 50
schedule calls — NOT the fresh goroutine `scheduleGeoLookup` launches for the
deduped lookup. The read raced the launch and observed 0 ~60% of the time
(verified flaky at the pre-P9 baseline e98731bf — NOT a P9 regression; the geo
code references no metrics and is byte-identical baseline↔exec). Fixed test-only:
the lookup stub signals an `entered` channel after bumping `calls`; the test waits
on it (2s timeout) so "the deduped lookup ran" is a happens-before. 30/30
deterministic, clean under `-race -count=3`.

### Exit gate
```
go test ./internal/fiscal ./internal/metrics ./internal/handlers ./internal/hygiene -count=1   # all ok
go build ./...                                                                                  # clean
```
Migration numbers used: none.

## Final whole-implementation review remediation

A 6-lens adversarial review (lifecycle, migrations, money, security, primitives,
leaks) over the integrated 100-commit diff vs main found 3 confirmed issues
(money / lifecycle / leak lenses came back clean). All fixed + regression-tested
(each test verified to FAIL on the pre-fix code):

- **CRITICAL — auth/PIN lockout fail-OPEN on DB error** (`255003aa`). Login
  (`internal/auth/handlers.go`) and manager-PIN (`internal/server/manager_pin.go`)
  gated on `IsLocked(...); lerr == nil && locked`, so a non-nil error (the
  auth_attempts table from mig 000091 missing, or DB erroring) fell THROUGH to
  unthrottled password/PIN verification — an attacker degrading just
  auth_attempts (users table fine) could brute-force unprotected. Now fails
  CLOSED (429) on an IsLocked error, logged + metered
  (`AuthOperations{login_throttle_unavailable}`); Record()/Clear() errors are
  logged instead of discarded. Tests pass the CORRECT credential and assert 429;
  pre-fix they returned 200 with a valid JWT / allowed the privileged action.
- **MEDIUM — circuit-breaker half-open probe race** (`36f0da38`). Two concurrent
  half-open probes: a success could clear `halfOpen` before a sibling failure
  recorded, so the failure saw `halfOpen=false`+`failures<threshold` and left the
  breaker closed, violating "first non-nil result re-opens". Capture a per-call
  `probing` flag at entry and decide re-open off it, not the shared field; a
  success only closes when it was itself a probe. Deterministic concurrent
  regression test (fails pre-fix).
- (The HIGH silent-Record finding folded into the CRITICAL fix above.)

Post-fix gate: `go build ./...` clean, `go vet ./...` clean, full suite
`go test ./... -count=1` = 64 ok / 0 FAIL; earlier full `-race` suite = 64 ok /
0 races. Plus an integration de-flake of P6.2's `TestScheduleGeoLookupDeduplicatesInFlight`
(pre-existing race, see P9 section).

---

## Low-severity parity fixes (state-of-app review follow-up, 2026-06-22)

Three LOW findings from the state-of-app adversarial risk scan, fixed TDD
(each test verified to FAIL pre-fix; 3 fixes
adversarially re-reviewed = all CORRECT, 0 blocking):

- **LOW-1 `09ab193c` exchange-rate per-tick panic isolation.** `StartPeriodicFetch`
  wrapped the whole loop in one `SafeGo` but ran `for range ticker.C { FetchLatestRates() }`
  with no per-tick recovery — a single panic permanently froze rate refresh until
  restart. Extracted `runRateFetchTick()` wrapping each pass (incl. on-boot) in
  `logger.SafeTick`, matching every sibling scheduler. `internal/services/exchange_rate.go`.
- **LOW-2 `d2076346` lifecycle inactivity/winback restart-safe dedup.** Both
  campaigns deduped only via the sliding window; an on-boot re-check after a same-day
  restart re-sent. Added recurring-claim columns (mig **000097**, additive/idempotent,
  real down) stamped with a `NULL OR < episodeStart` atomic UPDATE (mirrors
  `DirectorDigestLastSentAt`) — restart-safe yet re-triggers on a genuine future lapse.
  `internal/services/lifecycle_scheduler.go`, `internal/database/models.go`. **Deploy backend-first.**
- **LOW-3 `6913e76f` Origin-allowlist guard parity.** Added
  `RequireTrustedOriginForMutations` to the 3 cookie-auth mutations missing it
  (guest loyalty redeem-points/undo-redemption per-route; staff /logout group).
  Defense-in-depth (SameSite=Lax still holds); customer/* siblings prove the
  frontend origin is allowlisted, so no guest breakage. Wiring in `main()`
  (not unit-testable) verified by build+grep; behavior covered by existing
  `security_test.go`. `cmd/app/main.go`.

Gate: `go build ./...` clean, `go vet` clean, `go test -race -count=1
./internal/services ./internal/database ./internal/middleware` = 3 ok / 0 races.

---

## Staff Management Slice 3 — Availability & time-off (backend, 2026-06-30)

BE-first (4 path-scoped commits). New `internal/database/availability_service.go`
(`StaffAvailability`, `TimeOffRequest` + status consts + service) + migration
**000105_availability_timeoff** + handlers `internal/handlers/availability.go`.
Genesis-safe: struct-tag↔DDL index parity (`idx_staff_availabilities_biz_staff_weekday`,
`idx_time_off_requests_biz_status`, `idx_time_off_requests_biz_staff`) asserted via
`HasIndex` in `TestAvailabilityTimeOffGenesisShape`; both models registered in
`db_config.go` autoMigrate. No money fields.

- Availability: `ReplaceAvailability` is delete-then-insert in a txn, scoped to the
  caller's OWN staff_id (body staff_id ignored); reject-before-persist validation
  (weekday 0..6, 0 ≤ start < end ≤ 1440, kind∈{preferred,unavailable}).
- Time-off state machine: `DecideTimeOff` uses a CAS guard (`WHERE status='pending'`,
  RowsAffected!=1 → `ErrTimeOffNotPending`) inside a txn that also writes a
  best-effort `RBACAuditLog` (`RBACActionTimeOffDecided`). Cross-tenant/unknown →
  `ErrTimeOffNotFound`.
- SSE: `timeoff.requested` (perm `schedule:approve`), `timeoff.decided` (perm
  `schedule:self`) registered in `sse_permissions.go`.

**Perf gate (list endpoint).** `ListTimeOff(businessID, status, scopeStaffID *uint)`
row-scopes in SQL (manager queue = all by status via `(business_id,status)`; staff =
own via `(business_id,staff_id)`), explicit column projection (no `SELECT *`), single
bounded query (`LIMIT 500`). Access-shape regression: `TestListTimeOffAccessShape`
(asserts 1 SELECT, no `SELECT *`, `staff_id` scope in SQL, `LIMIT`). Baseline
benchmark `BenchmarkListTimeOff` (200 pending rows, SQLite, `-benchmem`):
**~702,579 ns/op, 242,081 B/op, 4,820 allocs/op** (single establishing baseline —
implementation written narrow from the start, no before/after delta). Command:
`go test ./internal/database -run '^$' -bench 'BenchmarkListTimeOff' -benchmem`.

Gate: `go build ./...` clean, `go vet ./internal/...` clean,
`go test ./internal/handlers ./internal/database ./internal/services -run
'Availability|TimeOff|Timeoff' -count=1` = 3 ok / 0 FAIL, gofmt clean.

---

## Staff Management Slice 4 — Time clock + labor actuals seam (backend, 2026-06-30)

BE-first (7 path-scoped commits). New `internal/database/timeentry_service.go` (`TimeEntry` +
source/status consts + clock service + `GetApprovedWorkedMinutes` aggregate) +
migration **000106_time_entries** + handlers `internal/handlers/timeclock.go` +
labor `AnalyzeActual` seam + `accounting.go` `?basis` switch. Genesis-safe:
struct-tag↔DDL index parity (`idx_time_entries_biz_staff_clockin`,
`idx_time_entries_biz_status`) asserted via `HasIndex` in
`TestTimeEntryGenesisShape`; `&TimeEntry{}` registered in `db_config.go`
autoMigrate. The `TimeEntry` row carries NO money column (pay rate stays owner-only
on `staff_positions`); staff-facing responses expose worked minutes/hours only.

- Clock service: `ClockIn` rejects a second open entry (`ErrAlreadyClockedIn`
  precondition); `ClockOut`/`AddBreak` operate on the open entry via CAS on
  `status='open'`; `ApproveTimeEntry` CAS `pending_review→approved` + best-effort
  `RBACAuditLog` (`RBACActionTimeEntryApproved`), illegal transition →
  `ErrTimeEntryNotPendingReview`; `CreateManualEntry` (source=manager_manual,
  status=pending_review) reject-before-persist. `WorkedMinutes(e)` is the single
  worked-minutes formula reused by handlers + mirrored by the SQL aggregate (DRY).
- RBAC: `timeclock:punch` (all four roles), `timeclock:manage` (manager only; owner
  bypasses) minted in `rbac.go`; `TestTimeclockPermissions_RoleDefaults`.
- Labor actuals seam (EXTEND, not fork): `labor.WorkedHoursProvider` +
  `Calculator.AnalyzeActual` (laborCost = Σ workedMin/60 × primary pay_rate;
  pct = laborCost/NetSales). Existing `Analyze`/`Report`/`Calculator` + the
  accounting call site untouched (additive: new fields are `omitempty`, new
  `WithWorkedHours` chain). `accounting.go` `GetLaborCost` gains
  `?basis=actual|scheduled|payroll` (default payroll = byte-compatible) + variance
  vs payroll; aggregate/per-person/variance/net-sales $ are owner/`financial:read`-
  gated (`callerHasFinancialRead`), non-financial callers see hours + labor-% only.
  *Deviation:* `scheduled` basis + true scheduled-vs-actual variance defer to Slice 6
  (which owns the scheduled-labor calculator per the plan file map); Slice 4 returns
  400 for `?basis=scheduled` and the variance compares actual vs the payroll basis.

**Perf gate (list + aggregate endpoints).** `ListTimesheet`/`ListForReview` row-scope
in SQL over an explicit column projection (no `SELECT *`), single bounded query
(`LIMIT 500`) on `(business_id,staff_id,clock_in_at)` / `(business_id,status)`.
`GetApprovedWorkedMinutes` is ONE GROUP BY aggregate (dialect-aware minute
expression: SQLite `strftime` / Postgres `EXTRACT(EPOCH)`) joining the primary
position rate, not per-row hydration. Access-shape regressions:
`TestListForReviewAccessShape` + `TestGetApprovedWorkedMinutesAccessShape` (assert
1 SELECT, no `SELECT *`, GROUP BY/SUM in SQL, `LIMIT`). Baseline benchmark
`BenchmarkListForReview` (200 pending rows, SQLite, `-benchmem`):
**~741,758 ns/op, 268,976 B/op, 5,219 allocs/op** (single establishing baseline —
written narrow from the start, no before/after delta). Command:
`go test ./internal/database -run '^$' -bench 'BenchmarkListForReview' -benchmem`.

Gate: `go build ./...` clean, `go vet ./internal/...` clean, `go test
./internal/handlers ./internal/database ./internal/services ./internal/services/labor
./internal/server -run 'TimeEntry|Timeclock|TimeClock|Labor|Timesheet|Rbac|RBAC|Perm'
-count=1` = 5 ok / 0 FAIL, gofmt clean.

---

## Staff Management — Slice 5 (Coverage: open shifts & swaps) — backend

State-machine-heavy coverage layer: staff claim open shifts / request swaps (or
giveups), eligible coworkers accept, managers approve/deny — driven by an audited
state machine `open → accepted → pending_approval → approved|denied`
(`kind=giveup` skips accept). New `internal/database/coverage_service.go`
(`OpenShiftClaim` + `ShiftSwapRequest` + 5 service methods + 2 list endpoints),
migration `000107_coverage`, `internal/handlers/coverage.go` (6 handlers; the
`/swaps/:swapId/decision` route is polymorphic on a body `kind: "swap" |
"open_claim"` discriminator), coverage SSE events, route wiring.

**Genesis-safety.** Both models encode the 000107 DDL indexes in gorm tags with
matching composite names (`idx_open_claims_business_shift/_status`,
`idx_swaps_business_status/_shift`), are registered in `db_config.go`
autoMigrate, and carry explicit `TableName()`. `TestCoverageGenesisShape`
asserts `Migrator().HasIndex` for all four indexes + `HasTable` + every column.

**State machine.** Every transition is a status-precondition CAS
(`UPDATE … WHERE status=? → act iff RowsAffected==1`) inside a transaction that
also writes an `RBACAuditLog` row. Double-claim, double-accept, double-decision,
and double-fill all ride the precondition guard. Eligibility (the accepting/
claiming staff must hold the shift's `PositionID` via a `StaffPosition` row) is
checked inside the txn. Deviation from the plan: `DecideSwap` gates on
source-decidability (`status ∈ {accepted, pending_approval}`) rather than
`validSwapTransition(status, target)`, because the locked transition table
intentionally has no `accepted→approved` edge (accepted only advances to
pending_approval) yet an `accepted` swap is a valid manager decision source
(design decision #1). This keeps `TestValidSwapTransition` and all `DecideSwap`
tests green simultaneously.

**Perf gate (list endpoints).** `ListOpenCoverage`/`ListMyCoverage` are
narrow-projection joins (explicit `Select(...)`, no `SELECT *`, no per-row
reload/N+1), bounded `LIMIT 200`, tenant-scoped. Eligibility is a SQL JOIN onto
`staff_positions` (not per-shift reloads). The manager pending-approval queue is
two bounded reads. Access-shape regression:
`TestListOpenCoverageEligibilityScopedAndBounded` (eligibility-scoped open shifts
+ swap inbox, approver-only pending queue) + `TestListMyCoverageTenantScoped`.
Baseline benchmark `BenchmarkListOpenCoverage` (100 open shifts, SQLite,
`-benchmem -count=3`): **~182,800 ns/op, ~68,600 B/op, 1,290 allocs/op**
(single establishing baseline — written narrow from the start, no before/after
delta). Command:
`go test ./internal/database -run '^$' -bench BenchmarkListOpenCoverage -benchmem -count=3`.

---

## Slice 9 — Engagement (checklists, documents, recognition, polls) — BACKEND

**Schema (Task 9.1).** 10 tables in migration `000110_engagement` +
`engagement_service.go` models registered in `db_config.go` autoMigrate, each
with explicit `TableName()`. The three composite uniques —
`idx_run_item` (run_id, item_id), `idx_doc_ack` (document_id, staff_id, version),
`idx_poll_one_vote` (poll_id, staff_id) — are PLAIN (no WHERE), so AutoMigrate
materializes them from `uniqueIndex:` tags (no raw Exec, unlike the Slice-0/5/7
partial-uniques). `TestEngagementSchemaMaterializes` asserts `Migrator().HasTable`
for all 10 tables AND `HasIndex` for every secondary index (17 named indexes) +
the 3 composite uniques. **Tag-vs-DDL drift fixed:** the plan's bare `gorm:"index"`
on `assigned_staff_id`/`shift_id`/`to_staff_id`/`option_id` would auto-name the
index with a `_id` suffix and drift from the DDL's shortened names
(`idx_checklist_runs_assigned`, `idx_checklist_runs_shift`, `idx_shoutouts_to_staff`,
`idx_poll_votes_option`); the struct tags now use named indexes matching the DDL.
`note`/`content` TEXT columns carry `type:text;not null;default:''` to match the
DDL (plan left them bare). No money columns anywhere.

**Constraints proven.**
- Poll one-vote-dedupe is a DB constraint (`idx_poll_one_vote`) + `OnConflict
  DoNothing` + `RowsAffected==0 → ErrAlreadyVoted` — NOT count-then-insert.
  `TestPollDoubleVoteRejected` (service) + `TestPollVoteSecondVoteIs409` (handler).
- Anonymous poll results: the COUNT(*) GROUP BY aggregate never selects staff_id;
  voter ids attach only when `!IsAnonymous`. `TestAnonymousPollResultsHideVoter`
  asserts `Voters==nil` AND marshaled JSON has no "voter";
  `TestPollAnonymousResultsJSONHasNoVoter` asserts the same over HTTP.
- Versioned doc ack: ack records the doc's CURRENT version; a version bump opens a
  fresh ack row, re-ack of the same version is a no-op. `TestDocumentAckRecordsVersion`.
- Own-run checklist scope: non-managers may only tick a run assigned to them.
  `TestChecklistTickItemOnlyOnOwnRun` (cross-staff → 403),
  `TestChecklistListRunsOwnScopeOnly` (own runs only).

**Perf gate (4 list endpoints).** All four are narrow-projection (explicit
`Select(...)`, no `SELECT *`), bounded (`LIMIT`), tenant-scoped, no N+1 (poll List
batch-loads options + the caller's own votes in 2 extra bounded queries via
`IN (?)`). Access-shape regressions: `TestListChecklistRunsAccessShape`,
`TestListDocumentsAccessShape`, `TestListShoutoutsAccessShape`,
`TestListPollsAccessShape` (each asserts no `SELECT *` + `LIMIT` present).
Baseline benchmarks (50 rows, SQLite, `-benchmem -count=3`, Apple M3):

| Benchmark | ns/op (median) | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkListChecklistRunsForStaff` | ~156,000 | 53,213 | 1,003 |
| `BenchmarkListDocuments` | ~166,000 | 57,444 | 1,309 |
| `BenchmarkListShoutouts` | ~115,000 | 40,920 | 895 |
| `BenchmarkListPolls` | ~140,000 | 51,817 | 1,148 |

Single establishing baselines (written narrow from the start, no before/after
delta). Command:
`go test ./internal/database -run '^$' -bench 'BenchmarkListChecklistRunsForStaff|BenchmarkListDocuments|BenchmarkListShoutouts|BenchmarkListPolls' -benchmem -count=3`.

**RBAC (Task 9.2).** 7 perms minted: `checklist:complete`/`doc:read`/
`recognition:send`/`poll:vote` (all staff roles) and `checklist:manage`/
`doc:manage`/`poll:manage` (manager+owner). `TestEngagementPermDefaults` proves
line roles never hold the manage perms.

**Route-perm deviation (Task 9.8).** `GET /shoutouts` is gated on `recognition:send`
(not `chat:read`) to keep Slice 9 independently shippable with no Slice-7
dependency — both are all-staff perms, identical audience.

**Additive endpoint (Task 9.10).** Added `GET /polls/:pollId/results` (poll:vote)
as the "GET detail" the plan references for fetching a poll's tally; the Vote
handler also returns results inline.

## Slice 9 (Engagement) — review-driven fixes (2026-06-30)

### IMP-1 — audience_filter enforced on documents & polls (privacy)

`ListDocuments`/`ListPolls` now take `(businessID, staffID, role, isManager, limit)`.
Managers/owners see all (no audience predicate, to manage); line staff get one
extra bounded predicate `AND audience_filter IN (?)` where the IN set is `["all"]`
+ `role:<role>` + `dept:<d>` for each department the caller holds an active
position in (resolved by the shared `callerAudiences` helper — one bounded
distinct-department query mirroring `ListAnnouncements`, no N+1). `CreateDocument`
/`CreatePoll` now validate the audience via `normalizeAudienceFilter` (empty →
"all", malformed → `ErrInvalidAudienceFilter` → 400).

Access-shape tests assert the IN-filtered read keeps the narrow projection +
LIMIT and adds `audience_filter IN`: `TestListDocumentsAudienceFilterAccessShape`,
`TestListPollsAudienceFilterAccessShape` (plus the manager-path
`TestListDocumentsAccessShape`/`TestListPollsAccessShape`).

Benchmarks (50 rows, SQLite, `-benchmem -count=3`, Apple M-series). The audience
path adds the dept-resolution query + IN predicate; the manager path is unchanged
from the Slice 9 baseline:

| Benchmark | ns/op (median) | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkListDocuments` (manager, unchanged) | ~175,700 | 57,444 | 1,309 |
| `BenchmarkListDocumentsAudience` (line staff, IN-filtered) | ~185,400 | 65,287 | 1,405 |
| `BenchmarkListPolls` (manager, unchanged) | ~144,800 | 51,820 | 1,148 |
| `BenchmarkListPollsAudience` (line staff, IN-filtered) | ~164,800 | 59,974 | 1,246 |

Command:
`go test ./internal/database -run '^$' -bench 'BenchmarkListDocumentsAudience|BenchmarkListPollsAudience|BenchmarkListDocuments$|BenchmarkListPolls$' -benchmem -count=3`.
The audience path costs ~+10–20k ns/op and ~+96–98 allocs/op (one extra
dept-resolution query) — bounded and acceptable for a per-caller list read.

### IMP-3 — versioned-document ack functional end-to-end

New `UpdateDocument(businessID, documentID, title, url, content, requireAck,
audienceFilter)` loads the active doc tenant-scoped (else `ErrDocumentNotFound`),
validates audience, increments `version`, and replaces the body fields —
re-opening acknowledgement since `DocumentAck` is keyed by version. Wired as
`PUT /businesses/:id/documents/:docId` (doc:manage). New `DocumentsAckedBy(
businessID, staffID, docs)` is ONE bounded `IN (?)` read over `document_acks`
(narrow `document_id,version` projection) that marks `acked[id]=true` only when an
ack row's version equals the doc's CURRENT version (a stale ack from an older
version does not satisfy the current one). `GET /documents` now returns the
`acked` map mirroring the Slice-7 announcements feed.

Access-shape: `TestDocumentsAckedByAccessShape` asserts exactly ONE query, narrow
projection, `IN (?)` batch (no N+1).

Benchmark (50 docs all acked at current version, SQLite, `-benchmem -count=3`):

| Benchmark | ns/op (median) | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkDocumentsAckedBy` | ~51,800 | 20,768 | 372 |

Command:
`go test ./internal/database -run '^$' -bench 'BenchmarkDocumentsAckedBy' -benchmem -count=3`.
Single establishing baseline (the read is written narrow from the start — one
IN-filtered query, no before/after delta).

### IMP-4 — checklist run-detail read (functional gap)

New `GetChecklistRunDetail(businessID, runID)` returns `ChecklistRunDetail{Run,
Items}`: the run loaded tenant-scoped (else `ErrChecklistRunNotFound`) plus its
items + per-run completion state in ONE join query
(`checklist_item_completions cc JOIN checklist_items ci`, narrow projection,
ORDER BY ci.sort_order, ci.id, LIMIT 500 — no per-item reload). Wired as
`GET /businesses/:id/checklists/runs/:runId` (checklist:complete); the handler
enforces own-run scope (403 "Not your checklist") for non-managers via
`RunOwnedByStaff`, managers/owners read any run.

Access-shape: `TestGetChecklistRunDetailAccessShape` asserts exactly ONE
`JOIN checklist_items` query, narrow projection, LIMIT present.

Benchmark (run with 25 items, SQLite, `-benchmem -count=3`):

| Benchmark | ns/op (median) | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkGetChecklistRunDetail` | ~66,800 | 23,651 | 449 |

Command:
`go test ./internal/database -run '^$' -bench 'BenchmarkGetChecklistRunDetail' -benchmem -count=3`.
Single establishing baseline (run load + one bounded join; no before/after delta).

---

## Staff notifications — Task 1 baseline (2026-06-30)

### Benchmark
`BenchmarkListStaffNotifications` in `internal/database/staff_notification_service_test.go`

### Setup
- SQLite in-memory, 200 seeded notifications, 50-per-page read
- Apple M3, Go 1.25, `goos: darwin goarch: arm64`

### Results (`-benchmem -count=1`)
| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkListStaffNotifications-8 | 353,510 | 41,071 | 933 |

### Access-shape notes
- Exactly one query over `staff_notifications` with explicit projection (no `SELECT *`), row-scoped by `staff_id`, bounded by `LIMIT`.
- The (business_id, staff_id, id) composite index serves both the paged inbox read and the unread count.

---

## Hours history — Task 9 baseline (2026-06-30)

### Benchmark
`BenchmarkListTimesheetRange` in `internal/database/timeentry_service_test.go`

### Setup
- SQLite in-memory, 200 seeded approved time entries spanning 200 days, half-open range of 60 days (days 30–90)
- Apple M3, Go 1.25, `goos: darwin goarch: arm64`

### Results (`-benchmem -count=1`)
| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkListTimesheetRange-8 | 1,165,859 | 76,374 | 1,619 |

### Access-shape notes
- Exactly one query over `time_entries` with explicit projection (no `SELECT *`), row-scoped by `staff_id`, filtered by `clock_in_at` (uses the `idx_time_entries_biz_staff_clockin` composite index), bounded by `LIMIT`.
- No new migration needed — the `(business_id, staff_id, clock_in_at)` index already ships in 000106.
## Sage Briefing Front Door — Director Console (2026-06-30)

New owner-gated `GET /api/v1/inside/businesses/:id/director-console/briefing`
assembles `state + pulse + insights + play + win` in one cached round-trip,
reusing `buildProactiveInsights()` and the analytics / foodcost / labor /
menuengineering calculators. `internal/server/director_briefing.go` (pure
assembler + pace/play/win math), `internal/server/director_briefing_handler.go`
(handler, 60s cache, providers, route in `cmd/app/main.go`).

### Perf contract — foodcost computed exactly once
`foodcost.Report` is computed ONCE per request (via the `briefingProviders.FoodCost`
closure) and fed to all three consumers: the pulse food-cost number,
`menuengineering.Classify` (the play), and the food-cost-high insight check.
`buildProactiveInsights` was refactored into `buildProactiveInsightsWithFood(businessID, *foodcost.Report)`
so the insight path reuses the precomputed report instead of recomputing
`foodcost.Analyze`. Extracted `foodCostOverThresholdFromReport` (pure) shared by
the digest and the briefing.

### Access-shape / regression tests
- `TestAssembleBriefingComputesFoodCostExactlyOnce` — spies the foodcost provider
  and asserts `Analyze` is invoked **exactly once** per briefing request (guards
  recompute regressions across the pulse/play/insight consumers).
- `TestBuildProactiveInsightsWithFood_ReusesPrecomputedReport` — the food-cost-high
  insight is derived from the precomputed report (the nil/recompute path surfaces
  nothing without recipe data, confirming reuse).
- `TestGetOrBuildBriefing_ServesFromCacheWithinTTL` / `_RebuildsAfterInvalidate` —
  60s cache contract.
- Gating: `TestGetDirectorBriefing_DeniesNonOwner` (staff + cross-tenant → 403),
  `TestGetDirectorBriefing_RequiresAIProPlan` (Core → 402), and
  `TestGetDirectorBriefing_GatedByRequireAIProPlanMiddleware` (route-level
  `ai_pro_required` contract) — mirror the proactive-insights gates.
- Pure math: `computeDayFraction`, `computePace`, `typicalDayRevenue`, `selectPlay`,
  `selectWin`, `briefingState`.

### Benchmark (`-benchmem -run '^$' -count=3`)
`BenchmarkGetDirectorBriefing` in `internal/server/director_briefing_test.go`
measures the per-request assembly hot path (menu-engineering classification +
pace/play/win math) with realistic fixture sizes (24-dish foodcost report, 28-day
timeseries). Provider closures are stubbed so the bench isolates assembly CPU /
alloc cost — the DB round-trips behind the real providers sit behind the 60s
cache and are not the per-request bottleneck.

- Apple M3, Go 1.25, `goos: darwin goarch: arm64`.

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkGetDirectorBriefing (assembler, 24-dish report, 28-day series) | ~10,600–48,600 (machine-load dependent) | 12,496 | 38 |

No "before" microbench exists — this is a new endpoint, so the numbers above are
the absolute assembly cost and the baseline to defend against future regressions.
`B/op` (12,496) and `allocs/op` (38) are stable across runs; `ns/op` varies with
host load. The DB-touching work is cached 60s and the result set is bounded
(≤3 insights, ≤1 play).

### Commands run
- `go test ./internal/server/ -run 'Briefing' -count=1` → ok
- `go test ./internal/server/ -bench 'BenchmarkGetDirectorBriefing' -benchmem -run '^$' -count=3`
- `go test ./internal/server ./internal/handlers ./internal/database -run '^$' -count=1` → 3 ok
- `go build ./...` clean, `go vet ./internal/server/` clean

### Follow-up: labor-once + writer-side cache invalidation (adversarial-review fixes, 2026-06-30)

Two perf/freshness fixes surfaced by the post-build adversarial review (no blockers; these were the substantive minors):

- **labor.Analyze computed exactly once.** The foodcost-once thread did not cover labor, which has the same dual-consumer shape (pulse number + labor-high insight). On a calm briefing (<3 exception insights, so the labor slot is reached) labor was recomputed twice per cache miss. Fixed by extending the insight seam to carry a precomputed labor report: `buildProactiveInsightsWithFood` → `buildProactiveInsightsWithReports(businessID, *foodcost.Report, *labor.Report)`, new pure `laborOverThresholdFromReport` (and `getLaborOverThreshold` now delegates to it), and `assembleBriefing` feeds the single `providers.Labor()` report to both the pulse and the insights consumer.
  - Access-shape regression: `TestAssembleBriefingComputesLaborExactlyOnce` (Labor spy asserts exactly one call + the insights consumer receives the report), `TestLaborOverThresholdFromReport`.
- **Writer-side briefing invalidation.** The briefing embeds the same insights + live open-bill count as proactive-insights, but only `InvalidateInsightCache` was wired to the 7 writer sites (bill paid, inventory restock/adjust ×3, AI conversation ×3) — the briefing showed stale "Needs you" items for up to 60s after an owner acted. Added `invalidateOwnerHomeCaches(businessID)` (clears both caches; the briefing is a superset of the insights) and routed all 7 sites through it. Regression: `TestInvalidateOwnerHomeCachesClearsBothCaches`.

Commands: `go test ./internal/server/ -run 'Briefing|ProactiveInsights|FoodCostOverThreshold|LaborOverThreshold|OwnerHomeCaches' -count=1` → ok; `go build ./internal/server/ ./internal/handlers/ ./cmd/...` clean; `go vet ./internal/server/` clean; gofmt clean.
## Marketing SuggestionEngine (2026-06-29)

`marketing.Engine.Suggest` turns a business's own POS signals into ranked campaign suggestions.
`BenchmarkSuggest` (SQLite in-memory, 50 customers + 50 settled bills + 1 bundle, menuEng stubbed, analytics nil):
- baseline: 98007 ns/op, 82133 B/op, 657 allocs/op (median of 3 runs: 85158 / 98007 / 111984 ns/op; B/op stable at ~82133; allocs/op=657)
Access shape: `TestSuggest_AccessShape` asserts ≤4 SELECTs (weakest-daypart, lapsed count, active bundles) — no N+1.
Commands: `go test ./internal/services/marketing/... -bench BenchmarkSuggest -benchmem -count=3`

## Marketing Ready-to-Post Gallery — image/offer enrichment (2026-06-30)

`Engine.Suggest` now resolves a free image per card (menu dish photo / offer /
bundle) and adds the `offer` play. Two new bounded reads: `menuImageIndex`
(one active-menu JSON read) and `activeOffers` (narrow projection, no Business
relation, LIMIT 5).

- Access shape: `TestSuggest_AccessShape` budget 4 → 6 SELECTs (added menu +
  offers); still O(1), no N+1.
- `BenchmarkSuggest -benchmem -count=3` (SQLite in-memory, 50 customers + 50
  settled bills + 1 bundle + 1 offer + 1 menu, menuEng stubbed, analytics
  nil), Apple M3, `goos: darwin goarch: arm64`:
  - Before: new reads — the pre-image engine (5-play, no menu/offer reads)
    baseline is the "Marketing SuggestionEngine (2026-06-29)" entry above
    (98007 ns/op median, 82133 B/op, 657 allocs/op). A clean re-benchmark of
    that exact pre-slice commit was not re-run for this closeout (Task 6
    explicitly allows this — re-measuring the old shape adds no signal since
    the whole point of the slice is the new reads); the numbers below are the
    honest AFTER measurement of the full enriched path.
  - After (enriched full path, 3 samples):
    - 113399 ns/op, 177452 B/op, 901 allocs/op
    - 114484 ns/op, 177448 B/op, 901 allocs/op
    - 114896 ns/op, 177441 B/op, 901 allocs/op
    - median: 114484 ns/op; B/op stable at ~177448; allocs/op stable at 901
  - Delta vs. the 2026-06-29 baseline: +~17,000 ns/op (+~17%), +~95,300 B/op
    (+~116%), +244 allocs/op (+~37%) — expected given the two new reads (menu
    JSON decode + offer projection) plus the extra `offer` play's
    image/discount payload; still a bounded-query, sub-millisecond operation
    with no N+1.
- Commands: `go test ./internal/services/marketing/ -bench BenchmarkSuggest -benchmem -run '^$' -count=3`

## Manager live-floor read (Phase 3, Slice 3b — 2026-06-30)

New backend for the manager live-floor board (who's on the clock / scheduled /
late / no-show for the business day). Two bounded, tenant-scoped read methods +
a pure status function; no writes, money-free (hours-context only).

- `database.GetActiveShiftsForDay(bizID, dayStart, dayEnd)` — published, assigned
  (staff_id NOT NULL), filled shifts overlapping [dayStart,dayEnd) (overnight-safe:
  `starts_at < dayEnd AND ends_at > dayStart`). Explicit `staffShiftColumns`
  projection (no SELECT *, no operator-only `reminded_at`), `LIMIT 500`.
- `database.ListEntriesForDay(bizID, dayStart, dayEnd)` — every open punch (even
  one opened before the window) + every punch clocked-in during the day.
  `timeEntryColumns` projection, `LIMIT 500`, single query.
- `database.StaffNamesByIDs(bizID, ids)` — one batched `id,name WHERE id IN` map
  (no N+1 name lookups on the board).
- `handlers.computeLiveFloor(shifts, entries, now, grace)` — pure, deterministic
  status logic (on_clock > done > scheduled/within-grace > late > no_show), plus
  clocked-in-without-a-shift extras (staff-id sorted). 5-min late grace.

Access-shape regressions (assert the dangerous shape is gone): `TestGetActiveShiftsForDayAccessShape`
(staff_id IS NOT NULL in SQL, no SELECT *, no reminded_at, single query, open+out-of-window
excluded), `TestListEntriesForDayAccessShape` (open-OR-clocked-in-today in SQL, no SELECT *,
single query, prior-day closed excluded). Handler path: `TestLiveFloorEndpoint`
(name resolution, {date,rows,summary} envelope, money-free — asserts no "$"/"rate").
Pure logic: 11 `TestComputeLiveFloor_*` + `TestSummarizeLiveFloor`.

Benchmarks (Apple M3, Go 1.25, `goos: darwin goarch: arm64`, SQLite in-memory, 40 rows):

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkGetActiveShiftsForDay | ~130,409 | 40,166 | 873 |
| BenchmarkListEntriesForDay | ~158,536 | 45,204 | 1,059 |

New reads, so no "before" — these are the baselines to defend. Both are single
bounded queries (proven by the access-shape tests); the board's total cost is
2 scans + 1 batched name map, independent of crew size beyond the LIMIT.

### Commands run
- `go test ./internal/database/ -run 'GetActiveShiftsForDay|ListEntriesForDay' -count=1` → ok
- `go test ./internal/database/ -bench 'GetActiveShiftsForDay|ListEntriesForDay' -benchmem -run '^$'` → measured
- `go test ./internal/handlers/ -run 'ComputeLiveFloor|SummarizeLiveFloor|LiveFloorEndpoint' -count=1` → ok
- `go test ./internal/handlers/ -count=1 -timeout 150s` → ok; `go test ./internal/database/ -skip Postgres -count=1` → ok (2.8s)
- `go build ./...` clean; `go vet ./internal/handlers/ ./internal/database/` clean; gofmt clean

Note: `internal/database` run-all hangs on the pre-existing `TestDeliveryMoneyMigration_Postgres`
(Testcontainers/Postgres, no Docker in sandbox); it is unrelated to this slice and skipped above.

## Phase 4b — Kiosk clock-in (shared-terminal PIN punch) — perf + security gate

A shared tablet runs under a manager/owner session (`timeclock:manage`); staff
enter a PIN to toggle their OWN punch. No independent staff session is minted —
the PIN only attributes the punch (device-bound "kiosk" security model, chosen by
the operator over full-PIN-login). Money-free on every surface.

### Surface
- `database.KioskRoster(bizID)` — active staff for the terminal, annotated with
  `has_pin` + current `on_clock`/`clock_in_at`. TWO bounded, tenant-scoped,
  explicitly-projected queries (staff via `kioskStaffColumns` = id,name,role,
  pin_set_at — **pin_hash is never selected**; open punches via staff_id,
  clock_in_at), joined in memory. O(1) queries regardless of roster size, `LIMIT 500`.
- `database.GetKioskStaff(bizID, staffID)` — single active staffer scoped to
  business+active in SQL (foreign/inactive → `ErrStaffNotFound`). This is the
  tenant safety net: `VerifyStaffPin` is NOT tenant-scoped, so the handler loads
  the staffer scoped FIRST, before any PIN check.
- `database.GetOpenEntry(bizID, staffID)` — public wrapper over `findOpenEntry`
  (toggle predicate: open punch → clock out, else clock in).
- `server.VerifyKioskPin(staffID, pin)` — reuses the manager-PIN `auththrottle`
  instance under a DISTINCT kind `kiosk_pin` (a kiosk lockout never bleeds into
  the manager step-up counter and vice versa). Fail-closed on throttle-backend
  error; records only genuine wrong-PINs.
- `handlers.KioskHandler` — `GET /businesses/:id/kiosk/roster`,
  `POST /businesses/:id/kiosk/punch {staff_id,pin}`, both `timeclock:manage`-gated.

### Access-shape regressions (assert the dangerous shape is gone)
`TestKioskRosterAccessShape` (exactly ONE staff read + ONE open-entries read = no
N+1; no SELECT *; **pin_hash never in SQL**; business_id + is_active + LIMIT in
SQL). `TestKioskRosterAnnotations` (has_pin/on_clock join). `TestKioskRosterTenantIsolation`
+ `TestGetKioskStaffTenantScoped` (no cross-business, inactive excluded).
`TestGetOpenEntryToggleState`. Security: `TestVerifyKioskPin_*` (kind isolated
from manager_pin, wrong-PIN locks, fail-closed on broken backend, nil-throttle
still verifies, no-lockout for unenrolled). Handler: `TestKioskPunch_*`
(toggle in→out, foreign staff 404, no-pin 409, wrong-pin 403 pin_invalid,
bad body 400, money-free) + `TestKioskRosterEndpoint`.

### Benchmark (Apple M3, Go 1.25, `goos: darwin goarch: arm64`, SQLite in-memory, 50-person roster, count=3)

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkKioskRoster | ~91,421–103,548 | ~33,629 | 783 |

New read, no "before" — this is the baseline to defend. Two bounded scans + an
in-memory join, independent of roster size beyond the LIMIT.

### Commands run
- `go test ./internal/database/ -run 'Kiosk|GetOpenEntry' -count=1` → ok
- `go test ./internal/database/ -bench 'BenchmarkKioskRoster' -benchmem -run '^$' -count=3` → measured
- `go test ./internal/server/ -run 'VerifyKioskPin' -count=1` → ok
- `go test ./internal/handlers/ -run 'Kiosk' -count=1` → ok
- Full regressions: `go test ./internal/database ./internal/server ./internal/handlers -skip 'Postgres|_PG|Testcontainer' -count=1` → all ok
- `go build ./...` clean; `go vet` on all three packages clean; new files gofmt-clean
  (a pre-existing double-blank-line at main.go:650 is unrelated to this slice, left untouched).

## Phase 4c — Coverage cancel/withdraw (staff retracts their own request)

Closes the last gap in the coverage loop: a staffer can now **cancel** a swap/giveup
they offered or **withdraw** an open-shift claim they filed, while it is still
non-terminal. FE-facing initiation (offer/give-up on own shifts) rides existing
`RequestSwap`; this BE slice adds only the retract path.

### Endpoint
- `POST /businesses/:id/coverage/:requestId/cancel` — `schedule:self`, body `{ kind:
  "swap"|"giveup"|"open_claim" }`. Polymorphic like the decision route: `:requestId`
  is the swap id (swap/giveup) or the claim id (open_claim). Lives in the POST tree,
  so no conflict with the GET `/coverage/open|mine` siblings.

### Service (`database.CancelSwap` / `database.WithdrawClaim`)
Ownership + CAS enforced in one transaction, **never touches the shift** (a
non-approved request never moved it — reassignment only happens in
`applySwapApproval`/`DecideClaim`):
- `CancelSwap`: only the requester; legal from open/accepted/pending_approval via
  `validSwapTransition(…, cancelled)`; atomic `UPDATE … WHERE requesting_staff_id=?
  AND status IN (open,accepted,pending_approval)` → `cancelled` (RowsAffected!=1 →
  `ErrCoverageConflict`, so a race with a manager decision loses cleanly).
- `WithdrawClaim`: only the claimant; legal only from pending; atomic `UPDATE … WHERE
  claiming_staff_id=? AND status='pending'` → `withdrawn`.
- New sentinel `ErrNotOwner` → 403; new audit action `coverage_cancelled` written
  atomically inside the tx (same discipline as the other coverage transitions).
- Cancel reuses the `shift.swap.decided` / `openshift.decided` SSE events so the
  cancelled request leaves BOTH the manager pending queue and the staffer's list.

### Regression tests (correctness, not a hot read → no benchmark)
This is a low-frequency O(1) guarded single-row mutation, not a list/hot path, so the
perf-gate benchmark step does not apply; the risk is *authorization + concurrency*,
covered by:
- `internal/database`: `TestCancelSwapByRequester` (cancels, shift untouched, audit
  row), `TestCancelSwapPendingApprovalGiveup` (giveup path, shift stays owned),
  `TestCancelSwapRejectsNonOwner` (ErrNotOwner + no mutation), `TestCancelSwapRejectsTerminal`
  (409), `TestCancelSwapNotFoundAndTenantScoped` (missing + cross-tenant → not found),
  `TestWithdrawClaimByOwner` (withdrawn, shift stays open, audit),
  `TestWithdrawClaimRejectsNonOwnerAndNonPending`.
- `internal/handlers`: `TestCoverageCancelSwapHTTP` (200 then 409),
  `TestCoverageCancelForeignRequestIs403` (foreign cancel 403 + no mutation),
  `TestCoverageWithdrawClaimHTTP`, `TestCoverageCancelBadKind400`,
  `TestCoverageCancelOwnerForbidden` (owner has no request → 403).

### Commands run
- `go test ./internal/database/ -run 'TestCancelSwap|TestWithdrawClaim' -count=1` → ok
- `go test ./internal/handlers/ -run 'TestCoverageCancel|TestCoverageWithdraw' -count=1` → ok
- Full regressions: `go test ./internal/database ./internal/handlers ./internal/server -skip 'Postgres|_PG|Testcontainer' -count=1` → all ok
- `go build ./...` clean; `go vet ./internal/database ./internal/handlers` clean; new
  files gofmt-clean (pre-existing main.go:650 double-blank unrelated, left untouched).

---

## Phase 5 Slice 5a — Engagement badges (surface buried comms/checklists)

Staff bury themselves: unacknowledged must-read announcements and not-yet-done
checklists live two taps deep in the "More" menu, so they get missed. Slice 5a
surfaces those as **badge counts** the staff nav can show without opening either
surface. Backend-first: two cheap counts behind one self-scoped `/me` endpoint.

### Data layer (`internal/database`)
- `CountUnackedAnnouncements(businessID, staffID, role)` (chat_announcement_service.go):
  counts `require_ack` announcements targeted at the caller — audiences resolved
  EXACTLY as `ListAnnouncements` (all + `role:<role>` + `dept:<each active dept>`) —
  that the caller has NOT yet acked. TWO bounded queries (the caller's depts, then
  ONE `NOT EXISTS (announcement_acks…)` count) — never N+1 per announcement. An
  owner (staffID 0, who cannot ack) counts nothing; foreign tenant → 0.
- `CountPendingChecklistRuns(businessID, staffID)` (engagement_service.go): ONE
  indexed COUNT over `idx_checklist_runs_assigned` of the caller's own runs with
  `status <> 'complete'` (pending + in_progress). Tenant-scoped; owner → 0.

### Handler + route (`internal/handlers`, `cmd/app/main.go`)
- `EngagementBadgesHandler.Badges` (engagement_badges.go) — a `/me` self route
  mirroring the notifications-unread-count pattern: self-scoped by context
  `staff_id` (owner/no-staff-row → 403), role from `staff_role` context. Envelope
  `{"success":true,"data":{"unacked_announcements":N,"pending_checklists":M}}`.
  Money-free.
- Route: `GET /businesses/:id/me/engagement-badges`, `RequireActiveSubscription()`
  only — NO `RoleBasedAccessMiddleware` (self-scoped; tenant isolation lives in
  HybridAuth, per the Slice-9 self-route pattern).

### Perf gate
This backs a polled staff-nav badge, so both reads are kept cheap/bounded:
- Access shape: `TestCountUnackedAnnouncementsAccessShape` seeds 20 targeted
  notices and asserts ≤2 SELECTs (dept resolution + one NOT-EXISTS count), never
  N+1. The checklist count is a single indexed COUNT (inherently O(1)).
- Benchmark: `BenchmarkCountUnackedAnnouncements` (50 mixed-audience notices, BOH
  server) → ~32.2µs/op, 11232 B/op, 143 allocs/op (baseline; SQLite micro).

### Tests
- `internal/database`: `TestCountUnackedAnnouncements` (audience match, non-ack /
  wrong-role / wrong-dept / acked excluded, owner + tenant → 0),
  `TestCountUnackedAnnouncementsAccessShape`, `TestCountPendingChecklistRuns`
  (pending + in_progress counted, complete + other-staffer excluded, owner +
  tenant → 0).
- `internal/handlers`: `TestEngagementBadgesCountsUnackedAndPending` (envelope +
  money-free), `TestEngagementBadgesRejectsOwnerWithNoStaffRow` (403).

### Commands run
- `go test ./internal/database/ -run 'TestCountUnacked|TestCountPending' -count=1` → ok
- `go test ./internal/handlers/ -run 'TestEngagementBadges' -count=1` → ok
- `go test ./internal/database/ -run '^$' -bench BenchmarkCountUnackedAnnouncements -benchmem` → 32238 ns/op
- `go build ./...` clean; new files gofmt-clean (pre-existing main.go:650 double-blank unrelated, left untouched).

---

## Phase 5 Slice 5b (BE) — Shift-logbook author enrichment

The operator logbook needs to show WHO logged each shift-handover note
(accountability); the staff feed hid authorship. `LogbookHandler.List` now
enriches each note with `author_name`.

- `shiftNoteResponse` embeds `database.ShiftNote` (json fields serialize flat) +
  `author_name`. Additive: the staff logbook feed simply ignores the new field.
- Names resolve via the existing batched `StaffNamesByIDs(businessID, distinctIDs)`
  — ONE query over the distinct authors, never N+1. Covers since-deactivated
  authors (id-scoped, not active-scoped); an unknown/removed author degrades to an
  empty name, never an error.

### Perf gate
This is a list read (operator feed). No new DB access pattern: the handler
composes two already-perf-gated primitives — `ListShiftNotes` (bounded, explicit
projection, single query, its own access-shape test) and `StaffNamesByIDs`
(batched id→name, benched for live-floor) — with an O(n) in-memory map join. The
List path issues exactly 2 bounded queries regardless of note count; N+1 is
structurally impossible. No redundant benchmark added for the composition.

### Tests
- `internal/handlers`: `TestLogbookListEnrichesAuthorNames` (known author → name;
  unknown author → empty, no error). Existing `TestLogbookHandlerHTTP` List path
  still green with the staff table now migrated in the test server.

### Commands run
- `go test ./internal/handlers/ -run 'TestLogbook' -count=1` → ok
- `go build ./...` clean; `go vet ./internal/handlers` clean; touched files gofmt-clean.

---

## Phase 5 Slice 5c (BE) — Coverage / approval history

Managers could see the pending coverage queue but had no record of what was
already decided. New read: the resolved coverage feed.

### Data layer (`internal/database/coverage_service.go`)
- `ListCoverageHistory(businessID, limit)` returns `[]CoverageHistoryItem` — the
  terminal swaps (approved/denied/cancelled) + terminal claims (approved/denied/
  withdrawn) flattened across both tables, newest resolution first. Each item
  carries kind (swap|giveup|open_claim), the joined shift (position_id, starts_at,
  ends_at), status, requester + decider staff IDs (nil decider for a self-cancel/
  withdraw), and resolved_at. Staff are IDs — the operator surface resolves names
  from its roster, matching the other coverage lists (money-free).
- Shape: TWO bounded, narrow-projection joins (one per table, each on its
  (business_id, status) index), merged + sorted in memory. The per-table LIMIT is
  a safe upper bound (global newest-`limit` ⊆ union of each table's newest-`limit`),
  so post-merge truncation yields the correct page. No N+1, no SELECT *.

### Handler + route (`internal/handlers/coverage.go`, `cmd/app/main.go`)
- `CoverageHandler.History` — gated `schedule:read` at the route AND restricted to
  approvers in the handler (`callerCanApprove`: owner/manager/schedule:approve).
  A non-approver gets 403, which the operator History panel treats as "stay
  hidden". `?limit` (default 50; the service clamps to coverageListMax=200).
- Route: `GET /businesses/:id/coverage/history`. No Gin conflict (literal GET
  segment; the only `/coverage/:x` route is the POST cancel).

### Perf gate
- Access shape: `TestListCoverageHistoryAccessShape` (laborSQLRecorder) asserts
  exactly 2 SELECTs, each LIMIT-bounded, no SELECT *.
- Benchmark: `BenchmarkListCoverageHistory` (100 terminal events, limit 50) →
  ~402µs/op, 121583 B/op, 2498 allocs/op (baseline; SQLite micro). On-demand read
  (not polled).

### Tests
- `internal/database`: `TestListCoverageHistory` (terminal-only, cross-table merge,
  newest-first, self-action → nil decider, position join, limit-after-merge, tenant
  scope), `TestListCoverageHistoryAccessShape`.
- `internal/handlers`: `TestCoverageHistoryHTTP` (approver sees enriched shape,
  open request excluded, money-free), `TestCoverageHistoryForbidsNonApprover` (403).

### Commands run
- `go test ./internal/database/ -run TestListCoverageHistory -count=1` → ok
- `go test ./internal/handlers/ -run TestCoverageHistory -count=1` → ok
- `go test ./internal/database ./internal/handlers -run 'Coverage|Logbook|Engagement|CountUnacked|CountPending' -skip 'Postgres|_PG|Testcontainer' -count=1` → ok
- `go build ./...` clean; touched files gofmt-clean.

## 2026-07-03 — Guest service-call ("call waiter") endpoints (Task 10, notification-UX phase 4)

### Handlers + routes (`internal/server/service_call_handlers.go`, `cmd/app/main.go`)
- `POST /guest/table/:code/service-call` (`guestOrderRateLimiter.RateLimit()`) —
  reason is a closed enum (water|order|check), anything else 400; idempotent
  while a call is open/acknowledged (200, no new row); 429 within the 2-minute
  post-resolve cooldown; otherwise 201 `{status:"open"}` via
  `operational_alerts.CreateServiceCallAlert`.
- `GET /guest/table/:code/service-call` — poll, 200 `{status}` (none/open/
  acknowledged/resolved). Table resolution reuses the existing cached
  `loadPublicGuestTableContext` (projected public business columns).
- `CreateServiceCallAlert` doc contract added: reason must be caller-validated
  enum, never guest free text (embedded verbatim in staff alert copy).

### Perf gate
- Access shape: `TestServiceCallStatusQueryShape` — status read is exactly one
  `SELECT status, resolved_at ... LIMIT 1` on operational_alerts (no SELECT *);
  table lookup stays a single projected join query.
- Benchmark: `BenchmarkGuestServiceCallStatus` (SQLite micro, route → resolve →
  status read) baseline ×3: 16573 / 16639 / 16988 ns/op, 15920 B/op, 122 allocs/op.
- Command: `go test ./internal/server -bench BenchmarkGuestServiceCallStatus -benchmem -run '^$' -count=3`

### Tests
- `internal/server`: create 201 + urgent alert row; bad reason 400 (incl. XSS
  string, empty, missing, non-JSON); idempotent second POST 200 + one row;
  cooldown 429 fresh-resolve / 201 after 3 min; GET none→acknowledged; unknown
  code 404 on both verbs.
- Gates: `go test ./internal/handlers ./internal/server ./internal/database
  ./internal/events ./internal/services/operational_alerts -run '^$' -count=1` ok;
  `go test ./internal/events/ -count=1` ok (SSE permission-map test green);
  `go test ./internal/server -run 'ServiceCall' -count=1` ok; touched files gofmt-clean.

## 2026-07-15 review remediation perf
See root `summary.md` section "2026-07-15 review remediation perf (Waves 3–4)" for BenchmarkSubscriptionValueSummarySQLite, BenchmarkGetAiAttributedOrderValueSQLite, BenchmarkGetTipsByStaff, BenchmarkGetReservationGuestHistory baselines.

## 2026-07-21 Delivery dashboard remediation (Stream 3) perf

### Multi-status delivery list (`status` accepts comma-separated list; optional/BE-first)
- Access shape: `TestGetBusinessDeliveriesFilteredKeepsNarrowShape` (Driver preload
  stays the projected id/business_id/name/phone/email summary; no SELECT * widening);
  `TestGetBusinessDeliveriesMultiStatusUsesINAndRejectsUnknown` (one `status IN (...)`
  query — not N queries; unknown value in the list → `ErrUnknownDeliveryStatusFilter`).
- Benchmarks (SQLite micro, Apple M3, `-benchmem -count=3`):
  - `BenchmarkGetBusinessDeliveriesWithStatusFilterSQLite` (legacy single-status
    baseline): 8.38 / 21.17 / 12.02 ms/op, ~3.696 MB/op, ~10,060 allocs/op.
  - `BenchmarkGetBusinessDeliveriesWithMultiStatusFilterSQLite` (after): 14.01 /
    11.10 / 18.59 ms/op, ~3.697 MB/op, ~10,102 allocs/op — bytes/allocs flat;
    latency within run-to-run noise of the baseline; still exactly one COUNT +
    one SELECT.
- Command: `go test ./internal/services/ -run '^$' -bench 'BenchmarkGetBusinessDeliveriesWith' -benchmem -count=3`

### Operator PATCH deliveries/:id (new scoped contact/address/instructions/ETA edit)
- Access shape: `TestUpdateDeliveryOrderContact_RejectsTerminalAndPatchesActive` —
  terminal rows 409 via conditional UPDATE (`status NOT IN (terminal)` +
  RowsAffected guard, race-proof), non-terminal patch writes only the supplied
  columns through one Updates map (no full-row Save) and appends one
  status-history audit row.
- Benchmark (new path, no before): `BenchmarkUpdateDeliveryOrderContactSQLite`
  9.62 / 14.67 / 14.24 ms/op, 6.3–10.1 MB/op, 5.7k–8.6k allocs/op.
- Command: `go test ./internal/services/ -run '^$' -bench 'BenchmarkUpdateDeliveryOrderContact' -benchmem -count=3`

### Gates
- `go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1` ok;
  `go test ./internal/services/ -run 'Delivery' -count=1` ok;
  `go test ./internal/handlers/ -run 'Delivery' -count=1` ok; `go build ./...` ok.

## Stream 8 — AI Waiter + Director Console (2026-07-21)

### AI insights aggregation (fix 4): bounded aggregate + 60s cache
- Access shape: `TestGetAiInsights_NoTimestampPluck` — the timeseries no longer
  plucks up to 10k `created_at` rows into Go; the 7-day trend and 24h
  distribution come from dialect-dispatched SQL `GROUP BY` aggregates
  (`database.GetAiConversationTimeSeries`). Any conv-table read touching
  `created_at` must contain `GROUP BY` (attribution's `bill_items` aggregate
  excluded). `TestGetAiInsights_ServesFromCacheWithinTTL` — a second call within
  60s does not re-run the folded conv-count / timeseries aggregation.
- `BenchmarkGetAiInsights` (500 conversations, realistic repeated-poll path
  hitting the cache):
  - before: 1.12 / 1.18 / 1.15 ms/op, 168,861 / 168,848 / 168,796 B/op, 3114 allocs/op
  - after:  0.273 / 0.259 / 0.284 ms/op, ~56,652 B/op, 743 allocs/op
    (-76% latency, -66% B/op, -76% allocs on the polled path)
- Cold build (cache invalidated each iteration, GROUP BY vs Pluck, no cache win):
  - after: 1.08 / 1.48 / 1.34 ms/op, ~86,600 B/op, 1128 allocs/op
    (vs 3114 allocs / 168.8 KB before — -64% allocs, -49% B/op from dropping the
    10k-timestamp materialization; latency a wash on tiny SQLite, but the win is
    the eliminated row materialization that dominates at Postgres scale)
- Existing guards kept green: `TestGetAiInsights_CountsActiveAndCompletedSessions`,
  `TestGetAiInsights_NoLeakAcrossBusinesses`, `TestGetAiInsights_TimeSeriesShape`,
  `TestGetAiInsights_ConvCountsUseSingleQuery`.

### AI conversations list (fixes 5/8/11)
- Access shape: `TestGetAiConversations_NoBusinessBlobInPayload` — the model's
  `Business` embed is now a pointer (`*Business`) so `omitempty` drops the
  ~200-field zero-value blob; the row serializes `business_id` only.
- `TestGetAiConversations_StatusFilterAndActiveCount` — optional whitelisted
  `status` (active|closed) narrows both the page and `total_count` so the
  paginator is truthful; `active_count` is an independent COUNT that stays
  correct on any page/filter.

### AI transcript incremental reads (fix 6)
- `TestGetAiConversationMessages_SinceCursor` — optional `since` (RFC3339) returns
  only messages created after the cursor (chronological append); absent => the
  legacy full bounded transcript. Reuses `GetAiWaiterMessagesForTranscript`'s
  existing `since` support so the masking/projection regression tests stay green.

### Director thread lifecycle (fixes 1/2/9) + applied-actions (fix 3)
- `TestListDirectorThreads_SerializesPinnedAndOrdersPinnedFirst`,
  `TestListDirectorThreads_ArchivedFilterAndPaging` — `pinned` in the DTO,
  pinned-first order, `?archived=1` filter, `?limit`/`?offset`/`total` paging
  (legacy shape when no param).
- `TestListAppliedDirectorActions_UndoEligibility` /
  `TestListAppliedDirectorActions_RequiresOwner` — bounded newest-25 applied
  actions joined to their proposal for `public_id`; `can_undo` is server-truth
  (undone_at IS NULL AND within 24h window AND menu version unchanged);
  owner-gated read.

### Gates
- `go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1` ok;
  `go build ./...` ok. Scoped suites: `go test ./internal/server/ -run 'Ai|Director'`,
  `go test ./internal/database/ -run 'AiWaiter|AiAttribut|Director'` ok.
## 2026-07-21 Settings cluster remediation (Stream 9) perf

### Subscription payment history — server pagination + SQL total_paid aggregate
Was fetch-all: `GetStripeSubscriptionPaymentsByBusiness` loaded every row and the
dashboard summed lifetime "Total paid" client-side (a trap for any future LIMIT).
New `GetStripeSubscriptionPaymentsByBusinessPaged` returns a bounded window +
SQL `COUNT` + SQL `SUM(amount)` over completed rows. BE-first: no `limit`/`offset`
param → legacy full array (aggregates still attached). `total_paid` is float64
**dollars** (`sumCents/100`), matching the shape the FE already displayed via
`formatAmount` (cents/100) — no double-convert; per-row `amount` stays cents.
- Regression/access-shape: `stripe_subscription_payments_paged_test.go`
  (window + aggregate correctness; legacy no-param path unchanged).
- Benchmarks (SQLite micro, `-benchmem`, 200 completed rows):
  - `BenchmarkGetStripeSubscriptionPaymentsByBusiness_FetchAll` (before):
    ~3,022,883 ns/op, 535,981 B/op, 8,647 allocs/op.
  - `BenchmarkGetStripeSubscriptionPaymentsByBusinessPaged_Window` (after,
    limit=8 first page): ~358,498 ns/op, 34,812 B/op, 591 allocs/op —
    **≈8.4× faster, ≈15× fewer bytes/op, ≈15× fewer allocs/op**.
- Command: `go test ./internal/database -bench 'BenchmarkGetStripeSubscriptionPayments' -benchmem -run '^$' -count=1`

### Gallery write-path — delete-all+re-insert → ID-preserving upsert
Was: every Business Page save deleted all gallery rows and re-inserted (fresh IDs,
orphaning ID-keyed caption translations), then re-translated every caption into up
to 21 locales even on a typo-free reorder. New `UpdateBusinessGalleryImages` upserts
(match by ID, or by URL for legacy full-replace payloads), preserves primary keys for
unchanged rows, and returns `GalleryUpdateResult{CaptionsChanged, Deleted/Created/PreservedIDs}`
so the handler skips the translation fan-out when `CaptionsChanged=false` and only
clears translations for deleted rows. The product win is the skipped 21-locale
fan-out + ID stability (not visible in the raw SQL microbench, which is comparable
to delete-all for N=12).
- Regression/access-shape: `hospitality_test.go` (ID preservation, caption-change
  detection, deletion cleanup, URL-match for ID-less payloads).
- Benchmarks (SQLite micro, `-benchmem`, 12 images):
  - `BenchmarkGalleryWrite_CaptionTypoFix_DeleteAll` (legacy baseline):
    ~578,682 ns/op, 156,774 B/op, 1,074 allocs/op.
  - `BenchmarkGalleryWrite_CaptionTypoFix_Upsert` (after): ~558,880 ns/op,
    260,887 B/op, 1,228 allocs/op (raw SQL parity; win is the skipped translate
    fan-out + preserved translation rows).
  - `BenchmarkGalleryWrite_ReorderOnly_Upsert`: ~790,694 ns/op.
- Command: `go test ./internal/database -bench 'BenchmarkGalleryWrite' -benchmem -run '^$' -count=1`

### Latent unbounded endpoints bounded (optional params, BE-first)
- Plugin payment history (`GetPluginPaymentHistory`): optional `limit`/`offset`
  (default limit 50) via new `GetPluginPaymentHistoryPaged`; no params → legacy full list.
- Gallery list default limit; subscription list clamps `limit` to [1,200].

### Gates
- `go build ./...` ok; `go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1` ok;
  full `./internal/database`, `./internal/handlers`, `./internal/server` packages green.
