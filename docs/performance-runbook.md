# Payverge Performance Runbook

Last updated: 2026-10-03.

Use this when production latency, throughput, memory, database load, or queue behavior looks abnormal on your install, and when doing backend performance work.

The rule is: stabilize first, measure second, optimize third. Do not patch blindly.

## Scope

This runbook covers backend/API performance, database read/write shape, restaurant hot paths, benchmark reproduction, and production profiling.

Primary references:

| Reference | Purpose |
| --- | --- |
| `backend/perf/bench/README.md` | Go benchmark suite and local benchstat comparison workflow |
| `backend/perf/k6/README.md` | HTTP load tests, profiles, k6 thresholds |
| `backend/perf/seed/README.md` | Deterministic fixture seeding |
| `backend/perf/staging/README.md` | Staging perf stack |
| `backend/perf/grafana/payverge-perf.json` | Grafana dashboard for flow latency and process metrics |
| `summary.md` | Historical notes and recorded before/after benchmark results |

## First 10 Minutes Of A Prod Incident

Goal: decide whether to mitigate, roll back, scale, or investigate in place.

1. Confirm whether this is a real user-facing issue.
2. Identify the affected flow: menu browse, checkout/order create, table QR, payment, delivery, AI, admin dashboard, SSE, or background worker.
3. Check health endpoints before profiling.
4. Check error rate and latency before changing code.
5. If core checkout/payment paths are degraded, prefer rollback or capacity mitigation over live optimization.
6. Capture enough evidence before restarting if the process is alive.

The commands below use two shell variables:

| Variable | Value |
| --- | --- |
| `PUBLIC_URL` | The public origin of your install (the same value as the `PUBLIC_URL` env var), e.g. `https://pos.example.com` |
| `BACKEND_URL` | The backend reached directly from the host, bypassing the reverse proxy, e.g. `http://127.0.0.1:8080` |

Health checks:

```bash
curl -fsS "$PUBLIC_URL/api/v1/health/live"
curl -fsS "$PUBLIC_URL/api/v1/health/ready"
curl -fsS "$PUBLIC_URL/api/v1/health"
curl -fsS "$PUBLIC_URL/api/health"
```

Container status on the host (run from the directory that holds your compose
file; add `-f <file>` if it is not the default name):

```bash
docker compose --env-file .env ps
docker compose --env-file .env logs --tail=300 backend
docker compose --env-file .env logs --tail=300 frontend
```

If a deploy just happened, compare against the last known good image tag or commit.

## Production Signals

### Metrics

`/metrics` is protected by `METRICS_TOKEN` or `METRICS_TOKENS`. Keep the route
off your public edge (have the reverse proxy return `404` for it) and collect it
from the backend on the host via `$BACKEND_URL`; do not expose the metrics token
or open the endpoint publicly.

Prometheus queries to start with:

```promql
histogram_quantile(0.95, sum by (le, flow) (rate(flow_request_duration_seconds_bucket[5m])))
histogram_quantile(0.99, sum by (le, flow) (rate(flow_request_duration_seconds_bucket[5m])))
sum by (flow, status) (rate(flow_request_duration_seconds_count[5m]))
sum by (path, method, status) (rate(http_requests_total{status=~"5.."}[5m]))
```

Current tagged flows:

| Flow label | Source | Meaning |
| --- | --- | --- |
| `menu_browse` | Public menu/table-code menu paths | Guest menu browsing |
| `checkout` | Order create handler | Guest/staff order submission |

SSE is intentionally not tagged with `flow_request_duration_seconds` because request duration equals connection lifetime, not server work. Use k6 SSE metrics and the SSE broadcast benchmark instead.

### Slow Queries

`DB_SLOW_QUERY_LOG=true` switches GORM to a warning logger for queries above 100ms.

Use it in staging first. In production, enable it only for a short diagnostic window because it can increase log volume.

```bash
DB_SLOW_QUERY_LOG=true
```

Watch for:

| Pattern | Likely cause |
| --- | --- |
| `SELECT * FROM bills` on polling/list routes | Full aggregate hydration where a projection should be used |
| Reads of `bills.items` on modern bills | Legacy JSON snapshot loaded instead of `bill_items` relation |
| N+1 `bills`, `tables`, or `businesses` reads | Handler rehydrating relations per row |
| Repeated menu/offer/bundle reads for same business | Missing or invalidated pricing/menu snapshot cache |
| Long transcript queries | Missing bounds, missing ordered index, or full-row message hydration |

## Profiling Safely

`PPROF_TOKEN` gates both `/debug/pprof/*` and `/internal/_pprof_snapshot`. If the env var is unset, the routes are not registered.

Never expose `PPROF_TOKEN` in logs, tickets, screenshots, or shell history shared outside the operator group.

CPU profile:

```bash
curl -fsS \
  -H "X-Pprof-Token: $PPROF_TOKEN" \
  "$BACKEND_URL/debug/pprof/profile?seconds=30" \
  -o /tmp/payverge-cpu.pprof

go tool pprof -top /tmp/payverge-cpu.pprof
go tool pprof -http=:0 /tmp/payverge-cpu.pprof
```

Heap profile:

```bash
curl -fsS \
  -H "X-Pprof-Token: $PPROF_TOKEN" \
  "$BACKEND_URL/debug/pprof/heap" \
  -o /tmp/payverge-heap.pprof

go tool pprof -top /tmp/payverge-heap.pprof
```

Goroutines and heap snapshot bundle:

```bash
curl -fsS \
  -H "X-Pprof-Token: $PPROF_TOKEN" \
  "$BACKEND_URL/internal/_pprof_snapshot" \
  -o /tmp/payverge-pprof-snapshot.zip
```

When production is under severe load, prefer a 10-30 second CPU profile. Avoid repeated long profiles during saturation.

## Reproduce Outside Production

### Local Go Benchmarks

Use Go benchmarks for handler/service/database changes. They are the fastest way to validate allocations and query-shape changes.

```bash
cd backend
export JWT_SECRET_KEY=local-dev-secret
make bench-ci
```

For a focused benchmark:

```bash
cd backend
export JWT_SECRET_KEY=local-dev-secret
go test -run=^$ -bench='BenchmarkOrderCreate$' -benchmem -count=6 -benchtime=2s ./internal/handlers/...
```

Compare with `benchstat` when baseline output is available:

```bash
benchstat baseline/bench.txt bench.txt
```

Important:

| Constraint | Detail |
| --- | --- |
| Docker required | Testcontainers-backed benchmarks need a working Docker daemon |
| `JWT_SECRET_KEY` required | Auth benchmarks panic without it |
| Log noise | `make bench-ci` writes `bench.raw.txt` and cleans output into `bench.txt` |
| Docker unavailable | Use SQLite microbenches only as a temporary diagnostic fallback; final validation should use Postgres where possible |

### Staging Load Tests

Use k6 for over-the-wire behavior, saturation, and mixed-flow testing.

Point `BASE_URL` at a staging copy of your install, never at production:

```bash
cd backend
make k6-smoke BASE_URL="$STAGING_URL"
make k6-burst PERF_EXTERNAL_CONFIRM=isolated-perf BASE_URL="$STAGING_URL"
```

See `backend/perf/k6/README.md` for the full profile list and what
`PERF_EXTERNAL_CONFIRM` guards.

Thresholds from `backend/perf/k6/lib/thresholds.js`:

| Scenario | SLO |
| --- | --- |
| menu browse | error rate < 0.1%, p95 < 200ms, p99 < 500ms |
| checkout | error rate < 0.5%, p95 < 800ms, p99 < 2s |
| SSE | connect p95 < 300ms, event lag p95 < 500ms |

Before k6 runs, seed deterministic fixtures against the target DB:

```bash
cd backend
DATABASE_URL="$DATABASE_URL" make perf-seed
```

Check `backend/perf/k6/README.md` for auth-gated scenario requirements such as `STAFF_TOKEN` and `OPEN_BILL_ID`.

## Optimization Decision Tree

Start by classifying the bottleneck:

| Symptom | First checks | Common fix shape |
| --- | --- | --- |
| High p95 on menu/table QR | `menu_browse` histogram, menu/promotion queries | Reuse pricing/menu snapshot, bound response shape, add index |
| Slow checkout/order create | `checkout` histogram, order create benchmark, DB transaction profile | Remove duplicate preloads, reuse in-memory bill/business/menu data, gate optional side effects |
| High DB CPU | slow query logs, query count tests, EXPLAIN in staging | Projection query, composite index, aggregate in SQL |
| High memory/allocs | heap profile, `-benchmem`, large JSON columns | Avoid full ORM aggregate hydration, skip legacy snapshots, manual hot-path encoding only when justified |
| SSE lag | k6 SSE profile, goroutine snapshot, hub benchmark | Check fanout/backpressure/drop behavior before changing HTTP middleware |
| AI/Director transcript latency | message count, ordered index, response limit | Bound latest messages, project UI fields, skip large tool payloads |
| Payment history/export slow | query count, joins, heap | Single projected join instead of per-row bill/table hydration |
| Telegram/plugin queue overhead | outbox writes, connected plugin state | Gate optional notification writes and batch worker finalization |

## Code Patterns That Worked

Prefer these before more complex data structures:

| Pattern | Use when |
| --- | --- |
| Narrow projection structs | A route returns only a subset of a large model |
| `EXISTS` probes | The handler only needs to know if a record exists |
| SQL aggregates | The code only needs counts or sums |
| Bounded latest-history queries | Chat/transcript/session views can page or cap results |
| Short TTL snapshots | Low-churn business/menu/promotion data is read repeatedly during traffic bursts |
| One-pass joined input loaders | A service currently loads the same bill/business/table more than once |
| Optional side-effect gates | Telegram, plugin notifications, metrics, or outbox work is not needed for every business |
| Batch finalization | Workers update many rows one by one after successful processing |

Use more complex data structures only after profiling proves the current access pattern is still the bottleneck. Most past gains came from data shape, not algorithmic cleverness.

## Code Patterns To Avoid

| Anti-pattern | Why |
| --- | --- |
| Full `Preload` trees for list/polling routes | Hydrates columns and relations the response does not render |
| Reading `bills.items` by default | Legacy JSON snapshots can be large; prefer normalized `bill_items` first |
| Unbounded message/transcript reads | Old conversations can dominate memory and latency |
| Per-row relation reloads in response loops | Creates N+1 latency and heap pressure |
| Debug stdout in hot paths | Contaminates benchmark output and adds request work |
| Optimizing without a before/after bench | Makes regressions hard to detect |
| Enabling GORM `PrepareStmt` globally | Previously regressed `BenchmarkOrderCreate`; pgx already prepares internally |
| Caching mutable ORM objects without discipline | Can leak state across requests/tests; cache read-only snapshots instead |

## Required Workflow For Future Perf Work

For any backend change that may affect latency, allocations, query count, or throughput:

1. Add or update a benchmark before optimizing when a relevant benchmark does not already exist.
2. Add a regression/access-shape test when changing preload/projection behavior.
3. Capture baseline benchmark output.
4. Make the smallest behavior-preserving optimization.
5. Run focused unit/regression tests.
6. Run the focused benchmark with `-benchmem`.
7. Compare with `benchstat` or documented before/after numbers.
8. Run the relevant broader package tests.
9. Update docs when adding a benchmark, k6 scenario, metric, or new perf-sensitive convention.

Minimum local validation example:

```bash
cd backend
go test ./internal/server ./internal/handlers ./internal/database -run 'RelevantTestName' -count=1
JWT_SECRET_KEY=local-dev-secret go test -run=^$ -bench='BenchmarkRelevantPath$' -benchmem -count=6 ./internal/server/...
```

If the change affects frontend API contracts, also run the relevant Jest tests:

```bash
cd frontend
npm test -- --watchman=false --runInBand src/api --testTimeout=10000
```

## Adding A New Benchmark

Use the closest existing benchmark as the template:

| New path type | Start from |
| --- | --- |
| Public route | `backend/internal/server/menu_bench_test.go` |
| Authenticated handler | `backend/internal/handlers/orders_bench_test.go` |
| Database read shape | `backend/internal/database/orders_list_perf_test.go` |
| Service operation | `backend/internal/services/delivery_perf_test.go` |
| JSON encoding | `backend/internal/database/models_json_bench_test.go` |
| SSE fanout | `backend/perf/bench/sse_broadcast_bench_test.go` |

Benchmark requirements:

| Requirement | Reason |
| --- | --- |
| Deterministic fixture setup | Results must be comparable across runs |
| `b.ResetTimer()` after setup | Exclude container/migration/fixture time |
| `b.ReportAllocs()` or `-benchmem` | Allocation regressions are common in this codebase |
| Clear response/behavior assertion | Prevent benchmarks from measuring broken no-op paths |
| README update | Future agents need to know the benchmark exists |

## Production Change Safety

During an active incident, prefer reversible mitigations:

| Mitigation | When to use |
| --- | --- |
| Roll back to last known good image | Regression began after deploy |
| Reduce optional traffic | Expensive non-critical flows are saturating backend |
| Temporarily disable or gate optional integrations | Telegram/plugin side effects are causing queue or DB pressure |
| Increase capacity | CPU-bound workload is understood and horizontal scaling is available |
| Enable slow-query logs briefly | DB bottleneck is suspected but not identified |

Do not deploy speculative rewrites during an active incident unless rollback is not viable and the bottleneck is already proven.

## Post-Incident Follow-Up Template

Record this in the incident ticket or a short markdown note:

```markdown
## Performance Incident Follow-Up

- Date/time:
- Affected flows:
- User-visible symptom:
- First bad deploy or traffic event:
- Metrics observed:
- Profiles captured:
- Slow queries:
- Mitigation used:
- Root cause:
- Benchmark added or updated:
- Before/after benchmark:
- Regression test added:
- Follow-up work:
```

## Current Known Limits

| Area | Limit |
| --- | --- |
| k6 auth-gated flows | `STAFF_TOKEN` and `OPEN_BILL_ID` still require operator setup |
| Docker dependency | Testcontainers benchmarks depend on Docker health |
| SSE event lag | k6 lag trend depends on timestamp availability in SSE payloads |
| `flow_request_duration_seconds` coverage | Only explicitly tagged request-shaped flows are included |

## Definition Of Done For Perf Improvements

A perf change is not done until:

1. The bottleneck is measured.
2. The optimization is behavior-preserving.
3. A benchmark or load-test covers the path.
4. Relevant tests pass.
5. Before/after numbers are recorded.
6. Any new operational command, metric, or convention is documented.
