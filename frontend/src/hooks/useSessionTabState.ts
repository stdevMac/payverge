"use client";

import { useCallback, useEffect, useState } from "react";

/**
 * Session-scoped twin of useLocalStorageState for ephemeral operator UI state
 * (M6): a rail's inner tab, view mode, or search filter should survive
 * dashboard rail switches for the length of this browser tab — but not leak
 * into tomorrow's shift the way localStorage would.
 *
 * Same contract as useLocalStorageState: SSR-safe (never touches `window`
 * during initial render), `null` key degrades to plain useState, malformed
 * storage is ignored. Adds an optional `isValid` guard so a persisted value
 * that is no longer legal (a retired tab key, a stale enum) falls back to
 * `initial` instead of bricking the view.
 */
export function useSessionTabState<T>(
  key: string | null,
  initial: T,
  isValid?: (value: unknown) => boolean,
) {
  const [state, setState] = useState<T>(initial);

  useEffect(() => {
    if (!key) return;
    try {
      const stored = window.sessionStorage.getItem(key);
      if (stored === null) return;
      const parsed = JSON.parse(stored) as unknown;
      if (isValid && !isValid(parsed)) return;
      setState(parsed as T);
    } catch {
      /* ignore malformed JSON or missing storage */
    }
    // initial/isValid are render-constant by contract; only re-hydrate on re-key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  const update = useCallback(
    (value: T) => {
      setState(value);
      if (!key) return;
      try {
        window.sessionStorage.setItem(key, JSON.stringify(value));
      } catch {
        /* ignore storage quota / disabled */
      }
    },
    [key],
  );

  return [state, update] as const;
}
