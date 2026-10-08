# OSS billing switch (BILLING_MODE): performance gate

> **Superseded.** `BILLING_MODE`, the plan predicates and the
> `payverge:normalize_billing` callback were removed with SaaS billing
> (migration `000001_drop_saas_billing`). This page is kept as the record of
> the 2026-10-03 measurement only; the commands below no longer apply.

Date: 2026-10-03. Branch `oss/billing` (open-source plan §3.3 / A1.1).
Scope: the three billing predicates that every paid route depends on, and
the Business load path, which now runs a read-time billing normalisation.

## What changed on hot paths

| Path | Stripe mode (hosted) | None mode (self-host) |
|---|---|---|
| `IsAIPlanEnabled`, `GetBusinessLockState`, `HasActiveSubscription` | One extra `config.BillingDisabled()` check: an atomic pointer load plus a 4-byte string compare. After that, the same logic as before. | The check returns early, so no `time.Now()` and no date math. |
| Every `Business` load (`First`/`Take`/`Find`/`Preload`) | GORM query callback `payverge:normalize_billing` runs after `gorm:after_query`. It does one atomic load and returns. | A reflect type check, then fields are written in place on `Business` rows only. Nothing is allocated. |
| `GetActiveTableWithBusinessByCode` (guest QR, `Joins("Business")`) | Explicit `NormalizeBillingFields`, which is a no-op in this mode. | The same in-place field writes. |

A query callback is used on purpose instead of a `Business.AfterFind` hook.
When a model has hooks, GORM dispatches them through `callMethod`, which
allocates a fresh `Session` on every query of that model even if the hook
does nothing. During development the AfterFind variant measured about 2 KB
and 3 extra allocations per Business load; that variant was not re-run for
the table below. The callback keeps the Business load allocation profile
identical to the code before the switch (see `BenchmarkBusinessLoadByID`).

## Regression and access-shape tests

- `internal/database/billing_mode_predicates_test.go`: `TestBillingModePredicates`
  checks the 3 predicates x 2 modes x 6 business shapes.
- `internal/database/billing_normalize_test.go`: covers load through
  First/Find (values and pointers)/Take/Preload, projections that are left
  alone, raw rows that are never rewritten, and the Joins-loaded QR path.
- `cmd/app/oss_billing_test.go`: `TestBillingModeNoneExpiredTrialCoreRoutesStay2xx`.
  With billing none, an expired-trial core business gets 2xx on
  menu/tables/orders/bills. With stripe, the same business gets 402.

## Method

- **Before** is `fda08a764`, the code before the switch (hosted semantics
  only). It was exported with `git archive` into a scratch tree. The
  benchmark bodies are identical except that the per-mode pin is dropped,
  because `config.BillingMode*` does not exist at that commit.
- **After** is `oss/billing`, run in both `stripe` and `none` modes.
- Before and after were interleaved as before / after / before / after,
  each with `-count=3`, giving n=6 per row. Results were compared with
  `benchstat`.

```bash
cd backend
env -u BILLING_MODE -u OPENROUTER_API_KEY GO_ENV=test \
  PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000 \
  go test ./internal/database -run '^$' \
  -bench 'BillingPredicate|BusinessLoad' -benchmem -count=3
```

- **Machine:** Apple M3, 8 cores, go1.26.6, SQLite in-memory for
  `BusinessLoadByID`. About 15 other agent workstreams shared the machine,
  and the load average was 15 to 29 during the run. The ns/op columns
  therefore carry wide confidence intervals (±14% to ±276%). The B/op and
  allocs/op columns are deterministic.

## Results (benchstat medians, n=6)

### ns/op

| Benchmark (shape) | Before (stripe) | After stripe | After none | Delta stripe |
|---|---:|---:|---:|---|
| IsAIPlanEnabled / active_ai_pro | 1.57 | 8.07 | 4.41 | +6.5 ns (p=0.009) |
| IsAIPlanEnabled / expired_trial_core | 1.56 | 5.65 | 3.01 | +4.1 ns (p=0.015) |
| IsAIPlanEnabled / grace_core | 1.20 | 3.24 | 2.96 | +2.0 ns (p=0.002) |
| IsAIPlanEnabled / suspended_core | 1.08 | 6.81 | 1.96 | +5.7 ns (p=0.009) |
| GetBusinessLockState / active_ai_pro | 62.07 | 66.31 | 4.40 | ~ (p=0.699) |
| GetBusinessLockState / expired_trial_core | 63.47 | 84.35 | 3.59 | ~ (p=0.180) |
| GetBusinessLockState / grace_core | 60.59 | 66.88 | 3.47 | ~ (p=0.310) |
| GetBusinessLockState / suspended_core | 57.09 | 59.33 | 3.43 | ~ (p=0.485) |
| HasActiveSubscription / active_ai_pro | 51.63 | 48.14 | 2.11 | ~ (p=0.937) |
| HasActiveSubscription / expired_trial_core | 53.55 | 71.33 | 2.64 | ~ (p=0.093) |
| HasActiveSubscription / grace_core | 56.59 | 48.67 | 2.49 | ~ (p=0.485) |
| HasActiveSubscription / suspended_core | 50.27 | 52.84 | 2.08 | ~ (p=0.394) |
| BusinessLoadByID (SQLite) | 166.4 µs | 153.8 µs | 188.8 µs | ~ (p=0.699) |

### B/op and allocs/op

| Benchmark | Before | After stripe | After none |
|---|---|---|---|
| All 12 predicate rows | 0 B, 0 allocs | 0 B, 0 allocs | 0 B, 0 allocs |
| BusinessLoadByID | 29.15 KiB, 616 allocs | 29.15 KiB, 616 allocs | 29.15 KiB, 616 allocs |

## Reading the numbers

- **Allocations are unchanged everywhere.** No predicate allocates in
  either mode. A Business load allocates exactly as before in both modes,
  so the normalisation callback adds no garbage.
- **`IsAIPlanEnabled` in stripe mode costs 2 to 6 ns more per call.**
  Before the switch the function was a constant compare that the compiler
  inlined into the loop, at about 1 ns. It now makes one non-inlined
  `config.BillingDisabled()` call. The fixed cost is a few nanoseconds and
  does not allocate, so it is negligible next to any request (the cheapest
  Business load here is about 150 µs).
- **`GetBusinessLockState` and `HasActiveSubscription` in stripe mode** show
  no statistically significant change; every row has p > 0.05.
- **In none mode `GetBusinessLockState` and `HasActiveSubscription` are
  15 to 25 times faster** than the hosted path, at 2 to 4.5 ns versus 48 to
  84 ns, because they return before the date math. `IsAIPlanEnabled` in
  none mode (2 to 4.5 ns) sits in the same few-nanosecond band as stripe.
- **`BusinessLoadByID`** is within noise in both modes. In none mode the
  in-place field writes do not show up next to the SQLite round trip.

## Optimisation tried and not kept

The pointer cache (`atomic.Pointer[BillingModeValue]`) was replaced with an
`atomic.Uint32` state. That removes the pointer dereference and the string
compare from `BillingDisabled`. The function still cannot be inlined, at a
cost of 81 to 87 against the budget of 80, because of the lazy env-read
fallback call.

An interleaved n=6 comparison against the pointer version, run at a load
average of about 42, found no significant difference on any
`IsAIPlanEnabled` row (all p > 0.05). The change was reverted to keep the
committed, fully tested cache.

Resolving the mode eagerly at package init would make `BillingDisabled`
inlinable. It would also break env-driven test resets and any future
runtime env loading, so it was not done.

## Not run / follow-ups

- Every benchmark here is a SQLite or pure-CPU microbenchmark. The
  per-Business cost against Postgres (Testcontainers) was not measured. It
  is only the callback's atomic load in stripe mode, so no change is
  expected.
- A rerun on a quiet machine would tighten the ns/op intervals. The
  allocation conclusions do not depend on machine load.

## Fix round 1 (review findings F2 and F3)

### What changed on hot paths

- **Admin lock in none mode (F2).** `GetBusinessLockState` and
  `HasActiveSubscription` no longer return active unconditionally under
  `BILLING_MODE=none`. They read `closed_at` and `is_active` (admin close
  and suspend), with no date math and no allocation. The lock-feeding
  projections now select those two columns:
  - `GetBusinessAuthScope`
  - `GetBusinessByCustomURL`
  - `GetActiveTableWithBusinessByCode`

  The table loader also selects `is_demo`.
- **Write guard (F3).** A new update callback, `payverge:guard_billing_write`,
  runs before `gorm:update`. In none mode it omits the billing columns from
  struct-shaped `Business` saves, so the read-time normalisation is never
  written back. Map updates and writes to other models only pay its
  atomic mode check.

### Regression and access-shape tests

- The none-mode admin cases in `TestBillingModePredicates`, plus
  `TestGetBusinessTierInfo_BillingModeNoneHonoursAdminLocks`
  (`billing_mode_predicates_test.go`).
- `TestProjectedLoadersHonourAdminClosureInBillingModeNone`, plus the
  projection column lists in `business_public_projection_test.go`
  (`is_active` and `closed_at` selected, still no `SELECT *`).
- `TestBillingNormalization_StructWritesDoNotPersistNormalisedValues` and
  `TestBillingWriteGuard_ExplicitColumnWritesPass`
  (`billing_normalize_test.go`).

### Method

Before is the tree at `247d8e799`, exported with `git archive` into a
scratch tree. After is the fix-round tree. Runs were interleaved
before / after.

```bash
cd backend
# Predicates: quiet window, single CPU, n=10.
env -u BILLING_MODE GO_ENV=test PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000 \
  go test ./internal/database -run '^$' -bench 'BillingPredicate' \
  -benchmem -cpu 1 -count=5
# Loaders: n=6 (count=3, twice).
env -u BILLING_MODE GO_ENV=test PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000 \
  go test ./internal/database -run '^$' \
  -bench 'BusinessLoadByID|GetBusinessAuthScope|GetBusinessByCustomURL|GetActiveTableWithBusinessByCode' \
  -benchmem -count=3
# Update chain (new benchmark, added to both trees): n=10 (count=5, twice).
env -u BILLING_MODE GO_ENV=test PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000 \
  go test ./internal/database -run '^$' -bench 'BusinessUpdateChain' \
  -benchmem -count=5
```

The predicate rerun ran at a load average of about 6 to 8. The first loader
run ran at about 15, so its ns/op intervals are wide (±100% to ±200%). The
B/op and allocs/op figures below are deterministic.

### Results

**Predicates** (sec/op medians; 0 B/op and 0 allocs/op on every row):

| Benchmark | Before | After | Delta |
|---|---|---|---|
| HasActiveSubscription none/active_ai_pro | 1.780 ns | 2.332 ns | +31% (p=0.000) |
| HasActiveSubscription none/expired_trial_core | 1.768 ns | 2.346 ns | +33% (p=0.000) |
| GetBusinessLockState none, all rows | 2.2 to 2.3 ns | 2.2 ns | ~ |
| GetBusinessLockState stripe, all rows | 35 to 41 ns | 34 to 39 ns | ~ |
| HasActiveSubscription stripe, all rows | 36 to 50 ns | 34 to 42 ns | ~ |

The +0.6 ns on the none-mode `HasActiveSubscription` rows is the two
field reads the admin check needs. The result is still about 15 times
cheaper than the stripe path.

**Loaders** (deterministic columns, n=6):

| Benchmark | B/op before | B/op after | allocs before | allocs after |
|---|---|---|---|---|
| GetBusinessAuthScope | 11.99 KiB | 12.20 KiB | 157 | 160 |
| GetBusinessByCustomURL | 34.69 KiB | 34.76 KiB | 493 | 496 |
| GetActiveTableWithBusinessByCode | 95.81 KiB | 95.92 KiB | 989 | 996 |
| BusinessLoadByID (both modes) | 29.14 KiB | 29.14 KiB | 616 | 616 |

The +3 / +3 / +7 allocs are the scanned `is_active`, `closed_at` and
`is_demo` columns. The lock decision cannot be made without them.

**Update chain** (n=10):

| Benchmark | sec/op before | sec/op after | B/op | allocs/op |
|---|---|---|---|---|
| stripe/business_struct_save | 109.1 µs ±10% | 123.0 µs ±33% | 109.8 KiB, = | 558, = |
| stripe/table_map_update | 13.27 µs ±5% | 15.19 µs ±113% | 10.33 KiB, = | 85, = |
| none/business_struct_save | 113.6 µs ±9% | 110.6 µs ±78% | 109.8 → 106.9 KiB (-2.6%) | 558 → 536 (-3.9%) |
| none/table_map_update | 27.39 µs ±57% | 13.87 µs ±7% | 10.33 KiB, = | 85, = |

- **Stripe rows.** B/op and allocs/op are identical before and after. The
  guard returns after one atomic load. The ns/op deltas on these rows sit
  inside intervals of ±33% to ±113% at load 7 to 8. Nothing new is
  allocated, so they are read as noise.
- **None-mode struct save.** It gets cheaper: 22 fewer allocs and 2.9 KiB
  less, because the guard drops the billing columns from the UPDATE
  statement.

### Not run

- No Postgres (Testcontainers) run, as in the first round.
- No quiet-machine rerun of the loader or update-chain ns/op columns.
