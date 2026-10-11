# Changelog

All notable changes to Payverge are documented here, newest first. GitHub
Releases are the canonical release notes; this file mirrors them. The format
and versioning rules are described in
[docs/governance/RELEASING.md](governance/RELEASING.md), and the project
follows [Semantic Versioning](https://semver.org/).

## [1.0.2](https://github.com/stdevMac/payverge/compare/v1.0.1...v1.0.2) (2026-10-11)


### Fixes

* **deploy:** forward media rate limits and AI retention settings to the backend ([c13a9c9](https://github.com/stdevMac/payverge/commit/c13a9c9cc938320de3196f44d687e46f8d326b22))
* **deploy:** require a domain at the installer prompt, withdraw HSTS on trial names and flag an unreachable install ([a291d0c](https://github.com/stdevMac/payverge/commit/a291d0c4afc50dadaf468f8fb4d66f4c935bc636))


### Other changes

* **self-hosting:** pin manual installs to a release tag and correct the proxy-trust defaults ([5116e8c](https://github.com/stdevMac/payverge/commit/5116e8c5531908ff87c6567785db8add68aefb07))

## [1.0.1](https://github.com/stdevMac/payverge/compare/v1.0.0...v1.0.1) (2026-10-11)


### Fixes

* **chat:** cap message and announcement length ([6268185](https://github.com/stdevMac/payverge/commit/6268185dcfcfcc06804ed8f57d30c5dfaff3afa6))
* **config:** accept loopback http PUBLIC_URL in the production instance check ([0760dde](https://github.com/stdevMac/payverge/commit/0760dde7bb379aa05f0321cfb7cb46d0bef57282))
* **dashboard:** let staff demo sign-in reach the venue ([cabe42e](https://github.com/stdevMac/payverge/commit/cabe42e0bf7679f1a4264476eb1a9abc13e200a9))
* **demo:** refuse outside image URLs and hide fiscal invoicing on the public demo ([b134f1c](https://github.com/stdevMac/payverge/commit/b134f1cb543fedc01c06ec16498f22336ba8e82c))
* **security:** address CodeQL hardening findings ([a626083](https://github.com/stdevMac/payverge/commit/a626083a1c0e3cd7d30b2fa0b42ae07bf8f52098))
* **security:** keep API keys out of logged outbound HTTP errors ([b360dc3](https://github.com/stdevMac/payverge/commit/b360dc3d9b71732aa6f6402fc50532fe53646356))


### Other changes

* correct guides that drifted from the tree ([1bcf1bb](https://github.com/stdevMac/payverge/commit/1bcf1bb97d57b6ebdfd3f1d44b9b928959332108))
* **deploy:** forward documented secrets and fix operator skills ([7bf82df](https://github.com/stdevMac/payverge/commit/7bf82dfbf000d20a901b4d49f6a6f04863fb2f85))
* remove pre-release private-history residue ([44d46e6](https://github.com/stdevMac/payverge/commit/44d46e6a86bc1ffe98678a6fb3d63a213ff8443a))
* **site:** confirm six landing launch gates for v1.0.0 ([338620c](https://github.com/stdevMac/payverge/commit/338620c7ee8a848fbb69fea13f629a72e117048a))
* **site:** qualify the language claim and add robots.txt ([a5bcf81](https://github.com/stdevMac/payverge/commit/a5bcf81a7001c6f6051a3cdcf75d75960bb14eed))

## [1.0.0] - 2026-10-09

Payverge's first public release under the Apache License 2.0. The system was
built for, and run in, production restaurant operations during 2025 and 2026,
and is now an open, community-maintained project. This release publishes the
whole platform as a single initial commit.

### Upgrade notes

This is the first public release, so a new install needs no upgrade step.
These notes are for **installs from before the first public release**, which
were built from pre-release source and run PostgreSQL 15.

- **PostgreSQL 18.** Release 1.0.0 runs PostgreSQL 18, and the backend
  refuses to start on any other major ("postgres major version X does not
  match the genesis baseline's Y"). Moving the data is a one-time step; see
  [docs/self-hosting/upgrades.md](self-hosting/upgrades.md#postgresql-18).
  - *Installer installs:* fetch the 1.0.0 `deploy/install.sh` first (an
    older local copy has no PostgreSQL 18 guard), then upgrade with it.
    Keep a copy of the deploy files you run now first: versions before 1.0.0
    cannot be downloaded, and the installer keeps the cached images as
    `<version>-pg15` because a pre-release install pinned to 1.0.0 shares
    that tag. It
    stops before starting anything and names `deploy/upgrade-postgres.sh`,
    which dumps with 15, restores into 18 in a new `pgdata` volume and checks
    every table's row count. The 15 data stays in the `db` volume, so rolling
    back means returning to the previous deploy files and that volume (or the
    pre-upgrade backup). It loses what was written on 18, and the 18 copy is
    refused afterwards until it is removed.
  - *Coolify / Dokploy:* there is no automatic guard. PostgreSQL 18 refuses to
    start on the old data (nothing is overwritten), so dump with the previous
    version first and restore after deploying.
  - *Render and other managed databases:* upgrade the database to 18 with the
    provider's tool before deploying. Render does not upgrade an existing
    database when `deploy/platforms/render/render.yaml` changes.

### Security

- **Email log provider.** `EMAIL_PROVIDER=log` no longer writes message content
  (reset links, login codes, invitation links) to the server log. Content is
  logged only with `EMAIL_LOG_CONTENT=true`, which production startup refuses
  unless explicitly allowed.
  The forgot-password, staff-login and MCP invite copy no longer claims the
  link or code is in the log.
- **Access log and observability.** Query secrets and capability tokens in URL
  paths (bill, guest table, space-scan, reservation codes) are redacted by route
  template, with a raw-path fallback for unmatched routes. Error logs, error
  ingest context and auth log lines use the route template instead of the raw
  path. The health detail token is accepted only as a Bearer header.
- **SSE connection caps.** Guest table and AI-waiter streams are limited per IP
  and in total, and refuse with a shared `sse_connection_limit` error.
- **Admin MCP allowlist.** The MCP admin token reaches only allowlisted routes,
  and blank or variant `error_log` sources are treated as untrusted.

### Platform

- **Guest experience over QR codes.** Digital menus with allergen information,
  table ordering, live bill view, bill splitting and tipping, public restaurant
  storefront pages, reservations, and delivery ordering with order tracking.
- **21 guest locales**, plus an operator console in English and Spanish
  (including an Argentine Spanish variant), all declared in a single locale
  registry and checked for parity in CI.
- **Owner and staff console.** Overview dashboard, menu management, tables and
  floor spaces, bills, cash register and counter service, a kitchen display
  system (KDS), receipt printing, reservations with a waitlist,
  delivery zones and drivers, inventory with recipe-based stock deductions,
  CRM, loyalty, staff scheduling with role-based permissions, accounting and
  analytics.
- **Fiscal invoicing** for Argentina (ARCA) for venues that need it.
- **Notifications** by email, Telegram and Web Push.

### AI in restaurant operations

- **AI waiter**: answers guests in their language, grounded in a snapshot of
  the current menu, with tool calls validated on the server.
- **Operations assistant and director console** for owners. The director
  console works in four steps: propose, preview, apply, and undo.
- **Guardrails** for personal data, allergens and alcohol; a **cost ledger**
  with daily spend caps per business, for guest traffic and for the whole
  instance, on by default whenever an LLM provider is configured; and an
  **offline evaluation harness** with recorded fixtures.

### Payments

- A payment plugin system with **Stripe**, **PayPal** and **MercadoPago**.
  Webhooks are signature-verified and fail closed.
- **USDC payments** verified on-chain, where one transfer can settle at most
  one bill.
- Server-authoritative amounts, with money stored as integer cents. Refunds
  are owner-only by default. Staff who are given refund rights must also enter
  a manager PIN, and an `Idempotency-Key` header deduplicates retried refund
  requests.

### Payment and fiscal safeguards

- Provider reversals are bound to the provider recorded on the payment and fail
  closed on a payment lookup error; cross-tenant reversals are rejected and a
  return capture already on the ledger is never refunded twice.
- A refund, dispute or reversal webhook that arrives before its payment is
  recorded is no longer acknowledged and lost: it gets `503` with
  `Retry-After` and is applied once the payment lands. After 48 hours it is
  acknowledged with a warning and a payment-review alert, including when the
  provider stops redelivering (a background sweeper expires it). Disputes are
  covered even though Stripe and PayPal dispute payloads name no bill
  (Stripe, PayPal, MercadoPago).
- Fiscal jobs: attempt recording is a compare-and-set on the lock holder and is
  serialized per series, stale same-number voucher claims are superseded, the
  attempted-voucher reconcile fails closed on AFIP errors, and credit notes of
  credit notes are refused.

### Self-hosting

- Runs with **zero third-party accounts**. A production instance starts with
  only `PUBLIC_URL`, `DB_PASSWORD`, `JWT_SECRET_KEY` and `PLUGIN_SECRET_KEY`
  set, plus `ADMIN_EMAIL` and `ADMIN_PASSWORD` for the first platform admin.
  Everything else is an optional integration that switches on when it is
  configured. The guides in [docs/self-hosting/](self-hosting/) cover each one.
- Object storage is optional: uploads go to a local volume by default
  (`STORAGE_DRIVER=local`). `STORAGE_DRIVER=s3` uses any S3-compatible
  service (AWS S3, MinIO, Cloudflare R2, Garage) through `S3_ENDPOINT`.
- Email delivery is optional: with no provider configured, transactional mail
  is written to the server log (`EMAIL_PROVIDER=log`). SMTP, Resend and
  Postmark send real mail.
- Payment providers and LLM providers are optional. The AI features work with
  OpenRouter or any OpenAI-compatible endpoint (`LLM_BASE_URL`), such as
  Ollama, vLLM or LiteLLM. USDC verification uses the public Base RPC unless
  `RPC_URL` points somewhere else.
- There is no subscription billing: every business has every feature, with
  no plans, trials or paywalls. The only lock on a business is the server
  administrator's suspend or close.
- `REGISTRATION_MODE` controls sign-up: `invite` (the default), `open` or
  `closed`. The first platform admin is created on boot from `ADMIN_EMAIL`
  and `ADMIN_PASSWORD`, and the server binary has `admin` and `demo`
  maintenance commands.
- A production preflight refuses to start with unsafe or incomplete
  configuration, such as placeholder or published secrets, and checks each
  optional integration only when it is configured.
- Instance identity and branding (`PRODUCT_NAME`, `LOGO_URL`, `SUPPORT_EMAIL`
  and related settings) come from configuration, and the public
  `/api/v1/instance` endpoint serves them.
- **One-line installer.** `deploy/install.sh` is attached to every GitHub
  release as install.sh. It downloads that release's deploy files, checks
  them against `SHA256SUMS`, writes `.env` with fresh random secrets, starts the stack and
  prints the first admin's one-time password. The same script upgrades an
  install (`--version X.Y.Z`) after taking a backup set.
- **Production deploy files** in [deploy/](../deploy/README.md): a Docker
  Compose stack in which Caddy obtains and renews Let's Encrypt certificates
  automatically (or uses its own CA for a `localhost` trial), PostgreSQL has
  no route out, and nightly database and upload backups run by default.
  Optional profiles add off-site copies through rclone, MinIO storage and a
  one-shot restore job, and a Cloudflare variant of the Caddy configuration
  is included.
- **Release images** on GitHub Container Registry,
  ghcr.io/stdevmac/payverge-backend and ghcr.io/stdevmac/payverge-frontend,
  for linux/amd64 and linux/arm64, with SBOM and SLSA provenance
  attestations and keyless cosign signatures. They
  carry no deployment-specific settings, so one image serves any domain.
- The root `docker-compose.yml` builds the same stack from source for
  development and for testing changes.
- An empty database bootstraps from a genesis schema and then applies numbered
  migrations on startup.
- Optional WhatsApp support is available only in builds made with
  `-tags whatsapp`. Those builds include GPL-3.0 code and are never published
  as release images.

### For developers and agents

- A monorepo with more than 8,000 Go test functions and 10,000 Jest test
  cases, plus Playwright end-to-end specs.
- `AGENTS.md`, `CLAUDE.md` and compact code maps, so coding agents can work in
  the codebase safely.
- `tools/payverge-admin-mcp`: a Model Context Protocol (MCP) server for
  administering an instance from MCP-compatible agents.
- Agent playbooks in `.claude/skills/` for self-hosters and contributors:
  `setup-and-deploy`, `upgrade`, `troubleshoot`, `configure-restaurant`,
  `rebrand`, `add-locale` and `add-payment-integration`.

### Project

- Licensed under Apache-2.0. Contributions use the Developer Certificate of
  Origin; there is no CLA. The Payverge name and logo are covered by
  `TRADEMARKS.md`.
- Community files: contributing guide, code of conduct, security policy,
  support guide, issue and pull request templates, and governance and release
  documentation.

[1.0.0]: https://github.com/stdevMac/payverge/releases/tag/v1.0.0
