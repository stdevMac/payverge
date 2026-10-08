"use client";

import { useEffect, useRef } from "react";

/** Polls a callback on an interval; skips the first immediate duplicate when enabled toggles. */
export function useAdminPolling(
  callback: () => void | Promise<void>,
  intervalMs: number,
  enabled = true,
) {
  const saved = useRef(callback);
  saved.current = callback;

  useEffect(() => {
    if (!enabled) return;
    const tick = () => {
      void saved.current();
    };
    const id = setInterval(tick, intervalMs);
    return () => clearInterval(id);
  }, [intervalMs, enabled]);
}
