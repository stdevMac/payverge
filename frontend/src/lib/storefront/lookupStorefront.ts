import { getServerApiUrl } from "@/lib/serverApiUrl";
import { stripOperatorLocalePrefix } from "@/utils/requestLocale";
import {
  normalizeGuestLangParam,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";

const STOREFRONT_LOOKUP_TIMEOUT_MS = 2500;

export type StorefrontLookup =
  | { kind: "found"; defaultLanguage?: StorefrontLocale }
  | { kind: "not_found" }
  | { kind: "unavailable" };

/**
 * Edge/SSR existence check for /b/:slug (and locale-prefixed /es/b/:slug).
 * Empty API URL and 404-shaped misses are not_found; 5xx/network stay unavailable
 * so a backend blip does not deindex a real storefront.
 */
export async function lookupStorefront(
  slug: string,
): Promise<StorefrontLookup> {
  const apiUrl = getServerApiUrl();
  if (!apiUrl || !slug) return { kind: "not_found" };

  try {
    const res = await fetch(
      `${apiUrl}/business/${encodeURIComponent(slug)}`,
      {
        cache: "no-store",
        signal: AbortSignal.timeout(STOREFRONT_LOOKUP_TIMEOUT_MS),
      },
    );
    if (res.status === 404) return { kind: "not_found" };
    if (!res.ok) return { kind: "unavailable" };
    const data = (await res.json()) as { default_language?: unknown } | null;
    if (data == null) return { kind: "not_found" };
    const defaultLanguage = normalizeGuestLangParam(
      typeof data.default_language === "string" ? data.default_language : null,
    );
    return defaultLanguage
      ? { kind: "found", defaultLanguage }
      : { kind: "found" };
  } catch {
    return { kind: "unavailable" };
  }
}

export function matchStorefrontRoute(
  pathname: string,
): { slug: string } | null {
  const stripped = pathname.replace(/\/+$/, "") || "/";
  const unprefixed = stripOperatorLocalePrefix(stripped);
  const match = unprefixed.match(/^\/b\/([^/]+)$/);
  if (!match?.[1]) return null;
  try {
    return { slug: decodeURIComponent(match[1]) };
  } catch {
    return { slug: match[1] };
  }
}
