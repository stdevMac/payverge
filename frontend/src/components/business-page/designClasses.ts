type CornerRadius = "none" | "small" | "medium" | "large";
type ShadowIntensity = "none" | "subtle" | "medium" | "strong";
export type HeroLayout = "centered" | "split-left" | "split-right";
export type SectionDensity = "compact" | "comfortable";
type MenuLayout = "grid" | "list";
type HeaderStyle = "banner" | "minimal" | "classic";

export interface DesignSettings {
  primary_color: string;
  secondary_color: string;
  font_family: string;
  theme: "light" | "dark";
  menu_layout: MenuLayout;
  show_images: boolean;
  show_descriptions: boolean;
  header_style: HeaderStyle;
  corner_radius: CornerRadius;
  shadow_intensity: ShadowIntensity;
  background_pattern: string;
  pattern_opacity: number;
  hero_layout: HeroLayout;
  section_density: SectionDensity;
}

/* eslint-disable no-restricted-syntax -- API-stored design defaults; not Tailwind class names */
export const DEFAULT_DESIGN_SETTINGS: DesignSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  font_family: "Inter",
  theme: "light",
  menu_layout: "grid",
  show_images: true,
  show_descriptions: true,
  header_style: "banner",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  background_pattern: "none",
  pattern_opacity: 0.1,
  hero_layout: "centered",
  section_density: "comfortable",
};
/* eslint-enable no-restricted-syntax */

const RADIUS_MAP: Record<string, string> = {
  none: "rounded-none",
  small: "rounded-sm",
  medium: "rounded-lg",
  large: "rounded-xl",
};

const SHADOW_MAP: Record<string, string> = {
  none: "shadow-none",
  subtle: "shadow-sm",
  medium: "shadow-md",
  strong: "shadow-xl",
};

export function getRadiusClass(radius: string | undefined | null): string {
  if (!radius) return RADIUS_MAP.medium;
  return RADIUS_MAP[radius] ?? RADIUS_MAP.medium;
}

export function getShadowClass(shadow: string | undefined | null): string {
  if (!shadow) return SHADOW_MAP.subtle;
  return SHADOW_MAP[shadow] ?? SHADOW_MAP.subtle;
}

export function getSectionPadding(density: string | undefined | null): string {
  if (density === "compact") return "py-8 md:py-12";
  return "py-12 md:py-16";
}

// Menu container layout. "grid" (default) is the multi-column card grid; "list"
// is a single-column stack of full-width rows. Any unknown value falls back to
// grid so a stale/garbage design value never breaks the menu.
export function getMenuLayoutClass(layout: string | undefined | null): string {
  if (layout === "list") return "flex flex-col gap-4";
  return "grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6";
}

// Per-card modifier: in list mode the card lays its media and body side-by-side
// on md+; in grid mode the card keeps its default media-on-top stack.
export function getMenuCardClass(layout: string | undefined | null): string {
  return layout === "list" ? "md:flex md:items-stretch" : "";
}

// Media-block modifier: in list mode the media is a fixed-width left column on
// md+; in grid mode it stays full width (media-on-top).
export function getMenuMediaClass(layout: string | undefined | null): string {
  return layout === "list" ? "md:w-56 md:flex-shrink-0" : "";
}

export interface HeroTreatment {
  /** Render the banner image(s) as the full-bleed hero background. */
  showBanner: boolean;
  /** Render the banner (if any) as a slim top strip rather than full-bleed. */
  bannerAsStrip: boolean;
  /** Force the centered content arrangement regardless of hero_layout. */
  forceCentered: boolean;
  /** Section min-height classes for this treatment. */
  minHeightClass: string;
}

// header_style is the hero TREATMENT, composable with hero_layout (arrangement):
//  - banner (default): full-bleed banner background, tall hero, hero_layout applies.
//  - minimal: no banner background, compact band in the merchant primary color,
//    centered only.
//  - classic: lighter, reduced-height centered treatment; banner (if present)
//    renders as a slim top strip.
// Unknown values fall back to banner so a stale design value never breaks the hero.
export function getHeroTreatment(style: string | undefined | null): HeroTreatment {
  switch (style) {
    case "minimal":
      return {
        showBanner: false,
        bannerAsStrip: false,
        forceCentered: true,
        minHeightClass: "min-h-[40vh] md:min-h-[44vh]",
      };
    case "classic":
      return {
        showBanner: false,
        bannerAsStrip: true,
        forceCentered: true,
        minHeightClass: "min-h-[48vh] md:min-h-[56vh]",
      };
    case "banner":
    default:
      return {
        showBanner: true,
        bannerAsStrip: false,
        forceCentered: false,
        minHeightClass: "min-h-[56vh] md:min-h-[68vh]",
      };
  }
}
