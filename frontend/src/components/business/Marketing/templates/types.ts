export type MarketingAspectRatio = "1:1" | "4:5" | "9:16";
export type AspectRatio = MarketingAspectRatio;

export const ASPECT_DIMS: Record<AspectRatio, { w: number; h: number }> = {
  "1:1": { w: 1080, h: 1080 },
  "4:5": { w: 1080, h: 1350 },
  "9:16": { w: 1080, h: 1920 },
};

export interface NormalizedRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Shared platform-safe geometry used by both export layouts and preview bands. */
export const PLATFORM_CONTENT_BOUNDS: Record<AspectRatio, NormalizedRect> = {
  "1:1": { x: 0.06, y: 0.06, w: 0.88, h: 0.88 },
  "4:5": { x: 0.06, y: 0.07, w: 0.88, h: 0.86 },
  "9:16": { x: 0.06, y: 0.12, w: 0.88, h: 0.7 },
};

export type TemplateStyle = "editorial" | "bold" | "minimal";

export type SlotKey = "dishName" | "price" | "badge" | "cta" | "handle";

export type SlotFont = "serif" | "sans";
export type SlotAlign = "left" | "center" | "right";
/** Color tokens resolved at render time against the business palette. */
type SlotColor = "onPhoto" | "onPhotoMuted" | "primary" | "secondary" | "ink";

export interface SlotDef {
  /** Normalized slot rectangle; rendering and safe-zone checks share it. */
  rect: NormalizedRect;
  font: SlotFont;
  /** Font weight (DM Sans supports 400-700; serif is 400 only). */
  weight: 400 | 500 | 600 | 700;
  /** Font size as a fraction of canvas WIDTH. */
  sizePct: number;
  align: SlotAlign;
  color: SlotColor;
  maxLines: number;
  lineHeight: number;
  uppercase?: boolean;
  /** Optional rounded pill behind the text. Padding is canvas-width based. */
  pill?: { color: SlotColor; padX: number; padY: number };
}

export interface LogoAnchor {
  /** Normalized top-left position and width-relative square size. */
  x: number;
  y: number;
  size: number;
}

export interface ScrimTreatment {
  region: NormalizedRect;
  from: string;
  to: string;
  adaptive: boolean;
  luminanceThreshold: number;
}

export interface TemplateLayout {
  contentBounds: NormalizedRect;
  imageArea: NormalizedRect;
  logoAnchor: LogoAnchor;
  slots: Partial<Record<SlotKey, SlotDef>>;
  scrim: ScrimTreatment;
}

export interface TemplateDef {
  id: TemplateStyle;
  labelKey: string;
  layouts: Record<MarketingAspectRatio, TemplateLayout>;
}
