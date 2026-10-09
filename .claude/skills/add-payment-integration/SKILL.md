---
name: add-payment-integration
description: Add a new payment provider to Payverge as a code plugin, covering the Go plugin and its registration, webhook signature verification, the provider allow-lists in the payment handlers, the production activation contract, the dashboard config form with its contract tests, and sandbox verification. Use when someone wants guests to pay through a provider Payverge does not support yet, such as a local card processor or wallet.
---

# Add a payment integration

Payment plugins move real money, so this is a code change in a fork or an
upstream contribution, made on a branch. The canonical checklist is
section 12, "Plugin Authoring Checklist", in `AGENTS.md`. This skill follows
it and adds the places the checklist does not name: the provider allow-lists
in the payment handlers and the production activation contract.

Use `backend/internal/plugins/paypal/` as the reference implementation. It is
a redirect checkout with a server-side capture. `backend/internal/plugins/stripe/`
and `backend/internal/plugins/mercadopago/` show OAuth onboarding and QR or
terminal flows.

## 0. Before writing code

First check whether a new plugin is needed at all. If Stripe, PayPal or
Mercado Pago already serves the operator's country, connecting it is a
dashboard task, not code (see the payments step of
`.claude/skills/configure-restaurant/SKILL.md`).

Then ask the operator:

1. **The plugin name**: a lowercase slug, written `<name>` below. It becomes
   the Go package, the catalog name, the webhook path
   `/api/v1/webhooks/<name>`, the icon file and the frontend constant.
   Renaming it later touches every step below.
2. **The checkout flow**: a redirect to a hosted page, a QR code, or a card
   terminal. The flow decides which optional interfaces you implement.
3. **How the provider signs webhooks**: the header, the algorithm, and
   whether verification needs more than one header or the query string.
4. **Currencies and refunds**: which currencies, and whether partial refunds
   exist.
5. **Sandbox credentials.** The operator keeps them in their own shell
   environment or types them into the dashboard. Never ask for them in chat,
   and never write them into code, fixtures, tests or commit messages.

## 1. Money rules

- Amounts in the plugin interface are `int64` minor units (cents) of the
  bill currency. Convert at the provider boundary with the helpers in
  `backend/internal/money/` (PayPal uses `money.MajorUnitString` and
  `money.ParseDollarStringToCents`). Never carry money as a float inside the
  plugin.
- JSON responses already emit major units through the `MarshalJSON` methods
  in `backend/internal/database/models_json.go`. Do not divide again.
- Bind every provider object to the business, the bill, the amount and the
  currency, and reject a webhook that disagrees with them. The
  `PaymentProviderContract` fields in
  `backend/internal/plugins/payment_contract.go` list what a first-party
  provider must guarantee: signature verification, replay protection,
  provider-object binding, amount and currency binding, business and bill
  binding, an under- and overpayment policy, and reconciliation.

## 2. Backend: the plugin package

Create `backend/internal/plugins/<name>/<name>.go`.

1. Implement `plugins.PaymentPlugin` from
   `backend/internal/plugins/interface.go`:
   - the `Plugin` methods: `GetName`, `GetDisplayName`, `GetDescription`,
     `GetCategory` (`"payment"`), `GetVersion`, `GetFeatures` (a JSON array),
     `GetConfigSchema` (a JSON Schema), `IsActive`, `ValidateConfig`,
     `Initialize` and `Cleanup`;
   - the payment methods: `ProcessPayment`, `RefundPayment`,
     `GetPaymentStatus`, `CreateBillPayment`, `HandleWebhook`,
     `GetWebhookEndpoint` (`"/api/v1/webhooks/<name>"`) and
     `VerifyWebhookSignature`.
2. Register it from `init()`, as `paypal.go` does:

   ```go
   func init() {
   	plugins.RegisterPluginInitializer("<name>", func(pluginService *services.PluginService) {
   		plugins.GlobalRegistry.RegisterPlugin(New<Name>Plugin(pluginService))
   	})
   }
   ```

3. In `Initialize`, check the credentials with one cheap provider call, so a
   bad key fails when the owner saves the form, not at the first payment.
   See `backend/internal/plugins/paypal/test_connection.go`.
4. `IsActive` returns
   `plugins.PaymentProviderStartsEnabled(p.GetName(), appconfig.IsProductionMode(false))`,
   as PayPal and Stripe do. Production then stays closed until the operator
   opts in (step 6).
5. Implement the optional interfaces in `interface.go` only if the flow needs
   them:
   - `WebhookRequestVerifier`, when one header and the body are not enough;
   - `PaymentReturnCapturer`, for a capture after the buyer returns;
   - `ConfigNormalizer`, to encrypt secrets before they are stored;
   - `PublicConfigProvider`, to mask secrets in dashboard responses;
   - `AlternativePaymentTracker` and `DetailedPaymentStatusProvider`.
6. **Catalog metadata lives in the `Get*` methods.** The registry sync
   overwrites the catalog row on every boot. Never add the name to
   `defaultSeedPlugins` in `backend/internal/services/plugin_init.go`;
   `TestSeedAndRegistryPluginNamesDisjoint` in `internal/plugins` fails if a plugin is both
   registered in Go and seeded.
7. **Translations.** Add `es` and `es-AR` entries in
   `InitializePluginTranslations` in the same file.

## 3. Backend: wire it in

| What | Where |
|---|---|
| Blank import `_ "github.com/stdevmac/payverge/backend/internal/plugins/<name>"` | the import block of `backend/cmd/app/main.go`, and the same in `backend/internal/plugins/catalog_source_of_truth_test.go`, which mirrors main.go's imports |
| `webhookRoutes.POST("/<name>", pluginHandlers.Handle<Name>Webhook)` | the `webhookRoutes` group (`/api/v1/webhooks`) in `backend/cmd/app/main.go`. Add return and cancel routes only for a redirect flow, as for `/paypal/return`. |
| `Handle<Name>Webhook`, which calls `ph.handlePaymentWebhook(c, "<name>")` | `backend/internal/handlers/plugin_handlers.go`, next to `HandleStripeWebhook` |
| The signature header and the ordered env-var fallbacks for the webhook secret | `pluginWebhookConfigs` in the same file. Without an entry, the payload counts as unverified. |
| `contract("<name>", "PAYMENT_PROVIDER_<NAME>_ENABLED", "<sandbox-job>")` | `productionPaymentProviderContracts` in `backend/internal/plugins/payment_contract.go`, plus the variable, set to `false`, in `.env.example` |

Adding the contract row claims that every property in it holds. Back each
one with a test (step 5).

### The provider allow-lists

Switches on the plugin name in `backend/internal/handlers/plugin_handlers.go`
decide what a provider may do. Without them the plugin registers, but guests
cannot pay with it and its webhooks get
`503 Secure webhook verification unavailable`.

| Function | Decides |
|---|---|
| `guestBillPaymentPluginSupported` | whether guests may start a bill payment with it |
| `guestPaymentOptionVisible` | whether it shows among the guest's payment options |
| `paymentWebhookProviderSupported` | whether its webhooks are accepted |
| `firstPartyRequiresWebhookID` | whether replay protection requires an event id |
| `extractPluginWebhookEventType`, `extractPluginWebhookID`, `extractPluginWebhookBusinessID`, `extractPluginWebhookBillID` | where the payload carries the event type, the event id, the business and the bill |
| `guestCardPluginHasCredentialShape`, `guestPaymentPluginRank` | whether it counts as a configured card rail, and its position (review both) |

To catch any switch added since this was written:

```bash
grep -n 'case "mercadopago"' backend/internal/handlers/plugin_handlers.go
git grep -n '"stripe", "paypal", "mercadopago"' -- backend ':!*_test.go'
```

The AI keyword lists in `backend/internal/agents/` also match. Leave them
alone.

## 4. Frontend

| What | Where |
|---|---|
| The name constant | `PLUGIN` in `frontend/src/constants/plugins.ts` |
| The icon | `frontend/public/images/plugins/<name>-logo.png` (the catalog row always points there), and the map in `frontend/src/utils/pluginImages.ts` |
| The config form | a `<Name>Config.tsx` next to `frontend/src/components/business/plugins/PayPalConfig.tsx`, and a `case PLUGIN.<name>:` in `PluginConfigFactory.tsx` |
| The form contract | `build<Name>InitialConfig` and `<NAME>_UI_ONLY_FIELDS` in `configFields.ts`; a copy of `GetConfigSchema` at `__fixtures__/plugin-config-schemas/<name>.json`; a case in `__tests__/configFields.contract.test.ts` (all under `frontend/src/components/business/plugins/`) |
| Guest checkout | `knownGuestRails` and `getPluginIcon` in `frontend/src/components/guest/PaymentSection.tsx`, `paymentMethodLabel` in `frontend/src/lib/paymentMethodLabels.ts`, and the method switch in `frontend/src/lib/paymentMethod.ts` |
| Marketplace order | `PAYMENT_PLUGIN_ORDER` and `CARD_PAYMENT_NAMES` in `frontend/src/utils/pluginMarketplaceOrder.ts` |

The dashboard saves the whole config object. The backend puts masked secrets
back (`restoreMaskedSecrets` in `plugin_handlers.go`), so the form never needs
to show a stored secret. If you add a translated guest label, add the key to
every bundle in `frontend/src/i18n/guest-messages/` so the parity hooks
pass.

## 5. Tests

Add the plugin to the backend tests that pin the provider set:

- the cases in `backend/internal/plugins/config_schema_contract_test.go`
  (the live schema must match the frontend fixture);
- `productionPaymentProviders` in
  `backend/internal/plugins/payment_provider_behavior_test.go`, which drives
  malformed config, unsigned callbacks and invalid attempts through every
  provider;
- `want` in `backend/internal/plugins/payment_contract_test.go`.

In the package, write tests in the style of `paypal_webhook_test.go` and
`paypal_refund_test.go`: a valid signature, a bad signature, a replayed
event, and a webhook whose amount, currency or bill does not match. A live
sandbox test goes behind `//go:build integration` and reads its credentials
from environment variables, as `paypal_sandbox_integration_test.go` does.

Run from `backend/`:

```bash
go build ./...
go test -short ./internal/plugins/... ./internal/handlers/... ./internal/services/...
go test -tags integration ./internal/plugins/<name>/   # only with the sandbox variables set in your shell
```

Run from `frontend/`:

```bash
npx jest src/components/business/plugins/ src/lib/ src/utils/
npm run typecheck
npm run lint
```

## 6. Turn it on

- **Development**: the provider starts enabled unless
  `PAYMENT_PROVIDER_<NAME>_ENABLED=false`.
- **Production** is closed by default. Set
  `PAYMENT_PROVIDER_<NAME>_ENABLED=true` only after a sandbox payment, a
  refund and a signed webhook have all passed. The compose file must forward
  the variable to the backend's `environment`. A value that sits only in
  `.env` never reaches the container. In your fork, add
  `PAYMENT_PROVIDER_<NAME>_ENABLED=${PAYMENT_PROVIDER_<NAME>_ENABLED:-false}`
  to the backend `environment` of every compose file you ship
  (`docker-compose.yml`, and `deploy/docker-compose.yml` once it exists),
  and a commented example to the matching `.env.example`. For an existing
  install, see "Forward a variable the compose file does not name" in
  `.claude/skills/troubleshoot/SKILL.md`.
- In the provider's dashboard, set the webhook URL to
  `<PUBLIC_URL>/api/v1/webhooks/<name>`. Put the webhook secret in the env
  var named in `pluginWebhookConfigs`, or in the business's plugin config.
- The owner enables and configures the plugin on the dashboard's plugins
  page. `payverge_payment_plugin_status` (MCP) then reports whether payments
  are ready.
- A platform admin can stop new payments for every provider with the
  `payments_enabled` control in `docs/runbooks/runtime-launch-controls.md`.
  Webhooks and reconciliation keep running.

## Do not

- Commit credentials, or real webhook payloads and recorded responses that
  contain customer data.
- Accept a webhook that has no signature check.
- Use `git add -A`. Stage each file by path.
