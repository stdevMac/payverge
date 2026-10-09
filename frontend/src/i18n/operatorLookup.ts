// Pure operator-tier lookup factory — server-safe (no "use client").
//
// Shared by the full operator catalog (./getTranslation) and the slim root
// chrome catalog (./operatorChromeCatalog), so both resolve keys, fall back
// and interpolate identically.

import { defaultLocale, isSupportedLocale, type Locale } from "./localeRegistry";
import { sentenceCaseLeaf } from "./sentenceCaseLeaf";
import { reportMissingTranslation } from "./missingTranslationReporter";

export type OperatorCatalogs = Partial<Record<Locale, Record<string, any>>>;

export type OperatorLookup = (
  key: string,
  locale?: Locale,
  params?: Record<string, string | number>,
) => string | string[];

export function createOperatorLookup(messages: OperatorCatalogs): OperatorLookup {
  // Simple translation function that doesn't use hooks.
  //
  // Lookup order: active locale -> English -> sentence-cased leaf of the
  // dotted key. Reaching the leaf fallback means *both* the active
  // locale AND English are missing the key, which we report once per
  // (locale, key) per session so the gap is visible in telemetry rather
  // than silently rendering a debug-looking identifier in the UI.
  return function getTranslation(
    key: string,
    locale: Locale = defaultLocale,
    params?: Record<string, string | number>,
  ): string | string[] {
    const translationKey = typeof key === "string" ? key : String(key);
    const safeLocale = isSupportedLocale(locale) ? locale : defaultLocale;

    const applyParams = (value: string): string => {
      if (!params) return value;
      let next = value;
      Object.entries(params).forEach(([k, v]) => {
        // Support both {{var}} and single-brace {var}. Operator message files use
        // the single-brace form (e.g. "{count} spots remaining"); previously only
        // {{var}} was substituted, so those tokens rendered raw. Matches the guest
        // provider's dual-syntax handling. ICU blocks like "{count, plural, ...}"
        // don't match (the key is followed by ',' not '}'), so they pass through.
        const escaped = k.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
        next = next.replace(
          new RegExp(`\\{\\{${escaped}\\}\\}|\\{${escaped}\\}`, "g"),
          String(v),
        );
      });
      return next;
    };

    const resolveFrom = (root: any): any => {
      let value: any = root;
      for (const k of translationKey.split(".")) {
        value = value?.[k];
      }
      return value;
    };

    try {
      const primary = resolveFrom(messages[safeLocale]);
      if (typeof primary === "string") return applyParams(primary);
      if (Array.isArray(primary)) return primary;

      // Locale miss — try English, but only report if the locale wasn't
      // already English. A missing EN key drops straight to the leaf
      // fallback and reports there to avoid double-firing.
      if (safeLocale !== "en") {
        const englishValue = resolveFrom(messages.en);
        if (typeof englishValue === "string") {
          reportMissingTranslation({
            locale: safeLocale,
            key: translationKey,
            fallbackUsed: "english",
          });
          return applyParams(englishValue);
        }
        if (Array.isArray(englishValue)) {
          reportMissingTranslation({
            locale: safeLocale,
            key: translationKey,
            fallbackUsed: "english",
          });
          return englishValue;
        }
      }

      reportMissingTranslation({
        locale: safeLocale,
        key: translationKey,
        fallbackUsed: "leaf",
      });
      return sentenceCaseLeaf(translationKey);
    } catch (error) {
      reportMissingTranslation({
        locale: safeLocale,
        key: translationKey,
        fallbackUsed: "leaf",
      });
      return sentenceCaseLeaf(translationKey);
    }
  };
}
