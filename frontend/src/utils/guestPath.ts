import { stripOperatorLocalePrefix } from "@/utils/requestLocale";

/**
 * True for every diner surface that renders under GuestTranslationProvider
 * and so resolves the guest locale: the instance root (a venue page or the
 * venue directory), /b/<slug>, /t/<code>, /scan, /delivery/<…> and
 * /reservations/<…>. A leading operator locale prefix (/es, /es-ar) is
 * stripped first.
 *
 * Shared by the middleware (which locale the SSR <html lang> carries) and
 * SimpleTranslationProvider (which must not write <html lang> over the guest
 * language), so the two can never disagree about which paths are guest.
 */
export function isGuestPath(pathname: string): boolean {
  const trimmed = pathname.replace(/\/+$/, "") || "/";
  const unprefixed = stripOperatorLocalePrefix(trimmed);
  if (unprefixed === "/") return true;
  if (unprefixed === "/scan") return true;
  return /^\/(?:b|t|scan|delivery|reservations)\//.test(unprefixed);
}
