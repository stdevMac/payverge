"use client";

import { useCallback, useEffect, useState } from "react";

/**
 * Synchronously initializes state with `initial`, then hydrates from
 * localStorage on mount. `update` writes both state and storage.
 * SSR-safe: never touches `window` during initial render.
 *
 * When `key` is `null`, behaves like a plain `useState(initial)` — no
 * localStorage read or write occurs. This lets callers defer the key
 * until a required value (e.g. a business ID) is available.
 */
export function useLocalStorageState<T>(key: string | null, initial: T) {
  const [state, setState] = useState<T>(initial);

  useEffect(() => {
    if (!key) return;
    try {
      const stored = window.localStorage.getItem(key);
      if (stored !== null) {
        setState(JSON.parse(stored) as T);
      }
    } catch {
      /* ignore malformed JSON or missing storage */
    }
  }, [key]);

  const update = useCallback(
    (value: T) => {
      setState(value);
      if (!key) return;
      try {
        window.localStorage.setItem(key, JSON.stringify(value));
      } catch {
        /* ignore storage quota / disabled */
      }
    },
    [key],
  );

  return [state, update] as const;
}
