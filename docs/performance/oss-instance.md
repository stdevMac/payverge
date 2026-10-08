# OSS instance workstream — performance evidence (2026-10-03)

Scope: the open-source "instance" slice (plan items A1.2, A1.9–A1.11). The
only new hot path is the public `GET /api/v1/instance` probe, which the
frontend reads on SSR and client boot. Other changes in this slice are
startup-only (env resolution, LLM provider wiring) or replace string
constants with cached-env accessors.

## `GET /api/v1/instance`

Design: the handler builds the payload from env + boot-time runtime facts
(`server.SetInstanceRuntime`), encodes it once, and serves the encoded bytes
from an `atomic.Pointer` cache for 30 s (`instanceInfoServerTTL`). Clients and
CDNs get `Cache-Control: public, max-age=60`. No database access, no locks.

Benchmark: `BenchmarkInstanceInfoHandler` in
`backend/internal/server/instance_handler_test.go` (gin test router +
`httptest` recorder, so the numbers include router and recorder overhead).

```
cd backend
env -u PUBLIC_URL -u FRONTEND_URL -u BASE_URL -u APP_BASE_URL -u OPENROUTER_API_KEY -u BILLING_MODE \
  PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000 GO_ENV=test \
  go test -short -count=3 -run '^$' -bench BenchmarkInstanceInfoHandler -benchmem ./internal/server
```

Apple M3, Go 1.26, `-count=3`:

| Sub-benchmark | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| `cached` (steady state) | 508.7 / 533.4 / 520.2 | 1472 | 11 |
| `uncached` (cache dropped every iteration) | 1733 / 1752 / 1738 | 2426 | 18 |

Caching the payload removes about 1.2 µs, 950 B and 7 allocations from each
request. Both paths are DB-free, so the route adds no query load at any
request rate. There is no "before" baseline because the route is new.

Integration rerun (2026-10-03, after the payload switched from local env
helpers to the shared `config.RegistrationMode`, `config.EmailProvider`,
`config.EmailVerificationModeFor`, `config.DemoDataEnabled` and
`services.WhatsAppAvailable` accessors). Same command, plus
`-u NEXT_PUBLIC_BASE_URL`; Apple M3, idle machine:

| Sub-benchmark | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| `cached` | 514.2 / 511.7 / 516.9 | 1472 | 11 |
| `uncached` | 1829 / 1846 / 1829 | 2378 | 17 |

The cached path is unchanged. The uncached path, which runs at most once per
`instanceInfoServerTTL`, saves 48 B and 1 allocation, at about 90 ns more per
build (the stranded-credential check behind `EMAIL_VERIFICATION=auto`).

Regression coverage (same file):
- `TestInstanceInfoDefaults`
- `TestInstanceInfoConfigured`
- `TestInstanceInfoEnvFallbacksWithoutRuntime`
- `TestInstanceInfoNeverExposesSecrets`
- `TestInstanceInfoServerCacheExpires`, which checks that the TTL is honoured
  and that `SetInstanceRuntime` invalidates the cache.

## Accessors that replaced constants

`config.PublicURL()`, `config.PublicHost()`, `config.ProductName()` and the
other brand accessors read the environment on every call; they do not cache.
Their callers are a few low-frequency paths:

- email composition
- marketing link building
- crypto-refund binding signatures
- startup

None of them is on a per-request guest or dashboard polling path, so no
benchmark was needed.

## Guest order `X-Request-Id` validation (A1.11)

`POST /api/v1/guest/table/{code}/order` now rejects an over-long
`X-Request-Id` in the handler (`guestOrderRequestID`), before checkout, with a
dedicated error. `GuestCheckoutService.Checkout` keeps its own check, which
already ran before any checkout query, so a rejected id does the same work as
before. The change adds no queries, preloads or allocations on the success
path: it is one `strings.TrimSpace` plus a length comparison on a header the
handler already read. No benchmark was needed. Coverage is in
`TestCreateGuestOrder_RequestIDContract`, which also checks that rejected ids
create no order rows and that a 64-byte id replays on retry.

## Payer-facing labels follow `PRODUCT_NAME`

The Mercado Pago bill description (QR and Point), the Mercado Pago POS name
and nameless-store fallback, and the PayPal `brand_name` now read
`config.ProductName()` instead of a `"Payverge"` literal. Each is built once
per outbound provider call, next to an HTTPS round trip to the provider: one
`os.Getenv`, a rune-capped trim and a `fmt.Sprintf`. There are no new queries,
preloads or JSON reads, so no benchmark was needed.

Coverage:

- `TestCreateBillPaymentBrandNameFollowsProductName`
- `TestEnsureStoreAndPOS_*`
- `TestChargeMercadoPagoQR_ProvisionsCreatesOrderAndTracker`
