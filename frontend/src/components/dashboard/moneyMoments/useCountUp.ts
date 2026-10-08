"use client";

import { useEffect, useRef, useState } from "react";

/** True when the user has asked the OS to minimize motion. SSR-safe. */
function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return false;
  }
  try {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  } catch {
    return false;
  }
}

interface CountUpOptions {
  durationMs?: number;
  /** Start value for the animation; defaults to the target (no mount animation). */
  startFrom?: number;
}

/**
 * Animate a number toward `target` with an ease-out curve. Honors reduced-motion
 * (and environments without rAF) by snapping straight to the target. Pass
 * `startFrom: 0` to get the satisfying mount tick-up used by the celebration card.
 */
export function useCountUp(target: number, { durationMs = 650, startFrom }: CountUpOptions = {}): number {
  const initial = startFrom ?? target;
  const [value, setValue] = useState(initial);
  const fromRef = useRef(initial);
  const rafRef = useRef<number | null>(null);

  useEffect(() => {
    const from = fromRef.current;
    if (from === target) return;

    if (
      prefersReducedMotion() ||
      typeof window === "undefined" ||
      typeof window.requestAnimationFrame !== "function"
    ) {
      fromRef.current = target;
      setValue(target);
      return;
    }

    let start: number | null = null;
    const tick = (ts: number) => {
      if (start === null) start = ts;
      const progress = Math.min(1, (ts - start) / durationMs);
      const eased = 1 - Math.pow(1 - progress, 3); // easeOutCubic
      setValue(from + (target - from) * eased);
      if (progress < 1) {
        rafRef.current = window.requestAnimationFrame(tick);
      } else {
        fromRef.current = target;
        setValue(target);
      }
    };

    rafRef.current = window.requestAnimationFrame(tick);
    return () => {
      if (rafRef.current !== null) window.cancelAnimationFrame(rafRef.current);
    };
  }, [target, durationMs]);

  return value;
}
