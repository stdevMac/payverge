import { cookies, headers } from "next/headers";
import {
  GUEST_LOCALE_COOKIE,
  resolveGuestRequestLocale,
  type ResolvedGuestLocale,
} from "./guestLocaleResolver";

/**
 * Server-only guest locale resolution for App Router pages (PG-21).
 * Mirrors middleware: ?lang= → guest cookie → Accept-Language → en.
 */
export async function resolveGuestLocaleFromRequest(
  langParam: string | null | undefined,
): Promise<ResolvedGuestLocale> {
  const cookieStore = await cookies();
  const headerStore = await headers();
  return resolveGuestRequestLocale({
    // Path-locale /es/b/{slug} arrives as x-payverge-locale from middleware
    // (#644). Prefer ?lang=, then that header, then cookie / Accept-Language.
    langParam: langParam || headerStore.get("x-payverge-locale"),
    guestCookie: cookieStore.get(GUEST_LOCALE_COOKIE)?.value ?? null,
    acceptLanguage: headerStore.get("accept-language"),
  });
}
