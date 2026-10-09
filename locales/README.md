# Locale Registry

The `locales/registry.json` file is the **single source of truth** for every
locale Payverge supports — operator dashboard, guest storefront, blog, emails,
prompts, all of it. Editing translations anywhere starts here.

For step-by-step instructions on adding a new locale (either side), read
**[LANGUAGES.md](./LANGUAGES.md)** — the canonical playbook.

## Quick reference

```bash
npm run i18n:scaffold -- --locale es-AR   # bootstrap a new locale (operator tree + guest bundle + folders)
npm run i18n:generate                     # regenerate locales.ts + registry_generated.go
npm run i18n:check                        # verify generated files are in sync (no write); used in CI + pre-commit
npm run i18n:validate                     # enforce registry ↔ files invariant
npm run i18n:report                       # surface review/AI-draft status
```

The generated files (`frontend/src/i18n/generated/locales.ts` and
`backend/internal/locales/registry_generated.go`) are checked in. `i18n:check`
re-derives both in memory and fails if either drifts from `registry.json`; it
runs in CI (the frontend `build-check` job) and in the pre-commit hook whenever
`registry.json` or a generated file is staged. The Go test
`TestGeneratedRegistryMatchesRegistryJSON` enforces the same invariant from the
backend side.

## Three locale tiers (orthogonal flags)

Each registry entry carries three booleans + a `requiredSurfaces` array.
**Don't conflate them.**

| Tier | Flag | Examples | What it gates |
|---|---|---|---|
| **Operator** | `operatorLocale: true` | en, es, es-AR | Dashboard cookies, routes, `Record<Locale,...>` message maps, auth redirects, blog, email families |
| **Guest** | `guestLocale: true` | all 21 today | Backend menu-translation gating, `business_languages` rows, `/api/v1/languages` endpoint, Google Translate target |
| **Storefront** | `requiredSurfaces` includes `"guestStorefront"` | en, es, es-AR + 18 guest-only | Public guest UI dropdown, `frontend/src/i18n/guest-messages/{code}.json` bundle |

The validator (`npm run i18n:validate`) enforces the `guestStorefront ↔ JSON
bundle exists` invariant. Adding a guest-only locale means: registry entry +
`status.json` + `guest-messages/{code}.json` + `i18n:generate`.

## Status files

Each `locales/{path-segment}/status.json` records rollout state for that
locale — draft AI generation and human review per surface. The validator
checks publishable locales have `review[surface] === "approved"` for every
surface in `requiredSurfaces`.

Guest-only locales use `state: "guest-target-only"` with empty `aiDraft` and
`review` blocks — they ship a JSON bundle but no operator-side dashboard
content, so there's nothing to draft or review.
