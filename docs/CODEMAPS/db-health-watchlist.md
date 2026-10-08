# DB Health Watch-List

A lightweight runbook for catching Postgres tables that are *fine today* but will
degrade as row counts grow. It started when `exchange_rates` was found to grow
without bound because a rate was stored on every poll (now fixed: rates are
stored only when they change).

## Mental model

A sequential scan is **not** a problem on its own. On a tiny table the planner
*correctly* prefers a seq scan, and an index on a tiny table is ignored. The
danger is a table that the planner currently seq-scans because it's small, then
keeps seq-scanning out of habit as it grows past the point where an index would
win. The job here is to notice that flip *before* it hurts, not to pre-index
small tables.

**Do not add preventive indexes to tiny tables.** They won't be used and they
slow writes. Add an index only when a table on this list crosses its threshold
*and* a real query is scanning it hot.

## Watch-list

Tables that are small on a typical install but sit on hot paths. Re-check them
periodically on your own database with the queries below.

| Table                | Re-check at | Notes |
|----------------------|-------------|-------|
| `bills`              | **~5k rows** | First likely to flip; watch `WHERE` shapes on the hot list/dashboard paths. |
| `table_reservations` | ~10k rows   | Reservation list/availability queries are the ones to index when it grows. |
| `tables`             | ~10k rows   | Bounded by physical tables per business. |
| `user_sessions`      | ~5k rows    | A high seq-scan count on a tiny table is just frequent auth checks. |
| `report_schedules`   | ~5k rows    | Same: the hourly scheduler polls a tiny table. |
| `exchange_rates`     | after upgrades | Rates are stored only when they change. Row count should track real rate volatility. |

When a table crosses its threshold, run the per-table query below, look at which
columns the hot queries filter/sort on, and add a composite index that matches.

## Diagnostic queries

Seq-scan pressure vs. table size (the "is anything flipping?" overview):

```sql
SELECT
  relname AS table,
  n_live_tup AS live_rows,
  seq_scan,
  seq_tup_read,
  CASE WHEN seq_scan > 0 THEN seq_tup_read / seq_scan ELSE 0 END AS avg_rows_per_seq_scan,
  idx_scan
FROM pg_stat_user_tables
ORDER BY seq_tup_read DESC
LIMIT 25;
```

A table is a candidate for indexing when `live_rows` is large **and**
`avg_rows_per_seq_scan` is a large fraction of it **and** `idx_scan` is low.

Table + index sizes (the "what's eating disk?" view):

```sql
SELECT
  relname AS table,
  pg_size_pretty(pg_total_relation_size(relid)) AS total_size,
  pg_size_pretty(pg_relation_size(relid)) AS table_size,
  pg_size_pretty(pg_indexes_size(relid)) AS indexes_size,
  n_live_tup AS live_rows
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT 25;
```

`exchange_rates` health (row growth should track real rate volatility, not
the polling interval):

```sql
SELECT count(*) AS rows,
       count(DISTINCT (from_currency, to_currency)) AS pairs,
       pg_size_pretty(pg_total_relation_size('exchange_rates')) AS total_size
FROM exchange_rates;

-- Confirm the composite index exists and the legacy single-column ones are gone.
SELECT indexname FROM pg_indexes WHERE tablename = 'exchange_rates';
```

Telegram outbox health (rows stuck in `pending`/`retry`):

```sql
SELECT plugin_name, status, count(*), min(created_at) AS oldest
FROM plugin_notification_deliveries
GROUP BY plugin_name, status
ORDER BY plugin_name, status;
```

A growing `pending`/`retry` count with an old `oldest` for a plugin whose worker
isn't running means the worker is misconfigured — the janitor will expire those
rows after `PLUGIN_NOTIFICATION_TTL_HOURS` (default 24h), and the startup/periodic
WARN logs call it out.

## Deferred follow-ups (out of scope, recommended next)

These are not set up by default, but close the observability gap when you're
ready:

1. **`pg_stat_statements`** — add to the Postgres service so per-query timings
   become available:
   ```yaml
   # docker-compose: postgres service
   command:
     - "postgres"
     - "-c"
     - "shared_preload_libraries=pg_stat_statements"
   ```
   then once, in the DB: `CREATE EXTENSION IF NOT EXISTS pg_stat_statements;`
   (requires a Postgres restart). Top offenders:
   ```sql
   SELECT query, calls, mean_exec_time, total_exec_time
   FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 25;
   ```

2. **Expose `/metrics`** — the endpoint already exists in code, gated on a token.
   Set `METRICS_TOKEN` (or `METRICS_TOKENS` for rotation), allow the path through
   Caddy with the bearer token, and add a Prometheus scrape job. This surfaces
   the latency histograms and the existing `PluginNotification*` counters the app
   already emits.
