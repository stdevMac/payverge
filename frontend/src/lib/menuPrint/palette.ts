/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import type {
  MenuDesignFamilyId,
  PrintPalette,
  ResolvedPrintPalette,
} from "./types";

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/;
const MIN_TEXT_CONTRAST = 4.5;

const FAMILY_FALLBACKS: Record<MenuDesignFamilyId, PrintPalette> = {
  atelier: {
    ground: "#faf7f0",
    ink: "#172c2a",
    accent: "#145c55",
    muted: "#5d5145",
  },
  maison: {
    ground: "#fbf7ed",
    ink: "#2c211b",
    accent: "#713f3b",
    muted: "#65584d",
  },
  osteria: {
    ground: "#fcf4e6",
    ink: "#30261d",
    accent: "#7a3028",
    muted: "#5f5548",
  },
  "night-house": {
    ground: "#171717",
    ink: "#f7f0dd",
    accent: "#d0aa57",
    muted: "#c7bda9",
  },
  counter: {
    ground: "#fffaf0",
    ink: "#272019",
    accent: "#8a3b22",
    muted: "#67594b",
  },
  street: {
    ground: "#fff7e8",
    ink: "#251b16",
    accent: "#9a301f",
    muted: "#665047",
  },
  field: {
    ground: "#f5f4e9",
    ink: "#263229",
    accent: "#3e6547",
    muted: "#566459",
  },
  gallery: {
    ground: "#fcfbf8",
    ink: "#1d1d1d",
    accent: "#3e4f69",
    muted: "#5d5d5d",
  },
};

export interface ResolvePrintPaletteInput {
  familyId: MenuDesignFamilyId;
  primaryColor?: string | null;
  secondaryColor?: string | null;
}

function normalizeHex(color: string): string {
  if (!HEX_COLOR.test(color)) {
    throw new TypeError(
      `Expected a full six-digit hex color, received ${color}`,
    );
  }
  return color.toLowerCase();
}

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

/** Return the WCAG contrast ratio between two strict #RRGGBB colors. */
export function contrastRatio(first: string, second: string): number {
  const firstLuminance = relativeLuminance(first);
  const secondLuminance = relativeLuminance(second);
  const lighter = Math.max(firstLuminance, secondLuminance);
  const darker = Math.min(firstLuminance, secondLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

/** Validate that every text-bearing palette token clears 4.5:1 on its ground. */
export function isPrintSafePalette(palette: PrintPalette): boolean {
  try {
    return [palette.ink, palette.accent, palette.muted].every(
      (color) => contrastRatio(color, palette.ground) >= MIN_TEXT_CONTRAST,
    );
  } catch {
    return false;
  }
}

function familyFallback(familyId: MenuDesignFamilyId): ResolvedPrintPalette {
  return { ...FAMILY_FALLBACKS[familyId], source: "family-fallback" };
}

/**
 * Preserve a restaurant's primary color when it remains legible on the chosen
 * family's paper ground. Unsafe optional secondary colors never become text.
 */
export function resolvePrintPalette(
  input: ResolvePrintPaletteInput,
): ResolvedPrintPalette {
  const fallback = FAMILY_FALLBACKS[input.familyId];
  if (!input.primaryColor) {
    return familyFallback(input.familyId);
  }

  let primary: string;
  try {
    primary = normalizeHex(input.primaryColor);
  } catch {
    return familyFallback(input.familyId);
  }

  let secondary: string | undefined;
  if (input.secondaryColor) {
    try {
      secondary = normalizeHex(input.secondaryColor);
    } catch {
      secondary = undefined;
    }
  }

  if (contrastRatio(primary, fallback.ground) < MIN_TEXT_CONTRAST) {
    return familyFallback(input.familyId);
  }

  const palette: ResolvedPrintPalette = {
    ground: fallback.ground,
    ink: fallback.ink,
    accent: primary,
    muted:
      secondary &&
      contrastRatio(secondary, fallback.ground) >= MIN_TEXT_CONTRAST
        ? secondary
        : fallback.muted,
    source: "restaurant",
  };

  return isPrintSafePalette(palette) ? palette : familyFallback(input.familyId);
}
