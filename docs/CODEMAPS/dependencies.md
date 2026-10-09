<!-- Generated: 2026-05-22 | Files scanned: ~10 manifests | Token estimate: ~600 -->

# Dependencies

## Third-Party Services
| Service | Used For | Touchpoints |
|---|---|---|
| Local disk or S3-compatible storage | Public + protected file storage (`STORAGE_DRIVER`, local default) | `backend/internal/s3/`, AWS SDK v2 |
| Stripe | Stripe Connect guest payments | `backend/internal/plugins/stripe/`, webhook `/api/v1/webhooks/stripe` |
| PayPal | Alt payment | `backend/internal/plugins/paypal/paypal.go` |
| MercadoPago | LATAM alt payment | `backend/internal/plugins/mercadopago/` |
| Resend / Postmark | Transactional email — `EMAIL_PROVIDER` selects | `backend/internal/emails/` (`resend_provider.go`, `postmark_provider.go`) |
| Telegram Bot API | Owner notifications, webhooks | `backend/internal/telegram/`, `plugins/telegram/`, webhook `/api/v1/webhooks/telegram` |
| WhatsApp (whatsmeow) | Owner notifications | `backend/internal/notifications/`, `whatsapp_business_devices` in genesis; `whatsmeow_*` created at runtime by sqlstore (`-tags whatsapp`) |
| Google OAuth | Google sign-in | `backend/internal/auth/handlers.go` (`GoogleAuthURL`, `GoogleCallback`); staff sign-in in `backend/internal/auth/staff_oauth.go`. Routes: `GET /api/v1/auth/google`, `/google/callback`, `/google/staff`, `/google/staff/callback` |
| Trustpilot | Review aggregation | `backend/internal/plugins/trustpilot/` |
| EVM RPC | USDC verification (non-fatal init) | `backend/internal/blockchain/` |
| OpenRouter (Gemini models) | AI waiter, menu tools, and director console | `backend/internal/llm/openrouter/`, wired in `cmd/app/main.go` |
| PostHog | Product analytics | `backend/internal/metrics/`, frontend providers |
| Prometheus | Metrics scrape (`/metrics` gated by `METRICS_TOKEN(S)`) | `backend/internal/observability/` |

## Backend (Go 1.26 language target; Go 1.27.2 toolchain) — key libs
```
gin-gonic/gin              HTTP framework
gorm.io/gorm + driver/postgres
golang-migrate/migrate/v4  SQL migrations
golang-jwt/jwt/v4          JWT
ethereum/go-ethereum       EVM client
mattevans/postmark-go      Email
go-telegram-bot-api/v5     Telegram
go.mau.fi/whatsmeow        WhatsApp
aws-sdk-go-v2/...          AWS S3
posthog/posthog-go         Analytics
prometheus/client_golang   Metrics
robfig/cron/v3             Job scheduling
sirupsen/logrus            Structured logging
stretchr/testify           Test assertions
skip2/go-qrcode + fogleman/gg + golang/freetype  QR + image gen
```

## Frontend (Node 26.10.0 release toolchain; Node 20.18+ package minimum) — key libs
```
next 15.5                  App Router
react 18.3
@nextui-org/react 2.4      UI
tailwindcss 3.4
lucide-react               Icons
wagmi 3.7 + viem 2.45      Web3 (USDC reads only)
@tanstack/react-query 5    Server state
zustand 4.5                Client state
zod 4                      Validation (no form library)
framer-motion 11           Animation
chart.js 4 + react-chartjs-2  Charts
date-fns 3                 Dates
react-hot-toast            Notifications
@lifi/sdk                  Cross-chain (LiFi)
@dnd-kit/*                 Drag-and-drop
jspdf + html2canvas + pdfjs-dist  PDF gen + parse
qrcode                     QR generation
dompurify 3                Sanitization
react-markdown + remark-gfm  Assistant rich text
axios 1.x                  HTTP (client only — SSR-unsafe)
```

> **Not present despite older docs**: `ethers` is **not** a direct dependency. References to "Ethers 6" in stale docs are wrong.
> **i18n runtime**: `next-intl` is not a direct dependency; the application uses the custom runtime in `src/i18n/`.

## Infra / Tooling
- **Docker Compose** (`docker-compose.yml` local stack; self-host production files under `deploy/`)
- **Caddy** TLS + reverse proxy: stock `caddy:2` with the checked-in config in `deploy/` (`Caddyfile`, `Caddyfile.cloudflare`, `payverge.caddy`, `cloudflare-cidrs.caddy`)
- **GitHub Actions**: `ci.yml` (pull request and main gate, aggregated by `ci-ok`), `e2e.yml` (compose Playwright journeys, label `e2e` and nightly), `codeql.yml`, `scorecard.yml` and `release.yml` (release-please, signed multi-arch images, release assets). CI reads Go from `backend/go.mod` and Node from `.nvmrc`.
- **Pre-commit hook**: guest-locale validator (`make install-hooks`)

## Env Source of Truth
Root `.env` only. Compose only forwards vars referenced in `docker-compose.yml`. Per-service `backend/.env.example` and `frontend/.env.example` are stubs.
