/**
 * Single source of truth for storefront (public business page) theming.
 *
 * Replaces three divergent implementations that used to exist:
 * - PATTERN_SVGS in ConvertingBusinessLandingPage.tsx (public page, currentColor)
 * - generatePatternCSS in api/publicBusiness.ts (CSS gradients)
 * - getPatternUrl in DesignCustomization.tsx (editor preview, different artwork)
 *
 * The same SVG geometry now backs both the guest-facing pattern layer and the
 * operator's editor preview, so what the operator picks is what guests see.
 */

export const STOREFRONT_PATTERN_IDS = [
  "stripes",
  "dots",
  "grid",
  "waves",
  "geometric",
  "hexagon",
  "circles",
  "mandala",
] as const;

export type StorefrontPatternId = (typeof STOREFRONT_PATTERN_IDS)[number];

/**
 * Pattern artwork keyed by id. Each entry is the inner markup of an SVG
 * `<pattern>` element using `currentColor`, so the public page controls the
 * tint via the text color of the surrounding svg.
 */
const PATTERN_DEFS: Record<StorefrontPatternId, { width: number; height: number; body: string }> = {
  stripes: {
    width: 8,
    height: 8,
    body: `<rect width="4" height="8" fill="currentColor"/>`,
  },
  dots: {
    width: 20,
    height: 20,
    body: `<circle cx="10" cy="10" r="2" fill="currentColor"/>`,
  },
  grid: {
    width: 20,
    height: 20,
    body: `<path d="M 20 0 L 0 0 0 20" fill="none" stroke="currentColor" stroke-width="1"/>`,
  },
  waves: {
    width: 40,
    height: 20,
    body: `<path d="M0 10 Q10 0 20 10 T40 10" fill="none" stroke="currentColor" stroke-width="2"/>`,
  },
  geometric: {
    width: 30,
    height: 30,
    body: `<polygon points="15,5 25,20 5,20" fill="none" stroke="currentColor" stroke-width="1"/><circle cx="15" cy="22" r="3" fill="none" stroke="currentColor" stroke-width="1"/>`,
  },
  hexagon: {
    width: 28,
    height: 24,
    body: `<polygon points="14,2 22,7 22,17 14,22 6,17 6,7" fill="none" stroke="currentColor" stroke-width="1"/>`,
  },
  circles: {
    width: 30,
    height: 30,
    body: `<circle cx="15" cy="15" r="8" fill="none" stroke="currentColor" stroke-width="1"/><circle cx="5" cy="5" r="3" fill="none" stroke="currentColor" stroke-width="1"/><circle cx="25" cy="25" r="3" fill="none" stroke="currentColor" stroke-width="1"/>`,
  },
  mandala: {
    width: 60,
    height: 60,
    body: `<g transform="translate(30,30)"><circle r="20" fill="none" stroke="currentColor" stroke-width="1"/><circle r="15" fill="none" stroke="currentColor" stroke-width="1"/><circle r="10" fill="none" stroke="currentColor" stroke-width="1"/><circle r="5" fill="none" stroke="currentColor" stroke-width="1"/><path d="M0,-20 L0,20 M-20,0 L20,0 M-14,-14 L14,14 M-14,14 L14,-14" stroke="currentColor" stroke-width="1"/></g>`,
  },
};

export function isStorefrontPattern(value: string | undefined | null): value is StorefrontPatternId {
  return !!value && value in PATTERN_DEFS;
}

/**
 * `<pattern>` element markup for the public page's inline SVG defs.
 * Hardcoded, safe for dangerouslySetInnerHTML.
 */
export function getPatternSvgDef(id: StorefrontPatternId): string {
  const def = PATTERN_DEFS[id];
  const rotate = id === "stripes" ? ` patternTransform="rotate(45)"` : "";
  return `<pattern id="${id}" patternUnits="userSpaceOnUse" width="${def.width}" height="${def.height}"${rotate}>${def.body}</pattern>`;
}

/**
 * Data-URI preview of the SAME pattern artwork for the editor's swatch grid,
 * tinted with an explicit color (the editor swatches render on colored chips,
 * so currentColor is not available there).
 */
export function getPatternDataUri(id: StorefrontPatternId, color: string): string {
  const def = PATTERN_DEFS[id];
  const safe = sanitizeCssColor(color);
  const body = def.body.replaceAll("currentColor", safe);
  const svg = `<svg width='${def.width}' height='${def.height}' viewBox='0 0 ${def.width} ${def.height}' xmlns='http://www.w3.org/2000/svg'>${body}</svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}

/**
 * Allow only hex colors (#rgb / #rrggbb / #rrggbbaa). Business-controlled color
 * settings reach CSS strings here; an unvalidated value like
 * `red; background-image:url(evil)` would inject arbitrary CSS. Fall back to a
 * safe opaque black on any non-conforming input.
 */
/* eslint-disable no-restricted-syntax -- CSS safety fallback hex literal, not a Tailwind class */
export const sanitizeCssColor = (color: string): string => {
  return /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(color)
    ? color
    : "#000000";
};
/* eslint-enable no-restricted-syntax */

/**
 * Append an alpha channel to a hex color, tolerating #rgb / #rrggbb / #rrggbbaa.
 * Previously done with `${hex}33` string concatenation, which silently produced
 * invalid CSS for 3-digit or named colors. Returns `transparent` on garbage
 * input rather than emitting broken CSS.
 */
export function hexWithAlpha(color: string, alphaHex: string): string {
  const safe = sanitizeCssColor(color);
  let base = safe.slice(1);
  if (base.length === 3) {
    base = base.split("").map((c) => c + c).join("");
  }
  if (base.length === 8) {
    base = base.slice(0, 6);
  }
  return `#${base}${alphaHex}`;
}

/** design_settings.font_family → Tailwind class. Single lookup; used to be
 *  duplicated verbatim in ConvertingBusinessLandingPage, PublicMenuDisplay,
 *  and BusinessPageLivePreview. */
export function resolveStorefrontFontClass(fontFamily: string | undefined | null): string {
  return fontFamily === "Serif" ? "font-title" : "font-sans";
}

/** design_settings.corner_radius → CSS length. */
export function resolveCornerRadius(cornerRadius: string | undefined | null): string {
  switch (cornerRadius) {
    case "none":
      return "0px";
    case "small":
      return "0.25rem";
    case "large":
      return "0.75rem";
    default:
      return "0.5rem";
  }
}
