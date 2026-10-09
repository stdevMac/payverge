/**
 * Geometric hit-test for the offer edit modal at a short viewport.
 * jsdom has no layout engine, so callers pass the live 1024×622 overlap
 * rects; stacking still honors `pointer-events: none`.
 */

export type HitRect = {
  left: number;
  top: number;
  right: number;
  bottom: number;
};

export type HitLayer = {
  el: Element;
  rect: HitRect;
};

export function pointInRect(x: number, y: number, rect: HitRect): boolean {
  return x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom;
}

export function resolvedPointerEvents(el: Element): string {
  if (typeof window !== "undefined") {
    const computed = window.getComputedStyle(el).pointerEvents;
    if (computed && computed !== "auto") return computed;
  }
  // jsdom does not load Tailwind. Honor the utility class the footer ships.
  if (
    el instanceof HTMLElement &&
    el.classList.contains("pointer-events-none")
  ) {
    return "none";
  }
  if (
    typeof el.className === "string" &&
    /(^|\s)pointer-events-none(?:\s|$)/.test(el.className)
  ) {
    return "none";
  }
  return "auto";
}

export function hitTestTopmost(
  x: number,
  y: number,
  layers: HitLayer[],
): Element | null {
  const hits = layers.filter(({ el, rect }) => {
    if (!pointInRect(x, y, rect)) return false;
    if (resolvedPointerEvents(el) === "none") return false;
    return true;
  });
  return hits.at(-1)?.el ?? null;
}

/** Live overlap at 1024×622: Target "Show suggestions" sits in the footer band. */
export const OFFER_FOOTER_OVERLAP_1024x622 = {
  viewport: { width: 1024, height: 622 },
  suggestions: { left: 780, top: 548, right: 820, bottom: 588 },
  footer: { left: 200, top: 530, right: 824, bottom: 622 },
  point: { x: 800, y: 568 },
} as const;
