<!-- Updated: 2026-10-04 for the public tree | Token estimate: ~700 -->

# Architecture

## Type
Monorepo, 3 code workspaces (Go backend monolith, Next.js frontend, plain Markdown docs) plus the self-hosting stack in `deploy/`.

## Data Flow
```
Browser ──HTTPS──> Caddy (TLS, security headers, one origin = PUBLIC_URL)
                    │
                    ├──> /*               ──> Next.js :3000 [SSR + client]
                    │                           │
                    │                           └──server-side fetch (BACKEND_INTERNAL_URL)──┐
                    │                                                                         │
                    └──> /api/v1/*, /media/* ──> Go/Gin :8080 <──────────────────────────────┘
                                              │
                                              ├──> PostgreSQL 18 (GORM)
                                              ├──> local disk or S3-compatible storage (public + protected)
                                              ├──> email: log / SMTP / Resend / Postmark
                                              ├──> Telegram; WhatsApp only in the -tags whatsapp build
                                              ├──> Stripe / PayPal / MercadoPago
                                              ├──> OpenAI-compatible LLM endpoint (optional)
                                              ├──> EVM RPC (USDC reads, non-fatal)
                                              ├──> PostHog (optional) + Prometheus
                                              └──> SSE to clients
```

## Service Boundaries
- **Frontend**: rendering, auth flow UI, dashboard tabs, guest QR/ordering flows. Server components must use raw `fetch` (axios SSR-unsafe).
- **Backend**: REST API under `/api/v1/`, plugin registry, blockchain verification, background jobs (`internal/jobs/`), SSE fan-out (`internal/events/`).
- **Caddy**: TLS + security headers. No CORS (backend owns CORS).
- **Plugins**: payment integrations live inside backend (`internal/plugins/`), self-register via `init()` + `RegisterPluginInitializer`.

## Workspaces
| Path | Tech | Port |
|---|---|---|
| `frontend/` | Next.js 15.5 + React 19 + Tailwind 3.4 + NextUI 2.4 | 3000 (container and dev) |
| `backend/` | Go 1.26 + Gin + GORM (Go 1.27.2 toolchain) | 8080 (container and dev) |
| `docs/` | Plain Markdown (no build step) | — |

## Production Topology
Single host, Docker Compose (`deploy/docker-compose.yml`). Caddy is the only service with published ports; Postgres, backend and frontend sit on internal networks. Optional profiles add backups, off-site copies, MinIO and restore. See `deploy/README.md`.

## Cross-Cutting Concerns
- **Auth**: JWT + SIWE + OAuth (Google). Session revocation store in PG.
- **RBAC**: Per-business permission keys. The business owner has every permission. Staff defaults are manager > server > host > kitchen (`StaffRolePermissions` in `backend/internal/server/rbac.go`). Platform admin (`/api/v1/admin`) is separate from business roles.
- **i18n**: Custom system (`frontend/src/i18n/`), not next-intl runtime.
- **Money**: int64 cents in DB, float64 dollars on wire (`backend/internal/database/models_json.go`).
- **Realtime**: SSE channels under `internal/events/`, plus domain polling where SSE is not wired. The old `/ws` WebSocket endpoint was removed.

## See Also
- `backend.md` — route map, middleware chain
- `frontend.md` — page tree, component layout
- `data.md` — tables, migrations
- `dependencies.md` — third-party + key libs
