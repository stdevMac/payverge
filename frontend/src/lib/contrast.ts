/**
 * WCAG 2.x relative-luminance contrast for brand colour validation.
 *
 * Brand primary is used as a solid fill (header, chips, FAB) with white
 * label/body text. Brand secondary is used as accent/body text on light
 * surfaces (prices, secondary CTAs). Both pairs must clear WCAG AA 4.5:1.
 */

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/;

/** WCAG AA minimum contrast for normal text. */
export const MIN_AA_CONTRAST = 4.5;

/** White label/body text drawn on primary brand fills. */
// eslint-disable-next-line no-restricted-syntax -- contrast math needs absolute hex, not Tailwind classes
export const BRAND_ON_PRIMARY_TEXT = "#ffffff";

/** Light surface behind secondary brand text. */
// eslint-disable-next-line no-restricted-syntax -- contrast math needs absolute hex, not Tailwind classes
export const BRAND_SECONDARY_SURFACE = "#ffffff";

export type ContrastValidation = {
  ok: boolean;
  ratio: number;
  /** Blocking message when !ok; always names the measured ratio. */
  error?: string;
};

export type BrandColorPreset = {
  key: string;
  primary: string;
  secondary: string;
};

/**
 * Eight shipped quick-presets. Every pair clears AA for its usage role
 * (white-on-primary and secondary-on-white). Keys match i18n
 * `colorPresets.*` entries.
 */
/* eslint-disable no-restricted-syntax -- design tokens stored in DB, not Tailwind */
export const SHIPPED_BRAND_PRESETS: readonly BrandColorPreset[] = [
  { key: "classicGray", primary: "#1f2937", secondary: "#2563eb" },
  { key: "warmBrown", primary: "#92400e", secondary: "#b45309" },
  { key: "forestGreen", primary: "#166534", secondary: "#15803d" },
  { key: "oceanBlue", primary: "#1e40af", secondary: "#1d4ed8" },
  { key: "purplePassion", primary: "#7c3aed", secondary: "#7e22ce" },
  { key: "sunsetOrange", primary: "#c2410c", secondary: "#c2410c" },
  { key: "roseGold", primary: "#be185d", secondary: "#be185d" },
  { key: "midnight", primary: "#0f172a", secondary: "#475569" },
] as const;
/* eslint-enable no-restricted-syntax */

function normalizeHex(color: string): string {
  if (!HEX_COLOR.test(color)) {
    throw new TypeError(
      `Expected a full six-digit hex color, received ${color}`,
    );
  }
  return color.toLowerCase();
}

/** Relative luminance per WCAG 2.x relative luminance definition. */
function relativeLuminance(color: string): number {
  const normalized = normalizeHex(color);
  const channels = [1, 3, 5].map((offset) =>
    Number.parseInt(normalized.slice(offset, offset + 2), 16),
  );
  const [red, green, blue] = channels.map((channel) => {
    const srgb = channel / 255;
    return srgb <= 0.04045 ? srgb / 12.92 : ((srgb + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}

/** WCAG contrast ratio between two strict #RRGGBB colors. */
export function contrastRatio(first: string, second: string): number {
  const firstLuminance = relativeLuminance(first);
  const secondLuminance = relativeLuminance(second);
  const lighter = Math.max(firstLuminance, secondLuminance);
  const darker = Math.min(firstLuminance, secondLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

function formatRatio(ratio: number): string {
  return ratio.toFixed(1);
}

function blockingError(ratio: number, minRatio: number): string {
  return `Contrast ${formatRatio(ratio)}:1 is below the WCAG AA minimum of ${formatRatio(minRatio)}:1`;
}

/**
 * Validate that text on a background meets the WCAG AA floor.
 * Returns a blocking error that names the measured ratio when it fails.
 */
export function validateTextOnBackground(
  textHex: string,
  backgroundHex: string,
  minRatio: number = MIN_AA_CONTRAST,
): ContrastValidation {
  const ratio = contrastRatio(textHex, backgroundHex);
  if (ratio >= minRatio) {
    return { ok: true, ratio };
  }
  return {
    ok: false,
    ratio,
    error: blockingError(ratio, minRatio),
  };
}

/** Primary brand fill: white body/label text must clear AA on the fill. */
export function validatePrimaryBrandColor(
  primaryHex: string,
  minRatio: number = MIN_AA_CONTRAST,
): ContrastValidation {
  return validateTextOnBackground(
    BRAND_ON_PRIMARY_TEXT,
    primaryHex,
    minRatio,
  );
}

/** Secondary brand colour: used as body/accent text on light surfaces. */
export function validateSecondaryBrandColor(
  secondaryHex: string,
  surfaceHex: string = BRAND_SECONDARY_SURFACE,
  minRatio: number = MIN_AA_CONTRAST,
): ContrastValidation {
  return validateTextOnBackground(secondaryHex, surfaceHex, minRatio);
}
