> Short commit SHAs and branch names in this log refer to pre-release private history. They do not resolve in the public repository.

# Storage driver: performance evidence (2026-10-03)

This covers the storage change: the pluggable object store, the local
driver, `GET /media/*key`, and the rate-limiter exemption for `/media`. The
Backend Performance Gate applies for two reasons. `/media` is a new public
route that loads many times per menu page, and the change touches the
per-IP rate limiter middleware that every route passes through.

**Environment.** Apple M3, Go 1.26.6, a shared developer machine. Every run
used the hermetic test env and these flags:

- `-benchmem -count=3`
- `-benchtime=20000x` for the middleware benchmarks

## 1. Rate limiter: `/media` exemption

`/media/*` is exempt from the per-IP API limiter, in the same way as the health
probes. A guest menu with 40 dish photos no longer uses up the API budget, and
the hot path skips the visitor lookup and the token-bucket bookkeeping.

```bash
go test -run '^$' -bench 'BenchmarkRateLimiter$|BenchmarkRateLimiterMediaPath' \
  -benchmem -benchtime=20000x -count=3 ./internal/middleware/
```

| Benchmark | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs | After allocs |
|---|---|---|---|---|---|---|
| `BenchmarkRateLimiterMediaPath` | 15916 / 13417 / 15132 | 1211 / 1257 / 1229 | 65255 | 5794 | 38 | 15 |
| `BenchmarkRateLimiter` (API path, control) | 12220 / 15350 / 19136 | 15218 / 9246 / 9252 | 60645 | 60611 | 34 | 33 |

- **`/media` requests.** About 12x faster and about 11x fewer bytes allocated.
- **API path.** Unchanged within noise. B/op and allocs/op are flat, and the
  ns/op spread is noise from the shared host.
- **Regression test.** `TestMediaExemptFromRateLimit` checks two things:
  `/media` stays unthrottled past the limit, and `/api` is still throttled.

## 1b. Per-IP limiter on `/media` (review fix F5)

The exemption above left `/media` with no throttle at all. That matters on the
`s3` driver without `S3_PUBLIC_BASE_URL`, where each request is a bucket `GET`
streamed through the backend. The route now has its own per-IP token bucket,
`middleware.MediaRateLimit`. It defaults to 1800/min with a burst of 600 and is
wired in `registerMediaRoutes`. The API budget is untouched.

"Before" is the route with no limiter, which is how it shipped. "After" adds
the media limiter in front of the same no-op handler. The budget is set too
high to bind, so the benchmark times the bookkeeping, not rejections.

```bash
go test -run '^$' -bench BenchmarkMediaRateLimit -benchmem -benchtime=200000x -count=5 ./internal/middleware/
```

| Benchmark | ns/op (5 runs) | B/op | allocs/op |
|---|---|---|---|
| `BenchmarkMediaRateLimit/none` (before) | 2977 / 3190 / 7487 / 8991 / 6273 | 5428 | 13 |
| `BenchmarkMediaRateLimit/limited` (after) | 7145 / 6893 / 6667 / 6724 / 6022 | 5460 | 15 |

- **Cost.** +32 B and +2 allocs per request: the client-IP parse and the bucket
  lookup under one mutex. ns/op overlaps between the two cases on this shared
  host, so the gap is below the noise. Either way it is small next to a
  `/media` `GET` (about 45 µs for 48 KiB on the local driver, section 2).
- **Memory.** One `rate.Limiter` per client IP. Entries idle for 10 minutes
  are evicted, the same as every other `RateLimiter`.
- **Regression tests.**
  - `TestMediaRateLimitThrottlesPastBudget`: 429 past the burst, GET and HEAD
    share one bucket, `Retry-After` and `no-store` are set, buckets are per IP.
  - `TestMediaRateLimitKeepsAPIBudgetSeparate`: media traffic does not spend
    the API budget.
  - `TestMediaRateLimitDisabled`: `0` turns the limiter off.
  - `TestRegisterMediaRoutesRateLimited` and
    `TestRegisterMediaRoutesRateLimitDisabled` (cmd/app): the env wiring.

### 1c. Internal-peer bucket (review fix R2-4)

next/image optimizer misses and server renders reach `/media` from the
frontend container with no `X-Forwarded-For`. They all landed in one per-IP
bucket keyed on the frontend's address, so a cold optimizer cache on a large
menu could 429 every guest at once. An unforwarded loopback or private peer
(no `X-Forwarded-For` or `X-Real-IP`, and Gin's client IP is the peer itself)
now gets its own bucket with `MediaRateLimitInternalMultiplier` (10) times the
rate and burst. Relayed and public clients keep the per-client budget.

The predicate parses with `net/netip` and reads headers by canonical key
straight from the map. The first draft used `net.ParseIP` and
`GetHeader("X-Real-IP")` and cost +16 B and +1 alloc per request, from the
non-canonical key being canonicalized. The final version costs nothing in B/op
or allocs/op.

HEAD (86c531f6d) and the change, run back to back on an Apple M3
with `-count=3`. `limited` uses RemoteAddr `192.168.1.1` (the internal bucket
after the change). `limited_public` uses `203.0.113.7` (the per-IP bucket):

```bash
go test -run '^$' -bench BenchmarkMediaRateLimit -benchmem -count=3 ./internal/middleware/
```

| Benchmark | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| `none` (before) | 1612 / 2589 / 3503 | 5429 | 13 |
| `limited` (before) | 4146 / 3566 / 2279 | 5461 | 15 |
| `none` (after) | 2494 / 1154 / 1106 | 5428 | 13 |
| `limited` (after, internal bucket) | 1327 / 1822 / 1430 | 5461 | 15 |
| `limited_public` (after, per-IP bucket) | 1506 / 1326 / 1400 | 5461 | 15 |

- **Cost.** No change in B/op or allocs/op. ns/op stays inside the host's noise
  band: `none` alone spans 1.1 to 3.5 µs across these six runs.
- **Memory.** One more `RateLimiter` and its cleanup goroutine per process. It
  holds one entry per internal peer, so in practice one entry, the frontend.
- **Abuse bound.** A private peer gains at most 10x one bucket. A trusted
  private proxy could already pick its bucket through `X-Forwarded-For`.
- **Regression tests.**
  - `TestMediaRateLimitGivesUnforwardedInternalPeerItsOwnBudget`: v4, v6,
    loopback and IPv4-mapped peers get burst x 10, and a public client keeps
    its own burst alongside. This test fails if the internal path is disabled.
  - `TestMediaRateLimitForwardedRequestsKeepTheClientBudget`: `X-Forwarded-For`
    (public or private guest), `X-Real-IP` and a public peer get the per-client
    burst.
  - `TestMediaRateLimitTrustedPlatformClientIsNotInternal`: a `CF-Connecting-IP`
    client relayed by a private tunnel peer is not internal.
  - `TestMediaRateLimitHugeBudgetDoesNotOverflow`: the multiplier saturates.

## 2. `/media` serving (local driver)

This is a new route, so there is no "before". These numbers are the baseline
for future changes. The benchmark covers a full 48 KiB image `GET` and a
conditional `If-None-Match` revalidation that answers `304`.

```bash
go test -run '^$' -bench BenchmarkServeMedia -benchmem -count=3 ./internal/s3/
```

| Benchmark | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| `BenchmarkServeMedia/get` (48 KiB, 200) | 45035 / 45571 / 41236 | ~143.5 K | 63 |
| `BenchmarkServeMedia/revalidate_304` | 19258 / 19067 / 19117 | 9489 | 50 |

Notes:

- **What `/get` B/op measures.** Most of it is the `httptest.ResponseRecorder`
  buffering the 48 KiB body. The handler itself streams the file through
  `http.ServeContent`: the object is opened and copied, never read fully into
  memory.
- **Cost of a revalidation.** A `304` costs one `stat` of the object plus one
  read of its JSON metadata sidecar, and no read of the object itself.
  Content-addressed upload keys are served `immutable`, so browsers rarely
  revalidate them at all.
- **ETags.** They come from the sha256 that was computed once at write time
  and stored in the sidecar. Serving never re-hashes the object.

## 2b. Symlink-safe local reads (review fix F6)

The local driver now resolves every path through `os.Root`, with sub-roots
opened once for `objects/` and `meta/`, so a symlink planted in the tree cannot
reach outside the store. `os.Root` walks the path one directory at a time with
`openat` and refuses any link that leaves the root. A key like
`businesses/1/menu_items/<file>` costs about 6 extra syscalls per walk (3
directory opens and 3 closes). A request does two walks, the object and its
sidecar, so it pays about 12 extra syscalls. The kernel used to resolve the
path in one call.

The measurement compares two test binaries built from the parent commit and
from this change, run alternately so that both see the same host load. The
host load average was 23-25 throughout.

```bash
go test -c -o s3-before.test ./internal/s3/   # parent commit's store_local.go
go test -c -o s3-after.test  ./internal/s3/   # this change
# run alternately, 5 rounds each:
./s3-$v.test -test.run '^$' -test.bench BenchmarkServeMedia -test.benchmem -test.count=1 -test.benchtime=3000x
```

| Benchmark | Before ns/op (5 rounds) | After ns/op (5 rounds) | B/op before / after | allocs before / after |
|---|---|---|---|---|
| `BenchmarkServeMedia/get` | 88920 / 146742 / 101339 / 87771 / 99892 | 253169 / 261733 / 145251 / 187844 / 168756 | ~143.6 K / ~143.8 K | 63 / 75 |
| `BenchmarkServeMedia/revalidate_304` | 62859 / 41775 / 33599 / 39366 / 32668 | 191731 / 193701 / 136493 / 122961 / 107828 | ~9.5 K / ~9.3 K | 50 / 61 |

- **Allocations.** About +12 per request, and B/op is flat.
- **Latency.** On this overloaded host, GET is about 1.8x slower and a `304`
  is about 3.5x slower. Each extra syscall is a point where a loaded scheduler
  can preempt the goroutine, so these ratios overstate the cost on an idle
  server, where an `openat` or `close` takes well under a microsecond. That was
  not measured here.
- **Why it is accepted.** This is a security fix, and the cost is still small
  next to everything else in a `/media` request:
  - Requests normally come through the frontend proxy, an extra HTTP hop.
  - Upload keys are served `immutable`, so browsers and the image optimizer
    rarely come back for them.
  - A CDN in front, or `S3_PUBLIC_BASE_URL`, takes the backend off the path
    entirely.
- **Faster option, not built.** A Linux `openat2(RESOLVE_BENEATH)` fast path
  would make each walk a single kernel call again. It is in the backlog, for
  if `/media` latency ever matters.
- **Regression tests** (`internal/s3/store_local_symlink_test.go`). Against the
  old driver, all three fail.
  - `TestLocalStoreSymlinkEscapes`
  - `TestLocalStoreIgnoresSymlinkedSidecar`
  - `TestServeMediaRefusesSymlinkEscape`

## 3. Access shape of the upload paths

No database query shapes changed:

- Upload, delete and download handlers keep their existing single-row reads.
- The accounting-attachment delete adds one best-effort object delete after
  the row delete. That is one storage call and no query.

## Reruns

The ns/op columns are noisy on this host; compare B/op and allocs/op first.
To rerun:

```bash
cd backend
go test -run '^$' -bench 'BenchmarkRateLimiter$|BenchmarkRateLimiterMediaPath' -benchmem -benchtime=20000x -count=3 ./internal/middleware/
go test -run '^$' -bench BenchmarkServeMedia -benchmem -count=3 ./internal/s3/
go test -run '^$' -bench BenchmarkMediaRateLimit -benchmem -benchtime=200000x -count=5 ./internal/middleware/
```
