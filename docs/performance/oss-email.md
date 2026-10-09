# OSS email workstream: perf-gate evidence

> Point-in-time evidence. Migration numbers cited here (0002xx) predate the
> squash into `backend/schema/genesis/current_schema.sql`; that schema now
> lives in the genesis baseline, and numbered migrations restart at 000001.

Machine: Apple M3, darwin/arm64, go1.26.2. SQLite in-memory microbenchmarks.
Base commit: e26f72b6d (`oss/main` at dispatch; migration head 000222).
No migrations, indexes or new tables in this workstream.

## Hot path: `verification.IsOperatorVerified`

`verifyPersistedOperatorSession`
(backend/internal/server/verified_email_session_boundary.go) calls it on every
authenticated email/register request. The email workstream made it
mode-aware:

- `EMAIL_VERIFICATION=required` keeps the base predicates (both
  `email_verified` flags plus the email binding).
- `off` checks only the binding, since off mode no longer writes
  `email_verified=true`.

The mode is read from the environment on each call through
`config.EmailVerificationRequired()`, which also checks for stranded delivery
credentials. That is a few `os.Getenv` calls and no allocation on the normal
path.

The first mode-aware version chained a third `Where`, which cost 9 extra
allocations per request. The final version issues one `Where` per shape, so
the request now allocates less than base.

- Access-shape test: `TestIsOperatorVerifiedAccessShape`
  (backend/internal/verification/is_operator_verified_shape_test.go). It
  asserts exactly one COUNT statement and no `SELECT *`. The binding predicate
  is always present, and the verified predicates appear only under `required`.
- Benchmark: `BenchmarkIsOperatorVerified`
  (backend/internal/verification/is_operator_verified_bench_test.go).
- Baseline: a temporary copy of the e26f72b6d function, benchmarked with the
  same fixture and then deleted.

```
cd backend
PLUGIN_SECRET_KEY=<any-32-byte-test-key> GO_ENV=test go test -short -run '^$' \
  -bench 'BenchmarkIsOperatorVerified' -benchmem -count=3 ./internal/verification
```

| Variant | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| base e26f72b6d, required, verified user | 10,865 / 11,397 / 10,751 | ~5,526 | 68 |
| first mode-aware version (chained `Where`), required | 10,724 / 13,878 / 12,635 | ~5,803 | 77 |
| final, required, verified user | 10,729 / 9,774 / 10,371 | ~5,029 | **57** |
| final, off, unverified user | 9,343 / 8,689 / 9,080 | ~4,236 | **54** |

Delta, final vs base, with `required`: -11 allocs/op (-16%) and about
-500 B/op (-9%). ns/op is within noise, about 5% faster on the median. Off
mode is cheaper again, because it has two fewer bound parameters.

## Other per-request readers

- `GetSessionInfo` (backend/internal/server/session_info_handler.go) now looks
  up `user_auths` only when verification is required. In off mode it skips a
  query it used to run, and with `required` the shape is unchanged.
- `ownerEmailUnverified` (backend/internal/services/lifecycle_scheduler.go)
  runs in the background lifecycle scheduler, not on a request path. In off
  mode it returns early without a query.

Neither of these adds a query or a preload, so they have no separate
benchmark.
