# 0005. Payment providers as compiled-in plugins behind one contract

- Status: Accepted
- Date: 2026-10-03 (records a decision already in effect)

## Context

Restaurants in different countries use different payment providers (Stripe,
PayPal, Mercado Pago), and each business brings its own account. Every
provider has its own API, webhook signature scheme, currency rules and
refund lifecycle. The rules for applying money to a bill (locking, the
amount still owed, idempotency, the fiscal receipt) must stay the same for
all of them.

## Decision

Each provider is a Go package that implements the `PaymentPlugin` interface
in [plugins/interface.go](../../backend/internal/plugins/interface.go), plus
optional capability interfaces it can opt into. Packages register through
`RegisterPluginInitializer` in `init()`, are compiled into the backend, and
are synced into a `plugins` catalog table at startup. A business enables a
plugin with its own configuration, whose credentials are encrypted at rest.

Plugins only talk to the provider. **Settlement is shared:** one webhook
handler verifies the signature, claims the event for idempotency and applies
the result to the bill.

A manifest,
[payment_contract.go](../../backend/internal/plugins/payment_contract.go),
lists the guarantees each production provider must meet, and a shared test
runner checks every provider against the same cases. Each provider has an
activation switch, off by default in production.

## Consequences

- **Easier:** a provider cannot skip signature checks, idempotency or the
  amount-owed check, because it does not implement them.
- **Easier:** a misbehaving provider can be switched off without a deploy,
  while payments already in flight keep reconciling.
- **Harder:** a new provider means a backend build and edits to shared code
  (webhook routes, dispatcher, manifest). There is no runtime loading of
  third-party plugins.
- **Accepted:** the interface is wide, and some methods do not apply to every
  provider.

See [architecture/plugins.md](../architecture/plugins.md).
