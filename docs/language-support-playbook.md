# Payverge Language Support Playbook

Last investigated: 2026-05-14. The blog, marketing SEO pages and the Docusaurus
site this investigation also covered have since been removed from the
repository. For the current step-by-step procedure see
[`locales/LANGUAGES.md`](../locales/LANGUAGES.md); `locales/registry.json` is
the single source of truth.

This document maps the current language surfaces in Payverge and gives a repeatable process for adding a new language or regional locale such as `es-AR`.

The short version: Payverge does not have one language system. It has several language surfaces that share language codes but have different responsibilities. A complete language rollout must account for all of them.

## Success Criteria For Full Language Support

A language is fully supported only when each relevant surface below has an explicit decision and implementation:

- Operator app UI copy resolves in the selected locale without leaking raw translation keys.
- Guest interface copy resolves in the selected locale or through an intentional fallback.
- Business/menu language records can be stored, selected, returned by APIs, and used for translated menu content.
- Emails either render in the requested language or intentionally fall back to a supported template family.
- Browser detection, user preference persistence, and authenticated language syncing preserve supported regional tags.
- Tests cover the supported code path and the fallback path.
- The process for adding the next language is documented and repeatable.

## Current Language Systems

### 1. Operator App UI

Files:

- `frontend/src/i18n/config.ts`
- `frontend/src/i18n/SimpleTranslationProvider.tsx`
- `frontend/src/i18n/useLanguage.tsx`
- `frontend/src/providers/HybridAuthProvider.tsx`
- `frontend/src/components/SimpleLanguageSwitcher.tsx`
- `frontend/src/components/FloatingLanguageSwitcher.tsx`
- `frontend/src/i18n/messages/en/`
- `frontend/src/i18n/messages/es/`

Current state:

- `frontend/src/i18n/config.ts` defines `locales = ['en', 'es']`.
- `Locale` is therefore only `en | es`.
- `SimpleTranslationProvider.tsx` imports only English and Spanish bundles and uses a static `messages` object.
- `useLanguage.tsx` treats only `en` and `es` as valid persisted user preferences.
- `HybridAuthProvider.tsx` applies `userData.language_selected` only when it equals `en` or `es`.
- `SimpleLanguageSwitcher.tsx` maps over `locales`, so it can grow if config/provider support grows.
- `FloatingLanguageSwitcher.tsx` is a binary mobile toggle: `en <-> es`.

Implication for `es-AR`:

- Adding `es-AR` only to `locales` is not enough.
- The operator app needs locale validation helpers, bundle fallback, and non-binary switcher behavior.
- If `es-AR` initially reuses the current Spanish copy, the provider should map `es-AR` to the `es` bundle rather than duplicating every JSON file.

### 2. Guest Interface

Files:

- `frontend/src/i18n/GuestTranslationProvider.tsx`
- `frontend/src/i18n/guest-messages/*.json`
- `frontend/src/components/guest/SimpleLanguageSelector.tsx`
- `frontend/src/components/guest/FloatingLanguageSelector.tsx`
- `frontend/src/components/guest/FloatingLanguageSelectorBusiness.tsx`
- `frontend/src/app/b/[customUrl]/BusinessPageClient.tsx`
- `frontend/src/app/b/[customUrl]/page.tsx`
- `frontend/src/app/t/[tableCode]/page.tsx`
- `frontend/src/app/t/[tableCode]/menu/page.tsx`
- `frontend/src/app/t/[tableCode]/bill/page.tsx`
- `frontend/src/api/publicBusiness.ts`
- `frontend/src/api/currency.ts`

Current state:

- `GuestTranslationProvider.tsx` has its own supported language list: `en`, `es`, `fr`, `de`, `it`, `pt`, `zh`, `ja`, `ko`, `ar`, `ru`, `hi`, `th`, `nl`, and `tr`.
- It dynamically imports `./guest-messages/${language}.json`.
- It sets `<html lang>` and `dir`, with `ar` marked RTL.
- Guest selected language is persisted as `guest-language-${businessId}`.
- Guest language selectors receive business languages from backend APIs, but only call `setLanguage` when the code exists in `GUEST_SUPPORTED_LANGUAGES`.
- Business page metadata emits alternates from `business.supported_languages`, including query URLs such as `/b/<slug>?lang=<code>`.

Implication for `es-AR`:

- If a business supports `es-AR`, guest menu translation can store and fetch that code, but guest UI copy will not switch unless `es-AR` is supported or aliased.
- Recommended first step is an alias map: `es-AR -> es` for guest UI messages.
- A distinct `frontend/src/i18n/guest-messages/es-AR.json` should be created only if the guest copy will actually be Argentine Spanish.

### 3. Backend Business/Menu Languages

Files:

- `backend/internal/database/currency.go`
- `backend/internal/services/translation.go`
- `backend/internal/handlers/currency.go`
- `backend/internal/server/business_menu_core_handlers.go`
- `backend/internal/server/business_settings_handlers.go`
- `backend/internal/server/public_guest_response_helpers.go`
- `backend/internal/database/plugins.go`

Current state:

- `SupportedLanguage.Code` is `gorm:"uniqueIndex;size:5;not null"`.
- `BusinessLanguage.LanguageCode` is `gorm:"size:5;not null"`.
- `Translation.LanguageCode` is `gorm:"size:5;not null"`.
- `PluginTranslation.LanguageCode` is `gorm:"size:5;not null"`.
- `es-AR` is five characters, so it fits the current schema.
- `InitializeDefaultLanguages()` seeds base languages but not `es-AR`.
- `LanguageService.SetBusinessLanguages()` stores whatever codes it receives and updates `business.default_language`.
- `CurrencyHandler.UpdateBusinessLanguages()` validates that the default code is in the submitted list, but it does not currently validate that each code exists in `supported_languages`.
- Menu translation APIs use exact `language_code` matches when reading and writing translations.

Implication for `es-AR`:

- No schema widening is required for `es-AR`.
- The backend seed list needs an `es-AR` row, for example:
  - `Code: "es-AR"`
  - `Name: "Argentine Spanish"`
  - `NativeName: "Español (Argentina)"`
- Add validation so unsupported or mistyped language codes are rejected before they become business settings or translation rows.
- For future BCP 47 tags longer than five characters, widen language columns before rollout.

Recommended future-proof DB width:

- `supported_languages.code`: `varchar(16)`
- `business_languages.language_code`: `varchar(16)`
- `translations.language_code`: `varchar(16)`
- `plugin_translations.language_code`: `varchar(16)`

Examples that need more than five characters:

- `zh-Hant`
- `pt-BR`
- `sr-Latn`
- `es-419`

### 4. Emails

Files:

- `backend/internal/emails/templates.go`
- `backend/internal/emails/templates_structure_test.go`
- `backend/email/templates/eng/`
- `backend/email/templates/es/`
- `backend/email/layout/base_eng.html`
- `backend/email/layout/base_es.html`
- `backend/internal/services/subscription_checker.go`
- `backend/internal/services/subscription_stripe.go`
- `backend/internal/handlers/subscription_stripe_handlers.go`
- `backend/internal/handlers/admin_user_handlers.go`
- `backend/internal/services/delivery_notifications.go`

Current state:

- Template families are `eng` and `es`.
- `TemplateManager.normalizeTemplateLanguage()` already maps any code with prefix `es` to Spanish and `en` or `eng` to English.
- `templates_structure_test.go` already verifies that `tm.Render("es-AR", "welcome_core", ...)` renders the Spanish template.
- Several service-specific email normalizers still use exact `== "es"` checks:
  - `normalizeBillingLanguage`
  - `normalizeGiftLanguage`
  - `normalizeStripeLanguage`
  - `adminNormalizeLanguage`
- Delivery notifications use prefix normalization and already treat `es-AR` as Spanish.

Implication for `es-AR`:

- Core email template rendering already supports `es-AR`.
- Service-specific normalizers should be unified behind one shared helper so `es-AR` does not accidentally fall back to English before reaching the template manager.
- A separate `es-AR` email template directory is optional and should only be added if Argentine-specific email copy is needed.

## Recommended Locale Architecture

Add a locale registry instead of continuing to scatter arrays and string checks.

Frontend registry responsibilities:

- canonical locale code, for example `es-AR`
- display name, for example `Español (Argentina)`
- direction: `ltr` or `rtl`
- operator UI message bundle code, for example `es`
- guest UI message bundle code, for example `es`
- browser detection aliases
- default fallback chain, for example `es-AR -> es -> en`

Backend registry responsibilities:

- canonical locale code
- supported-language seed data
- email template language mapping
- translation-provider target code
- validation helpers for API inputs

The frontend and backend cannot literally share one source file because one is TypeScript and the other is Go. They should share the same contract and each have tests that keep the same canonical codes valid.

Recommended naming:

- Store canonical BCP 47-style tags in user/business settings, for example `es-AR`.
- Preserve canonical case in database/API values.
- Do not strip region tags in browser detection when the regional tag is supported.

## Recommended `es-AR` Rollout

### Phase 1: Supported Locale With Spanish Fallback

This gets Argentine Spanish selectable without duplicating all Spanish copy.

Backend:

- Add `es-AR` to `InitializeDefaultLanguages()`.
- Add tests that `InitializeDefaultLanguages()` seeds `es-AR`.
- Add language-code validation for `UpdateBusinessLanguages` and `SetBusinessLanguages`.
- Keep menu translation storage exact: translated menu rows for `es-AR` should use `language_code = 'es-AR'`.
- If the translation provider does not support region-specific translation targets, call the provider with `es` while storing the result under `es-AR`.

Frontend operator UI:

- Add a locale registry and `isSupportedLocale()` helper.
- Add `es-AR` to supported operator locales.
- Map `es-AR` to the existing `es` message bundle.
- Update `SimpleTranslationProvider` to resolve bundle locale through the registry.
- Update `useLanguage.tsx` and `HybridAuthProvider.tsx` to validate through the registry instead of hard-coded `en/es`.
- Replace the binary mobile language toggle with a dropdown-style selector or hide it when more than two locales exist. The current binary toggle does not scale.

Guest UI:

- Add `es-AR` to guest-supported languages or add a guest alias map.
- Recommended first implementation: `guestMessageAlias['es-AR'] = 'es'`.
- Update guest selectors so a business language can control menu translation even when UI copy falls back to a base bundle.
- Keep the persisted guest choice as `es-AR`, not `es`, so menu translations and business settings stay exact.

Emails:

- Reuse Spanish templates for `es-AR`.
- Replace exact `== "es"` service normalizers with a shared prefix-aware helper.
- Keep `TemplateManagerRenderNormalizesLanguageCodes` as the core render test.

Tests:

- Frontend unit: `getTranslation("common.yes", "es-AR")` returns Spanish copy through fallback.
- Frontend unit: locale registry validates `es-AR`.
- Guest unit: provider can initialize with `es-AR` and load Spanish guest messages through alias.
- Backend unit: supported-language seed includes `es-AR`.
- Backend email unit: `Render("es-AR", ...)` renders Spanish.
- API/client unit: browser detection preserves `es-AR`.

### Phase 2: Argentine Spanish Copy

Only do this if Payverge wants visibly Argentine language, not just a regional selector.

Operator UI:

- Create `frontend/src/i18n/messages/es-AR/` by copying `es/`.
- Review and localize copy intentionally. Do not bulk replace every phrase.
- Keep `es` fallback for missing keys while the copy is being reviewed.

Guest UI:

- Create `frontend/src/i18n/guest-messages/es-AR.json` only if guest-facing copy differs from neutral Spanish.
- Maintain interpolation syntax with `{name}` style, not `{{name}}`.

Emails:

- Either keep `es-AR -> es`, or create `backend/email/templates/es_ar/` and `backend/email/layout/base_es_ar.html`.
- If adding an `es_ar` family, extend `templateLanguages` and template parity tests.

### Phase 3: Generic Future-Language Foundation

Use this when adding languages beyond `es-AR`.

- Widen language-code DB fields to `varchar(16)`.
- Centralize email template language mapping.
- Add a single locale fixture test that proves every locale can resolve operator copy, guest copy, email mapping.

## Future Language Checklist

Use this section when asking: "I want to add X/Y/Z language, please help me do it."

### Step 1: Define Scope

Choose one or more:

- Operator app UI only.
- Guest UI only.
- Business/menu translation language only.
- Emails.
- Full support across all surfaces.

Required decisions:

- Canonical code: for example `fr`, `pt-BR`, `zh-Hant`.
- Native display name.
- Text direction.
- Whether operator UI copy is real or falls back.
- Whether guest UI copy is real or falls back.
- Whether emails are real or fall back.
- Whether machine translation provider supports the exact target code.

### Step 2: Backend

Files to inspect and usually modify:

- `backend/internal/services/translation.go`
- `backend/internal/database/currency.go`
- `backend/internal/handlers/currency.go`
- `backend/internal/database/plugins.go`

Actions:

- Confirm the language code fits DB columns.
- Add a migration if it needs more than five characters.
- Seed `SupportedLanguage`.
- Validate language codes in business-language update handlers.
- Decide translation-provider target code.
- Add/update tests.

### Step 3: Operator Frontend

Files to inspect and usually modify:

- `frontend/src/i18n/config.ts`
- `frontend/src/i18n/SimpleTranslationProvider.tsx`
- `frontend/src/i18n/useLanguage.tsx`
- `frontend/src/providers/HybridAuthProvider.tsx`
- `frontend/src/components/SimpleLanguageSwitcher.tsx`
- `frontend/src/components/FloatingLanguageSwitcher.tsx`
- `frontend/src/i18n/messages/<locale>/`

Actions:

- Add locale metadata.
- Add or alias the message bundle.
- Update validation and fallback helpers.
- Update browser detection and persisted user preference handling.
- Update switcher UX if more than two locales exist.
- Add tests for fallback and preference handling.

### Step 4: Guest Frontend

Files to inspect and usually modify:

- `frontend/src/i18n/GuestTranslationProvider.tsx`
- `frontend/src/i18n/guest-messages/<locale>.json`
- `frontend/src/components/guest/*LanguageSelector*.tsx`
- `frontend/src/app/b/[customUrl]/*`
- `frontend/src/app/t/[tableCode]/*`

Actions:

- Add or alias guest message bundle.
- Preserve exact business language code in localStorage and menu translation requests.
- Fall back guest UI copy independently from menu language.
- Update RTL list when needed.
- Add tests for guest provider loading and fallback.

### Step 5: Emails

Files to inspect and usually modify:

- `backend/internal/emails/templates.go`
- `backend/internal/emails/templates_structure_test.go`
- `backend/email/templates/`
- email caller normalizers under `backend/internal/services/` and `backend/internal/handlers/`

Actions:

- Add an email template family only when real email copy exists.
- Otherwise map the locale to an existing family.
- Use shared language normalization so callers do not collapse regional Spanish to English.
- Run email template render tests.

### Step 6: Verification Commands

Run the smallest relevant set first:

```bash
cd frontend
npx jest --watchman=false --runInBand src/i18n scripts/i18n
```

```bash
cd backend
go test ./internal/emails ./internal/services ./internal/handlers -run 'Language|Template|Translation'
```

Before merging a full language rollout:

```bash
cd frontend
npm run typecheck
npm run test
npm run build
```

```bash
cd backend
make quick-test
```

Known current caveat:

- A previous baseline `npm run typecheck` failed in `frontend/src/components/business-page/GuestReservationForm.tsx` because several reservation translation keys are not assignable to the typed reservation key union. That appears unrelated to `es-AR`, but it will block full typecheck verification until fixed.

## Completion Gate For A Language Rollout

Before calling a language rollout complete, verify all of the following with actual evidence:

- The language appears in `GET /api/v1/languages`.
- A business can select it and persist it.
- Guest selectors display it when the business enables it.
- Guest menu requests use the exact language code.
- Operator UI can select it or intentionally excludes it.
- User language preference can persist it when it is an operator locale.
- Emails render through the intended template family.
- Tests prove fallback behavior and exact-code persistence.
- There are no raw i18n key leaks on the touched surfaces.

## Recommended First Implementation For `es-AR`

The safest first implementation is:

- Add `es-AR` as a backend-supported business/menu language.
- Preserve `es-AR` in database records and guest language preferences.
- Alias operator UI messages to `es`.
- Alias guest UI messages to `es`.
- Alias emails to `es`.
- Add tests proving the aliases and exact-code persistence.

This gives product-visible Argentine Spanish support without a large copywriting project. A later content pass can add true `es-AR` copy where it is worth maintaining.
