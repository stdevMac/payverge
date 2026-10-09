# Adding a New Locale to Payverge

This is the canonical playbook. Pick the scenario that matches what you want
to do, follow the checklist exactly, then run the verification suite at the
bottom.

> **Architecture refresher:** Payverge has two distinct i18n systems.
>
> 1. **Operator i18n** — what the business operator sees in the dashboard
>    (Overview, Menu Builder, Reservations, etc.). Today: en, es, es-AR.
> 2. **Guest i18n** — what end customers see on the public storefront when
>    scanning a QR code at a table. Operators type menu items in their own
>    language; the backend translates dynamically via Google Translate to
>    every active **guest locale**.
>
> Both systems are driven by a single registry: `locales/registry.json`. Flags
> on each entry determine which tier(s) the locale participates in. See the
> [README](./README.md) for the three-tier model.

---

## Scenario A: Add a new **operator dashboard locale**

Use this when the operator should be able to switch the dashboard UI into
the new language (cookies, routes, emails, prompts — the full suite).

### Step 1 — Registry entry

Add an entry under `locales/registry.json` with:

```json
"<code>": {
  "canonical": "<code>",                  // BCP-47, e.g. "es-AR" or "de"
  "pathSegment": "<lowercase-code>",      // URL segment, e.g. "es-ar"
  "sourceFolder": "<lowercase-code>",     // folder name under locales/, messages/, etc.
  "displayName": "<English name>",        // "Argentine Spanish"
  "nativeName": "<endonym>",              // "Español (Argentina)"
  "direction": "ltr",                     // or "rtl" for Arabic, Hebrew, etc.
  "emailFamily": "<email key>",           // matches backend/email/layout/base_*.html
  "promptFamily": "<prompt key>",         // matches backend/internal/services/prompts/*/<key>.md
  "translationProviderTarget": "<code>",  // Google Translate code (e.g. "zh-CN", "es-AR")
  "publicRoutes": "prefixed",             // "default" only for en
  "operatorLocale": true,
  "guestLocale": true,                    // operator locales should also be guest targets
  "publishable": true,                    // false until all surfaces are approved
  "requiredSurfaces": [
    "frontend",
    "apiErrors",
    "emails",
    "prompts",
    "backendValidation",
    "guest",
    "guestStorefront"
  ]
}
```

### Step 2 — Operator translation files

Create these under `frontend/src/i18n/locales/<sourceFolder>/`:

- `common.json` — main UI strings
- `apiErrors.json` — backend error message map
- `routes.json` — localized route slugs

And the parallel set under `frontend/src/i18n/messages/<sourceFolder>/` if
you use those (check what `en` has).

### Step 3 — Email templates

Create `backend/email/layout/base_<emailFamily>.html` and a directory
`backend/email/templates/<emailFamily>/` mirroring the `en` family's files.

> **Email family (Wave 2):** A new `base_<fam>.html` inherits the branded
> reskin verbatim; translate only the three footer strings (brand line, guest
> context line, "manage preferences" link). The footer variant is chosen by a
> language-agnostic Go classifier, and each content template owns its
> `{{ define "subject" }}` block — so adding a language stays content-only. The
> `internal/emails` consistency tests (variable parity, subject block, palette,
> footer content) cover the new family automatically.

### Step 4 — AI prompts

Create both:

- `backend/internal/services/prompts/menu_wizard/<promptFamily>.md`
- `backend/internal/services/prompts/director_console/<promptFamily>.md`

Copy the `en` versions as a starting point.

### Step 5 — Status file

Create `locales/<pathSegment>/status.json`:

```json
{
  "canonical": "<code>",
  "pathSegment": "<lowercase-code>",
  "state": "draft",
  "publishable": false,
  "reviewOwner": null,
  "reviewDate": null,
  "aiDraft": {
    "frontend": "ai_drafted",
    "apiErrors": "ai_drafted",
    "emails": "ai_drafted",
    "prompts": "ai_drafted",
    "guest": "ai_drafted",
    "guestStorefront": "ai_drafted"
  },
  "review": {},
  "unresolvedQuestions": []
}
```

Once a surface has been human-reviewed and approved, move it from `aiDraft`
to `review` with state `"approved"`, then record the accountable
`reviewOwner` and ISO `reviewDate`. The locale becomes publishable when
**every** `requiredSurfaces` entry has `review[surface] === "approved"` and
`unresolvedQuestions` is empty.

### Step 6 — Guest storefront bundle

Because `requiredSurfaces` includes `guestStorefront`, you also need
`frontend/src/i18n/guest-messages/<code>.json`. See **Scenario B Step 3**
for the bundle format.

### Step 7 — Wire-up

- Add a flag emoji to `localeFlagByCode` in
  `frontend/src/i18n/localeRegistry.ts` (optional — the resolver falls back to
  the uppercase code if the entry is absent).
- The `Locale` TypeScript type narrows to operator codes automatically once
  `operatorLocale: true` lands in the registry — no code change needed.

### Shortcut

```bash
npm run i18n:scaffold -- --locale <code>
```

scaffolds the operator-side directory structure for you (frontend message
files, email/prompt folders, status.json). You still need to write the
translations.

---

## Scenario B: Add a new **guest storefront locale** (menu-translation target)

Use this when the guest UI dropdown should offer the language to customers,
the backend should accept it as a translation target for menu items, but
the operator dashboard itself doesn't need to support the language.

This is the common case — adding more languages for guests of restaurants.

### Step 1 — Registry entry

```json
"<code>": {
  "canonical": "<code>",
  "pathSegment": "<lowercase-code>",
  "sourceFolder": "<lowercase-code>",
  "displayName": "<English name>",        // "Vietnamese"
  "nativeName": "<endonym>",              // "Tiếng Việt"
  "direction": "ltr",                     // or "rtl"
  "emailFamily": "<code>",
  "promptFamily": "<code>",
  "translationProviderTarget": "<code>",
  "publicRoutes": "prefixed",
  "operatorLocale": false,
  "guestLocale": true,
  "publishable": false,
  "requiredSurfaces": ["guestStorefront"]
}
```

### Step 2 — Status file

Create `locales/<pathSegment>/status.json`:

```json
{
  "canonical": "<code>",
  "pathSegment": "<lowercase-code>",
  "state": "guest-target-only",
  "publishable": false,
  "aiDraft": {},
  "review": {},
  "unresolvedQuestions": []
}
```

### Step 3 — Guest storefront bundle

Create `frontend/src/i18n/guest-messages/<code>.json` — a full translation of
the en.json bundle (~1000 lines). The structure must match exactly; only the
string values change. Notes:

- Use ICU interpolation: `{varname}` — never Mustache `{{varname}}`. The
  Jest test in `__tests__/guest-messages.test.ts` enforces this.
- Plural blocks pass through ICU `{count, plural, one {…} other {…}}`
  unchanged at runtime; translate the inner words.
- Match the shape of `en.json` exactly — missing keys fall back to English at
  runtime, but the validator does not require key completeness today.

### Step 4 — Flag emoji

Add to `localeFlagByCode` in `frontend/src/i18n/localeRegistry.ts`:

```ts
const localeFlagByCode: Partial<Record<LocaleCode, string>> = {
  // ...
  vi: "🇻🇳",
  pl: "🇵🇱",
  sv: "🇸🇪",
};
```

This single registry-keyed map is the canonical flag source for both the guest
storefront selector and the operator language switcher. The resolver
(`getLocaleFlag`) falls back to the uppercase code if the entry is missing — not
fatal, just less polished.

### Step 5 — Regenerate + verify

```bash
npm run i18n:generate
npm run i18n:validate
cd frontend && npm test -- src/i18n/
cd backend && go test ./internal/locales/... ./internal/services/...
```

Backend `InitializeDefaultLanguages` will pick up the new locale on next boot
and insert a `supported_languages` row automatically — no SQL migration
needed.

---

## Scenario C: Promote a guest-only locale to a full operator locale

E.g. you've been shipping `de` as a guest-only locale but now want a German
dashboard.

1. Flip `operatorLocale: false` → `true` in `registry.json`.
2. Expand `requiredSurfaces` to the full operator set (see Scenario A Step 1).
3. Update `status.json` `state` from `"guest-target-only"` to `"draft"` and
   populate `aiDraft` with `ai_drafted` for each new surface.
4. Run `npm run i18n:scaffold -- --locale de` to create the operator-side
   file scaffolds.
5. Translate frontend `common.json` / `apiErrors.json` /
   `routes.json`, create the email templates and the prompt families.
6. Get the surfaces reviewed and flip `review[surface]` to `"approved"`.
7. Flip `publishable: true` once every required surface is approved.
8. Run `npm run i18n:generate` + `npm run i18n:validate`.

The narrow `Locale` TypeScript type will automatically widen to include the
new code because it's derived from `localeRegistry[L]["operatorLocale"]
extends true`.

---

## Verification suite (run for any locale change)

```bash
# Frontend
cd frontend
npm run i18n:generate           # regenerate locales.ts + registry_generated.go
npm run i18n:check              # verify the generated files are in sync (CI runs this too)
npm run i18n:validate           # enforce registry ↔ files invariant
npm run typecheck               # tsc --noEmit (Locale union narrowed correctly)
npm run lint
npx jest src/i18n/              # ICU placeholder check + GUEST_SUPPORTED_LANGUAGES regression pin

# Backend
cd ../backend
go build ./...
go test ./internal/locales/... ./internal/services/... ./internal/server/...
```

`npm run i18n:report` (frontend) is a useful debrief — prints publishability
status per locale, which surfaces are still drafted vs. approved, and any
unresolved review questions.

---

## es-AR is an OVERRIDE LAYER, not a full translation

Argentine Spanish (`es-AR`) is special: it is the neutral `es` base with a thin
**Rioplatense override layer** deep-merged on top, rather than a duplicated full
translation. This keeps it DRY and structurally free of missing-key gaps.

- **Operator tier:** `frontend/src/i18n/messages/es-ar/<namespace>.json` holds
  ONLY the keys whose value differs in Argentine Spanish (voseo verbs, local
  vocab). `messages/es-ar/index.ts` barrels them; `SimpleTranslationProvider`
  builds `messages["es-AR"] = deepMerge(esTree, esArOverrides)`. Any key not
  overridden inherits `es`. To add/extend: edit the relevant
  `messages/es-ar/<ns>.json` (or add a new one + import it in `index.ts`).
- **Guest tier:** `guest-messages/es-AR.json` is the EXCEPTION — it must be a
  COMPLETE bundle (the guest provider loads one JSON per locale and falls
  missing keys back to **English**, not es). Keep full key parity with es.
- **Style + glossary:** `docs/i18n/rioplatense-style-guide.md` is the source of
  truth (voseo rules, es→es-AR glossary, "cuenta" not "factura" for the open
  bill, "mozo" not "mesero"). Follow it for any es-AR edit.
- **Quality gates:** `operator-es-ar-voseo.test.ts` fails if a merged es-AR
  string contains a Peninsular tú-form (legal/terms excluded — deferred);
  `guest-messages.test.ts` pins es-AR↔es key parity + no tú-form leaks. Run
  `npx jest src/i18n/` after any es or es-AR edit.

**Defaulting → es-AR:** `resolveLocaleForBrowser` (operator) and
`resolveGuestInitialLanguage` (guest) map `es-AR` and `es-419` (generic LatAm)
to es-AR, and Iberian `es`/`es-ES` to es; the guest selectors auto-detect the
browser language among the business's enabled languages. So an Argentine visitor
lands on es-AR automatically.

## Gotchas

- **`IsSupported` is operator-scoped, not registry-scoped.** Never use
  `locales.IsSupported(code)` to gate menu translation — use
  `locales.IsGuestLocale(code)` instead. Conflating these is exactly the
  2026-05-14 regression that wiped 18 guest languages from prod.
- **`Locale` (TS) ≠ `LocaleCode` (TS).** `Locale = OperatorLocale` (narrow,
  3 codes). `LocaleCode` covers everything in the registry. Code that
  expects `Record<Locale, ...>` (SimpleTranslationProvider, apiErrors,
  metadata) must keep working with the narrow set.
- **Path segments are lowercase, canonical codes preserve BCP-47.** e.g.
  `canonical: "es-AR"` but `pathSegment: "es-ar"`. The guest-messages JSON
  file name uses the **canonical** code (`es-AR.json`, not `es-ar.json`) —
  the file resolves via dynamic `import('./guest-messages/${code}.json')`
  where `code` is the canonical form.
- **`translationProviderTarget` is what Google Translate sees**, not what
  appears in the URL. For Chinese use `zh-CN` (or `zh-TW` if you want
  Traditional). For Norwegian, `no` (or `nb`/`nn` if you want a specific
  variant). **Google Cloud Translation v2 (NMT) only accepts a handful of
  region variants — `zh-CN`/`zh-TW` are essentially it. Region-qualified
  Spanish (`es-AR`, `es-419`, `es-MX`) is REJECTED**, so `es-AR`'s target is
  `"es"`, not `"es-AR"`. Using a region code Google doesn't support makes
  `translateTextWithSource` 400 and silently persist untranslated source text
  as that locale's menu translation (`TestTranslationProviderTarget` guards
  this). The Rioplatense voseo lives in the static UI bundle, not in
  machine-translated menu content, so `"es"` loses nothing.
- **Generated files are checked in.** Re-run `npm run i18n:generate`
  whenever `registry.json` changes. CI does not regenerate, but it does
  **verify**: `npm run i18n:check` (`i18n` job in `.github/workflows/ci.yml`)
  regenerates both outputs in memory and fails the build if either checked-in
  file drifts from `registry.json`. The pre-commit hook runs the same check when
  `registry.json` or a generated file is staged, and the Go test
  `TestGeneratedRegistryMatchesRegistryJSON` independently re-derives the
  registry from `registry.json` and asserts the generated Go map matches — so a
  forgotten regenerate fails fast on either side, even without the TS toolchain.
- **Backend seed is idempotent + self-healing.** Adding a guest locale
  doesn't require a SQL migration — `InitializeDefaultLanguages` upserts
  registry-known rows on every backend boot, and reactivates inactive rows
  (the self-heal block from the 2026-05-23 fix). Unknown codes (e.g.
  experimental rows hand-added by an operator) are preserved untouched.

---

## File checklist (cheat sheet)

**Operator locale (Scenario A):**

| File | Required for |
|---|---|
| `locales/registry.json` (entry) | always |
| `locales/<seg>/status.json` | always |
| `frontend/src/i18n/locales/<folder>/common.json` | `frontend` surface |
| `frontend/src/i18n/locales/<folder>/apiErrors.json` | `frontend`, `apiErrors`, `backendValidation` surfaces |
| `frontend/src/i18n/locales/<folder>/routes.json` | `frontend`, `guest` surfaces |
| `frontend/src/i18n/guest-messages/<code>.json` | `guestStorefront` surface |
| `backend/email/layout/base_<emailFamily>.html` | `emails` surface |
| `backend/email/templates/<emailFamily>/*.html` | `emails` surface |
| `backend/internal/services/prompts/menu_wizard/<promptFamily>.md` | `prompts` surface |
| `backend/internal/services/prompts/director_console/<promptFamily>.md` | `prompts` surface |

**Guest-only locale (Scenario B):**

| File | Required |
|---|---|
| `locales/registry.json` (entry) | always |
| `locales/<seg>/status.json` | always |
| `frontend/src/i18n/guest-messages/<code>.json` | `guestStorefront` surface |
| Flag emoji in `GuestTranslationProvider.tsx` | recommended (cosmetic) |
