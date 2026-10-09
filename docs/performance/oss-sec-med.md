# oss/sec-med performance evidence (2026-10-03)

> Point-in-time evidence. Migration numbers cited here (0002xx) predate the
> squash into `backend/schema/genesis/current_schema.sql`; that schema now
> lives in the genesis baseline, and numbered migrations restart at 000001.

Benchmark log for the open-source-release security workstream "sec-med"
(plan §5 S-Medium + selected S-Low). Every change below touches a hot path
named by the Backend Performance Gate (many-route middleware, public guest
routes), so each one records a before/after run.

Environment: Apple M3, darwin/arm64, in-memory SQLite fixtures, hermetic test
env (`PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000 GO_ENV=test`, URL and
provider env vars unset). Other workstreams were building on the same machine,
so ns/op carries noise; B/op and allocs/op are the stable signal.

## M-role: HybridAuthenticationMiddleware live admin role

`BenchmarkHybridAuthUserToken` (`backend/internal/server/hybrid_live_role_test.go`)
drives `HybridAuthenticationMiddleware` with a bearer user token backed by a
live session row (session validate + get + operator-verification reads).

```
go test -short -count=3 -run '^$' -bench 'BenchmarkHybridAuthUserToken' -benchmem ./internal/server/
```

| Case | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs | After allocs |
|---|---|---|---|---|---|---|
| role=user  | 65049 / 64477 / 66148 | 64858 / 64301 / 64188 | ~42.3k | ~42.3k | 563 | 563 |
| role=admin | 66792 / 65304 / 65038 | 71044 / 70939 / 71078 | ~42.3k | ~47.2k | 563 | 639 |

Non-admin tokens (the overwhelming majority of `/inside` traffic) pay nothing:
a non-admin claim grants no platform power, so it is passed through without a
read. Admin-claim tokens pay one narrow `SELECT id, role, address FROM users`
by primary key (about +5 us, +76 allocs), the same shape
`AuthenticationAdminMiddleware` already uses.

## IPv6 /64 limiter keying + M-track per-client fallback

`BenchmarkIPKeyedRateLimit` (`backend/internal/middleware/client_key_test.go`)
drives `RateLimit` and `AuthRateLimiter` on the allow path. The change routes
every IP-keyed limiter (`RateLimit`, `AuthRateLimiter`,
`BusinessRateLimitWithBurst`, `PaymentRateLimit` IP fallback,
`ManagerActionRateLimit` IP fallback, `SimpleRateLimiter`) through
`middleware.ClientRateLimitKey`, which buckets IPv6 clients per /64.

```
go test -short -count=3 -run '^$' -bench 'BenchmarkIPKeyedRateLimit' -benchmem ./internal/middleware/
```

| Case | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs | After allocs |
|---|---|---|---|---|---|---|
| RateLimit/ipv4       | 309.6 / 310.3 / 315.7 | 315.2 / 325.1 / 320.9 | 224  | 224  | 5  | 5  |
| RateLimit/ipv6       | 327.0 / 363.2 / 334.6 | 464.1 / 527.4 / 556.5 | 224  | 264  | 5  | 7  |
| AuthRateLimiter/ipv4 | 636.6 / 634.9 / 753.3 | 837.8 / 703.5 / 765.9 | 1048 | 1048 | 11 | 11 |

IPv4 keys are used verbatim (no parse, no allocation), so the common path is
unchanged within noise. IPv6 pays one `netip` parse plus the /64 prefix string
(about +150 ns, +2 allocs) per request, which is what closes the
rotate-through-the-/64 bypass. A first draft that parsed every address added
one allocation to the IPv4 path too (240 B / 6 allocs); the verbatim IPv4 fast
path removed it.

The delivery tracking route gains a dedicated `AuthRateLimiter(120, 20)`
(one token-bucket lookup, same cost as the AuthRateLimiter row above) and
`TrackDelivery` only adds `public_token` to the bill summary when the order is
awaiting payment; its query shape is unchanged.

## M-sse: separate guest SSE pool

`Hub.SubscribeGuestLimited` counts public guest streams in their own
per-business / per-IP counters (`SSE_MAX_GUEST_SUBSCRIBERS_PER_BUSINESS`,
default 150; `SSE_MAX_GUEST_SUBSCRIBERS_PER_IP`, default 10), so guests can no
longer exhaust the authenticated pool (`SSE_MAX_SUBSCRIBERS_PER_BUSINESS` 200,
`SSE_MAX_SUBSCRIBERS_PER_IP` 30). The change is subscribe-time only: the same
single-lock check-and-register runs against a different pair of maps, and
`Publish` (the fan-out hot path) is untouched, so no benchmark delta applies.
Both SSE handlers now pass the /64-bucketed client key to the per-IP caps.

## M-pin: keyed demo staff PINs

Demo staff PINs are now HMAC-SHA256(HMAC(PLUGIN_SECRET_KEY, label), email)
instead of sha256(constant + email); the derivation cost is unchanged (one
HMAC per staff member). The Demo Center summary additionally verifies each
re-derived hint against the stored bcrypt hash before showing it, so a hint
never survives a key change.

```
go test -short -count=3 -run '^$' -bench 'BenchmarkDemoPINMatchesHash' -benchmem ./internal/demo/
```

| Case | ns/op | B/op | allocs/op |
|---|---|---|---|
| DemoPINMatchesHash (bcrypt.MinCost) | 921379 / 901454 / 1002858 | ~5.2k | 12 |

About 0.9 ms per seeded staff member, so roughly 5 ms per demo business on
the admin-only, non-polled `SummaryForAdmin` read. No guest or operator hot
path is affected.

## M-siwe: SIWE message validation and nonce-keyed challenges

Wallet sign-in (`POST /auth/challenge`, `/auth/signin`, `/auth/wallet/link`)
is behind `authLimiter` (10/min per client) and is not one of the gate's hot
paths. The added work per sign-in is one line scan of a ~10-line message, two
`url.Parse` calls and three RFC 3339 parses, next to an ECDSA public-key
recovery that dominates the request. The challenge store keeps the same
single-mutex map; `Issue` only sweeps expired entries when the 100k cap is
reached. No benchmark delta is recorded.

## Low: CSRF guard for cookie-authenticated mutations without Origin

`BenchmarkRequireTrustedOriginForMutations`
(`backend/internal/middleware/security_test.go`) drives
`RequireTrustedOriginForMutations`, which sits on every authenticated route
group, on the browser path (trusted Origin) and both no-Origin paths.

```
go test -short -count=3 -run '^$' -bench 'BenchmarkRequireTrustedOriginForMutations' -benchmem ./internal/middleware/
```

| Case | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs | After allocs |
|---|---|---|---|---|---|---|
| trusted_origin      | 132.3 / 128.2 / 132.0 | 214.6 / 268.8 / 362.5 | 208 | 208 | 4 | 4 |
| no_origin_bearer    | 167.7 / 152.9 / 144.0 | 436.5 / 378.9 / 287.2 | 208 | 208 | 4 | 4 |
| no_origin_no_cookie | 132.6 / 128.8 / 131.1 | 200.2 / 174.7 / 199.5 | 208 | 208 | 4 | 4 |

The after run overlapped heavy builds from sibling workstreams (the
trusted-Origin path executes byte-identical code before and after, yet moved
by +80..230 ns), so ns/op here is load, not the change. B/op and allocs/op
are identical on every path. The browser path never reaches the new code; the
no-Origin path checks the raw `Cookie` header first, then the bearer header,
and only parses cookies when both are present-but-ambiguous. A first draft
that parsed cookies before checking for a bearer header cost the
bearer-plus-cookie path +200 B / +2 allocs; the reordering removed it.

## Low: route limiters on the remaining public reads

`GET /reservations/:confirmationCode` now shares `publicReservationLimiter`
(10/min per client, burst 5) with the create/cancel routes,
`POST /staff/accept-invitation` shares `staffAuthLimiter` with the login-code
routes, and `GET /guest/table/:code/menu-translations` plus
`GET /businesses/:business_id/payment-plugins` use a new per-client
`SimpleRateLimiter` with the guest table-read budget (240/min,
`GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE`). Each route gains one
in-memory bucket lookup on the allow path, the same code measured by
`BenchmarkIPKeyedRateLimit` above (about 0.3 us, 224 B, 5 allocs for
`RateLimit/ipv4`; about 0.7 us for `AuthRateLimiter/ipv4`). Handler query
shapes are unchanged, so no separate before/after run is recorded.

`GET /referrals/stats` moved from the public group to `/api/v1/admin`; its
aggregate query is unchanged.

## Fix round 1: stateless wallet challenges and the legacy demo PIN sweep

Wallet sign-in challenges (`internal/logic/challenge.go`) no longer live in a
capped map. A nonce is `hex(expiry || random || HMAC)`, issuing stores nothing,
and a nonce is recorded only once its signature has verified, until it
expires. Memory now grows with successful sign-ins (behind the auth limiter)
instead of with `/auth/challenge` calls, so the 100k cap that let anyone lock
out wallet sign-in is gone. One sign-in's challenge work (issue, verify,
redeem):

```
go test -short -count=3 -run '^$' -bench BenchmarkChallengeIssueVerifyRedeem -benchmem ./internal/logic/
```

| run | ns/op | B/op | allocs/op |
|---|---|---|---|
| 1 | 1498 | 1717 | 26 |
| 2 | 1465 | 1738 | 26 |
| 3 | 2465 | 1709 | 26 |

No before figure: the old store was a map insert plus a delete, and the route
is a once-per-login auth call, not a hot path. Two HMAC-SHA256 calls and a hex
round trip are about 1.5 us, against a wallet signature check that costs much
more.

`demo.RotateLegacyDemoStaffPINs` (a boot-time re-key of demo staff seeded
with the pre-M-pin derivation) was removed before the open-source release: no
instance ever held such rows. `BenchmarkDemoPINMatchesHash` still covers the
Demo Center hint check.

## Fix round 2: M-545 guest fiscal identity bound to the paying session

`POST /guest/bill/:bill_token/fiscal-customer` now records the caller's guest
session fingerprint as the setter (migration 000226), lets a different guest
session replace a guest-set identity only with strictly stronger payment
proof, and the fiscal issue worker drops a guest-set identity whose setter
paid nothing on a bill other guest sessions paid.

Guest POST, before = HEAD 53a88cbcb (first-writer-wins), after = this change.
The before run used the same benchmark body without the session cookie and
with no payer stamp, run from a `git archive` copy of HEAD back to back with
the after run:

```
go test -short -count=3 -run '^$' -bench BenchmarkGuestFiscalCustomerPost -benchmem ./internal/server/
```

| Case | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs | After allocs |
|---|---|---|---|---|---|---|
| first_write | 182288 / 181603 / 183720 | 187716 / 198524 / 192598 | ~106.4k | ~110.2k | 1212 | 1253 |
| override_rejected_equal_proof | 155968 / 169304 / 161087 | 219860 / 220526 / 220865 | ~85.7k | ~110.4k | 1069 | 1268 |

The common first write runs the same number of queries as before: the setter
column rides in the existing conditional `UPDATE ... WHERE all five fiscal
columns IS NULL`. The +41 allocs are the cookie HMAC check and fingerprint.
Only a write from a different session against a guest-set identity reads
payment proof: two `GuestPaymentProofForSession` queries (caller and setter),
one round trip each. That path is a dispute, not the dinner-rush path.

Payment initiation (`RequestAlternativePayment`, guest crypto and cross-chain
settlement, plugin checkout trackers, split-share settlement) adds no query:
`payer_guest_session` is one more column in the existing INSERT.

The proof read and the issuance guard are new code with no before figure:

```
go test -short -count=3 -run '^$' -bench 'BenchmarkClearUnpaidGuestFiscalIdentity|BenchmarkGuestPaymentProofForSession' -benchmem ./internal/database/
```

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| ClearUnpaidGuestFiscalIdentity (one conditional UPDATE) | 70891 / 73507 / 72218 | ~24.6k | 119 |
| GuestPaymentProofForSession (one SELECT CASE) | 44450 / 38444 / 34643 | ~10.4k | 80 |

The issuance guard runs once per issue job and only when the bill has a
guest-set identity (`fiscal_customer_guest_session IS NOT NULL`); other jobs
skip it without a query.

Postgres plan check (local PostgreSQL 14, 200k payments and 100k alternative
payments, `ANALYZE`; Docker was unresponsive, so no Testcontainers run): every
branch of both statements is an index probe on
`idx_payments_bill_payer_guest_session`,
`idx_alternative_payments_bill_payer_guest_session`, `idx_payments_tx_hash` or
`bills_pkey`, with no sequential scan. A first draft nested the plugin-tracker
EXISTS under an OR; Postgres planned it as a hashed subplan over every
confirmed payment (`Seq Scan on payments`). Rewriting that branch as a join
made it a nested-loop probe of the unique `tx_hash` index. On SQLite the
rewrite costs about +10 us and +5 allocs per proof read (24.2 us / 75 allocs
before).
