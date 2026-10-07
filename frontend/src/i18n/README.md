# Payverge Internationalization System

A comprehensive internationalization (i18n) system for the Payverge platform supporting multiple languages with easy extensibility.
Payverge runs a **custom, in-repo translation system** — there is no `next-intl`
and no `TranslationProvider`/`config.ts` (those were removed). Translations are
plain JSON message files consumed through two providers.

## Two tiers

| Tier | Provider | Locales | Message source |
| --- | --- | --- | --- |
| **Operator** (dashboard, marketing, auth) | `SimpleTranslationProvider` (`useSimpleLocale` + `getTranslation`) | `en`, `es`, and `es-ar` (Rioplatense voseo) | `src/i18n/messages/<locale>/*.json` (one file per namespace, aggregated by `index.ts`) |
| **Guest** (storefront, table & delivery flows) | `GuestTranslationProvider` (`useGuestTranslation`) | 21 locales | `src/i18n/guest-messages/<locale>.json` |

`es-ar` is a **deltas-only** override: each `messages/es-ar/*.json` carries only
the keys whose wording differs from Peninsular `es` (typically voseo imperatives:
`Revisa` → `Revisá`). At load time `getTranslation.ts` deep-merges the override
tree over the full `es` tree (`deepMerge(esTree, esArOverrides)`), so any key not
present in an `es-ar` file transparently inherits the `es` string. Never copy the
whole `es` bundle into `es-ar`.

## File structure

```
src/i18n/
├── README.md                    # This file
├── getTranslation.ts            # Operator lookup + es-ar deep-merge
├── deepMerge.ts                 # Deltas overlay helper
├── localeRegistry.ts            # IsOperatorLocale / IsGuestLocale helpers
├── SimpleTranslationProvider.tsx  # Operator context (en / es / es-ar)
├── GuestTranslationProvider.tsx   # Guest context (21 locales)
├── messages/
│   ├── en/*.json  es/*.json  es-ar/*.json   # per-namespace operator files
│   └── <locale>/index.ts                    # aggregates that locale's namespaces
└── guest-messages/
    └── <locale>.json                        # one flat bundle per guest locale
```

## Using translations

**Operator components:**

```tsx
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";

function MyComponent() {
  const { locale } = useSimpleLocale(); // "en" | "es" | "es-ar"
  const label = getTranslation("businessDashboard.dashboard.overview.title", locale);
  return <h1>{label}</h1>;
}
```

`getTranslation` returns the key itself on a miss, so a common pattern is a small
`tOr(key, fallback)` wrapper (see `PluginManager.tsx`) for keys not yet in every
namespace file.

**Guest components:**

```tsx
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

function GuestView() {
  const { t } = useGuestTranslation();
  return <span>{t("bill.payNow")}</span>; // params: t("payment.pluginError", { method })
}
```

**Locale checks:** use `IsOperatorLocale` / `IsGuestLocale` from `localeRegistry.ts`
— never a single `IsSupported`, because the two tiers have different locale sets.

## Adding or editing translations

1. **Operator string:** add the key to the matching namespace in `messages/en/*.json`
   **and** `messages/es/*.json`. Add an `es-ar` delta only if the voseo wording
   differs. Import any new namespace file in that locale's `index.ts`.
2. **Guest string:** avoid adding new keys when an already-translated key fits.
   A genuinely new guest key must be added to **all 21** `guest-messages/*.json`
   files, machine-translated consistently with neighbouring keys.
3. **Adding a whole locale:** follow `locales/LANGUAGES.md` — the canonical playbook
   (scenarios A/B/C) for operator vs guest tiers. Do not hand-roll a new locale.

## Validation scripts

| Command | Purpose |
| --- | --- |
| `npm run i18n:check` | Verify generated locale files are in sync with `locales/registry.json` (CI + pre-commit gate). |
| `npm run i18n:validate` | Locale-parity + structural validation across operator and guest tiers. |
| `npm run i18n:report` | Human-readable coverage report. |
| `npm run i18n:scaffold` | Scaffold a new locale's files. |

Git hooks installed by `make install-hooks` run the guest + operator locale-parity
validators on commit, so parity drift is caught before it lands.

## Canonical references

- `locales/LANGUAGES.md` — the playbook for adding/editing locales (read before any locale work).
- `locales/registry.json` — source of truth for the supported-locale lists.

