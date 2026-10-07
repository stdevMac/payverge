# Invoice / Fiscal Compliance Architecture

Last updated: 2026-05-22

This document describes the current invoice/fiscal compliance implementation in Payverge after the latest frontend productization pass. The user-facing product is now called **Invoices**. The backend still uses the word **fiscal** because it models country-specific official document issuance, audit records, provider contracts, and tax-authority integration boundaries.

## Product Intent

Payverge already has bills, payments, receipts, accounting, and payment plugins. Invoices add a separate official-document layer for markets where the restaurant may need an authorized fiscal receipt or e-invoice tied to a paid sale.

The key product principle is operator control:

- Businesses can leave invoice automation off.
- Businesses can issue manually.
- Businesses can choose automatic non-blocking issuance.
- Businesses can choose automatic blocking behavior where appropriate.

We are giving restaurants tools and auditability rather than forcing every operator into a compliance workflow by default.

## User-Facing Placement

Invoices live inside the **Accounting** dashboard, not as a standalone Fiscal navigation item.

Current route behavior:

- `/business/:businessId/dashboard?tab=accounting` opens Accounting normally.
- `/business/:businessId/dashboard?tab=fiscal` remains supported as a legacy deep link.
- Legacy `tab=fiscal` now renders Accounting with the internal **Invoices** tab selected.
- The sidebar highlights **Accounting** for legacy fiscal deep links.
- Fiscal is no longer shown as a separate item under More.

Why:

- Operators understand invoices as part of financial/accounting work.
- A separate "Fiscal" tab felt too raw and implementation-oriented.
- Keeping `tab=fiscal` avoids breaking saved URLs, browser history, and any customer/internal links created during the first implementation.
- The sidebar stays cleaner, especially after moving official invoices into Accounting.

## Frontend Architecture

### Route Bridge

File:

- `frontend/src/app/(shop)/business/[businessId]/dashboard/page.tsx`

The dashboard still accepts `"fiscal"` in `validTabs` for backward compatibility. In `renderTabContent`, the `fiscal` case now renders:

```tsx
<AccountingDashboard
  businessId={numericBusinessId.toString()}
  initialTab="invoices"
/>
```

The `DashboardLayout` receives `activeTab="accounting"` when the visible active tab is fiscal. That is why the sidebar highlights Accounting while the URL can still say `?tab=fiscal`.

### Accounting Internal Tab

File:

- `frontend/src/components/business/AccountingDashboard.tsx`

Accounting now supports:

```ts
type AccountingDashboardTab = "overview" | "entries" | "payroll" | "invoices";
```

It accepts:

```ts
initialTab?: AccountingDashboardTab;
```

That prop is used by the legacy fiscal route so a deep link can open directly on Invoices without creating a second dashboard page.

### Invoice Dashboard

File:

- `frontend/src/components/business/fiscal/FiscalDashboard.tsx`

Despite the file/package name, this component now renders the user-facing **Invoices** experience. It owns the invoice setup and history surface:

- Summary tiles:
  - Mode
  - Country
  - Pending
  - Failed
  - Last issued
- Invoice setup:
  - What this does
  - Where it works
  - Tax profile
  - Issuance mode
  - Review
- Country/provider cards:
  - Argentina / ARCA
  - United Arab Emirates / EDICOM
- Editable setup fields:
  - Country
  - Provider
  - Tax ID
  - Tax condition
  - Point of sale
  - Invoice mode
  - Environment
- Invoice history:
  - Bill ID manual issue control
  - Receipt status table
  - Retry action only for retryable failures
  - Empty state when no receipts exist

The old split components were removed:

- `FiscalSettings.tsx`
- `FiscalReceiptList.tsx`

Why:

- The old UI exposed backend concepts too directly: Country, Provider, Mode, Environment, Tax ID, Tax condition, Point of sale, and raw receipt list in plain cards.
- The new UI explains the workflow without making the user feel like they are configuring an integration registry.
- A single component is appropriate for now because setup, summary, and history all depend on the same fiscal settings and receipts fetch.

### Frontend API Client

File:

- `frontend/src/api/fiscal.ts`

Current API methods:

```ts
getSettings(businessID)
updateSettings(businessID, payload)
listReceipts(businessID, status?)
issueReceipt(businessID, billID)
retryReceipt(businessID, receiptID)
```

The frontend still talks to `/fiscal` endpoints because the backend domain is fiscal compliance. The UI translation layer is where we turn that into invoice-oriented copy.

### Sidebar IA

Files:

- `frontend/src/components/business/sidebar/sidebarConfig.ts`
- `frontend/src/components/business/DashboardSidebar.tsx`

`SECONDARY_TABS` now contains Accounting but not Fiscal. The More disclosure was also cleaned up so it shows the label and chevron only, without a raw count like `More 5`.

Why:

- Accounting is the entry point.
- The raw More count was visually noisy and could read as `More5` in accessible text extraction.

### i18n

Files:

- `frontend/src/i18n/messages/en/fiscal.json`
- `frontend/src/i18n/messages/es/fiscal.json`
- `frontend/src/i18n/messages/en/businessDashboard.json`
- `frontend/src/i18n/messages/es/businessDashboard.json`

The copy now says "Invoices" / "Facturas" at the UI level. The backend/API naming remains fiscal.

## Backend Architecture

### Domain Boundary

Package:

- `backend/internal/fiscal/`

Fiscal compliance is intentionally separate from:

- Payment plugins
- Accounting
- Printing
- Bill management

Reasoning:

- Payment plugins answer: how did the restaurant collect money?
- Fiscal providers answer: what official document exists for this sale?
- Accounting answers: how should money and expenses be reported?
- Printing answers: what physical artifact should be produced?

Those workflows touch the same bill, but they should not own each other's state.

### Database Tables

Schema: part of the genesis baseline,
`backend/schema/genesis/current_schema.sql`.

Tables:

- `business_fiscal_settings`
- `fiscal_receipts`
- `fiscal_jobs`
- `fiscal_audit_events`

What each table does:

- `business_fiscal_settings`: stores per-business fiscal setup such as country, provider, mode, environment, tax ID, point of sale, credential metadata, setup status, and validation errors.
- `fiscal_receipts`: durable official-document records tied to bills, payments, provider receipt IDs, auth codes, QR payloads, PDFs, status, provider errors, and raw provider request/response payloads.
- `fiscal_jobs`: idempotent issuance/retry queue records. These prevent duplicate official-document attempts when webhooks or staff actions repeat.
- `fiscal_audit_events`: planned audit trail for receipt/job lifecycle events.

### Service Layer

File:

- `backend/internal/fiscal/service.go`

Responsibilities:

- Normalize and validate settings.
- Store settings through the repository.
- List receipts.
- Decide whether a paid bill should enqueue an invoice job.
- Create idempotent issue jobs.
- Create or reactivate retry jobs for retryable receipts.

Current fiscal modes:

```go
off
manual
automatic_non_blocking
```

Current behavior:

- `off`: no invoice jobs are created.
- `manual`: staff can manually request issuance, but paid-bill automation does nothing.
- `automatic_non_blocking`: paid bills enqueue jobs automatically.

Important limitation:

The current backend foundation enqueues fiscal jobs and defines provider boundaries, but it does not yet include the production worker that consumes `fiscal_jobs`, calls providers, and writes final `fiscal_receipts` from provider responses. That worker/credential path is the next major backend step before making this a real production compliance engine.

### Repository Layer

File:

- `backend/internal/fiscal/repository.go`

Responsibilities:

- Get latest settings for a business.
- Upsert settings by business/country/provider.
- Preserve credential metadata when updating the same provider.
- Create jobs idempotently using `idempotency_key`.
- List recent receipts with optional status filtering.

Why idempotency matters:

Payment webhooks, manual staff retries, and plugin confirmation paths can fire more than once. Official invoices must not be duplicated just because Payverge received the same payment event twice.

### Provider Contract

File:

- `backend/internal/fiscal/provider.go`

The provider interface is intentionally small:

```go
type Provider interface {
  Country() string
  Name() string
  ValidateSettings(ctx context.Context, settings Settings) error
  IssueReceipt(ctx context.Context, input IssueInput) (*ReceiptResult, error)
  IssueCreditNote(ctx context.Context, input CreditNoteInput) (*ReceiptResult, error)
  GetStatus(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error)
}
```

Why:

- The fiscal service should not know about ARCA WSFE, CAE, QR internals, UAE ASP transport, UBL, Peppol, or provider-specific request formats.
- Country/provider adapters own those details.
- This keeps future providers additive rather than invasive.

### Argentina Provider Foundation

Package:

- `backend/internal/fiscal/providers/ar/`

Current pieces:

- `provider.go`: ARCA provider shape using a `WSFEClient` interface.
- `mapper.go`: maps Payverge issue input into WSFE payload shape.
- `qr.go`: builds ARCA QR verification payload.
- Tests for mapping, provider flow, and QR generation.

Current state:

- The provider is mockable and test-covered.
- It expects an injected WSFE client.
- Production WSAA certificate handling and real WSFE client registration are not wired into `main.go` yet.

### UAE

No UAE fiscal provider ships. Fiscal providers are pluggable via the provider interface in `backend/internal/fiscal/provider.go`; only AR/ARCA (`backend/internal/fiscal/providers/ar/`) is implemented.

Why:

- UAE EIS depends on Accredited Service Providers.
- Current UAE consumer restaurant bills are not the immediate required path.
- Corporate hospitality, B2B, B2G, catering, hotel F&B, and account billing are the likely future use cases.
- A UAE provider waits on an accredited service provider contract.

## API Surface

Registered in:

- `backend/cmd/app/main.go`

Routes:

```text
GET  /api/v1/inside/businesses/:id/fiscal/settings
PUT  /api/v1/inside/businesses/:id/fiscal/settings
GET  /api/v1/inside/businesses/:id/fiscal/receipts
POST /api/v1/inside/businesses/:id/fiscal/receipts/issue
POST /api/v1/inside/businesses/:id/fiscal/receipts/:receiptId/retry
```

RBAC permissions:

```text
fiscal:read
fiscal:write
fiscal:issue
fiscal:retry
fiscal:export
```

Why RBAC is separate:

Reading invoice records, changing setup, issuing official documents, retrying failures, and exporting accountant data are different operational permissions.

## Data Flows

### 1. Save Invoice Setup

```text
Invoices UI
  -> updateSettings()
  -> PUT /fiscal/settings
  -> FiscalHandlers.UpdateSettings
  -> fiscal.Service.UpdateSettings
  -> Repository.UpsertSettings
  -> business_fiscal_settings
```

The frontend validates point of sale as a positive safe integer when present. The backend normalizes country/provider/environment and rejects unsupported combinations.

### 2. Manual Issue From Bill ID

```text
Invoices UI
  -> issueReceipt(businessID, billID)
  -> POST /fiscal/receipts/issue
  -> FiscalHandlers.IssueReceipt
  -> fiscal.Service.IssueReceipt
  -> CreateJobIfNotExists
  -> fiscal_jobs
```

Current behavior is job creation, not immediate provider authorization.

### 3. Paid Bill Automation

Payment/plugin paths call fiscal enqueue helpers when a bill transitions to paid:

- crypto payments
- cross-chain payments
- plugin payments
- alternative payment confirmation
- webhook payment confirmation

Flow:

```text
Payment marks bill paid
  -> enqueueFiscalJobForPaidBill(...)
  -> fiscal.Service.HandleBillPaid
  -> check bill status and paid amount
  -> check active settings and mode
  -> CreateJobIfNotExists
  -> fiscal_jobs
```

Manual and off modes intentionally do not enqueue automatic jobs.

### 4. Retry

```text
Invoices UI
  -> retryReceipt(businessID, receiptID)
  -> POST /fiscal/receipts/:receiptId/retry
  -> FiscalHandlers.RetryReceipt
  -> fiscal.Service.RetryReceipt
  -> only allows failed_retryable
  -> creates or reactivates a pending retry job
```

Why only `failed_retryable`:

Permanent failures, rejected documents, cancelled receipts, and already-authorized documents should not be blindly retried from the UI.

## Why Some Names Still Say Fiscal

The UI says **Invoices** because that is what restaurant operators understand.

The backend says **fiscal** because:

- It models tax-authority/official-document concerns.
- It has to cover receipts, invoices, credit notes, provider auth codes, QR payloads, audit events, and country policy.
- Future countries may call the documents different names.

This split is intentional. It lets us keep product language friendly while preserving a precise backend domain.

## Current Test Coverage

Backend coverage includes:

- Fiscal model table names and enum validation.
- Repository upsert/idempotency behavior.
- Service paid-bill enqueue decisions.
- Manual issue and retry rules.
- Fiscal handler request/response behavior.
- RBAC permission registration.
- Payment/plugin paths enqueueing fiscal jobs.
- Argentina mapper, provider, and QR behavior.

Frontend coverage includes:

- Fiscal API client behavior.
- Invoice dashboard rendering, setup save, validation, manual issue, and retry actions.
- Accounting internal `Invoices` tab.
- Legacy `tab=fiscal` deep link behavior.
- Sidebar grouping and More disclosure behavior.

## Current Limitations / Next Work

The current implementation is a strong foundation, but not yet the full production compliance engine.

Next backend work:

- Register real providers in `main.go`.
- Add encrypted credential upload/storage UX and backend validation.
- Implement the fiscal job worker that consumes `fiscal_jobs`.
- Persist provider results into `fiscal_receipts`.
- Write audit events for settings, issue, retry, provider response, and failure transitions.
- Add provider health/status checks.

Next frontend work:

- Add bill-detail issue entry point.
- Add fiscal exception surfaces in Accounting.
- Add accountant export flow.
- Show official invoice data on guest receipts when available.
- Add credential/setup validation states beyond the current draft setup.

Next product/spec work:

- Decide Argentina receipt type rules for first customers.
- Decide whether UAE starts as B2B-only manual issuance.
- Select UAE ASP before implementing a live provider.
- Define how much legal/compliance guidance the product copy should include versus deferring to accountant/legal advisors.

## Design Decisions Worth Remembering

- **Invoices live under Accounting** because that matches user mental models better than a standalone Fiscal tab.
- **Legacy fiscal URLs still work** to avoid breaking links.
- **Fiscal providers are not payment plugins** because money collection and official-document authorization are different responsibilities.
- **Modes exist because restaurants need choice**; compliance automation should not surprise operators.
- **Automatic non-blocking is safer as a default** because provider downtime should not casually stop restaurant payment settlement.
- **Jobs are idempotent** because payment systems retry.
- **Retry is limited to retryable failures** because official documents should not be spammed.
- **UAE is provider-ready but not overbuilt** because the ASP choice matters.
- **Frontend says invoices; backend says fiscal** because one is product language and the other is domain language.
