import { sentenceCaseLeaf } from "@/i18n/sentenceCaseLeaf";

/** Hard fallback when even the venue-native unknown key is a missing leaf. */
export const SEATED_EMPTY = "—";

/** Instants before this year are Go-zero / unix-epoch sentinels, not seated times. */
const MIN_USABLE_YEAR = 2020;

export type SeatedTimeInput = string | number | Date | null | undefined;

export type SeatedTimeSource = {
  created_at?: SeatedTimeInput;
  updated_at?: SeatedTimeInput;
  last_seen?: SeatedTimeInput;
  seated_at?: SeatedTimeInput;
  createdAt?: SeatedTimeInput;
  updatedAt?: SeatedTimeInput;
};

function instantMs(raw: SeatedTimeInput): number | null {
  if (raw == null) return null;
  if (typeof raw === "string" && !raw.trim()) return null;
  const ms =
    raw instanceof Date
      ? raw.getTime()
      : typeof raw === "number"
        ? raw
        : new Date(raw).getTime();
  if (!Number.isFinite(ms)) return null;
  if (new Date(ms).getUTCFullYear() < MIN_USABLE_YEAR) return null;
  return ms;
}

export function firstValidIso(...candidates: SeatedTimeInput[]): string | null {
  for (const raw of candidates) {
    const ms = instantMs(raw);
    if (ms == null) continue;
    return new Date(ms).toISOString();
  }
  return null;
}

/**
 * Seated instant for an occupied table: check opened_at, then row seated_at,
 * then last activity. Bad/missing created_at must not hide a usable last_seen.
 */
export function resolveSeatedIso(source: SeatedTimeSource): string | null {
  return firstValidIso(
    source.created_at,
    source.createdAt,
    source.seated_at,
    source.last_seen,
    source.updated_at,
    source.updatedAt,
  );
}

/** Map /tables/status fields so Table 9 still gets a seated clock. */
export function seatedSourcesFromStatus(tws: {
  seated_at?: SeatedTimeInput;
  last_seen?: SeatedTimeInput;
  active_bills?: Array<{
    created_at?: SeatedTimeInput;
    updated_at?: SeatedTimeInput;
    createdAt?: SeatedTimeInput;
    updatedAt?: SeatedTimeInput;
  } | null>;
}): { created_at: string | null; last_seen: string | null } {
  const first = tws.active_bills?.find((bill) => bill != null) ?? null;
  return {
    created_at: firstValidIso(
      first?.created_at,
      first?.createdAt,
      tws.seated_at,
    ),
    last_seen: firstValidIso(
      first?.updated_at,
      first?.updatedAt,
      tws.last_seen,
      first?.created_at,
      first?.createdAt,
      tws.seated_at,
    ),
  };
}

/**
 * True when getTranslation fell through to the key or its sentence-cased leaf.
 * Compare to sentenceCaseLeaf(key) — never a locale-specific regex.
 */
export function isMissingTranslationLeaf(value: string, key: string): boolean {
  if (!value.trim()) return true;
  if (value === key) return true;
  if (value === sentenceCaseLeaf(key)) return true;
  const leaf = key.split(".").pop() || key;
  return value === leaf || value === sentenceCaseLeaf(leaf);
}

export function translationOrFallback(
  value: string,
  key: string,
  fallback: string,
): string {
  return isMissingTranslationLeaf(value, key) ? fallback : value;
}
