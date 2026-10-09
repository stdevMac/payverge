# OSS C1 — Guest USDC payer binding: Perf Gate Evidence

> Point-in-time evidence. Migration numbers cited here (0002xx) predate the
> squash into `backend/schema/genesis/current_schema.sql`; that schema now
> lives in the genesis baseline, and numbered migrations restart at 000001.

Workstream C1 of the open-source release plan (§3.1, §5 C1) fixes S-Critical
USDC payment replay. Every guest USDC quote now reserves a unique exact amount in
`crypto_payment_quotes` (migration 000223). Settlement loads that row and consumes
it in the same transaction as the payment insert. This file records what that
adds to the two guest crypto routes.

Machine: Apple M3, darwin/arm64, Go toolchain from `backend/go.mod`. SQLite
in-memory microbenchmarks with a silent GORM logger.

- **Before:** `9041d27eb`, the persistence layer without the handler binding, so
  the routes behave as they did before binding. It was exported with
  `git archive` into a scratch directory and the new issuance benchmark copied
  in, so both sides ran on the same host in alternating rounds.
- **After:** branch `oss/c1` at the handler-binding commit plus this perf slice.

## What the binding adds to the database path

| Route | Added statements | Index used |
|---|---|---|
| `POST /guest/bill/:token/crypto-quote` (issuance) | 1 `UPDATE` that expires the wallet's lapsed active slots | partial unique `idx_crypto_payment_quotes_active_amount` (status literal, see below) |
| | 1 `COUNT` of the bill's active quotes (cap of 40), with this client's share folded into the same read (cap of 12) | `idx_crypto_payment_quotes_bill_status` |
| | 1 read of the taken amounts, `exact_microunits` only, `BETWEEN base+1 AND base+9999` | `idx_crypto_payment_quotes_wallet_amount` (range-bounded) |
| | 1 `INSERT` (retried up to 8 times only on a unique violation) | — |
| `POST /guest/bill/:token/crypto-payment` (settlement) | 1 primary-key read of the bound quote | PK |
| | 1 conditional consume `UPDATE` in the settlement transaction | PK |

Nothing here scales with quote history. The taken-amount read is bounded to a
window 9,999 amounts wide, and the bill count is bounded by the cap of 40.

The stale-slot `UPDATE` filters on `status = 'active'` as a SQL literal rather
than a bind parameter. That lets Postgres prove the partial unique index
predicate and scan only the wallet's live rows instead of every quote the wallet
has ever had.

`TestCryptoQuoteBindingAccessShape` (`backend/internal/handlers/crypto_quote_perf_test.go`)
pins these statement counts and shapes. It checks that issuance never runs a
`SELECT *`, that the taken-amount read stays bounded, that settlement loads the
quote by PK, and that settlement consumes it with a conditional UPDATE.

## Benchmarks

```
cd backend
go test ./internal/handlers/ -run '^$' \
  -bench 'BenchmarkProcessCryptoPaymentSQLite$|BenchmarkIssueCryptoQuoteSQLite$' \
  -benchmem -count=3
```

Both benchmarks are in `backend/internal/handlers/`:

- **`BenchmarkProcessCryptoPaymentSQLite`** (`crypto_payment_perf_test.go`) runs a
  full guest settlement.
  - **After:** each iteration first issues a fresh persisted quote outside the
    timer, because a quote settles once.
  - **Before:** it reused one unbound legacy token.
- **`BenchmarkIssueCryptoQuoteSQLite`** (`crypto_quote_perf_test.go`, new) runs
  quote issuance end to end.
  - Quote rows are cleared outside the timer, so it measures one live quote per
    wallet rather than an exhausted offset space.
  - The same file ran against the before tree, where no quote table exists.

The shared host was under heavy, varying load from other sessions during these
runs (load average 10–32), so **ns/op is noisy**.

- **B/op and allocs/op are deterministic.** They are the reliable signal.
- **ns/op:** use round 3 (load ~10) as the cleanest alternating pair. The quiet
  single-run baseline from before any change was made is listed for reference.

### `BenchmarkProcessCryptoPaymentSQLite`

| Run | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| Before, quiet baseline | 365,311 / 365,339 / 366,796 | 273,842–273,920 | 1908–1909 |
| Before, round 1 | 425,914 / 452,775 / 502,678 | 273,977–274,098 | 1909 |
| After, round 1 | 850,037 / 668,556 / 1,246,906 | 290,403–290,554 | 2129 |
| Before, round 2 | 1,788,718 / 905,458 / 810,628 | 274,111–274,642 | 1909–1910 |
| After, round 2 | 1,464,530 / 2,063,924 / 1,181,504 | 290,587–290,917 | 2127–2130 |
| Before, round 3 | 870,804 / 599,404 / 623,650 | 273,923–274,307 | 1909–1910 |
| **After, round 3 (final code)** | **428,618 / 568,503 / 470,690** | **290,154–290,211** | **2128–2129** |

**Delta: +16.3 KB/op (+5.9%) and +220 allocs/op (+11.5%).** These cover the
quote PK load and the consume UPDATE. ns/op stays inside host noise: round 3's
after is faster than its before, and rounds 1 and 2 go the other way.

### `BenchmarkIssueCryptoQuoteSQLite`

| Run | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| Before, quiet baseline | 72,735 / 69,756 / 73,561 | 67,457–67,460 | 579 |
| Before, round 1 | 109,460 / 106,504 / 116,259 | 67,413–67,420 | 578 |
| After, round 1 | 471,019 / 660,517 / 1,188,439 | 95,063–95,079 | 910 |
| Before, round 2 | 366,120 / 5,384,588 / 1,827,888 | 67,379–67,388 | 578 |
| After, round 2 | 631,587 / 354,383 / 272,795 | 95,057–95,085 | 910 |
| Before, round 3 | 103,732 / 111,227 / 85,174 | 67,399–67,406 | 578 |
| **After, round 3 (final code)** | **145,210 / 142,651 / 145,005** | **95,019–95,023** | **908** |

**Delta: +27.6 KB/op (+41%), +330 allocs/op (+57%), and about +40 µs/op
(median 103.7 µs → 145.0 µs, +40%) in round 3.** That cost is the four indexed
statements in the table above.

The literal-status change cut 2 allocs/op (910 → 908).

### Per-client quote cap (review c1 L3)

Before L3, anyone holding a bill link could take the bill's 40 live-quote slots
in seconds and lock every other payer out for a quote TTL. Each quote now
carries a `client_key`, an HMAC of the client IP keyed with the quote secret.
The IP itself is never stored, and the key is cleared on consume and on expiry.
One client may hold at most 12 live quotes per bill.

- **No new statement.** The per-client count is a `SUM(CASE ...)` folded into
  the existing bill `COUNT` and scanned with `Row().Scan` into two ints, so
  `TestCryptoQuoteBindingAccessShape` still sees exactly 2 SELECTs, 1 UPDATE
  and 1 INSERT on issuance.
- **Run:** `BenchmarkIssueCryptoQuoteSQLite`, `-benchmem -count=3`, alternating
  on the same host.
  - **Before:** committed `9901305bc`, exported with `git archive`.
  - **After:** the L3 working tree.
  - **Load:** 19–36, so ns/op is noise again.

| Run | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| Before, round 1 | 262,935 / 219,344 / 155,511 | 95,115–95,118 | 908 |
| After, round 1 | 312,556 / 276,737 / 331,945 | 97,638–97,662 | 937 |
| Before, round 2 | 219,398 / 197,488 / 222,317 | 95,119–95,123 | 908 |
| **After, round 2 (final code)** | **215,099 / 228,107 / 270,744** | **97,646–97,659** | **937** |

**Delta: +2.5 KB/op (+2.7%) and +29 allocs/op (+3.2%).** That covers the HMAC
of the client key, the extra INSERT column and the second scanned count.

A first version scanned the folded count into a struct and cost 955 allocs/op.
Switching to `Row().Scan` recovered 18 of them.

## Why this cost is accepted

The added cost is the binding itself, and every part of it is required:

- **The reservation** makes an amount identify one guest's quote.
- **The bill cap** limits offset-space exhaustion.
- **The stale-slot release** keeps the partial unique index from filling with
  expired-but-active rows.
- **The conditional consume** is the single-use guarantee under concurrency.

Issuance runs once per payment attempt. It is not a polling route. Settlement
already performs on-chain verification over RPC, which costs milliseconds and
dwarfs a PK read and a PK update.

Two cheaper variants were considered and left for later; both are in the C1
backlog:

- an `INSERT ... RETURNING` raw insert, which needs RETURNING support checked
  on both drivers;
- folding the stale-slot release into the unique-violation retry. This would
  leave lapsed rows `active` until someone collides with them. Every reader
  already checks `expires_at`, but the forensic queries would read less clearly.

## Concurrency evidence (Postgres)

`TestCryptoPaymentQuotes_Postgres` (`backend/internal/database/crypto_payment_quotes_pg_test.go`)
runs migration 000223 twice, to prove it is idempotent, on an isolated
Testcontainers Postgres. It then checks:

- the CHECK constraints;
- that `client_key` rejects a raw IP and accepts the 32-hex keyed hash;
- that 8 concurrent issuers for one wallet all get distinct exact amounts;
- that the per-client cap counts only that client's live quotes on real
  Postgres, where `SUM` of an integer CASE returns bigint;
- that two racing settlements consume one quote exactly once;
- that the down migration drops the table.

```
cd backend
go test -count=1 ./internal/database/ -run 'TestCryptoPaymentQuotes_Postgres' -v
--- PASS: TestCryptoPaymentQuotes_Postgres (1.75s)
```

## Regression tests run

```
go test -short -p 2 -count=1 ./internal/handlers/ -run 'Crypto|Quote|CrossChain|Payment|Replay|Binding|Plugin'
go test -short -p 2 -count=1 ./internal/handlers/      # only the known TestListTimesheetsPaginationBEFirst calendar failure
go test -short -count=1 ./internal/database/ -run 'CryptoPaymentQuote|ConsumesQuote|ExpiredQuote|QuoteBound|SubmitCryptoRefundTxHash'
```

## Review round 2: recorded-transfer lookup (R2-M1)

A guest who replays a transfer that already paid another bill used to get
`amount_mismatch`, because that transfer carries the other quote's exact amount.
Settlement now looks the transfer up in `payments` before it answers. If the
transfer is recorded, settlement returns 409 `crypto_tx_already_recorded` and
raises the `tx_hash_conflict` alert.

The lookup is `database.FindPaymentTxHashHolder`: one read of
`payments LEFT JOIN bills` on the unique `payments.tx_hash` index, `LIMIT 1`,
with four projected columns. It runs **only on refusal paths**:

- the amount-mismatch branch;
- the transfer-predates-quote branch;
- the settlement `ErrPaymentTxHashConflict` branch.

The happy path does not change. `TestCryptoQuoteBindingAccessShape` now asserts
that a successful settlement runs zero holder lookups, and that a replay
refused for amount mismatch runs exactly one.

`BenchmarkProcessCryptoPaymentSQLite` ran on the same host in alternating
rounds (load average about 5). **Before** is `169a1f87b`, the round-1 head:
the round-2 backend diff was reverse-applied, then re-applied byte for byte.
**After** is the round-2 working tree.

| Run | ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|
| After, round A | 749,474 / 1,146,760 / 447,418 | 290,497–291,223 | 2135–2136 |
| Before | 463,218 / 590,712 / 491,976 | 290,599–290,793 | 2135 |
| After, round B | 525,438 / 444,485 / 452,818 | 290,595–290,784 | 2134–2135 |

**Delta: none.** B/op and allocs/op match within run-to-run jitter, and ns/op
stays inside host noise.

The round-1 table above recorded 2128–2129 allocs/op for its final code. The
round-1 head now measures 2135. That difference predates this slice: the
before run, which contains none of this slice's code, already shows it.

```
cd backend
go test -short -run '^$' -bench 'BenchmarkProcessCryptoPaymentSQLite' -benchmem -count=3 ./internal/handlers/
go test -short -p 2 -count=1 ./internal/handlers/ -run 'Crypto|Quote|CrossChain|Payment|Replay|Binding|Conflict'
go test -short -count=1 ./internal/database/ -run 'TestFindPaymentTxHashHolder|CryptoPaymentQuote|ConsumesQuote'
```
