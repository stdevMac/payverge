# Onboarding Guide: Payverge

Scannable 2-minute orientation for new contributors and AI agents joining the project. For deeper detail see `AGENTS.md` (canonical) and `CLAUDE.md`.

## Overview
AI-powered restaurant management platform with Web3 (USDC) payment support. Monorepo containing a Next.js operator dashboard + guest-facing flows, a Go/Gin API, and a plain Markdown documentation tree (no build step). On-chain code is limited to USDC verification on the backend and ERC-20 / Wagmi wallet flows on the frontend — no custom Payverge contracts ship in the bundle.

## Tech Stack
| Layer | Technology | Notes |
|---|---|---|
| Frontend | Next.js 15.5 (App Router) + React 19 | package name `payverge-frontend` |
| UI | NextUI 2.4 + Tailwind 3.4 + lucide-react | DM Serif Display headings, DM Sans body |
| Web3 | Wagmi 2 + Viem | USDC reads only; `ethers` is **not** a direct dependency |
| State | Zustand + TanStack Query | |
| Backend | Go 1.26 + Gin + GORM | Repository and CI pin Go 1.27.0 |
| DB | PostgreSQL 18 | Production schema SoT: genesis baseline (`backend/schema/genesis/`) + versioned SQL (`backend/migrations/`); startup performs no GORM AutoMigrate or ad-hoc schema ensures |
| Infra | Docker Compose + Caddy (TLS) | Self-host stack and installer in `deploy/`; release images on GHCR |
| Notifications | Email via `EMAIL_PROVIDER` (`log` default, `smtp`, Resend, Postmark), Telegram bot, `whatsmeow` | WhatsApp only in the `-tags whatsapp` build |
| Observability | PostHog + Prometheus | `/metrics` gated by `METRICS_TOKEN(S)` |

## Architecture
3 code workspaces plus the self-hosting stack in one repo:

```
payverge/
├── frontend/   Next.js 15 App Router — operator dashboard + guest flows
├── backend/    Go/Gin API — handlers → services → GORM/PG
├── deploy/     Self-hosting compose, Caddy, install.sh
└── docs/       Plain Markdown — self-hosting, AI, architecture, runbooks, codemaps (no build)
```

Backend is a layered monolith (handlers → services → database). Frontend talks to it through a typed API client layer in `frontend/src/api/`, one file per domain. In a `deploy/` install, Caddy fronts both on one origin: `/api/v1/*` and `/media/*` go to the backend, everything else to the frontend.

## Key Entry Points
- **Backend bootstrap**: `backend/cmd/app/main.go` wires DB, migrations, all route registration, and plugin `init()` side-effect imports (Stripe, PayPal, MercadoPago, Telegram, Trustpilot).
- **Frontend App Router root**: `frontend/src/app/layout.tsx` + `frontend/src/app/providers.tsx`.
- **Migrations / schema**:
  - **Source of truth**: genesis baseline (`backend/schema/genesis/current_schema.sql`) + versioned SQL in `backend/migrations/NNNNNN_name.up.sql` / `.down.sql` (numbering restarted at the open-source squash; no numbered migration yet, so the next migration is `000001`).
  - **New schema changes**: numbered migration in `backend/migrations/` only. Production startup is genesis (empty DB only) → pending migrations → read-only verification.
  - GORM AutoMigrate remains available only as a SQLite/benchmark fixture builder; it is not part of production startup.
- **Plugin contracts**: `backend/internal/plugins/interface.go` + `registry.go` + `init.go`. Plugin folders register via `init()` → `plugins.RegisterPluginInitializer("<name>", func(svc *services.PluginService) { ... })` (deferred; `main.go` runs initializers in name order once `PluginService` exists).
- **Frontend API client**: `frontend/src/api/<domain>.ts` paired with `<domain>.test.ts`.
- **Money JSON shape**: `backend/internal/database/models_json.go` — custom `MarshalJSON` converts `int64` cents to `float64` dollars on the wire.

## Directory Map

**Backend `internal/`** (35+ packages — grouped by concern):
| Folder | Purpose |
|---|---|
| `server/` | **Larger handler home** (~130 non-test files): business, auth, RBAC, AI waiter, director console, staff, reservations, inventory, telegram connection, fiscal RBAC |
| `handlers/` | Secondary handler home (~80 non-test files): payments, analytics, accounting, customer addresses, delivery, error logs, fiscal, paypal OAuth, plugin handlers, print jobs, splitting, telegram webhooks |
| `services/` | Business logic; one file per domain |
| `database/`, `migrations/` | GORM models, migration runner |
| `auth/`, `session/`, `middleware/` | JWT + SIWE + OAuth, RBAC chain |
| `plugins/{stripe,paypal,mercadopago,telegram,trustpilot}/` | Payment & integration plugins |
| `blockchain/` | USDC verification (non-fatal init) |
| `fiscal/`, `accounting/`, `analytics/`, `crm/` | Vertical domains |
| `events/` | SSE channels; other realtime views use polling where SSE is not wired |
| `notifications/`, `emails/`, `telegram/` | Outbound channels |
| `tests/` | Integration tests (vs. unit `*_test.go` next to source) |

**Frontend `src/`**:
| Folder | Purpose |
|---|---|
| `app/(home)/`, `app/b/`, `app/t/`, `app/scan/` | Guest-facing routes: `/` (the published venue or a directory, else a redirect to `/dashboard`), `/b/<slug>`, `/t/<code>` |
| `app/(shop)/` | Operator sign-in and venue list at `/dashboard`, account, legal pages |
| `app/(shop)/business/[businessId]/dashboard/` | Operator dashboard (tab-based via `?tab=`) |
| `app/admin/` | Platform admin |
| `api/` | Typed fetch/axios wrappers per domain |
| `components/business/` | Dashboard tab components (Kitchen, Bills, Reservations…) |
| `contexts/`, `providers/` | Auth, i18n, theme |
| `i18n/` | Custom translation system; `next-intl` has been removed from dependencies |
| `store/` | Zustand stores |

## Request Lifecycle
1. Browser → Caddy (`PUBLIC_URL`, `/api/v1/*`) → Go binary on `:8080` inside the compose network (no host port in a `deploy/` install).
2. Gin middleware chain (in `internal/server/`): RequestID → CORS → SecurityHeaders → InputValidation → JSONSizeLimit → RateLimit → PrometheusMiddleware → Auth → Business Validation → handler.
3. Handlers read `user_id`, `address`, `staff_id`, `role` from `gin.Context`.
4. Handler calls into a service in `internal/services/`.
5. Service uses GORM models from `internal/database/`.
6. JSON response — money values are marshalled cents → dollars via `models_json.go`.
7. Frontend axios instance (client only — never in server components) or raw `fetch` handles the response.

## Conventions
- **Commits**: Conventional Commits (`feat(frontend): …`, `fix(frontend): …`). Most current scope: `frontend`.
- **Backend tests**: unit `*_test.go` paired with source under `internal/<pkg>/`; integration under `internal/tests/`. Run with `make test` (race) or `make quick-test`.
- **Frontend tests**: `*.test.ts` paired with source (Jest); Playwright E2E under `frontend/tests/`.
- **Migrations**: paired `up`/`down` SQL with zero-padded sequence in `backend/migrations/`. SQL migration failure aborts startup. Genesis bootstrap, schema verification, auth schema work, and required data repairs also fail fast; production startup runs no GORM AutoMigrate safety net.
- **Plugins**: register via `init()` + `plugins.RegisterPluginInitializer(...)` + blank-import in `cmd/app/main.go`.
- **Money**: `int64` cents in DB, `float64` dollars on the JSON wire (`models_json.go`) — both ends must honor this.
- **Server components**: do not import a shared axios instance. Use raw `fetch`.
- **`NEXT_PUBLIC_*`**: build-time inline. Runtime settings go through `getPublicConfig()` (`frontend/src/config/publicConfig.ts`, rendered as `window.__PAYVERGE_ENV__`), so one image serves any domain.

## Common Tasks
```
# Setup
make install-hooks
cp .env.example .env

# Frontend (port 3000)
cd frontend && npm run dev | typecheck | lint | test | test:e2e:staff

# Backend (port 8080)
cd backend && make run | test | quick-test | test-integration | lint | fmt
go test -v -race ./internal/services/... -run TestSpecificName

# Docs — plain Markdown; edit files in docs/ directly (no npm start / no package.json)

# Full stack
docker compose --env-file .env up -d --build
```

## Where to Look
| I want to… | Look at… |
|---|---|
| Add an HTTP endpoint | `backend/cmd/app/main.go` (route wiring) + handler in `backend/internal/server/<domain>.go` (or `internal/handlers/` for payments/analytics/accounting/fiscal/etc.) |
| Add business logic | `backend/internal/services/<domain>.go` |
| Add a DB column | new pair in `backend/migrations/` + regenerated genesis baseline + GORM model in `backend/internal/database/` |
| Add a payment integration | new package under `backend/internal/plugins/` + blank-import in `main.go` + `init()` calling `plugins.RegisterPluginInitializer(...)` |
| Add a dashboard tab | `frontend/src/components/business/` + register in `DashboardSidebar.tsx` + tab switch in dashboard `page.tsx` |
| Add a guest route | `frontend/src/app/b/` or `app/t/` (`app/(home)/` is only `/`) |
| Add an API call | `frontend/src/api/<domain>.ts` + paired `.test.ts` |
| Add a translation key | `frontend/src/i18n/messages/en/` + `es/` (custom system, not next-intl); guest keys in every `frontend/src/i18n/guest-messages/*.json` |
| Configure or self-host an instance | `deploy/README.md`, `docs/self-hosting/` |
| Understand an AI feature | `docs/ai/README.md` |
| Change env defaults | root `.env.example` (development) and `deploy/.env.example` (self-hosting) — per-service stubs are dead |
| Adjust CI gating | `.github/workflows/ci.yml` (PR and main gate; `ci-ok` aggregates every job) |
| Change money serialization | `backend/internal/database/models_json.go` |

## Operating Caveats (high-friction items)
- Root `.env` is canonical; `backend/.env.example` and `frontend/.env.example` are stubs.
- `--font-inter` Tailwind variable is bound to **DM Sans**, not Inter.
- Dark mode is scaffolded but not shipped — don't scatter new `dark:` variants until the toggle and design tokens land as a focused initiative.
- A published release image is not a deployment. Rolling out to a host is always a separate, manual step.
- Backend treats `--production`, `ENV=production`/`prod`, and `APP_ENV=production` as prod mode.
- The shared frontend axios instance silently short-circuits under SSR — use raw `fetch` in server components and tests that need real backend shapes.

---

For the design system, brand voice, and UI principles, see the **Design Context** section of `CLAUDE.md`.
