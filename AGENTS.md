# Payverge — Agent Guide

This document is the single source of truth for AI coding agents (and the humans working with them) on the Payverge codebase. Read it before making any non-trivial change. Payverge was built largely through agentic development; the rules below are the ones that kept agent-written changes safe in production, and most of them are enforced by tests.

> **New here?** Start with `docs/ONBOARDING.md` for a 2-minute scannable orientation, then return to this guide for depth.
>
> **Need token-lean structure refs?** See `docs/CODEMAPS/` — `architecture.md`, `backend.md`, `frontend.md`, `data.md`, `dependencies.md`. Maintained by hand; update them after major route or schema changes.

---

## 1. Project Overview

**Payverge** is an AI-powered restaurant management platform that bridges traditional hospitality operations with modern automation and Web3 payments. It provides:

- **AI Waiter** — autonomous guest service with multilingual support
- **Director Console** — strategic analytics and proactive insights for owners
- **Digital Menus & QR Ordering** — dynamic menu management with AI-generated images
- **POS & Kitchen Display System** — order management and kitchen workflows
- **Table Reservations** — booking system with waitlist management
- **Delivery Management** — driver tracking and delivery zones
- **Inventory & Recipes** — stock tracking with recipe-based deductions
- **CRM** — customer relationship management with tags and notes
- **USDC Payments** — on-chain USDC transfers verified by the backend (no custom Payverge contracts)
- **Traditional Payments** — Stripe, PayPal and MercadoPago plugins
- **Fiscal Receipts** — ARCA/AFIP electronic invoicing for Argentina
- **Staff Management with RBAC** — role-based access control per business
- **Multi-currency & Multi-language** — automated exchange rates and translations

The codebase lives in a monorepo with three main workspaces:

| Workspace | Technology | Purpose |
|-----------|-----------|---------|
| `backend/` | Go 1.26 + Gin (Go 1.26.6 toolchain) | REST API, business logic, blockchain integration |
| `frontend/` | Next.js 15 + React 18 | Web application (App Router) |
| `docs/` | Plain Markdown (no build step) | Self-hosting, AI, architecture, ADRs, runbooks, codemaps |
| `deploy/` | Docker Compose + Caddy + shell | Self-hosting stack and `install.sh` |
| `tools/` | Node / shell | Admin MCP server, public-repo export tooling, screenshot extension |

---

## 2. Technology Stack

### Backend
- **Language**: Go 1.26 (`go 1.26.0` in backend/go.mod); repository and release toolchain pinned to Go 1.26.6
- **Framework**: Gin web framework
- **ORM**: GORM with PostgreSQL driver
- **Migrations**: `golang-migrate/migrate/v4` (versioned SQL files in `backend/migrations/`)
- **Blockchain**: `go-ethereum` for EVM interactions
- **Auth**: JWT (`golang-jwt/jwt/v4`), bcrypt for passwords, SIWE for wallets
- **Email**: the API providers are Resend and Postmark, chosen with `EMAIL_PROVIDER` (Resend is inferred from `RESEND_API_KEY`). With nothing configured the provider is `log` (written to the backend log), and `smtp` works with any relay; see `docs/self-hosting/email.md`
- **Telegram**: `go-telegram-bot-api/telegram-bot-api/v5`
- **WhatsApp**: `go.mau.fi/whatsmeow`, compiled only with `-tags whatsapp` (that binary is GPL-3.0-encumbered; the default build is not)
- **AI**: OpenRouter with Gemini model IDs behind `internal/llm` by default; any OpenAI-compatible endpoint via `LLM_BASE_URL` (Ollama, vLLM, LiteLLM). AI is optional
- **Storage**: local disk by default (`STORAGE_DRIVER=local`, served under `/media`), or S3-compatible via AWS S3 SDK v2 (dual bucket: public + protected)
- **Metrics**: Prometheus client + PostHog
- **Scheduling**: `robfig/cron/v3`
- **Testing**: `stretchr/testify`, race detector enabled

### Frontend
- **Framework**: Next.js 15.5 with App Router
- **Language**: TypeScript 5.9
- **Styling**: Tailwind CSS 3.4 + NextUI 2.4
- **State**: Zustand + React Query (`@tanstack/react-query`)
- **Forms**: Zod
- **Blockchain**: Wagmi ^3.7 + Viem 2.45 (no direct `ethers` dependency — older docs referencing "Ethers 6" are stale)
- **Charts**: Chart.js via react-chartjs-2
- **Animations**: Framer Motion
- **i18n**: Custom translation system (not next-intl — it has been removed from `package.json`). `make install-hooks` runs guest+operator locale parity + `i18n:check`; CI also runs the hardcoded-string scan.
- **Testing**: Jest 30 + React Testing Library + jsdom

### Infrastructure
- **Reverse Proxy**: Caddy (stock image; self-host config and edge test under `deploy/` and `scripts/ci/deploy-caddy-edge_test.sh`)
- **Containerization**: Docker + Docker Compose
- **CI/CD**: GitHub Actions (`.github/workflows/ci.yml`, `e2e.yml`, `acceptance.yml`, `codeql.yml`, `scorecard.yml`, `release.yml`)
- **Node Version**: 22.22.0 for repository, CI, and release builds (package minimum remains >= 20.18.0)

---

## 3. Repository Structure

```
payverge/
├── backend/
│   ├── cmd/app/
│   │   └── main.go              # Application entry point; all routes wired here
│   ├── internal/
│   │   ├── auth/                # Email/OAuth authentication (new system)
│   │   ├── blockchain/          # EVM client wrapper
│   │   ├── config/              # Config validation and test helpers
│   │   ├── crm/                 # Customer relationship management
│   │   ├── database/            # GORM models, migrations runner, DB wrapper
│   │   ├── emails/              # Resend and Postmark providers, SMTP, log sink, templates
│   │   ├── events/              # SSE (Server-Sent Events) handler
│   │   ├── handlers/            # HTTP handlers for non-business domains
│   │   ├── health/              # Health check endpoints
│   │   ├── jobs/                # Background job schedulers
│   │   ├── logger/              # Structured logging (logrus)
│   │   ├── management/          # Admin management utilities
│   │   ├── metrics/             # Prometheus + PostHog
│   │   ├── middleware/          # CORS, rate limiting, validation, security headers, RBAC
│   │   ├── models/              # Shared data structures (legacy, prefer database/)
│   │   ├── notifications/       # Notification dispatcher
│   │   ├── plugins/             # Plugin registry: payments (Stripe, PayPal, MercadoPago) + Telegram, Trustpilot
│   │   ├── server/              # Core HTTP handlers, auth middleware, business logic
│   │   ├── services/            # Business services (AI, analytics, delivery, etc.)
│   │   ├── session/             # Token revocation store
│   │   ├── structs/             # DTOs and shared types
│   │   ├── telegram/            # Telegram bot dispatcher
│   │   └── tests/               # Integration tests
│   ├── migrations/              # golang-migrate SQL files (000001_*.up.sql, etc.)
│   ├── schema/genesis/          # Genesis baseline schema for empty databases
│   ├── email/                   # HTML email templates
│   ├── Dockerfile
│   ├── Makefile
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── app/                 # Next.js App Router pages: (home)/ is /, (shop)/ holds /dashboard; b/, t/, business/, admin/
│   │   ├── api/                 # API client modules (one file per domain, with *.test.ts)
│   │   ├── components/          # React components (organized by domain)
│   │   ├── hooks/               # Custom React hooks
│   │   ├── store/               # Zustand stores
│   │   ├── contexts/            # React contexts (auth, toast, translation)
│   │   ├── i18n/                # Translation messages (en/, es/) + providers
│   │   ├── types/               # TypeScript type declarations
│   │   ├── utils/               # Utility functions
│   │   ├── lib/                 # Shared libraries
│   │   ├── config/              # Frontend configuration
│   │   ├── constants/           # Constant values
│   │   ├── schemas/             # Zod/Yup validation schemas
│   │   ├── styles/              # Global styles
│   │   ├── __tests__/           # Jest tests (component-level)
│   │   └── middleware.ts        # Next.js middleware: locale prefixes, CSP, guest table/storefront lookups, invite links on / → /dashboard, bare-path redirects
│   ├── public/                  # Static assets
│   ├── Dockerfile
│   ├── next.config.mjs
│   ├── tailwind.config.ts
│   ├── jest.config.js
│   └── eslint.config.mjs
├── docs/                        # Plain Markdown tree — no build step, no package.json
│   ├── adr/ · architecture/     # Decision records and architecture pages
│   ├── agents/ · ai/            # Agentic layer and in-app AI docs
│   ├── self-hosting/            # Per-topic operator docs (admin, storage, email, AI, ...)
│   ├── CODEMAPS/                # Token-lean architecture refs (maintained by hand)
│   ├── design/                  # UI standards (sidebar standard / gap list)
│   ├── fiscal/                  # AFIP-ARCA + e-invoicing architecture
│   ├── improvements/            # Numbered improvement backlog + README index
│   ├── performance/             # Per-stream perf-gate evidence
│   ├── product/                 # Product/feature notes
│   ├── plans/                   # Design specs and implementation plans
│   └── runbooks/                # Operational runbooks (indexed by catalog.json)
├── evals/                       # Prompt/AI evaluation harnesses
├── scripts/                     # Repo-level scripts (incl. hooks/, ci/)
├── site/                        # Static landing page (no build step; checks: node site/tools/check.mjs)
├── deploy/                      # Self-host stack: compose, Caddy, install.sh, backups
├── infra/                       # Backup tooling (infra/backup)
├── config/dashboard-tabs.json   # Read at runtime by backend ops guides — must stay at root
├── locales/registry.json        # i18n source of truth for codegen — must stay at root
├── tools/                       # Standalone tools (screenshot extension, admin MCP, oss-export public-repo exporter)
├── videos/                      # Media production workspace (gitignored, local-only)
├── summary.md                   # Benchmark/perf log required by the Backend Performance Gate
├── docker-compose.yml           # Local compose (postgres + backend + frontend)
├── package.json                 # Root eval scripts only (ai:eval*) — not a workspace root
├── .claude/skills/              # Shared operator playbooks for coding agents (tracked)
├── .github/                     # Workflows, community files (CONTRIBUTING, SECURITY, ...)
└── .env.example                 # Development environment template
```

### 3.1 Where new files go

The repository root is a **closed set** of tracked files. The list is
exhaustive and verifiable — `git ls-files -- ':(glob)*' | grep -v /` must
return only these, and `scripts/ci/d10_hygiene_contract.test.mjs` enforces the
allow-list (which also names the community files such as `CONTRIBUTING.md`,
`SECURITY.md` and `CHANGELOG.md`):

| Root file | Why it is allowed to be here |
|---|---|
| `README.md`, `CLAUDE.md`, `AGENTS.md` | Entry-point docs; tooling reads them by fixed path |
| `Makefile` | Top-level task runner |
| `LICENSE`, `NOTICE`, `TRADEMARKS.md` | Licence text, attributions and trademark policy |
| `docker-compose.yml` | Local full-stack compose |
| `package.json` | Root workspace scripts (`ai:eval*`) |
| `promptfooconfig.yaml` | Referenced as `-c promptfooconfig.yaml` by those scripts |
| `.env.example` | Canonical env template; compose reads the root `.env` |
| `.gitignore` | — |
| `.nvmrc` | Node version pin (22.22.0) picked up by nvm/CI |
| `summary.md` | Benchmark log written by the Backend Performance Gate |

Two untracked-but-expected files also live at root, both tool-managed and
gitignored: `.env` and `skills-lock.json`.

**Do not add a file to the root.** If something feels like it belongs there, it
belongs in one of these:

| What you are writing | Where it goes |
|---|---|
| Audit / review / findings report | Not in the repo — file an issue or keep it local |
| Design spec, implementation plan | Not in the repo — put it in the GitHub issue or pull request description |
| Runbook, deploy or ops procedure | `docs/runbooks/` (add it to `docs/runbooks/catalog.json`) |
| Perf-gate evidence (benchmarks) | `summary.md`, or `docs/performance/` for a scoped stream |
| UI/design standard | `docs/design/` |
| Product or feature write-up | `docs/product/` |
| Backlog item | A GitHub issue (`docs/improvements/` is a private-repo backlog and is not exported) |
| Scratch notes, TODO lists, dumps | Nowhere in the repo — use the session scratchpad |
| Screenshots, renders, video, audio | `videos/` (gitignored); never the repo root |
| Generated tool output | A gitignored path, and add the ignore rule in the same commit |

Rules that keep it that way:

1. **Name files for their content, not their moment.** `e-stuff.md`,
   `to-check.md` and `improvements.md` all had to be renamed later. Prefer
   `fiscal-architecture.md`; date-stamp anything point-in-time
   (`frontend-uiux-audit-2026-07-13.md`).
2. **A file is either tracked or ignored — never both.** Before committing
   generated output, add its ignore rule in the same commit. To check the whole
   tree: `git ls-files | git check-ignore --no-index --stdin` must print
   nothing. (`--no-index` is required; git skips indexed files without it.)
3. **Never `git add -A`.** It has swept multi-GB media into commits here. Stage
   explicit paths.
4. **Per-tool AI scratch directories are local.** Whatever directory your
   agent or editor keeps its state in stays out of git (`.gitignore` lists the
   common ones). `.claude/` is the only shared agent directory, and the one
   tracked part of it is `.claude/skills/<name>/SKILL.md` (shared, plain-Markdown
   operator playbooks; see `docs/agents/README.md`). Never commit a symlink
   into an ignored directory; it is broken for every other clone.
5. **Moving a document means fixing its inbound links.** Grep for the old
   basename before you finish. Leave historical plans and specs pointing at the
   old names — they are a record of what was true then.
6. **Delete by moving, not by erasing.** Media goes under `videos/`; documents
   go into `docs/`. Only genuine artifacts (tool dumps, render
   byproducts) get removed outright.

---

## 4. Build & Run Commands

### Backend
```bash
cd backend

# Install dependencies
go mod download

# Run
make run                    # go run ./cmd/app
make build                  # go build -o bin/app ./cmd/app

# Test
make test                   # go test -v -race ./...
make test-unit              # -short suite, no Docker
make test-integration       # Postgres-backed suites from scripts/ci/go-pg-packages.sh; needs Docker or TEST_DATABASE_URL
make test-coverage          # Coverage report via scripts/test-coverage.sh
make quick-test             # go test ./... (no race detector)

# Dev helpers
make fmt                    # go fmt ./...
make lint                   # golangci-lint run
make clean                  # Remove bin/, coverage/, test cache
```

The backend accepts configuration via **CLI flags** (not just env vars). Key flags:
- `--db-host`, `--db-port`, `--db-user`, `--db-password`, `--db-name`, `--db-sslmode`
- `--production`
- `--rpc-url`
- `--aws-access-key`, `--aws-secret-key`, `--s3-bucket`, `--s3-endpoint`
- `--telegram-token`, `--email-provider`, `--email-api-key`

Every flag resolves **flag first, then env var** (a resolution block right after
`flag.Parse()` in `cmd/app/main.go`), so either source works for a direct CLI run.

**Compose must never pass a secret as a flag.** Command-line arguments are
readable from `/proc/<pid>/cmdline` by any process on the host and are printed
verbatim by `docker inspect`, so argv is not a private channel. These six live
in `environment:` in every Compose file and must never reappear in a `command:`
block:
`--db-password`, `--telegram-token`, `--aws-secret-key`, `--aws-protected-secret-key`,
`--email-api-key`, `--google-translate-api-key`.
Non-secret identifiers (`--aws-access-key`, `--email-provider`, `--from-email`,
hosts, buckets, regions) stay in `command:`.

### Frontend
```bash
cd frontend

# Install dependencies
npm install

# Development
npm run dev                 # Next.js dev server (port 3000)

# Build
npm run build               # Production build
npm run typecheck           # tsc --noEmit (clears .next/types first)
npm run lint                # eslint src/
npm run format              # prettier --write "src/**/*.tsx"

# Test
npm run test                # jest
npx jest --watchman=false --runInBand   # CI mode (used in GitHub Actions)
```

### Docs
```bash
# Plain Markdown tree under docs/ — no package.json, no build step, no dev server.
# Edit .md files directly. Runbooks live in docs/runbooks/; codemaps in docs/CODEMAPS/.
```

### Docker (Full Stack)
```bash
# Development stack, empty local DB, built from this checkout:
docker compose --env-file .env up -d --build

# The compose file defines:
# - postgres:15-alpine (internal only; no host port)
# - backend on 127.0.0.1:8080
# - frontend on 127.0.0.1:3000
```

Production self-hosting uses `deploy/docker-compose.yml` (Caddy, release images, backups) and `deploy/install.sh`; see `deploy/README.md`.

---

## 5. Database & Migrations

- **Database**: PostgreSQL 15
- **ORM**: GORM v2 with `gorm.io/driver/postgres`
- **Migration Tool**: `golang-migrate/migrate/v4`
- **Migration Files**: `backend/migrations/NNNNNN_*.up.sql` + `.down.sql` (numbering restarted at the open-source squash, where the genesis baseline is version 0; no numbered migration yet, so the next migration is `000001`)
- **Schema source of truth**: the **genesis baseline** (`backend/schema/genesis/current_schema.sql`, embedded via `schema/genesis/embed.go`) plus **versioned SQL migrations** in `backend/migrations/`. On an **empty** database, `cmd/app/main.go` bootstraps from the genesis dump (`database.BootstrapGenesisSchema`); established versioned databases apply only pending numbered migrations. A non-empty database without a usable migration ledger is treated as inconsistent and startup refuses it rather than force-baselining an unverifiable shape.

**Schema ownership (important)**:
- **Genesis baseline + numbered SQL migrations are the schema source of truth.** All **new** schema changes **must** be a numbered migration in `backend/migrations/` (`.up.sql` + `.down.sql`).
- Production startup executes no GORM AutoMigrate or schema-changing `RunEnsure*` helpers. SQL-only artifacts belong directly in paired numbered migrations. GORM `AutoMigrate` appears only in `_test.go` SQLite fixtures (each test migrates the models it needs); Postgres tests and benchmarks build their schema from the genesis baseline (`internal/testperf/genesisdb.Start` → `database.BootstrapGenesisSchema`).
- **No new production `.AutoMigrate(` call sites.** `TestProductionStartupHasNoAdHocDDL` gates the application entrypoint; `TestAutoMigrateSourceGate` fails on any `.AutoMigrate(` in non-test Go source (its allowlist is empty). Add a numbered SQL migration instead.
- Startup refuses schema drift: after `VerifySchemaAtVersion`, `VerifySchemaFingerprint` requires the Postgres major and the live schema fingerprint to match `postgres_major` and `fingerprint_sha256` in `backend/schema/genesis/version.json` (written by `generate-genesis-schema.sh`). Integration gate `TestProductionStartup_WorksWithoutDDLPrivileges` runs the second-startup migration and verification path through a role that owns no schema objects and lacks `CREATE`.

**Running migrations**: The backend runs `database.RunMigrations(...)` on startup in `cmd/app/main.go` (fatal on failure), then verifies the schema at that version (also fatal). Genesis bootstrap failures on empty DBs are fatal. There is no GORM AutoMigrate safety net. Applied migrations are never renumbered.

**Auth schema**: Auth tables, indexes, and user columns are part of the genesis baseline. Production startup runs one idempotent data step after schema verification: `auth.MigrateExistingUsers` creates the wallet `user_auths` row for wallet users that lack one.

**Key schema domains**:
- Users, businesses, staff, invitations
- Menu categories, items, offers, bundles
- Tables, reservations, bills, bill items, orders
- Inventory items, recipes, movements
- Delivery orders, drivers, zones, settings, status history (`delivery_orders`, `delivery_drivers`, `delivery_zones`, `delivery_settings`, `delivery_status_history`)
- CRM customers, tags, notes
- Analytics events, page views
- WhatsApp meow tables
- Plugins (catalog, per-business config)

---

## 6. API Architecture

All routes are wired in `backend/cmd/app/main.go`. The API is organized under `/api/v1/` with these route groups:

| Group | Path | Auth | Purpose |
|-------|------|------|---------|
| Public | `/api/v1/` | None | Health, instance info, home (`/api/v1/home`), guest tables, currencies, reservations, AI waiter, unsubscribe, manual onboarding |
| Auth | `/api/v1/auth` | None (rate limited) | Login, register, OAuth, SIWE, password reset, email verify |
| Protected | `/api/v1/inside` | Hybrid (wallet + staff) | Business CRUD, menu, tables, bills, orders, analytics, staff, plugins |
| Customer | `/api/v1/customer` | Customer JWT for the protected routes; session-info, refresh, and logout are unauthenticated | Session info, refresh, logout, connect-business, table check-in, profile, businesses, preferences, link-wallet, delete account |
| Staff | `/api/v1/staff` | Staff token | Staff profile, logout |
| Admin | `/api/v1/admin` | Admin + rate limit | Runtime controls, stats, system health, businesses, users (detail, close, reset-password), demo, escalations, fiscal, plugins, failed webhooks, operator emails, analytics, errors |
| Webhooks | `/api/v1/webhooks` | Provider signature | Resend email events via `registerEmailWebhookRoutes` (Postmark has no webhook route), Telegram, PayPal, Stripe, MercadoPago |
| MercadoPago OAuth | `/api/v1/mercadopago` | None | Public OAuth connect callback (`GET /oauth/callback`) |
| Stripe OAuth | `/api/v1/stripe` | None | Public OAuth connect callback (`GET /oauth/callback`) |

**Middleware stack** (applied in order):
1. `gin.Recovery()`
2. `middleware.RequestID()`
3. `middleware.CORS(allowedOrigins)` — origin validation, NOT wildcard
4. `middleware.SecurityHeaders()` — HSTS, CSP, X-Frame-Options, etc.
5. `middleware.InputValidation()` — prevents NoSQL injection, path traversal
6. `middleware.JSONSizeLimit(10MB)`
7. `rateLimiter.RateLimit()` — 300 req/min prod, 1200 req/min dev
8. `PrometheusMiddleware()` — request metrics

**Auth middleware variants**:
- `AuthenticationMiddleware()` — wallet-based JWT
- `AuthenticationAdminMiddleware()` — admin-only
- `StaffAuthenticationMiddleware()` — staff token
- `CustomerAuthenticationMiddleware()` — customer JWT
- `HybridAuthenticationMiddleware()` — wallet OR staff
- `RoleBasedAccessMiddleware("permission:key")` — RBAC enforcement (AND semantics — user must hold every listed permission)
- `RequireOperationalBusiness()` — admin lifecycle lock (403 `business_suspended` / `business_closed`)

### Web routes (frontend)

| URL | Serves |
|---|---|
| `/` | The published venue: `PRIMARY_VENUE`, else the only published one, else a directory of `/b/<slug>` links, else a redirect to `/dashboard` (`frontend/src/app/(home)/page.tsx`, `GET /api/v1/home`, `docs/api/home.md`) |
| `/b/<slug>` | A venue's public page (published when `business_page_enabled`, `is_active` and a `custom_url` are set) |
| `/t/<code>` | A table: ordering, plus `/menu`, `/bill`, `/signin`, `/profile` |
| `/dashboard` | Operator sign-in and venue list |
| `/business/<id>/dashboard` | A venue's dashboard (`?tab=`) |
| `/admin` | Platform admin |

The project's landing page (payverge.io) is the separate static site in `site/` (see `site/README.md`), not part of the app.

### Kitchen/orders toggle semantics

- `Business.KitchenEnabled` + `Business.OrdersEnabled` gate **guest** ordering endpoints only (`guestOrderingEnabled` in `internal/server/guest_handlers.go`). Operator order routes intentionally ignore the toggle — staff can always enter orders (phone orders, corrections) while guest self-ordering is off.
- `ToggleKitchenAndOrders` (`internal/database/business.go`) always writes both flags to one value; the columns are separate for historical reasons but cannot diverge through the API. Treat them as a single switch.

### Thermal printer subsystem (Sprint 1)

- **Tables** (genesis baseline): `printers`, `print_jobs`, `print_audit_log`.
- **Service** `backend/internal/services/print/` composes `Router` (role-based printer lookup), `Queue` (persistence + status transitions), `Service.Enqueue/MarkPrinted/Cancel/Reprint`, plus `formatters/{bill,receipt}.go` with 80mm/58mm `html/template` output and golden tests under `testdata/`.
- **Handlers** `backend/internal/handlers/printer_handlers.go` (CRUD + test-print) and `print_job_handlers.go` (list / create / mark-printed / cancel / reprint) mounted under `/api/v1/inside/businesses/:id/printers` and `.../print/jobs`.
- **RBAC perms** (`backend/internal/server/rbac.go`): `printers:read`, `printers:write`, `print:bill`, `print:receipt`. Defaults — Manager: all four; Server: print:bill + print:receipt; Host: print:bill; Kitchen: none (kitchen tickets auto-fire and are out of Sprint 1 scope).
- **Auto-receipt**: `enqueueReceiptForPaidBill` in `plugin_handlers.go` fires at every `BillStatusPaid` transition (PayPal return, webhook, internal markTrackedPluginPaymentConfirmed). Failures log and continue — printing must not block payment settlement.
- **Frontend** lives at `frontend/src/components/business/printers/` (`PrintersSettings`, `AddPrinterWizard`, `useIframePrint`). Page route: `/business/[businessId]/settings/printers`. API client: `src/api/print.ts` (raw fetch, not axios). **No feature flag** — visibility is permission-gated: settings link requires `printers:read` (owners always); print actions require `print:bill` (owners always). Operator copy lives in `frontend/src/i18n/messages/{en,es}/printers.json`.
- **Sprint 2 scope** (deferred): CloudPRNT transport + ESC/POS builder, kitchen/bar/void/modify formatters, `menu_categories.printer_role`, fallback chains, audit-log UI.

### Fiscal compliance subsystem (foundation)

- **Tables** (genesis baseline): `business_fiscal_settings`, `fiscal_receipts`, `fiscal_jobs`, `fiscal_audit_events`.
- **Service** `backend/internal/fiscal/` owns settings, provider contracts, country policies, idempotent issuance jobs, and receipt records, with audit-event schema/model groundwork.
- **Provider boundary**: fiscal providers are not payment plugins. Payment plugins collect money; fiscal providers issue official documents for paid bills.
- **Argentina v1** lives under `backend/internal/fiscal/providers/ar/` with ARCA policy, QR generation, WSFE mapping, and a mockable provider client.
- Fiscal providers are pluggable via the provider interface; only AR/ARCA ships.
- **RBAC perms** (`backend/internal/server/rbac.go`): `fiscal:read`, `fiscal:write`, `fiscal:issue`, `fiscal:retry`, `fiscal:export`.

---

## 7. Code Style Guidelines

### Go (Backend)
- Standard Go formatting (`go fmt`)
- Use `golangci-lint` for linting
- Tests use `stretchr/testify` suites; table-driven tests preferred
- File naming: `domain_action.go` for handlers, `domain.go` for services, `domain_test.go` for tests
- Error handling: use `logger.Logger.Warnf/Errorf` for operational errors; `Fatal` only for startup failures
- Context: pass `*gin.Context` through handlers; use `context.Background()` for background jobs
- Package structure: domain-based under `internal/` (not layer-based)

### TypeScript / React (Frontend)
- **ESLint**: `next/core-web-vitals` + `@typescript-eslint`
- **No console logs**: `no-console` rule enforces only `warn` and `error`
- **Color palette enforcement**: Off-palette Tailwind colors (slate, blue, indigo, etc.) are banned by ESLint. Use the custom tokens:
  - `brand` (teal family: `#1a6b6a`)
  - `ink` (warm grayscale)
  - `warm` (warm neutrals)
  - `emerald`, `amber`, `rose` for semantic colors
- **No inline hex colors**: Add to `tailwind.config.ts` first
- Legal third-party brand icons (Google, Telegram, WhatsApp) go under `src/components/legal-brand-icons/` with a per-file `eslint-disable` comment
- Component naming: PascalCase
- Hook naming: `useCamelCase`
- API client files: `src/api/domain.ts` with corresponding `domain.test.ts`

### CSS / Tailwind
- Custom font sizes defined in `tailwind.config.ts` (display-2xl through label)
- Dark mode: `darkMode: "class"` (not media query)
- Custom animations: `subtle-bounce`, `slideFromLeft`, `slideToRight`
- Theme transitions: `.theme-transitioning` class applied during theme toggles

---

## 8. Testing Instructions

### Backend Tests
```bash
cd backend

# All tests with race detection
make test

# -short suite, no Docker
make test-unit

# Postgres-backed suites from scripts/ci/go-pg-packages.sh; needs Docker or TEST_DATABASE_URL
make test-integration

# Specific package
go test -v -race ./internal/server/...
go test -v -race ./internal/middleware/...

# With Postgres integration tag
go test -tags integration ./internal/server/...
```

**Test configuration**: `internal/config/test_config.go` provides test helpers. Integration tests expect `TEST_DATABASE_URL` (e.g., `postgres://test:test@localhost:5432/test?sslmode=disable`).

### Frontend Tests
```bash
cd frontend

# Run all tests
npm run test

# CI mode (no watchman)
npx jest --watchman=false --runInBand

# Coverage thresholds (enforced in jest.config.js):
# - branches: 74
# - functions: 60
# - lines: 74
# - statements: 74
```

**Jest configuration notes**:
- `testEnvironment: 'node'` (not jsdom by default — some tests may need env override)
- Module alias: `@/` maps to `src/`
- Canvas mocked via `<rootDir>/test/mocks/canvas.js`
- Transform ignore patterns allow ESM packages: `framer-motion`, `@nextui-org`, `@react-aria`, `@react-stately`

---

## 9. Security Considerations

### Authentication & Authorization
- **Hybrid auth**: Wallet (SIWE/JWT), Email/Password (bcrypt), OAuth (Google), Staff tokens
- **Token revocation**: Session store in PostgreSQL (`session.GlobalStore`) with hourly cleanup
- **RBAC**: Granular permission keys (e.g., `menu:write`, `bills:close`, `staff:invite`). The business owner has every permission. Staff roles are `manager` > `server` > `host` > `kitchen`, each with default permission keys in `StaffRolePermissions` (`backend/internal/server/rbac.go`). Platform admin (`/api/v1/admin`) is separate from business roles.
- **Business lock**: there are no plans. `RequireOperationalBusiness()` returns `403 {"code":"business_suspended"}` or `403 {"code":"business_closed"}` when the server administrator suspended or closed the business; guest routes answer `403 business_unavailable`
- **Admin bootstrap**: `ADMIN_EMAIL` (+ optional `ADMIN_PASSWORD`) creates the first platform admin on startup; the `admin` CLI subcommand resets passwords; `REGISTRATION_MODE` (`invite` by default) controls who may sign up. See `docs/self-hosting/admin.md`.

### Input Validation
- `middleware.InputValidation()` blocks NoSQL injection attempts (`$ne`, `$gt`, etc.) and path traversal (`../`)
- `middleware.JSONSizeLimit(10MB)` prevents large JSON payloads
- `middleware.SecurityHeaders()` sets CSP, HSTS, X-Frame-Options, Referrer-Policy, Permissions-Policy
- File uploads: `MaxMultipartMemory = 100MiB`

### Rate Limiting
- General: 300 req/min (prod), 1200 req/min (dev)
- Auth endpoints: 10 req/min, burst 3
- Public forms (email unsubscribe): 5 req/min, burst 2
- Error logs: 30 req/min
- Admin routes: 60 req/min, burst 20
- Payment plugins: dedicated `PaymentRateLimit()`

### Secrets & Environment
- JWT secret from `JWT_SECRET_KEY` env var (fatal if missing)
- S3 credentials: dual bucket setup (public + protected)
- Metrics endpoint protected by `METRICS_TOKEN` or `METRICS_TOKENS` (comma-separated for rotation)

### CORS
- Origins validated explicitly from `PUBLIC_URL` / `ALLOWED_ORIGINS`; outside production, `localhost:3000` and `localhost:3001` are added
- CORS is handled **only by the backend** — the reverse proxy must NOT add CORS headers, to avoid duplication

---

## 10. Deployment

### Production Stack
- **Caddy** (`deploy/`) serves one host (`PUBLIC_URL`): `/api/v1/*` and `/media/*` go to the backend, everything else to the frontend (300s timeouts for AI and uploads, 15 MB bodies); TLS and edge security headers live there
- **Docker Compose** for container orchestration
- **Health checks**: Backend exposes `/api/v1/health`, `/api/v1/health/ready`, `/api/v1/health/live`

### CI/CD Pipeline (GitHub Actions)
`.github/workflows/ci.yml` runs on PRs and on `main`; the `ci-ok` job aggregates every job:
1. **Repository contracts** — hygiene, workflow and release contracts, CI helper and backup script tests, the self-host Caddy edge test and the admin MCP tests
2. **Backend** — genesis schema check, build, vet, sharded `go test -short`, Postgres integration tests, lint on new issues, the `-tags whatsapp` compile and the GPL-free default build check
3. **Frontend** — i18n validators, lint, typecheck, sharded Jest, `next build`
4. **AI offline evals and audits** — recorded-fixture eval suites and AI contract scenarios, dependency and secret scans

`e2e.yml` runs Playwright against the compose stack, `acceptance.yml` runs the weekly one-click self-host acceptance, `codeql.yml` and `scorecard.yml` do static analysis, and `release.yml` (release-please) builds multi-arch images, an SBOM and the `install.sh` release assets. A published image is not a deployment; rolling it out to a host is always a separate step.

### Frontend Build Behavior
- Browser-visible settings are runtime config, not build-time: `window.__PAYVERGE_ENV__` is rendered from the container environment by `src/config/publicConfig.ts`, so one image serves any domain (see `docs/self-hosting/frontend-config.md`). Do not add new `NEXT_PUBLIC_*` variables for runtime settings; an ESLint rule guards direct reads
- `NEXT_OUTPUT_STANDALONE=1` produces standalone output for Docker
- `NEXT_STRICT_BUILD=1` or `CI=true` enforces TypeScript and ESLint checks during build
- Local dev allows `ignoreBuildErrors` and `ignoreDuringBuilds` for faster iteration
- Console logs are stripped in production (except `warn`/`error`)

---

## 11. Key Conventions for Agents

### Before Making Changes
1. **Check for tests**: If modifying `backend/internal/server/business_handlers.go`, check `business_handlers_test.go` and `business_handlers_regression_test.go`
2. **Run relevant tests**: `go test -v -race ./internal/server/... -run "Business"`
3. **Check migrations**: Schema changes require a new `backend/migrations/0000XX_description.up.sql` and `.down.sql`
4. **Update API clients**: If you change a backend endpoint, update the corresponding `frontend/src/api/*.ts` file
5. **i18n**: New UI copy must be added to `frontend/src/i18n/messages/en/` and `es/` (and `es-ar` for operator). Guest keys go in all 21 `guest-messages` bundles. `make install-hooks` enforces guest+operator parity + `i18n:check` on commit.

### Common Patterns
- **Backend handlers**: Accept `*gin.Context`, use `database.GetDBWrapper()` or `database.GetDB()`, return JSON with `c.JSON(http.StatusOK, result)`
- **Frontend API calls**: Use the typed API modules in `src/api/`, not raw `fetch`
- **State management**: Server state → React Query; client state → Zustand; form state → React Hook Form
- **Error handling**: Backend returns structured errors; frontend uses `react-hot-toast` for notifications
- **RBAC checks**: Frontend should mirror backend permission checks to hide UI elements, but never rely solely on frontend checks

### File Naming Conventions
- Go: `snake_case.go` for files, `PascalCase` for exported identifiers
- TypeScript: `PascalCase.tsx` for components, `camelCase.ts` for utilities, `domain.test.ts` for tests
- CSS: Tailwind utilities preferred; global styles in `src/styles/` or `globals.css`
- Markdown: `kebab-case.md`, date-suffixed when point-in-time
  (`frontend-uiux-audit-2026-07-13.md`). See §3.1 for which directory it goes in.

### Creating New Files
Before adding a file, confirm it is not landing in the repository root — see
[§3.1 Where new files go](#31-where-new-files-go). Generated output must be
gitignored in the same commit that introduces it, and
`git ls-files | git check-ignore --no-index --stdin` must stay empty.

### Operator playbooks (skills)

Step-by-step playbooks for people who run Payverge. Each is one plain
Markdown file with `name` and `description` frontmatter. Claude Code loads
them automatically. Any other agent can read the file and follow it, for
example: "Read `.claude/skills/upgrade/SKILL.md` and follow it."

| Task | Skill |
|---|---|
| Install a server with Docker Compose, create the first admin, verify it | `.claude/skills/setup-and-deploy/SKILL.md` |
| Set up a restaurant: profile, hours, menu, tables, staff, payments, AI | `.claude/skills/configure-restaurant/SKILL.md` |
| Give an instance or fork its own name, logo, colour and domain | `.claude/skills/rebrand/SKILL.md` |
| Add a guest or dashboard language | `.claude/skills/add-locale/SKILL.md` |
| Add a payment provider as a code plugin | `.claude/skills/add-payment-integration/SKILL.md` |
| Upgrade with a backup, watch migrations, roll back | `.claude/skills/upgrade/SKILL.md` |
| Diagnose a server that is down or misbehaving | `.claude/skills/troubleshoot/SKILL.md` |

`docs/agents/README.md` explains how the skills fit with the in-app AI and
the admin MCP server (`tools/payverge-admin-mcp/`). A new skill goes in
`.claude/skills/<name>/SKILL.md` and gets a row here.

---

## 12. Plugin Authoring Checklist

Adding a plugin (payment provider, integration, reporting, or marketing) touches
both stacks. Work the list top-to-bottom; every item was verified against the
code paths named in parentheses. Skip the payment-only rows for non-payment
plugins.

**Backend — registration & catalog**

1. **Go package with `init()` → `plugins.RegisterPluginInitializer(name, fn)`.**
   Put the plugin under `backend/internal/plugins/<name>/`. Its `init()` calls
   `plugins.RegisterPluginInitializer("<name>", func(svc *services.PluginService) { ... })`,
   and the callback constructs the plugin and calls
   `plugins.GlobalRegistry.RegisterPlugin(instance)`. `main.go` runs every
   registered initializer once `PluginService` exists
   (`plugins.InitializeAllPlugins`), in name-sorted order — do NOT rely on a
   specific ordering beyond that (see `internal/plugins/init.go`).
2. **Blank import in `cmd/app/main.go`.** Add `_ "github.com/stdevmac/payverge/backend/internal/plugins/<name>"`
   to the import block so the package's `init()` runs. (Telegram is imported
   with an alias, not blank, only because `main.go` needs its instance for the
   webhook reply sender — most plugins use the blank form.)
3. **Catalog entry — know the source-of-truth rule.** A code plugin's DB catalog
   row is authored by the **Go registry**: `InitializeAllPlugins` calls
   `SyncPluginsToDatabase` (`internal/plugins/registry.go`), which upserts each
   registered plugin's `ConvertToDBPlugin(...)` row and **overwrites** display
   name, description, image, price, category, version, features, and config
   schema on every boot. So for a code plugin the authoritative metadata lives
   in your plugin's `Get*` methods, not in a seed literal. The seed list in
   `internal/services/plugin_init.go` (`InitializeDefaultPlugins`) is only the
   source of truth for **seed-only** plugins that ship no Go implementation —
   e.g. the crypto rails and the email reports. (Note:
   `plugin_init.go` may be under concurrent edit; treat the rule, not any line,
   as canonical.) If you add a seed-only plugin, add it to that slice. Do not
   seed placeholders for integrations that do not exist yet.
4. **Icon asset — `<name>-logo.png`.** `ConvertToDBPlugin` hardcodes
   `Image: "/images/plugins/" + GetName() + "-logo.png"`, so a code plugin's icon
   must live at `frontend/public/images/plugins/<name>-logo.png` regardless of
   what `GetImage`/seed says. (Older seed rows use bare names like `stripe.png`;
   the registry sync rewrites them to `-logo.png` on boot.)
5. **Translations.** Add `es` and `es-AR` entries for the plugin in
   `InitializePluginTranslations` (`plugin_init.go`) — `display_name`,
   `description`, `message`, `features`. Missing translations fall back to the
   English catalog fields.

**Backend — payment plugins only**

6. **Webhook route + `pluginWebhookConfigs` entry.** Register the provider's
   webhook route in the `webhookRoutes` group in `main.go`
   (`webhookRoutes.POST("/<name>", ...)`), and add an entry to
   `pluginWebhookConfigs` in `internal/handlers/plugin_handlers.go` giving the
   signature header and the ordered env-var fallbacks for the webhook secret.
   Without that entry, `resolvePluginWebhookSignature` returns
   `errPluginWebhookConfigUnknown` and the payload is treated as **unverified**
   (only acceptable for ad-hoc/test plugins — all first-party providers MUST
   have an entry).

**Frontend**

7. **`src/constants/plugins.ts` entry.** Add the backend `name` to the `PLUGIN`
   map. This is the single source of truth the FE switches on; the `PluginName`
   union gives the config-factory switch compile-time exhaustiveness. Callers
   keep tolerant `default:` branches, so an unknown backend name degrades to the
   generic UI rather than crashing.
8. **`PluginConfigFactory` switch + config component.** Add a `case PLUGIN.<name>:`
   to `src/components/business/plugins/PluginConfigFactory.tsx` returning your
   `<NameConfig>` component, and write that component next to it. Omit the case
   to fall back to `GenericPluginConfig` (fine for zero-config plugins). Config
   components receive `{ plugin, config, onConfigChange, onSave, onCancel }` and
   push edits up through `onConfigChange`; the parent PUTs the whole config blob
   to `/inside/businesses/:id/plugins/:plugin_id/config` (full replace — the
   backend restores masked secrets and preserves nothing the form omits, except
   via `restoreMaskedSecrets`).
9. **Payment plugins: `configFields.ts` descriptor + schema fixture + contract
   test case (added in commit `7b14c7e6`).** New **payment** plugins MUST add:
   a `build<Name>InitialConfig` + `<NAME>_UI_ONLY_FIELDS` export in
   `configFields.ts`; a copy of the backend config schema JSON under
   `__fixtures__/plugin-config-schemas/<name>.json`; and a case in
   `__tests__/configFields.contract.test.ts`. The contract test fails the build
   if the form omits a backend-`required` field (the plugin would 400 on every
   save and never enable) or submits a field the backend never reads (dead
   weight). Keep the fixture in sync with the plugin's `GetConfigSchema`.

**Optional interfaces worth implementing** (all in `internal/plugins/interface.go`;
implement only the ones your flow needs — they are detected by type assertion at
the call sites):

- `WebhookRequestVerifier` — provider verification that `VerifyWebhookSignature`
  can't express (custom request-level verification).
- `PaymentReturnCapturer` — redirect providers needing a server-side capture
  call after buyer approval (PayPal-style).
- `ConfigNormalizer` — inject server-owned config values / encrypt secrets
  before persist (`NormalizeConfig`).
- `PublicConfigProvider` — mask sensitive config before dashboard responses
  reach the browser.
- `AlternativePaymentTracker` — link a provider intent row to the generic
  `AlternativePayment` row created by `plugin_handlers`.
- `DetailedPaymentStatusProvider` — provider-specific metadata for non-redirect
  flows (QR/address stablecoin checkout).

**Category interfaces** (pick one to be discoverable by category fan-out):
`PaymentPlugin`, `IntegrationPlugin` (typed `SendNotification` +
`GetConnectionStatus` — enforced by a compile-time assertion in the telegram
package), `ReportingPlugin`, `MarketingPlugin`.

## 13. Useful References

- **Backend entry**: `backend/cmd/app/main.go` (all routes, all initialization)
- **Frontend entry**: `frontend/src/app/layout.tsx` (root layout, providers, metadata)
- **Database models**: `backend/internal/database/models.go` + domain files (`business.go`, `inventory.go`, etc.)
- **Middleware**: `backend/internal/middleware/`
- **API clients**: `frontend/src/api/`
- **Tailwind config**: `frontend/tailwind.config.ts` (custom colors, fonts, animations)
- **ESLint config**: `frontend/eslint.config.mjs` (strict rules, color palette enforcement)
- **Docker Compose**: `docker-compose.yml` (full stack definition)
- **Env template**: `.env.example` (all required variables)

---

*Last updated: 2026-10-06. If you change build commands, routing patterns, auth flows, or deployment configs, update this file.*
