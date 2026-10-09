<!-- Generated: 2026-05-22 | Files scanned: ~400 | Token estimate: ~900 -->

# Frontend (Next.js 15 App Router)

## Entry
- `frontend/src/app/layout.tsx` — root layout, fonts, providers
- `frontend/src/app/providers.tsx` — Wagmi, React Query, NextUI, i18n, AuthContext, Toast
- `frontend/src/middleware.ts` — locale cookies and prefixes, guest locale on `/`, invite links on `/` → `/dashboard`

## Page Tree
```
src/app/
├── (home)/page.tsx            "/": the primary venue, a venue directory, or a
│                              redirect to /dashboard (GET /api/v1/home)
├── (shop)/                    Operator entry and public account pages
│   ├── account/  app/  join/  unsubscribe/
│   ├── business/[businessId]/dashboard/  Operator dashboard (?tab=; page.tsx, tabParams.ts)
│   ├── business/register/
│   ├── dashboard/             Sign-in, then one venue redirects, several pick
│   ├── es/  es-ar/  staff/    Staff login and home (locale-prefixed too)
│   └── privacy-policy/  refund/  terms-and-conditions/
├── admin/                     Platform admin (admin JWT required)
│   └── {analytics,businesses,demo,emails,errors,escalations,fiscal,plugins,stripe,translations,users}
├── api/health/                Next-side health route
├── b/[customUrl]/             Venue storefront ("/b/<primary>" canonicalizes to "/")
├── business/[businessId]/     Slug redirect to /b/<custom_url>, plus alternative-payments and settings/printers
├── delivery/[deliveryNumber]/track    Public delivery tracking
├── forgot-password/  reset-password/  verify-email/
├── reservations/  scan/  space-scan/  t/   Guest flows
└── error.tsx  not-found.tsx  globals.css  sitemap.ts  robots.ts
```

## Component Hierarchy
`src/components/` is domain-grouped:
```
components/
├── account/  admin/  assistant/  auth/  business/   (largest — dashboard tabs)
├── business-page/  chat/  common/  customer/  dashboard/  delivery/
├── guest/  icons/  instance/  legal/  legal-brand-icons/
├── main/  menu/  navigation/  notifications/  opsAssistant/
├── payment/  pwa/  qr/  receipt/  shared/  splitting/
├── staff/  table/  title/  ui/  user/  venue-directory/
└── Footer.tsx  ComingSoon.tsx  Maintenance.tsx  *LanguageSwitcher*.tsx
```

### Dashboard Tab Components (`components/business/`)
`BillManager` · `BillCreator` · `BillDetailsModal` · `BillFilters` · `BillsTable` · `BundlesManager` · `BusinessOverview` · `BusinessPageEditor` · `BusinessSettings` · `CounterManager` · `CRMManager` · `DashboardSidebar` · `DashboardLayout` · `DashboardLockedTabView` · `AlternativePaymentManager` · `AccountingDashboard` · `CurrencySettings` · `DeliverySettings` · `AiWaiter/`

Tab routing: `?tab=` is read with `useSearchParams` in `(shop)/business/[businessId]/dashboard/page.tsx`. `tabParams.ts` beside that page holds aliases and the param whitelist.

### Spaces & Tables (operational integration)
- **Spaces editor/overview**: `components/business/spaces/` (`SpacesOverview`, `SpaceEditorPage`, phone-scan flow). Sub-view under Tables via `?tab=tables&tablesView=spaces` (+ optional `spaceId=`).
- **Live map**: `components/business/tables/TablesLiveMap.tsx` — published layout canvas, statuses from existing `/tables/status`, click → existing `TableDetailModal`. List|Map toggle on Live View only when a published space exists.
- **Unassigned legacy tables**: `UnassignedTablesPanel` on Spaces overview assigns existing QR tables to a space (IDs/codes preserved) via `POST …/spaces/:id/tables/assign`.
- **Reservations**: `ReservationTablePicker` soft-groups by `space_id`/`space_name` when present; flat list otherwise.
- **Onboarding**: optional layout chip after tables exist (`spacesTables.onboarding.*`); deep-link `tables?tablesView=spaces`. Layout never required for go-live.
- **API client**: `src/api/spaces.ts`. Scan env: `SPACE_SCAN_*` in root `.env.example`.

### Marketing Creative System (`components/business/Marketing/`)
- `MarketingDashboard` composes the approval-first queue, library/history, creative settings drawer, editor drawer, and export outcome confirmation.
- `hooks/useMarketingSuggestions`, `useSuggestionCaptions`, `useMarketingSettings`, `useMarketingActivity` and `useCampaignKitExport` own server state; `usePostComposer` owns the current editable session.
- `marketingCapabilities.ts` maps the authenticated principal and effective staff permissions to read, write, generation, upload/gallery, and owner-approval controls.
- `templates/renderPost.ts` is the deterministic renderer for Editorial, Bold, and Minimal feed/story/landscape assets; activity history stores the exact creative snapshot used.
- `src/api/marketing.ts` is the typed API boundary. The UI exports or shares assets and records a post only after explicit operator confirmation; it does not schedule or automatically publish.

## State Management
| State type | Tool | Location |
|---|---|---|
| Server state | TanStack Query | per-`src/api/<domain>.ts` |
| Global client state | Zustand | `src/store/` (`useUserStore.ts`, `store.tsx`, `ui/`) |
| Auth | React Context (Wagmi-backed) | `src/providers/HybridAuthProvider.tsx` |
| i18n | React Context | `src/i18n/` (GuestTranslationProvider, SimpleTranslationProvider) |
| Form state | React Hook Form + Yup/Zod | per-component |

## API Client Layer
`src/api/` has 71 files — one per domain, each paired with `<name>.test.ts`. Examples: `accounting.ts`, `admin.ts`, `analytics.ts`, `auth/`, `bills.ts`, `crm.ts`, `currency.ts`, `customerTable.ts`, `delivery.ts`, `directorConsole.ts`, `errorLogs.ts`, `print.ts`, …

Axios instance is **client-only**; in server components and tests use raw `fetch`. The shared instance silently short-circuits under SSR.

## Tailwind Tokens
- Colors: `brand` (#1a6b6a teal), `ink` (warm gray), `warm` (cream `#faf9f6`), `emerald`/`amber`/`rose` for semantics. Off-palette colors (slate/blue/indigo) banned by ESLint.
- Fonts: `font-title` = DM Serif Display; `--font-inter` Tailwind var is **bound to DM Sans** (legacy name).
- Dark mode: `darkMode: "class"` — scaffolded but no runtime toggle ships. Avoid scattering `dark:` variants.

## Build Flags
- `NEXT_OUTPUT_STANDALONE=1` → standalone bundle for Docker.
- `NEXT_STRICT_BUILD=1` / `CI=true` → enforce TS + ESLint on build.
- Runtime config: `src/config/publicConfig.ts` reads `PUBLIC_ENV_KEYS` from the process environment at request time and publishes them as `window.__PAYVERGE_ENV__`. Examples: `PUBLIC_URL`, `NETWORK`, and `PUBLIC_RPC_URL` (the runtime name for the `RPC_URL` key).
- `NEXT_PUBLIC_*` build args are only a legacy fallback, used when that runtime env and `window.__PAYVERGE_ENV__` are both empty.
