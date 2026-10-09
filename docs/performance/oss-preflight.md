# OSS preflight — performance evidence (2026-10-03)

Scope: the self-host production preflight change. Only one part of it
touches a request hot path: the guest crypto settlement chain gate that refuses
Base Sepolia (84532) settlement in production. Everything else in the change
(production preflight, secret denylist, plugin-key bootstrap, RPC default)
runs once at startup.

## Hot path

`backend/internal/handlers/payments.go` — `guestSettlementChainSupported`, called
from `currentGuestSettlementContract` on every guest crypto quote
(`IssueCryptoQuote`) and every crypto payment verification
(`ProcessCryptoPayment`, via the quote re-check), before any on-chain RPC call.

- Before: the inline expression
  `chainID == 8453 || (method == usdc_payment && chainID == 84532)`.
- After: a `switch` on the chain id. Mainnet stays a constant compare. Only the
  Sepolia branch calls `config.IsProductionMode(false)`, which reads an atomic
  plus the `ENV`/`APP_ENV` env vars.

Database access shape: unchanged. No queries were added, removed, or reordered.

## Benchmark

`BenchmarkGuestSettlementChainSupported`
(`backend/internal/handlers/guest_settlement_chain_bench_test.go`).

Command (hermetic test env prefix, Apple M3, go1.26.2):

```bash
cd backend
go test -short -p 2 -count=3 -run '^$' -bench 'BenchmarkGuestSettlementChainSupported' -benchmem ./internal/handlers
```

The baseline was captured after extracting the predicate with its old
semantics, then the production gate was added and the benchmark rerun.

| Case | Before ns/op (3 runs) | After ns/op (3 runs) | B/op | allocs/op |
|---|---|---|---|---|
| mainnet_usdc | 0.341 / 0.342 / 0.336 | 0.409 / 0.373 / 0.374 | 0 → 0 | 0 → 0 |
| mainnet_cross_chain | 0.350 / 0.366 / 0.371 | 0.365 / 0.397 / 0.380 | 0 → 0 | 0 → 0 |
| sepolia_usdc | 0.907 / 0.907 / 1.000 | 33.84 / 33.84 / 33.71 | 0 → 0 | 0 → 0 |
| unsupported_chain | 0.938 / 0.970 / 0.880 | 0.862 / 0.841 / 0.844 | 0 → 0 | 0 → 0 |

How to read it:

- The production path is Base mainnet. It does no extra work, and its sub-ns
  numbers are within run-to-run noise.
- The Sepolia path grows by about 33 ns because of the env lookups. It runs
  only on dev/test instances, and only before an RPC round trip that costs
  milliseconds.
- Nothing allocates.

## Regression tests

Run with the hermetic test env prefix:

```bash
go test -short -p 2 -count=1 ./internal/handlers \
  -run 'GuestSettlementChain|SepoliaIsDevelopmentOnly|RejectsSepoliaTransfer|CryptoQuote|CryptoPayment|SettlementQuote'
go test -short -p 2 -count=1 ./internal/handlers   # whole package once
```

- `TestGuestSettlementChainSupported`: a production/development × chain × method table.
- `TestIssueCryptoQuoteSepoliaIsDevelopmentOnly`: in production a Sepolia quote
  returns 422 `plugin_unavailable`. In development it still returns 200 with
  `chain_id` 84532.
- `TestProcessCryptoPaymentRejectsSepoliaTransferInProduction`: a signed
  Sepolia quote presented in production returns 422 before on-chain
  verification. Before the fix this returned 200.
- Whole package: the only failure is the known pre-existing
  `TestListTimesheetsPaginationBEFirst`.
