# Money flow

This page covers how Payverge stores amounts, how a bill is totalled and
split, and how a payment becomes "paid" exactly once. The design decision
behind the storage format is [ADR 0003](../adr/0003-money-as-int64-cents.md).

```mermaid
sequenceDiagram
  participant G as Guest
  participant A as Backend
  participant P as Payment provider
  participant D as Postgres
  G->>A: start checkout for a bill or a split share
  A->>P: create payment (amount converted to the currency's minor unit)
  P-->>G: hosted checkout
  P->>A: signed webhook
  A->>A: verify signature, then claim the webhook event row
  A->>D: BEGIN; SELECT bill FOR UPDATE
  A->>D: insert payment (tx_hash unique), update paid amount and status
  A->>D: milestones, fiscal outbox job (same transaction); COMMIT
  A-->>G: realtime "bill.updated" and "payment.received"
```

## Storage: integer cents

Every settled amount is an `int64` holding major units times 100, whatever
the currency. On a `Bill` that is `Subtotal`, `TaxAmount`,
`ServiceFeeAmount`, `TotalAmount`, `PaidAmount`, `TipAmount` and
`LoyaltyDiscountCents` ([models.go](../../backend/internal/database/models.go),
search `type Bill struct`). Payments, alternative payments, split shares,
ledger entries and payroll use the same unit.

**Prices are the exception.** Menu item prices and `BillItem.Price` are
`float64` major units, because an operator types them that way. They are
converted to cents with `CentsFromMajor` the moment they enter a total.

**On the wire, amounts are dollars.** The JSON encoders in
[models_json.go](../../backend/internal/database/models_json.go) turn cents
into a decimal number (`centsToDollars`) for `Bill`, `Payment`, `Order`,
`AlternativePayment`, `PaymentBreakdown`, delivery orders, ledger entries,
payroll and others. So `"total_amount": 18.64` in a response is `1864` in the
database. The frontend sends dollars too, and the backend parses them with
`ParseDollarStringToCents` or `Float64ToCents`
([convert.go](../../backend/internal/money/convert.go)). Printer payloads use
the same dollar JSON and must not divide again.

## Totals

[bill_totals.go](../../backend/internal/money/bill_totals.go) is the only
place that computes a bill's totals:

- `LineSubtotalCents` prices one line: unit price plus option surcharges,
  times quantity. Discount lines carry their own (negative) subtotal, and the
  children of a bundle count as zero because the price sits on the bundle.
- `PercentageCents` applies a tax or service-fee rate. The rate is kept to a
  thousandth of a percent, so 8.875% stays 8.875%, and the result is rounded
  half-up once, on the final cent.
- `BillTotalsCents(subtotal, taxRate, serviceFeeRate)` returns tax, fee and
  gross total.

## Splitting

[splitting/service.go](../../backend/internal/splitting/service.go)
divides a bill without losing or inventing a cent:

- **Equal split** (`allocateCentsEqual`): everyone but the last person gets
  the rounded-down share, and the last person gets what is left.
- **By item or by amount** (`allocateProportionalCents`): base, tax and
  service fee are each allocated in proportion to what each person ordered,
  with the same last-person remainder, so each of the three sums exactly to
  the bill's figure.

When guests pay their own share, the share is first **held**
([bill_split.go](../../backend/internal/database/bill_split.go)): a
`bill_split_shares` row in state `held` reserves that amount for five minutes,
so two guests cannot pay the same cents. A held share becomes `settled` when
its payment confirms, or `released` or `failed`. A ticker in `main.go`
releases expired holds every minute and pushes the new split state to every
phone at the table.

## Settling a provider payment

Card and wallet payments go through a payment plugin (Stripe, PayPal or
Mercado Pago; see [plugins.md](plugins.md)). The guest pays on the provider's
hosted page. The money is recorded only when the provider's **webhook**
arrives, never when the browser returns.

`handlePaymentWebhook` in
[plugin_handlers.go](../../backend/internal/handlers/plugin_handlers.go):

1. **Resolves the business.** For Stripe Connect it trusts the connected
   account on the event, and refuses an event whose metadata names a
   different business (`403`) or whose account maps to several (`409`).
2. **Verifies the signature** with the provider's webhook secret, before
   writing anything. A missing secret is a `503`, so the provider retries.
3. **Claims the event** in a webhook idempotency table
   ([webhook_events.go](../../backend/internal/database/webhook_events.go)),
   keyed by provider and event ID. A redelivery of a processed event is
   acknowledged and ignored; a redelivery of a failed one is processed again,
   by one worker at a time.
4. **Dispatches on status:**
   - **Paid**: `updateBillPaymentStatus` checks the currency and the
     bill/tip breakdown, then calls `database.ApplyConfirmedPayment`.
   - **Refunded**, fully or partly: `ReversePluginPayment` or
     `PartialReversePluginPayment`
     ([payment_reversal.go](../../backend/internal/database/payment_reversal.go)).
     Both are idempotent and skip a payment the operator is already refunding
     from the dashboard, so the books are never reduced twice.
   - **Disputed**: an operator alert only. The money stays settled until the
     provider decides.

`ApplyConfirmedPayment`
([business.go](../../backend/internal/database/business.go)) does all of the
following in one transaction:

- locks the bill row (`SELECT … FOR UPDATE`);
- refuses a payment larger than what is still owed, or on a bill that is no
  longer payable;
- inserts the payment, whose provider transaction ID is unique, so a second
  delivery of the same capture is a no-op (`applied=false`);
- updates `PaidAmount`, `TipAmount` and the bill status;
- records revenue milestones;
- enqueues the fiscal-receipt job through an injected hook (a transactional
  outbox), so a bill can never be paid without its receipt job.

**A capture that cannot settle is refunded.** If the currency does not match
the business, the share was already paid, the breakdown does not add up, or
the bill is closed, the guest has been charged for something the bill cannot
absorb. `autoRefundUnsettleablePluginCapture` refunds it through the provider
and raises an operator alert. If that refund fails, the webhook answers
non-2xx so the provider retries, and a louder alert asks for manual action.

### Minor units at the provider boundary

Stored cents are major times 100 for every currency, but providers expect the
currency's real minor unit.
[minorunits.go](../../backend/internal/money/minorunits.go) converts on the
way out (`MinorUnits`) and back (`FromMinorUnits`). Zero-decimal currencies
(JPY, KRW, CLP, VND and others) divide by 100; three-decimal currencies (BHD,
KWD, JOD, OMR, TND and others) multiply by 10. Without it, Stripe would read
¥1,000 (stored as `100000`) as ¥100,000.

## Cash and manual payments

Cash, the restaurant's own card terminal, wallet apps and other
off-platform payments are `AlternativePayment` rows. A guest can ask to pay that way (a pending request
that holds the split share), and staff confirm it.
`CreateConfirmedAlternativePayment` settles it through the same locked
transaction and fiscal hook as a provider payment. A client-supplied
idempotency key, stored as a hash with a unique index per bill, makes a
double-tapped "confirm" button safe.

## Crypto (USDC on Base)

Off per business until its owner turns it on; there is no instance-wide
switch. Payverge never holds a private key.

- **The RPC is always there.** `RPC_URL` defaults to the public Base mainnet
  endpoint, so every instance can verify USDC transfers. In production,
  startup refuses an RPC that serves any chain other than Base mainnet
  (chain id 8453). If the RPC cannot be reached at startup, crypto
  settlement is unavailable and startup only warns.
- **A business opts in.** Guest crypto routes answer `422 plugin_unavailable`
  until the owner enables the `usdc_payment` plugin and sets a settlement
  wallet ([payments.go](../../backend/internal/handlers/payments.go),
  `requireGuestCryptoPlugin`).

- **Quote, then pay.** The guest asks for a quote
  (`POST /guest/bill/:bill_token/crypto-quote`), sends USDC from their own
  wallet to the business's settlement address, and submits the transaction
  hash (`…/crypto-payment`,
  [payments.go](../../backend/internal/handlers/payments.go),
  `ProcessCryptoPayment`).
- **Verification.**
  [blockchain/service.go](../../backend/internal/blockchain/service.go)
  reads the receipt and requires a successful USDC `Transfer` to the expected
  address for the exact amount in micro-USDC, mined after the quote was
  issued, with at least three confirmations (`USDC_MIN_CONFIRMATIONS`). It
  then settles through `ApplyConfirmedPayment`, with the transaction hash as
  the idempotency key.
- **Reorg watch.** [reorgwatch](../../backend/internal/services/reorgwatch/reconciler.go)
  re-reads recently confirmed transfers against the canonical chain
  (`REORG_FINALITY_WINDOW_MIN`, 30 minutes by default) and raises a
  high-priority alert if one disappears. It does not reverse the ledger on
  its own.
- **Refunds.** An operator creates and approves a refund in the dashboard,
  signs and sends it from their own wallet, and submits the hash. The
  [cryptorefund](../../backend/internal/services/cryptorefund/worker.go)
  worker verifies the outbound transfer and applies the refund and the fiscal
  credit note exactly once. Mainnet refund submission is off unless
  `CRYPTO_REFUND_MAINNET_ENABLED` is set.

## Known limitations

- **Prices are floats.** Menu and line prices are `float64` until they reach
  a total. Rounding is controlled at that point, but a price such as `0.1 +
  0.2` is still a float until then.
- **One currency per business.** A bill takes its currency from its business.
  There is no multi-currency bill and no FX conversion.
- **One chain, one token.** Crypto settlement is USDC on Base.
- **Crypto cannot be switched off for the whole instance.** Leaving the
  `usdc_payment` plugin off for every business is the only way to keep it
  unused. `GET /api/v1/instance` reports `features.crypto: true` whenever an
  RPC URL is set, which the default makes always.
- **Reorgs alert, they do not unwind.** A reorged payment needs an operator
  decision.
- **Provider webhooks are the source of truth.** If a provider cannot reach
  `${PUBLIC_URL}/api/v1/webhooks/...`, payments stay pending until its retry
  succeeds or the reconciliation scheduler polls the provider.
