/**
 * Brand lock helpers for the Marketing tab (Season 1 — Craft / S1-Brand).
 *
 * Pure and leaf-friendly: no React, no canvas. Callers seed composers, cards,
 * and settings previews from these so logo, palette, kit bias, CTA, and the
 * wire `font_family` agree on one decision tree.
 *
 * ## font_family vs kit typefaces — decision
 *
 * **Retired for the scene pipeline (honest).** `design_settings.font_family`
 * still drives the public business site and still steers AI image mood
 * derivation on the backend, but it is NOT an input to kit typefaces or
 * `buildScene`. Wave 1 made faces art-direction-owned (`CreativeKit.typePairing`);
 * honouring brand Inter/Sans/Serif as a kill-switch would make serif kits
 * unreachable for the default population.
 *
 * Snapshots still carry `font_family` for wire compatibility. We **derive** it
 * from the kit's display face so the stored value matches what was painted,
 * rather than echoing an inert design setting.
 */

import type {
  MarketingCTAStyle,
  MarketingRenderFontFamily,
  MarketingVisualMood,
} from "@/api/marketing";
import type { Business } from "@/api/business";
import {
  isKitId,
  kitForLegacyTemplate,
  KITS,
  type KitId,
} from "./artDirection/kits";
import type { TemplateStyle } from "./templates/types";
import { logoUrlFromBusiness, normalizeHandle } from "./postContent";

/** Canvas palette hex fallbacks (Tailwind tokens do not paint on canvas). */
/* eslint-disable no-restricted-syntax -- canvas paint strings */
const FALLBACK_PRIMARY = "#1a6b6a";
const FALLBACK_SECONDARY = "#0f3d3c";
/* eslint-enable no-restricted-syntax */

export interface BrandPalette {
  primary: string;
  secondary: string;
  /**
   * Wire snapshot font derived from kit display face (or Sans when no kit).
   * Not a face override for the renderer — kits own typefaces.
   */
  fontFamily: MarketingRenderFontFamily;
}

/**
 * Map a kit's display face to the snapshot/wire `font_family` vocabulary.
 *
 * Serif display kits → Serif; everything else → Sans. "Inter" remains a valid
 * wire value for legacy rows but is no longer emitted for new seeds (it was
 * the pre-rewrite serif kill-switch default, not a real brand face choice).
 */
export function fontFamilyFromKit(kit: KitId): MarketingRenderFontFamily {
  return KITS[kit].typePairing.display === "serif" ? "Serif" : "Sans";
}

/**
 * Resolve the wire font for a creative: stored kit wins, then recognised
 * stored font, else Sans. Never reads `design_settings.font_family`.
 */
export function resolveMarketingFontFamily(input: {
  kit?: string | null;
  storedFontFamily?: string | null;
}): MarketingRenderFontFamily {
  if (isKitId(input.kit)) return fontFamilyFromKit(input.kit);
  const raw = input.storedFontFamily?.trim().toLowerCase();
  if (raw === "serif") return "Serif";
  if (raw === "sans" || raw === "inter") return "Sans";
  return "Sans";
}

/**
 * Brand colours always come from business design settings. Font on the palette
 * is kit-derived honesty for snapshots / handoffs, not a face override.
 */
export function resolveBrandPalette(
  business: Pick<Business, "design_settings">,
  opts?: { kit?: string | null; storedFontFamily?: string | null },
): BrandPalette {
  return {
    primary: business.design_settings?.primary_color?.trim() || FALLBACK_PRIMARY,
    secondary:
      business.design_settings?.secondary_color?.trim() || FALLBACK_SECONDARY,
    fontFamily: resolveMarketingFontFamily({
      kit: opts?.kit,
      storedFontFamily: opts?.storedFontFamily,
    }),
  };
}

/**
 * Default kit bias from creative-profile visual mood.
 *
 * Returns null when mood is blank or "natural" so play-level defaults
 * (`defaultTemplateForPlay` → `kitForLegacyTemplate`) still apply. Non-empty
 * directional moods bias the opening kit so settings feel chosen.
 */
export function kitBiasFromVisualMood(
  mood?: MarketingVisualMood | string | null,
): KitId | null {
  switch ((mood ?? "").trim().toLowerCase()) {
    case "bright":
      return "bold";
    case "moody":
      return "chalkboard";
    case "editorial":
      return "editorial";
    case "rustic":
      return "linen";
    case "natural":
      // Explicit natural = no bias; play template still picks the kit.
      return null;
    default:
      return null;
  }
}

/**
 * Resolve the kit a creative opens with.
 *
 * Precedence: stored (recognised) kit → mood bias → legacy template map.
 * Reuse/Tweak preserve the stored kit; brand lock only re-applies logo/colors.
 */
export function seedKitForCreative(input: {
  storedKit?: string | null;
  visualMood?: MarketingVisualMood | string | null;
  templateStyle: TemplateStyle;
}): KitId {
  if (isKitId(input.storedKit)) return input.storedKit;
  const bias = kitBiasFromVisualMood(input.visualMood);
  if (bias) return bias;
  return kitForLegacyTemplate(input.templateStyle);
}

/** Current business logo URL for the canvas watermark (empty when unset). */
export function brandLogoUrl(
  business: Pick<Business, "logo">,
): string {
  return logoUrlFromBusiness(business as Business);
}

/** Canonical @handle for slots and captions; empty when none is valid. */
export function brandHandle(
  business: Pick<Business, "social_media">,
): string {
  return normalizeHandle(business.social_media);
}

/**
 * CTA canvas label: profile `cta_style` wins when set, else play-specific default.
 *
 * Style keys live under `defaults.ctaStyle.{soft|direct|urgent}`. Callers pass
 * the same `t` used for play CTAs so missing keys fall through cleanly.
 */
export function ctaLabelForBrand(
  t: (key: string) => string,
  play: string | undefined,
  ctaStyle?: MarketingCTAStyle | string | null,
): string {
  const style = (ctaStyle ?? "").trim().toLowerCase();
  if (style === "soft" || style === "direct" || style === "urgent") {
    const key = `defaults.ctaStyle.${style}`;
    const label = t(key);
    if (label && !label.startsWith("defaults.ctaStyle.")) return label;
  }
  if (play) {
    const specific = t(`defaults.cta.${play}`);
    if (specific && !specific.startsWith("defaults.cta.")) return specific;
  }
  return t("defaults.cta.default");
}

/**
 * Whether starter / fallback captions should force zero hashtags client-side.
 * AI captions still honour profile hashtag_behavior server-side; this is for
 * template strings that might accrue tags in future locales.
 */
export function shouldSuppressHashtags(
  hashtagBehavior?: string | null,
): boolean {
  return (hashtagBehavior ?? "").trim().toLowerCase() === "none";
}

/** Strip #tags from a caption when profile hashtag_behavior is none. */
export function applyHashtagPreference(
  caption: string,
  hashtagBehavior?: string | null,
): string {
  if (!shouldSuppressHashtags(hashtagBehavior)) return caption;
  return caption
    .replace(/(^|\s)#[\p{L}\p{N}_]+/gu, " ")
    .replace(/\s+/g, " ")
    .trim();
}

export interface BrandLockSummary {
  logoUrl: string;
  primaryColor: string;
  secondaryColor: string;
  /** Kit the mood would open with, or null when play defaults apply. */
  moodKit: KitId | null;
  fontNote: "kit_owned";
  handle: string;
  ctaStyle: MarketingCTAStyle | "";
  hashtagBehavior: string;
  visualMood: string;
}

/** Read-only brand lock preview for settings / diagnostics. */
export function brandLockSummary(
  business: Pick<
    Business,
    "logo" | "design_settings" | "social_media"
  >,
  profile?: {
    visual_mood?: string;
    cta_style?: string;
    hashtag_behavior?: string;
  } | null,
): BrandLockSummary {
  const visualMood = profile?.visual_mood?.trim() ?? "";
  const ctaRaw = profile?.cta_style?.trim() ?? "";
  const ctaStyle =
    ctaRaw === "soft" || ctaRaw === "direct" || ctaRaw === "urgent"
      ? ctaRaw
      : "";
  return {
    logoUrl: brandLogoUrl(business),
    primaryColor:
      business.design_settings?.primary_color?.trim() || FALLBACK_PRIMARY,
    secondaryColor:
      business.design_settings?.secondary_color?.trim() || FALLBACK_SECONDARY,
    moodKit: kitBiasFromVisualMood(visualMood),
    fontNote: "kit_owned",
    handle: brandHandle(business),
    ctaStyle,
    hashtagBehavior: profile?.hashtag_behavior?.trim() ?? "",
    visualMood,
  };
}

export type LogoLoadStatus = "idle" | "missing" | "loading" | "ok" | "failed";
