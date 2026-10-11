# D5 capacity and performance readiness

Date: 2026-08-01

This document separates locally verified capacity contracts from proof that
requires an isolated external environment. No production or shared-staging load
was generated while preparing this package.

## Contract

The canonical machine-readable limits are in
`backend/perf/capacity-contract.json`. Each request surface has independent
p50/p95/p99 and error-rate gates, preventing a cheap anonymous read from
masking a slow write or callback. System ceilings cover CPU, container memory,
disk use, PostgreSQL connections and lock wait time,
goroutines, background-job lag, queue depth, and SSE event lag.

Runtime limits remain conservative in the isolated stack:

| Control | Limit |
| --- | ---: |
| Application PostgreSQL pool | 25 open / 10 idle |
| PostgreSQL `max_connections` | 100 |
| Evidence gate for PostgreSQL connections | 70 |
| Waiting PostgreSQL locks | 0 |
| Global source rate | 1,200 requests/minute |
| SSE connections per source / business | 30 / 200 |
| Backend / PostgreSQL container allocation | 2 CPU, 2 GiB each |

## Repeatable workload

`backend/perf/seed` now creates the active state that the old harness required
operators to stage manually: one open bill per business, orderable feature
flags, normalized staff identities/memberships, and one one-use staff login
code per business. The k6 setup resolves numeric IDs, authenticates through the
real staff route, and aborts on missing fixtures or credentials.

`backend/perf/k6/scenarios/capacity.js` exercises:

- anonymous menu reads;
- authenticated operator order-list reads;
- guest order quotes and idempotent order creation;
- fail-fast invalid-signature callback validation without provider side effects;
- authenticated SSE through a pinned `xk6-sse` build with JSON-envelope
  timestamp parsing and independent connection/lag/error gates;
- Prometheus sampling for goroutines, queues, and background-job lag.

The two-minute burst and one-hour soak profiles are external-only. The exact
candidate profile is a one-hour run at five times the projected peak and also
requires a candidate SHA. Per-source rate and SSE controls remain enabled, so
the five-times run requires distributed source IPs.

## Evidence bundle

Retain these artifacts together for a candidate:

1. exact image digest and `CANDIDATE_SHA`;
2. seed command and fixture counts;
3. `capacity-summary.json` from k6, including p50/p95/p99, errors, checks, and
   dropped iterations;
4. `capacity-observations.csv` from `observe/capture.sh`;
5. successful `observe/evaluate.sh` output;
6. PostgreSQL slow-query and container logs for the same time window;
7. load-generator count/source-IP allocation and profile JSON.

An absent metric, fixture, baseline, or observation sample is a failure.
Microbench comparisons run `perf/bench/scripts/validate-inputs.sh` first and
fail when the baseline is absent, empty or unrelated to the candidate.

## Local RED/GREEN evidence

RED before implementation:

- nine D5 surfaces/limits were not represented in one enforceable contract;
- authenticated k6 depended on a manual JWT and open bill;
- SSE parsed a nonexistent `timestamp:<unix-ms>` line;
- no one-hour soak or guarded exact-candidate profile existed;
- weekly drift could compare the latest artifact to itself;
- missing PR/drift baselines were reported as informational success;
- no single sampler captured container, PostgreSQL, queue, and SSE signals.

GREEN commands:

```bash
cd backend
go test ./perf/contracts ./perf/seed
bash perf/bench/scripts/gate_test.sh
bash perf/observe/evaluate_test.sh
```

Local contract/unit proof does not claim launch capacity. The exact-candidate
five-times one-hour soak, signed provider callbacks, and real multi-source SSE
fanout remain external evidence pending an isolated candidate deployment.
