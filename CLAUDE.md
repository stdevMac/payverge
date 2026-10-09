# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) and other coding agents working in this repository.

Payverge is developed with coding agents as a first-class workflow: agents read
this file and `AGENTS.md`, follow the engineering rules below, and use the
shared playbooks in `.claude/skills/` and the MCP tools in
`tools/payverge-admin-mcp/` (see `docs/agents/README.md`). The rules here are
the ones that kept agent-written changes safe in production: they are enforced
by tests where possible, so read the Gotchas before touching money, schema or
config.

## Project Overview

Payverge is an AI-powered restaurant management platform with Web3 payment integration. It's a monorepo with three workspaces: a Next.js frontend (`frontend/`), a Go/Gin backend (`backend/`), and a Markdown documentation tree (`docs/`), plus `deploy/` (self-hosting compose, Caddy, installer), `site/` (static landing page) and `tools/` (export tooling and the admin MCP server). On-chain interactions are limited to USDC verification (backend) and standard ERC-20/Dynamic-wallet flows (frontend) — no custom Payverge contract ABIs ship with the app.

**See also**:
- `AGENTS.md` — deeper agent-oriented guide (technology stack, full directory map, env flags, plugin authoring). When CLAUDE.md and AGENTS.md disagree, AGENTS.md is canonical.
- `docs/ONBOARDING.md` — 2-minute scannable orientation for new contributors with a "Where to Look" task → file map.
- `docs/CODEMAPS/` — token-lean architecture references, maintained by hand: `architecture.md`, `backend.md`, `frontend.md`, `data.md`, `dependencies.md`. Regenerate after major route/schema changes.

## Build & Development Commands

### Setup (one-time after clone)
```bash
make install-hooks   # installs git hooks (guest + operator locale-parity validators, pre-commit)
cp .env.example .env # root-level .env feeds docker compose AND local dev
```

### Frontend (`/frontend`)
```bash
cd frontend
npm run dev          # Dev server on :3000
npm run build        # Production build (use NODE_OPTIONS="--max-old-space-size=4096" if OOM)
npm run typecheck    # tsc --noEmit (clears .next/types first) — part of pre-deploy gate
npm run lint         # ESLint
npm run test         # Jest tests
npm run test:e2e:staff  # Playwright staff-roles spec
npm run format       # Prettier
```

### Backend (`/backend`)
```bash
cd backend
make build           # Build binary to bin/app
make run             # Run dev server on :8080
make test            # All tests with race detection
make quick-test      # All tests without race detection
make test-unit       # -short suite, no Docker
make test-integration # Postgres-backed suites from scripts/ci/go-pg-packages.sh; needs Docker or TEST_DATABASE_URL
make test-coverage   # Coverage report via scripts/test-coverage.sh
make lint            # golangci-lint
make fmt             # gofmt

# Run a single test
go test -v -race ./internal/services/... -run TestSpecificName
```

### Backend Performance Gate

Backend work that can affect latency, allocations, query count, queue throughput, or route fan-out is not complete until it is benchmarked and regression-tested. This applies especially to:

- Public/guest routes, dashboard polling routes, payment/webhook paths, AI transcript/session reads, bill/order/table/reservation/delivery list endpoints, middleware used by many routes, print/fiscal/notification queues, background schedulers, and any path that preloads ORM relations or reads large JSON/text payloads.
- Changes to database access shape, indexes, caches, serializers, queue workers, event fan-out, or hot service methods called by handlers.

Required workflow for these changes:

1. Add a failing regression/access-shape test first. Assert the behavior and, when the risk is data access, assert the dangerous query shape is gone (`SELECT *`, unused `Preload`, legacy JSON snapshot reads, N+1 queries, duplicate counts, etc.).
2. Capture a baseline benchmark before changing production code. Use `testing.B` with `-benchmem`; use local SQLite microbenchmarks for deterministic handler/service access-shape work and Testcontainers/Postgres benchmarks for larger flow realism when Docker is healthy.
3. Optimize the underlying implementation, not the benchmark. Prefer narrow projections, joins over per-row reloads, batched reads/writes, SQL aggregates, explicit indexes/migrations, bounded result sets, cache invalidation hooks, and relation preloads with selected columns. Avoid hydrating full aggregates unless the response or mutation truly needs them.
4. Validate with benchmark deltas: rerun the benchmark with `-count=3` where practical and record before/after latency, bytes/op, and allocs/op.
5. Run the relevant regression tests plus a compile/no-op gate for adjacent packages, for example:
   ```bash
   go test ./internal/handlers ./internal/server ./internal/database -run '^$' -count=1
   ```
6. Update `summary.md` or the relevant perf docs with the benchmark name, before/after numbers, commands run, and any blocked reruns.
7. Commit the slice as its own rollback point. Do not merge or mark the task done while benchmark-required work is unmeasured or test evidence is missing.

### Docs (`/docs`)
Plain Markdown tree (no build step, no package.json) — runbooks, codemaps,
plans, audits. Edit files directly; no dev server exists.

### Docker (full stack)
```bash
# Development stack built from this checkout (frontend :3000, backend :8080)
docker compose --env-file .env up -d --build
docker compose --env-file .env logs -f
docker compose --env-file .env down
```

The `--env-file .env` flag is not optional — compose only forwards variables that are referenced in `docker-compose.yml`, and the root `.env` is the canonical source. Per-service `backend/.env.example` and `frontend/.env.example` are stubs that point back to the root template.

Production self-hosting is a separate compose file under `deploy/` (Caddy, release images, backups, `install.sh`); see `deploy/README.md`. Do not add production-only settings to the root compose file.

## Architecture

### Backend (`backend/`)
- **Entry point**: `cmd/app/main.go` initializes all services, DB work, and routes.
- **Layered architecture**: Handlers (`internal/server/` is the larger home — about 130 non-test files, business/auth/RBAC/AI/director live here; `internal/handlers/` about 80 non-test files carries payments, analytics, accounting, fiscal, telegram webhooks) → Services (`internal/services/`) → Database (`internal/database/`).
- **Framework**: Gin web framework with GORM ORM on PostgreSQL 18. Go 1.26 language target (`go 1.26.0` in backend/go.mod); repository and CI use Go 1.27.2.
- **Migrations / schema ownership**: Source of truth is the **genesis baseline** (`backend/schema/genesis/current_schema.sql`) plus **versioned SQL** in `backend/migrations/` (zero-padded `NNNNNN_*.up.sql`/`.down.sql`; numbering restarted at the open-source squash, where the baseline is version 0; no numbered migration yet, so the next migration is `000001`). Empty DBs bootstrap from the genesis baseline then apply only pending numbered migrations; production startup does not run GORM AutoMigrate or schema-changing `RunEnsure*` helpers. **Do not add new `.AutoMigrate(` call sites** (enforced by `TestProductionStartupHasNoAdHocDDL` / `TestAutoMigrateSourceGate`). All new schema changes must be a numbered migration in `backend/migrations/`. Fatal startup paths: genesis bootstrap (empty DB), `RunMigrations`, schema verification.
- **Plugin system**: Payment integrations (Stripe, PayPal, MercadoPago) and the Telegram and Trustpilot integrations live in `internal/plugins/` and register via a **deferred initializer**: each plugin's `init()` calls `plugins.RegisterPluginInitializer(func(svc *services.PluginService) { ... })`, then `main.go` runs all initializers once `PluginService` is ready.
- **Auth**: JWT tokens with SIWE (Sign-In with Ethereum) and OAuth support in `internal/auth/`.
- **RBAC**: Role-based access control for admin/staff/user permissions per business.
- **Middleware chain**: Security → Auth → Business Validation → Handler (defined in `internal/server/`).
- **Blockchain service**: Initialized non-fatally (warns but doesn't crash if RPC unavailable).
- **Gin context values**: Handlers read `user_id`, `address`, `staff_id`, `role` from context.
- **Configuration**: Accepts both env vars and CLI flags (`--db-host`, `--production`, `--rpc-url`, etc.); see `cmd/app/main.go`. The self-host runtime contract (`PUBLIC_URL`, `REGISTRATION_MODE`, `ADMIN_EMAIL`, `DEMO_DATA`, `STORAGE_DRIVER`, `EMAIL_PROVIDER`, `LLM_BASE_URL`, `PRIMARY_VENUE`) lives in `internal/config/` and `cmd/app/oss_*.go`; `GET /api/v1/instance` reports the effective non-secret values. Startup preflight refuses known-unsafe production settings.
- **Admin CLI**: the backend binary has `admin`, `demo` and `invite` subcommands (`cmd/app/cli.go` and `cli_*.go`); see `docs/self-hosting/admin.md`.

### Frontend (`frontend/src/`)
- **Next.js 15 App Router** with route groups: `(home)` (the instance root), `(shop)` (operator sign-in at `/dashboard`, account, legal pages), `admin/`, `business/`, `b/`, `t/`.
- **URL map**: `/` serves the published venue (`PRIMARY_VENUE`, else the only published one, else a directory of `/b/<slug>` links, else a redirect to `/dashboard`; see `docs/api/home.md`). `/b/<slug>` is a venue page, `/t/<code>` a table (with `/menu`, `/bill`, `/signin`, `/profile`), `/dashboard` the operator sign-in and venue list, `/business/<id>/dashboard` a venue's dashboard, `/admin` the platform admin. The project's landing page (payverge.io) is the separate static site in `site/`, not part of the app.
- **API client layer**: Typed API functions in `src/api/` (one file per domain, paired `*.test.ts`).
- **State**: Zustand stores (`src/store/`) for global state, React Query for server state.
- **Web3**: Wagmi 2 + Viem for wallet + ERC-20 reads (USDC balance/allowance); no custom Payverge contract ABIs are vendored. `ethers` is not a direct dependency — references to "Ethers 6" in older docs are stale.
- **UI**: NextUI components + Tailwind CSS 3.4.
- **i18n**: Custom translation system in `src/i18n/` (`GuestTranslationProvider` + `guest-messages/*.json` for the 21-locale guest tier; `SimpleTranslationProvider` for the en/es operator tier). `next-intl` has been removed from `package.json` — it is **not** the runtime. Editing translations means editing the providers and message files directly.
- **Auth context**: `src/providers/HybridAuthProvider.tsx` (operator sessions: email, Google, wallet via Wagmi); `src/contexts/CustomerAuthContext.tsx` for guests.
- **Runtime config**: browser-visible settings come from `window.__PAYVERGE_ENV__`, rendered per request from the container environment by `src/config/publicConfig.ts` (whitelist `PUBLIC_ENV_KEYS`); server-only values live in `src/config/serverConfig.ts`. One image serves any domain. See `docs/self-hosting/frontend-config.md`.

## API Structure

Backend routes are organized under `/api/v1/`:
- `/` (public) — health, instance info, home, guest tables, currencies, public reservations, AI waiter
- `/auth` — authentication (SIWE, OAuth, email/password)
- `/staff` — staff token
- `/customer` — customer JWT
- `/inside` — authenticated operator endpoints; business-scoped routes under `/inside/businesses/:id`
- `/mercadopago` and `/stripe` — OAuth connect callbacks
- `/admin` — platform admin
- `/webhooks` — provider-signed: email provider, Telegram, PayPal, Stripe, MercadoPago
- `/health/live`, `/health/ready` — liveness and readiness probes
- `/metrics` — Prometheus; gated by `METRICS_TOKEN` / `METRICS_TOKENS` if exposed externally
- SSE under `internal/events/` plus domain polling — realtime channels. The old `/ws` WebSocket endpoint was removed.

## CI/CD

GitHub Actions under `.github/workflows/`:
- `ci.yml` — the PR and main gate: repository contracts (hygiene, workflow, docs links), backend build/vet/tests (sharded), Postgres integration tests, WhatsApp-tag compile, frontend lint/typecheck/Jest/build, offline AI evals, dependency and secret audits; `ci-ok` aggregates every job.
- `e2e.yml` — Playwright against the compose stack (labelled PRs and nightly).
- `acceptance.yml` — weekly (and manual) one-click self-host acceptance: builds the images, boots `deploy/` with zero third-party accounts, runs a restaurant end to end and proves restarts keep data (`scripts/acceptance/README.md`).
- `codeql.yml`, `scorecard.yml` — static analysis and OpenSSF Scorecard.
- `release.yml` — release-please, multi-arch images, SBOM and the `install.sh` release assets. A published image is not a deployment.

## Infrastructure

- **Database**: PostgreSQL 18 (internal-only inside compose; not exposed on the host).
- **Reverse proxy**: Caddy (stock image, config under `deploy/`) — one host: `/api/v1/*` and `/media/*` go to the backend, the rest to the frontend. Caddy owns TLS and edge security headers; backend keeps its own CORS.
- **File storage**: local disk by default (`STORAGE_DRIVER=local`, served under `/media`), or any S3-compatible service (public + protected buckets). See `docs/self-hosting/storage.md`.
- **Notifications**: email through `EMAIL_PROVIDER` (`log` writes to the backend log and is the default with nothing configured; `smtp`; or the API providers Resend and Postmark). Telegram, and WhatsApp (`whatsmeow`) only in the separate `-tags whatsapp` build, handle the other outbound channels.
- **Analytics**: PostHog (optional) + Prometheus.

## Repository Hygiene

**The repo root is a closed set — do not add files to it.** The complete list of
tracked root files is an allow-list enforced by
`scripts/ci/d10_hygiene_contract.test.mjs`; `git ls-files -- ':(glob)*' | grep -v /`
must return only these, plus the community files that allow-list names (such
as `CONTRIBUTING.md` and `SECURITY.md`):

```
.env.example  .gitignore  .nvmrc  AGENTS.md  CLAUDE.md  LICENSE  NOTICE
Makefile  README.md  TRADEMARKS.md  docker-compose.yml  package.json
promptfooconfig.yaml  summary.md
```

`summary.md` is a load-bearing document, not clutter: it is the benchmark log
the Backend Performance Gate writes to. The `config/` and `locales/`
directories must also stay at root — the backend walks up the tree for
`config/dashboard-tabs.json`, and i18n codegen treats `locales/registry.json`
as its single source of truth.

Only two untracked files belong at root, both tool-managed and gitignored:
`.env` and `skills-lock.json`.

Everything else has a home. `AGENTS.md` §3.1 has the full table; the short form:

| What you are writing | Where it goes |
|---|---|
| Audit / review / findings | Not in the repo — file an issue or keep it local |
| Spec or implementation plan | Not in the repo — put it in the GitHub issue or pull request description |
| Runbook / ops procedure | `docs/runbooks/` (add it to `docs/runbooks/catalog.json`) |
| Benchmark evidence | `summary.md`, or `docs/performance/` for a scoped stream |
| UI/design standard | `docs/design/` |
| Product or feature note | `docs/product/` |
| Backlog item | A GitHub issue (`docs/improvements/` is a private-repo backlog and is not exported) |
| Scratch notes / TODO dumps | Not in the repo — use the session scratchpad |
| Any video, audio, render, screenshot | `videos/` (gitignored, local-only) |

Four rules worth memorizing:

- **A file is either tracked or ignored, never both.** Add the ignore rule in the
  same commit that generates the output. `git ls-files | git check-ignore
  --no-index --stdin` must print nothing — `--no-index` is required, since git
  silently skips files that are already in the index.
- **Never `git add -A`.** It has swept multi-GB media into commits in this repo.
  Stage explicit paths.
- **Per-tool AI directories are local.** Agent and editor state directories
  stay out of git (`.gitignore` lists the common ones). `.claude/` is the only
  shared agent directory, and only its project skills in
  `.claude/skills/<name>/SKILL.md` are tracked (see `docs/agents/README.md`).
  Never commit a symlink that points into an
  ignored directory — it is broken in every other clone.
- **Name for content, not for the moment.** `e-stuff.md` and `to-check.md` both
  had to be renamed later; date-stamp anything point-in-time.

Media specifics: since `/videos/` is gitignored, deletions there are permanent;
there is no git history to restore from.

## Gotchas

These are easy to get wrong and worth re-reading before related work:

- **Env file is root-only.** Compose only injects vars that are declared in `docker-compose.yml`. Putting secrets in `backend/.env.example` or `frontend/.env.example` does nothing for compose.
- **`NEXT_PUBLIC_*` is build-time; runtime config is not.** Next.js inlines `NEXT_PUBLIC_*` at `npm run build`, so setting them at runtime is dead code. New browser-visible settings belong in the runtime whitelist in `frontend/src/config/publicConfig.ts` (read via `getPublicConfig()`), not in a new `NEXT_PUBLIC_*` variable; an ESLint rule guards direct `process.env.NEXT_PUBLIC_*` reads.
- **Production mode is shared.** The backend treats `--production`, `ENV=production`, `ENV=prod`, and `APP_ENV=production` as production mode for startup validation, cookies, redirects, and rate limits.
- **No runtime dark mode yet.** Tailwind `dark:` variants exist on a handful of components but no theme toggle ships. Don't scatter new `dark:` variants until the toggle and design tokens land as a focused initiative.
- **axios is unsafe in server components.** The shared `axiosInstance` short-circuits silently under SSR. Use raw `fetch()` in server components, and mock `fetch` (with real backend shapes) in tests.
- **Money wire contract:** backend stores `int64` cents and emits `float64` dollars via custom `MarshalJSON` (canonical file: `backend/internal/database/models_json.go` — Bill, Payment, AlternativePayment, WithdrawalHistory, ManualLedgerEntry, PayrollRun, PayrollLineItem, PaymentBreakdown, ReservationSettings). Honor this on both ends.
- **`--font-inter` is DM Sans.** The Tailwind variable name is a legacy holdover; layout.tsx binds DM Sans to it. There is no real Inter in the bundle.
- **Printer payloads are dollars, not cents.** Backend formatters consume the bill row's `float64` JSON shape (`MarshalJSON` already converts cents → dollars). Do not re-divide.
- **`window.print()` only runs client-side.** All printer-triggering components must be `"use client"`. Server components must not import `useIframePrint`.
- **No `FEATURE_FLAGS` registry.** Printers are not build-time gated: the printers UI is permission-gated (`printers:read` for settings, `print:bill` for print actions; owners always). Payment plugin availability follows the business plugin `is_enabled` state.
- **No upstream hosts.** Never hard-code `payverge.io` or any other deployment's host in code, env templates or compose defaults; derive URLs from `PUBLIC_URL`.

## Design Context

### Users
Mixed audience of traditional restaurant owners/managers and crypto-native operators. Ranges from small business owners managing daily operations to tech-savvy restaurateurs adopting Web3 payments and AI tools. Users arrive seeking operational clarity and modern payment solutions — the interface must feel accessible to non-technical operators while signaling innovation to early adopters.

### Brand Personality
**Premium, Trustworthy, Modern.** Payverge should feel like a high-end fintech product that inspires confidence in handling money. The emotional target is **excitement and possibility** — "this is the future of my business" — energizing and forward-looking without being hype-driven.

### Aesthetic Direction
- **Visual tone**: Premium fintech meets modern dev-tools polish. Think Stripe's clarity, Linear's refinement, Vercel's sharpness, and Raycast's premium feel.
- **Current palette**: Deep teal (#1a6b6a) primary with warm cream (#faf9f6) backgrounds and charcoal (#1c1917) text. Serif headings (DM Serif Display) + clean sans body (DM Sans).
- **Theme**: Light mode is the primary, production-ready experience. Dark mode is a planned follow-up (partially scaffolded with `dark:` Tailwind variants in a few components, but no runtime theme toggle ships yet). Do not scatter half-baked `dark:` variants across new work until the toggle and token mapping land as a dedicated initiative.
- **Anti-references**: No generic SaaS/Bootstrap templates. No playful cartoon illustrations. No crypto-bro neon gradients or hacker aesthetics. No enterprise bloatware density.

### Design Principles
1. **Confidence through craft** — Every pixel should reinforce trust. Precise alignment, consistent spacing, and intentional color use signal that the product handles money with the same care.
2. **Premium restraint** — Less is more. Generous whitespace, limited color palette, and purposeful typography. Let content breathe rather than competing for attention.
3. **Progressive disclosure** — Surface simplicity with depth available on demand. Don't overwhelm operators with complexity; reveal power features contextually.
4. **Dual-theme excellence** — Both light and dark themes must feel intentional and polished, not afterthoughts. Each should have its own considered palette, not just inverted colors.
5. **Accessible by default** — WCAG AA compliance as baseline. Sufficient contrast ratios, keyboard navigation, screen reader support, and reduced motion accommodations built into every component.
