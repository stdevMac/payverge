import { readGuestLocaleCookie } from "@/i18n/guestLocaleResolver";
import {
  normalizeGuestLangParam,
  storefrontLocales,
} from "@/i18n/localeRegistry";
import type { GuestLanguageCode } from "@/i18n/GuestTranslationProvider";

function asSupportedGuestLanguage(
  raw: string | null | undefined,
): GuestLanguageCode | null {
  const normalized = normalizeGuestLangParam(raw);
  if (
    normalized &&
    (storefrontLocales as readonly string[]).includes(normalized)
  ) {
    return normalized as GuestLanguageCode;
  }
  return null;
}

export function resolveDeliveryInitialLanguage(
  raw: string | null | undefined,
): GuestLanguageCode {
  return (
    asSupportedGuestLanguage(raw) ??
    asSupportedGuestLanguage(readGuestLocaleCookie()) ??
    "en"
  );
}

export function deliveryGuestPath(
  deliveryNumber: string,
  leaf: "track" | "pay",
  lang?: string | null,
): string {
  const base = `/delivery/${encodeURIComponent(deliveryNumber)}/${leaf}`;
  const resolved = lang?.trim();
  if (!resolved || resolved === "en") return base;
  return `${base}?lang=${encodeURIComponent(resolved)}`;
}
