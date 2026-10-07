# Guest payments

Guests pay a bill by card through Stripe, PayPal or MercadoPago, or in USDC on
Base. Each restaurant connects its own provider account in the dashboard
(**Settings → Payments**); the money goes straight to that account, and
Payverge takes no fee. As the operator of the instance you only provide the
platform pieces: webhook secrets, the OAuth apps behind the "Connect" buttons,
and an RPC for USDC.

Nothing here is required. With no provider configured, guests can still ask
for the bill and pay at the table, and restaurants can record cash and card
payments by hand.

All variables are in [configuration.md](configuration.md#guest-payments).

## URLs the providers call

Replace `pay.example.com` with your `DOMAIN`. Every path below is public and
routed through Caddy to the backend.

| Provider | Path | Method | What it is |
|---|---|---|---|
| Stripe | `/api/v1/webhooks/stripe` | POST | Payment events for guest bills. |
| Stripe | `/api/v1/stripe/oauth/callback` | GET | Redirect URI of the Stripe Connect app. |
| PayPal | `/api/v1/webhooks/paypal` | POST | Payment events. |
| PayPal | `/api/v1/webhooks/paypal/return`, `/cancel` | GET | Where PayPal sends the guest back. Set by the backend per order; nothing to configure. |
| MercadoPago | `/api/v1/webhooks/mercadopago` | POST | Payment notifications. The backend also sets it as `notification_url` on every preference. |
| MercadoPago | `/api/v1/webhooks/mercadopago/return` | GET | Where MercadoPago sends the guest back. |
| MercadoPago | `/api/v1/mercadopago/oauth/callback` | GET | Redirect URI of the MercadoPago app. |
| Telegram | `/api/v1/webhooks/telegram` | POST | Not a payment route; listed because it shares the group. |

The webhook routes are registered even when no secret is set. When no secret is
configured the backend answers `503 Webhook configuration unavailable`, and a
bad signature gets `401`; either way the provider retries, and no payment is
marked paid without proof.

## Webhook secrets

For each webhook the backend collects a list of secrets and accepts the
request when the signature matches any of them:

1. **Per-restaurant secret.** When a restaurant pasted a webhook signing
   secret in its payment settings (`webhook_secret`, plus
   `webhook_secret_previous` during a rotation), only those are used.
2. **Instance secrets.** Otherwise the backend uses the environment, in this
   order, each followed by the same name with `_PREVIOUS` appended:
   - Stripe: `STRIPE_CONNECT_WEBHOOK_SECRET`, `STRIPE_PLUGIN_WEBHOOK_SECRET`,
     `STRIPE_WEBHOOK_SECRET`.
   - PayPal: `PAYPAL_WEBHOOK_SECRET`.
   - MercadoPago: `MERCADOPAGO_WEBHOOK_SECRET`,
     `MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS`.
3. **Stripe accounts connected by OAuth** use the instance secrets first and
   still accept a leftover per-restaurant secret, because Connect events are
   signed with the platform's endpoint secret.

To rotate an instance secret without dropping events, move the old value to
`NAME_PREVIOUS`, put the new one in `NAME`, run `docker compose up -d`, switch
the provider over, and remove `NAME_PREVIOUS` once the provider's retry window
has passed.

Source: `resolvePluginWebhookSignatures` in
`backend/internal/handlers/plugin_handlers.go`.

## Production activation

Card providers are fail-closed in production. In production mode (`--production`,
`ENV=production`, `ENV=prod` or `APP_ENV=production`, which the deploy stack
uses) Stripe and PayPal stay switched off until you set their switch to
`true` in `.env`:

```sh
PAYMENT_PROVIDER_STRIPE_ENABLED=true
PAYMENT_PROVIDER_PAYPAL_ENABLED=true
```

Turn a provider on only after you have tested it end to end in that
provider's own sandbox (Stripe test mode, PayPal sandbox) with your own test
merchant account: connect a restaurant, take a payment, refund it, and check
the webhook arrives. The tests in this repository do not replace that check.

How the switch behaves (source: `PaymentProviderStartsEnabled` in
`backend/internal/plugins/payment_contract.go`):

| Value | Production | Development |
|---|---|---|
| unset or empty | off | on |
| `true` (any case) | on | on |
| anything else, e.g. `false` | off | off (kill switch) |

While a provider is off, the boot-time registry sync marks it inactive in the
plugin catalog, so it is not offered to restaurants. Status reconciliation of
payments that had already started is exempt from the switch, so turning a
provider off does not hide money already in flight. A change takes effect on the next
backend restart (`docker compose up -d`). A platform admin who has
overridden a plugin's active flag in the admin dashboard keeps that override;
the boot-time sync does not change it.

`PAYMENT_PROVIDER_MERCADOPAGO_ENABLED` works differently: when it is unset
or empty, MercadoPago is active in every mode (it is the default card rail).
Set it to `false` (any value other than `true`) to turn MercadoPago off. Test
it in the MercadoPago sandbox (test users) before restaurants rely on it.

Cash and USDC are not behind these switches.

## Stripe

Two ways for a restaurant to connect:

- **Manual keys.** The restaurant pastes its own secret key and the signing
  secret of a webhook endpoint it created in its own Stripe dashboard,
  pointing at `https://pay.example.com/api/v1/webhooks/stripe`. No instance
  configuration is needed.
- **Connect (OAuth).** The "Connect Stripe" button needs a Stripe Connect
  platform account of yours:
  1. In Stripe, enable Connect (Standard accounts) and add
     `https://pay.example.com/api/v1/stripe/oauth/callback` as a redirect URI.
  2. Create a **Connect** webhook endpoint at
     `https://pay.example.com/api/v1/webhooks/stripe` ("Events on connected
     accounts").
  3. Set `STRIPE_SECRET_KEY`, `STRIPE_CONNECT_CLIENT_ID` (`ca_...`) and
     `STRIPE_CONNECT_WEBHOOK_SECRET` in `.env`, then `docker compose up -d`.

Subscribe the endpoint to at least `checkout.session.completed`,
`checkout.session.async_payment_succeeded`,
`checkout.session.async_payment_failed`, `checkout.session.expired`,
`payment_intent.succeeded`, `payment_intent.payment_failed`,
`charge.refunded`, `charge.dispute.created`, `charge.dispute.funds_withdrawn`,
`charge.dispute.funds_reinstated`, `charge.dispute.closed` and, for Connect, `account.application.deauthorized`.

## PayPal

Restaurants enter their PayPal REST app credentials in the dashboard. Point
the PayPal app's webhook at `https://pay.example.com/api/v1/webhooks/paypal`.
`PAYPAL_WEBHOOK_SECRET` is the instance fallback secret. There are no
instance-wide PayPal client credentials: each restaurant brings its own.

## MercadoPago

- **Manual.** The restaurant pastes its access token and the webhook secret
  from its MercadoPago application.
- **OAuth.** For the "Connect MercadoPago" button, create a MercadoPago
  application of yours with the redirect URI
  `https://pay.example.com/api/v1/mercadopago/oauth/callback` and the webhook
  URL `https://pay.example.com/api/v1/webhooks/mercadopago`. Set
  `MERCADOPAGO_CLIENT_ID`, `MERCADOPAGO_CLIENT_SECRET` and
  `MERCADOPAGO_WEBHOOK_SECRET`.

The platform fee (`marketplace_fee`) is always 0.

## USDC on Base

USDC payments need no provider account. The guest's wallet sends USDC to the
restaurant's address, and the backend checks the transfer on chain.

- `RPC_URL` is the backend's RPC. The public `https://mainnet.base.org` works
  for a trial; use a private RPC (Alchemy, QuickNode, your own node) in
  production, because public endpoints rate-limit.
- `PUBLIC_RPC_URL` is what browsers use. It is public by definition: never
  put a keyed RPC URL there.
- `NETWORK=baseSepolia` (frontend) with a Sepolia `RPC_URL` gives a test
  setup with test USDC.
- A transfer counts after `USDC_MIN_CONFIRMATIONS` blocks (3). A reorg
  reconciler rechecks paid transfers for `REORG_FINALITY_WINDOW_MIN` minutes.
- Refunds in USDC on mainnet stay off until you set
  `CRYPTO_REFUND_MAINNET_ENABLED=true`.

## Checking that it works

Not run for these docs (needs provider accounts and a public domain):

```sh
docker compose logs backend | grep -i webhook     # signature failures are logged
```

In the provider's dashboard, send a test event to the endpoint. A `2xx`
means the signature matched, `401` means the secret is wrong, and `503` means
no secret is configured.

Failed webhooks (every provider's) have no screen in the admin UI. Review
them through the API with a platform-admin token, or with the
`payverge_list_failed_webhooks` / `payverge_ack_failed_webhook` tools in
[tools/payverge-admin-mcp](../../tools/payverge-admin-mcp/README.md):

| Method and path | What it does |
|---|---|
| `GET /api/v1/admin/webhooks/failed` | lists failed events across providers (`?limit=1..200`, default 50) with provider, event type and error |
| `POST /api/v1/admin/webhooks/:id/acknowledge` | marks one failed event as ignored (optional JSON `{"reason": "..."}`); it is not retried |

There is no retry endpoint. Fix the cause (usually the secret), then resend
the event from the provider's dashboard.

### Refunds and disputes that arrive before the payment

Providers do not deliver events in order. A refund, a dispute or a dispute
reversal can reach Payverge before the event for the payment it applies to,
for example when a restaurant refunds from the Stripe dashboard while the
payment event is still being retried. Payverge does not drop such an event:

- When the event names one of the restaurant's bills and the payment is not
  on that bill yet, or it is a dispute for a payment Payverge has not
  recorded (Stripe and PayPal dispute events often name no bill), the backend answers `503 Capture not recorded yet; retry
  later` with `Retry-After: 300`. The provider redelivers it on its own
  schedule, and the first delivery after the payment is recorded is applied
  once. A `503` from this path in the provider's dashboard is expected, not a
  misconfiguration.
- While it waits, the event's `webhook_events` row has status
  `retry_pending`. It does not appear in `GET /api/v1/admin/webhooks/failed`,
  and it counts in `payverge_payment_webhook_processing_failures_total` with
  `reason="capture_pending"`, which the
  [alert rule](../runbooks/alerts.md#1-payment-webhook-failures) excludes.
- The wait is bounded at 48 hours, inside the providers' redelivery window
  of about three days. Age is measured from the provider's signed event
  timestamp (Stripe `created`, PayPal `create_time`) or the first time
  Payverge stored the event, whichever is older. MercadoPago's
  `date_created` is not covered by its signature, so MercadoPago events are
  aged from first-seen only. Past the bound the event is acknowledged with
  `200`, a warning is logged, the counter
  `payverge_payment_webhook_unsupported_actions_total{action="capture_pending.expired"}`
  is incremented and a payment-review alert is raised in the dashboard: on
  the bill when the event named one, otherwise on the restaurant (resource
  type `webhook_event`). Nothing was applied to the ledger: reconcile it by
  hand against the provider's dashboard.
- If the provider stops redelivering before the bound, a background sweeper
  (every 15 minutes) expires `retry_pending` rows older than 48 hours the
  same way, so the alert is raised even without another delivery.
- If Payverge itself refunded the payment because the bill could not take it
  (for example the bill was already settled), the provider's refund event is
  acknowledged straight away.
- MercadoPago notifications carry only an id; the backend fetches the
  payment's current state. A payment refunded before its approval was ever
  recorded is never reported as approved again, so such an event ends at the
  48-hour acknowledgement and alert.
- A refund that names no bill of the restaurant is acknowledged as before:
  Payverge's own charges always carry the bill reference, so it is a charge
  Payverge did not create.

To see events waiting for their payment:

```sql
SELECT id, provider, event_type, status, error, created_at, received_at
FROM webhook_events
WHERE status = 'retry_pending'
ORDER BY created_at;
```

Source: `deferPluginWebhookUntilCaptureRecorded` and
`SweepExpiredPluginCapturePendingWebhooks` in
`backend/internal/handlers/plugin_webhook_capture_pending.go`.

To be alerted when webhooks fail, see the
[alert runbook](../runbooks/alerts.md#1-payment-webhook-failures).
