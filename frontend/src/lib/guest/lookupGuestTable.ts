import { getServerApiUrl } from "@/lib/serverApiUrl";
import { fetchWithTransientRetry } from "@/lib/guest/fetchWithTransientRetry";
import {
  normalizeGuestLangParam,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";

const GUEST_TABLE_LOOKUP_TIMEOUT_MS = 2500;

export type GuestTableLookup =
  | {
      kind: "found";
      businessName: string | null;
      defaultLanguage?: StorefrontLocale;
    }
  | { kind: "not_found" }
  | { kind: "unavailable" };

/**
 * SSR/edge existence check for /t/:code.
 * Empty API URL and 404-shaped misses are not_found; 5xx/network stay unavailable.
 */
export async function lookupGuestTable(
  tableCode: string,
  path: "table" | "business" = "table",
): Promise<GuestTableLookup> {
  const apiUrl = getServerApiUrl();
  if (!apiUrl) return { kind: "not_found" };

  const suffix = path === "business" ? "/business" : "";
  try {
    const res = await fetchWithTransientRetry(
      `${apiUrl}/guest/table/${encodeURIComponent(tableCode)}${suffix}`,
      {
        cache: "no-store",
        signal: AbortSignal.timeout(GUEST_TABLE_LOOKUP_TIMEOUT_MS),
      },
    );
    if (res.status === 404) return { kind: "not_found" };
    if (!res.ok) return { kind: "unavailable" };
    const data = (await res.json()) as {
      business?: { name?: string; default_language?: string };
      default_language?: string;
    } | null;
    if (data == null) return { kind: "not_found" };
    const defaultLanguage = normalizeGuestLangParam(
      data.business?.default_language ?? data.default_language,
    );
    return {
      kind: "found",
      businessName: data.business?.name ?? null,
      ...(defaultLanguage ? { defaultLanguage } : {}),
    };
  } catch {
    return { kind: "unavailable" };
  }
}

export function matchGuestTableRoute(
  pathname: string,
): { tableCode: string; surface: "table" | "menu" } | null {
  const stripped = pathname.replace(/\/+$/, "") || "/";
  const match = stripped.match(
    /^\/t\/([^/]+)(?:\/(menu|bill|profile|signin))?$/,
  );
  if (!match?.[1]) return null;
  return {
    tableCode: match[1],
    surface: match[2] === "menu" ? "menu" : "table",
  };
}
