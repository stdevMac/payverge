// Server-safe helpers for deriving the root <html lang>/<dir> attributes during
// SSR, plus the operator-tier locale collapse. Kept free of "use client" so
// layout.tsx (a server component) can call them at first paint — before any
// client effect (GuestTranslationProvider / SimpleTranslationProvider) runs.
//
// The middleware writes `x-payverge-locale`. For operator routes this is one of
// en|es|es-ar (see utils/requestLocale). For a guest storefront URL carrying a
// shared/crawled `?lang=` (e.g. /b/slug?lang=ar) the middleware now writes the
// canonical storefront code (ar, es-AR, fr, …) so the SSR first paint reflects
// the right language and direction (and RTL) immediately, instead of waiting for
// GuestTranslationProvider's post-hydration effect to correct it.

import {
  defaultLocale,
  getLocaleDirection,
  isSupportedLocale,
  localeRegistry,
  type Locale,
  type LocaleCode,
} from "./localeRegistry";

// BCP-47 tags are case-insensitive (es-AR == es-ar == ES-AR), but our canonical
// registry codes preserve a fixed casing. Map any incoming header value back to
// its canonical registry code via a lowercase → canonical lookup.
const registryCodeByLower = new Map<string, LocaleCode>(
  (Object.keys(localeRegistry) as LocaleCode[]).map((code) => [
    code.toLowerCase(),
    code,
  ]),
);

function toCanonicalLocaleCode(
  raw: string | null | undefined,
): LocaleCode | undefined {
  if (typeof raw !== "string" || raw.length === 0) return undefined;
  return registryCodeByLower.get(raw.toLowerCase());
}

// Resolve the canonical <html lang> for an x-payverge-locale header value.
// Accepts any registry code (operator OR guest storefront), normalising casing
// (es-ar → es-AR). Unknown / missing values fall back to the default locale so
// the attribute is always a valid BCP-47 tag.
export function resolveHtmlLang(raw: string | null | undefined): LocaleCode {
  return toCanonicalLocaleCode(raw) ?? defaultLocale;
}

// Resolve the <html dir> for an x-payverge-locale header value, driven by the
// registry `direction` field (so future RTL locales — he/fa/ur — light up
// automatically once added to the registry). Unknown values default to ltr.
export function resolveHtmlDir(raw: string | null | undefined): "ltr" | "rtl" {
  const code = toCanonicalLocaleCode(raw);
  return code ? getLocaleDirection(code) : "ltr";
}

// Collapse the header value to an OPERATOR-tier locale (en | es | es-AR) for
// SimpleTranslationProvider / buildMetadata. Guest-only storefront codes (ar,
// fr, …) that may arrive via a guest ?lang= flow are NOT operator locales, so
// they degrade to the default — the operator dashboard never ships their
// messages, and a guest storefront uses GuestTranslationProvider instead.
export function resolveOperatorLocale(
  raw: string | null | undefined,
): Locale {
  const code = toCanonicalLocaleCode(raw);
  return code && isSupportedLocale(code) ? code : defaultLocale;
}
