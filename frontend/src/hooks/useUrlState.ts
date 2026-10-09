"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  buildSearchWithParam,
  buildSearchWithParams,
  needsOptionalUrlNormalization,
  needsUrlNormalization,
  normalizeOptionalUrlParam,
  normalizeUrlParam,
  type UrlHistoryMode,
  type UrlParamPatch,
} from "./urlState";

export type { UrlHistoryMode, UrlParamPatch };

export interface UseUrlStateOptions<T extends string> {
  /** Query-string key this piece of view state owns. */
  key: string;
  /** Allow-list of valid values. */
  valid: readonly T[];
  /** Value used when the param is missing or invalid. */
  fallback: T;
  /**
   * When true (default), a value equal to `fallback` is omitted from the URL
   * so default views stay clean.
   */
  omitDefault?: boolean;
  /**
   * History mode for user-initiated setValue writes.
   * Normalization of invalid URL values always uses `replace`.
   * Default: `replace`.
   */
  history?: UrlHistoryMode;
  /**
   * Only rewrite the URL while this query param is one of `values`.
   * Shared keys (Settings and Business Page both own `section`) must set this
   * to their dashboard `tab` so a still-mounted previous owner cannot
   * normalize the incoming tab's value away (#225).
   */
  activeWhen?: { key: string; values: readonly string[] };
}

function useNavRefs() {
  const searchParams = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const routerRef = useRef(router);
  const pathnameRef = useRef(pathname);
  const searchParamsRef = useRef(searchParams);
  useEffect(() => {
    routerRef.current = router;
    pathnameRef.current = pathname;
    searchParamsRef.current = searchParams;
  }, [router, pathname, searchParams]);
  return { searchParams, pathname, router, routerRef, pathnameRef, searchParamsRef };
}

type NavRouter = { replace: (href: string, opts?: { scroll?: boolean }) => void; push: (href: string, opts?: { scroll?: boolean }) => void };

function commitSearch(
  routerRef: { current: NavRouter },
  pathnameRef: { current: string | null },
  searchParamsRef: { current: URLSearchParams | null },
  nextSearch: string,
  mode: UrlHistoryMode,
) {
  const path = pathnameRef.current || "/";
  const href = nextSearch ? `${path}?${nextSearch}` : path;
  const navigate = mode === "push" ? routerRef.current.push : routerRef.current.replace;
  navigate(href, { scroll: false });
  // Same-tick sibling writers must see this search, not the pre-replace snapshot.
  searchParamsRef.current = new URLSearchParams(nextSearch);
}

function navigateTo(
  routerRef: { current: NavRouter },
  pathnameRef: { current: string | null },
  searchParamsRef: { current: URLSearchParams | null },
  key: string,
  value: string | null,
  options: { fallback?: string; omitDefault?: boolean },
  mode: UrlHistoryMode,
) {
  const current = searchParamsRef.current?.toString() ?? "";
  const nextSearch = buildSearchWithParam(current, key, value, {
    fallback: options.fallback,
    omitDefault: options.omitDefault,
  });
  commitSearch(routerRef, pathnameRef, searchParamsRef, nextSearch, mode);
}

/**
 * Shared URL ↔ view-state binding (Session P / Root B).
 *
 * - Reads `key` from the current search params
 * - Validates against `valid`; invalid → `fallback`
 * - Rewrites the URL via `router.replace` when the raw value was invalid
 * - `setValue` writes back on every in-page change
 * - SSR-safe: never touches `window` during render; relies on Next App Router
 *   `useSearchParams` / `usePathname` / `useRouter`
 */
function isUrlStateActive(
  searchParams: URLSearchParams | null,
  activeWhen?: { key: string; values: readonly string[] },
): boolean {
  if (!activeWhen) return true;
  const owner = searchParams?.get(activeWhen.key) ?? null;
  return owner != null && activeWhen.values.includes(owner);
}

export function useUrlState<T extends string>(
  options: UseUrlStateOptions<T>,
): [T, (next: T) => void] {
  const {
    key,
    valid,
    fallback,
    omitDefault = true,
    history = "replace",
    activeWhen,
  } = options;
  const { searchParams, routerRef, pathnameRef, searchParamsRef } = useNavRefs();
  const active = isUrlStateActive(searchParams, activeWhen);

  const raw = searchParams?.get(key) ?? null;
  const value = useMemo(
    () => normalizeUrlParam(raw, valid, fallback),
    [raw, valid, fallback],
  );

  // Normalize invalid/stale URL values with replace (no history spam).
  useEffect(() => {
    if (!active) return;
    if (!needsUrlNormalization(raw, value, { fallback, omitDefault })) {
      return;
    }
    navigateTo(routerRef, pathnameRef, searchParamsRef, key, value, {
      fallback,
      omitDefault,
    }, "replace");
  }, [active, raw, value, key, fallback, omitDefault, routerRef, pathnameRef, searchParamsRef]);

  const setValue = useCallback(
    (next: T) => {
      if (!active) return;
      navigateTo(routerRef, pathnameRef, searchParamsRef, key, next, {
        fallback,
        omitDefault,
      }, history);
    },
    [active, key, fallback, omitDefault, history, routerRef, pathnameRef, searchParamsRef],
  );

  return [value, setValue];
}

export interface UseOptionalUrlStateOptions<T extends string> {
  key: string;
  valid: readonly T[];
  history?: UrlHistoryMode;
}

/**
 * Optional URL enum param (e.g. CRM `?focus=`): absence is a valid state
 * (returns null). Invalid garbage is stripped via replace.
 */
export function useOptionalUrlState<T extends string>(
  options: UseOptionalUrlStateOptions<T>,
): [T | null, (next: T | null) => void] {
  const { key, valid, history = "replace" } = options;
  const { searchParams, routerRef, pathnameRef, searchParamsRef } = useNavRefs();

  const raw = searchParams?.get(key) ?? null;
  const value = useMemo(
    () => normalizeOptionalUrlParam(raw, valid),
    [raw, valid],
  );

  useEffect(() => {
    if (!needsOptionalUrlNormalization(raw, value)) return;
    // Strip invalid garbage
    navigateTo(routerRef, pathnameRef, searchParamsRef, key, null, {}, "replace");
  }, [raw, value, key, routerRef, pathnameRef, searchParamsRef]);

  const setValue = useCallback(
    (next: T | null) => {
      navigateTo(routerRef, pathnameRef, searchParamsRef, key, next, {}, history);
    },
    [key, history, routerRef, pathnameRef, searchParamsRef],
  );

  return [value, setValue];
}

/**
 * Write several query keys in a single replace/push. Use this instead of
 * calling two `useUrlState` setters in one handler — each setter reads the
 * current search snapshot, so the second replace drops the first write.
 */
export function useSetUrlParams() {
  const { routerRef, pathnameRef, searchParamsRef } = useNavRefs();
  return useCallback(
    (patches: readonly UrlParamPatch[], mode: UrlHistoryMode = "replace") => {
      const current = searchParamsRef.current?.toString() ?? "";
      const nextSearch = buildSearchWithParams(current, patches);
      commitSearch(routerRef, pathnameRef, searchParamsRef, nextSearch, mode);
    },
    [routerRef, pathnameRef, searchParamsRef],
  );
}
