/**
 * Pure URL-state helpers (Session P / S-7).
 *
 * The URL is the source of view state: read → validate → normalize invalid to
 * the fallback AND rewrite the URL → write back on every in-page change.
 * History policy: `replace` for normalization/cleanup; callers may opt into
 * `push` for user-initiated navigation that should appear in the back stack.
 */

export type UrlHistoryMode = "replace" | "push";

/**
 * Resolve a raw query value against an allow-list, returning the fallback when
 * the raw value is missing or not in `valid`.
 */
export function normalizeUrlParam<T extends string>(
  raw: string | null | undefined,
  valid: readonly T[],
  fallback: T,
): T {
  if (raw != null && (valid as readonly string[]).includes(raw)) {
    return raw as T;
  }
  return fallback;
}

/**
 * Build the next search string for a single key, preserving other params.
 * - When `value` equals `fallback` and `omitDefault` is true (default), the key
 *   is removed so default views stay clean.
 * - When `value` is null/undefined/empty string, the key is removed.
 * Returns the search string WITHOUT a leading `?` (empty string when no params).
 */
export function buildSearchWithParam(
  current: URLSearchParams | string,
  key: string,
  value: string | null | undefined,
  options?: { fallback?: string; omitDefault?: boolean },
): string {
  const next = new URLSearchParams(
    typeof current === "string" ? current : current.toString(),
  );
  const omitDefault = options?.omitDefault !== false;
  const fallback = options?.fallback;

  if (value == null || value === "") {
    next.delete(key);
  } else if (omitDefault && fallback !== undefined && value === fallback) {
    next.delete(key);
  } else {
    next.set(key, value);
  }

  return next.toString();
}

/** One key/value write applied by `buildSearchWithParams`. */
export type UrlParamPatch = {
  key: string;
  value: string | null | undefined;
  fallback?: string;
  omitDefault?: boolean;
};

/**
 * Apply several query-string patches in one pass so callers can update
 * related keys (e.g. CRM `tab` + `sub` + `focus`) without a second
 * `router.replace` that races and drops the first write.
 */
export function buildSearchWithParams(
  current: URLSearchParams | string,
  patches: readonly UrlParamPatch[],
): string {
  return patches.reduce(
    (search, patch) =>
      buildSearchWithParam(search, patch.key, patch.value, {
        fallback: patch.fallback,
        omitDefault: patch.omitDefault,
      }),
    typeof current === "string" ? current : current.toString(),
  );
}

/**
 * True when the raw URL value differs from the normalized value (caller should
 * rewrite via replace to clear garbage without spamming history).
 */
export function needsUrlNormalization(
  raw: string | null | undefined,
  normalized: string,
  options?: { omitDefault?: boolean; fallback?: string },
): boolean {
  const omitDefault = options?.omitDefault !== false;
  const fallback = options?.fallback;

  // Missing key while normalized is the fallback → URL is already clean.
  if ((raw == null || raw === "") && normalized === fallback) {
    return false;
  }
  // Missing key but normalized is not fallback (shouldn't happen for enum
  // params) — treat as needing write if we want the key present.
  if (raw == null || raw === "") {
    // With omitDefault, absence of the default is correct.
    if (omitDefault && normalized === fallback) return false;
    return true;
  }
  return raw !== normalized;
}

/**
 * Optional enum param: missing → null; valid → value; invalid → null (caller
 * should strip the garbage key via replace).
 */
export function normalizeOptionalUrlParam<T extends string>(
  raw: string | null | undefined,
  valid: readonly T[],
): T | null {
  if (raw != null && raw !== "" && (valid as readonly string[]).includes(raw)) {
    return raw as T;
  }
  return null;
}

/** True when an optional param is present but invalid and must be stripped. */
export function needsOptionalUrlNormalization(
  raw: string | null | undefined,
  normalized: string | null,
): boolean {
  if (raw == null || raw === "") return false;
  return raw !== normalized;
}
