// Restaurant menu creation — shared art direction, planning, and render types.
// Pure: no DOM and no side effects.

import type { PrintFontFamilyName } from "./fonts";

/** Supported paper sizes for physical menu print. */
export type PaperFormat = "letter" | "a4" | "half-letter" | "a5";

/** Page dimensions in millimetres (width × height, portrait). */
export const PAPER_DIMENSIONS_MM: Record<
  PaperFormat,
  { widthMm: number; heightMm: number }
> = {
  letter: { widthMm: 215.9, heightMm: 279.4 },
  a4: { widthMm: 210, heightMm: 297 },
  "half-letter": { widthMm: 139.7, heightMm: 215.9 },
  a5: { widthMm: 148, heightMm: 210 },
};

interface Margins {
  top: number;
  right: number;
  bottom: number;
  left: number;
}

export const MENU_DESIGN_FAMILY_IDS = [
  "atelier",
  "maison",
  "osteria",
  "night-house",
  "counter",
  "street",
  "field",
  "gallery",
] as const;

export type MenuDesignFamilyId = (typeof MENU_DESIGN_FAMILY_IDS)[number];

export const MENU_TREATMENTS = [
  "type-led",
  "balanced",
  "photo-led",
  "compact",
] as const;

export type MenuTreatment = (typeof MENU_TREATMENTS)[number];

export const MENU_OUTPUT_FORMATS = [
  "single-sheet",
  "two-page-spread",
  "folded-booklet",
  "takeaway-trifold",
  "drinks-card",
  "counter-menu",
] as const;

export type MenuOutputFormat = (typeof MENU_OUTPUT_FORMATS)[number];

export type LogoTreatment = "wordmark" | "contained" | "mark-only" | "hidden";
export type CoverMode = "none" | "typographic" | "photographic";
export type OrnamentIntensity = "none" | "restrained" | "expressive";
export const PRINT_TYPOGRAPHY_PERSONALITIES = [
  "editorial",
  "refined",
  "classic",
  "warm",
  "dramatic",
  "bold",
  "modern",
  "graphic",
  "natural",
] as const;
export type PrintTypographyPersonality =
  (typeof PRINT_TYPOGRAPHY_PERSONALITIES)[number];
export const PRINT_CONTACT_PLACEMENTS = ["none", "footer", "cover"] as const;
export type PrintContactPlacement = (typeof PRINT_CONTACT_PLACEMENTS)[number];
export const PRINT_TYPOGRAPHY_BY_FAMILY: Readonly<
  Record<MenuDesignFamilyId, readonly PrintTypographyPersonality[]>
> = {
  atelier: ["editorial", "refined"],
  maison: ["classic", "refined"],
  osteria: ["warm", "editorial"],
  "night-house": ["dramatic", "refined"],
  counter: ["bold", "modern"],
  street: ["bold", "graphic"],
  field: ["natural", "editorial"],
  gallery: ["editorial", "refined"],
};
export type DiagnosticSeverity = "ready" | "warning" | "blocked";

export type ImageRole =
  | "cover"
  | "full-width"
  | "half-page"
  | "editorial-crop"
  | "category-opener"
  | "paired"
  | "compact-tile";

export type CompositionStrategyId =
  | "editorial-asymmetric"
  | "formal-course"
  | "lively-columns"
  | "cinematic-sections"
  | "modular-counter"
  | "bold-scan"
  | "ingredient-air"
  | "image-gallery";

/** Four print color roles; this shape alone does not certify text contrast. */
export interface PrintPalette {
  ground: string;
  ink: string;
  accent: string;
  muted: string;
}

export interface MenuDesignFamily {
  id: MenuDesignFamilyId;
  nameKey: string;
  descriptionKey: string;
  supportedTreatments: readonly MenuTreatment[];
  supportedFormats: readonly MenuOutputFormat[];
  typography: {
    display: PrintFontFamilyName;
    body: PrintFontFamilyName;
    displayScale: number;
    bodyFloorPt: number;
    align: "start" | "center";
  };
  /** Art-direction fallback; its accent may be decorative and requires text-safe resolution. */
  paletteFallback: PrintPalette;
  compositions: readonly CompositionStrategyId[];
  photography: {
    roles: readonly ImageRole[];
    maxFeatureImagesPerPage: number;
  };
  ornament: "none" | "rule" | "double-rule" | "frame" | "block";
}

/** Defensively frozen family data returned by the built-in registry. */
export type RegisteredMenuDesignFamily = Readonly<
  Omit<
    MenuDesignFamily,
    | "supportedTreatments"
    | "supportedFormats"
    | "typography"
    | "paletteFallback"
    | "compositions"
    | "photography"
  > & {
    supportedTreatments: readonly MenuTreatment[];
    supportedFormats: readonly MenuOutputFormat[];
    typography: Readonly<MenuDesignFamily["typography"]>;
    paletteFallback: Readonly<PrintPalette>;
    compositions: readonly CompositionStrategyId[];
    photography: Readonly<{
      roles: readonly ImageRole[];
      maxFeatureImagesPerPage: number;
    }>;
  }
>;

/** Contrast-checked palette produced by the text-safe resolution path. */
export interface ResolvedPrintPalette extends PrintPalette {
  source: "restaurant" | "family-fallback";
}

export interface MenuDirectionRecommendation {
  familyId: MenuDesignFamilyId;
  treatment: MenuTreatment;
  outputFormat: MenuOutputFormat;
  reasonKey: string;
  score: number;
}

export interface ImageFocalPoint {
  x: number;
  y: number;
}

export interface FeaturedImageSelection {
  itemId: string;
  url: string;
  role: ImageRole;
}

export interface MenuArtDirection {
  familyId: MenuDesignFamilyId;
  treatment: MenuTreatment;
  outputFormat: MenuOutputFormat;
  paperFormat: PaperFormat;
  palette: ResolvedPrintPalette;
  imageSelections: FeaturedImageSelection[];
  imageFocalPoints: Record<string, ImageFocalPoint>;
  logoTreatment: LogoTreatment;
  coverMode: CoverMode;
  ornamentIntensity: OrnamentIntensity;
  typographyPersonality: PrintTypographyPersonality;
  contactPlacement: PrintContactPlacement;
}

export interface PrintGeometry {
  paperFormat: PaperFormat;
  outputFormat: MenuOutputFormat;
  orientation: "portrait" | "landscape";
  widthMm: number;
  heightMm: number;
  safeArea: Margins;
  trimGuideInsetMm: number;
  foldsMm: number[];
  minimumPages: number;
  panelCount: number;
}

export interface PlannedRect {
  xMm: number;
  yMm: number;
  widthMm: number;
  heightMm: number;
}

type PlannedBlockKind =
  | "masthead"
  | "cover"
  | "category-heading"
  | "item-list"
  | "item-feature"
  | "image"
  | "panel-note"
  | "footer"
  | "folio";

export interface PlannedMenuBlock {
  id: string;
  kind: PlannedBlockKind;
  rect: PlannedRect;
  sectionId?: string;
  itemIds?: string[];
  image?: FeaturedImageSelection;
  continuation?: boolean;
}

export interface PlannedMenuPage {
  index: number;
  blocks: PlannedMenuBlock[];
}

export interface MenuPrintDiagnostic {
  id: string;
  severity: DiagnosticSeverity;
  messageKey: string;
  target: {
    kind: "image" | "category" | "page" | "setting";
    id: string;
  };
  values?: Record<string, string | number>;
}

export interface PlannedMenuDocument {
  direction: MenuArtDirection;
  geometry: PrintGeometry;
  pages: PlannedMenuPage[];
  diagnostics: MenuPrintDiagnostic[];
  readiness: DiagnosticSeverity;
}

/** Locale-aware price presentation used by planned menu renderers. */
export interface MenuPriceFormatSpec {
  mode: "nested" | "column";
  leaders: boolean;
  symbol: "off" | "locale";
  decimals: "strip-zeros" | "always";
}

/**
 * Per-family price presentation. Formal families use bare numerals
 * (menu-industry convention); casual scan-first families keep the
 * locale currency symbol. Nested mode stacks the price under the copy.
 */
export const PRINT_PRICE_TREATMENTS: Readonly<
  Record<MenuDesignFamilyId, MenuPriceFormatSpec>
> = {
  atelier: {
    mode: "column",
    leaders: true,
    symbol: "off",
    decimals: "strip-zeros",
  },
  maison: {
    mode: "nested",
    leaders: false,
    symbol: "off",
    decimals: "strip-zeros",
  },
  osteria: {
    mode: "column",
    leaders: true,
    symbol: "locale",
    decimals: "strip-zeros",
  },
  "night-house": {
    mode: "column",
    leaders: false,
    symbol: "off",
    decimals: "strip-zeros",
  },
  counter: {
    mode: "column",
    leaders: false,
    symbol: "locale",
    decimals: "strip-zeros",
  },
  street: {
    mode: "column",
    leaders: false,
    symbol: "locale",
    decimals: "strip-zeros",
  },
  field: {
    mode: "column",
    leaders: false,
    symbol: "off",
    decimals: "strip-zeros",
  },
  gallery: {
    mode: "column",
    leaders: false,
    symbol: "off",
    decimals: "strip-zeros",
  },
};

/** Source-content choices used to normalize a menu before planning. */
export interface MenuPrintOptions {
  paperFormat: PaperFormat;
  language: string;
  showDescriptions: boolean;
  showImages: boolean;
  showTags: boolean;
  /** When true, format prices with locale currency symbol. Default false (Cornell study). */
  currencySymbol?: boolean;
  /** Optional restaurant-specific accent override. */
  accentColor?: string;
  includeQr: boolean;
  qrDataUrl?: string;
  /**
   * Pre-localized QR footer caption (caller owns i18n). Default empty.
   * Only shown when includeQr && qrDataUrl.
   */
  qrCaption?: string;
  selectedCategoryIds: string[] | "all";
  /** Include items with is_available === false. Default false. */
  includeUnavailable?: boolean;
}

type PrintWarningType =
  | "too-many-items"
  | "long-description"
  | "missing-descriptions"
  | "missing-photos";

export interface PrintWarning {
  type: PrintWarningType;
  sectionName: string;
  count?: number;
}

export interface PrintMenuItem {
  id: string;
  name: string;
  description?: string;
  /** Dollars (frontend wire shape — backend already converts cents→dollars). */
  price: number;
  imageUrl?: string;
  imageCandidates?: string[];
  dietaryTags: string[];
  allergens: string[];
}

export interface PrintMenuSection {
  id: string;
  name: string;
  description?: string;
  items: PrintMenuItem[];
}

/** Normalized render model consumed by renderHtml. */
export interface PrintMenuModel {
  business: {
    name: string;
    logoUrl?: string;
    tagline?: string;
    address?: string;
    customUrl?: string;
    businessType?: string;
    primaryColor?: string;
    secondaryColor?: string;
  };
  sections: PrintMenuSection[];
  /** ISO 4217 currency code. */
  currency: string;
  language: string;
  warnings: PrintWarning[];
}
