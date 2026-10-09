import { publicPathForLocale } from "@/i18n/metadata";
import { normalizeGuestLangParam } from "@/i18n/localeRegistry";
import { getSiteUrl } from "@/config/publicConfig";

const OG_LOCALE: Record<string, string> = {
  en: "en_US",
  es: "es_ES",
  "es-AR": "es_AR",
};

/** Path-prefix es / es-AR diner URLs; other guest locales stay unprefixed. */
export function guestPublicPath(locale: string, path: string): string {
  const canonical = normalizeGuestLangParam(locale);
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  if (canonical === "es" || canonical === "es-AR") {
    return publicPathForLocale(canonical, normalizedPath);
  }
  return normalizedPath;
}

export function guestPublicUrl(
  locale: string,
  path: string,
  baseUrl: string = getSiteUrl(),
): string {
  return `${baseUrl.replace(/\/+$/, "")}${guestPublicPath(locale, path)}`;
}

export function guestOgLocale(locale: string): string {
  const canonical = normalizeGuestLangParam(locale) ?? locale;
  return OG_LOCALE[canonical] ?? canonical.replace("-", "_");
}
