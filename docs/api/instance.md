# Instance API

`GET /api/v1/instance` describes this Payverge install: its brand and contacts,
how sign-up works, and which optional integrations are configured.
The frontend reads it on the server and in the browser to brand the UI and to
hide features this install cannot serve.

The endpoint is public and needs no login. The response holds display values
and booleans only. It never includes keys, tokens, DSNs or third-party hosts.

There is no OpenAPI spec for the Payverge API yet. This page is the reference
for this endpoint. The handler is `backend/internal/server/instance_handler.go`.

## Request

```http
GET /api/v1/instance
```

## Response

```json
{
  "product_name": "Acme POS",
  "company_name": "Acme Hospitality",
  "legal_entity": "Acme Hospitality LLC",
  "logo_url": "/brand/logo.svg",
  "brand_color": "#1a6b6a",
  "public_url": "https://pos.example.com",
  "support_email": "help@example.com",
  "security_email": "security@example.com",
  "legal_terms_url": "",
  "legal_privacy_url": "",
  "registration_mode": "invite",
  "email_verification": "required",
  "features": {
    "ai": true,
    "whatsapp": false,
    "telegram": false,
    "email": true,
    "google_oauth": false,
    "crypto": true,
    "fiscal_ar": true
  },
  "demo": { "enabled": false, "mode": false, "reset_utc": "" }
}
```

### Identity and modes

| Field | Source | Notes |
|---|---|---|
| `product_name` | `PRODUCT_NAME` | Defaults to `Payverge`. |
| `company_name` | `COMPANY_NAME` | Defaults to `product_name`. |
| `legal_entity` | `LEGAL_ENTITY` | Defaults to `COMPANY_NAME` when that is set, otherwise empty. |
| `logo_url` | `LOGO_URL` | An absolute http(s) URL or a root-relative path. An invalid value is ignored and reported as empty. The frontend's Content-Security-Policy only allows images from its own origin, the API origin and `MEDIA_ORIGINS`, so host the logo there (for example under `/media/`) or add its origin to `MEDIA_ORIGINS`. |
| `brand_color` | `BRAND_COLOR` | `#rgb` or `#rrggbb`. Defaults to `#1a6b6a`. |
| `public_url` | `PUBLIC_URL` | Has no trailing slash. Falls back to `http://localhost:3000` when unset. |
| `support_email` | `SUPPORT_EMAIL` | Empty when unset or invalid. |
| `security_email` | `SECURITY_EMAIL` | Defaults to `support_email`. |
| `legal_terms_url` | `LEGAL_TERMS_URL` | An absolute http(s) URL of the operator's own terms, or empty. When set, `/terms-and-conditions` redirects there. A relative or invalid value is ignored. |
| `legal_privacy_url` | `LEGAL_PRIVACY_URL` | Same for the privacy policy; `/privacy-policy` redirects there. |
| `registration_mode` | `REGISTRATION_MODE` | `invite` (the default), `open` or `closed`. An unknown value fails closed to `closed`, and production preflight refuses to boot on it. |
| `email_verification` | `EMAIL_VERIFICATION` | The effective mode, either `required` or `off`. The default `auto` resolves to `off` with the `log` provider and to `required` otherwise. It also resolves to `required` when a delivery credential (`EMAIL_API_KEY` or SMTP settings) is set without `EMAIL_PROVIDER`. |

### Feature flags

A flag is `true` when this install can serve the feature. Hide the UI for a
`false` flag rather than offering a feature that will fail.

| Flag | True when |
|---|---|
| `ai` | An LLM provider is wired at boot, through `OPENROUTER_API_KEY` or `LLM_BASE_URL`. See [AI on a self-hosted instance](../self-hosting/ai.md). |
| `whatsapp` | The binary was built with `-tags whatsapp` and `WHATSAPP_ENABLED=true`. |
| `telegram` | A Telegram bot token is configured. |
| `email` | The email provider wired at boot can deliver mail, which means any provider except `log`. Resend or Postmark without an API key is downgraded to `log` in development and refused at startup in production. |
| `google_oauth` | Both `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` are set. |
| `crypto` | A settlement RPC endpoint is resolved at boot. `RPC_URL` or `--rpc-url` wins, and when neither is set the backend uses the public Base mainnet RPC, so this is `true` on every running backend except the public demo: under `DEMO_MODE` it is `false`, because the demo refuses every guest crypto payment and quote route. |
| `fiscal_ar` | Always, except on the public demo: under `DEMO_MODE` it is `false`, because the demo refuses every fiscal (ARCA) write. See below. |

`demo.enabled` is `true` when `DEMO_DATA` is `true`, `1`, `yes` or `on`.
`demo.mode` is `true` when `DEMO_MODE` is on: the install is a public demo
(one-click demo sign-in, outbound side effects off, nightly reset; see
[public demo](../self-hosting/public-demo.md)). `demo.reset_utc` is the
displayed nightly reset time (`DEMO_RESET_UTC`, default `03:00`), empty unless
`demo.mode`.

#### `fiscal_ar` is a capability, not an activation

ARCA (formerly AFIP) e-invoicing for Argentina is built into every binary, and
no build tag or instance setting turns it off. That is why `fiscal_ar` is `true`
on every install except the public demo (`DEMO_MODE`). The flag means "this
install can offer ARCA". It does not mean that anyone on the install uses ARCA,
or that the install is in Argentina.

Fiscal behaviour is decided per business, never per instance:

- A business invoices through ARCA only after it opts in. It needs fiscal
  settings and its own ARCA certificate. Without them, no fiscal job runs for
  that business.
- The dashboard shows ARCA surfaces only to businesses whose country is `AR`.
  Other businesses keep the generic receipt and invoice labels
  (`frontend/src/components/business/fiscal/receiptTypes.ts`).
- Operators can pause fiscal work for the whole install with the
  `fiscal_enabled` runtime control
  ([runtime launch controls](../runbooks/runtime-launch-controls.md)). The
  control does not change this flag.

So a client should check the business country, not just `fiscal_ar`, before it
shows ARCA UI.

## Caching

- The response carries `Cache-Control: public, max-age=60`.
- The backend also reuses the encoded payload for up to 30 seconds.
- A config change can take up to about 90 seconds to reach a browser. Restart
  the backend to apply env changes, since most values are read at boot.

## Errors

The endpoint always answers `200`. If the payload cannot be encoded, it returns
`{}` instead of a 5xx. A client should treat missing fields as unconfigured.
