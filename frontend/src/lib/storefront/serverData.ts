import {
  normalizePublicBusiness,
  type PublicBusiness,
} from "@/api/publicBusiness";
import { getServerApiUrl } from "@/lib/serverApiUrl";
import { fetchWithTransientRetry } from "@/lib/guest/fetchWithTransientRetry";
import { SsrLastKnownGoodCache } from "@/lib/guest/ssrLastKnownGood";

/**
 * Server-side storefront fetch helpers, shared by
 * `app/b/[customUrl]/page.tsx` (SSR body + generateMetadata) and
 * `app/b/[customUrl]/opengraph-image.tsx` (dynamic OG card).
 *
 * Everything here is best-effort and `cache: "no-store"` (except the Google
 * rating, see below): the storefront is publish-gated, so a stale snapshot
 * must never outlive an unpublish.
 * Last-known-good is only consulted on *transient* failures (5xx/network),
 * never on a confirmed 404.
 */

export type BusinessFetchResult =
  | { business: PublicBusiness }
  | { business: null; reason: "not_found" | "unavailable" };

const inflightBusinessFetches = new Map<string, Promise<BusinessFetchResult>>();
const lastKnownBusiness = new SsrLastKnownGoodCache<PublicBusiness>();

export function clearStorefrontBusinessSsrCache(): void {
  inflightBusinessFetches.clear();
  lastKnownBusiness.clear();
}

function lastKnownResult(customUrl: string): BusinessFetchResult | null {
  const cached = lastKnownBusiness.get(customUrl);
  return cached ? { business: { ...cached } } : null;
}

async function loadStorefrontBusiness(
  customUrl: string,
  language?: string,
): Promise<BusinessFetchResult> {
  try {
    const qs = language ? `?language=${encodeURIComponent(language)}` : "";
    const res = await fetchWithTransientRetry(
      `${getServerApiUrl()}/business/${customUrl}${qs}`,
      { cache: "no-store" },
    );
    if (res.status === 404) {
      lastKnownBusiness.delete(customUrl);
      return { business: null, reason: "not_found" };
    }
    if (!res.ok) {
      return lastKnownResult(customUrl) ?? { business: null, reason: "unavailable" };
    }
    const data = await res.json();
    const business = normalizePublicBusiness(data);
    lastKnownBusiness.set(customUrl, business);
    return { business };
  } catch {
    // Network or fetch error - treat as transient so we don't falsely claim 404.
    // A known-live slug must not drop to "Loading business…" after a later
    // stampede (#685); serve last-known-good when we have it.
    return lastKnownResult(customUrl) ?? { business: null, reason: "unavailable" };
  }
}

/**
 * Fetch the public business by custom URL. When a validated guest language is
 * supplied, the backend returns the translated storefront so the SERP/share
 * snippet matches the visitor's language.
 *
 * Concurrent callers in the same SSR request (generateMetadata + page + OG)
 * share one in-flight promise so a burst of storefront renders does not
 * multiply backend load into "temporarily unavailable" HTML.
 */
export function fetchStorefrontBusiness(
  customUrl: string,
  language?: string,
): Promise<BusinessFetchResult> {
  const key = `${customUrl}::${language ?? ""}`;
  const existing = inflightBusinessFetches.get(key);
  if (existing) return existing;
  const pending = loadStorefrontBusiness(customUrl, language).finally(() => {
    inflightBusinessFetches.delete(key);
  });
  inflightBusinessFetches.set(key, pending);
  return pending;
}

export const STOREFRONT_MENU_FETCH_TIMEOUT_MS = 5000;

/**
 * Raw guest-menu payload for the SSR cache seed + crawlable menu block.
 * Returns null on ANY failure — a menu outage must never fail the page.
 * Normalize with `normalizeStorefrontMenuPayload` (shared with the client
 * hook so the dehydrated React Query entry matches the client queryFn shape).
 */
export async function fetchStorefrontMenuPayload(
  customUrl: string,
  language?: string,
): Promise<unknown | null> {
  try {
    const qs = language ? `?language=${encodeURIComponent(language)}` : "";
    const res = await fetch(
      `${getServerApiUrl()}/business/${customUrl}/menu${qs}`,
      {
        cache: "no-store",
        signal: AbortSignal.timeout(STOREFRONT_MENU_FETCH_TIMEOUT_MS),
      },
    );
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

export interface StorefrontGoogleRating {
  ratingValue: number;
  reviewCount: number;
}

/**
 * Google rating data changes slowly and the backend already caches the
 * upstream Places response per Place ID. Revalidating here keeps crawler and
 * OG-card bursts from re-hitting the backend on every render. Only the rating
 * is reused: the page itself stays gated by the no-store business fetch, so an
 * unpublished venue still 404s immediately.
 */
export const STOREFRONT_GOOGLE_RATING_REVALIDATE_SECONDS = 6 * 60 * 60;

/**
 * Best-effort Google rating for JSON-LD `aggregateRating` and the OG card.
 * Returns null unless a positive numeric rating is present.
 */
export async function fetchStorefrontGoogleRating(
  customUrl: string,
): Promise<StorefrontGoogleRating | null> {
  try {
    const res = await fetch(
      `${getServerApiUrl()}/business/${customUrl}/google/details`,
      { next: { revalidate: STOREFRONT_GOOGLE_RATING_REVALIDATE_SECONDS } },
    );
    if (!res.ok) return null;
    const data = await res.json();
    const details = data?.place_details;
    const rating = details?.rating;
    if (typeof rating !== "number" || !(rating > 0)) return null;
    const count = details?.user_ratings_total;
    return {
      ratingValue: rating,
      reviewCount: typeof count === "number" && count > 0 ? count : 0,
    };
  } catch {
    return null;
  }
}
