---
name: add-locale
description: Add a language to Payverge, usually as a guest storefront locale that guests can pick on the table page and that menus are translated into, or by promoting an existing guest locale to a full operator dashboard locale. Covers the locale registry, status file, message bundles, flag, code generation and verification. Use when someone asks to add, support or translate Payverge into a new language.
---

# Add a locale

The canonical playbook is `locales/LANGUAGES.md`. This skill picks the right
scenario from it and runs it end to end. The tier model is in
`locales/README.md`. Everything is driven by one file,
`locales/registry.json`.

## 0. Decide the scenario

Ask the operator which of these they want:

| They want | Scenario in `locales/LANGUAGES.md` | Size |
|---|---|---|
| Guests can read the table page and menu in the language | **B: guest storefront locale** (the common case) | one bundle of about 1000 lines |
| Staff can use the dashboard in the language | **A: operator dashboard locale**, or **C** if the locale already exists as a guest locale | many surfaces: UI, email, AI prompts, blog |

Check what already exists:

```bash
jq '.locales | to_entries[] | select(.key | ascii_downcase == "<code>")' locales/registry.json
ls locales/ frontend/src/i18n/guest-messages/
```

If the code already exists with `guestLocale: true`, scenario B is done:
offer C instead.

## Scenario B: guest storefront locale

1. **Registry entry.** Add the entry under `locales` in `locales/registry.json` with the
   field set shown in Scenario B, Step 1 of `locales/LANGUAGES.md`:
   `operatorLocale: false`, `guestLocale: true`, `publishable: false` and
   `requiredSurfaces: ["guestStorefront"]`. Use `"direction": "rtl"` for
   right-to-left scripts. Set `translationProviderTarget` to the code the
   menu translation provider expects.
2. **Status file.** Create `locales/<pathSegment>/status.json` with
   `"state": "guest-target-only"`, exactly as Step 2 shows.
3. **Guest bundle.** Create `frontend/src/i18n/guest-messages/<code>.json`
   by translating `frontend/src/i18n/guest-messages/en.json`.
   - Keep the key structure identical. Change values only.
   - Interpolation is ICU `{name}`, never `{{name}}`. Keep plural blocks
     (`{count, plural, one {...} other {...}}`) and translate the words
     inside them.
   - Do not translate the product name, currency codes or placeholders.
   - A machine draft is acceptable as a start. Say clearly that it needs a
     native speaker's review before `publishable` is set to `true`.
4. **Flag.** Add the code to `localeFlagByCode` in
   `frontend/src/i18n/localeRegistry.ts`.
5. **Generate.** Run from `frontend/`:

   ```bash
   npm run i18n:generate    # rewrites frontend/src/i18n/generated/locales.ts and backend/internal/locales/registry_generated.go
   ```

   Never edit those two generated files by hand.

The backend adds the `supported_languages` row on its next boot
(`InitializeDefaultLanguages`). No migration is needed.

## Scenario C: promote a guest locale to the dashboard

Follow Scenario C in `locales/LANGUAGES.md`:

1. Flip `operatorLocale` to `true` and expand `requiredSurfaces`.
2. Set `status.json` `state` to `"draft"`.
3. Run `npm run i18n:scaffold -- --locale <code>` from `frontend/`.
4. Translate the scaffolded operator files under
   `frontend/src/i18n/messages/<code>/`, the email templates and the AI
   prompt families that Scenario A lists.

Set `publishable: true` only after every required surface has been
reviewed. Operator locales are a large job. Agree with the operator on
which surfaces you draft now and which are left for reviewers.

## Verify (any scenario)

Run the full suite from `locales/LANGUAGES.md`:

```bash
cd frontend
npm run i18n:generate
npm run i18n:check
npm run i18n:validate
npm run typecheck
npm run lint
npx jest src/i18n/

cd ../backend
go build ./...
go test ./internal/locales/... ./internal/services/... ./internal/server/...
```

`npm run i18n:report` prints each locale's publishability and review state.
Include its line for the new locale in your summary.

## Commit

- Stage by explicit path: the registry, the status file, the bundle,
  `localeRegistry.ts` and the two generated files. Never use `git add -A`.
- `make install-hooks` installs the guest and operator parity hooks. Do not
  bypass a failing hook. Fix the bundle instead.

## Pitfalls

- `es-AR` is an override layer on top of `es`, not a full translation. See
  "es-AR is an OVERRIDE LAYER" in `locales/LANGUAGES.md` before you touch
  any regional variant.
- The AI waiter detects cart requests only in en, es and es-AR, and it uses
  the English prompt body for any locale without a reviewed prompt (see
  `docs/ai/ai-waiter.md`). Tell the operator that the waiter will answer in
  the guest's language but cannot take chat orders in it yet.
- Older notes in `docs/language-support-playbook.md` predate the registry.
  Where it disagrees with `locales/LANGUAGES.md`, the latter wins.
