# Payverge — Implementation Backlog

> **Generated 2026-05-29**, **updated 2026-05-30**, from a grounded, read-only codebase
> audit (5-agent fan-out). Every file path, line number, count, and locale list below was
> verified against the tree — not pulled from memory. This is a living dump of outstanding
> work; update or prune items as they ship. **2026-05-30:** the three i18n items (§1.1–1.3)
> are now resolved — see each section.

This file consolidates **everything currently deferred or stubbed** across the app, so
there's one place to look instead of chasing TODOs, `ComingSoon` flags, and 501 stubs
scattered through the code. It is **not** an implementation plan — when an item is picked
up, write a focused spec/plan in its GitHub issue or pull request description.

---

## Status & priority key

| Field | Values |
|-------|--------|
| **Status** | 🟥 Open · 🟦 Deferred-by-decision (intentional, safe stub in place) · 🟩 Resolved (recorded for completeness) |
| **Priority** | P1 (correctness/compliance) · P2 (product value) · P3 (polish / nice-to-have) · — (gated on a business decision) |
| **Effort** | S (<1 day) · M (1–3 days) · L (week+) · XL (multi-week / multi-locale) |

Priorities are an engineering read, not a product mandate — re-rank freely.

---

## 1. Internationalization

### 1.1 Per-locale transactional email templates — NOT a bug (by design, now enforced)
- **Status / Priority / Effort:** 🟩 Resolved 2026-05-30 · — · done
- **The reframe:** This was logged as an "18-locale gap," but the 18 locales
  (`ar, de, fr, hi, it, ja, ko, nl, pt, ru, th, tr, zh, vi, pl, sv, da, no`) are
  `operatorLocale:false`, `publishable:false`, with `requiredSurfaces:["guestStorefront"]`
  **only**. Transactional email is an **operator** surface, and the registry contract
  explicitly does not require emails for guest-storefront-only locales. Their English
  fallback is the **designed** behavior — only `en`/`es`/`es-AR` are operator/email locales,
  and all three already ship complete, genuine families. Translating 828 templates nobody's
  contract asks for would have been wrong.
- **What was done instead (enforce the contract):**
  - Added `locales.IsEmailLocale(code)` (keys on the `"emails"` required surface), mirroring
    the existing `IsPromptLocale` — `backend/internal/locales/locales.go`.
  - Refactored `templateLanguageFamilies()` to **derive** from the registry instead of a
    hardcoded `["eng","es","es_ar"]` — single source of truth, self-maintaining —
    `backend/internal/emails/templates.go`.
  - Added `TestEmailFamiliesMatchRegistryContract` —
    `backend/internal/emails/templates_surface_contract_test.go` — which fails the build if a
    locale is promoted to an email locale without shipping its template family + base layout
    (or if an orphan family exists). The silent English fallback can no longer hide a real gap.
- **If a market is later promoted:** add `"emails"` to that locale's `requiredSurfaces`,
  ship `backend/email/templates/<family>/` (46 templates) + `layout/base_<family>.html`; the
  contract test then enforces completeness. That promotion is a **business call**, not a bug.

### 1.2 Per-locale AI prompt templates — NOT a bug (by design, now enforced)
- **Status / Priority / Effort:** 🟩 Resolved 2026-05-30 · — · done
- **The reframe:** Identical story to 1.1. Prompts (`director_console`, `menu_wizard`) are an
  operator surface; the same 18 guest-only locales don't require them. `IsPromptLocale`
  already gates selection at runtime, and es/en/es-AR prompts are genuine.
- **What was done (enforce the contract):** Added
  `TestPromptFamiliesMatchRegistryContract` —
  `backend/internal/services/prompts_surface_contract_test.go` — asserting every
  prompt-required locale has both `.md` files embedded, and no orphan prompt family ships.
  (Verified to have teeth: planting an orphan `fr.md` makes it fail.)

### 1.3 es-AR legal & terms Rioplatense localization ("Batch B") — DONE
- **Status / Priority / Effort:** 🟩 Resolved 2026-05-30 · P3 · done
- **What was the gap:** `legal.json`'s `terms` + `refund` sections used informal Peninsular
  **tú** ("aceptas", "debes", "puedes", "contáctanos"), so es-AR operators saw non-Argentine
  legal copy; the namespaces were excluded from the voseo gate.
- **What was done:**
  - Added `frontend/src/i18n/messages/es-ar/legal.json` — a voseo override of the informal
    `terms` + `refund` copy (tú→vos, `ti`→`vos`, `contigo`→`con vos`), carrying full arrays
    where any element diverged. The **formal usted** passages (the `privacy` policy and the
    `terms.json` investment terms) are intentionally left inherited — Argentine legal register
    uses usted too, and the change is register-only, never legal meaning.
  - Wired `legal` into `messages/es-ar/index.ts`.
  - Removed the `DEFERRED = /^(legal|terms)\./` exclusion from
    `operator-es-ar-voseo.test.ts` so legal/terms now ride the regression gate.
  - Adversarially verified (3 independent reviewers: Rioplatense linguistics, legal-meaning
    preservation, structural/contract integrity) — all clean: no Peninsular leak, no
    over-correction, no meaning/number/email/jurisdiction drift, no orphan keys.
- **Not touched (correctly):** the guest tier has no legal/terms keys; the stale
  vehicle-investment `terms.json` content (UAE/Dubai, "tokenized vehicle ownership") is a
  separate **content-staleness** problem, out of scope for a register pass.

### 1.4 Neutral `es` copy polish (optional)
- **Status / Priority / Effort:** 🟩 Resolved-enough · P3 · M
- **What:** The neutral `es` bundle was never wholesale-rewritten during the es-AR work —
  it's already decent human-quality. Logged only so it isn't mistaken for a gap. Pick up
  only if a dedicated quality pass is wanted.

---

## 2. Payments & money

### 2.1 Split payment execution — held-share settlement
- **Status / Priority / Effort:** ✅ Resolved · P1 · L
- **What changed:** `POST /guest/bill/:bill_number/split/execute` is no longer a 501 stub.
  Split shares persist as `bill_split_shares`, guest-held shares settle through the
  existing payment rails, and split state is exposed to guests/operators for live progress.
- **Files:** `backend/internal/handlers/splitting.go`,
  `backend/internal/database/bill_split.go`,
  `frontend/src/components/splitting/GuestBillSplitPanel.tsx`
- **Evidence:** Regression coverage now asserts persisted share settlement, idempotent
  execution, receipt access, held-share release/refund behavior, and provider-backed
  split payment paths. Remaining refund-provider limitations are tracked separately in
  2.2.
- **Per-payer receipt delivery — intentional in-app-only (by design):** A settled split
  share's receipt is itemized, tender-specific, guest-session-scoped, settled-only, and
  recoverable after a refresh (`GetGuestBillSplitShareReceipt` + `getMySplitShares`). It is
  delivered **in-app, not by email** — split is no-login by design ("zero email/account
  requirement"), so no payer email is collected and the existing `SendPaymentReceiptEmail`
  path is deliberately not wired in. Emailing a split receipt would require an **optional**
  email capture at pay time; deferred per decision (2026-06-13) and not a gap against the
  no-login product principle.

### 2.2 Card refunds via live PSP API (Stripe / PayPal / MercadoPago)
- **Status / Priority / Effort:** ✅ Resolved (PSP card) · 🟦 crypto original-tender deferred · P2 · M
- **What changed:** `RefundBillPayment` now calls the provider refund hook **before** the
  local ledger is reversed for plugin-backed (card) payments. Stripe, PayPal, and
  MercadoPago each implement `RefundPayment`, so the customer's card is genuinely credited.
  If the provider call fails or reports the operation unsupported, the payment stays
  confirmed and the local ledger is left untouched (409/502, no false local-only refund).
- **Files:** `backend/internal/handlers/bill_void_refund.go`
  (`refundBillPaymentWithExternalTender` → `refundPluginTenderIfNeeded`),
  `backend/internal/plugins/stripe/stripe.go` (`RefundPayment`, live `POST /v1/refunds`),
  `backend/internal/plugins/paypal/paypal.go` (`RefundPayment`),
  `backend/internal/plugins/mercadopago/mercadopago.go` (`RefundPayment`)
- **Evidence:** `backend/internal/handlers/bill_void_refund_test.go`,
  `backend/internal/plugins/stripe/stripe_refund_test.go`,
  `backend/internal/plugins/paypal/paypal_refund_test.go`,
  `backend/internal/plugins/mercadopago/mercadopago_refund_test.go`.
- **Remaining limitation (honest):** Direct-crypto / original-tender refunds (USDC,
  cross-chain LI.FI) are **not** auto-refunded to the original tender.
  `paymentRequiresOriginalTenderRefund` short-circuits these non-plugin tenders to a 409
  (`errPluginRefundUnavailable`) so Payverge never books a false local-only reversal of an
  on-chain payment — the operator must refund on-chain out of band. Closing this needs
  origin-tender replay (`source_token`/`source_chain` per the crypto-resume idempotency
  rule) and is **deferred per standing decision** until a dedicated initiative. (The
  stablecoin-plugin split-refund handling was reverted off `main` with the rest of that
  plugin's split work; the plugin itself has since been removed entirely.)
  Cashier/alternative (cash/operator-attested) payments remain intentional local reversals
  via `RefundBillAlternativePayment`.

### 2.3 Subscription cancellation follow-up workflow (Stripe cancel)
- **Status / Priority / Effort:** 🟥 Open · P2 · M
- **What:** The `Business` model carries a `SubscriptionsCancelPending` flag for a
  Stripe-cancel follow-up, but the workflow that acts on it isn't implemented.
- **Files:** `backend/internal/database/models.go` (`SubscriptionsCancelPending`)
- **Scope to close:** On owner cancel request, call Stripe subscription cancellation +
  sync billing-platform state; clear the flag on confirmation.

---

## 3. Printing

### 3.1 Thermal printer Sprint 2 formatters (bar / kitchen / void / modify)
- **Status / Priority / Effort:** 🟥 Open · P2 · M
- **What:** `PrintJobKind` enumerates `bar`, `kitchen`, `void`, `modify`, but
  `renderHTML()` rejects them with "kind %q not implemented in Sprint 1." The kinds are
  model-complete; only the formatter logic is missing.
- **Files:** `backend/internal/services/print/service.go:188` (+ kinds in `models.go`)
- **Scope to close:** Implement an HTML formatter per kind. Remember the printer gotchas:
  payloads are **dollars not cents** (already converted via `MarshalJSON`), and
  `window.print()` is client-only. `FEATURE_FLAGS.printers` is still dark by default.

---

## 4. Plugins / integrations (coming-soon)

### 4.1 QuickBooks accounting sync
- **Status / Priority / Effort:** 🟥 Open · — (roadmap) · L
- **What:** Registered in the plugin catalog with `ComingSoon: true` ("connect restaurant
  sales, payments, and reporting to your accounting workflow"). Category: Integration.
- **Files:** `backend/internal/services/plugin_init.go` (QuickBooks entry)
- **Scope to close:** QBO OAuth + real-time sync orchestration, then flip `ComingSoon`.

### 4.2 Tax reporting exports
- **Status / Priority / Effort:** 🟥 Open · — (roadmap) · M
- **What:** Catalog entry with `ComingSoon: true` ("summarize sales, taxes, tips, service
  fees, and payment activity"). Category: Reporting.
- **Files:** `backend/internal/services/plugin_init.go` (Tax Reports entry)
- **Scope to close:** Sales/tax/tip/fee aggregation + export formats, then flip `ComingSoon`.

---

## 5. Loyalty

### 5.1 Per-business redemption rate configuration
- **Status / Priority / Effort:** 🟥 Open · P3 · S
- **What:** `defaultRedemptionRate` (100 points = $1) is a hardcoded global constant; it
  can't be overridden per business on the `LoyaltyProgram` model.
- **Files:** `backend/internal/loyalty/redemption.go:17-18` (TODO)
- **Scope to close:** Add a nullable rate column to `LoyaltyProgram` (migration), admin UX
  to set it, and fall back to the global default when unset.

---

## 6. Dashboard / AI

### 6.1 AI-inbox pending-action field in `DashboardSummary`
- **Status / Priority / Effort:** 🟥 Open · P3 · S
- **What:** `DashboardSummary` is missing an `ai_pending` field. The frontend currently
  shows the AI-inbox action whenever `active_bills > 0` instead of when AI is actually
  processing.
- **Files:** `frontend/src/components/business/BusinessOverview.tsx:555` (`TODO(backend)`)
- **Scope to close:** Add `ai_pending` to the `DashboardSummary` serializer (backend), then
  gate the frontend action on it. Mind the backend performance gate — `DashboardSummary`
  is a dashboard polling path, so add the field without widening the query shape.

---

## 7. Compliance / data lifecycle

### 7.1 Account hard-delete cron sweep (GDPR / UAE PDPL)
- **Status / Priority / Effort:** 🟥 Open · P1 · M
- **What:** Account soft-delete is implemented (`DeletionScheduledAt = now() + 30d`), but
  there's no background job that permanently purges rows past the 30-day expiry.
- **Files:** `backend/internal/database/models.go` (`DeletionScheduledAt`)
- **Scope to close:** A scheduled sweep that hard-deletes (or anonymizes) accounts past
  `DeletionScheduledAt`, with audit logging and a dry-run/safety guard. Compliance-relevant
  — a soft-delete that never purges doesn't satisfy a deletion request.

---

## 8. Onboarding unification — essentially complete

### 8.1 Manual QA checklist on a live dashboard
- **Status / Priority / Effort:** 🟩 Resolved (impl) / 🟥 Open (QA only) · P3 · S
- **What:** All 18 tasks of the onboarding-unification plan are implemented and verified by
  code inspection + typecheck (welcome step stripped, `tutorial.takeATour` rename,
  `FirstRunWizard` + `businessTutorialModal` deleted, doc deprecated). The **only**
  remainder is a manual QA pass of the onboarding hub against a running dashboard.
- **Files / refs:** the onboarding-unification design (an internal spec, not part of
  this tree); post-delivery fixes in commits `a81e4f61`, `f89e248e`, `4cc9a5ad`.
- **Scope to close:** Execute the manual QA checklist on a live server; record the result.

---

## Resolved during the es-AR initiative (no action — recorded so they aren't re-opened)

- **es-AR blog content** — 20/20 post parity with `en`/`es`, genuine Rioplatense
  (slugs `mozo`/`contable`/`combos`, localized internal links). Complete.
- **es-AR AI prompts** — `director_console` + `menu_wizard` are genuine voseo. Complete.
- **es-AR operator UI + guest bundle** — voseo override layer over `es` (operator) and a
  complete 21-locale-parity guest bundle, both behind regression gates. Complete.
- **es-AR backend routing** — gift emails, Director Console pill labels, Google-Translate
  target, and SEO hreflang clusters all fixed in the 2026-05-29 best-state audit
  (`4fabef53`→`a7ff1429`).
- **es-AR legal/terms** — Argentinized (voseo) and un-deferred from the gate; see §1.3
  (2026-05-30).
- **Email + prompt locale surfaces** — the "18-locale gaps" were contract-backed design,
  now enforced by `IsEmailLocale` + two surface-contract tests; see §1.1 / §1.2 (2026-05-30).

---

### How this was generated

5 parallel read-only agents inventoried: email templates, AI prompts, blog content, the
es-AR legal/terms stream, and a repo-wide TODO/`ComingSoon`/501-stub sweep. Findings were
cross-checked against `locales/registry.json` and the relevant Go/TS source. Re-run the
sweep after major feature work to keep this list honest.
