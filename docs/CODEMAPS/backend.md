<!-- Generated: 2026-08-01 | Files scanned: ~250 | Token estimate: ~900 -->

# Backend (Go/Gin)

## Entry
`backend/cmd/app/main.go` wires DB, migrations, all routes, and plugin initialization.

## Route Groups (under `/api/v1/`)
| Group | Auth | Source |
|---|---|---|
| `/` (public) | None | health, instance info, home (`/api/v1/home`), guest tables, currencies, public reservations, **AI waiter (guest chat + session creation)** |
| `/auth` | None (rate limited) | login, register, OAuth, SIWE, password reset, email verify |
| `/inside` | Wallet JWT or hybrid | business CRUD, menu, tables, bills, orders, analytics, staff, plugins |
| `/customer` | Customer JWT for protected routes; session-info, refresh, and logout are unauthenticated | session-info, refresh, logout, connect-business, table check-in, profile, businesses, preferences, link-wallet, delete account |
| `/staff` | Staff token | staff profile, logout |
| `/admin` | Admin JWT + rate limit | user mgmt, business oversight, plugins, analytics, errors |
| `/webhooks` | Provider signature | Stripe, PayPal, MercadoPago, Telegram, Resend email (`registerEmailWebhookRoutes`; Postmark has no route) |
| `/mercadopago` | None (public) | OAuth connect callback `GET /oauth/callback` |
| `/stripe` | None (public) | OAuth connect callback `GET /oauth/callback` |

**New AI routes** (campaign 2026-06-06):
- `POST /api/v1/ai-waiter/:businessId/session` → `server.CreateAIWaiterSession` — issues 64-hex crypto session token; greeting carries EU AI Act Art. 50 disclosure.
- `POST /api/v1/ai-waiter/:businessId` → `server.HandleAIWaiter` — guest chat with allergen safety, guardrails, PII redaction, budgets.
- `GET /api/v1/ai-waiter/:businessId/stream` → `server.HandleAIWaiterStream` — SSE streaming (Lane I, replaces 3s polling).

## Sample Endpoints
```
GET    /api/v1/customer/profile                → crmHandler.GetCustomerProfile
POST   /api/v1/customer/logout                 → crmHandler.LogoutCustomer
GET    /api/v1/admin/businesses                → server.GetBusinessList
POST   /api/v1/admin/plugins                   → pluginHandlers.CreatePlugin
POST   /api/v1/admin/users/:id/close           → adminUserHandler.AdminCloseAccount
GET    /metrics                                → promhttp (gated by METRICS_TOKEN)
GET    /api/v1/health/{live,ready}             → health checks
```

## Middleware Chain (in order)
```
gin.Recovery
  → RequestID
  → CORS (origin allowlist, not wildcard)
  → SecurityHeaders (HSTS/CSP/XFO/Referrer/Permissions)
  → InputValidation (blocks NoSQL injection, path traversal)
  → JSONSizeLimit(10MB)
  → RateLimit (300/min prod, 1200/min dev; 10/min on /auth, 5/min public forms)
  → PrometheusMiddleware
  → [route-group auth middleware]
  → [RBAC: RoleBasedAccessMiddleware("permission:key")]
  → [RequireOperationalBusiness → 403 business_suspended / business_closed]
  → Handler
```

## Auth Middleware Variants
`AuthenticationMiddleware` (wallet) · `AuthenticationAdminMiddleware` · `StaffAuthenticationMiddleware` · `CustomerAuthenticationMiddleware` · `HybridAuthenticationMiddleware` (wallet OR staff)

## Handler → Service Mapping
Two handler homes (CLAUDE.md gotcha — `server/` is the larger one):

| Concern | Handler dir | Notable files |
|---|---|---|
| Business, auth, RBAC, AI waiter, director console, staff, reservations, inventory | `internal/server/` (~130 non-test files) | `auth_handlers.go`, `business_handlers.go`, `ai_waiter_handler.go`, `ai_waiter_session_handler.go`, `director_console_handler.go`, `staff_handlers.go`, `reservation_handlers.go`, `inventory_handlers.go`, `rbac.go` |
| Payments, analytics, accounting, fiscal, delivery, customer addresses, splitting, telegram webhooks, printers | `internal/handlers/` (~80 non-test files) | `payments.go`, `analytics.go`, `accounting.go`, `fiscal_handlers.go`, `delivery_handlers.go`, `splitting.go`, `printer_handlers.go`, `print_job_handlers.go` |

Services: `internal/services/<domain>.go` — business logic. AI lives in `internal/services/ai.go` + `services/director_tools/`. Post-campaign (2026-06-06): `ai.go` no longer hand-builds prompts via `fmt.Sprintf` — prompts moved to spotlighted asset files under `services/prompts/ai_waiter/**`; `GetGreeting` map deleted in favor of 21-locale greeting assets (`services/prompts/ai_waiter/greetings/*.md`). New service files: `services/allergen_intent.go` (deterministic allergen safety), `services/waiter_greeting.go` (EU AI Act greeting), `services/prompt_sanitize.go` (spotlighting/sanitize helpers).

### Marketing Creative System
- `internal/server/marketing_handlers.go` owns Marketing suggestions, caption briefs, image generation, and the Marketing-authorized image-credit read route.
- `internal/server/marketing_activity_handlers.go` owns creative settings, explicit posted confirmation, history, reuse snapshots, dismiss, restore, and the isolated one-time legacy-history import action.
- `internal/services/marketing/engine.go` derives and prioritizes restaurant opportunities; it does not schedule or publish content.
- `internal/services/ai.go` builds fact-grounded captions and native-aspect image requests from the validated creative profile.
- Routes require an operational (not suspended) business and mirror `marketing:read` / `marketing:write` RBAC.

## Plugin Registration
```go
// in plugin's package
func init() {
    plugins.RegisterPluginInitializer("name", func(svc *services.PluginService) {
        // wire handlers, configure provider
    })
}
```

## Spaces & Tables
- Handlers: `internal/server/space_handlers.go`, `space_scan_handlers.go`
- Domain service: `internal/spaces/` (layout validate/publish, `AssignLegacyTables`)
- Models: `internal/database/space_models.go`, table placement columns on `Table` (`space_id`, `pos_*_mm`, …) — **nullable**; create/update table APIs remain name/capacity/QR-only
- Setup status: optional `steps.layout` (published space count) — does **not** affect `required_done` / `total_count` / `all_done`
- Scan worker env: `SPACE_SCAN_SESSION_TTL_MINUTES`, `SPACE_SCAN_MAX_UPLOAD_BYTES`, `SPACE_SCAN_RAW_RETENTION_DAYS`, `SPACE_SCAN_MAX_FRAMES`
Then blank-import in `main.go`. Active: `stripe`, `paypal`, `mercadopago`, `telegram`, `trustpilot`.

## Background Workers
- `internal/jobs/` — cron schedules via `robfig/cron/v3`
- `internal/fiscal/` — idempotent issuance jobs (Argentina ARCA + UAE shape)
- `internal/services/print/` — print queue + router + formatters
- Translation jobs in `internal/server/translation_jobs.go`

## Gin Context Keys
Handlers read: `user_id`, `address`, `staff_id`, `role`.

## Gotchas
- Main SQL migration failure aborts startup. Genesis bootstrap, schema verification, and required startup data repairs also fail fast; production startup runs no GORM AutoMigrate safety net.
- Blockchain init is non-fatal (warns if RPC unavailable).
- Money JSON: `MarshalJSON` in `internal/database/models_json.go` converts int64 cents → float64 dollars.

## AI Excellence Campaign additions (2026-06-06)

### New packages
- `internal/guardrails/` — input moderation + scope classifier (Gemini-based, fail-open).
- `internal/pii/` — `Redact(s string) string` (email/phone; date/ID-safe).
- `internal/llmeval/` — offline + live eval harness (`langid`, `suites/`, judge/grader/runner).

### New service files
- `services/allergen_intent.go` — deterministic allergen-safety path (C6).
- `services/waiter_greeting.go` — EU AI Act greeting with 21-locale assets.
- `services/prompt_sanitize.go` — spotlighting helpers (`SanitizePromptField`, `wrapDataBlock`).
- `services/ai_retention_janitor.go` — GDPR retention cleanup job (Lane F).
- `services/prompts/ai_waiter/**` — spotlighted prompt assets (21 locales, static-first ordering).
- `services/prompts/ai_waiter/greetings/*.md` — 21-locale greeting assets (replaces `GetGreeting` map).

### Migrations
The AI retention indexes and `ai_generated_images` are in the genesis baseline.

### Provider layer
- `internal/llm/` — provider-neutral interface (`Generate`, `ModelConfig`, `ImageConfig`).
- `internal/llm/openrouter/` — OpenRouter provider with fallback-chain support.
- Model: `google/gemini-2.5-flash` for chat/director/wizard; `google/gemini-2.5-flash-image` for images.

## Dead-code inventory (2026-10-05)

`cd backend && ~/go/bin/deadcode ./...` reports **199** functions that the
production binaries never reach (default build tags). With `-test` added the
list shrinks to 5, and all 5 are expected: four interface-conformance stubs in
`plugins/interface_optional_test.go`, plus `WhatsAppTerminalReply`, which is
only reachable under `-tags whatsapp`. So every remaining entry is exercised by
a test. Each one falls into a class below, and only the last class is real
cleanup debt. Nine unreachable legacy wrappers were deleted on 2026-10-05:
`finalizeOpsGuidance`, `DecryptOAuthTokenAtRest`,
`GetReservationsByBusinessIDPaginated`, `imageJobDedupKey`,
`aiImageProvenanceModel`, `menuUnavailableItemIDs`, `normalizeLocale`,
`HasCandidateTables` and `IsScanDraftOperatorContent`.

| Class | Why `deadcode` flags it | Members (package → symbols) |
|---|---|---|
| Build-tag only | Reached only from `whatsapp_manager.go` (`//go:build whatsapp`) | `database/whatsapp_device.go` (all), `services/whatsapp_sender_quota.go`, `whatsapp_helpers.go`, `whatsapp_responses.go`, `waiter_locale.go`, `waiter_runtime_context.go`, `allergen_intent.go` (`MentionsStaffConfirmation`, `anyContains`, `lowerAll`) |
| Test seams | Swap or reset package state for tests (`*ForTest*`, `Set*`, `reset*`, clocks) | `config` (`Set*ForTesting`, `test_config.go`), `database/db_config.go` (`NewDB`, `InitTestDB`, `SetTestDB`), `database/offers.go` schedule clock, `s3/store.go` (`SetStores`, `SetPublicURL`, `ActiveDriver`, `updateState`), `services` (`DisablePricingCacheForTest`, `DefaultSeedPluginNames`), `services/marketing` `Engine.Set*`, `cryptorefund` `ProcessOneForTest`, `dailyquota` `Count`/`Reset`/`SetClock`, `logic` `ChallengeStore.SetMaxRedeemed`/`Len`, `events` `Hub.SetGuestConnectionLimits` + conn counters, `plugins` `UnregisterPlugin`, `paypal` token-cache reset, `security` dev-key reset, `signedid.NewWithSecret`, `auth.SetAdminPasswordDenylist`, `server` `reset*ForTest`/`resetTranslationJobs`/`waitForPromotionTranslationBackfills`/`marketingCaptionCache.clear`, `handlers` geo-lookup reset |
| Test harness packages | Whole packages imported only from `_test.go` | `testperf`, `testperf/genesisdb`, `database/dbtest`, `services/stripetest`, `aicontract`, `assistantcontract` (`Schema` + helpers), `llmeval`, `paymentcontract.RunContract`, `agents/ops_guides` (`ValidateCatalog`, `AllTabs`, `RegistryLoadError`), `server` (`ValidateCartToolCallsForContract`, `AIErrorLeaksSecret`) |
| Enumerations for contract tests | Return the closed set a test pins | `activation.Names`, `telegramPluginProductionModels`, `emails.managedTemplateNames`, `llm.KnownProductionFeatures`, `locales.TranslationProviderTarget`/`PublishableLocales`, `metrics` `*OutcomeValues`/`RefreshRealms`/`Current*Outcome`, `reporting.AllMethods`/`RawAliases`, `demo` `isExplorerLinkableTxHash`/`zoneHoursMondayOnly`, `structs.NewDefaultNotificationPreferences` |
| Test oracles | Independent reference math that production results are checked against | `database` `equalShareAmounts`/`SummarizeRecognizedPaymentEvents`, `splitting.roundToTwoDecimals`, `services` `guessedEvenLayerYs`/`canonicalFromPromptFamily`, `spaces.ValidateLayoutDocument`, `server` `WaiterMenuSnapshot.VisibleMenuCategories` |
| Reflection or interface | Called through GORM `TableName` or an interface no production path invokes | `TableName` on `activation.State`, `PaymentEvent`, `TableCombination`; `DisabledReportSender.Send`; `config.Report.Codes`; `ScriptedProvider`/`FixtureProvider.Generate` |
| **Unwired production code (debt)** | Production-shaped, still used as test fixtures, not called by any route or job | `agents.FinalizeOpsV2`, `events` `ReplayAfter`/`hasPermission`, `guardrails.NewGeminiClassifier`, `handlers` `resolvePluginWebhookSignature`/`staffDisplayPermissions`, `httpclientx.Shared`, `jobs` `ServiceCallTTLJanitor.Stop`, `llm` `NewAICostGate`/`NewFeatureCostGate`/`NewGenerateRequest`, `middleware` `RateLimit`/`SimpleRateLimiter.Stop`, `plugins.PaymentProviderReconciliationAllowed`, `mercadopago` `makeMercadoPagoAPICall`, `runtimecontrol` `WithRegistrationMode`/`HasInviteClaim`, `s3` `buildLocation`/`IsNotFound`, `services` `MenuExtractionWorker.ProcessOne`, `cmd/ai-image-backfill` `publicBucketConfig.summary` |

Each debt entry is load-bearing for one or more test files, so deleting it
means rewriting those tests against the live entry point. Do that one package
at a time and re-run `deadcode -test` (expected output: only the five entries
above).
