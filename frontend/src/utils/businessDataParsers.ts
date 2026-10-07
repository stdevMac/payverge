import type { BusinessDesignSettings, MenuCategory } from "@/api/business";

export interface SocialMedia {
  instagram?: string;
  facebook?: string;
  twitter?: string;
  linkedin?: string;
  youtube?: string;
}

/* eslint-disable no-restricted-syntax -- user-customizable design settings; hex values are API/DB contracts, not Tailwind classes */
const DEFAULT_DESIGN_SETTINGS: BusinessDesignSettings = {
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

export function parseSocialMedia(raw: unknown): SocialMedia {
  if (!raw) return {};
  try {
    const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
    if (typeof parsed !== "object" || parsed === null) return {};
    return parsed as SocialMedia;
  } catch {
    return {};
  }
}

export function parseBannerImages(raw: unknown): string[] {
  if (!raw) return [];
  try {
    const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function parseDesignSettings(raw: unknown): BusinessDesignSettings {
  if (!raw) return { ...DEFAULT_DESIGN_SETTINGS };
  try {
    const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
    if (typeof parsed !== "object" || parsed === null) return { ...DEFAULT_DESIGN_SETTINGS };
    return {
      primary_color: parsed.primary_color || DEFAULT_DESIGN_SETTINGS.primary_color,
      secondary_color: parsed.secondary_color || DEFAULT_DESIGN_SETTINGS.secondary_color,
      font_family: parsed.font_family || DEFAULT_DESIGN_SETTINGS.font_family,
      theme: parsed.theme || DEFAULT_DESIGN_SETTINGS.theme,
      menu_layout: parsed.menu_layout || DEFAULT_DESIGN_SETTINGS.menu_layout,
      show_images: parsed.show_images !== undefined ? parsed.show_images : true,
      show_descriptions: parsed.show_descriptions !== undefined ? parsed.show_descriptions : true,
      header_style: parsed.header_style || DEFAULT_DESIGN_SETTINGS.header_style,
      corner_radius: parsed.corner_radius || DEFAULT_DESIGN_SETTINGS.corner_radius,
      shadow_intensity: parsed.shadow_intensity || DEFAULT_DESIGN_SETTINGS.shadow_intensity,
      background_pattern: parsed.background_pattern || DEFAULT_DESIGN_SETTINGS.background_pattern,
      pattern_opacity: parsed.pattern_opacity !== undefined ? parsed.pattern_opacity : DEFAULT_DESIGN_SETTINGS.pattern_opacity,
      hero_layout: parsed.hero_layout || DEFAULT_DESIGN_SETTINGS.hero_layout,
      section_density: parsed.section_density || DEFAULT_DESIGN_SETTINGS.section_density,
    };
  } catch {
    return { ...DEFAULT_DESIGN_SETTINGS };
  }
}

/**
 * Decode the menu-categories field from a menu API response into a
 * MenuCategory[]. Consolidates 7 divergent copy-paste decoders (Q-1).
 *
 * Resolution order, matching the most-complete prior copies
 * (InventoryManager / BusinessOverview / useMenuData):
 *   1. parsed_categories — the translated copy when a language was requested;
 *      always preferred when it is a real array.
 *   2. categories as an already-parsed array.
 *   3. categories as a JSON string → JSON.parse → array (or [] on bad JSON).
 * Anything else yields []. Never throws.
 */
export function parseMenuCategories(menuResponse: {
  categories?: MenuCategory[] | string | null;
  parsed_categories?: MenuCategory[];
} | null | undefined): MenuCategory[] {
  if (!menuResponse) return [];

  if (Array.isArray(menuResponse.parsed_categories)) {
    return menuResponse.parsed_categories;
  }

  const { categories } = menuResponse;
  if (Array.isArray(categories)) {
    return categories;
  }
  if (typeof categories === "string") {
    try {
      const parsed = JSON.parse(categories);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }
  return [];
}
