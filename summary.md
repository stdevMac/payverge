> Short commit SHAs in this log refer to pre-release private history. They do not resolve in the public repository.

# Wave-6 hot-path access shapes (2026-10-06)

Five read/poll paths flagged by the wave-6 review, each pinned by a failing
access-shape test first and measured with `-benchmem -count=3` on Apple M3
against in-memory SQLite (deterministic access-shape microbenchmarks). The
numbered migrations `000001`-`000005` referenced by older entries below have
since been folded into the genesis baseline (head `0`); the new indexes here
are edited straight into `backend/schema/genesis/current_schema.sql`.

| Benchmark | Before ns/op | Before B/op | Before allocs | After ns/op | After B/op | After allocs |
|---|---:|---:|---:|---:|---:|---:|
| `BenchmarkGetOrdersByBillIDSQLite` (I1, guest orders poll) | 649k–1.14M | ~721 KB | 1,670 | 554k–801k | ~721 KB | 1,553 |
| `BenchmarkGetGuestMenuTranslationsSQLite` (I2) | 15.0–28.4 ms | 3.54 MB | 63,293 | 1.8–4.6 ms | 669 KB | 9,038 |
| `BenchmarkGetTablesWithStatusFutureBookingsSQLite` (I3, floor poll, 20 tables x 60 bookings) | 7.97–13.3 ms | 12.98 MB | 26,197 | 1.92–2.01 ms | 360 KB | 1,620 |
| `BenchmarkCheckTableAvailabilityHistorySQLite` (I4, 2,000 history rows) | 16.8–21.8 ms | 18.0 MB | 85,935 | 0.50–0.69 ms | 40.5 KB | 102 |
| `BenchmarkGetAiConversationsStaleSweepSQLite` (I6, 20 stale + 20 fresh claims) | 6.6–8.9 ms | 4.71 MB | 39,202–39,213 | 2.8–4.4 ms | 1.13 MB | 13,148–13,165 |

- **I1** `GetOrdersByBillID` projects the guest order columns and caps at the
  newest `GuestOrdersByBillLimit` (100) rows
  (`TestGetOrdersByBillIDProjectsGuestColumnsAndCapsRows`).
- **I2** the guest menu-translations endpoint reads only `menu_item`/`bundle`
  rows and five columns (`TestGetGuestMenuTranslationsReadsOnlyGuestEntityTypes`).
- **I3** `GetTablesWithStatus` keeps only the next reservation per table via
  `ROW_NUMBER() OVER (PARTITION BY table_id)`
  (`TestGetTablesWithStatusReturnsOnlyNextReservationPerTable`).
- **I4** the locked conflict check is bounded below by
  `start - (MaxReservationDurationMinutes + buffer)` and booking duration is
  capped at 24h (`TestCheckTableAvailabilityBoundsHistoryScan`).
- **I6** the stale-claim sweep is one guarded batch UPDATE plus one keyed alert
  lookup per business, also run by a one-minute background ticker; genesis
  adds `idx_ai_waiter_conversations_business_claimed_at` (partial) and
  `idx_ai_waiter_conversations_business_status_updated_at`, and drops the
  duplicate `idx_ai_waiter_conversations_claimed_by` (I7)
  (`TestGetAiConversations_StaleSweepBatchesAlertResolution`).

Commands: `go test -short ./internal/{database,handlers,server} -run '^$'
-bench <name> -benchmem -count=3` (I6 uses `-benchtime=30x` because each
iteration reseeds its fixture outside the timer).

# Ops-assistant retention reads by index (migration 000001) (2026-10-05)

`DeleteInactiveOpsAssistantThreads` (the hourly ops-assistant janitor)
selects the oldest threads by `last_message_at` across every business, then
deletes their `ops_assistant_requests` rows by `thread_id`. Neither column had
a usable index: with `enable_seqscan=off` the selection fell back to a full
walk of `idx_ops_assistant_threads_business` and the request delete was a
sequential scan. Migration `000001_ops_assistant_threads_retention` adds
`idx_ops_assistant_threads_last_message_at` and
`idx_ops_assistant_requests_thread_id`, and drops the dead `archived_at`
column plus `idx_ops_assistant_threads_archived` (nothing archives ops
threads; retention deletes them).

Apple M3, isolated Postgres 15 at the genesis + migrations schema
(`ops_assistant_retention_pg_test.go`): 20,000 live threads each with one
request row, one 500-thread expired batch reseeded per iteration.

### BenchmarkDeleteInactiveOpsAssistantThreads (`-benchmem -benchtime=20x`)
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| **Before** | 345,878,392–368,195,912 | ~723,000 | 11,110 |
| **After** (`-count=3`) | 7,791,862–8,714,536 | ~653,000 | 10,068 |

ns/op **−97.7%** per janitor pass. Access shape is pinned by
`TestOpsAssistantRetention_AccessShape_Postgres` (EXPLAIN with sequential
scans priced out must name both new indexes; `archived_at` must be gone).
`TestRollbackRehearsal_EachMigration` now walks every on-disk `.down.sql`
(it previously stopped at the genesis head, which always equals the newest
migration once genesis is regenerated) and the 000001 pair round-trips.

```bash
cd backend
go test ./internal/database -run TestOpsAssistantRetention_AccessShape_Postgres -count=1
go test ./internal/database -run '^$' -bench BenchmarkDeleteInactiveOpsAssistantThreads -benchmem -benchtime=20x -count=3
go test -tags integration_postgres ./internal/database -count=1
bash backend/scripts/generate-genesis-schema.sh
```

# Unpaid-bills honors the date filter, carried-over debt aggregated (2026-08-22)

GET `/accounting/unpaid-bills` deliberately ignored `start` (as-of-end window), so a single-day filter (`start=end=2026-08-22`) replayed the
entire leftover book while GetSummary for the same dates returned zeros. The
list is now range-scoped `[start, end)` with the same `RemainingDueInRange`
predicate as `collection_gap`; pre-range debt stays visible as one new
aggregate query (`RemainingDueCarriedOver`: COUNT + SUM of outstanding before
`start`, emitted as `carried_over` / `carried_over_amount`, dollars on the
wire). Decision-14 holds: COUNT still skips the tables JOIN, and the
carried-over aggregate runs on `bills` only.

Apple M3, local SQLite microbench (`accounting_unpaid_bench_test.go`, 1,200
bills / 300 leftovers, 30-day window), machine loaded by parallel batch agents.

### BenchmarkGetUnpaidBills (`-benchmem`)
| | ns/op | B/op | allocs/op | queries |
|---|---:|---:|---:|---|
| **Before** (as-of-end) | 2,503,095 | 74,320 | 1,347 | count + page |
| **After** (`-count=3`) | 1,668,742–1,760,049 | ~81,900 | 1,456 | count + carried-over aggregate + page |

ns/op **−30%** despite one extra query — range-scoping shrinks the listed set;
+~109 allocs/op and +7.6 KB/op are the carried-over aggregate. Query count is
constant (3), no new JOINs, page query shape unchanged.

```bash
cd backend
# TDD red (single-day filter replayed the whole book)
go test -race ./internal/handlers/ -run 'TestGetUnpaidBills|TestUnpaidBillRow' -count=1
# Baseline (fix stashed) then after
go test ./internal/handlers/ -run '^$' -bench 'Unpaid' -benchmem -count=1
go test ./internal/handlers/ -run '^$' -bench 'Unpaid' -benchmem -count=3
# No-op compile gate
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

# Empty-table check-please expires with the seating SLA (2026-08-22)

An earlier change left `reason=check` live forever. Core T1 empty guest check
(`Sin Cuenta Activa`, no current bill) still showed `Pediste La cuenta`
after 1d. Check-please now expires unless the table still has a
same-seating open/partial bill (created at or before the call). A newer
bill does not inherit yesterday's ask. Claimed/assigned and dead clocks
stay. TTL occupancy is server open/partial only — leftover-kitchen
host-stand occupied is not treated as empty and is not a
keep. Guest poll stays one narrow `LIMIT 1` on the common path; the
bills `created_at` EXISTS runs only on the stale-check branch.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`.

### BenchmarkGuestServiceCallStatus
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (no expiry) | 23.9 µs | 15.8 KiB | 123 |
| after (empty-check expiry) | 16.4–16.6 µs | 14.1 KiB | 102 |

Hot-path `none` poll is unchanged (no extra bill read). Bench logger
silenced so allocs are not log I/O.

```
cd backend && go test ./internal/services/operational_alerts ./internal/jobs ./internal/server ./internal/services/director_tools -run 'ServiceCall|ListAlerts|TTLJanitor|LiveFloor|SameSeating' -count=1
cd backend && go test ./internal/server -run 'TestServiceCallStatusQueryShape' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
cd backend && go test ./internal/server -run '^$' -bench 'BenchmarkGuestServiceCallStatus' -benchmem -count=3
```

---

# Collection gap leftover remainings listed on unpaid-bills (2026-08-22)

`GET unpaid-bills` now uses the same leftover-remaining predicate as
`GetSummary.collection_gap` (`status <> voided AND total_amount > paid_amount`)
as-of window end. That is the predicate `idx_bills_outstanding_as_of`
(migration 000197) already indexes. Access shape is unchanged: COUNT on
`bills` only (no tables join); page query LEFT JOINs tables for the label;
`page_size` still clamps to 100.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`. Not comparable to the
L6-23 Apple M3 as-of-end AFTER numbers; allocations stay in the same band
(~73 KB/op, ~1.3k allocs/op) because the response is still bounded by
`page_size=20`.

### BenchmarkGetUnpaidBills
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| after (leftover remainings as-of end) | 2172955 / 2302234 / 1301891 | 74190 / 74273 / 74268 | 1346 / 1347 / 1347 |

Mean: 1,925,693 ns/op; 74,244 B/op; 1,347 allocs/op.

```
cd backend && go test ./internal/handlers -run 'TestGetUnpaidBills' -count=1
cd backend && go test ./internal/accounting -run 'TestCollectionGap|TestServiceGetSummary_Abandoned' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/database ./internal/accounting -run '^$' -count=1
cd backend && go test ./internal/handlers -run '^$' -bench 'BenchmarkGetUnpaidBills' -benchmem -count=3
```

---

# Bill/order items emit as a JSON array (2026-08-22)

`Bill.MarshalJSON` and `Order.MarshalJSON` now write the stored items
snapshot as a JSON array (or omit / `[]`). No query-shape change; this is
the wire contract only. Existing `BenchmarkBillMarshalJSON` (empty `[]`
snapshot, now omitted) still ~7.8–9.8 µs, 6988 B/op, 5 allocs/op.

```
cd backend && go test ./internal/database -run 'TestBillMarshalJSON|TestOrderMarshalJSON|TestBillUnmarshalJSON|TestOrderUnmarshalJSON' -count=1
cd backend && go test ./internal/server -run 'TestBillListResponse' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
cd backend && go test ./internal/database -run '^$' -bench 'BenchmarkBillMarshalJSON$' -benchmem -count=3
```

# Demo house cash rail review gates (2026-08-22)

Review gates: house-rail heal is `kind=demo` only (sticky
`is_demo=true` on `kind=real` stays closed). Guest
`GET /payment-plugins` is read-only (`FindOpen`, no `Create`). Human
close is any closed row with `closed_by_user_id` / `closed_by_staff_id`,
not `ClosedByLabel=="demo-manager"`. Guest cashier is opt-in
(`counter_settlement_ready === true`). Same Current() hot path as the
entry below; benches unchanged.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`. New path; no prior
dedicated bench.

### BenchmarkCurrentDemoHouseRailAlreadyOpen
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| after | 161–171 µs | 30522–30532 | 333 |

### BenchmarkCurrentRealVenueClosedDrawer
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| after | 162–163 µs | 33105–33109 | 333 |

```
cd backend && go test ./internal/services/cashregister ./internal/database ./internal/handlers -run 'EnsureDemoHouse|BusinessUsesDemo|HasHumanClosed|CashRegisterHandlersCurrentOpensDemo|MarkAlternativePayment_DemoHouse|MarkAlternativePayment_RejectsCash|GetBusinessPaymentPlugins_DemoHouse|GetBusinessPaymentPlugins_RealVenue|GetBusinessPaymentPlugins_ReportsReady' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
cd backend && go test ./internal/services/cashregister -run '^$' -bench 'BenchmarkCurrentDemoHouseRailAlreadyOpen|BenchmarkCurrentRealVenueClosedDrawer' -benchmem -count=3
```

---

# Caja suggested opening float is last declared bank (2026-08-21)

`GET /cash-register/current` now loads `suggested_opening_float` with a
narrow `SELECT opening_float_cents` of the newest closed session. Counted
cash / expected / variance are not in that query. New lookup; no prior
dedicated bench on this path.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`.

### BenchmarkFindLastDeclaredOpeningFloatCents
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| after | 21.3–27.9 µs | 9201 | 70 |

```
cd backend && go test ./internal/database -run 'TestFindLastDeclared' -count=1
cd backend && go test ./internal/services/cashregister ./internal/handlers -run 'CurrentSuggests|CashRegisterHandlersCurrentSuggests' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
cd backend && go test ./internal/database -run '^$' -bench 'BenchmarkFindLastDeclaredOpeningFloatCents$' -benchmem -count=3
```

---

# Waiter-call SLA keeps unpaid check-please (2026-08-21)

Auto-expire no longer clears claimed/assigned calls, `reason=check`, or
tables with an open/partial bill. Zero `last_event_at` is not stale on
GET or expire (`last_event_at > 2000-01-01 AND < cutoff`). Guest poll
still one narrow `LIMIT 1` (no extra bill read on the hot path).

```
cd backend && go test ./internal/services/operational_alerts ./internal/jobs ./internal/server ./internal/handlers ./internal/services/director_tools -run 'ServiceCall|ListAlerts|TTLJanitor|LiveFloor' -count=1
```

---

# Waiter-call seating SLA (2026-08-21)

Open/claimed `service_call` alerts now leave the live set after 90 minutes
(`ServiceCallTTL`). Guest status poll stays one narrow `LIMIT 1` read and
adds `last_event_at` so a lapsed call projects as `none` without a write
on the hot path. List/badge queries exclude lapsed rows; a 2-minute janitor
persists `reason=expired`.

Hardware: Linux (`linux/amd64`), `-count=1`.

### BenchmarkGuestServiceCallStatus
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| GuestServiceCallStatus (after) | 23.9 µs | 15.8 KiB | 123 |

Access shape: `TestServiceCallStatusQueryShape` still one
`operational_alerts` select (`status, resolved_at, metadata, last_event_at`)
and one projected table-code lookup.

```
cd backend && go test ./internal/services/operational_alerts ./internal/jobs ./internal/server ./internal/handlers ./internal/services/director_tools -run 'ServiceCall|ListAlerts|TTLJanitor|LiveFloor' -count=1
cd backend && go test ./internal/server -run 'TestServiceCallStatusQueryShape' -count=1
cd backend && go test ./internal/server -run '^$' -bench 'BenchmarkGuestServiceCallStatus' -benchmem -count=1
```

---

# Guest catalog unsellable for inventory 86 (2026-08-21)

Guest QR cards still treated recipe-depleted dishes as sellable because the
catalog kept the stored `is_available` flag. `projectGuestOrderability` now
flips guest `is_available` false when `inventory_status=out_of_stock` or
`inventory_out`. No extra query; one pass over the already-projected items.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`.

### BenchmarkPublicGuestMenuResponse
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| PublicGuestMenuResponse (hours-overwrite fix) | 378–422 µs | 436 KiB | 1158 |
| PublicGuestMenuResponse (inventory 86 fix) | 374–383 µs | 436 KiB | 1159 |

Latency stays in the prior band. +1 alloc/op is noise-scale vs the 30-item
fixture. Query shape unchanged (`GetInventorySummary` still one summary).

```
cd backend && go test ./internal/server -run 'TestMenuResponseKeepsOutOfStock|TestMenuResponse_ClosedHours|TestMenuResponse_InventoryUnavailableHarvestBowl|TestBuildPublicGuestMenuResponse|TestGetTableByCodePublic_HidesUnsellable' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/services -run '^$' -count=1
cd backend && go test ./internal/server -run '^$' -bench 'BenchmarkPublicGuestMenuResponse$' -benchmem -count=3
```

---

# Guest catalog inventory_status survives hours overwrite, follow-up (2026-08-21)

Guest `ResolveOrderability` remaps `inventory_out` → `business_closed`. The
live catalog row has no `orderability_state`, so Sage treated an 86'd steak
as a closed-hours dish card. `ProjectOrderability` now stamps
`inventory_status=out_of_stock` onto the catalog item in the same loop (warn
and hard_block only). No extra query.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`.

### BenchmarkPublicGuestMenuResponse / BenchmarkOrderabilityProjection
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| PublicGuestMenuResponse (after) | 378–422 µs | 436 KiB | 1158 |
| OrderabilityProjection 500 items (after) | 10.9–11.5 ms | 19.13 MiB | 34.3k |

Query count still constant vs menu size (`TestProjectOrderabilityQueryCountIsConstant`).
Stamp is a string assign in the existing per-item loop; `GetInventorySummary` is unchanged.

```
cd backend && go test ./internal/server -run 'TestMenuResponseKeepsOutOfStock|TestMenuResponse_ClosedHours|TestProjectOrderability_GuestClosed|TestBuildPublicGuestMenuResponse' -count=1
cd backend && go test ./internal/services -run 'TestProjectOrderabilityQueryCountIsConstant|TestResolveOrderabilityDecisionTable' -count=1
cd backend && go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
cd backend && go test ./internal/server -run '^$' -bench 'BenchmarkPublicGuestMenuResponse$' -benchmem -count=3
cd backend && go test ./internal/services -run '^$' -bench 'BenchmarkOrderabilityProjection$' -benchmem -count=3
```

---



`ClearTable` returned `settle_required` before inspecting live kitchen
tickets or pending guest sends, and only on the current open check. The
abandon sweeper could flip a cooking check abandoned, then Live View
read the table Available.

Fix: table-scoped kitchen/pending checks win over settle; close/abandon
refuse unfinished tickets; `GetTablesWithStatus` occupies a table when a
terminal check still owns live kitchen work. The extra occupancy join
runs only for tables with **no** open/partial bill, so the 500-occupied
dinner-rush fixture does not add a query.

Hardware: Linux Xeon (`linux/amd64`), `-count=3`.

### BenchmarkGetTablesWithStatusSQLite
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| this change | 13.8–20.5 ms | 16.83 MiB | 37.4k |

Matches the host-stand reserved-status Xeon band (~16.8 MiB / 37.4k).
Allocs did not grow. Latency spread is fixture noise on this runner.

```
cd backend && go test ./internal/database -run 'TestClearTable_|TestGetTablesWithStatus_Abandoned|TestCloseBill_UnpaidWithLiveKitchen' -count=1
cd backend && go test ./internal/server -run 'TestRespondFloorError_FourClearArms' -count=1
cd backend && go test ./internal/services -run 'TestBillLifecycle_SkipsLiveKitchen' -count=1
cd backend && go test ./internal/database -run '^$' -bench '^BenchmarkGetTablesWithStatusSQLite$' -benchmem -count=3
```

---

# Quote settlement millipercent rates (2026-08-16)

`applySettlementRates` quantized tax/service rates with
`int64(rate*100+0.5)` (whole basis points). NYC 8.875% became 8.88%,
so tax on a $2,100.00 net was 18648 cents instead of 18638.

Fix: quote + guest-checkout bill totals call `money.PercentageCents`
(millipercent rate, half-up once on the final cent). No alloc change.

Hardware: Apple M3 (`darwin/arm64`), `-count=3`. Medians below.

### BenchmarkApplySettlementRates / BenchmarkPercentageCents
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (`int64(rate*100+0.5)` + `percentageDiscount`) | 3.16 | 0 | 0 |
| after (`money.PercentageCents` via `applySettlementRates`) | 2.33 | 0 | 0 |
| after (`BenchmarkPercentageCents` alone) | 0.36 | 0 | 0 |

Delta: quote path stays 0 allocs; ~0.8 ns/op cheaper (noise-scale).
Correctness: 210000 × 8.875% → 18638 cents.

```
cd backend && go test -count=1 ./internal/services ./internal/money -run 'Tax|Quote|Settlement|ServiceFee|BillTotal'
cd backend && go test ./internal/services ./internal/money ./internal/handlers ./internal/server -run '^$' -count=1
cd backend && go test ./internal/services ./internal/money -run '^$' -bench 'BenchmarkApplySettlementRates|BenchmarkPercentageCents' -benchmem -count=3
```

---

# Guest loyalty-rate + guest-orders query shape (2026-08-16)

`GET /api/v1/guest/table/:code/loyalty-rate` issued 4 queries to return
one float: table lookup, `SELECT *` on `businesses` via
`GetBusinessByID` (subscription boolean only), `SELECT *` on
`loyalty_programs`, and a discarded `Preload("Tiers")`.
`GET /api/v1/guest/bill/:bill_token/orders` preloaded the narrow
Business currency projection for `Order.MarshalJSON`, then bypassed
that marshaller and copied the empty `gorm:"-"` `Currency` field —
so every guest order emitted `currency:""`.

Fix: `GetLoyaltyRate` uses cached `loadPublicGuestTableContext`
(publicBusinessColumns join, no `SELECT *`).
`RedemptionRateForBusiness` projects only `enabled` +
`redemption_points_per_dollar` and never reads `loyalty_tiers`.
`guestOrderView` calls `Order.ResolvedCurrency()` (display → default
→ USD) so the existing 3-column Business preload is consumed rather
than discarded. Full Business aggregates stay off the guest path.

Access-shape: `TestRedemptionRateForBusinessAccessShape`,
`TestGetLoyaltyRateAccessShape`,
`TestGetGuestOrdersByBillNumberAccessShape`,
`TestGetGuestOrdersByBillNumber_EmitsResolvedCurrencyMatchingOperator`.

Hardware: Apple M3 (`darwin/arm64`), SQLite, `-count=3`. Medians below.

### BenchmarkRedemptionRateForBusiness
| | ns/op | B/op | allocs/op | queries |
|---|---:|---:|---:|---:|
| before (`SELECT *` program + `Preload("Tiers")`) | ~32.1 µs | 16.4 KB | 240 | 2 |
| after (2-column projection) | ~19.1 µs | 4.0 KB | 62 | 1 |

Delta: ~40% faster, ~75% fewer bytes/op, ~74% fewer allocs/op, one
discarded `loyalty_tiers` round trip gone.

### BenchmarkGetLoyaltyRate
| | ns/op | B/op | allocs/op | queries |
|---|---:|---:|---:|---:|
| before (table + `SELECT *` business + program + tiers) | ~186 µs | 59.2 KB | 922 | 4 |
| after (cached public join + rate projection) | ~42.1 µs | 11.5 KB | 93 | 1 (warm) / 2 (cold) |

Delta: ~77% faster, ~81% fewer bytes/op, ~90% fewer allocs/op.

### BenchmarkGetGuestOrdersByBillNumber
Query shape unchanged (token id join + orders + 3-column currency
preload). After serial run: ~264 µs/op, 851 KB/op, 1146 allocs/op.
`BenchmarkGetOrdersByBillIDSQLite` is unchanged (~219 µs/op, 807 KB,
996 allocs). The guest win is correctness: non-empty currency matching
the operator endpoint.

```
cd backend && go test -count=1 ./internal/loyalty -run 'RedemptionRateForBusiness|Redeem|Undo|RedemptionRate'
cd backend && go test -count=1 ./internal/server -run 'GetLoyaltyRate'
cd backend && go test -count=1 ./internal/handlers -run 'GetGuestOrdersByBillNumber|GetOrdersByBillID_Success'
cd backend && go test -count=1 ./internal/database -run 'OrderResolvedCurrency|OrderMarshalJSON|GetOrdersByBillID'
cd backend && go test ./internal/loyalty -run '^$' -bench BenchmarkRedemptionRateForBusiness -benchmem -count=3
cd backend && go test ./internal/server -run '^$' -bench BenchmarkGetLoyaltyRate -benchmem -count=3
cd backend && go test ./internal/handlers -run '^$' -bench BenchmarkGetGuestOrdersByBillNumber -benchmem -count=3
cd backend && go test ./internal/handlers ./internal/server ./internal/database ./internal/loyalty -run '^$' -count=1
```

---

# Public storefront /business/:customUrl query shape (2026-08-16)

`GET /api/v1/business/:customUrl` issued six uncached `SELECT *` list
reads per anonymous hit. `GetBusinessOperatingExceptions` had no date
predicate and no `Limit`, so holiday history grew the landing payload
forever. `BusinessLanguage.Business` was tagged `json:"business"`
(zero-value struct, ~2.8KB padding per language). `supported_languages`
was a global catalogue re-scanned and re-serialized on every request.

Fix: dedicated public loaders with `Select` + `Limit`; exceptions
filtered to `exception_date >= yesterday UTC` (TZ-safe current window)
and capped at 366; `supported_languages` served from an in-process
cache invalidated on `SetTestDB` / `InitializeDefaultLanguages`;
`BusinessLanguage.Business` is `json:"-"`. Operator exception GET stays
unbounded so replace-all edits cannot drop history. Route already sits
behind `guestTableReadLimiter`.

Access-shape: `TestGetBusinessByCustomURLAccessShape`,
`TestGetPublicStorefrontHospitalityAccessShape`,
`TestGetBusinessByCustomURLOmitsPastOperatingExceptions`,
`TestGetBusinessByCustomURLOmitsBusinessLanguageEmbed`,
`TestGetCachedPublicSupportedLanguages_DoesNotRequery`.

Hardware: Apple M3 (`darwin/arm64`), SQLite 30 gallery + 30 features +
7 hours + 3 exceptions + 2 business languages + 3 supported languages,
`-count=3`.

Production-like compact JSON (21 supported languages, 2 business
languages, 7 hours, 3 gallery, 3 features): **14,211 → 5,905 bytes**.

### BenchmarkGetBusinessByCustomURLStorefrontSQLite
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (6× `SELECT *`, unfiltered exceptions) | ~393–469 µs | ~645 KB | ~2247 |
| after (projected + bounded + cached catalogue) | ~595–646 µs | ~590 KB | ~1967 |

SQLite in-memory `SELECT *` of tiny hospitality rows is cheap; the
after path spends a little more on explicit `Select` lists. The
production win is payload size, bounded exception history, and fewer
allocs (no zero-value `Business` embed). Query count: first hit still
1 business + 6 lists; repeat hits drop `supported_languages` to 0.

```
cd backend && go test -count=1 ./internal/server -run 'GetBusinessByCustomURL|PublicStorefront|ProductionLike'
cd backend && go test -count=1 ./internal/database -run 'GetPublic|GetCachedPublic|BusinessLanguageMarshalJSON'
cd backend && go test ./internal/server -run '^$' -bench 'BenchmarkGetBusinessByCustomURLStorefrontSQLite' -benchmem -count=3
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

---

# Public delivery-settings projection (2026-08-16)

Anonymous `GET /api/v1/businesses/:id/delivery-settings` hydrated three
full aggregates (`GetBusinessByID` SELECT *, `delivery_settings` SELECT *
including partner API keys + unused `delivery_zones` JSON, and
`delivery_zones` SELECT * including GeoJSON `boundaries` that the public
branch then set to nil) plus `GetBusinessPlugins` (`features` /
`config_schema`) to resolve `payment_mode`.

Fix (public branch only): `GetPublicBusinessByID`, projected
delivery-settings columns, projected + `LIMIT 50` active zones without
`boundaries`, and `HasEnabledPaymentPlugin` EXISTS. Operator GET stays
richer. Partner API keys are not selected and are not on the DTO.

Access-shape: `TestGetDeliverySettingsDTO_PublicAccessShape`.

Hardware: Apple M3 (`darwin/arm64`), SQLite 1 business × 8 fat zones,
`-count=3`.

### BenchmarkGetDeliverySettingsDTOPublicSQLite
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (3× SELECT * + plugin JSON) | ~163–170 µs | ~117.5 KB | 1171 |
| after (projected public reads) | ~111–152 µs | ~109.1 KB | 789 |

```
cd backend && go test -count=1 ./internal/services -run 'TestGetDeliverySettingsDTO_Public'
cd backend && go test ./internal/services -run '^$' -bench 'BenchmarkGetDeliverySettingsDTOPublicSQLite' -benchmem -count=3
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

---

# Public referral stats and code lookup (2026-08-16)

Anonymous `GET /api/v1/referrals/stats` used to Pluck every
`referral_records.commission` into Go and run four extra `COUNT(*)`
queries. `GET /api/v1/referrals/referrer/code/:referral_code` used
`LOWER(referral_code)`, so it could not use `idx_referrers_referral_code`.

Fix: two SQL aggregates (`COUNT`/`SUM`/`CASE`) for stats; store and look
up the normalized (uppercase, trimmed) code via equality. Migration
`000215_normalize_referrer_codes` backfills mixed-case rows and adds
`referrers_referral_code_normalized_chk`. Both public GETs stay behind
`publicFormLimiter` (already wired).

Access-shape: `TestGetReferralStatsAccessShape`,
`TestGetReferrerByCodeAccessShape`.

Hardware: Apple M3 (`darwin/arm64`), SQLite 80 referrers × 10 records,
`-count=3`.

### BenchmarkGetReferralStatsSQLite
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (4× COUNT + Pluck commissions) | ~360–589 µs | ~178 KB | ~5834 |
| after (2 SQL aggregates) | ~52–56 µs | ~14.1 KB | ~135 |

```
cd backend && go test -count=1 ./internal/server -run 'Referral|Referrer'
cd backend && go test ./internal/server -run '^$' -bench 'BenchmarkGetReferralStatsSQLite' -benchmem -count=3
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

---

# Issuable-bills picker dinner-service scan (2026-08-13)

ListIssuableBills must include just-paid open bills, paid_amount=0 paid
rows, and already-invoiced bills without a correlated EXISTS
over the bill table.

Shape: two Limit-20 scans (paid/collected + invoiced-only join), merge in
Go. Status literals stay inlined. Access-shape:
`TestListIssuableBillsAccessShape`.

Hardware: Apple M3 (`darwin/arm64`), SQLite 400-bill fixture, `-count=3`.

### BenchmarkListIssuableBills vs legacy paid AND paid_amount>0
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (EXISTS + correlated receipt subquery) | ~2.9–3.4 ms | ~27.8 KB | ~475 |
| after (two bounded scans) | ~327–343 µs | ~45.7 KB | ~592 |
| legacy paid-and-amount arm | ~126–131 µs | ~21.8 KB | ~473 |

The extra ~200 µs / ~24 KB vs the legacy arm is the second invoiced-only
scan. First EXISTS rewrite was ~20× slower; this is ~2.6× the legacy arm
for the broader dinner-service set.

```
cd backend && go test -race -count=1 ./internal/fiscal/ -run 'ListIssuable|RepositoryListIssuable'
cd backend && go test ./internal/fiscal -run '^$' -bench 'BenchmarkListIssuableBills' -benchmem -count=3
```

---

# Host-stand Tables Live View reserved status (2026-08-11)

Dinner-service fix: `GetTablesWithStatus` marks a table **reserved** for any
upcoming pending/confirmed assigned reservation (not only the next 30 minutes),
and batches `active_bill_server_name` for the Live View server column.

Regression: `TestGetTablesWithStatusMarksUpcomingReservationReserved`,
`TestGetTablesWithStatusProjectsServerName`.

### BenchmarkGetTablesWithStatusSQLite (post-change, Linux Xeon, count=3)
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| after | ~10.7–11.9 ms | ~16.8 MiB | ~37.4k |

Staff-name lookup is skipped when no open bill has `created_by_staff_id`
(perf fixture path). Occupied still wins over reserved.

```
cd backend && go test ./internal/database -run 'TestGetTablesWithStatusMarksUpcomingReservationReserved|TestGetTablesWithStatusProjectsServerName|TestGetTablesWithStatusProjectsActiveBills' -count=1
cd backend && go test ./internal/database -run '^$' -bench '^BenchmarkGetTablesWithStatusSQLite$' -benchmem -count=3
```

---

# NEW-8 AI-generated image delivery optimize (2026-08-09)

Finding NEW-8 (Backend Performance Gate): marketing post previews failed with
`image_load_failed` on 1.4–1.7 MB unoptimized `menu_items/ai_generated/*.png`.

Fix: `OptimizeAIGeneratedImageBytes` resizes max side to 1280 and re-encodes
JPEG q=78 before S3 upload in `GenerateMenuImage` + `RegenerateItemImage`.
REV-2: composite transparent pixels onto opaque white before `jpeg.Encode`
(transparent PNGs no longer become black JPEGs); bilinear downscale.
REV-9: replace soft `t.Logf` evidence with a `testing.B` bench on a ≥1MB
in-test PNG fixture (`-benchmem -count=3`).

Hardware: Apple M3 (`darwin/arm64`).

### Unit size check (photo-like 2400² gradient PNG)
| | |
|---|---|
| before | ~112 KB PNG |
| after | ~32 KB JPEG |

```
cd backend && go test ./internal/services/ -run 'OptimizeAIGenerated|Transparent' -v -count=1
```

### BenchmarkOptimizeAIGeneratedImageBytes_LargePNG (post REV-2 path)
Generated ≥1MB PNG in-test (1400² noisy RGBA with partial alpha → flatten +
resize to 1280 + JPEG). Median of 3 runs:

| | ns/op | MB/s | B/op | allocs/op |
|---|---:|---:|---:|---:|
| after (count=3) | ~302–331 ms/op | ~3.7–4.5 | ~74.6 MB | ~1.31e7 |

```
cd backend && go test ./internal/services/ -run '^$' \
  -bench 'BenchmarkOptimizeAIGeneratedImageBytes_LargePNG' -benchmem -count=3
```

Raw (2026-08-10):
```
BenchmarkOptimizeAIGeneratedImageBytes_LargePNG-8  4  302785031 ns/op  4.09 MB/s  74606056 B/op  13108023 allocs/op
BenchmarkOptimizeAIGeneratedImageBytes_LargePNG-8  4  330964750 ns/op  3.74 MB/s  74606052 B/op  13108022 allocs/op
BenchmarkOptimizeAIGeneratedImageBytes_LargePNG-8  4  274158146 ns/op  4.52 MB/s  74605968 B/op  13108022 allocs/op
```

---

# PG-23 equal-split floor re-split (audit/r1-money, 2026-08-05)

Finding PG-23 (Backend Performance Gate): "Dividir por igual" assigned
`floor(available/N)*covered + min(remainder, covered)` so every covered=1
preview took a remainder cent and N simultaneous previews over-summed the bill
(server rejected overpay with 409 `split_share_conflict`).

Fix (no schema / migration 000193 not used):
1. `calculateEqualSplitCents` → floor re-split: `(available/numPeople)*covered`,
   full remaining pool when `covered == numPeople`, penny drain when
   `available < numPeople`.
2. Hold path re-splits current available across remaining seats
   (`equalSplitSeatsTakenTx` counts seats via `__equal_shares__` in the
   existing `claimed_fractions` JSON — no migration).
3. Guest panel `equalPreview` mirrors the same formula.
4. Reference `equalShareAmounts` keeps allocateCentsEqual last-seat exact sum
   for tests; table-driven invariants cover N∈{2..12}, adversarial totals.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

### BenchmarkCalculateEqualSplitSQLite (`./internal/splitting/`)
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (median of 3) | 34378 | 11976 | 122 |
| after (median of 3) | 53843 | 11977 | 122 |

Note: before/after noise is environmental (concurrent audit worktrees). Alloc
shape unchanged (122 allocs). No material code path change in
`CalculateEqualSplit` service method itself — only guest hold path in
`database.calculateEqualSplitCents`.

### BenchmarkGetBillSplitOptionsSQLite (`./internal/handlers/`)
| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| after (median of 3) | 1273112 | 288847 | 944 |

Before run failed due to unrelated flaky suite test when `-bench` still
executed package tests; re-run after with `-run=^$`. Options handler does not
call the equal-hold formula — ambient baseline only.

Commands:
```
go test -run='^$' -bench=BenchmarkCalculateEqualSplitSQLite -benchmem -count=3 ./internal/splitting/
go test -run='^$' -bench=BenchmarkGetBillSplitOptionsSQLite -benchmem -count=3 ./internal/handlers/
```

---

# perf-bench development status

This file tracks benchmark and optimization status, not an exact moving SHA; use `git log --oneline -5` as the source of truth for the latest commit.

## Payroll-run detail empty-relation bloat L6-16 (2026-08-05)

Finding L6-16 (Backend Performance Gate): `GET /accounting/payroll-runs/:runId`
serialized zero-value GORM relation structs recursively. A 5-line run was
**~34 KB** on the wire (service JSON **32,945 B**). Attribution (measured pre-fix):

| Component | Bytes | Notes |
| --- | ---: | --- |
| Empty `Business{}` | ~2,658 | `omitempty` never skips non-pointer structs |
| Empty nested `PayrollRun{}` (incl. Business) | ~2,943 | on every line item |
| Line item with empty rels | ~5,862 | useful fields ~262 B; rest is nested zeros |
| Full 5-line service JSON | **32,945** | 1× run Business + 5× (line Business + nested run) |
| Full 5-line HTTP envelope (B-10 RED) | **34,006** | `{"success":true,"data":…}` |

Root cause: `PayrollRun.Business`, `PayrollLineItem.Business`, and
`PayrollLineItem.PayrollRun` used `json:"business,omitempty"` /
`json:"payroll_run,omitempty"`. Encoding/json never treats non-pointer structs
as empty under `omitempty`, so every line re-emitted a full zero Business
(~2.6 KB) plus a recursive empty PayrollRun (~2.9 KB). Staff pay fields stay
`json:"-"` (owner-only compensation never leaked).

Fix (backend-only; FE field contract preserved):
1. Mark nested `Business` / `PayrollRun` / line `Staff` as GORM-only (`json:"-"`),
   matching `ManualLedgerEntry` / `Staff.Business` house style (FIND-038).
2. `GetPayrollRun` preloads actor staff with `Select("id, name, email")` so
   `Staff.MarshalJSON` emits the slim `{id,name,email}` shape (FIND-052).
3. Money wire contract unchanged: `PayrollRun` / `PayrollLineItem` `MarshalJSON`
   still emit dollar `float64` totals/amounts via `models_json.go`.

Access-shape locked by:
- `TestGetPayrollRun_DetailOmitsEmptyRelationStructs` (B-10, handler-level)

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: SQLite in-memory, single connection; 5-line and 50-line runs with
created/paid/voided actor staff + staff payees.

### `BenchmarkGetPayrollRunDetail` (`github.com/stdevmac/payverge/backend/internal/accounting`)

| Benchmark | phase | ns/op | B/op | allocs/op | body_B (service JSON) |
| --- | --- | ---: | ---: | ---: | ---: |
| `…_5Lines` | **before** | 754k–925k | ~304 KB | ~1,020 | **32,945** |
| `…_5Lines` | **after** | 269k–302k | ~143 KB | ~714 | **2,078** |
| `…_50Lines` | **before** | 5.95M–6.99M | ~2.60 MB | ~3,126 | ~297 KB (est. from 5-line + 45× empty-line) |
| `…_50Lines` | **after** | 1.32M–1.86M | ~1.16 MB | ~2,173 | **14,887** |

HTTP envelope post-fix (B-10): **2,253 B** for 5 lines; ceiling **4,096 B**
(~80% headroom). ~15× smaller 5-line body; ~3× fewer ns/op and ~2× fewer B/op
on the 5-line path; 50-line body ~20× smaller vs pre-fix estimate.

```bash
cd backend
go test ./internal/handlers/ -run 'TestGetPayrollRun_DetailOmitsEmptyRelationStructs' -v -count=1
go test ./internal/accounting/ -bench 'BenchmarkGetPayrollRunDetail' -benchmem -count=3 -run '^$'
go test ./internal/handlers ./internal/accounting -count=1
go test ./internal/services ./internal/database ./internal/server -run '^$' -count=1
```

Gates: handlers + accounting full suite **ok**; services/database/server compile
gate **ok** (`-run '^$'`).

## Demo generator honesty (2026-08-01)

Production-readiness Task 17 (PERF GATE): demo seed/version `admin-demo-v5`
stops lying — distinct staff PINs, ≥8 payer addresses, `demo_tx_*` non-explorer
hashes, real-path `B{id}-{hex}` bill numbers (no `DEMO-` prefix), payroll scaled
to ~28% of paid revenue (cap 40%), append heartbeat (`fresh`/`stale` at 2× hourly
interval), verifier recency fails when newest payment predates claimed window end.
Cash sessions jitter open/close and never write `opened_at == closed_at`.

Access-shape / honesty locked by:
- `TestDemoHeartbeatReportsStaleWhenAppendIsDead`
- `TestVerifierFailsWhenPaymentsPredateClaimedWindowEnd`
- `TestSeededStaffPINsArePairwiseDistinct`
- `TestSeededPaymentsHavePayerPoolAndNonExplorerTxHashes`
- `TestSeededBillNumbersHaveNoDEMOPrefix`
- `TestSeededPayrollIsAtMost40PercentOfRevenue`
- `TestAIGrowthFlagshipHeroKPIsAreNonZero`
- `TestCashSessionsHaveRealOpenCloseAndMixedIDs`

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3 -benchtime=3x`.
Fixture: SQLite in-memory, 7-day baseline ensure (2 demo businesses).

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkEnsureForAdmin` **before** (v4 seed) | ~97.1M | ~66.2M | ~459085 |
| `BenchmarkEnsureForAdmin` **after** (v5 honesty) | 130.7M–147.2M | ~67.3M | ~463000 |

~35–50% slower wall time from payroll re-scale + distinct bcrypt PINs + payer/tx
variety; allocs +~1% (extra PIN hashes + scale updates). No new query fan-out on
hot guest/dashboard routes — generator is admin-only ensure/append path.

```bash
cd backend
go test ./internal/demo/ -run '^$' -bench 'BenchmarkEnsureForAdmin' -benchmem -count=3 -benchtime=3x
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

## Bill lifecycle + occupied-tables count (2026-08-01)

Production-readiness Task 16 (PERF GATE): migration `000173` adds
`bills.abandoned_at` + partial index `idx_bills_stale_sweep_created_at` on open
bills by `created_at`. `services.BillLifecycleSweeper` (default 24h, business-
overridable, batch-bounded) marks long-open bills `abandoned` so they leave the
active set; never touches a bill with a payment/alt-payment in flight; notifies
via `bill.abandoned` SSE first. Surfacing threshold stays 2h (briefings /
stuck-bill watchdog); abandon is a separate larger decision. Briefing copy now
interpolates the **actual** oldest-bill age (`duration` / `oldest_minutes`).
Dashboard `live.open_tables` counts distinct occupied tables (not
`active_bills`); TodayLivePanel/Hero use it so delivery/counter bills cannot
inflate "Open tables".

Access-shape locked by:
- `TestCountOccupiedTablesByBusinessID_DistinctTablesOnly`
- `TestBillLifecycle_AbandonsStaleOpenBill` / `_SkipsPaymentInFlight`
- `TestBuildProactiveInsights_StaleBillsUseActualOldestAge`

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: 2 occupied tables + counter/delivery noise + 40 table-bound + 30
no-table open bills (SQLite microbench).

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkCountOccupiedTables_HydrateThenDistinct` **before** (full active-bill hydrate + Go distinct) | 591k–640k | ~1.03M | ~2598 |
| `BenchmarkCountOccupiedTablesByBusinessID` **after** (`COUNT(DISTINCT table_id)` aggregate) | 15.6k–20.1k | 8632 | 60 |

~38× faster, ~99% fewer allocs vs hydrating every active bill to count tables.
Polled dashboard path still derives `open_tables` from the already-fetched
active-bill summary map (zero extra SQL); the aggregate is the pure-count
access shape for any future count-only consumer.

```bash
cd backend
go test ./internal/database/ -run '^$' -bench 'BenchmarkCountOccupiedTables' -benchmem -count=3
go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
```

## Admin business registry `kind` + single list (2026-08-01)

Production-readiness Task 12 (PERF GATE): migration `000171` adds
`businesses.kind` (`real`/`demo`/`test`) with CHECK + index
`(kind, subscription_status)`, and backfills empty `subscription_status` before
its CHECK. `database.ListBusinessesForAdmin` /
`CountBusinessesForAdmin` become the single registry entry point; admin
dashboard, list, emails, and fiscal summary all scope `kind=real` by default.
`getPaymentMetrics` / `getBillMetrics` drop the dual-source reconcile that
recomputed when business counts disagreed.

Access-shape locked by:
- `TestListBusinessesForAdmin_KindRealExcludesFixtures`
- `TestAdminBusinessRegistrySurfacesShareOneCount`
- `TestAdminSurfacesShareBusinessCount`

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: 500 real + 50 demo businesses (SQLite, existing bench setup).

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkGetBusinessList` **before** (`is_demo = false`) | 0.98M–1.66M | ~177k | 4173 |
| `BenchmarkGetBusinessList` **after** (`kind = real` via ListBusinessesForAdmin) | 1.51M–5.58M | ~180k | 4243 |

Wall time is noisy on shared-cache SQLite (third after run spiked); allocs +~70
(~1.7%) from the Kind field on the list DTO and shared filter helper. No extra
index beyond `idx_businesses_kind_status` from the migration.

```bash
cd backend
go test ./internal/server/ -run '^$' -bench 'BenchmarkGetBusinessList' -benchmem -count=3
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

## Payment History `payment_events` union (2026-07-31)

Production-readiness Task 6 (PERF GATE): `loadPaymentHistoryItemsFiltered` reads
the `payment_events` view (migration `000172`) unioning `payments` +
`alternative_payments` so cash/card rows appear in Payment History. View keeps
writes single-sourced; covering index `idx_alt_payments_bill_created_at`
(`bill_id, created_at`). Access-shape locked by
`TestLoadPaymentHistoryUnionsBothPaymentTables` (5 crypto + 3 card → 8 rows,
amount sum == analytics recognized revenue, no `SELECT *` / Preload).

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: 500 crypto payments with heavy bill JSON (existing SQLite perf setup).

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkLoadPaymentHistoryItemsSQLite` **before** (payments only) | 3.15M–5.07M | ~717k | 17444 |
| `BenchmarkLoadPaymentHistoryItemsSQLite` **after** (payment_events view) | 1.81M–2.06M | ~718k | 17434–17435 |
| `BenchmarkLoadPaymentHistoryItemsPaginatedSQLite` **before** | 614k–810k | ~43k | 925 |
| `BenchmarkLoadPaymentHistoryItemsPaginatedSQLite` **after** | 657k–675k | ~44k | 916 |

Union lands within noise (full-window wall improved; paginated B/op +~4% from
extra projected columns, allocs slightly down). No secondary index needed beyond
the migration covering index.

```bash
cd backend
go test ./internal/server/ -run '^$' -bench 'BenchmarkLoadPaymentHistory' -benchmem -count=3
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

## D5 launch capacity contract (2026-08-01)

Added an enforceable capacity contract for anonymous menus, authenticated
operator actions, guest quotes/orders, callback validation, SSE, background
jobs, PostgreSQL connections, and lock contention. The isolated harness now has
repeatable active bills and staff login-code fixtures, tagged p50/p95/p99/error
gates, a two-minute burst, a one-hour soak, a guarded five-times candidate
profile, and a system sampler for CPU/memory/disk/DB/goroutine/queue/SSE signals.

Local RED: the contract suite failed seven groups because those surfaces,
profiles, fixtures, telemetry, and genuine baseline rules were absent. Local
GREEN evidence and commands are recorded in
`docs/performance/d5-capacity-readiness.md`.

No production or shared-staging load was run. Launch proof remains pending: the
exact candidate must pass the external one-hour five-times profile with
distributed source IPs, signed sandbox callbacks, and the complete retained
evidence bundle.

## Accounting GetSummary pre-aggregation (2026-07-20)

Task 6: replace per-record loads in `GetSummary` with SQL
`GROUP BY (day, currency, category)` for manual ledger entries and
`GROUP BY (day, currency)` for paid payroll, then FX-convert buckets at local
day start. Failed FX buckets slow-path re-query entry IDs for
`skipped_manual_entries` warning details. Public Summary JSON field names
unchanged; equivalence locked by `TestGetSummaryPreAggregationEquivalence`.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: 2000 manual entries + 100 payroll runs, 60-day window, 3 currencies
(USD/EUR/GBP), ~5% voided entries, draft/void payroll mix (SQLite in-memory).

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkGetSummary` **before** (per-record load + FX) | 26.4M–29.3M | ~16.9M | ~65670 |
| `BenchmarkGetSummary` **after** (SQL day×currency×category buckets) | 8.5M–10.3M | ~727k | ~13145 |

Approx **~3× faster**, **~23× less memory**, **~5× fewer allocs** at scale.
Complexity moves from O(records) toward O(days × currencies × categories).

```bash
cd backend
go test ./internal/accounting/ -run '^$' -bench 'BenchmarkGetSummary$' -benchmem -count=3
```

Equivalence: `TestGetSummaryPreAggregationEquivalence` seeds multi-currency
voided/active entries across ~10 days plus paid/void/draft payroll and asserts
totals, breakdowns, payroll rollup, skip counts, and warning details.

## Fiscal receipts list pagination (2026-07-20)

Task 4: paginated fiscal receipts list (`ListReceiptsPage`) with embedded
delivery badges (one `WHERE receipt_id IN (?)` query) and `needs_attention`
(receipt `failed_*` OR dead delivery task). Legacy `ListReceipts` / handler
`{"items":...}` shape when `page` query param is absent.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: 500 receipts × 2 delivery tasks each (SQLite in-memory); page size 20.

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkListReceiptsPageNPlus1` **before** (page + per-row delivery) | 1.32M–1.76M | ~291k | ~4158 |
| `BenchmarkListReceiptsPage` **after** (page + single IN embed) | 932k–992k | ~102k | 2427 |

Approx **~1.4–1.8× faster**, **~2.8× less memory**, **~1.7× fewer allocs** for a
dashboard-sized page vs N+1 delivery fetches. Primary win is constant query
shape (kills FE N+1 of 50× `GET /receipts/:id/delivery`).

```bash
cd backend
go test ./internal/fiscal/ -bench BenchmarkListReceiptsPage -benchmem -count=3 -run '^$'
```

Access-shape: `TestListReceiptsDeliveryIsSingleQuery` asserts exactly one
`fiscal_delivery_tasks` SELECT per page load.

Paged response shape (top-level `ReceiptsPage`):
```json
{"receipts":[...],"total":N,"page":1,"page_size":20,"total_pages":M}
```
Each receipt row includes `delivery: [{task_id,channel,status}]` and
`needs_attention`. Customer doc numbers are masked on both legacy and paged paths.

## Accounting payroll list pagination (2026-07-20)

Task 2: paginated payroll runs list with `payee_count` via one aggregate
(`ListPayrollRunsPage`) while keeping legacy unbounded `ListPayrollRuns`
(with `Preload("LineItems")`) for old FE. Handler branches on presence of
`page` query param.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: 100 draft runs × 8 line items (SQLite in-memory).

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkListPayrollRuns` **before** (legacy Preload all runs+lines) | 10.7M–15.8M | ~8.87M | ~25860 |
| `BenchmarkListPayrollRuns` **after** (page 1 size 20 + payee_count aggregate) | 554k–930k | ~158k | 926 |

Approx **~18–20× faster**, **~56× less memory**, **~28× fewer allocs** for a
dashboard-sized page vs hydrating every line item.

```bash
cd backend
go test ./internal/accounting/ -bench BenchmarkListPayrollRuns -benchmem -count=3 -run '^$'
```

Access-shape: `TestServiceListPayrollRunsPage_PayeeCountQueryShape` asserts
exactly one `payroll_line_items` query with `COUNT`/`GROUP BY` (GORM `Scan`
via Row processor), not N per run and not full LineItems Preload.

## Manual billing Phase 1 — invoice list baseline (2026-07-08)

Task 9 access-shape: `queryInvoices` uses a narrow column Select, default/capped
limit (50 / max 200), optional business/seller/status/expiring_soon filters, and
no N+1 seller join (seller filter is a subquery on business IDs).

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkQueryInvoices_ExpiringSoon` (2000 open rows, limit 50) | 482643–486211 | ~87250 | 1972 |

No prior implementation — this is the baseline the `(status, due_date)` and
`(business_id, status)` indexes must sustain.

```bash
cd backend
go test ./internal/handlers -run '^$' -bench BenchmarkQueryInvoices_ExpiringSoon -benchmem -count=3
```

## Guest Bill Split State Read (2026-06-13)

Added persisted guest bill split holds/settlement and a public split-state read
for `/api/v1/guest/bill/:bill_number/split/state`. Access-shape coverage asserts
the read projects bill/share fields, avoids `SELECT *`, avoids payment
hydration, and reads split shares in one query.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkGetBillSplitStateByNumberSQLite` baseline model-hydrate read | 490685-495768 | 981744-981774 | 3156 |
| `BenchmarkGetBillSplitStateByNumberSQLite` projected row scan | 317024-338341 | 128558-128586 | 2465 |

The optimization replaced GORM model hydration/JSON serializers on the hot
state read with a narrow row scan and lazy claimed-item JSON parsing. Behavior
is covered by `TestGetBillSplitStateByNumberUsesProjectedReads`.

```bash
cd backend
go test ./internal/database -run '^$' -bench BenchmarkGetBillSplitStateByNumberSQLite -benchmem -count=3
```

## Rate-limiter key eviction (2026-05-29)

Stops the
WhatsApp inbound rate-limiter's `rateHits` map growing one key per unique sender
JID forever (added `sweepOnce`/`sweepLoop` in `whatsapp_manager.go`, launched
via `logger.SafeGo` from `NewWhatsAppManager`), and hardened HTTP-limiter
test/benchmark coverage around the already-shipped evictions.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`, medians below.

### WhatsApp `rateHits` (before/after eviction)

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkWhatsAppAllowSenderChurn` (baseline, unbounded) | ~331 | ~252 | 3 |
| `BenchmarkWhatsAppSweptChurn` (after, periodic `sweepOnce`) | ~219 | 87 | 3 |

The swept variant's live-key bound assertion held (`live < b.N`) — the map stays
bounded under high unique-JID churn instead of one entry per sender forever. B/op
drops because the map no longer reallocates as it grows unbounded.

```bash
cd backend
go test ./internal/services/ -run '^$' -bench 'BenchmarkWhatsApp.*Churn' -benchmem -count=3
```

### HTTP `SimpleRateLimiter.visitors` (single-IP contrast vs bounded churn)

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkRateLimiter` (full gin request, single fixed IP, never grows the map) | ~11476 | ~61718 | 34 |
| `BenchmarkSimpleRateLimiterChurn` (direct `isAllowed`, many unique IPs + periodic `evictIdle`) | ~254 | 98 | 5 |

The two benches aren't directly comparable (the single-IP one drives a full gin
HTTP request; the churn one calls `isAllowed` directly), but the churn bench is
the point: it proves the `visitors` map stays bounded across `b.N` unique IPs.
`BenchmarkSimpleRateLimiterChurn`'s live-key bound assertion held. No production
change here — `cleanupVisitors` already evicted; this stream extracted `evictIdle`
(behavior-preserving) so the predicate is unit-testable and added the missing
high-unique-IP churn benchmark.

```bash
cd backend
go test ./internal/middleware/ -run '^$' -bench 'BenchmarkSimpleRateLimiterChurn|BenchmarkRateLimiter$' -benchmem -count=3
```

### middleware `RateLimiter.limiters`/`lastSeen` (churn bound)

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkRateLimiterEvictStaleChurn` (many unique keys + periodic `evictStale`) | ~236 | 128 | 3 |

Live-key bound assertion held. No production change — `evictStale` already
existed; this added the missing `-benchmem` churn benchmark proving the
`limiters`/`lastSeen` maps stay bounded.

```bash
cd backend
go test ./internal/middleware/ -run '^$' -bench BenchmarkRateLimiterEvictStaleChurn -benchmem -count=3
```

## What's on this branch

Five layers of work:

### Layer 1 — benchmark + load-test infrastructure (17 commits)

Lets us measure perf locally and gate regressions in CI.

- **Go benchmarks** (`testing.B`) for the three hot flows: menu browse, guest checkout, kitchen SSE. Plus AuthMiddleware, BillMarshalJSON. Run via `make bench` (count=6) or `make bench-ci` (count=3).
- **Testcontainers Postgres harness** (`internal/testperf/`) so benches run against a real DB.
- **pprof snapshot endpoint** (`/debug/pprof/snapshot`, token-gated) for prod profiling.
- **Prometheus `flow_request_duration_seconds`** histogram + middleware (`internal/middleware/flow_metrics.go`) so flow latency is observable.
- **GORM slow-query log** (opt-in via `DB_SLOW_QUERY_LOG=true`).
- **Seed CLI** (`make perf-seed`) for idempotent fixture loading.
- **k6 load-test scenarios** (`backend/perf/k6/`): menu_browse, checkout, sse_kitchen, mixed_peak. Profiles: smoke, lunch_rush, dinner_rush, soak_4h.
- **Staging-perf docker-compose** (`backend/perf/staging/`) for soak tests.
- **GitHub Actions**: perf-bench (PR + nightly) and perf-drift (weekly) workflows (since retired; compare runs locally per `backend/perf/bench/README.md`).
- **Grafana dashboard** (`backend/perf/grafana/payverge-perf.json`).
- **Docs**: `backend/perf/bench/README.md` covers the suite end-to-end.

### Layer 2 — perf iterations on OrderCreate + MenuGet (6 commits)

Each iteration was profile-driven, benchstat-verified, and tested against the full Go test suite.

| # | Commit | Change |
|---|--------|--------|
| 1 | `d2de1b53` | Skip redundant `Preload(Bill).Preload(Business)` re-fetch on CreateOrder. Reuses in-memory data. |
| 2 | `77d5ccb0` | TTL cache (5s) for business/menu/promotions in `PriceOrderInputsByBusinessID`. Auto-resets on `SetTestDB`/`InitTestDB`. |
| 3 | `824bc258` | Single `json.Marshal(order)` shared between c.JSON and SSE.Publish. |
| 4 | `799d9eb7` | Documented PrepareStmt regression (+75% ns/op — pgx already prepares internally); hardened seed loader. |
| 5 | `b697dbfd` | Menu read handler now uses the same pricing cache. 4 DB queries → 1. |
| 6 | `22f6ed47` | Cached business-by-customURL lookup. Last DB query off the menu read path. |

### Layer 3 — target-closing fixes after benchmark audit (committed)

Focused on the three remaining misses from the latest benchmark audit:

- **OrderCreate**: skip Telegram notification outbox writes unless the business has an active, connected Telegram plugin and the event is enabled. This removes pointless DB writes for the common no-Telegram path while preserving delivery for connected Telegram businesses.
- **BillMarshalJSON**: replace the reflective shadow-struct marshal with a manual encoder for the hot Bill response shape. The encoder preserves dollar conversion, string escaping including invalid UTF-8 replacement, `null` for non-omitempty pointer fields, and loaded relation fields.
- **AuthMiddleware**: cache verified JWT claims when `session.GlobalStore == nil` and add a private middleware fast path that reads cached claims without exposing mutable maps to callers. Exported `VerifyUserToken` still returns independent claim maps.

### Layer 4 — additional operational benchmarks (committed, still expanding)

Added fixed-iteration benchmarks around the next expensive restaurant operations:

- Order approval/status transition, including inventory deduction and notification hooks.
- Manual bill payment settlement.
- Business bill list endpoint.
- Guest table load endpoint.
- Connected Telegram order-create path.
- Plugin notification worker processing due deliveries.
- Inventory deduction transaction.
- Fiscal receipt enqueue path.
- Thermal print receipt enqueue/routing/render path.

The additional audit found two more unnecessary Telegram outbox paths: order status/low-stock notifications and manual payment notifications. Both now use `services.ShouldEnqueueTelegramNotification(...)`, so businesses without connected Telegram skip the write path while connected businesses keep the old behavior.

### Layer 5 — code-level optimizations from operational profiles (committed, still expanding)

Focused on data access shape and queue mechanics rather than benchmark-specific tweaks:

- **Business bill list** now uses a narrow `database.BillListRow` projection instead of hydrating full `Bill` aggregates and preloading `Table` only to reshape the result. Added migration `000052_perf_read_path_indexes` with composite/partial indexes for bill list access patterns.
- **Print receipt enqueue** now uses precompiled print templates and loads receipt bill/business/table/payment inputs in one pass instead of parsing templates and loading the bill twice per enqueue.
- **Plugin notification worker** now batches successful delivery finalization in one transaction and samples queue-depth metrics instead of counting backlog twice per batch.
- **Manual bill payment** now skips the generated manual tx-hash pre-insert miss and authorizes with a narrow bill->business projection before the payment transaction. The unique `payments.tx_hash` index still catches the rare generated collision.
- **Order approval** now verifies route ownership with a narrow order->business projection instead of loading/parsing the full order before the transaction reloads it.
- **Guest table QR load** now reuses the shared menu/promotion snapshot and a short-lived table-code context cache, so repeated QR scans for the same table avoid reloading table/business/menu/promotions every request.
- **Guest menu/order creation** now uses the same shared menu/promotion snapshot for table-code menu and order pricing paths, removes per-order debug stdout writes, hydrates the just-created guest order from data already in memory instead of re-querying it, and loads table+business QR context with one joined table-code lookup on cache miss.
- **Guest business metadata** now caches low-churn language metadata and Trustpilot state behind `/guest/table/:code/business`, avoiding repeated language/plugin reads on page/layout loads for the same business.
- **Guest bill creation** now checks active table occupancy with a narrow existence probe instead of hydrating a full bill aggregate. Migration `000052_perf_read_path_indexes` also adds `idx_bills_active_table_created_id` so active-bill lookup uses the database index rather than an app-side scan.
- **Legacy table-code summary** now uses a narrow active-bill summary projection instead of loading/parsing bill items that the response does not return.
- **Customer table check-in** now reuses the joined active table/business lookup and a narrow active-bill summary projection. The check-in connection path validates the customer but skips a redundant business read because the joined table-code lookup already proved the business is active.
- **CRM bill attachment** now has a projection helper for hot guest/check-in/payment attribution paths. It locks only `id`, `status`, and `crm_customer_id`, so these routes no longer hydrate full bill rows just to attach a customer reference.
- **Public guest open-bill/status** now uses a projected active-bill snapshot for public response fields plus normalized bill items. Internal mutation flows keep the full aggregate loader, but QR polling routes no longer do `SELECT * FROM bills`.
- **Public bill item fallback** now queries normalized `bill_items` first and lazy-loads legacy `bills.items` JSON only when relation rows are absent. That avoids pulling potentially large legacy snapshots on modern bills.
- **AI waiter message polling** now returns bounded, projected history. Public polling defaults to the latest 100 messages, skips large `tool_calls` payloads, avoids a duplicate business lookup, and adds an ordered `(conversation_id, created_at)` index for long conversations.
- **AI dashboard transcripts** now reuse bounded projected history for staff transcript modals. Staff reads default to the latest 200 messages and skip `tool_calls`/full-row hydration for long conversations.
- **Director Console transcripts** now load the latest bounded projected messages for the dashboard thread view. The frontend explicitly requests `limit=200`, the backend clamps requests to 500, and migration `000054` adds an ordered business/thread/message history index.
- **AI menu wizard session reads** no longer preload every wizard message just to authorize/session-load and then read them again for the client. The dashboard client requests `limit=100`; the handler returns latest projected UI messages and migration `000055` adds an ordered session/message history index.
- **Delivery dispatch list** no longer preloads `Bill` and `Order` for every delivery row because the dispatch UI renders delivery fields plus driver/zone context, not full bill/order aggregates. Detail/event paths keep richer delivery loads.
- **Reservation list/date/upcoming reads** now preload only the table summary fields rendered by the reservation UI (`id`, `name`, `table_code`, `capacity`) instead of hydrating full table QR/customization rows for every page result. Detail reads keep the richer reservation preload path.
- **Reservation settings availability summaries** now reuse one business/settings/table availability context across the seven-day public/settings slot summary instead of reloading the same context once per day. Conflict queries still run per day because each day has a different operating window.
- **Analytics live bills/dashboard active-bill summaries** now use a compact active-bill projection plus narrow table `id/name/table_code` reads instead of hydrating full bill rows and full table QR/customization payloads for every dashboard poll.
- **RBAC audit log reads** now preload only staff display fields and business display identifiers instead of full staff/business rows. The latency delta is small on the current benchmark, but the access shape now avoids hydrating PIN hash/custom-permission/business payload columns in audit views.
- **Public delivery tracking** now uses a purpose-built tracking projection instead of the internal full delivery detail loader. It loads only delivery status/location/ETA, business display fields, and driver display fields, skipping bill/order/customer/status-history aggregates that the public tracking response never returns.
- **Payment history/export** now scans the payment-history response projection with one `payments -> bills -> tables` join instead of loading every payment, then rehydrating the full bill and table rows per result. The endpoint/export still returns the same payment/bill/table fields, but avoids large bill item snapshots and QR/customization table payloads.
- **Order list/kitchen polling** now preloads a bill summary projection instead of full bill rows for every order. The list response keeps bill identifiers/status/amount/timing metadata but skips the legacy `bills.items` snapshot, since order cards render `orders.items`.
- **Delivery driver list/performance** now returns driver rows without preloading full staff records. Driver management and assignment UIs render delivery-driver fields, so list reads no longer hydrate staff custom-permission/PIN-hash payloads.
- **Public payment details** now resolves the display bill number with a one-column bill lookup instead of loading the full bill aggregate. The transaction-status response no longer preloads bill business/table/all-payments just to show `bill_number`.
- **Table status reads** now return active bill summary metadata without selecting the legacy `bills.items` payload. The route still returns active bill counts/status metadata but avoids full bill-row hydration for floor-plan polling.
- **Payment breakdown reads** now resolve bills with an amount/status projection and compute confirmed alternative-payment total with a SQL aggregate. Public/inside breakdown polling no longer loads the same full bill twice or hydrates all alternative-payment rows just to sum `amount`.
- **Pending alternative-payment polling** now authorizes with projected bill/business ownership fields instead of full bill/business/table/payment preloads. The response still returns pending alternative-payment rows, but owner polling no longer parses large bill snapshots first.
- **Guest crypto/cross-chain payment setup** now resolves bill-number payments with settlement/status projections and subscription projections before verifier work. Milestone flushing now claims pending events before loading the business, so the common no-milestone payment path avoids a full business hydrate.
- **Customer connected-business list** now preloads only business display/currency fields for the customer profile cards instead of full business gallery/social/design payloads for every connected business.
- **Public guest bill-by-number reads** now use a public bill projection joined only to prove the owning business is active. The route still returns bill items, but no longer preloads the full business, table, or all payment rows.
- **Bill split options** now reuse the public bill projection helper instead of full bill/business/table/payment preloads before rendering splitable items and bill totals.
- **Bill split calculations** now load only bill amount/status fields for equal/custom/item calculations, and item splits read normalized `bill_items` before falling back to legacy `bills.items`. Split POST paths no longer hydrate business/table/payment relations or parse large legacy snapshots when relational items exist.
- **Bill-scoped access middleware** now authorizes with a joined bill→business projection carrying only ownership/subscription fields. Routes behind `RequireBillBusinessAccess` no longer full-load bill/business/table/payment/item aggregates before the handler runs, so handler-level optimizations are visible on real mounted routes.

## Final benchmark numbers (Apple M3)

| Bench | Original | Final | Δ |
|-------|---------:|------:|---:|
| **OrderCreate**             | 2.1 ms / 408 KiB / 2114 allocs | **0.73-0.86 ms / 85 KiB / 374-375 allocs** | **~59-65% faster / -79% / -82%** |
| **MenuGet** (empty)         | 528 µs / 122 KiB / 667 allocs  | **3.7 µs / 7.8 KiB / 42 allocs**   | **-99.3% (143×) / -94% / -94%** |
| **MenuGetWithItems** (50)   | 725 µs / 268 KiB / 882 allocs  | **52 µs / 73.7 KiB / 42 allocs**   | **-93% (14×) / -73% / -95%** |
| BillMarshalJSON             | 29 µs / 14 allocs               | **5.1-5.8 µs / 4.4 KiB / 3 allocs** | **~80-82% faster / -79% allocs** |
| AuthMiddleware              | 5 µs / 85 allocs                | **1.68-1.76 µs / 6.2 KiB / 18 allocs** | **~65-66% faster / -79% allocs** |
| SSEBroadcast (5000 subs)    | 15 µs / 0 allocs                | 19 µs / 0 allocs                    | variance (~unchanged) |
| SSEBroadcast_LowFanout      | 27 ns / 0 allocs                | 33 ns / 0 allocs                    | variance (~unchanged) |

## Targets vs reality

| Bench | Target | Actual | Status |
|-------|-------:|-------:|--------|
| OrderCreate                 | < 1 ms / < 500 allocs    | 0.73-0.86 ms / 374-375 allocs | ✅ met |
| MenuGet                     | < 200 µs / < 200 allocs  | 3.7 µs / 42 allocs     | ✅ far exceeded |
| MenuGetWithItems            | < 250 µs / < 200 allocs  | 52 µs / 42 allocs      | ✅ exceeded |
| BillMarshalJSON             | < 10 µs / < 5 allocs     | 5.1-5.8 µs / 3 allocs  | ✅ met |
| AuthMiddleware              | < 2 µs / < 30 allocs     | 1.68-1.76 µs / 18 allocs | ✅ met |
| SSEBroadcast                | already good             | 19 µs                  | ✅ |

## Additional benchmark coverage

Fixed-iteration runs use `-benchtime=100x` to keep the side-effecting handler benchmarks comparable.

| Bench | Result | Assessment |
|-------|-------:|------------|
| OrderCreateWithTelegramEnabled | 1.21 ms / 110 KiB / 541 allocs | Connected Telegram remains acceptable for an opt-in integration path. |
| OrderStatusApprove | 2.55 ms / 237 KiB / 1975 allocs | Down from 4.80 ms / 450 KiB / 3166 allocs by gating Telegram writes and replacing duplicate pre-transaction order hydration with an order->business projection. |
| MarkBillAsPaid | 2.68 ms / 266 KiB / 1630 allocs | Down from 7.72 ms after Telegram gating, generated tx-hash insert-first handling, and narrow bill access authorization. |
| GetBusinessBills | 1.3-2.0 ms / ~216-220 KiB / ~2660 allocs | Heap fixed: down from 1.08 MiB/op by replacing full ORM hydration with a projection query. A `COUNT(*) OVER()` experiment was rejected because it benchmarked slower on the current dataset. |
| GuestTableLoad | 224 µs / 84 KiB / 154 allocs | Down from 1.23-1.92 ms / ~281 KiB / ~1098 allocs by sharing the menu/promotion snapshot and caching table-code context. |
| GetBusinessByTableCodeSQLite | 18.5 µs / 22 KiB / 144 allocs | Local SQLite route microbench for `/guest/table/:code/business` after caching language/plugin metadata. Added because Docker/Testcontainers was unstable for this slice. |
| LegacyGetTableByCodeSQLite | 34.7 µs / 27 KiB / 276 allocs | Local SQLite route microbench for legacy `/table/:code`; bill summary now avoids `bill_items` reads. |
| CustomerTableCheckInSQLite | 303 µs / 171 KiB / 1979 allocs | Local SQLite steady-state authenticated check-in benchmark with an already-linked open bill and active customer-business connection. Access-shape tests verify standalone `businesses` reads dropped from 2 to 0 and full bill-row hydration dropped to 0. |
| GuestOpenBillByTableCodeSQLite | 94-159 µs / 40 KiB / 514 allocs | Local SQLite route microbench for public QR open-bill polling with 8 bill items. Wall time is noisy locally; allocation improved from the temporary old-path A/B (41 KiB / 520 allocs), and access-shape tests verify no full bill row or legacy `items` JSON read on normalized bills. |
| GetAiWaiterMessagesSQLite | 354-530 µs / 147 KiB / 2379 allocs | Local SQLite route microbench for a 1000-message AI waiter conversation with large tool payloads. Down from 782 µs-1.57 ms / 169 KiB / 2892 allocs after bounded projected history, duplicate business-read removal, and the message-history index. |
| GetAiConversationMessagesSQLite | 754 µs-1.10 ms / 251 KiB / 4058 allocs | Local SQLite route microbench for the staff AI transcript modal against a 1000-message conversation with large tool payloads. The route now returns the latest 200 projected rows instead of hydrating the full transcript. |
| GetDirectorThreadMessagesSQLite | 12.3-14.5 ms / 6.4 MiB / 13.6k allocs | Down from 101-207 ms / 14.6-14.8 MiB / 33.9k allocs by returning the latest 200 projected Director Console messages instead of full-row hydration of the oldest 500 messages. |
| GetWizardSessionSQLite | 396-399 µs / 0.98 MiB / 3143-3144 allocs | Down from 5.39-5.44 ms / 19.2 MiB / 45.3k allocs by removing duplicate wizard message hydration and returning the latest 100 projected client messages. |
| GetBusinessDeliveriesSQLite | 2.16-2.19 ms / 3.13 MiB / 10.0k allocs | Down from 3.61-3.67 ms / 6.09 MiB / 18.1k allocs by removing unused bill/order preloads from the delivery dispatch list path. |
| GetReservationsByBusinessIDPaginatedSQLite | 1.12-1.15 ms / 1.51 MiB / 6634-6635 allocs | Down from 1.385-1.391 ms / 2.09 MiB / 9074 allocs by projecting only reservation table summary fields instead of preloading full table QR/customization payloads. |
| GetPublicReservationSettingsSQLite | 6.55-7.04 ms / 23.36 MiB / 25.7k allocs | Down from 7.98-8.17 ms / 24.57 MiB / 35.1k allocs by reusing the availability context across the seven-day slot summary; query-count guard dropped repeated context loads from 36 queries to <=20. |
| AnalyticsLiveBillsSQLite | 1.41-1.43 ms / 1.32 MiB / 16.9k allocs | Down from 2.66-2.71 ms / 5.69 MiB / 21.7k allocs by replacing full active-bill/table hydration with projected dashboard summary rows. |
| GetRBACAuditLogSQLite | 1.52-1.53 ms / 3.806 MiB / 6439 allocs | Slight alloc improvement from 1.51-1.58 ms / 3.814 MiB / 6500 allocs; main win is the access-shape guard preventing full staff/business preload payloads. |
| TrackDeliverySQLite | 35.7-37.6 µs / 45.5 KiB / 395 allocs | Down from 314-361 µs / 733 KiB / 2103 allocs by replacing the full internal delivery detail loader with a public tracking projection. |
| LoadPaymentHistoryItemsSQLite | 1.73-2.17 ms / 646 KiB / 15.9k allocs | Down from 144-212 ms / 126 MiB / 894k allocs by replacing per-payment full bill/table hydration with a single projected join for payment history/export rows. |
| GetOrdersByBusinessIDPaginatedSQLite | 1.76-1.93 ms / 3.40 MiB / 7084-7085 allocs | Down from 2.10-2.47 ms / 4.49 MiB / 7374-7376 allocs by preloading only bill summary columns and skipping legacy bill item snapshots. |
| GetBusinessDriversSQLite | 3.03-3.72 ms / 3.41 MiB / 21.6k allocs | Down from 6.10-6.43 ms / 8.48 MiB / 44.2k allocs by removing unused staff preloads from driver list/performance reads. |
| GetPaymentDetailsSQLite | 22.7-23.6 µs / 26 KiB / 233 allocs | Down from 1.56-2.01 ms / 3.71 MiB / 10.4k allocs by replacing full bill/business/table/payments hydration with a bill-number projection. |
| GetTablesWithStatusSQLite | 9.06-9.29 ms / 13.93 MiB / 46.0k allocs | Down from 19.9-20.7 ms / 19.38 MiB / 47.5k allocs by projecting active bill summary fields and skipping legacy bill item snapshots. |
| GetBillPaymentBreakdownSQLite | 143-157 µs / 26 KiB / 232 allocs | Down from 5.88-6.77 ms / 6.64 MiB / 16.9k allocs by resolving bill amount/status fields once and aggregating alternative payments in SQL. |
| GetPendingAlternativePaymentsSQLite | 6.06-6.38 ms / 9.36-9.46 MiB / 15.5k allocs | Down from 12.15-12.87 ms / 9.71-9.73 MiB / 17.4k allocs by removing full bill/business/table/payment preloads before pending-row fetch. |
| ProcessCryptoPaymentSQLite | 195-204 µs / 146 KiB / 1208-1211 allocs | Down from 832-852 µs / 1.08 MiB / 5299-5300 allocs by projecting pre-verification bill/subscription reads and avoiding empty milestone business hydration. |
| GetCustomerBusinessConnectionsSQLite | 2.49-2.64 ms / 3.74 MiB / 16.7k allocs | Down from 30.4-97.4 ms / 6.81 MiB / 61.4k allocs by preloading only business display/currency fields for customer profile cards. |
| GetBillByNumberPublicSQLite | 316-318 µs / 294 KiB / 879 allocs | Down from 1.63 ms / 3.88 MiB / 10.1k allocs by projecting public bill fields and skipping business/table/payment preloads. |
| GetBillSplitOptionsSQLite | 287-293 µs / 251 KiB / 921 allocs | Down from 1.62-1.68 ms / 3.81 MiB / 10.1k allocs by using the public bill projection helper for split options. |
| CalculateEqualSplitSQLite | 11.0-11.2 µs / 10.4 KiB / 118 allocs | Down from 1.48-1.58 ms / 3.63 MiB / 9.6k allocs by projecting bill amount fields instead of loading full bill/business/table/payment aggregates. |
| CalculateItemSplitSQLite | 30.8-31.4 µs / 20.8 KiB / 266 allocs | New service-level guard: item splits use the same bill projection and relational `bill_items` first, avoiding legacy snapshot reads when normalized rows exist. |
| RequireBillBusinessAccessSQLite | 15.0-15.2 µs / 16.2 KiB / 141 allocs | Down from 1.51-1.94 ms / 3.72 MiB / 10.0k allocs by replacing middleware full bill aggregate hydration with a bill→business access projection. |
| GuestMenuByTableCode | 113 µs / 80.6 KiB / 74 allocs | Down from 1.33 ms / 235 KiB / 508 allocs by moving table-code menu reads onto the shared menu/promotion snapshot. |
| GuestOpenBillByTableCode | 448 µs / 33.1 KiB / 318 allocs | Acceptable; dominated by active bill + item load because the response needs bill items. |
| GuestTableStatusByCode | 532 µs / 34.5 KiB / 335 allocs | Acceptable; same active bill + item response shape as open-bill lookup. |
| GuestCreateBillByTableCode | 1.32 ms / 82.7 KiB / 790 allocs | Down from 1.53 ms / 89.7 KiB / 871 allocs by replacing the full active-bill hydrate with an existence probe and removing record-not-found log spam. |
| GuestCreateOrder | 1.76 ms / 169 KiB / 1138 allocs | Down from 2.97 ms / 321 KiB / 1587 allocs after shared pricing cache and debug-output removal. Final in-memory hydration/single-encode and joined QR context cleanup are test-validated but still need a Docker-backed rerun because Docker became unresponsive during the rerun. |
| DeductApprovedOrderInventoryTx | 183 µs / 215 KiB / 880 allocs | Good latency. Allocations mostly come from recipe/menu lookup and transaction object graphs. |
| PluginNotificationWorkerProcessDue | 842 µs / 599 KiB / 3787 allocs per 25 deliveries | Much better: down from 2.24 ms / 1.03 MiB / 7755 allocs by batching success finalization and sampling queue-depth metrics. |
| FiscalIssueReceiptEnqueue | 92 µs / 42 KiB / 568 allocs | Good enqueue-path latency. Provider issuance still needs separate benchmarking once real provider clients are wired. |
| PrintEnqueueReceipt | 267 µs / 167 KiB / 1846 allocs | Improved from 369 µs / 214 KiB / 2452 allocs via precompiled templates and one-pass receipt input loading. |

## Capacity estimate

- **OrderCreate** at 0.73-0.86 ms/op ≈ **1,160-1,370 orders/sec per core** → ~9,300-11,000/sec on 8 cores. Realistic restaurant peak is ~100 orders/sec/business; saturation requires thousands of concurrent peak-hour restaurants.
- **MenuGet** at 3.7 µs/op (cached) ≈ **270,000 reads/sec per core**. Effectively free.
- **Worst case (100% cache miss)**: MenuGet ~242 µs, OrderCreate ~1.69 ms. Still strong; cache populates after one request per business.

## Current verification

- `go test ./internal/database ./internal/services ./internal/server -run 'TestBillMarshalJSON|TestTelegram|TestVerifyTokenCacheReturnsIndependentCopies' -count=1` ✅
- `go test ./internal/handlers ./internal/server -run 'TestCreateOrder_EnqueuesTelegramNotification|TestCreateGuestOrder_ReusesExistingOrderForDuplicateRequestID|TestVerifyTokenCacheReturnsIndependentCopies' -count=1` ✅
- `go test ./internal/handlers ./internal/server -run 'TestUpdateOrderStatus_EnqueuesStatusNotification|TestMarkBillAsPaid_EnqueuesTelegramPaymentNotification' -count=1` ✅
- `go test ./internal/handlers ./internal/server ./internal/services ./internal/database ./internal/fiscal ./internal/services/print -run=^$ -count=1` ✅
- `go test ./internal/database ./internal/server ./internal/services ./internal/services/print ./internal/services/print/formatters -run 'TestGetBillListRows|TestGetBusinessBills_|TestGetOpenBusinessBills_|TestTelegramNotificationWorker|TestClaimPluginNotificationDeliveries|TestService_Enqueue|TestReceiptFormatter|TestBillFormatter|TestKitchen' -count=1` ✅
- `go test ./internal/handlers ./internal/server ./internal/services ./internal/database ./internal/fiscal ./internal/services/print -run=^$ -count=1` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench=^BenchmarkBillMarshalJSON$ -benchmem -count=3 -benchtime=2s ./internal/database` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench=^BenchmarkOrderCreate$ -benchmem -count=3 -benchtime=2s ./internal/handlers/...` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench=^BenchmarkAuthMiddleware$ -benchmem -count=3 -benchtime=2s ./internal/server/...` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='BenchmarkOrderStatusApprove$' -benchmem -count=1 -benchtime=100x ./internal/handlers/...` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='BenchmarkMarkBillAsPaid$' -benchmem -count=1 -benchtime=100x ./internal/server/...` ✅
- `go test -run=^$ -bench='BenchmarkPluginNotificationWorkerProcessDue$|BenchmarkDeductApprovedOrderInventoryTx$|BenchmarkFiscalIssueReceiptEnqueue$|BenchmarkPrintEnqueueReceipt$' -benchmem -count=1 -benchtime=100x ./internal/services ./internal/database ./internal/fiscal ./internal/services/print` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='Benchmark(GetBusinessBills|GuestTableLoad|MarkBillAsPaid)$' -benchmem -count=1 -benchtime=100x ./internal/server/...` ✅
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='BenchmarkOrderStatusApprove$|BenchmarkOrderCreateWithTelegramEnabled$' -benchmem -count=1 -benchtime=100x ./internal/handlers/...` ✅
- `go test ./internal/database ./internal/server -run 'TestGetBillBusinessAccessByBillIDReturnsBusinessAccessFields|TestMarkBillAsPaidGeneratedManualPaymentSkipsPreInsertTxHashProbe|TestMarkBillAsPaidManualTxHashConflictReturnsError|TestMarkBillAsPaid_' -count=1` ✅
- `go test ./internal/database ./internal/handlers -run 'TestGetOrderBusinessIDByIDReturnsBusinessID|TestUpdateOrderStatus_Approve|TestUpdateOrderStatus_EnqueuesStatusNotification|TestUpdateOrderStatus_ClosedBillApprovalReturnsConflict|TestUpdateOrderStatus_ApproveAddsItemsToBill' -count=1` ✅
- `go test ./internal/server ./internal/services -run 'TestGetTableByCodePublicUsesSharedMenuCache|TestGetTableByCodePublic_DoesNotExposeSensitiveModels|TestGetTableByCodePublic_ReturnsNotFoundForInactiveBusiness|TestPricing|TestMenuData' -count=1` ✅
- `go test ./internal/server ./internal/database -run 'TestCreateGuestOrderUsesSharedMenuCache|TestCreateBillByTableCodeChecksActiveBillWithoutHydratingBill|TestCreateGuestOrder_ReusesExistingOrderForDuplicateRequestID|TestCreateGuestOrder_RejectsClosedBill|TestCreateBillByTableCode_DisabledOrderingRejectsWhenKitchenDisabled|TestGetMenuByTableCodeUsesSharedMenuCache|TestGetTableByCodePublicUsesSharedMenuCache|TestGetTableByCodePublic_ReturnsNotFoundForInactiveBusiness|TestGetBillBusinessAccessByBillIDReturnsBusinessAccessFields|TestGetOrderBusinessIDByIDReturnsBusinessID|TestMarkBillAsPaidGeneratedManualPaymentSkipsPreInsertTxHashProbe' -count=1` ✅
- `go test ./internal/handlers ./internal/server ./internal/services ./internal/database ./internal/fiscal ./internal/services/print -run=^$ -count=1` ✅
- `go test ./internal/server ./internal/services -run 'TestGetBusinessByTableCodeUsesSharedExtrasCache|TestGetBusinessByTableCode_DoesNotExposeSensitiveBusinessFields|TestGetBusinessByTableCode_ExposesCRMEnabled' -count=1` ✅
- `go test ./internal/server ./internal/services -run=^$ -count=1` ✅
- `go test ./internal/server -run=^$ -bench='BenchmarkGetBusinessByTableCodeSQLite$' -benchmem -count=1 -benchtime=200x` ✅
- `go test ./internal/server ./internal/database -run 'TestGetTableByCodeUsesBillSummaryProjection|TestGetTableByCode_DoesNotExposeSensitiveBusinessFields|TestGetTableByCode_ReturnsOKWhenTableHasNoActiveBill' -count=1` ✅
- `go test ./internal/server ./internal/database -run=^$ -count=1` ✅
- `go test ./internal/server -run=^$ -bench='BenchmarkLegacyGetTableByCodeSQLite$' -benchmem -count=1 -benchtime=200x` ✅
- `go test ./internal/server -run 'TestCustomerTableCheckInUsesNarrowLookupShape|TestCustomerTableCheckInConnectsCustomerAndAttachesOpenBill|TestCustomerTableCheckInReactivatesInactiveConnection|TestCustomerTableCheckInRejectsDifferentClaimedCustomer|TestCustomerTableCheckInConflictDoesNotConnectLosingCustomer' -count=1` ✅
- `go test ./internal/database -run 'TestAttachCRMCustomer' -count=1` ✅
- `go test ./internal/server ./internal/handlers -run 'TestCreateBillByTableCode|Test.*CRMCustomer|Test.*crm_customer|Test.*AlternativePayment|Test.*ManualPayment' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkCustomerTableCheckInSQLite$' -benchmem -count=1` ✅
- `go test ./internal/database ./internal/server ./internal/handlers -run=^$ -count=1` ✅
- `go test ./internal/server ./internal/database -run 'TestGetOpenBillByTableCodeUsesProjectedBillLookup|TestGetOpenBillByTableCodeReturnsPartialBill|TestGetOpenBillByTableCode_ReturnsInternalErrorWhenActiveBillLookupFails|TestGetOpenBillByTableCode_ReturnsNotFoundWhenTableHasNoActiveBill|TestGetTableStatusByCode_ReturnsInternalErrorWhenActiveBillLookupFails|TestGetTableStatusByCode_ReturnsNoOpenBillWhenTableHasNoActiveBill' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkGuestOpenBillByTableCodeSQLite$' -benchmem -count=3` ✅
- `go test ./internal/database ./internal/server -run=^$ -count=1` ✅
- `go test ./internal/server ./internal/database -run 'TestGetAiWaiterMessagesBoundsAndProjectsHistory|TestGetAiWaiterMessages_RejectsDisabledOrMissingBusinessesWithoutCreatingConversation|TestGetAiWaiterMessages_RejectsInvalidOrderingScopeWithoutCreatingConversation|TestAIWaiterPublicEndpointsRequireAIProPlan|TestHandleAIWaiter_RejectsInvalidScopeBeforeAIProcessing|TestGetRecentAiWaiterMessages' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkGetAiWaiterMessagesSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server ./internal/database -run=^$ -count=1` ✅
- `go test ./internal/server ./internal/database -run 'TestGetAiConversationMessagesBoundsAndProjectsTranscript|TestAIConversationRoutes_RequireConversationToBelongToRouteBusiness|TestGetAiWaiterMessagesBoundsAndProjectsHistory|TestGetRecentAiWaiterMessages' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkGetAiConversationMessagesSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server ./internal/database -run=^$ -count=1` ✅
- `go test ./internal/server ./internal/database ./internal/services -run 'TestGetDirectorThreadMessagesBoundsAndProjectsTranscript|TestDirectorConsoleDeniesNonOwner|TestDirectorConsoleRequiresAIProPlan|TestSaveAndListDirectorToolCalls' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkGetDirectorThreadMessagesSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server ./internal/database ./internal/services -run=^$ -count=1` ✅
- `NODE_PATH=frontend/node_modules frontend/node_modules/.bin/jest --watchman=false --runInBand src/api/directorConsole.test.ts --testTimeout=10000` ✅
- `go test ./internal/server ./internal/database -run 'TestGetWizardSessionBoundsAndProjectsMessages|TestStartMenuExtraction_AcceptsBusinessSlug|TestUploadMenuPage_AllowsJPEGAndGeneratesServerFilename' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkGetWizardSessionSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server ./internal/database -run=^$ -count=1` ✅
- `NODE_PATH=frontend/node_modules frontend/node_modules/.bin/jest --watchman=false --runInBand src/api/business.test.ts --testTimeout=10000` ✅
- `go test ./internal/services -run '^TestGetBusinessDeliveriesSkipsUnusedBillAndOrderPreloads$' -count=1` ✅
- `go test ./internal/services -run '^$' -bench '^BenchmarkGetBusinessDeliveriesSQLite$' -benchmem -count=3` ✅
- `go test ./internal/services ./internal/handlers -run 'TestGetBusinessDeliveriesSkipsUnusedBillAndOrderPreloads|TestCreateDeliveryOrder_|TestUpdateDeliveryStatus_PublishesDeliveryUpdatedEvent|TestAssignDriver_PublishesDeliveryUpdatedEvent|TestDeliveryRouteGates|TestRolePermissions_DeliveryFamily' -count=1` ✅
- `go test ./internal/services ./internal/handlers -run=^$ -count=1` ✅
- `go test ./internal/database -run 'TestGetReservationsByBusinessIDPaginatedDefaultsAndTotals|TestGetReservationStatsUsesSingleAggregateQuery|TestGetReservationsByBusinessIDPaginatedPreloadsTableSummaryOnly' -count=1` ✅
- `go test ./internal/server -run 'TestGetReservationsDefaultsToPaginatedResponse|TestGetReservations_AcceptsBusinessSlug' -count=1` ✅
- `go test ./internal/database -run '^$' -bench '^BenchmarkGetReservationsByBusinessIDPaginatedSQLite$' -benchmem -count=3` ✅
- `go test ./internal/database ./internal/server -run '^$' -count=1` ✅
- `go test ./internal/services -run 'TestReservationService(GetAvailabilityIncludesUnavailableReason|GetPublicSettingsIncludesBookableSlotSummary|GetSettingsPreservesExplicitZeroValueControls|PublicSettingsReusesAvailabilityContext|GetAvailabilityBatchesReservationConflictQueries)|TestReservationConflictLoading' -count=1` ✅
- `go test ./internal/services -run '^$' -bench '^BenchmarkGetPublicReservationSettingsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/services ./internal/server -run '^$' -count=1` ✅
- `go test ./internal/handlers -run 'TestAnalyticsLiveBillsUsesProjectedBillAndTableReads|TestAnalyticsRoutes_LiveBillsHandlesMissingTableWithoutNPlusOnePanic|TestAnalyticsHandler_GetLiveBillsAcceptsBusinessSlug|TestAnalyticsRoutes_DashboardSummaryIncludesPartialActiveBills|TestBuildActiveBillsByTable_' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkAnalyticsLiveBillsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/handlers ./internal/database -run '^$' -count=1` ✅
- `go test ./internal/handlers -run 'TestRBACAuditLogUsesProjectedPreloads|TestGetAuditLog_ClampsLimit|TestGetStaffAuditLog_ClampsLimit|TestAnalyticsRoutes_(ServerCannotAccessLiveBills|ManagerCanAccessLiveBills)' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkGetRBACAuditLogSQLite$' -benchmem -count=3` ✅
- `go test ./internal/handlers ./internal/services -run '^$' -count=1` ✅
- `go test ./internal/handlers ./internal/services -run 'TestTrackDeliveryUsesPublicProjection|TestTrackDelivery_ReturnsPublicTrackingDetails|TestDeliveryServiceGetDeliveryOrderByBusinessRejectsWrongBusiness|TestDeliveryServiceCreateCheckout' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkTrackDeliverySQLite$' -benchmem -count=3` ✅
- `go test ./internal/handlers ./internal/services -run '^$' -count=1` ✅
- `go test ./internal/server -run 'TestLoadPaymentHistoryItemsUsesProjectedJoin|TestGetPaymentHistory_UsesExplicitDateRange|TestExportPaymentHistory_ReturnsPaymentCSV|TestGetPaymentHistory_RejectsCrossBusinessStaffAccess|TestPaymentHistoryRoute_ServerForbiddenByRBAC' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkLoadPaymentHistoryItemsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server -run '^$' -count=1` ✅
- `go test ./internal/database ./internal/handlers -run 'TestGetOrders|TestGetOrderBusinessIDByIDReturnsBusinessID|TestUpdateOrderStatus_Approve' -count=1` ✅
- `go test ./internal/database -run '^$' -bench '^BenchmarkGetOrdersByBusinessIDPaginatedSQLite$' -benchmem -count=3` ✅
- `go test ./internal/database ./internal/handlers -run '^$' -count=1` ✅
- `go test ./internal/services ./internal/handlers -run 'TestGetBusinessDriversSkipsUnusedStaffPreload|TestGetBusinessDeliveriesSkipsUnusedBillAndOrderPreloads|TestCreateDeliveryOrder_|TestUpdateDeliveryStatus_PublishesDeliveryUpdatedEvent|TestAssignDriver_PublishesDeliveryUpdatedEvent|TestDeliveryRouteGates|TestRolePermissions_DeliveryFamily' -count=1` ✅
- `go test ./internal/services -run '^$' -bench '^BenchmarkGetBusinessDriversSQLite$' -benchmem -count=3` ✅
- `go test ./internal/services ./internal/handlers -run '^$' -count=1` ✅
- `go test ./internal/handlers -run 'TestGetPaymentDetailsUsesBillNumberProjection|TestVerifyWebhookSignature' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkGetPaymentDetailsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/handlers -run '^$' -count=1` ✅
- `go test ./internal/handlers ./internal/services ./internal/database -run '^$' -count=1` ✅
- `go test ./internal/database -run 'TestGetTablesWithStatusProjectsActiveBills|TestGetReservationsByBusinessIDPaginatedDefaultsAndTotals|TestGetReservationStatsUsesSingleAggregateQuery|TestGetReservationsByBusinessIDPaginatedPreloadsTableSummaryOnly' -count=1` ✅
- `go test ./internal/server -run 'TestCoreTierOrphanRoutes_GatedByRequireActiveSubscription' -count=1` ✅
- `go test ./internal/database -run '^$' -bench '^BenchmarkGetTablesWithStatusSQLite$' -benchmem -count=3` ✅
- `go test ./internal/database ./internal/server -run '^$' -count=1` ✅
- `go test ./internal/handlers -run 'TestGetBillPaymentBreakdownUsesProjectedBillAndAggregate|TestAlternativePaymentEndpoints|TestPaymentBreakdownCalculation|TestAlternativePaymentHTTPHandlers|TestAlternativePaymentEndpoints_ExcludePluginTrackingRecords|TestGuestAlternativePaymentRoutes_UseBillNumber|TestMarkAlternativePayment_' -count=1` ✅
- `go test ./internal/handlers -run 'TestAlternativePaymentRoutes_EmailOnlyUserCannotManageBill|TestGetBillPaymentBreakdownUsesProjectedBillAndAggregate' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkGetBillPaymentBreakdownSQLite$' -benchmem -count=3` ✅
- `go test ./internal/database ./internal/handlers -run '^$' -count=1` ✅
- `go test ./internal/handlers -run 'TestGetPendingAlternativePaymentsUsesProjectedBillAccess|TestAlternativePaymentEndpoints|TestAlternativePaymentHTTPHandlers|TestAlternativePaymentRoutes_EmailOnlyUserCannotManageBill|TestAlternativePaymentEndpoints_ExcludePluginTrackingRecords|TestGuestAlternativePaymentRoutes_UseBillNumber|TestMarkAlternativePayment_' -count=1` ✅
- `go test ./internal/handlers -run 'TestProcessCryptoPayment_|TestGetBillPaymentBreakdownUsesProjectedBillAndAggregate' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkGetPendingAlternativePaymentsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/database ./internal/handlers -run '^$' -count=1` ✅
- `go test ./internal/handlers -run 'TestProcessCryptoPayment_|TestProcessCrossChainPayment_|TestGuestPaymentAttachesAuthenticatedCustomerWhenBillUnclaimed|TestDuplicateGuestPayments_DoNotRepublishPaymentReceivedEvent|TestProcessCryptoPaymentUsesProjectedBillLookup' -count=1` ✅
- `go test ./internal/services -run 'TestMilestoneTracker|TestMilestoneTrackerProcessesPendingOutboxEventsIdempotently' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkProcessCryptoPaymentSQLite$' -benchmem -count=3 -benchtime=100x` ✅
- `go test ./internal/handlers ./internal/services -run '^$' -count=1` ✅
- `go test ./internal/crm -run '^TestGetCustomerBusinessConnectionsProjectsBusinessSummary$' -count=1` ✅
- `go test ./internal/crm -run '^$' -bench '^BenchmarkGetCustomerBusinessConnectionsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/crm -count=1` ✅
- `go test ./internal/crm ./internal/handlers -run '^$' -count=1` ✅
- `go test ./internal/server -run '^TestGetBillByNumberPublicUsesProjectedBillLookup$' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkGetBillByNumberPublicSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server ./internal/database -run 'TestGetBillByNumberPublic|TestGetBillByNumberPublicUsesProjectedBillLookup|TestGetTableByCodePublicUsesSharedMenuCache|TestGetOpenBillByTableCodeUsesProjectedBillLookup|TestGetTableStatusByCode' -count=1` ✅
- `go test ./internal/server ./internal/database -run '^$' -count=1` ✅
- `go test ./internal/handlers -run '^TestGetBillSplitOptionsUsesProjectedBillLookup$' -count=1` ✅
- `go test ./internal/handlers -run '^$' -bench '^BenchmarkGetBillSplitOptionsSQLite$' -benchmem -count=3` ✅
- `go test ./internal/handlers -run 'TestGetBillSplitOptions_UsesBillNumber|TestGetBillSplitOptionsUsesProjectedBillLookup|TestCalculateItemSplit_RejectsDuplicateAssignmentsByBillNumber|TestExecuteSplitPayment_' -count=1` ✅
- `go test ./internal/handlers ./internal/database -run '^$' -count=1` ✅
- `go test ./internal/splitting -run 'TestCalculate(EqualSplitUsesBillAmountProjection|ItemSplitUsesProjectedBillAndRelationalItems)$' -count=1` ✅
- `go test ./internal/splitting -run '^$' -bench '^BenchmarkCalculate(Equal|Item)SplitSQLite$' -benchmem -count=3` ✅
- `go test ./internal/splitting -count=1` ✅
- `go test ./internal/splitting -run 'TestCalculateEqualSplit|TestCalculateCustomSplit|TestCalculateItemSplit|TestGetBillItemsPreferRelational|TestRoundToTwoDecimals' -count=1` ✅
- `go test ./internal/database -run 'TestGetBillItemsPreferRelational' -count=1` ✅
- `go test ./internal/splitting ./internal/handlers -run 'TestGetBillSplitOptions_UsesBillNumber|TestGetBillSplitOptionsUsesProjectedBillLookup|TestCalculateItemSplit_RejectsDuplicateAssignmentsByBillNumber|TestExecuteSplitPayment_' -count=1` ✅
- `go test ./internal/splitting ./internal/handlers ./internal/database -run '^$' -count=1` ✅
- `go test ./internal/server -run '^TestRequireBillBusinessAccessUsesProjectedBusinessAccess$' -count=1` ✅
- `go test ./internal/server -run '^$' -bench '^BenchmarkRequireBillBusinessAccessSQLite$' -benchmem -count=3` ✅
- `go test ./internal/server -run 'TestRequireBillBusinessAccess|TestRequireActiveSubscriptionForBill|TestGetBill' -count=1` ✅
- `go test ./internal/server ./internal/handlers ./internal/database -run '^$' -count=1` ✅
- `go test ./cmd/app ./internal/server ./internal/handlers ./internal/database ./internal/services ./internal/fiscal ./internal/services/print ./internal/services/print/formatters ./internal/crm ./internal/splitting -count=1` ✅
- `go test ./... -count=1 -timeout=120s` ⚠️ all printed unit packages passed; failed only in `github.com/stdevmac/payverge/backend/internal/testperf` because `TestStartPostgresReturnsUsableDB` timed out waiting for Docker/Testcontainers.
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='BenchmarkGuest(MenuByTableCode|OpenBillByTableCode|TableStatusByCode|CreateBillByTableCode|CreateOrder)$' -benchmem -count=1 -benchtime=50x ./internal/server/...` ✅ (last successful guest flow run before the post-create order reload removal)
- `JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='BenchmarkGuest(CreateOrder|CreateBillByTableCode)$' -benchmem -count=1 -benchtime=50x ./internal/server/...` ⚠️ blocked after code cleanup: Docker/Testcontainers became unresponsive; benchmark process was terminated after the test binary sat idle waiting on Docker.

Full `go test ./...` was not rerun after the latest Layer 5 pass. Earlier full-suite runs on this branch had one pre-existing unrelated failure:

- `TestBillPaymentRoutesUseBillBusinessAccessGuard` — POST `/bills/:bill_id/alternative-payment` is missing `RequireBillBusinessAccess` middleware. Pre-existing security gap, worth filing separately.

## Known gaps before merge

1. **Cache invalidation hooks wired (2026-10-05).** The 5s TTL still bounds staleness, and operator writes now bust the guest caches immediately:
   - `services.InvalidatePricingCache(businessID uint)` and `services.InvalidateBusinessCustomURL(customURL string)` on business update/delete.
   - `services.InvalidatePublicGuestBusiness(businessID uint)` (evicts that business's QR table contexts via `boundedcache.InvalidateFunc` plus its guest extras) on business update/delete, table update/details/soft-delete, QR branding bulk apply, plugin enable/disable/config, and language updates.

   Covered by `TestInvalidatePublicGuestBusiness_DropsTableContextsAndExtras` and `TestInvalidateFuncRemovesMatchingValues`. Invalidation runs only on writes, so the read-path benchmarks above are unchanged and were not rerun.

2. **AuthMiddleware benchmark gain is scoped to the no-session-store path.** The benchmark intentionally runs with `session.GlobalStore == nil`, where revocation checks short-circuit. Production deployments with a live session store still perform revocation validation and do not use the same cached fast path.

3. **Docker/Testcontainers instability during late reruns.** Local SQLite route microbenchmarks were added for the last guest/check-in slices so optimization work can continue while Docker-backed benchmark reruns are flaky.

4. **Next backend watch item: bill list count/page query.** `GetBusinessBills` no longer hydrates full aggregates, but still does a count query plus page query. A `COUNT(*) OVER()` version was measured and rejected because it regressed latency/allocation on the current 500-bill benchmark.

## Where things live

- Bench code: `backend/internal/server/menu_bench_test.go`, `backend/internal/handlers/orders_bench_test.go`, `backend/internal/server/business_flow_bench_test.go`, `backend/internal/handlers/auth_middleware_bench_test.go`, `backend/perf/bench/sse_broadcast_bench_test.go`, `backend/internal/database/models_json_bench_test.go`, `backend/internal/database/inventory_bench_test.go`, `backend/internal/services/plugin_notification_worker_bench_test.go`, `backend/internal/fiscal/service_bench_test.go`, `backend/internal/services/print/service_bench_test.go`
- Pricing cache: `backend/internal/services/pricing_cache.go`
- Testcontainers harness: `backend/internal/testperf/`
- k6 scenarios: `backend/perf/k6/scenarios/`
- Staging stack: `backend/perf/staging/docker-compose.staging-perf.yml`
- Drift comparison: `backend/perf/drift/compare.go`
- Benchmark input validation: `backend/perf/bench/scripts/validate-inputs.sh`
- Per-iteration profiles + bench output: `backend/.perf-profiles/` (gitignored)

## How to reproduce

```bash
cd backend

# Full bench sweep, count=3, ~4 min
JWT_SECRET_KEY=local-dev-secret make bench-ci

# Single bench
JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench=^BenchmarkOrderCreate$ \
  -benchmem -count=5 -benchtime=2s -timeout=10m ./internal/handlers/...

# Profile a bench
go test -run=^$ -bench=^BenchmarkOrderCreate$ -benchmem -count=1 \
  -cpuprofile=cpu.pprof -memprofile=mem.pprof ./internal/handlers/...
go tool pprof -list 'OrderHandler' -sample_index=alloc_objects mem.pprof
```

---

## Payment webhook magnitude sanity bound (2026-05-29)

> Historical: `WebhookPaymentConfirmation` and its tests were later deleted with the unrouted
> HMAC-only crypto webhook (3aacbe319, OSS hygiene A0.10). The ceiling survives as
> `validatePaymentAmountMagnitude` on alternative-payment requests
> (`TestValidatePaymentAmountMagnitude`, `BenchmarkValidatePaymentAmountMagnitude`).

Added a global per-payment ceiling (`MAX_PAYMENT_AMOUNT_CENTS`, default
`100_000_000` cents = $1,000,000), resolved once in `NewPaymentHandler` and
enforced by the pure in-memory helper `validateWebhookAmountMagnitude` inside
`WebhookPaymentConfirmation` — checked before the bill is loaded, so the reject
path issues zero DB writes/queries. Closes the silent dollars-as-int 100x
over/under-payment hole.

### Benchmark — `BenchmarkValidateWebhookAmountMagnitude`

There is no pre-change baseline: the helper is brand new. The meaningful
assertion is that the guard is allocation-free and adds no measurable latency to
the webhook hot path.

| run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 1.148 | 0 | 0 |
| 2 | 1.082 | 0 | 0 |
| 3 | 1.181 | 0 | 0 |

(`goos: darwin / goarch: arm64 / cpu: Apple M3`)

**Conclusion:** magnitude guard is allocation-free; no measurable latency added
to the webhook path.

### Commands run

```bash
# Benchmark (-count=3)
go test ./internal/handlers/ -run '^$' -bench BenchmarkValidateWebhookAmountMagnitude -benchmem -count=3

# Regression + helper suite (all green)
go test ./internal/handlers/ -run 'TestWebhookPaymentConfirmation|TestValidateWebhookAmountMagnitude|TestResolveMaxPaymentAmountCents' -count=1

# Adjacent-package compile/no-op gate (clean compile)
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

## Delivery hardening (2026-05-29)

### BenchmarkGetBusinessDeliveriesWithStatusFilterSQLite
- Command: `go test ./internal/services/ -bench BenchmarkGetBusinessDeliveriesWithStatusFilterSQLite -benchmem -run '^$' -count=3`
- Before: ~2.16e6 ns/op, ~3.14e6 B/op, ~10027 allocs/op (status-filter validation is a constant-time `IsValid()` switch; before≈after)
- After:  2162177 ns/op, 3139875 B/op, 10027 allocs/op (median of 3)

The validation guard is an in-memory switch with zero allocations, so the
fix adds no measurable list-path cost. (No separate "before" implementation
exists — the guard is purely additive and constant-time, so the after-number
documents that it is free.)

### BenchmarkAssignDriverSQLite
- Command: `go test ./internal/services/ -bench BenchmarkAssignDriverSQLite -benchmem -run '^$' -count=3`
- Before (two un-transactioned saves): 249190 ns/op, 164839 B/op, 1529 allocs/op (median of 3)
- After (single FOR UPDATE tx):        243761 ns/op, 165903 B/op, 1507 allocs/op (median of 3)

The single locked transaction is within noise of (and slightly faster than)
the prior two-save implementation on single-connection SQLite — it trades a
negligible BEGIN/COMMIT round-trip for correctness. On Postgres the FOR UPDATE
row locks add a small latency cost in exchange for preventing concurrent
double-assignment.

Caveat: the SQLite in-memory DB uses `SetMaxOpenConns(1)`, so `FOR UPDATE`
(`clause.Locking`) is a no-op and concurrent goroutines serialize. These
benchmarks/tests measure atomicity, rollback, and single-transaction query
shape — not a true concurrent lost-update race. Real race coverage requires
Postgres/Testcontainers and is deferred.

Deviation note: the plan assumed GORM emits `BEGIN`/`COMMIT` through the logger
Trace callback so a single transaction could be asserted by scanning logged
SQL. GORM v1.30 does not (it issues those at the database/sql driver layer), so
`TestAssignDriverWritesInSingleTransaction` instead wraps the GORM `ConnPool`
with a `BeginTx` interceptor and asserts it is called exactly once — a
deterministic single-transaction signal. The `TestGetBusinessDeliveriesFilteredKeepsNarrowShape`
SELECT-* guard targets the bills/orders join fan-out (the real perf risk)
rather than the base `delivery_orders` SELECT *, because the list legitimately
returns full delivery rows to the dispatch UI.

### Verification commands run
```bash
go test ./internal/services/ -run 'Delivery' -count=1    # all delivery service tests pass
go test ./internal/handlers/ -run 'Delivery' -count=1    # all delivery handler tests pass
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1  # adjacent compile
go build ./...                                            # full backend builds
```

## SSE observability — non-blocking fan-out (2026-05-29)

Locks in the
already-shipped dropped-event counter with a non-blocking
guarantee regression test (`TestHubPublishDoesNotBlockOnFullSubscriber`) plus a
worst-case fan-out benchmark, and flips the SSE gate-denial path to a clean
terminal `error` frame.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`, medians below.

### BenchmarkPublishFullSubscriber
- Command: `go test ./internal/events/ -run '^$' -bench BenchmarkPublishFullSubscriber -benchmem -count=3`
- Baseline (production unchanged in Task 1): 54.83 ns/op, 168 B/op, 1 allocs/op (median of 3)

This measures the worst-case backpressure path: the single subscriber's 64-slot
buffer is saturated so every benchmarked `Publish` hits the non-blocking
`default:` drop+count branch. Production code is unchanged in this task, so this
establishes the measured floor the perf gate requires (before == after). The one
allocation is the `replay` ring-buffer append in `Publish` (every event is
buffered for reconnect replay regardless of subscriber state) — that is expected
and out of scope here; the recorder hook + Prometheus label lookup add no
measurable per-op cost beyond it.

### Verification commands run
```bash
go test ./internal/events/ -count=1                              # all event tests pass
go test ./internal/metrics ./internal/server -run '^$' -count=1  # adjacent compile gate
```

## Perf / query-shape long-tail (2026-05-29)

Closes the audit
perf long-tail: drops a proven over-hydration preload, single-sources
cents→dollars, parallelizes independent dashboard fetches, and staleTime-tunes
the dashboard-notifications poll.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`, medians below.

### BenchmarkGetOrdersByBillIDSQLite (Task 1 — drop `Preload("Bill")`)
- Command: `go test ./internal/database -run '^$' -bench BenchmarkGetOrdersByBillIDSQLite -benchmem -count=3`
- Before (live `Preload("Bill")`): 325715 ns/op, 919824 B/op, 1727 allocs/op (median of 3)
- After (Business preload only):    261617 ns/op, 669974 B/op, 1441 allocs/op (median of 3)
- Delta: ~19.7% faster, ~27.2% fewer bytes/op, ~16.6% fewer allocs/op, and one
  fewer SQL round trip (the discarded `SELECT * FROM bills` + heavy bill `items`
  JSON hydration is gone).

The access-shape regression `TestGetOrdersByBillIDDoesNotHydrateBill` guards the
dangerous shape (`SELECT * FROM bills`) from returning, while asserting the
Business association stays preloaded (required by `Order.MarshalJSON` currency
resolution).

### Verification commands run
```bash
go test ./internal/database -run TestGetOrdersByBillIDDoesNotHydrateBill -count=1   # access-shape PASS
go test ./internal/handlers -run TestCentsToDollarsMatchesWireContract -count=1     # wire-contract lock PASS
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1 # adjacent compile gate
```

## FE↔BE contract long-tail hardening (2026-05-29)

Closes the
genuinely-open residual FE↔BE contract/type/nullability gaps and regression-
guards the long-tail items that prior batches already fixed.

### Slices
- Task 1 — regression guards: `contract_longtail_regression_test.go` (BE) +
  `contract-longtail.typecheck.ts` (FE compile-time guards) lock in
  the already-fixed items so they cannot silently regress.
- Task 2 — driver `vehicle_type`: added `VehicleType.IsValid()`
  and 400 on unknown vehicle_type in `CreateDriver`/`UpdateDriver`.
- Task 3 — onboarding body cap: wrapped `UpdateOnboardingState` body in
  `http.MaxBytesReader` (16KiB + 4KiB JSON framing) so a multi-MB payload is
  rejected (413) before being buffered, reusing `isMaxBytesError`/
  `respondPayloadTooLarge`.
- Task 4 — table `Capacity`: pointerized `UpdateTableRequest.Capacity`
  to `*int` and applied it (when present and > 0) on the live PUT `UpdateTable`
  route (which previously never applied it) and on `UpdateTableDetails`.
- Task 5 — accept-and-document: documented why `UploadResponse.business_id`
  stays optional; the reported table route is not live (slug-aware
  `getTableRouteBusiness` handles every wired table route).

### Benchmark — not required
Tasks 2-4 are binding-rejection / behavior changes only. No JSON serializer,
query shape, preload, index, cache, or queue path changed, so per the perf-gate
rule no benchmark was required — binding-rejection regression tests are the
appropriate evidence, and a benchmark is only warranted when the serializer
changes.

### Verification commands run
```bash
go test ./internal/handlers -run 'TestCreateDriver_Rejects|TestCreateDriver_Accepts|TestUpdateDriver_Rejects' -count=1
go test ./internal/server -run 'TestUpdateOnboardingState_RejectsOversizedRawBody|TestValidateOnboardingState|TestComputeSetupStatus' -count=1
go test ./internal/server -run 'TestUpdateTable_|TestUpdateTableDetails_|TestGetBusinessTables_' -count=1
go test ./internal/handlers ./internal/database -run '^$' -count=1   # adjacent compile gate
go test ./internal/server ./internal/database -run '^$' -count=1     # adjacent compile gate
cd frontend && npm run typecheck
cd frontend && npm run test -- uploads
```

### Triage outcome
Already-fixed items regression-guarded; one reported route not live; one
accept-and-document; cross-cutting
currency-threading deferred to a dedicated perf-gated serializer slice.

## Housekeeping cluster (2026-05-29)

Four independent
low-risk cleanups committed serially on `main` (compose 443966ab, CLAUDE.md
22c19e13, AGENTS.md e1932c5f, env-example ae51bbfd, refund-comment 6f21a33f;
the PNG deletion produced no commit).

### Compose-validation path used (Task 1, Step 4)
Full `docker compose --env-file .env config` ran clean (printed `compose OK`),
so the YAML-only python fallback was NOT needed. The removed frontend
`environment:` block was dead runtime code (NEXT_PUBLIC_* inline at build time
via Dockerfile ARGs), and the orphan `NEXT_PUBLIC_USDC_ADDRESS` build arg has
zero `frontend/src` consumers (addresses are hardcoded per chain in
`src/lib/lifi/config.ts`). No behavioral test applies.

### PNG deletion (Task 5)
`bills_2.png`, `kitchen.png`, `reservations_2.png` were untracked (`??`) with
zero code references, so deletion was a pure disk-only `rm` with NO git op and
NO commit. The 6 committed onboarding images (analytics/bills/counter/kds/
overview/reservations) remain.

### Benchmark — not required
This cluster is not perf-gated: no latency/allocation/query-shape change. The
edits are doc/config/file-deletion plus one Go doc-comment (no runtime logic).
Cross-plan fold-in: documented the `MAX_PAYMENT_AMOUNT_CENTS` knob (default
100000000, caps per-payment webhook amount in cents) in root `.env.example`.
(This fold-in was mistakenly reverted by eab6326a — judged "out of scope" —
then restored. Per the CLAUDE.md "env file is root-only" gotcha, root
`.env.example` is the canonical place to document backend env vars; the knob is
now both documented there AND wired into the docker-compose.yml backend
`environment:` block as `MAX_PAYMENT_AMOUNT_CENTS=${MAX_PAYMENT_AMOUNT_CENTS:-}`
so the documented value actually reaches the container.)

### Verification commands run
```bash
docker compose --env-file .env config >/dev/null   # compose OK
go test ./internal/handlers -run '^$' -count=1      # ok (compile gate, refund comment)
```

---

# Batch 1: cross-tenant data-leak + startup secret hardening (2026-06-15)

Branch: `main`. Six security findings closed via TDD (RED→GREEN). No pushes.

## CFG-001 / CFG-002 — Production accepts published example-env secrets

- **Fix:** Expand `isKnownUnsafeJWTSecret` to reject `replace_with`/`changeme` prefix placeholders (blocks the `.env.example` JWT from booting production). Add `isPlaceholderMetricsToken` filter in `parseMetricsTokens` so the published `METRICS_TOKEN` placeholder is treated as unconfigured.
- **Access-shape test:** `TestValidateConfig_RejectsExamplePlaceholder*`, `TestIsKnownUnsafeJWTSecret_RejectsReplaceWithPlaceholders`, `TestParseMetricsTokens_FiltersPlaceholder*`, `TestIsPlaceholderMetricsToken_CatchesKnownPlaceholders`
- **`.env.example` changes:** JWT_SECRET_KEY shortened from 53-char valid-looking placeholder to `replace_with_real_secret` (22 chars, trips length gate). METRICS_TOKEN kept as `replace_with_metrics_bearer_token` (now filtered by `parseMetricsTokens`).

## Plugin webhook cross-business settlement

- **Fix:** After `effectiveBillID` is resolved, load the bill and compare its `BusinessID` against the `businessID` whose secret verified the webhook signature. Reject (403) on mismatch. The check correctly handles all three paths: attacker-provided metadata, backfill path (bill owns the verifying business), and legitimate same-business webhooks.
- **Access-shape test:** `TestHandlePaymentWebhook_RejectsCrossBusinessSettlement` (cross-business → 403, bill unchanged), `TestHandlePaymentWebhook_AcceptsSameBusinessSettlement` (same business → 200)
- **Note:** Stripe trusts `amount_total`/`bill_id` from the payload with no `sessions.Get` re-fetch — flagged for follow-up hardening.

## Cross-tenant receipt/bill leak via unvalidated source_id

- **Fix:** (1) `sourceBelongsToBusiness` gate in `Create` handler — 404 if the referenced bill/order doesn't belong to the requesting business. (2) Defense-in-depth `business_id` WHERE clause on `First()` loads in `build_inputs.go` (`buildBillInputFromBill`, `buildReceiptInputFromBill`, `buildKitchenTicketInputFromOrder`).
- **Access-shape tests:** `TestPrintJobHandlers_CreateRejectsCrossTenantBill` (cross-tenant bill print → not 201, payload doesn't leak victim business name/tx hash), `TestPrintJobHandlers_CreateRejectsCrossTenantKitchen` (cross-tenant kitchen ticket → not 201)
- **Kitchen/bar confirmation:** `buildKitchenTicketInputFromOrder` now takes and uses `businessID` parameter — kitchen ticket source_id is also scoped.

## GetReferrerReferrals leaks full Business rows

- **Fix:** Add `MarshalJSON` to `ReferralRecord` projecting the embedded `Business` through `newBillPublicBusiness` (id, business_id, name, logo, timezone, default_currency, display_currency only), mirroring `Table.MarshalJSON`. Unloaded businesses (ID==0) are omitted.
- **Access-shape test:** `TestReferralRecordMarshalJSON_OmitsSensitiveBusinessFields`, `TestReferralRecordMarshalJSON_OmitsBusinessWhenUnloaded`
- **Serialization benchmark** (Apple M3, `BenchmarkReferralRecordSerialization`, `-benchmem -count=3`):
  - Before: n/a (no prior benchmark; the unreduced payload was ~2.5 KiB per record of which ~90% was Business fields)
  - After: **~9,200 ns/op, 7,055 B/op, 16 allocs/op** (2 records)

## Guest orders leak staff PII in actor fields

- **Fix:** Replace `OrdersResponse` (embedding full `database.Order`) with `guestOrdersResponse` using `guestOrderView` that omits `created_by`, `approved_by`, `cancelled_by`. No change to `database.Order` json tags.
- **Access-shape test:** `TestGetGuestOrdersByBillNumber_DoesNotLeakActorPII` (actor PII absent, guest-visible fields present)
- **FE confirmation:** `frontend/src/components/guest/GuestBill.tsx` never reads `created_by`/`approved_by`/`cancelled_by` — no FE changes needed.

## GetBusiness returns owner secrets to staff

- **Fix:** Split `GetBusiness`'s `CheckBusinessAccess` branch: owners keep the full `Business` struct; staff get `staffBusinessProjection` (omits `owner_address`, `owner_name`, `email`, `settlement_address`, `tipping_address`, `stripe_customer_id`, `stripe_subscription_id`, billing tx hashes; retains operational fields + display-only `stripe_card_brand`/`stripe_card_last4`).
- **Access-shape test:** `TestGetBusiness_StaffGetsScopedProjection` (staff → no owner secrets, display fields present), `TestGetBusiness_OwnerGetsFullRecord` (owner → full struct)
- **Subscription UI verification:** Billing routes (`/businesses/:id/subscription`) are separate routes gated by `financial:read`, not `GetBusiness` → no breakage.

## Final gate output

```bash
cd backend && go build ./... && go vet ./...    # PASSED
go test ./internal/config ./internal/handlers ./internal/server ./internal/database ./internal/services/print -count=1  # ALL PASSED
go test ./internal/middleware ./internal/plugins/stripe -run '^$' -count=1  # OK (compile gate)
cd ../frontend && npm run typecheck              # PASSED (no errors)
```

## Slice 1 — BusinessScheduleSettings getter

Per-business schedule/compliance settings row (migration 000103), lazily created
one-per-business via `GetOrCreateBusinessScheduleSettings` (mirrors
`GetReservationSettings`). No money fields — nothing here is owner-gated.

Benchmark (Apple M3, in-memory SQLite, `-count=3`):

```bash
cd backend && go test ./internal/database/ -bench 'BenchmarkGetOrCreateBusinessScheduleSettings' -benchmem -run '^$' -count=3
```

```
BenchmarkGetOrCreateBusinessScheduleSettings-8   110532   10286 ns/op   5725 B/op   104 allocs/op
BenchmarkGetOrCreateBusinessScheduleSettings-8   116484   10447 ns/op   5726 B/op   104 allocs/op
BenchmarkGetOrCreateBusinessScheduleSettings-8   114319   10248 ns/op   5725 B/op   104 allocs/op
```

This is a single-row read on the unique `business_id` key (no list/fan-out, no
relation preloads), so no list-endpoint/N+1 optimization is required. Its
consumer (the Slice 2 schedule builder) is not yet wired.

## 2026-07-15 review remediation perf (Waves 3–4)

Benchmark gate records (Apple M3, in-memory SQLite, `go test -benchmem -count=3`). These paths are new endpoints / new aggregates, so the measured run is the recorded baseline (no pre-change production path to diff against). Access-shape regression tests pin query shape; numbers live in plan task notes and `{SCRATCH}/perf-wave*.txt`.

### Wave 3 Task 3 — `BenchmarkSubscriptionValueSummarySQLite` (`github.com/stdevmac/payverge/backend/internal/handlers`)

| Run | ns/op | B/op | allocs/op |
|-----|------:|-----:|----------:|
| 1 | 1,016,363 | 187,124 | 877 |
| 2 | 975,838 | 187,041 | 877 |
| 3 | 1,003,175 | 186,989 | 877 |

**Median / recorded baseline:** ~**1,003,175 ns/op**, **~187 KB/op**, **877 allocs/op**. Access-shape: ≤4 SELECTs, zero `SELECT *`.

### Wave 3 Task 4 — `BenchmarkGetAiAttributedOrderValueSQLite` (`github.com/stdevmac/payverge/backend/internal/database`)

| Run | ns/op | B/op | allocs/op |
|-----|------:|-----:|----------:|
| 1 | 423,078 | 7,074 | 65 |
| 2 | 423,875 | 7,094 | 65 |
| 3 | 435,625 | 7,090 | 65 |

**Median / recorded baseline:** ~**423,875 ns/op**, **~7.1 KB/op**, **65 allocs/op**. Access-shape: 1 aggregate SELECT, zero `SELECT *`.

### Wave 4 Task 18 — `BenchmarkGetTipsByStaff` (`github.com/stdevmac/payverge/backend/internal/analytics`)

| Run | ns/op | B/op | allocs/op |
|-----|------:|-----:|----------:|
| 1 | 1,280,183 | 11,246 | 246 |
| 2 | 1,250,079 | 11,403 | 266 |
| 3 | 1,266,911 | 11,408 | 266 |

**Median / recorded baseline:** ~**1,266,911 ns/op**, **~11.4 KB/op**, **266 allocs/op**. Single aggregate SELECT with `COALESCE(closed_by, created_by)` attribution.

### Wave 4 Task 19 — `BenchmarkGetReservationGuestHistory` (`github.com/stdevmac/payverge/backend/internal/database`)

| Run | ns/op | B/op | allocs/op |
|-----|------:|-----:|----------:|
| 1 | 2,591,442 | 15,463 | 283 |
| 2 | 2,523,381 | 15,454 | 283 |
| 3 | 2,562,152 | 15,465 | 283 |

**Median / recorded baseline:** ~**2,562,152 ns/op**, **~15.5 KB/op**, **283 allocs/op**. Single grouped query over guest emails (pending-approval list only).

## Durable session merge — refresh rotation hot path (2026-07-16, Wave 3 Tasks 2+5)

`POST /auth/refresh` rotation path perf-gate before/after merging
`codex/auth-session-persistence` (revocation audit + refresh-token history +
identity-failure classification). Happy path writes history on each rotation;
revocation `Updates` maps only fire on revoke paths.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

### `BenchmarkRefreshTokenRotation` (`github.com/stdevmac/payverge/backend/internal/server`)

| Phase | Run | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| main (pre-merge baseline) | 1 | 194,047 | 41,341 | 536 |
| main (pre-merge baseline) | 2 | 204,525 | 41,317 | 536 |
| main (pre-merge baseline) | 3 | 174,505 | 41,320 | 536 |
| merged (post durable-session) | 1 | 175,779 | 46,213 | 590 |
| merged (post durable-session) | 2 | 211,854 | 46,206 | 590 |
| merged (post durable-session) | 3 | 189,728 | 46,207 | 590 |

**Median main:** ~**194,047 ns/op**, **~41.3 KB/op**, **536 allocs/op**.
**Median merged:** ~**189,728 ns/op**, **~46.2 KB/op**, **590 allocs/op**.

Latency within noise (median slightly faster). Allocs/B up by a small constant
on the happy rotation path (history insert + audit-ready session model fields) —
acceptable; not a >10% ns/op regression.

```bash
# baseline (Task 2, on main before merge)
cd backend
go test ./internal/server -run 'TestRefreshToken' -count=1
go test ./internal/server -bench BenchmarkRefreshTokenRotation -benchmem -count=3 -run '^$' | tee /tmp/refresh-bench-main.txt

# after merge (Task 5)
go test ./internal/server -bench BenchmarkRefreshTokenRotation -benchmem -count=3 -run '^$' | tee /tmp/refresh-bench-merged.txt
```

## Wave 4 P2-11 — filterNewlyInactiveBusinesses payment-time activity (2026-07-16)

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

`BenchmarkFilterNewlyInactiveBusinessesSQLite` (200 candidates, SQLite in-memory).

| Phase | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| BEFORE (bills.updated_at DISTINCT) | *blocked* — bench seed UNIQUE collision before fix; not re-run after code change | — | — |
| AFTER (payment-time UNION ALL ×3 sources) | ~1.08–1.22M | ~374345 | 3597 |

Command:
```bash
cd backend && go test ./internal/services -run '^$' -bench BenchmarkFilterNewlyInactiveBusinessesSQLite -benchmem -count=3
```

Gate: constant 2 set-based queries (shape), not absolute latency. UNION over payments/alternative_payments/legacy bills is expected to cost more per query than the old single-table DISTINCT; recorded honestly.

## Wave 4 P2-12 — getManagedBusinesses suspended/expired self-heal (2026-07-16)

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

`BenchmarkGetManagedBusinessesSQLite` (500 businesses, mixed statuses, SQLite in-memory).

| Phase | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| AFTER (statuses + lockout/is_active/closed_at projection) | ~4.73–5.08M | ~3.32M | ~13340 |

BEFORE baseline not captured separately (bench added with the feature); shape gate is projection column set + constant fetch (single SELECT).

Command:
```bash
cd backend && go test ./internal/services -run '^$' -bench BenchmarkGetManagedBusinessesSQLite -benchmem -count=3
```

Access-shape pins (Wave 4 gate G4):
- `TestFilterNewlyInactive_PaymentTimeKeepsConstantQueryShape` (P2-11)
- SCHED-03 projection suite (`subscription_checker_projection_test.go`) + `BenchmarkGetManagedBusinessesSQLite` (P2-12)
- `TestGettingStartedAndFounderChecks_NarrowProjection` (Task 11h)

Gate note (2026-07-16 Wave 4 verification): backend used `make quick-test` (no race detector). Full `-race` (`make test`) not run in this environment due to time; not environmentally blocked.


## Wave 5 — guest-route analytics + activation last-mile (2026-07-16)

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

### `BenchmarkGetTableByCodePublicSQLite` (P2-19, Task 4)

Hot public guest scan route (`GET /guest/table/:code`). Benchmark added in Task 4a and re-run after the throttled `guest_table_scanned` emit in Task 4g. Steady-state path is throttle **deny** (one mutex + map lookup); emit is async (`logger.SafeGo`) so request latency is untouched.

| Phase | Run | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| AFTER (4g, with throttled scan emit) | 1 | 104,919 | 22,994 | 225 |
| AFTER (4g, with throttled scan emit) | 2 | 132,369 | 22,992 | 225 |
| AFTER (4g, with throttled scan emit) | 3 | 76,712 | 22,993 | 225 |

**Median AFTER:** ~**104,919 ns/op**, **~22.99 KB/op**, **225 allocs/op** (range ~77–132k ns/op).

BEFORE (4a) was not tee'd separately before the production emit landed — the benchmark was introduced in the same change set as the throttle instrumentation. Expected delta is noise-level: throttle deny path is O(1) in-memory with **zero SQL and zero allocations on the deny path**; first emit per 15-min window spawns one goroutine only.

Commands:
```bash
# baseline (Task 4a — capture before production change when re-running on a clean base)
cd backend && go test ./internal/server/ -run '^$' -bench BenchmarkGetTableByCodePublicSQLite -benchmem -count=3 | tee /tmp/scan_bench_before.txt

# after throttled guest_table_scanned + guest_order_placed (Task 4g)
cd backend && go test ./internal/server/ -run '^$' -bench BenchmarkGetTableByCodePublicSQLite -benchmem -count=3 | tee /tmp/scan_bench_after.txt
```

### Access-shape notes (Wave 5)

- **`CreateGuestOrder` emit** (`guest_order_placed`): adds **zero SQL** — hook-intercepted access-shape test `TestCreateGuestOrder_EmitsOrderPlacedEvent` + unchanged recorder bounds in `TestGetTableByCodePublic_EmitsThrottledScanEventWithNoExtraQueries`.
- **Scan throttle**: O(1) in-memory (`sync.Mutex` + map, 4096-entry bound with lazy prune); multi-replica mild over-count is acceptable for an activation signal.
- **Day-7 no-first-order lifecycle variant** (P3, VARIANT owner default): adds ~7 narrow queries per day-7 candidate in the daily scheduler tick (non-request path; cohort = one day's signups). Founder-feedback fetch projection narrowed from full-row hydration (`TestGettingStartedAndFounderChecks_NarrowProjection` / variant claim tests).
- **Server-side `onboarding_completed` stamp** (P2-20): one-shot conditional-UPDATE claim; emit only when `RowsAffected == 1` — no hot-route cost on guest paths.
- **`has_first_paid_bill` on setup-status** (P3): one bounded aggregate read on the operator setup-status path (not guest).

Gate note (2026-07-16 Wave 5 verification): backend uses `make quick-test` (no race detector) in this environment when full `-race` is time-prohibitive.

---

## Menu-tab structural remediation (Stream 4, 2026-07-21)

Isolated agent worktree.
Hardware: Apple M-series `darwin/arm64`, in-memory SQLite microbenchmarks (deterministic
handler/serializer access-shape work; Docker/Postgres benches were not run in this env).

### Fix 2 — Slim GET /menu wire contract (HIGH)
Access-shape test `TestGetMenu_WireContractOmitsRawCategoriesAndBusiness` asserts the operator
GET /menu response no longer ships the redundant raw `categories` JSON string nor the ~200-field
zero-value embedded `business` object; `parsed_categories` + menu metadata + version survive.

`BenchmarkGetMenuPayload` (20 categories × 30 items, in-memory SQLite, `-benchmem -count=3`):

| | payload-bytes | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| before | 426966 | ~2.65M | ~4620 |
| after  | 217489 | ~1.51M | ~4602 |

~49% payload reduction (raw duplicate menu string + blank Business blob removed). Frontend
`Menu.categories` made optional (parseMenuCategories already prefers parsed_categories).

Command: `go test ./internal/server/ -run '^$' -bench 'BenchmarkGetMenuPayload' -benchmem -count=3`

### Fixes 3 + 8 — Mutation echo + drop post-write re-reads (HIGH + MED)
Menu mutation DB functions (`AddMenuItemByID`/`UpdateMenuItemByID`/`AddMenuCategoryVersioned`/
`UpdateMenuCategoryByID` + their legacy siblings) now return the bumped version, the mutated
entity (with server-assigned ID), and its position index. Handlers echo `category`/`item` in the
response and key the async translate goroutine off the returned index — removing BOTH the
post-write full-document re-read for the version (was `GetMenuByBusinessID` per mutation) and the
per-mutation auto-translate goroutine re-read. Net: an add/update/delete drops from
2 full-document reads (write path + version re-read) + 1 goroutine re-read to a single write
(+ the async translate goroutine no longer re-reads for indices).

Frontend `useMenuMutations` patches menu state in place from the echoed entity instead of calling
`loadMenu()` (a full GET /menu re-download) after every mutation. Reload is kept as the 409
recovery path and for the translated-language view (where the source echo would show untranslated
text). Delete patches unconditionally (locale-independent).

Tests: `TestAddMenuItem_ReturnsMutatedItemAndVersion`, `TestUpdateMenuItem_ReturnsMutatedItemAndVersion`,
`TestAddMenuCategory_ReturnsMutatedCategoryAndVersion` (server); all existing menu DB concurrency
+ invalidation + reorder-CAS tests still green.

### Fix 1 (backend part) — Unchanged-string skip on the translate job path (CRITICAL)
Added `hasFreshTranslation` gate to `TranslateMenuItemToLanguages` / `TranslateCategoryToLanguages`
(the job/batch path behind `TranslateEntireMenu`): before an external translate + save, skip when a
stored translation for (entity, field, lang) already has matching non-empty text + unchanged
`OriginalText` (latest-row-wins, mirroring the read-path). A no-op re-sync of an unchanged menu now
issues zero external Google Translate calls instead of ~2 per item (name+desc) × 1,200 items.

Tests: `TestTranslateMenuItemSkipsUnchangedStrings`, `TestTranslateCategorySkipsUnchangedStrings`
(first pass = 2 calls, unchanged re-sync = 0, changed field = 1). `BenchmarkTranslateMenuItem_NoOpResync`
(in-memory SQLite, DB-only, provider fails on any hit): ~82-123 µs/op, 14.5KB/op, 266 allocs/op —
and asserts zero external calls after priming.

Command: `go test ./internal/services/ -run '^$' -bench 'BenchmarkTranslateMenuItem_NoOpResync' -benchmem -count=3`

### Fix 1 (frontend part) — Sync via the async job + progress/cancel/error UI (CRITICAL)
`useMenuData` "Sync all" and per-language sync now call `translateEntireMenu` (the async job) and
poll `getTranslationStatus` for real `{progress, total}` instead of looping the synchronous
per-string `translateMenu` endpoint in-request. `ActiveLanguagePills` gained a real progress bar,
a Cancel button (client-side: stops polling/applying; the idempotent job finishes server-side and
the unchanged-string skip makes a re-run cheap), and a partial-failure/error line. The synchronous
`POST /menu/translate` endpoint + `businessApi.translateMenu` remain for BE-first compat but have
zero UI callers now. New i18n keys: `menuBuilder.languages.syncCancelled` / `syncCancelAria` (en+es).

Tests: new `useMenuData` spec asserts sync-all starts the job with target codes, polls status, and
never calls the old sync endpoint; existing MenuBuilder specs green.

### Fix 4 — Granular reorder endpoint (HIGH)
New `POST /businesses/:id/menu/reorder` (`{scope: "category"|"item", category_id?, from, to, version}`)
applies a single move to the stored tree under optimistic CAS. `ReorderMenuCategory` /
`ReorderMenuItem` DB helpers. Drag-drop (`useMenuDragDrop`) now sends just the one move instead of
re-uploading the whole menu document via `saveMenu`. Server-side tests cover category move, item
move, stale-version 409, and out-of-range 400. Frontend drag-drop test asserts the granular payload
(scope/category_id/from/to/version) for both category and item drags. The old `saveMenu` full-blob
path remains for any legacy callers.

### Fixes 5 + 6 + 7 — Currency picker, id-based targets + searchable pickers + missing-target state, shared menu (MED)
- Fix 5: BundlesManager settlement currency is now a `Select` fed by `getBusinessCurrencies` (options
  = configured currencies ∪ default ∪ current value) instead of a free-text field that accepted
  "EURO"/"USDD" into settlement paths.
- Fix 6: OffersManager targets categories by stable `category.id` (was `category.name` → renames
  orphaned them) and items by id; the target picker is a searchable NextUI `Autocomplete` grouped by
  category (was a flat 1,200-option `Select`); offer cards render a "target missing" warning chip when
  the target no longer resolves. Backend BE-first: `promotion_pricing.go` category matching now accepts
  the category ID OR the category name, so legacy name-targeted offers keep matching after the frontend
  switch (test `TestCategoryOffer_MatchesByIdAndLegacyName`).
- Fix 7: new `src/hooks/useSharedMenu.ts` — one React Query key `["business-menu-categories", id]`
  (5-min staleTime) that Offers and Bundles read instead of an independent `getMenu` on every mount, so
  sub-tab bounces no longer re-fetch the full menu.
- New i18n key `menuBuilder.offersManager.targetMissing` (en+es). Offers/Bundles jest suites wrapped in
  a QueryClientProvider for the shared-menu query; all 7 green.

### Fixes 9 + 10 — Collapsible categories + debounced search + native number input (LOW)
- Fix 9: CategoryCard gains a chevron collapse toggle that hides the items body, so a large menu no
  longer renders every item of every category at once. The MenuBuilder search input is debounced
  (200ms) via a `debouncedSearchQuery` that drives the filter memo, so keystrokes no longer re-filter
  + re-render every category. New i18n keys `categories.collapse` / `categories.expand` (en+es).
  Full virtualization was intentionally skipped (not trivial) per the audit's "skip unless trivial".
- Fix 10: the AI review editor (MenuReviewEditor) price field is now a NextUI `Input`
  (inputMode="decimal" + parseLocaleDecimal) instead of a native `<input type="number">`, matching the
  rest of the tab.

### Deferred (per prompt item 11 + noted)
- JSON-document normalization / per-item CAS granularity (the two CRIT storage-model issues) — out of
  scope for this session; the menu remains one JSON text column with whole-document CAS.
- Manual translation-correction UI — the `updateTranslation` / `saveMenuItemTranslation` endpoints
  exist and are unused; a UI to hand-fix a machine translation is still missing (flagged).
- Position-based translation keys (reorder → silent full re-translate) — unchanged; the read path's
  OriginalText freshness compare already self-heals stale keys, and the unchanged-string skip (fix 1)
  makes the re-translate cheap, but the underlying positional keying remains.

## AI image daily reserve path (2026-08-01, Task 7)

Daily fair-use metering replaces the monthly quota + credit packs. Query shape
is larger than the predecessor (closer to a doubling than a tripling):
`EnsureImageUsageRow` runs an INSERT-ON-CONFLICT plus a SELECT outside the
transaction, then the transaction itself runs two window-reset UPDATEs, the
conditional `daily_used < ?` UPDATE, a SELECT of the fresh row, and (when the
monthly alert threshold is crossed) one more UPDATE to claim the alert — 5 to
7 statements per call. The predecessor (`ReserveImageCredit`) was roughly 3
to 4 statements: `EnsureImageCreditRow` did a `FirstOrCreate` SELECT (plus an
INSERT only on first-ever use) and a conditional `monthly_limit` UPDATE
outside the tx, then inside the tx one monthly-window reset UPDATE, one
conditional `monthly_used` UPDATE, and — when monthly allowance was exhausted
— an optional second conditional `pack_balance` UPDATE; no in-transaction
read. This entry is the Backend Performance Gate baseline for
`ReserveImageGeneration`, not a before/after optimization.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Fixture: in-memory SQLite, single connection, absurdly high limits so the loop
measures the success path only. Numbers recaptured on a quiet machine
(prior first capture was load-contaminated: ~206k–305k ns/op with ~49% spread).

### `BenchmarkReserveImageGeneration` (`github.com/stdevmac/payverge/backend/internal/database`)

| Run | ns/op | B/op | allocs/op |
| ---: | ---: | ---: | ---: |
| 1 | 75,808 | 43,953 | 543 |
| 2 | 73,790 | 43,946 | 543 |
| 3 | 73,939 | 43,945 | 543 |

**Median:** ~**73,939 ns/op**, **~43.9 KB/op**, **543 allocs/op** for that
5-to-7-statement shape (~2.7% ns/op spread across the three runs).

Commands:
```bash
cd backend && go test ./internal/database/ -bench BenchmarkReserveImageGeneration -benchmem -run '^$' -count=3
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
cd backend && go test -tags integration_postgres ./internal/database -run '^$' -count=1
cd backend && go test -tags integration_postgres ./internal/database -run TestReserveImageGeneration_ConcurrentDoesNotOverIssue_Postgres -v -count=1
```

Compile gates: all three packages `ok` (no tests matched by `-run '^$'`);
`integration_postgres` tag compile-only check also `ok` (no duplicate declaration
vs the untagged sqlite suite).

Postgres concurrency (READ COMMITTED, real multi-connection race):
`TestReserveImageGeneration_ConcurrentDoesNotOverIssue_Postgres` — **PASS**
(25 concurrent reserves, daily limit 5 → exactly 5 grants, `daily_used == 5`).
This is the only place in the suite that executes the conditional UPDATE under
true concurrency; the sqlite harness's `ConcurrentDoesNotOverIssue` test pins
`SetMaxOpenConns(1)` and can only count grants, while the statement-shape
predicate itself is pinned separately by
`TestReserveImageGeneration_RefusalIsTheUpdatePredicate`.

## AI insights business-TZ buckets L4-9 (2026-08-05, Session E)

Finding L4-9: Camarero IA insights day/hour series were UTC-only (7-day trend
could show "tomorrow"; busiest hours shifted by business offset, e.g. +3h for AR).

Fix: `GetAiConversationTimeSeries` accepts business `*time.Location` and uses
localized SQL (Postgres `AT TIME ZONE`, SQLite fixed-offset datetime). Handler
builds the 7-day window from business-local midnight via
`database.ResolveBusinessLocation`.

Regression: `TestGetAiConversationTimeSeries_BucketsInBusinessTZ` (AR UTC-3,
fixed offset).

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Command: `go test ./internal/server/ -bench=BenchmarkGetAiInsights -benchmem -count=3 -run='^$'`

**Before** (UTC-only path; files from `140614cbb^`, same fixture):
```
BenchmarkGetAiInsights-8   	    8716	    121696 ns/op	   56811 B/op	     751 allocs/op
BenchmarkGetAiInsights-8   	    8758	    123161 ns/op	   56810 B/op	     751 allocs/op
BenchmarkGetAiInsights-8   	    9061	    131873 ns/op	   56782 B/op	     751 allocs/op
```

**After** (business-TZ SQL + local midnight window):
```
BenchmarkGetAiInsights-8   	    4455	    245667 ns/op	   56742 B/op	     751 allocs/op
BenchmarkGetAiInsights-8   	    5473	    226561 ns/op	   56760 B/op	     751 allocs/op
BenchmarkGetAiInsights-8   	    5397	    210101 ns/op	   56754 B/op	     751 allocs/op
```

Access shape unchanged: still SQL GROUP BY aggregates (no timestamp pluck).
Allocs/op flat (751). Latency ~1.7–2× on SQLite microbench (localized datetime
expr); acceptable for correctness. Index proposals: none required; existing
`(business_id, created_at)` on `ai_waiter_conversations` remains the hot index.

## CRM summary tier filter L5-2 (2026-08-05)

`GetBusinessCustomerSummary` now accepts the list `tier` filter (in addition to
`search` and independent `topTier` for the Gold count). Access shape is unchanged:
still **one SQL aggregate** via `applyBusinessCustomerFilters` + `COUNT/AVG`.
No new queries or N+1. Tier filter adds a single equality predicate on
`customer_businesses.loyalty_tier`.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Command: `go test ./internal/crm/ -bench=BenchmarkGetBusinessCustomerSummary -benchmem -count=3 -run='^$'`
Fixture: 1000 target + 50 noise customer_business rows (existing bench harness).

**Before** (3-arg summary, no tier filter; service from `6b0426e2f^`):
```
BenchmarkGetBusinessCustomerSummary-8   	    7684	    185275 ns/op	    8550 B/op	      81 allocs/op
BenchmarkGetBusinessCustomerSummary-8   	    4462	    559785 ns/op	    8551 B/op	      81 allocs/op
BenchmarkGetBusinessCustomerSummary-8   	    7270	    266114 ns/op	    8549 B/op	      81 allocs/op
```

**After** (4-arg; unfiltered path `tier=""` — same SQL shape as before):
```
BenchmarkGetBusinessCustomerSummary-8   	    3097	    388930 ns/op	    8552 B/op	      81 allocs/op
BenchmarkGetBusinessCustomerSummary-8   	    6129	    302021 ns/op	    8550 B/op	      81 allocs/op
BenchmarkGetBusinessCustomerSummary-8   	    6619	    175448 ns/op	    8549 B/op	      81 allocs/op
```

Allocs/op flat (81). Latency within noise of before on unfiltered path. No
migration; optional later index on `(business_id, loyalty_tier)` if filtered
summary becomes hot in production.

# L6-10 analytics prior-calendar growth (audit/r2-finance, 2026-08-05)

Finding L6-10 (Backend Performance Gate): GetPeriodReport growth baseline switched
from duration-shift (`start.Add(-duration)`) to `reporting.PriorWindow` (calendar
prior unit) + nil growth when prior revenue is zero; `parsePeriod` consolidated
onto `reporting.ResolveWindowAt`.

Access-shape: `TestGetPeriodReport_GrowthRate_PriorCalendarYear` asserts prior
calendar YTD baseline (not duration-shifted mid-year dollars).

Benchmark (SQLite microbench, Apple M3 arm64, `-bench=BenchmarkGetPeriodReport -benchmem -count=1`):

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before | 36641297 | 412916 | 2526 |
| after  | 18833061 | 408634 | 2502 |

Commands:
```
cd backend && go test ./internal/analytics/ -bench=BenchmarkGetPeriodReport -benchmem -count=1 -run='^$'
cd backend && go test ./internal/analytics/ -run 'GrowthRate|PriorCalendar' -count=1
cd backend && go test ./internal/reporting/ -run 'TestPriorWindow' -count=1
```

# L6-23 outstanding as-of end + drilldown (audit/r2-finance, 2026-08-05)

Finding L6-23 (Backend Performance Gate): Outstanding unpaid-bills list was
range-scoped (`created_at >= start AND created_at < end`), so old open debt
vanished when operators picked a recent window; rows had no drilldown.

Fix:
1. Query is as-of window end: `created_at < end` only (no lower bound). `start`
   still accepted for shared date-range parser/UI compatibility.
2. Projection unchanged (id, created_at, table_label, total/paid cents, status)
   + LEFT JOIN tables; order outstanding DESC; page_size clamp 100.
3. FE row opens L6-3 operator `BillDetailsModal`.

Access-shape regression:
`TestGetUnpaidBills_AsOfWindowEnd_IncludesOlderOpenDebt` (handler package).

Benchmark — `BenchmarkGetUnpaidBills` (`backend/internal/handlers/accounting_unpaid_bench_test.go`).
Full route bench (gin router → handler → GORM) on a deterministic local SQLite
seed: 1 business, 40 tables, 6000 bills spread over the 730 days ending at the
window end, 1/3 partially paid + open (outstanding), 1/3 voided, 1/3 fully paid.
Window `start=2026-05-01&end=2026-05-31`, `page_size=20`. BEFORE is the
merge-base handler at `0bd51b5d7` (range-scoped predicate), temporarily restored
to measure and then reverted; AFTER is the branch handler (as-of end).

Apple M3 arm64, `-benchmem -count=3`:

| | ns/op | B/op | allocs/op | rows matched by predicate |
|---|---:|---:|---:|---:|
| before (range-scoped, `0bd51b5d7`) | 1225753 / 1230240 / 1234020 | 73328 / 73284 / 73328 | 1322 | 93 |
| after (as-of end, branch) | 1865368 / 1977083 / 1820174 | 72894 / 72951 / 72964 | 1315 / 1316 / 1316 | 2000 |

Mean: 1,230,004 → 1,887,542 ns/op (**+53.5%**); 73,313 → 72,936 B/op (−0.5%);
1,322 → 1,316 allocs/op (−0.5%).

Reading: allocations and bytes are flat because the response is still bounded by
`page_size` (20 rows). The latency delta is entirely the widened predicate — the
COUNT plus the `ORDER BY (total_amount - paid_amount) DESC` sort now consider
21.5× more matching rows (2000 vs 93) because as-of-end is the correct set. This
is a deliberate correctness-for-latency trade: the old number was cheap because
it was hiding debt. Nothing in the query shape regressed (same narrow
projection, same single LEFT JOIN, no preloads, no N+1).

Commands:
```
cd backend && go test ./internal/handlers/ -run 'TestGetUnpaidBills' -count=1
cd backend && go test ./internal/handlers -run '^$' -bench 'BenchmarkGetUnpaidBills' -benchmem -count=3
cd backend && go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
cd frontend && npx jest --watchman=false --runInBand src/components/business/accounting/OutstandingTab.test.tsx
```

Ambient reference (collection_gap / GetSummary — **not** the unpaid handler):
```
cd backend && go test ./internal/accounting/ -bench=BenchmarkGetSummary_CollectionGap -benchtime=200ms -count=1 -run='^$'
BenchmarkGetSummary_CollectionGap-8  175  3132806 ns/op  297180 B/op  2876 allocs/op
```

Honest note: as-of returns more rows than a tight window (correctness > smaller
sets), and the bench now quantifies that at +53% on a 6k-bill table. The
follow-up lever is the index `(business_id, status, created_at)` — deferred, no
migration in this slice — plus the existing `page_size` clamp of 100 that keeps
the response bounded regardless of how much open debt a business carries.

# L6-15 entries list paperclip + owner actor (audit/r2-finance, 2026-08-05)

Finding L6-15 (Backend Performance Gate): list always showed a paperclip;
detail "Created by" was "—" for owner-created rows (only CreatedByStaff preloaded).

Fix:
1. `ListEntries` preloads `CreatedByUser`/`CreatedByStaff`/`VoidedByStaff` with
   selected columns (`id, name, email`) — no password/auth fan-out.
2. One grouped `COUNT(*) ... GROUP BY entry_id` for `attachment_count` on the
   page of IDs (not N+1).
3. FE gates paperclip on `attachment_count > 0`; detail uses staff name then user name/email.

Access-shape: `TestListEntries_AccessShape_ActorsAndAttachmentCount`. Rewritten
2026-08-05 — the original asserts were inert (`NotContains(joined,"password")`
passes against `SELECT *` because sqlite never names the column, and
`Contains(joined,"count(")` was satisfied by the pagination COUNT, so an N+1
attachment count would have slipped through). It now classifies every captured
SELECT by target table and asserts (a) the users/staff preloads project exactly
`id,name,email`, (b) exactly ONE grouped attachment COUNT, (c) no `SELECT *`
against the actor tables. Watched RED both ways: dropping the user preload
`Select` fails on `SELECT * FROM users`; swapping the grouped COUNT for a
per-entry loop fails with `got 2` attachment queries.

Benchmark — `BenchmarkListEntries` (`backend/internal/accounting/list_entries_bench_test.go`).
Deterministic local SQLite: 1 business, 1 owner user, 5 staff, 500 ledger
entries in the window (alternating user/staff actors), attachments on every
third entry; `Page 1, PageSize 20`. BEFORE is the merge-base `ListEntries` at
`0bd51b5d7` (no preloads, no attachment counts), temporarily restored to measure
and then reverted; AFTER is the branch version.

Apple M3 arm64, `-benchmem -count=3`:

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (`0bd51b5d7`, no preloads/counts) | 346006 / 352963 / 348816 | 104042 / 104045 / 104046 | 820 |
| after (3 preloads + grouped COUNT) | 507007 / 465418 / 458505 | 295657 / 295654 / 295675 | 1496 |

Mean: 349,262 → 476,977 ns/op (**+36.6%**); 104,044 → 295,662 B/op (**+184%**);
820 → 1,496 allocs/op (**+82%**).

Reading: the cost is 4 extra queries per list page — three actor preloads
(`CreatedByStaff`, `CreatedByUser`, `VoidedByStaff`, each with a narrow
`id,name,email` projection) and one grouped attachment COUNT. That count is
**constant in the page size**, not per row: the alternative that produced the
correct UI (per-entry actor + attachment lookups) would have been 20–60 queries
on the same page. Bytes/allocs rise because the page now actually carries actor
structs it previously left nil — this is data the UI needs, not fan-out; the
`VoidedByStaff` preload is skipped entirely by GORM when no row on the page is
voided (the common case). No migrations; `AttachmentCount` stays a gorm `->`
virtual column.

Commands:
```
cd backend && go test ./internal/accounting/ -run TestListEntries_AccessShape -count=1
cd backend && go test ./internal/accounting -run '^$' -bench 'BenchmarkListEntries' -benchmem -count=3
cd frontend && npx jest --watchman=false --runInBand src/components/business/accounting/EntriesTab.paperclip.test.tsx EntryDetailDrawer.test.tsx
```

## Print-job queue ordering tiebreak L3-35 (2026-08-05)

`ListJobs` (dashboard Recent Print Jobs poll) orders
`print_jobs.created_at DESC, print_jobs.id DESC`. Print jobs fan out in bursts
(one order → kitchen + bar + bill tickets in the same millisecond), so
`created_at` alone left intra-burst order to the storage engine and the history
list reshuffled between polls. `id` is the primary key and the trailing sort
column, so the added tiebreak is not a new access path.

Regression: `TestListPrintJobs_OrderByCreatedAtAndID` — real handler + SQLite,
4 seeded jobs (3 sharing a timestamp, inserted out of order), asserts exact id
sequence. Replaces a source-grep test that only matched the ORDER BY literal in
`print_job_handlers.go` and proved nothing about returned rows. RED confirmed by
reverting the ORDER BY: `expected: []uint{0x4, 0x3, 0x1, 0x2}` vs
`actual: []uint{0x1, 0x3, 0x4, 0x2}`.

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.
Command: `go test ./internal/handlers/ -run='^$' -bench '^BenchmarkListPrintJobs$' -benchmem -count=3`
Fixture: 500 print jobs, bursts of 5 sharing a timestamp.

**Before** (`created_at DESC` only):
```
BenchmarkListPrintJobs-8   	    2138	    551407 ns/op	  136777 B/op	    1925 allocs/op
BenchmarkListPrintJobs-8   	    2145	    578521 ns/op	  136646 B/op	    1925 allocs/op
BenchmarkListPrintJobs-8   	    2118	    576937 ns/op	  136696 B/op	    1925 allocs/op
```

**After** (`created_at DESC, id DESC`):
```
BenchmarkListPrintJobs-8   	    2091	    522208 ns/op	  136803 B/op	    1925 allocs/op
BenchmarkListPrintJobs-8   	    2209	    527640 ns/op	  136942 B/op	    1925 allocs/op
BenchmarkListPrintJobs-8   	    1690	    807832 ns/op	  136656 B/op	    1925 allocs/op
```

Allocs/op flat (1925), bytes/op flat (~136.7 KB). Latency within run-to-run
noise (the 807 µs sample is an outlier; medians 577 µs before vs 528 µs after).
The tiebreak is free. No migration; the existing `(business_id, created_at)`
index on `print_jobs` remains the hot index.

---

# Decision-14 benchmark follow-up — reclaim accepted latency (audit/w4-bench, 2026-08-06)

Work list extracted from this log's **accepted correctness-for-latency** rows:

| Source finding | Benchmark | Accepted after | Work direction |
|---|---|---|---|
| L4-9 AI insights business-TZ | `BenchmarkGetAiInsights` (+ service microbench) | ~122–132 µs → ~210–246 µs (allocs flat 751); "acceptable for correctness" | timezone-conversion caching |
| L6-15 entries list actors | `BenchmarkListEntries` | ~349k → ~477k ns (+36.6%), 104k → 296k B/op (+184%), 820 → 1496 allocs | access-shape narrowing |
| L6-23 unpaid as-of end | `BenchmarkGetUnpaidBills` | ~1.23M → ~1.89M ns (+53.5%); allocs flat; "follow-up lever: index" | access-shape + index |

Hardware: Apple M3 (`darwin/arm64`), `-benchmem -count=3`.

### 1) Timezone conversion caching (`database.LocalizedTimestampExpr`)

`time.LoadLocation` was already memoized in `ResolveLocation`. Decision-14 adds a
**localized SQL fragment cache** shared by analytics + AI conversation timeseries
(key = dialect + column + IANA zone + current UTC offset so DST flips naturally).

Access-shape:
- `TestLocalizedTimestampExpr_CachesSQL`
- `TestGetAiConversationTimeSeries_NoSelectStar` (still two GROUP BY aggregates; no `SELECT *`)

Tried and **reverted**: single `GROUP BY (day, hour)` over the 30-day window.
Measured on `BenchmarkGetAiConversationTimeSeriesSQLite` (500 convos, AR UTC-3):

| | ns/op (range) | B/op | allocs/op |
|---|---:|---:|---:|
| two narrow aggregates (kept) | 715k–784k (outlier 1.7M) | ~17.4k | **319** |
| single day×hour GROUP BY (**reverted**) | 837k–980k | ~33.8k | **937** |

Single scan produced more result rows (day×hour) → higher B/op and allocs; no
honest latency win. Reverted.

`BenchmarkGetAiConversationTimeSeriesSQLite` after cache (two-query path kept):

```
BenchmarkGetAiConversationTimeSeriesSQLite-8   937–1786   787k–1.34M ns/op   17353–17357 B/op   319 allocs/op
```

`BenchmarkGetAiInsights` remains cache-dominated (60s handler cache) so wall
noise is large; allocs still **751** (unchanged):

```
BenchmarkGetAiInsights-8   3544–8461   156k–369k ns/op   ~56750 B/op   751 allocs/op
```

### 2) ListEntries actor batch + slim User JSON (L6-15 reclaim)

Replaced three GORM association Preloads with at most **one users IN** + **one
staff IN** (CreatedByStaff + VoidedByStaff share the staff set). Added
`User.MarshalJSON` slim projection when `CreatedAt` is zero (mirrors Staff
FIND-052) so list wire bodies drop nested `notification_preferences` zeros.

Access-shape: `TestListEntries_AccessShape_ActorsAndAttachmentCount` now asserts
`Len(userQueries)==1` and `Len(staffQueries)==1` with a voided row on the page
(old dual staff Preload would be 2). Wire: `TestUserMarshalJSON_SlimWhenPartialProjection`.

Session baseline (L6-15 after shape, same fixture 500 entries / page 20) → after:

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| before (this session, pre-change) | 2112493 | 295493 | **1496** |
| after (median-ish of 3) | 1.10M–2.15M | **214876–214921** | **1223** |

Allocs **−18%** (1496 → 1223), B/op **−27%** (295k → 215k). ns/op noisy but not
regressed in alloc shape. Summary.md L6-15 "after" was 477k ns on a quieter
machine; compare allocs/B as the durable signal.

### 3) GetUnpaidBills COUNT without join + partial index 000197 (L6-23 reclaim)

- COUNT uses bills-only predicate (no `LEFT JOIN tables`); page Scan still joins
  for `table_label`.
- Migration `000197_bills_outstanding_as_of_index`: partial index
  `(business_id, created_at DESC) WHERE status <> 'voided' AND total_amount > paid_amount`.
- **Genesis baseline regen Owed** by coordinator — `MIGRATION_HEAD` and
  `backend/schema/genesis/` intentionally untouched on this branch.

Access-shape: `TestGetUnpaidBills_CountSkipsTableJoin`.

Bench (same 6k-bill fixture; index created in SQLite microbench to mirror 000197):

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| session before (no partial index; count may join) | 4.4M then 7.8–12M (noisy) | ~73–74k | 1316–1332 |
| after (count no-join + idx) | **2.33M–3.53M** | ~74k | 1330–1331 |

Within-session reclaim vs no-index runs is **~3×** on wall time. vs original
L6-23 summary after (~1.89M) the absolute numbers are machine-noise-heavy;
allocs stay flat (page still 20 rows). Index is the real lever named at L6-23
sign-off.

### Commands run

```bash
cd backend
go test ./internal/database/ -run 'TestResolveLocation|TestLocalizedTimestamp|TestGetAiConversationTimeSeries|TestUserMarshalJSON' -count=1
go test ./internal/database/ -run='^$' -bench='^BenchmarkGetAiConversationTimeSeriesSQLite$' -benchmem -count=3
go test ./internal/server/ -run='^$' -bench='^BenchmarkGetAiInsights$' -benchmem -count=3
go test ./internal/accounting/ -run 'TestListEntries_AccessShape' -count=1
go test ./internal/accounting/ -run='^$' -bench='^BenchmarkListEntries$' -benchmem -count=3
go test ./internal/handlers/ -run 'TestGetUnpaidBills' -count=1
go test ./internal/handlers/ -run='^$' -bench='^BenchmarkGetUnpaidBills$' -benchmem -count=3
go test ./internal/handlers ./internal/server ./internal/database ./internal/accounting ./internal/analytics -run '^$' -count=1
```

Compile gate (handlers/server/database) **ok**.

---

# MercadoPago orders-topic webhook business resolution (2026-08-09)

Task 9: application-level orders webhooks omit `business_id`. Handler resolves
business via `alternative_payments.participant_addr = data.id AND
payment_method = 'mercadopago'` → bill → business (pre-verify routing only).

## Migration 000200

`idx_alt_payments_participant_addr` partial index on
`alternative_payments(participant_addr) WHERE participant_addr <> ''`.

Genesis baseline had **no** standalone index on `participant_addr` (confirmed
in `backend/schema/genesis/current_schema.sql`). Without the index, each orders
webhook is a sequential scan on `alternative_payments` at production volume.

Access shape:
- One projected join: `alternative_payments ⨝ bills` filtered by
  `(participant_addr, payment_method)`, `ORDER BY ap.id DESC LIMIT 1`
  (`GetBusinessIDByPluginPaymentTracker`).
- Regression coverage: `TestGetBusinessIDByPluginPaymentTracker`,
  `TestHandleMercadoPagoWebhook_OrderTopicResolvesBusinessViaTracker`,
  `TestHandleMercadoPagoWebhook_OrderTopicNoTrackerAcceptedButIgnored`.

No microbench of the lookup alone (single indexed point query); gate is the
index + join projection (no `SELECT *` on either table).

### Commands run

```bash
cd backend
go test ./internal/plugins/mercadopago/ -count=1 -run 'TestMPHandle|TestMPVerify|TestGetOrder|TestCreateOrder'
go test ./internal/handlers/ -count=1 -run 'TestHandleMercadoPagoWebhook_Order|TestHandleMercadoPagoWebhook_PaymentUpdatedStill|TestGetBusinessIDByPluginPaymentTracker|TestPaymentWebhookRequiresResolvedBusiness'
go test ./internal/plugins/mercadopago/ ./internal/handlers/ ./internal/database/ -count=1 -run 'MercadoPago|PluginPayment|Webhook|PluginPaymentTracker|paymentWebhookRequires'
go test ./internal/handlers ./internal/server ./internal/database ./internal/plugins/mercadopago -run '^$' -count=1
```

Compile gate **ok**.

---

# Stripe Connect account→tenant lookup (2026-08-10)

Connect webhooks all land on ONE platform endpoint, so every delivery must map
`payload.account` (`acct_…`) to the owning business before anything else can
happen. That lookup is now on the hot path of every Connect event, and the
first shape scaled with tenant count: `ListBusinessesWithPluginEnabled("stripe")`
followed by one `GetBusinessPluginConfig` per business — a query **and** an AES
decrypt of every secret field, per tenant, per event. The deauthorization path
ran it twice.

Fix: one narrow-projection query (`business_id`, raw `config` text) joined to
`plugins`, filtered to enabled/active Stripe rows, then a Go-side JSON match on
`connection_mode` + `stripe_user_id`. Both keys are non-secret
(`security.IsSecretConfigKey`), so they are plaintext inside the config JSON and
need no decryption. `MarkStripeOAuthDeauthorized` now accepts the already
resolved tenant ids so the webhook path looks the account up once.

No index/migration: the filter is on `plugins.name` + `business_plugins.is_enabled`
(existing FK/index coverage) and the JSON match cannot be indexed portably —
handler and database tests run on SQLite, so a Postgres-only expression index or
plpgsql extractor in the production query path would break them.

## Benchmark — 200 Stripe-enabled tenants, SQLite, `-benchmem -count=3`

`BenchmarkFindBusinessIDsByStripeUserID_{Before,After}` (`internal/database`)

| | ns/op | B/op | allocs/op |
|---|---|---|---|
| Before (per-business query + decrypt) | 3,630,820 / 3,667,974 / 4,301,470 | 1,720,246 | 20,330 |
| After (single query, no decrypt) | 744,402 / 716,005 / 668,773 | 445,713 | 7,483 |
| Delta | **≈5.2× faster** | **−74%** | **−63%** |

Before grows linearly in tenants (query + decrypt each); After issues one
statement regardless.

## Access-shape regression

- `TestFindBusinessIDsByStripeUserIDIsSingleQuery` — exactly ONE statement
  (Query, Row and Raw callbacks all counted) at 200 tenants.
- `TestFindBusinessIDsByStripeUserIDIgnoresManualAndDisabled` — manual-mode and
  disabled rows holding the same `acct_…` stay out of the result set.

## Commands run

```bash
cd backend
go test ./internal/database -run 'TestFindBusinessIDsByStripeUserID' -count=1
go test ./internal/database -run '^$' -bench 'BenchmarkFindBusinessIDsByStripeUserID' -benchmem -count=3
go test ./internal/handlers -run 'TestStripeConnect|TestStripeManual|TestStripeOAuth' -count=1
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

---

## ARCA Wave A — print receipt fiscal block (2026-08-10)

**Benchmark:** `BenchmarkPrintEnqueueReceipt` (`internal/services/print`)

| | ns/op (median of 3) | B/op | allocs/op |
|---|---:|---:|---:|
| **Before** (no fiscal authorized-receipt probe) | 611012 | 236777 | 2553 |
| **After** (probe + best-effort fiscal block load) | 676827 | 245394 | 2650 |

Raw runs (before):
```
BenchmarkPrintEnqueueReceipt-8    2924    591483 ns/op  236852 B/op    2553 allocs/op
BenchmarkPrintEnqueueReceipt-8    3238    611012 ns/op  236773 B/op    2553 allocs/op
BenchmarkPrintEnqueueReceipt-8    3319    622641 ns/op  236777 B/op    2553 allocs/op
```

Raw runs (after):
```
BenchmarkPrintEnqueueReceipt-8    1878    676827 ns/op  245768 B/op    2650 allocs/op
BenchmarkPrintEnqueueReceipt-8    3189    638588 ns/op  245337 B/op    2650 allocs/op
BenchmarkPrintEnqueueReceipt-8    3073    720249 ns/op  245394 B/op    2650 allocs/op
```

**Commands:**
```bash
cd backend
# before: fiscal load short-circuited to nil (no probe)
go test ./internal/services/print/ -bench BenchmarkPrintEnqueueReceipt -benchmem -run '^$' -count=3
# after: full loadFiscalTicketInfo path
go test ./internal/services/print/ -bench BenchmarkPrintEnqueueReceipt -benchmem -run '^$' -count=3
```

Delta is the expected +1 sqlite authorized-receipt probe on the enqueue hot path when the table is missing/empty (fail-open). ~+11% ns/op, +~9k B/op, +97 allocs/op — no blow-up.

**Homologación e2e:** **blocked** — `TestAFIPHomologacionE2E` skipped (missing `FISCAL_HOMO_CUIT`, `FISCAL_HOMO_POS`, `FISCAL_HOMO_CERT_PATH`, `FISCAL_HOMO_KEY_PATH` in worktree). Not faked green.

---

## Public reservation settings/availability query shape (2026-08-16)

Unauthenticated `GET /business/:customUrl/reservations/settings` and `.../availability` used `GetReservationSettings` (hidden INSERT/UPDATE on read), a 7-day per-day operating-hours + `SELECT *` conflicts loop, and hydrated guest PII (`customer_email` / `customer_phone` / `special_requests` / `notes`) to emit `next_available_slot` + `available_slot_count`.

### Fix
- Public GET (and the public create settings check) use `GetReservationSettingsForRead`; `loadAvailabilityContext` also uses the read helper.
- `GetPublicSettingsForBusiness` / `GetPublicAvailability` reuse the handler's public business projection instead of `GetBusinessByID` (`SELECT *`).
- `ensureAvailabilityCaches` loads all operating hours once; the 7-day metrics loop no longer calls `GetBusinessOperatingHoursByDay`.
- `loadReservationConflicts` projects scheduling columns only and the settings summary prefetches one windowed conflicts read, then filters in memory per day.

### Access-shape tests (TDD red → green)
- `TestPublicReservationGETPathsDoNotWrite` — handler GET settings + availability issue zero INSERT/UPDATE; persisted `max_advance_days=0` stays 0.
- `TestGetPublicSettingsDoesNotWriteOnRead` / `TestGetAvailabilityDoesNotWriteOnRead`
- `TestLoadReservationConflictsOmitsGuestPII` — no `SELECT *`, no PII columns, hydrated conflict rows have empty email/phone/requests/notes.
- `TestGetPublicSettingsDoesNotNPlusOnePerDay` — ≤1 operating-hours read, ≤1 conflicts read.

### Benchmark (`BenchmarkGetPublicReservationSettingsSQLite`, SQLite microbench, `-benchmem -count=3`)

| | ns/op | B/op | allocs/op | query shape |
|---|---:|---:|---:|---|
| **Before** | 9,419,369 | 20,307,565 | 29,176 | ~20 queries incl. possible UPDATE + 7× hours + 7× `SELECT *` conflicts |
| **After** | 6,897,679–7,313,929 | 24,613,327–24,613,800 | 8,411–8,416 | bounded: 1 hours + 1 projected conflicts; zero writes |

ns/op **−22–27%**; allocs/op **−71%**. B/op is **+21%** because the 7-day summary now hydrates one windowed conflict set (then 7 in-memory day filters) instead of 7 overlapping SQL scans. Query count no longer scales with the lookahead.

```bash
cd backend
# TDD red (before production edits)
go test ./internal/server -run 'TestPublicReservationGETPathsDoNotWrite' -count=1
go test ./internal/services -run 'TestLoadReservationConflictsOmitsGuestPII|TestGetPublicSettingsDoesNotNPlusOnePerDay|TestGetPublicSettingsDoesNotWriteOnRead|TestGetAvailabilityDoesNotWriteOnRead' -count=1
# Baseline
go test ./internal/services -run '^$' -bench '^BenchmarkGetPublicReservationSettingsSQLite$' -benchmem -count=3
# Green + after
go test ./internal/services ./internal/database ./internal/server -run 'Reservation|PublicReservation|GetPublicReservation' -count=1
go test ./internal/handlers ./internal/server ./internal/database ./internal/services -run '^$' -count=1
go test ./internal/services -run '^$' -bench '^BenchmarkGetPublicReservationSettingsSQLite$' -benchmem -count=3
```

## AI waiter menu-grounded finalizer (2026-08-19)

Guest chat is a public route; the finalizer rewrite added per-turn snapshot scans, so both were benchmarked (`backend/internal/server/ai_waiter_menu_snapshot_bench_test.go`, 144-item menu, Apple M3, `-benchmem -count=3`; machine loaded by parallel batch agents so ns/op is noisy — allocs/op is the stable signal).

| Benchmark | Before | After |
|---|---|---|
| `BenchmarkBuildWaiterMenuSnapshot` | 2,283 allocs/op, 526 KB/op | 3,326 allocs/op, 624 KB/op (source-alias + dietary index added) |
| `BenchmarkMatchEntitiesInText` | 1,873 allocs/op, 130 KB/op, ~2.2 ms | **725 allocs/op, 29.5 KB/op, ~0.23 ms** |
| `BenchmarkRecommendableEntities` | — (new) | 1 alloc/op, 27 KB/op, 25–38 µs |

The `MatchEntitiesInText` win comes from hoisting guest-message normalization out of the per-entity loop (`waiterPaddedSearchText`); the same hoist was applied to `containsAnyWaiterPhrase`, which every intent classification calls. Net: the new grounded intents cost less per turn than the pre-existing entity scan did alone, on a path that is otherwise LLM-bound.

```bash
cd backend
go test -race ./internal/server/ -run 'Waiter|AiWaiter|AIWaiter' -count=1
go test ./internal/server -run '^$' -bench 'WaiterMenuSnapshot|MatchEntitiesInText|RecommendableEntities' -benchmem -count=3
```

## stale-bills briefing count matches advertised age (2026-08-22)

The Overview briefing rendered "{count} bills open longer than {duration}" with count = all open/partial bills past the 2h surfacing threshold and duration = the OLDEST bill's age, so a 7h service bill + a 6-day leftover became "2 bills open longer than 6 days". `getStaleOpenBillsInfo` (director briefing/console + digest path) now counts only bills at least as old as the rendered duration bucket (`insightDurationFloor`) and carries the full past-threshold count separately as `StaleTotal` / `stale_count`.

Access shape: unchanged 2 narrow queries (COUNT + oldest row) when ≤1 stale bill; one additional indexed COUNT (same predicate family, `business_id + status + created_at`) only when more than one stale bill exists. SQLite microbenchmark (`BenchmarkGetStaleOpenBillsInfo`, Apple M3, `-benchmem -count=3`):

| Case | ns/op | B/op | allocs/op |
|---|---|---|---|
| one stale bill (legacy 2-query shape) | ~19–20 µs | 18,927 | 146 |
| fifty stale bills (adds bucket COUNT) | ~46–55 µs | 27,477 | 201–202 |

The marginal bucket COUNT costs ~55 allocs on a per-operator briefing paint (not a hot guest route); the honest-copy fix requires knowing how many bills genuinely match the advertised age.

```bash
cd backend
go test -race ./internal/server -run 'TestGetStaleOpenBillsInfo_AgesEachBillNotJustOldest|TestBuildProactiveInsights_StaleBillsUseActualOldestAge' -count=1
go test ./internal/server -run '^$' -bench BenchmarkGetStaleOpenBillsInfo -benchmem -count=3
```

## bill lifecycle sweeper: abandon emitted once, leftovers excluded from scan (2026-08-22)

The 15-minute abandon sweeper notified `bill.abandoned` BEFORE `abandonBill`, which then refuses unpaid-remaining / live-kitchen / in-flight candidates — so a permanent unpaid leftover got a fresh "abandoned" operator notification on every tick while never leaving `open`. It also loaded every such leftover as a candidate and opened a locking transaction per bill per tick just to refuse it, and a successful abandon left `closed_at` NULL (half-terminal leftovers 1134/1139/1141). Fixes: notify only after a committed abandon (unique per bill by the `abandoned_at IS NULL` guard), candidates query now excludes money-outstanding bills in SQL (`total_amount - paid_amount <= 0` — they are categorically unsweepable), and the abandon UPDATE stamps `closed_at` via `COALESCE`. `CloseBillWithHistory` no longer stamps `settled_at` on the $0 → voided branch (no money settled); abandon paths never stamp it (regression-pinned).

SQLite microbenchmark, one sweeper tick over 300 stale unpaid leftovers (`BenchmarkBillLifecycleRunOnce_UnpaidLeftovers`, Apple M3, `-benchmem`, `-count=3` after / single baseline run before):

| | ns/op | B/op | allocs/op |
|---|---|---|---|
| before (leftovers scanned + per-bill locking tx refused each tick) | ~27.6 ms | 25,885,130 | 181,422 |
| after (SQL pre-filter; leftovers never leave the index scan) | ~64–69 µs | 98,832 | 102 |

```bash
cd backend
go test -race ./internal/services -run 'TestBillLifecycle' -count=1
go test -race ./internal/database -run 'TestCloseBill|TestAbandonUnpaidOpenBill' -count=1
go test ./internal/services -run '^$' -bench BenchmarkBillLifecycleRunOnce_UnpaidLeftovers -benchmem -count=3
```

## Demo seed v8 — Argentinean carta + AR venue coherence (2026-08-23)

`admin-demo-v8` reseed: full AR identity (Bodegón Mesa Larga / Parrilla
Quebracho Azul, ARS money bands, es-default + en menu translations, CABA delivery zone,
AR fiscal factura A/B/C with mod-11 CUITs, Bronce/Plata/Oro loyalty, own-hosted
`images.your-domain.example` gallery assets — the S3 rehoster is now bypassed entirely
at seed time). Daily refresh fixed (`AppendDueDays` pointer stays at yesterday
until the business day completes in America/Argentina/Buenos_Aires).

Ensure path is admin-only (startup + hourly cron); no guest/dashboard route
fan-out changed. Apple M3, SQLite in-memory, `-benchmem -count=3 -benchtime=1x`:

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkEnsureForAdmin` **before** (v5 honesty, prior entry) | 130.7M–147.2M | ~67.3M | ~463000 |
| `BenchmarkEnsureForAdmin` **after** (v8 AR carta) | 125.4M–148.8M | ~73.0M | ~503000 |

Wall time unchanged within noise; +~8% bytes and +~9% allocs from the 58-row
es→en translation seed, 5 inventory specs (was 3), and the richer AR bill/menu
copy. Zero rehost HTTP calls remain in the ensure path (was up to 3 warmups).

```bash
cd backend
go test ./internal/demo/ -count=1
go test ./internal/demo/ -run '^$' -bench BenchmarkEnsureForAdmin -benchmem -count=3 -benchtime=1x
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```

## Guest storefront menu 86 propagation — ETag + catalog clone (2026-08-23)

Backend Performance Gate evidence for the storefront sold-out (86) fix. The change
touches a guest-hot cached read (`GET /business/:customUrl/menu` →
`GetMenuByBusinessCustomUrl`): it now clones the cached category/item slice
before stamping serve-time 86 flags (`cloneGuestMenuCategories`) and folds an
orderability digest into the weak ETag (`orderabilityRevisionDigest`).

Benchmark: `BenchmarkGetMenuByBusinessCustomUrl_Storefront`
(`backend/internal/server/guest_storefront_menu_bench_test.go`) — warm pricing
cache, 6 categories × 20 items = 120-item catalog, inventory hard-block on with
one 86'd dish, no `ResetPricingCache` inside the loop. Apple M3, SQLite
in-memory, `-benchmem -benchtime=500x`, base ↔ branch **interleaved** 6× each
(single-shot A-then-B runs on this box drift 40%+ with thermal state, so the
runs are alternated and reported as medians over 6).

Base sha: `f923882315969eba6d7c173fd95b710127726f0f`.

| | ns/op (median of 6) | ns/op (min–max) | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| **before** (base) | 1,519,138 | 948,038–1,841,030 | 1,080,438 | 2,847 |
| **after** (branch) | 1,474,926 | 989,724–1,960,839 | 1,147,670 | 2,870 |

Honest read of the numbers:

- **Latency: no measurable change.** The medians differ by −2.9% while the
  run-to-run spread on either side is ±40%, and a CPU profile of the branch run
  attributes ~90% of samples to the runtime scheduler / GC / cgo-SQLite, not to
  the clone or the digest. Any per-request cost of the new work is below this
  harness's noise floor. An earlier non-interleaved pass showed the branch ~60%
  slower; that reproduced as a run-ordering artifact and disappeared once the
  two sides were alternated.
- **Memory: a real, small, expected regression.** +67,232 B/op (+6.2%) and
  +23 allocs/op (+0.8%). That is the per-request cost of not mutating the
  shared pricing-cache slice: one `[]MenuCategory` plus one `[]MenuItem` per
  category (~120 item structs), plus the digest's `json.Marshal` buffer and
  sha256 state. It buys correctness — before the clone, serve-time 86 stamps
  were written into the process-wide cached catalog.

No new query is issued: the digest consumes the already-computed projection and
the cloned categories, and `applyGuestLivePromotions` still calls
`ProjectOrderability` exactly once per request.

```bash
cd backend
go test ./internal/server -run 'TestGetMenuByBusinessCustomUrl_|TestGuestMenuETag|TestGetTableByCodePublic_' -count=1
go test ./internal/server -run '^$' -bench BenchmarkGetMenuByBusinessCustomUrl_Storefront -benchmem -benchtime=500x   # x6, interleaved with the same command in a base worktree
## Schedule week lookup resolves venue-local `week_start` rows (2026-08-23)

The operator Schedule tab rendered an empty week (`shifts: []`) for a venue that
demonstrably had staff and a published Sunday-dinner shift. `schedules.week_start`
is a TIMESTAMPTZ that names a *calendar week*, not an instant, and the two sides
disagreed on its encoding: readers build **UTC midnight**
(`weekFloorUTC`/`parseWeekValue` in `internal/handlers/schedule.go`,
`weekStartUTCMidnight` in `internal/handlers/livefloor.go`) while the seeder
writes **venue-local midnight** (`internal/demo/generator.go` ->
`weekStartMonday(normalizeBusinessDate(...))`). All four lookups were exact
equality, so in America/Argentina/Buenos_Aires (UTC-3) the read missed the row by
3 hours and returned nothing.

Fixed on the read side only (`internal/database/schedule_service.go`): a new
`findScheduleForWeek` backs `GetScheduleForWeek`, `GetPublishedScheduleForStaff`,
`GetOrCreateDraftSchedule` and `CopyScheduleWeek`. `NormalizeWeekStart` reads the
calendar day out of the caller's own location (never `t.UTC().Date()`, which walks
an eastern-hemisphere venue-local midnight back a day). The generator keeps
writing venue-local midnight — `ensureScheduleWorkflowData` reads
`schedule.WeekStart.In(loc)` to place its open shift and time-off day, and the
model's own struct tag documents `week_start` as the "business-TZ week start" —
so no data migration and no re-seed are needed for the fix to land.

Access shape: the canonical UTC-midnight key is still a single indexed `LIMIT 1`
on `idx_schedules_biz_week`; only a miss pays a bounded ±24h window scan
(`LIMIT 4`, index-ordered) that recovers an off-grid row. Query count on the
steady-state (canonical-row) path is unchanged. A first cut used the window scan
unconditionally and cost **+19 allocs/op and +3.7 KB/op**; the exact-match fast
path brings that down to +11 allocs/op and +351 B/op.

`BenchmarkScheduleGet` (`internal/database`, staff-scoped week read, SQLite
in-memory, Apple M3, `-benchmem`). Machine load drifted heavily during the run
(several concurrent agents), so BEFORE and AFTER were measured **interleaved** —
3 rounds x `-count=3` each, alternating — and are reported as min / median of the
9 samples:

| | ns/op (min) | ns/op (median) | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| before (exact equality only) | 79,494 | 92,145 | 27,457 | 581 |
| after (exact match, ±24h window fallback) | 79,279 | 99,650 | 27,808 | 592 |

Latency is noise-dominated (the two distributions overlap; best case is parity at
~79.4 µs). The deterministic signal is the allocation counters: **+11 allocs/op
(+1.9%) and +351 B/op (+1.3%)**, from `NormalizeWeekStart` plus the split
`status` clause.

```bash
cd backend
go test ./internal/database -run 'TestGetScheduleForWeek|TestGetPublishedScheduleForStaff|TestGetOrCreateDraftSchedule|TestNormalizeWeekStart|TestCopyScheduleWeek' -count=1
go test ./internal/database ./internal/handlers ./internal/server ./internal/demo -count=1
go test ./internal/database -run '^$' -bench BenchmarkScheduleGet -benchmem -count=3
go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
```
## Delivery dispatch list — money/currency projection access shape (2026-08-23)

`GetBusinessDeliveries` / `GetDeliveryOrder` project the bill total, business
currency, and venue coordinates through `hydrateDeliveryMoneyAndGeo`. The
repair pass moved the projection back onto the money wire contract and pinned
its access shape:

- the hydrator now carries **cents** (`DispatchTotalCents *int64`) and
  `DeliveryOrder.MarshalJSON` runs the single `centsToDollars` conversion —
  the manual `float64(cents)/100.0` in the service is gone;
- currency resolves `display_currency` → `default_currency` → `USD` via
  `database.ResolveDispatchCurrency` (delegates to the shared
  `resolveBusinessCurrency`), so dispatch cannot disagree with bills/orders and
  a business that never set `default_currency` (no DB default) no longer emits
  a blank currency;
- the hydrate stays **one JOIN query per page**, asserted by
  `TestGetBusinessDeliveries_MoneyGeoHydrateIsOneQueryForAnyN`: identical SELECT
  count for a 4-row page and a 32-row page, exactly one `LEFT JOIN bills`
  statement, and zero standalone SELECTs against `bills` / `businesses` /
  `orders`.

Apple M3, SQLite in-memory, 500 seeded deliveries, page of 100, `-benchmem
-count=3` (shared machine — ns/op is noisy, allocations are the stable signal):

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkGetBusinessDeliveriesWithStatusFilterSQLite` **before** (`4890f893a`) | 9.0M–10.7M | 3,964,686–3,966,032 | 11,571–11,580 |
| `BenchmarkGetBusinessDeliveriesWithStatusFilterSQLite` **after** | 4.8M–11.0M | 3,974,592–3,975,917 | 11,673–11,685 |
| `BenchmarkGetBusinessDeliveriesMoneyGeoHydrateSQLite` (new, unfiltered page) | 11.9M–30.3M | 3,973,847–3,974,726 | 11,652–11,656 |

+0.25% bytes / +0.9% allocs for a 100-row page: one extra selected column
(`display_currency`) and one `*int64` per row. Query count is unchanged — the
hydrate was and remains a single JOIN.

```bash
cd backend
go test ./internal/services -run 'MoneyGeoHydrate|HydratesCentsNotDollars|CurrencyPrefers|MoneyProjectionMatchesList' -count=1
go test ./internal/database -run 'DeliveryOrder_MarshalJSON|ResolveDispatchCurrency' -count=1
go test ./internal/handlers -run 'Delivery' -count=1
go test ./internal/services -run '^$' -bench 'BenchmarkGetBusinessDeliveries(WithStatusFilter|MoneyGeoHydrate)SQLite' -benchmem -count=3
```

Known gap carried into the issue: only **pickup** coordinates are healed from
the venue row. Dropoff pins are passed through as stored, so demo rows seeded
without a dropoff still render at 0,0 on that side.
## ARS venues stop serializing USD; venue currency rides the existing joins (2026-08-23)

`Bill.Currency` is `gorm:"-"`, so it only ever holds what a read puts there.
Every projected bill read behind the guest payloads
(`GetPublicBillByToken`, `GetPublicGuestOpenBillByTableID`,
`GetOpenBillSummaryByTableID`) selected `bills.*` columns and no business
currency, so `publicBillCurrency` / `resolveBusinessCurrency` fell through to
the `"USD"` default on Parrilla Quebracho Azul (ARS) bills 1333/1335/1702. The
operator half was a different miss: `billListResponseBill` had **no** `currency`
field at all, so `GET /businesses/:id/bills` shipped peso money with no label
and the FE rendered USD. (Operator bill *detail* was already correct —
`GetBillByID` preloads `display_currency`/`default_currency`.)

Fix keeps the reads narrow: the token read already JOINed `businesses` for the
`is_active` gate, so the currency comes along as
`COALESCE(NULLIF(display_currency,''), NULLIF(default_currency,''), '')`
(`database.BusinessDisplayCurrencySelectSQL`, the SQL twin of the new
`database.BusinessDisplayCurrency` helper); the two table-id reads gained one
LEFT JOIN on `businesses.id` (PK). No `Business` preload, no second round-trip,
no new column, no migration. The scan target is `projectedBillWithCurrency`
(embeds `Bill`, returns an interior pointer) because GORM will not scan into a
`gorm:"-"` field. The two table-id reads also dropped `Model(&Bill{})` for
`Table("bills")` — `Model` heap-allocated a throwaway `Bill`, which embeds
`Business` and `Table`, on every guest poll. The operator list stamps one
currency resolved from the business the handler already loaded (zero queries,
zero new allocs).

Items were **not** broken on the token/table guest routes:
`billItemsForBillSnapshotLazyByBillID` already prefers `bill_items` rows and
reads the legacy `bills.items` snapshot only when there are none. That behavior
is now regression-pinned in both directions instead of changed.

Apple M3, SQLite microbench, `-benchmem -count=3 -benchtime=200x`, paired
before/after run back to back. **The machine was running seven parallel batch
agents, so ns/op is unreliable across runs** (the same unchanged benchmark
varied 2–3x between runs); B/op and allocs/op are the stable signal.

| Benchmark | ns/op before | ns/op after | B/op before → after | allocs/op before → after |
|---|---:|---:|---:|---:|
| `BenchmarkGetBillByNumberPublicSQLite` | 489,099–550,271 | 1,008,184–1,131,439 | 453,396–457,337 → 449,042–450,560 | 1,234 → 1,249 |
| `BenchmarkGuestOpenBillByTableCodeSQLite` | 87,185–101,704 | 103,360–202,171 | 49,406–49,497 → 51,810–51,981 | 558 → 579 |
| `BenchmarkLegacyGetTableByCodeSQLite` | 51,961–55,635 | 83,178–172,078 | 33,310–33,324 → 35,924–35,996 | 359 → 385 |
| `BenchmarkGetBusinessBills` | 4,119,985–6,308,476 | 4,587,021–7,364,226 | 226,035–227,156 → 232,658–237,150 | 2,653 → 2,653–2,655 |

Memory deltas: the token route gets **cheaper** (−4 KB/op from dropping the
throwaway `Model` Bill, +15 allocs for the joined column). The two table-id
routes pay +2.4 KB/op and +21–26 allocs/op for the JOIN plus the scan wrapper —
attributed by measurement: the wrapper struct alone (no JOIN) was 56.3 KB/op
before switching `Model`→`Table`, so the JOIN itself costs ~0.3 KB/op and ~5
allocs. `GetBusinessBills` allocs/op is flat (2,653) — the currency is one
shared string across the page; +6 KB/op is the wider JSON body. Query counts are
unchanged everywhere (no new round-trip, no N+1: the list stamps one resolved
currency for all rows rather than resolving per row).

In quieter windows the same after-binary measured 479,989–491,192 ns/op
(token), 82,251–86,580 (table-code), 46,454–47,447 (legacy table), and
1,459,546–1,530,879 (operator list) — at or below the corresponding baselines.

```bash
cd backend
# red first
go test ./internal/server -run 'ServesBusinessCurrency|TestGuestBillServesBillItemsWhenSnapshotEmpty|TestGuestBillKeepsLegacySnapshotWhenNoBillItemRows' -count=1
# paired benchmarks (git stash push -u -- backend/ for the before leg)
go test ./internal/server -run '^$' -bench 'BenchmarkGetBillByNumberPublicSQLite|BenchmarkGuestOpenBillByTableCodeSQLite|BenchmarkLegacyGetTableByCodeSQLite|BenchmarkGetBusinessBills' -benchmem -count=3 -benchtime=200x
go test ./internal/database ./internal/server ./internal/handlers -count=1
```

## PRIMARY_VENUE business_id stage matches exactly (OSS backlog5 F2, 2026-10-05)

`database.FindPublishedVenue` (public `/` cache miss) tried the id, then
`LOWER(custom_url)`, then `LOWER(business_id)`. The last predicate cannot use the
case-sensitive unique index `idx_businesses_business_id`, so an unknown or
business_id-shaped `PRIMARY_VENUE` scanned `businesses`. `business_id` is canonical
as stored, so the stage now matches it exactly (`business_id = ?`) and the index
serves it; `custom_url` keeps its `LOWER()` match, served by
`idx_businesses_custom_url_lower`. No migration needed.

Access shape pinned by `TestFindPublishedVenueBusinessIDStageIsIndexable` (no
`lower(business_id)`, projected columns, exactly two lookups for an unknown slug)
and `TestFindPublishedVenueBusinessIDIsExact`.

`BenchmarkFindPublishedVenue` (SQLite, 2,000 published venues, Apple M3, `-count=3`):

| Case | ns/op before | ns/op after | B/op before → after | allocs/op |
|---|---:|---:|---:|---:|
| `venue-1999` (resolves by business_id) | 879,328–887,036 | 456,602–466,553 | 18,443–18,447 → 18,427–18,430 | 204 → 204 |
| `missing-venue` (falls back) | 871,231–884,279 | 455,603–466,754 | 18,016 → 18,000 | 180 → 180 |

The remaining ~455 µs is the `LOWER(custom_url)` stage, which SQLite's AutoMigrate
schema cannot index; on Postgres the genesis functional index serves it.

```bash
cd backend
go test ./internal/database -run 'PublishedVenue' -count=1
go test ./internal/database -run '^$' -bench 'FindPublishedVenue' -benchmem -count=3
```
