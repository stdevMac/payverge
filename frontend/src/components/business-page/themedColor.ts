import {
  BRAND_ON_PRIMARY_TEXT,
  contrastRatio,
  MIN_AA_CONTRAST,
} from "@/lib/contrast";
import { sanitizeCssColor } from "@/lib/storefront/theme";

/**
 * Color helpers for merchant-themed storefront CTAs (storefront plan 2.1).
 *
 * The business-page editor already enforces WCAG AA for primary against white
 * text, but design_settings is a public surface that can also arrive via older
 * clients or direct API writes. These helpers defensively darken any sub-AA
 * primary until white text clears the floor, so a guest-facing solid CTA can
 * never render unreadable label text.
 */

/** Normalize any sanitizeCssColor-acceptable input to lowercase #rrggbb. */
function normalizeHex6(color: string): string {
  const safe = sanitizeCssColor(color); // falls back to opaque black on garbage
  let base = safe.slice(1);
  if (base.length === 3) {
    base = base
      .split("")
      .map((c) => c + c)
      .join("");
  }
  if (base.length === 8) {
    base = base.slice(0, 6);
  }
  return `#${base}`.toLowerCase();
}

/** Multiply RGB channels by `factor` (< 1 darkens, > 1 lightens). */
function darkenHex(color: string, factor: number): string {
  const hex = normalizeHex6(color);
  const channels = [1, 3, 5].map((offset) =>
    Math.max(
      0,
      Math.min(
        255,
        Math.round(Number.parseInt(hex.slice(offset, offset + 2), 16) * factor),
      ),
    ),
  );
  return `#${channels.map((c) => c.toString(16).padStart(2, "0")).join("")}`;
}

const DARKEN_STEP = 0.92;
const MAX_STEPS = 40;

/**
 * Return `color` unchanged when white text on it clears `minRatio` (WCAG AA
 * 4.5:1 by default); otherwise darken in small steps until it does. The step
 * budget is generous — even pure white converges in ~10 steps.
 */
export function ensureContrastWithWhiteText(
  color: string,
  minRatio: number = MIN_AA_CONTRAST,
): string {
  let current = normalizeHex6(color);
  let steps = 0;
  while (
    contrastRatio(BRAND_ON_PRIMARY_TEXT, current) < minRatio &&
    steps < MAX_STEPS
  ) {
    current = darkenHex(current, DARKEN_STEP);
    steps += 1;
  }
  return current;
}
