"use client";

import { useCallback, useMemo, useRef, useState } from "react";

/**
 * Deep-compare form state against a loaded baseline.
 *
 * Not dirty until a baseline has been captured — mirrors the
 * CurrencySettings "not dirty until loaded" guard so a form never lights
 * Save against placeholder defaults during the first fetch.
 *
 * @param current live form value (object/array/primitive). Compared via
 *   `JSON.stringify` so nested edits register without reference equality.
 * @returns `dirty`, plus `markClean` / `clearBaseline` to re-baseline after
 *   load/save or to return to the not-loaded state.
 *
 * Prefer `markClean(snapshot)` when calling immediately after a `setState` —
 * React has not re-rendered yet, so the closed-over `current` may still be
 * the previous value. `markClean` itself is referentially stable so it is
 * safe to put in load-effect dependency arrays.
 */
export function useDirtyForm<T>(current: T): {
  dirty: boolean;
  markClean: (snapshot?: T) => void;
  clearBaseline: () => void;
} {
  const [baseline, setBaseline] = useState<string | null>(null);
  const currentRef = useRef(current);
  currentRef.current = current;

  const dirty = useMemo(() => {
    if (baseline === null) return false;
    return JSON.stringify(current) !== baseline;
  }, [baseline, current]);

  const markClean = useCallback((snapshot?: T) => {
    const value = snapshot !== undefined ? snapshot : currentRef.current;
    setBaseline(JSON.stringify(value));
  }, []);

  const clearBaseline = useCallback(() => {
    setBaseline(null);
  }, []);

  return { dirty, markClean, clearBaseline };
}
