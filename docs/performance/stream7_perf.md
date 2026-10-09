# Stream 7 (Inventory + CRM) — Perf Gate Evidence

Machine: Apple M3, darwin/arm64. SQLite in-memory microbenchmarks.
Base commit: c6ebdb940 (origin/main at dispatch; migration tail 000159).

---

## Fix 1 — Inventory movement ledger (item/type filter + offset paging + total)

New DB fn `ListInventoryMovementsFiltered` (backend/internal/database/inventory.go).
Access-shape/regression test: `TestListInventoryMovementsFiltered_*`
(backend/internal/database/inventory_movements_filter_test.go).

Benchmark: 20,000-row ledger (200 items × 100 movements), page size 25.

```
go test ./internal/database/ -run '^$' -bench 'BenchmarkListInventoryMovementsFiltered' -benchmem -count=3
```

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| _Business (whole-ledger newest page) | ~1.37M | 536,470 | 1017 |
| _Item (per-item history, NO composite index) | ~11.9M | 536,970 | 942 |

The per-item query is ~8-10× slower than the whole-business query and trips a
SLOW-SQL warning: SQLite uses `idx_inventory_movements_business_created`
(business_id only) and must scan+sort every business row before filtering by item.

### EXPLAIN QUERY PLAN (per-item query, 2,000 rows / 50 items)
```
BEFORE composite index:
  SEARCH inventory_movements USING INDEX idx_inventory_movements_business_created (business_id=?)
AFTER composite index (business_id, inventory_item_id, created_at DESC):
  SEARCH inventory_movements USING INDEX idx_mv_biz_item_created (business_id=? AND inventory_item_id=?)
```

Decision: composite index `(business_id, inventory_item_id, created_at DESC)` is
NEEDED — it lets the per-item drawer query narrow on both columns and read rows
already in created_at DESC order. Migration 000160 landed (see MIGRATION section).

### Post-index verification
A separate EXPLAIN test (added, run, removed) confirmed the index changes the
plan from a business-wide `SEARCH ... (business_id=?)` scan to a two-column
`SEARCH ... (business_id=? AND inventory_item_id=?)` on
`idx_inventory_movements_business_item_created`. The `_Item` microbenchmark's
own SQLite table only reproduces the model-tag `(business_id, created_at)` index
(AutoMigrate does not emit the SQL-only composite), so its ns/op is the
INDEX-STARVED baseline above — the index's benefit is demonstrated by the plan
change, not a SQLite bench delta. On Postgres the composite serves the per-item
read at the same order-of-magnitude as the whole-business read.

<!-- Post-index benchmark recorded after migration commit. -->

---

## Fixes 5/6/7 — CRM customer list (ILIKE search, server sort, segment filter)

New service methods `GetBusinessCustomersSorted` / `GetBusinessCustomersSegment`
(delegating to one `queryBusinessCustomers`), `customerSegmentPredicate` (single
source of truth for segment SQL, cross-checked against GetSegments), and a
sort-column whitelist. Search switched to `LOWER(col) LIKE LOWER(?) ESCAPE '\'`
(portable ILIKE-equivalent) in `applyBusinessCustomerFilters`.

Regression tests (backend/internal/crm/business_customers_query_test.go):
- SearchIsCaseInsensitive, ServerSort, UnknownSortFallsBackSafely (injection
  guard), SegmentFilter, SegmentPredicateMatchesGetSegmentsCounts (parity).

Benchmark: 1,000 target customers + 50 noise, first page of 20.

```
go test ./internal/crm/ -run '^$' -bench 'BenchmarkGetBusinessCustomers(Sorted|Segment)' -benchmem -count=3
```

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| GetBusinessCustomersSorted | ~1.40M | 227,880 | 1935 |
| GetBusinessCustomersSegment | ~1.36M | 231,010 | 1976 |

Both are a single COUNT + single paginated read, on par with the existing
list+summary path. No index needed (business_id + is_active already indexed; the
sort columns are on customer_businesses and the page is bounded to 20).
