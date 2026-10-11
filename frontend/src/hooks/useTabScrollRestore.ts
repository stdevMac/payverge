"use client";

/**
 * M2 — per-tab scroll retention for the dashboard scroller.
 *
 * The dashboard remounts the rail panel on every tab switch, so the browser's
 * native scroll memory is useless here. This hook keeps an in-memory map of
 * `scope:tab → scrollTop` and restores it on return — instantly (the layout's
 * `scroll-smooth` class must not animate the jump) and twice: once after the
 * first paint, once 350ms later for async chunk content that grows the page.
 *
 * In-memory only by design: positions are ephemeral navigation context, not
 * operator data (M6 owns persisted per-tab state via sessionStorage).
 */

import React from "react";

const positions = new Map<string, number>();

const RESTORE_DELAY_MS = 350;

export function useTabScrollRestore(
  ref: React.RefObject<HTMLElement | null>,
  tabKey: string,
  scope: string,
): void {
  const key = `${scope}:${tabKey}`;

  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const restore = () => {
      el.scrollTo({ top: positions.get(key) ?? 0, behavior: "instant" });
    };
    const raf = requestAnimationFrame(restore);
    const followUp = setTimeout(restore, RESTORE_DELAY_MS);

    return () => {
      cancelAnimationFrame(raf);
      clearTimeout(followUp);
      // Leaving the tab (or the dashboard): remember where the operator was.
      positions.set(key, el.scrollTop);
    };
  }, [key, ref]);
}
