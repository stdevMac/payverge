# Perf benches

Go benchmarks that cover the hot paths from the API benchmarking plan. They run
locally via `make bench` (or the shorter `make bench-ci` variant).

## Run locally

```bash
cd backend
export JWT_SECRET_KEY=local-dev-secret   # required by BenchmarkAuthMiddleware
make bench                                # count=6 for stable benchstat stats
```

Requirements:
- Docker daemon running (testcontainers spins up a Postgres 18 container per
  bench package that needs a DB).
- `JWT_SECRET_KEY` env var set (`internal/structs.GetSecretKey()` panics if it
  is missing, and `BenchmarkAuthMiddleware` calls it during setup).

For a quicker pass while iterating, the CI variant uses `count=3` with a short
explicit benchtime:

```bash
make bench-ci   # writes backend/bench.txt (and bench.raw.txt with full noise)
```

`bench-ci` pipes the raw `go test -bench` output through
`perf/bench/scripts/strip-log-noise.awk`, which removes `log.Println` lines
that the `testing` package interleaves with bench output on stdout. Without
that filter, benchstat silently drops any bench whose name shares a line with
a log timestamp (the RBAC bootstrap is the loudest offender). The unfiltered
output is preserved as `bench.raw.txt` for debugging.

## Benches in this suite

| Bench | Package | Measures |
|---|---|---|
| `BenchmarkMenuGet` | `internal/server` | Public menu read, no items expanded |
| `BenchmarkMenuGetWithItems` | `internal/server` | Public menu read with items joined in |
| `BenchmarkOrderCreate` | `internal/handlers` | POST `/orders` end-to-end through handler + DB |
| `BenchmarkSSEBroadcast` | `perf/bench` | Fanout cost of a single SSE event to many subscribers |
| `BenchmarkSSEBroadcast_LowFanout` | `perf/bench` | Same path with a small subscriber set (baseline) |
| `BenchmarkAuthMiddleware` | `internal/server` | JWT parse + RBAC lookup on the auth middleware chain |
| `BenchmarkBillMarshalJSON` | `internal/database` | Money wire contract: cents → dollars serialization |

## Comparing runs

Capture a baseline on the base branch and a candidate on your branch with
`make bench-ci`, then compare them:

```bash
bash perf/bench/scripts/validate-inputs.sh baseline/bench.txt bench.txt
benchstat baseline/bench.txt bench.txt
go run ./perf/drift/compare baseline/bench.txt bench.txt   # top-5 movers as Markdown
```

`validate-inputs.sh` rejects an absent, empty or unrelated baseline before
benchstat runs, so a comparison never passes against nothing. Treat a time
regression of 15% or more on a hot path as a blocker and record the
before/after numbers in `summary.md`.

## Adding a new bench

Use `BenchmarkMenuGet` in `backend/internal/server/menu_bench_test.go` as the
template. The setup pattern is:

1. Call `genesisdb.Start(ctx)` from `backend/internal/testperf/genesisdb` to get
   an isolated Postgres (a child of `TEST_DATABASE_URL`, or a testcontainer)
   bootstrapped from the genesis baseline, then `database.SetTestDB(pg.DB)`.
   `setupServerBenchmarkDB` in `internal/server/postgres_bench_test.go` does
   this once per package and truncates between benches.
2. Call `testperf.LoadFixtures(db)` from `backend/internal/testperf` to seed the
   deterministic business/menu/staff fixtures.
3. Reset the timer (`b.ResetTimer()`) before the loop so container/fixture
   setup time is excluded.
4. Register the bench in this README's table.

Make sure the package the new bench lives in is already covered by the
discovery globs in `Makefile` (`./internal/server/...`,
`./internal/handlers/...`, `./internal/database/...`, `./perf/bench/...`). If
you add a bench under a new package tree, extend the globs in both `bench` and
`bench-ci` targets.

## Reading benchstat output

Each row in the diff is `(metric, geomean, delta, p-value)`. The `delta` column
is the percent change of the geomean across the two inputs; a `p-value` of `~`
means "no statistically significant change at p < 0.05" — treat those as
noise. A trustworthy regression looks like a positive `delta` (`+18.4%`) with a
small p-value (`p=0.002`). Run with `-count` >=6 locally if a CI delta looks
suspicious — CI runs at `count=3 -benchtime=100ms` to keep the job under
45 min as the benchmark suite grows.
