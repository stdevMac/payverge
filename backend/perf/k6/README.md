# Capacity load harness

This k6 package exercises the launch-critical D5 surfaces against an isolated
Payverge stack: anonymous menus, authenticated operator reads, guest quotes and
orders, callback validation, SSE, queue lag, and Go runtime telemetry.

## Safety boundary

The default target is `http://127.0.0.1:8080`. Every Make target rejects a
`payverge.io` hostname. Burst and soak profiles additionally require
`PERF_EXTERNAL_CONFIRM=isolated-perf`; the five-times proof also requires the
exact deployed `CANDIDATE_SHA`. Do not point these profiles at production or a
shared staging database.

## Repeatable fixtures and authentication

Run migrations, then reseed immediately before each run:

```bash
cd backend
DATABASE_URL='postgres://...' make perf-seed
```

The seeder creates 20 deterministic businesses and menus, one open bill per
business, normalized staff identities/memberships, and one active login code
per manager. `capacity.js` resolves numeric IDs from public endpoints and calls
the real `POST /api/v1/staff/verify-login-code` route during `setup()`. The
default business-one fixture is:

- slug: `perf-seed-001`
- table: `perf-seed-001-t01`
- manager: `perf-seed-staff-001-1@example.test`
- one-use code: `860001`

The code is consumed by authentication; rerunning the seeder restores it. A
pre-issued `STAFF_TOKEN` can be supplied instead, but missing authentication or
an open bill always aborts setup rather than degrading into 401/404 traffic.

## Local smoke

With the isolated stack running and seeded:

```bash
cd backend/perf/k6
make build-k6
make smoke BASE_URL=http://127.0.0.1:8080
```

The smoke lasts 20 seconds. It is the only profile intended for routine local
execution. `build-k6` creates `/tmp/payverge-k6` with pinned xk6, k6, and
`xk6-sse` versions; the repository does not install a global binary.

## External-only profiles

Profiles are documented in `profiles/` and mapped to Make targets:

- `burst.json`: two minutes, two-times projected service burst.
- `soak_1h.json`: one hour at projected peak.
- `candidate_5x.json`: one hour at five-times projected peak; final launch
  proof only.

Example for an explicitly isolated perf host:

```bash
make soak_1h \
  BASE_URL=http://isolated-perf-host:8080 \
  PERF_EXTERNAL_CONFIRM=isolated-perf \
  METRICS_TOKEN='...'
```

The runtime global limiter remains 1,200 requests/minute per source IP and SSE
remains capped at 30 connections per source IP. Higher profiles therefore need
multiple load-generator source IPs. Do not disable those controls to manufacture
a higher throughput result.

## Thresholds and evidence

`lib/thresholds.js` enforces separate tagged p50/p95/p99 and error thresholds
for every request flow. It also gates dropped iterations, SSE event lag,
goroutines, combined queue depth, and background-job lag. The canonical values
and system ceilings live in `../capacity-contract.json`.

Every run writes `/tmp/payverge-perf-results/capacity-summary.json` by default.
Set `RESULT_DIR` to retain it elsewhere. In parallel, capture host/container and
PostgreSQL signals:

```bash
OUTPUT_DIR=/tmp/payverge-perf-results \
DURATION_SECONDS=3600 \
bash ../observe/capture.sh

bash ../observe/evaluate.sh \
  /tmp/payverge-perf-results/capacity-observations.csv
```

The observation CSV contains CPU, memory, disk use, PostgreSQL connections,
waiting locks and wait duration, goroutines, queue depth, and SSE drops. Keep it
with the k6 JSON and exact candidate SHA.

## Known proof boundary

The callback scenario measures the signed-callback boundary's fail-fast invalid
signature path and performs no provider calls. A full signed callback/webhook
burst, and the exact-candidate five-times one-hour soak, require isolated
provider sandboxes and external load generators. They are deliberately not run
by pull-request CI.

The pinned `xk6-sse` client holds real streams and parses the application's JSON
envelope RFC3339 `timestamp`. The external proof must retain
`sse_events_received > 0`; missing events, connection errors, or an empty lag
series fail rather than being reported as success.
