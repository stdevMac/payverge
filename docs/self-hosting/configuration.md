# Configuration reference

Every environment variable that the backend, the frontend and the deploy
stack read, plus the backend's command-line flags. Set them in `deploy/.env`
(next to `deploy/docker-compose.yml`) and run `docker compose up -d`, which
recreates only the containers whose settings changed.

This page is checked by a test: `scripts/docs/env-inventory.mjs` scans the Go
backend, the Next.js frontend and `deploy/docker-compose.yml` for every
variable they read, and `node --test scripts/docs/env-inventory.test.mjs`
fails when one is missing here. To see where a variable is read:

```sh
node scripts/docs/env-inventory.mjs | grep RATE_LIMIT
node scripts/docs/env-inventory.mjs --json   # every source file:line
```

How to read the tables:

- **Default** is what happens when the variable is unset or empty. "compose:"
  marks a default that `deploy/docker-compose.yml` supplies; the bare binary
  default can differ, and both are given when they do.
- **Req.** is "yes" when the stack refuses to start without it, "prod" when
  only production mode needs it, and blank when it is optional.
- **Compose** says whether `deploy/docker-compose.yml` passes the variable to
  the container. A variable marked "no" has to be added to the service's
  `environment:` list (or a `docker-compose.override.yml`) before setting it
  in `.env` has any effect. Compose only forwards variables it names.

Production mode is on when the backend gets `--production` (the deploy
compose always passes it) or when `APP_ENV`, `ENV` or `NODE_ENV` is
`production` (`ENV=prod` also counts).

## Contents

1. [Required](#required)
2. [Public URLs, domain and cookies](#public-urls-domain-and-cookies)
3. [Edge, TLS and proxy trust](#edge-tls-and-proxy-trust)
4. [Database](#database)
5. [Admin, sign-ups and demo data](#admin-sign-ups-and-demo-data)
6. [Branding and contacts](#branding-and-contacts)
7. [Email](#email)
8. [File storage](#file-storage)
9. [Guest payments](#guest-payments)
10. [USDC on Base](#usdc-on-base)
11. [AI](#ai)
12. [Telegram, WhatsApp and web push](#telegram-whatsapp-and-web-push)
13. [Fiscal (Argentina, ARCA)](#fiscal-argentina-arca)
14. [Rate limits and quotas](#rate-limits-and-quotas)
15. [Retention and background workers](#retention-and-background-workers)
16. [Observability](#observability)
17. [Admin MCP, debug and maintenance switches](#admin-mcp-debug-and-maintenance-switches)
18. [Frontend](#frontend)
19. [Deploy stack only](#deploy-stack-only)
20. [Backend command-line flags](#backend-command-line-flags)
21. [Not for operators](#not-for-operators)

## Required

The deploy compose refuses to start until these are set. `./install.sh`
generates all of them (see [install.md](install.md)).

| Variable | Default | Req. | Effect |
|---|---|---|---|
| `DOMAIN` | none | yes | Public host name, no scheme (`pay.example.com`). `localhost` runs a local trial on Caddy's own CA. Builds the default `PUBLIC_URL`. |
| `DB_PASSWORD` | none | yes | Postgres password, applied when the database volume is first created. Changing it later locks the backend out until you change it inside Postgres too. `openssl rand -hex 24`. |
| `JWT_SECRET_KEY` | none | yes | Signs session tokens. Changing it signs everyone out. `openssl rand -base64 48`. |
| `PLUGIN_SECRET_KEY` | none | yes | Encrypts the payment-provider credentials restaurants store. Changing it makes them unreadable. `openssl rand -hex 32` (a base64 key of 32 bytes also works). |
| `ADMIN_EMAIL` | none | first boot | The first platform admin, created on first boot. Also the owner of `DEMO_DATA` restaurants and a fallback recipient for admin mail. See [admin.md](admin.md). |
| `ADMIN_PASSWORD` | none | first boot | Password for `ADMIN_EMAIL` (12+ characters, 5+ distinct). Remove it after the first start; `install.sh` does. |

Back up `deploy/.env`: without `PLUGIN_SECRET_KEY` a restored database is
missing every stored provider credential. See [backups.md](backups.md).

## Public URLs, domain and cookies

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `PUBLIC_URL` | compose: `https://<DOMAIN>` | prod | yes | Canonical origin. Used for links in email, OAuth redirects, CORS, CSRF origin checks, SIWE domains and the frontend's absolute URLs. Include the port if `HTTPS_PORT` is not 443. |
| `APP_BASE_URL` | `PUBLIC_URL` | | set from `PUBLIC_URL` | Public origin of the API, used for OAuth and webhook callbacks and emailed API links. Set it only when the API lives on its own host. It never becomes the instance URL or a trusted CORS origin. |
| `APP_DOMAIN` | none | | set from `DOMAIN` | One more host added to the default redirect allow-list (only when `ALLOWED_REDIRECT_DOMAINS` is unset). |
| `ALLOWED_ORIGINS` | `PUBLIC_URL` only | | yes | Extra comma-separated CORS and SIWE origins (another frontend host, a staging domain). |
| `ALLOWED_REDIRECT_DOMAINS` | the `PUBLIC_URL` host plus `APP_DOMAIN` | | yes | Comma-separated hosts the backend may redirect to after OAuth and payment returns. No automatic `www.` twin: list it if you serve both. |
| `COOKIE_DOMAIN` | host-only cookies | | yes | Set a parent domain (`example.com`) to share the session cookie across subdomains. Leave empty for a single host. |
| `PRIMARY_VENUE` | none | | yes | Numeric id, storefront slug (`custom_url`) or `business_id` slug of the venue served at `/`, tried in that order. The storefront slug ignores case; the `business_id` must match exactly. Unset: `/` serves the only published venue, a directory when there are several, or redirects to `/dashboard` when there are none. An unknown value logs a warning and falls back to that rule. It does not skip the operator venue picker: an owner with several venues still chooses one at `/dashboard`. Changes reach `/` within about 90 seconds (the home answer is cached). See [api/home.md](../api/home.md). |

## Edge, TLS and proxy trust

How the client IP reaches the backend decides whether per-client rate limits
work. [reverse-proxies.md](reverse-proxies.md) walks through each setup.

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `EDGE` | compose: `none` | | yes | `none`: Caddy faces the internet. `cloudflare`: Caddy takes the client IP from `CF-Connecting-IP`, only from Cloudflare's ranges. The backend and frontend read it too (CSP, preflight). |
| `TLS_TERMINATION` | compose: `caddy` | | yes (Caddy) | `caddy`: Caddy gets certificates. `proxy`: another server terminates TLS and forwards plain HTTP to Caddy. |
| `TRUSTED_PROXY_CIDRS` | none (trust no one) | | yes (Caddy) | Space-separated CIDRs of proxies in front of Caddy. Caddy then reads `X-Forwarded-For` right to left in strict mode and skips only those hops. |
| `TRUSTED_PROXIES` | see below | | yes | Comma-separated CIDRs or IPs whose `X-Forwarded-For` the backend believes. |
| `TRUSTED_PLATFORM` | empty | | forced empty | `cloudflare` makes Gin read `CF-Connecting-IP` directly. The deploy compose blanks it because Caddy already resolved the IP; only for running the backend bare behind Cloudflare. |
| `FRONTEND_TRUSTED_PROXIES` | empty (trust no one); compose: private ranges | | yes (frontend) | CIDRs whose `X-Forwarded-For` the frontend's same-origin proxy keeps. See [frontend-config.md](frontend-config.md#client-ip-trust). |
| `HSTS_POLICY` | `max-age=31536000` | | yes (Caddy) | `Strict-Transport-Security` that Caddy sends for `DOMAIN`. |
| `FORCE_HSTS` | `false` | | yes | `true` makes the backend send HSTS itself; only matters when the backend is reached without Caddy. |
| `ACME_EMAIL` | none | | yes (Caddy) | Let's Encrypt account email for expiry notices. |
| `HTTP_PORT` | `80` | | yes (Caddy) | Host port (or `127.0.0.1:port`) Caddy publishes for HTTP. |
| `HTTPS_PORT` | `443` | | yes (Caddy) | Host port Caddy publishes for HTTPS. |
| `EDGE_SUBNET` | `172.30.0.0/24` | | yes (compose) | Subnet of the deploy stack's edge network; the default for `TRUSTED_PROXIES` and `FRONTEND_TRUSTED_PROXIES`. |

`TRUSTED_PROXIES` default: the binary defaults to loopback only
(`127.0.0.0/8,::1`). The deploy compose gives its edge network a fixed
subnet, `EDGE_SUBNET` (default `172.30.0.0/24`), and sets `TRUSTED_PROXIES` to
that subnet plus `127.0.0.1` and `FRONTEND_TRUSTED_PROXIES` to that subnet,
because Caddy and the frontend are the only peers and neither app publishes a
host port. Change `EDGE_SUBNET` when the range is taken on the host (a second
stack, such as a restore drill, needs its own); see
[reverse-proxies.md](reverse-proxies.md#trusted_proxies-on-the-backend).

## Database

The server binary takes its database location from flags (see
[flags](#backend-command-line-flags)); the deploy compose passes
`--db-host postgres` and the values below. The admin CLI
(`/app/server admin ...`) reads the `DB_*` variables directly.

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `DB_USER` | `payverge` | | yes | Postgres role the apps, backup and restore log in as. The deploy compose creates it on the first start as the database owner, not a superuser (`deploy/postgres/init-app-role.sql`); it cannot be `postgres`. |
| `DB_NAME` | `payverge` | | yes | Database name (`POSTGRES_DB`). |
| `DB_PASSWORD` | none | yes | yes | See [Required](#required). |
| `DB_HOST` | CLI: `localhost`; compose: `postgres` | | via flag | Host for the admin CLI. |
| `DB_PORT` | `5432` | | via flag | Port for the admin CLI. |
| `DB_SSLMODE` | CLI: `require`; compose flag: `disable` | | via flag | libpq `sslmode`. The compose network is private, so it disables TLS. Use `require` or `verify-full` for a managed database. |
| `DB_SLOW_QUERY_LOG` | `false` | | no | `true` logs every query slower than 100 ms. |
| `DATA_DIR` | `data` (relative to the working directory) | | no | Directory for instance secrets and small state files the backend keeps outside the database. Must be on a volume and outside `STORAGE_DIR`. |

## Admin, sign-ups and demo data

Details in [admin.md](admin.md).

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `REGISTRATION_MODE` | `invite` | | yes | `invite`: operators need an invite code. `open`: anyone can sign up. `closed`: no new accounts. |
| `REGISTRATION_RATE_LIMIT_PER_HOUR` | `5` | | yes | Sign-ups per client IP per hour. |
| `EMAIL_VERIFICATION` | `auto` | | yes | `auto`: required when mail is really delivered. `required` or `off` force it. See [email.md](email.md#verification-modes). |
| `DEMO_DATA` | `false` | | yes | `true` seeds demo restaurants owned by `ADMIN_EMAIL` on boot (idempotent). |
| `DEMO_MODE` | `false` | | demo overlay | `true` turns the install into a public demo: one-click owner/staff sign-in, write limits, outbound actions refused, email forced to `log`, sign-up closed. Requires `DEMO_DATA=true` (startup refuses it alone). Never on an instance with real restaurants. See [public-demo.md](public-demo.md). |
| `DEMO_RESET_UTC` | `03:00` | | demo overlay | Nightly reset time (UTC, `HH:MM`) shown in the demo banner and `/api/v1/instance`. Display only: the timer in `deploy/demo` owns the schedule. |
| `DEMO_WRITE_RATE_PER_MIN` | `30` | | demo overlay | State-changing requests per client IP per minute while `DEMO_MODE` is on. `0` or invalid falls back to the default. |
| `ADMIN_DEMO_AUTOMATION_ENABLED` | follows `DEMO_DATA` | | yes | Explicit on/off for the periodic demo refresh jobs. |
| `ADMIN_DEMO_EMAIL_DOMAIN` | built-in demo domain | | no | Domain used for generated demo staff and guest addresses. |
| `ADMIN_EMAILS` | `ADMIN_EMAIL` | | no | Comma-separated recipients of platform-admin notifications (new sign-ups, alerts). |
| `GOOGLE_CLIENT_ID` | none | | yes | Google sign-in OAuth client. Redirect URI: `https://<DOMAIN>/api/v1/auth/google/callback`. Both id and secret are needed. |
| `GOOGLE_CLIENT_SECRET` | none | | yes | Secret for the client above. |

## Branding and contacts

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `PRODUCT_NAME` | `Payverge` | | yes | Product name in email and pages (80 chars max). |
| `COMPANY_NAME` | Payverge default | | yes | Company in email footers (120 chars max). |
| `COMPANY_ADDRESS` | empty | | yes | Postal address in email footers. Many anti-spam laws require one for marketing mail. |
| `LEGAL_ENTITY` | Payverge default | | yes | Legal entity name in footers and legal pages (160 chars max). |
| `LEGAL_TERMS_URL` | empty | | yes | Absolute `https://` URL of your own terms. When set, `/terms-and-conditions` redirects there instead of rendering the generic template. |
| `LEGAL_PRIVACY_URL` | empty | | yes | Absolute `https://` URL of your own privacy policy. When set, `/privacy-policy` redirects there. |
| `LOGO_URL` | built-in logo | | yes | Absolute `https://` URL of the logo used in email. |
| `BRAND_COLOR` | built-in teal | | yes | Hex color (`#1a6b6a`) for email buttons and accents. |
| `SUPPORT_EMAIL` | none | | yes | Contact shown to guests and operators. Also passed to the frontend. |
| `SECURITY_EMAIL` | `SUPPORT_EMAIL` | | yes | Contact in `/.well-known/security.txt`. |

## Email

Full guide: [email.md](email.md). With no provider configured, mail is
written to the backend log.

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `EMAIL_PROVIDER` | `log` (unset) | | yes | `smtp`, `resend`, `postmark` or `log`. |
| `SMTP_HOST` | none | smtp | yes | SMTP server. |
| `SMTP_PORT` | `587` | | yes | SMTP port. |
| `SMTP_USERNAME` | none | | yes | SMTP login. |
| `SMTP_PASSWORD` | none | | yes | SMTP password. |
| `SMTP_FROM` | `FROM_EMAIL` | | yes | Envelope and header sender for SMTP. |
| `SMTP_TLS` | `auto` | | yes | `auto` (STARTTLS when offered, implicit TLS on 465), `starttls`, `tls` or `none`. |
| `RESEND_API_KEY` | none | resend | yes | Resend API key. |
| `RESEND_WEBHOOK_SECRET` | none | | yes | Verifies Resend bounce and complaint webhooks. |
| `EMAIL_API_KEY` | none | | yes | Provider API key for `postmark` (and a fallback for `resend`). |
| `FROM_EMAIL` | none | | yes | Sender for transactional mail. |
| `FROM_EMAIL_UPDATES` | `FROM_EMAIL` | | yes | Sender for product updates and digests. |
| `EMAIL_REPLY_TO` | none | | yes | `Reply-To` on outgoing mail. |
| `EMAIL_ALLOWED_FROM_DOMAINS` | the `FROM_EMAIL` domain | | yes | Comma-separated domains a sender address may use. The production preflight rejects senders outside it. |
| `EMAIL_SPF_DOMAIN` | none | | yes | Domain the production preflight checks for SPF. |
| `EMAIL_DKIM_DOMAIN` | none | | yes | Domain the preflight checks for DKIM. |
| `EMAIL_RETURN_PATH_DOMAIN` | none | | yes | Bounce domain the preflight checks. |
| `EMAIL_DMARC_POLICY` | none | | yes | Expected DMARC policy (`quarantine`, `reject`). |
| `EMAIL_OUTBOX_ENABLED` | `true` | | no | `false` sends synchronously instead of through the retrying outbox. |
| `EMAIL_OUTBOX_TTL_HOURS` | `24` | | no | How long a failing message is retried before it is dropped. |
| `EMAIL_OUTBOX_RETENTION_DAYS` | `14` | | no | How long sent and dead outbox rows are kept. |
| `EMAIL_TENANT_DAILY_CAP` | `1000` | | yes | Mail a single restaurant may send per day. |
| `EMAIL_TENANT_DEMO_DAILY_CAP` | `20` | | yes | Same, for a demo restaurant. |
| `EMAIL_TENANT_RECIPIENT_DAILY_CAP` | `20` | | yes | Mail one restaurant may send to one recipient per day. |
| `EMAIL_TENANT_RECIPIENT_GLOBAL_DAILY_CAP` | `10` | | yes | Mail all restaurants together may send to one recipient per day for tenant-triggered messages. |
| `EMAIL_TENANT_RECIPIENT_GLOBAL_ALL_DAILY_CAP` | `50` | | yes | Mail all restaurants together may send to one recipient per day, for every tenant-triggered purpose (staff invites, receipts, bookings). Stops many restaurants from flooding one address. Receipts count against a separate key with the same cap, so invites cannot crowd them out. |
| `EMAIL_TENANT_RESERVATION_DAILY_CAP` | `200` | | yes | Reservation mail per restaurant per day. |
| `EMAIL_TENANT_RESERVATION_RECIPIENT_DAILY_CAP` | `3` | | yes | Reservation mail per recipient per restaurant per day. |
| `EMAIL_TENANT_DEDUPE_MINUTES` | `10` | | yes | Identical tenant messages to one recipient inside this window are sent once. |
| `UNSUBSCRIBE_TOKEN_SECRET` | derived from `JWT_SECRET_KEY` | | no | Separate HMAC key for unsubscribe links. Changing it (or `JWT_SECRET_KEY` when unset) invalidates links already sent. |

## File storage

Full guide: [storage.md](storage.md).

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `STORAGE_DRIVER` | compose: `local` | | yes | `local`: files on the `storage` volume, served at `/media/*`. `s3`: S3 or compatible. |
| `STORAGE_DIR` | `/data/storage` in production, `./data/storage` otherwise | | set | Directory of the local driver inside the container. Compose sets it to `/data/storage` (the `storage` volume). |
| `S3_BUCKET` | none | s3 | yes | Public bucket. |
| `S3_PROTECTED_BUCKET` | `protected/` prefix of `S3_BUCKET` | | yes | Private bucket for receipts and exports. Never make it public. |
| `AWS_REGION` | `us-east-1` | | via flag | Bucket region (`auto` for R2). |
| `AWS_ACCESS_KEY` | none | s3 | yes | Access key for the public bucket. |
| `AWS_SECRET_KEY` | none | s3 | yes | Secret for the key above. |
| `AWS_PROTECTED_ACCESS_KEY` | `AWS_ACCESS_KEY` | | yes | Access key for the protected bucket. |
| `AWS_PROTECTED_SECRET_KEY` | `AWS_SECRET_KEY` | | yes | Secret for the key above. |
| `S3_ENDPOINT` | AWS | | yes | `https://...` endpoint for R2, B2, Wasabi, MinIO. |
| `S3_PROTECTED_ENDPOINT` | `S3_ENDPOINT` | | yes | Endpoint for the protected bucket. |
| `S3_FORCE_PATH_STYLE` | `false` | | yes | `true` for MinIO and most self-hosted S3. |
| `S3_PUBLIC_BASE_URL` | `PUBLIC_URL/media` | | yes | Public URL (or CDN) of the public bucket. |
| `S3_PROTECTED_BASE_URL` | none | | via flag | Base URL for protected objects when served through a signed CDN. |
| `MEDIA_ORIGINS` | none | | yes (frontend) | Origins of `S3_PUBLIC_BASE_URL`, for the frontend's CSP and image optimizer. |
| `MEDIA_RATE_LIMIT_REQUESTS_PER_MINUTE` | built-in | | no | Per-IP limit on `/media/*`. |
| `MEDIA_RATE_LIMIT_BURST` | built-in | | no | Burst for the limit above. |
| `MINIO_ROOT_USER` | `payverge-root` | | yes (minio) | Root user of the bundled MinIO (profile `minio`). |
| `MINIO_ROOT_PASSWORD` | none | minio | yes (minio) | Root password of the bundled MinIO. |
| `MINIO_IMAGE` | Chainguard MinIO, pinned | | yes (minio) | Another MinIO image. |

## Guest payments

Restaurants connect their own provider accounts in the dashboard; these are
platform-level values. Guide: [payments.md](payments.md).

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `PAYMENT_PROVIDER_STRIPE_ENABLED` | production: off; otherwise on | | yes | `true` lets restaurants enable Stripe in production. Unset keeps it off in production; any value other than `true` turns it off everywhere. See [payments.md](payments.md#production-activation). |
| `PAYMENT_PROVIDER_PAYPAL_ENABLED` | production: off; otherwise on | | yes | Same, for PayPal. |
| `PAYMENT_PROVIDER_MERCADOPAGO_ENABLED` | none | | yes | Declared for MercadoPago, but the current release does not read it: MercadoPago is always available. See [payments.md](payments.md#production-activation). |
| `STRIPE_SECRET_KEY` | none | | yes | Platform key for Stripe Connect OAuth and connected-account calls. |
| `STRIPE_CONNECT_CLIENT_ID` | none | | yes | Stripe Connect client id (`ca_...`) for the "Connect Stripe" button. |
| `STRIPE_WEBHOOK_SECRET` | none | | yes | Signing secret for `/api/v1/webhooks/stripe`. |
| `STRIPE_PLUGIN_WEBHOOK_SECRET` | none | | yes | Signing secret of the per-plugin Stripe webhook. |
| `STRIPE_CONNECT_WEBHOOK_SECRET` | none | | yes | Signing secret of the Connect webhook. Any of the three Stripe secrets is accepted on the plugin route. |
| `PAYPAL_WEBHOOK_SECRET` | none | | yes | Shared secret checked on PayPal webhooks. |
| `MERCADOPAGO_CLIENT_ID` | none | | yes | MercadoPago app for the OAuth "connect" button and token refresh. |
| `MERCADOPAGO_CLIENT_SECRET` | none | | yes | Secret for the app above. |
| `MERCADOPAGO_WEBHOOK_SECRET` | none | | yes | Signature secret for MercadoPago webhooks. |
| `MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS` | none | | yes | Previous secret, also accepted during a rotation. |
| `MAX_PAYMENT_AMOUNT_CENTS` | `100000000` (1,000,000.00) | | yes | Hard ceiling on one payment. Invalid values fall back to the default. |
| `CRYPTO_QUOTE_SECRET` | derived from `JWT_SECRET_KEY` | | no | Signs locked crypto price quotes. |
| `QUOTE_RATE_MAX_AGE_MINUTES` | `360` | | yes | Oldest exchange rate a quote may lock. |
| `EXCHANGE_RATE_MAX_STEP_FACTOR` | `10` | | no | A new rate more than this factor away from the last one is rejected as corrupt. |

## USDC on Base

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `RPC_URL` | `https://mainnet.base.org` | | via flag | Backend RPC for verifying USDC transfers. Use a private RPC for volume. |
| `PUBLIC_RPC_URL` | public Base RPC | | yes (frontend) | RPC the browser uses. Never put a keyed RPC here. |
| `NETWORK` | `base` | | yes (frontend) | `base` or `baseSepolia` for the wallet UI. Any other value falls back to `baseSepolia`. |
| `USDC_MIN_CONFIRMATIONS` | `3` | | yes | Blocks a transfer needs before it counts as paid. |
| `CRYPTO_REFUND_MAINNET_ENABLED` | `false` | | yes | `true` allows USDC refunds on mainnet. |
| `CRYPTO_REFUND_WORKER_CONCURRENCY` | built-in | | no | Parallel refund jobs. |
| `CRYPTO_REFUND_WORKER_INTERVAL_SEC` | built-in | | no | Refund worker poll interval. |
| `REORG_WATCH_ENABLED` | `true` | | no | `false` turns off the chain-reorg reconciler. |
| `REORG_WATCH_INTERVAL_SEC` | `60` | | no | Reconciler interval. |
| `REORG_FINALITY_WINDOW_MIN` | `30` | | no | How long after payment a transfer is rechecked. |
| `REORG_WATCH_COOLDOWN_SEC` | `120` | | no | Minimum gap between checks of one payment. |
| `REORG_SUSPICION_GRACE_SEC` | `300` | | no | Grace before a missing transfer raises an alert. |
| `REORG_WATCH_BATCH` | `100` | | no | Payments checked per run. |

## AI

Full guide: [ai.md](ai.md). Off until a model endpoint is configured.

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `OPENROUTER_API_KEY` | none | | yes | OpenRouter key. Turns the AI features on. |
| `LLM_BASE_URL` | OpenRouter | | yes | Any OpenAI-compatible endpoint (Ollama, vLLM, LiteLLM). |
| `LLM_API_KEY` | `OPENROUTER_API_KEY` | | yes | Key for `LLM_BASE_URL`. |
| `OPENROUTER_MODEL_CHAT` | `google/gemini-2.5-flash` | | yes | AI waiter model. |
| `OPENROUTER_MODEL_MENU` | `google/gemini-2.5-flash` | | yes | Menu import model. |
| `OPENROUTER_MODEL_IMAGE` | `google/gemini-2.5-flash-image` | | yes | Image model. |
| `OPENROUTER_MODEL_DIRECTOR` | `google/gemini-2.5-flash` | | yes | Director console model. |
| `OPENROUTER_MODEL_GUARDRAIL` | `google/gemini-2.5-flash-lite` | | yes | Prompt-injection screen model. |
| `OPENROUTER_CHAT_FALLBACKS` | none | | yes | Comma-separated fallback models for chat. |
| `OPENROUTER_MENU_FALLBACKS` | none | | yes | Same, menu. |
| `OPENROUTER_IMAGE_FALLBACKS` | none | | yes | Same, image. |
| `OPENROUTER_DIRECTOR_FALLBACKS` | none | | yes | Same, director. |
| `OPENROUTER_GUARDRAIL_FALLBACKS` | none | | yes | Same, guardrail. |
| `OPENROUTER_APP_URL` | none | | yes | `HTTP-Referer` sent to OpenRouter. |
| `OPENROUTER_APP_TITLE` | none | | yes | `X-Title` sent to OpenRouter. |
| `OPENROUTER_ZDR_MODE` | `enforce` in production, `audit` otherwise | | yes | Zero-data-retention routing on OpenRouter (`audit` or `enforce`). The production preflight refuses `audit` on OpenRouter; ignored on a local endpoint. See [ai.md](ai.md#docker-compose). |
| `OPENROUTER_ZDR_APPROVED_MODELS` | none | | yes | Comma-separated models proven under ZDR. In `enforce` mode an empty list is bootstrapped from the configured models. |
| `OPENROUTER_PRICES` | built-in table | | yes | Comma-separated `model=in/out` USD prices (per million tokens) for cost accounting of models missing from the built-in table. See [ai.md](ai.md#cost-accounting-and-budgets). |
| `GUARDRAIL_STRICT` | on in production | | yes | `true`: requests are refused when the guardrail model fails. `false`: they pass unscreened. |
| `AI_DAILY_BUDGET_USD` | compose: `5` | | yes | Daily AI spend per restaurant (UTC day). |
| `AI_BUDGET_PER_BUSINESS_USD_DAY` | `AI_DAILY_BUDGET_USD` | | no | Alias for the setting above. |
| `AI_BUDGET_GLOBAL_USD_DAY` | derived: max(`20`, 2 x per restaurant) | | yes | Instance-wide daily AI spend ceiling. |
| `AI_BUDGET_GUEST_POOL_USD_DAY` | built-in | | yes | Daily ceiling for all guest-facing AI together. |
| `AI_WAITER_DAILY_MESSAGE_BUDGET` | `2000` | | yes | AI waiter messages per restaurant per day (HTTP and WhatsApp). |
| `AI_WAITER_DAILY_MESSAGES_PER_DEVICE` | `150` | | yes | AI waiter messages per guest device per day. Also caps WhatsApp messages per sender. |
| `AI_WAITER_DAILY_MESSAGES_PER_IP` | `600` | | yes | AI waiter messages per IP per day. |
| `GOOGLE_TRANSLATE_API_KEY` | none | | yes | Menu translation. |
| `GOOGLE_PLACES_API_KEY` | none | | yes | Restaurant place lookup during onboarding. |
| `SPACE_SCAN_MAX_FRAMES` | `120` | | no | Frames per floor-plan scan. |
| `SPACE_SCAN_MAX_UPLOAD_BYTES` | `26214400` (25 MiB) | | no | Upload limit of a floor-plan scan. |
| `SPACE_SCAN_SESSION_TTL_MINUTES` | `30` | | no | Lifetime of a scan session. |

## Telegram, WhatsApp and web push

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `TELEGRAM_TOKEN` | none | | yes | Bot token for staff notifications. |
| `TELEGRAM_BOT_TOKEN` | none | | no | Older name for `TELEGRAM_TOKEN`, read when that is empty. |
| `TELEGRAM_BOT_USERNAME` | none | | yes | Bot username for the "connect Telegram" deep link. |
| `TELEGRAM_WEBHOOK_SECRET` | none | with token | yes | Secret Telegram sends with each webhook. `openssl rand -hex 32`. |
| `TELEGRAM_NOTIFICATION_EVENTS` | all (`*`) | | yes | Comma-separated event types to forward. |
| `TELEGRAM_NOTIFICATION_WORKER_CONCURRENCY` | `2` | | no | Parallel Telegram deliveries. |
| `TELEGRAM_CONNECTION_TOKEN_TTL_MINUTES` | built-in | | no | Lifetime of a "connect Telegram" link. |
| `TELEGRAM_ESCALATION_BOT_TOKEN` | none | | yes | Separate bot that receives platform escalations. |
| `TELEGRAM_ESCALATION_CHAT_ID` | none | | yes | Chat that receives them. |
| `WHATSAPP_ENABLED` | `false` | | yes | Turns the WhatsApp channel on. Only in a backend built with `GO_TAGS=whatsapp`; see [whatsapp.md](whatsapp.md). |
| `WHATSAPP_STORE_NAME` | `payverge_whatsapp_store` | | no | Name of the WhatsApp session store. |
| `VAPID_PUBLIC_KEY` | none | | yes | Web push public key (`npx web-push generate-vapid-keys`). Also served to the browser. |
| `VAPID_PRIVATE_KEY` | none | | yes | Web push private key. |
| `VAPID_SUBJECT` | none | | yes | `mailto:` or `https:` contact for push services. |

## Fiscal (Argentina, ARCA)

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `FISCAL_WSAA_URL` | ARCA production | | no | WSAA endpoint. Point it at homologación for testing. |
| `FISCAL_WSFE_URL` | ARCA production | | no | WSFE endpoint. |
| `FISCAL_AR_CF_ID_THRESHOLD_CENTS` | ARCA's current figure | | no | Amount above which a consumidor final invoice needs the buyer's ID. |
| `FISCAL_WORKER_INTERVAL_SECONDS` | `10` | | no | Invoice worker interval. |
| `FISCAL_DELIVERY_WORKER_INTERVAL_SECONDS` | `10` | | no | Invoice delivery worker interval. |
| `FISCAL_DELIVERY_WORKER_CONCURRENCY` | `1` | | no | Parallel invoice deliveries. |

## Rate limits and quotas

Per client IP, so they rely on the proxy trust in
[reverse-proxies.md](reverse-proxies.md).

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE` | `300` | | yes | All API requests. |
| `AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE` | `10` | | yes | Login and token routes. |
| `AUTH_RATE_LIMIT_BURST` | `3` | | yes | Burst for the above. |
| `STAFF_AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE` | `5` | | no | Staff PIN login. |
| `STAFF_AUTH_RATE_LIMIT_BURST` | `2` | | no | Burst for the above. |
| `PUBLIC_FORM_RATE_LIMIT_REQUESTS_PER_MINUTE` | `5` | | no | Public forms (contact, reservations). |
| `PUBLIC_FORM_RATE_LIMIT_BURST` | `2` | | no | Burst for the above. |
| `GUEST_ORDER_RATE_LIMIT_REQUESTS_PER_MINUTE` | `120` | | no | Guest order writes. |
| `GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE` | `240` | | no | Guest table reads. |
| `SSE_MAX_SUBSCRIBERS_PER_BUSINESS` | `200` | | no | Staff live-event connections per restaurant. |
| `SSE_MAX_SUBSCRIBERS_PER_IP` | `30` | | no | Staff live-event connections per IP. |
| `SSE_MAX_GUEST_SUBSCRIBERS_PER_BUSINESS` | `150` | | no | Guest live-event connections per restaurant. |
| `SSE_MAX_GUEST_SUBSCRIBERS_PER_IP` | `10` | | no | Guest live-event connections per IP. |

`REGISTRATION_RATE_LIMIT_PER_HOUR`, `MEDIA_RATE_LIMIT_*`, the AI
quotas and the email caps are in their own sections.

## Retention and background workers

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `AI_TRANSCRIPT_RETENTION_DAYS` | `90` | | no | AI waiter transcripts are deleted after this. |
| `OPS_ASSISTANT_RETENTION_DAYS` | `30` | | no | Ops assistant threads with no new message for this long are deleted. |
| `SPACE_SCAN_RAW_RETENTION_DAYS` | `7` | | no | Raw floor-plan scan frames. |
| `WEBHOOK_EVENT_RETENTION_DAYS` | `60` | | no | Stored provider webhook events. |
| `PLUGIN_NOTIFICATION_TTL_HOURS` | `24` | | no | Plugin notifications. |

Email outbox retention is under [Email](#email).

## Observability

All off by default; nothing leaves the server. See
[security.md](security.md#metrics-health-and-profiling) for the tokens.

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `METRICS_TOKEN` | none | prod | yes | Bearer token for `GET /metrics`. In production `/metrics` stays closed without one. |
| `METRICS_TOKENS` | none | | no | Comma-separated list, for rotating tokens. |
| `HEALTH_DETAIL_TOKEN` | none | | yes | Token that unlocks the detailed `/api/v1/health/ready` body. |
| `SENTRY_DSN` | none | | yes | Backend Sentry DSN (also the frontend's server-side fallback). |
| `SENTRY_ENABLED` | on when a DSN is set | | no | `false` keeps Sentry off with a DSN set. |
| `SENTRY_ENVIRONMENT` | `APP_ENV`, then compose `production` | | yes | Sentry environment tag. |
| `SENTRY_RELEASE` | `NEXT_PUBLIC_RELEASE_SHA`, then `dev`; compose: `PAYVERGE_VERSION` | | set from `PAYVERGE_VERSION` | Sentry release tag. |
| `SENTRY_TRACES_SAMPLE_RATE` | `0.05` | | no | Performance trace sampling (backend; frontend fallback). |
| `SENTRY_ENABLE_TRACING` | `true` | | no | Backend tracing on or off. |
| `SENTRY_ENABLE_LOGS` | `false` | | no | Send backend logs to Sentry. |
| `SENTRY_LOG_LEVELS` | built-in | | no | Comma-separated levels sent when logs are on. |
| `POSTHOG_API_KEY` | none | | yes | Backend product analytics. Needs `POSTHOG_HOST`. |
| `POSTHOG_HOST` | none | | yes | PostHog host (backend and frontend). |
| `POSTHOG_KEY` | none | | yes (frontend) | Browser PostHog project key. |

## Admin MCP, debug and maintenance switches

| Variable | Default | Req. | Compose | Effect |
|---|---|---|---|---|
| `PAYVERGE_ADMIN_MCP_TOKEN_SHA256` | none | | yes | SHA-256 of the admin MCP bearer token. Turns on the admin MCP API. |
| `PAYVERGE_ADMIN_MCP_TOKEN` | none | | yes | The token in clear text. Prefer the hash. |
| `PAYVERGE_ADMIN_MCP_ALLOWED_IPS` | none (any) | | yes | Comma-separated IPs or CIDRs allowed to call the admin MCP API. |
| `PPROF_TOKEN` | none (off) | | no | Enables a token-protected pprof snapshot route. Leave unset. |
| `CLIENT_IP_DEBUG` | `false` | | no | `true` logs how the client IP of each sign-up attempt (`POST /api/v1/auth/register`) was resolved. Only that route logs it. For proxy setup only; see [reverse-proxies.md](reverse-proxies.md#checking-it). |
| `PORT` | `8080` | | set (`8080`) | Backend listen port. |
| `APP_ENV` | none; compose: `production` | | set (`production`) | `production` turns on production mode; also the Sentry environment. |
| `ENV` | none | | no | Same as `APP_ENV` (`production` or `prod`). |
| `NODE_ENV` | none | | no | Backend: `production` turns on production mode. Frontend: set by Next.js. |

## Frontend

Full guide: [frontend-config.md](frontend-config.md). The published image reads
these at runtime; the `NEXT_PUBLIC_*` names are build-time spellings that are
read only when the runtime name is empty.

| Variable | Default | Compose | Effect |
|---|---|---|---|
| `PUBLIC_URL` | `https://<DOMAIN>` | yes | Canonical origin (build-time spelling `NEXT_PUBLIC_PUBLIC_URL`). |
| `BACKEND_INTERNAL_URL` | compose: `http://backend:8080` | yes | Where server-side code and the `/api`, `/media` proxy reach the backend. |
| `INTERNAL_API_URL` | `BACKEND_INTERNAL_URL` | no | Older name for the setting above. |
| `API_URL` | same origin | no | Browser API base. Leave unset for the same-origin proxy. |
| `NEXT_PUBLIC_API_URL` | same origin | no | Build-time spelling of `API_URL`. |
| `NEXT_PUBLIC_PUBLIC_URL` | none | no | Build-time spelling of `PUBLIC_URL`. |
| `SUPPORT_EMAIL` / `NEXT_PUBLIC_SUPPORT_EMAIL` | none | yes | Support contact shown in the UI. |
| `SECURITY_EMAIL` | `SUPPORT_EMAIL` | yes | Contact in `security.txt`. |
| `NETWORK` / `NEXT_PUBLIC_NETWORK` | `base` | yes | Chain for the wallet UI. |
| `RPC_URL` / `NEXT_PUBLIC_RPC_URL` | public Base RPC | no | Browser RPC (deploy compose passes `PUBLIC_RPC_URL` as `RPC_URL`). |
| `MEDIA_ORIGINS` / `NEXT_PUBLIC_MEDIA_ORIGINS` | none | yes | Extra image origins for CSP. |
| `VAPID_PUBLIC_KEY` / `NEXT_PUBLIC_VAPID_PUBLIC_KEY` | none | yes | Web push public key. |
| `MAINTENANCE_MODE` / `NEXT_PUBLIC_MAINTENANCE_MODE` | `false` | yes | `true` shows a maintenance page to everyone. |
| `LIFI_INTEGRATOR` / `NEXT_PUBLIC_LIFI_INTEGRATOR` | none | no | LI.FI integrator id for cross-chain payments. |
| `POSTHOG_KEY` / `NEXT_PUBLIC_POSTHOG_KEY` | none | yes | Browser analytics key. |
| `NEXT_PUBLIC_POSTHOG_HOST` | none | no | Build-time spelling of `POSTHOG_HOST`. |
| `FRONTEND_SENTRY_DSN` / `NEXT_PUBLIC_SENTRY_DSN` | none | yes | Browser Sentry DSN. |
| `FRONTEND_SENTRY_ENABLED` / `NEXT_PUBLIC_SENTRY_ENABLED` | on with a DSN | no | Turn browser Sentry off with a DSN set. |
| `FRONTEND_SENTRY_ENVIRONMENT` / `NEXT_PUBLIC_SENTRY_ENVIRONMENT` | compose: `production` | yes | Sentry environment. |
| `FRONTEND_SENTRY_TRACES_SAMPLE_RATE` / `NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE` | built-in | no | Browser trace sampling. |
| `FRONTEND_SENTRY_REPLAYS_SESSION_SAMPLE_RATE` / `NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE` | `0` | no | Session replay sampling. |
| `FRONTEND_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE` / `NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE` | built-in | no | Replay sampling on error. |
| `NEXT_PUBLIC_SENTRY_RELEASE` | none | no | Browser Sentry release. |
| `FRONTEND_TRUSTED_PROXIES` | none | no | See [Edge, TLS and proxy trust](#edge-tls-and-proxy-trust). |
| `CLOUDFLARE_INSIGHTS` | off | yes | `true` allows the Cloudflare Web Analytics beacon in the CSP. |
| `SEO_INDEXING` | `false` | yes | `true` lets search engines index public pages. |
| `SEO_TWITTER_HANDLE` | none | yes | `twitter:site` handle. |

Build-time only (read by `next build` when you build the image yourself, see
[deploy/README.md](../../deploy/README.md#build-from-source)):

| Variable | Default | Effect |
|---|---|---|
| `NEXT_PUBLIC_RELEASE_SHA` | `dev` | Release id baked into the build (also the backend's Sentry release fallback). |
| `NEXT_PUBLIC_VERSION` | `unknown` | Version string in the build header. |
| `NEXT_PUBLIC_BUILD_TIMESTAMP` | `unknown` | Build time in the build info. |
| `NEXT_OUTPUT_STANDALONE` | off (on in CI) | `1` emits the standalone server the Docker image uses. |
| `NEXT_STRICT_BUILD` | strict | `0` relaxes type and lint errors (local work only). |
| `NEXT_OPTIMIZE_CSS` | off | `1` enables Next's CSS optimizer. |
| `NEXT_PROD_SOURCEMAPS` | off | `1` emits browser source maps. |
| `SENTRY_BUILD_PLUGIN` | off | `1` runs the Sentry webpack plugin. |
| `SENTRY_ORG` | none | Sentry org for source-map upload. |
| `SENTRY_AUTH_TOKEN` | none | Token for source-map upload. Never put it in the runtime env. |
| `SENTRY_FRONTEND_PROJECT` | `payverge-frontend` | Sentry project for upload. |
| `CI` | unset | Quieter Sentry plugin output and standalone output. |

## Deploy stack only

Read by `deploy/docker-compose.yml` and its helper containers, not by the
apps. See [deploy/README.md](../../deploy/README.md).

| Variable | Default | Effect |
|---|---|---|
| `PAYVERGE_VERSION` | the release the deploy files came from | Image tag for backend and frontend. The installer writes the exact version; `latest` follows every new release ([upgrades.md](upgrades.md)). |
| `COMPOSE_PROFILES` | `backup` (from `.env.example`) | Optional services: `backup`, `backup-offsite`, `minio`. |
| `POSTGRES_USER` | `postgres` | The superuser of the `postgres` container. Its password is removed on the first start, so it connects only from inside the container. |
| `PAYVERGE_DB_USER` | `DB_USER` | Set on the `postgres` container from `DB_USER`; the init script creates that role. |
| `POSTGRES_DB` | `DB_NAME` | Set on the `postgres` container from `DB_NAME`. |
| `TZ` | `UTC` | Timezone of the backup scheduler. |
| `BACKUP_TIME` | `03:30` | Daily backup time, `HH:MM` in `TZ`. |
| `BACKUP_KEEP_DAILY` | `7` | Daily sets kept. |
| `BACKUP_KEEP_WEEKLY` | `4` | Weekly sets kept. |
| `BACKUP_KEEP_MONTHLY` | `6` | Monthly sets kept. |
| `BACKUP_DIR` | `./backups` | Host directory for backup sets. |
| `BACKUP_UID` | root | Host user that owns the sets. |
| `BACKUP_GID` | root | Host group that owns the sets. |
| `BACKUP_ARCHIVE_ATTEMPTS` | `3` | Retries of a volume archive when files change during the copy. |
| `BACKUP_MAX_AGE_HOURS` | `26` | The `backup` service reports unhealthy when the last run failed or none succeeded for this many hours (`backups/.status`). |
| `RCLONE_REMOTE` | none (required for `backup-offsite`) | rclone remote and path, such as `offsite:bucket/payverge`. |
| `RCLONE_MODE` | `copy` | `copy` keeps pruned sets off-site; `sync` mirrors pruning. |
| `RCLONE_INTERVAL_SECONDS` | `3600` | Off-site sync interval. |

The `PAYVERGE_*` variables `install.sh` itself reads are listed in
[install.md](install.md#installer-options).

## Backend command-line flags

The server binary takes these flags; each falls back to the variable named.
The deploy compose passes the database flags and `--production`. The admin
subcommands (`/app/server admin ...`) take their own flags; see
[admin.md](admin.md#3-command-reference).

| Flag | Variable | Effect |
|---|---|---|
| `--production` | (`APP_ENV`) | Production mode: strict startup checks, secure cookies, production rate limits. |
| `--db-host` | (none) | Postgres host. Default `localhost`. |
| `--db-port` | (none) | Postgres port. Default `5432`. |
| `--db-user` | (none) | Postgres role. |
| `--db-password` | `DB_PASSWORD` | Postgres password. |
| `--db-name` | (none) | Database name. |
| `--db-sslmode` | (none) | libpq `sslmode`. |
| `--rpc-url` | `RPC_URL` | Base RPC. |
| `--aws-region` | (`AWS_REGION` via compose) | S3 region. |
| `--aws-access-key` | `AWS_ACCESS_KEY` | S3 key. |
| `--aws-secret-key` | `AWS_SECRET_KEY` | S3 secret. |
| `--aws-protected-access-key` | `AWS_PROTECTED_ACCESS_KEY` | Protected bucket key. |
| `--aws-protected-secret-key` | `AWS_PROTECTED_SECRET_KEY` | Protected bucket secret. |
| `--s3-bucket` | `S3_BUCKET` | Public bucket. |
| `--s3-protected-bucket` | `S3_PROTECTED_BUCKET` | Protected bucket. |
| `--s3-endpoint` | `S3_ENDPOINT` | S3 endpoint. |
| `--s3-protected-endpoint` | `S3_PROTECTED_ENDPOINT` | Protected endpoint. |
| `--s3-public-base-url` | `S3_PUBLIC_BASE_URL` | Public base URL. |
| `--s3-protected-base-url` | (`S3_PROTECTED_BASE_URL` via compose) | Protected base URL. |
| `--email-provider` | `EMAIL_PROVIDER` | Email provider. |
| `--email-api-key` | `EMAIL_API_KEY` | Email API key. |
| `--from-email` | `FROM_EMAIL` | Sender. |
| `--from-email-updates` | `FROM_EMAIL_UPDATES` | Updates sender. |
| `--templates-dir` | (none) | Email template directory. Default `email/templates` (bundled in the image). |
| `--google-translate-api-key` | `GOOGLE_TRANSLATE_API_KEY` | Translation key. |
| `--telegram-token` | `TELEGRAM_TOKEN` | Telegram bot token. |
| `--telegram-webhook-secret` | `TELEGRAM_WEBHOOK_SECRET` | Telegram webhook secret. |
| `--telegram-notification-worker-concurrency` | `TELEGRAM_NOTIFICATION_WORKER_CONCURRENCY` | Telegram workers. |
| `--vapid-public-key` | `VAPID_PUBLIC_KEY` | Push public key. |
| `--vapid-private-key` | `VAPID_PRIVATE_KEY` | Push private key. |
| `--vapid-subject` | `VAPID_SUBJECT` | Push contact. |

## Not for operators

Read by tools in the repository, not by a running instance. Listed so the
inventory test can tell them apart from a forgotten setting.

| Variable | Read by | Effect |
|---|---|---|
| `EMAIL_SYNTHETIC_MODE` | `backend/cmd/email-synthetic` | Synthetic delivery check mode. |
| `EMAIL_SYNTHETIC_RECIPIENT` | `backend/cmd/email-synthetic` | Inbox for the synthetic check. |
| `EMAIL_HEALTHCHECK_TO` | `backend/cmd/email-smoke` | Recipient of the email smoke test. |
| `EMAIL_TEMPLATES_DIR` | `backend/cmd/email-synthetic` | Template directory for the email tools. |

Test-only names (`TEST_*`, `TESTPERF_DATABASE_URL`, `GO_ENV`, `GIN_MODE`) and
names Next.js sets itself (`NEXT_RUNTIME`) are excluded by the commented
allow-list in `scripts/docs/env-inventory.test.mjs`.
