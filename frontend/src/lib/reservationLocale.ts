import {
  GUEST_SUPPORTED_LANGUAGES,
  type GuestLanguageCode,
} from "@/i18n/GuestTranslationProvider";
import { readGuestLocaleCookie } from "@/i18n/guestLocaleResolver";
import { normalizeGuestLangParam } from "@/i18n/localeRegistry";

/**
 * Resolve guest language for post-booking reservation micro-pages (PG-12).
 * Priority: ?lang= (email deep link) → guest cookie → English.
 */
export function resolveReservationInitialLanguage(
  raw: string | null | undefined,
): GuestLanguageCode {
  const fromQuery = normalizeGuestLangParam(raw);
  if (fromQuery && fromQuery in GUEST_SUPPORTED_LANGUAGES) {
    return fromQuery as GuestLanguageCode;
  }
  const fromCookie = readGuestLocaleCookie();
  if (fromCookie && fromCookie in GUEST_SUPPORTED_LANGUAGES) {
    return fromCookie as GuestLanguageCode;
  }
  return "en";
}
