/**
 * Server-side read of GET /api/v1/home: what the instance root ("/") serves.
 *
 *   - mode "venue":     one storefront (PRIMARY_VENUE, or the only published one)
 *   - mode "directory": several published storefronts
 *   - mode "empty":     nothing published yet → "/" sends visitors to /dashboard
 *
 * Read by middleware (guest locale + empty redirect), the "/" page and the
 * /b/[customUrl] page (so the primary venue canonicalizes to the site root).
 * Cached like serverInstance: a good answer is reused for HOME_TTL_MS and
 * kept as the fallback when a refresh fails; with no answer at all the
 * result is null and callers render the directory/empty fallbacks.
 */

import { getServerApiUrl } from "@/lib/serverApiUrl";

export const HOME_TTL_MS = 30_000;
const HOME_FETCH_TIMEOUT_MS = 1500;
export const HOME_FAILURE_TTL_MS = 5_000;

type HomeMode = "venue" | "directory" | "empty";

export interface HomeVenue {
  id: number;
  name: string;
  logo: string;
  custom_url: string;
  city: string;
}

export interface HomeInfo {
  mode: HomeMode;
  primary: HomeVenue | null;
  venues: HomeVenue[];
}

function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function parseVenue(raw: unknown): HomeVenue | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const customUrl = str(r.custom_url).trim();
  const id = typeof r.id === "number" ? r.id : Number(r.id);
  if (!customUrl || !Number.isFinite(id)) return null;
  return {
    id,
    name: str(r.name) || customUrl,
    logo: str(r.logo),
    custom_url: customUrl,
    city: str(r.city),
  };
}

/** Defensive parse of the /home payload; null when the shape is unusable. */
export function parseHomeInfo(raw: unknown): HomeInfo | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const venues = Array.isArray(r.venues)
    ? r.venues.map(parseVenue).filter((v): v is HomeVenue => v !== null)
    : [];
  const primary = parseVenue(r.primary);
  if (r.mode === "venue" && primary) {
    return { mode: "venue", primary, venues };
  }
  if (r.mode === "directory" && venues.length > 0) {
    return { mode: "directory", primary: null, venues };
  }
  if (r.mode === "empty") {
    return { mode: "empty", primary: null, venues: [] };
  }
  return null;
}

let lastGood: HomeInfo | null = null;
let expiresAt = 0;
let inflight: Promise<HomeInfo | null> | null = null;

/** Test-only: forget the cached payload. */
export function resetServerHomeCacheForTests(): void {
  lastGood = null;
  expiresAt = 0;
  inflight = null;
}

async function fetchHome(): Promise<HomeInfo | null> {
  const apiUrl = getServerApiUrl();
  if (!apiUrl) return null;
  try {
    const res = await fetch(`${apiUrl}/home`, {
      cache: "no-store",
      headers: { accept: "application/json" },
      signal: AbortSignal.timeout(HOME_FETCH_TIMEOUT_MS),
    });
    if (!res.ok) return null;
    return parseHomeInfo(await res.json());
  } catch {
    return null;
  }
}

/** The /home payload, cached; null when the backend never answered. */
export async function getServerHome(
  now: number = Date.now(),
): Promise<HomeInfo | null> {
  if (now < expiresAt) return lastGood;
  if (!inflight) {
    inflight = fetchHome()
      .then((info) => {
        if (info) {
          lastGood = info;
          expiresAt = Date.now() + HOME_TTL_MS;
        } else {
          expiresAt = Date.now() + HOME_FAILURE_TTL_MS;
        }
        return info ?? lastGood;
      })
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

/** True when `slug` is the venue served at the site root. */
export function isPrimaryVenueSlug(
  home: HomeInfo | null,
  slug: string,
): boolean {
  return (
    home?.mode === "venue" &&
    home.primary != null &&
    home.primary.custom_url.toLowerCase() === slug.toLowerCase()
  );
}
