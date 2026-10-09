import type { TemplateStyle } from "../templates/types";

export type KitId =
  | "editorial"
  | "bold"
  | "minimal"
  | "chalkboard"
  | "linen"
  | "ticket";

export type MotifId =
  | "rule"
  | "cornerBrackets"
  | "seal"
  | "ticketNotch"
  | "halftone"
  | "grain"
  | "tape";

export type TextureId = "none" | "grain" | "paper" | "halftone";

interface TypePairing {
  /** Display face for the headline slot. */
  display: "serif" | "sans";
  /** Body face for supporting slots. */
  body: "serif" | "sans";
  /** Multiplier applied to a composition's base type scale. */
  scale: number;
  /** Letter-spacing for uppercase slots, as a fraction of font size. */
  tracking: number;
}

interface GradePreset {
  /** Canvas filter string applied to the photo. Wave 2 uses it; Wave 1 stores it. */
  filter: string;
}

interface ScrimStyle {
  from: string;
  to: string;
  adaptive: boolean;
  luminanceThreshold: number;
}

export interface CreativeKit {
  id: KitId;
  labelKey: string;
  typePairing: TypePairing;
  gradePreset: GradePreset;
  texture: TextureId;
  motifSet: MotifId[];
  scrimStyle: ScrimStyle;
  /** Prefer a light surface behind content instead of the photo. */
  prefersLightSurface: boolean;
}

const DARK_SCRIM: ScrimStyle = {
  from: "rgba(28,25,23,0)",
  to: "rgba(28,25,23,0.78)",
  adaptive: true,
  luminanceThreshold: 155,
};

const LIGHT_SCRIM: ScrimStyle = {
  from: "rgba(250,249,246,0)",
  to: "rgba(250,249,246,0.94)",
  adaptive: false,
  luminanceThreshold: 155,
};

export const KITS: Record<KitId, CreativeKit> = {
  editorial: {
    id: "editorial",
    labelKey: "kits.editorial",
    typePairing: { display: "serif", body: "sans", scale: 1, tracking: 0.04 },
    gradePreset: { filter: "saturate(1.04) contrast(1.03)" },
    texture: "none",
    motifSet: ["rule"],
    scrimStyle: DARK_SCRIM,
    prefersLightSurface: false,
  },
  bold: {
    id: "bold",
    labelKey: "kits.bold",
    typePairing: { display: "sans", body: "sans", scale: 1.15, tracking: 0.06 },
    gradePreset: { filter: "saturate(1.18) contrast(1.12)" },
    texture: "none",
    motifSet: ["rule", "cornerBrackets"],
    scrimStyle: DARK_SCRIM,
    prefersLightSurface: false,
  },
  minimal: {
    id: "minimal",
    labelKey: "kits.minimal",
    typePairing: { display: "sans", body: "sans", scale: 0.92, tracking: 0.02 },
    gradePreset: { filter: "saturate(0.96)" },
    texture: "none",
    motifSet: ["rule"],
    scrimStyle: LIGHT_SCRIM,
    prefersLightSurface: true,
  },
  chalkboard: {
    id: "chalkboard",
    labelKey: "kits.chalkboard",
    typePairing: { display: "serif", body: "sans", scale: 1.05, tracking: 0.08 },
    gradePreset: { filter: "saturate(0.88) contrast(1.1) brightness(0.94)" },
    texture: "grain",
    motifSet: ["rule", "cornerBrackets", "seal"],
    scrimStyle: { ...DARK_SCRIM, to: "rgba(20,18,17,0.88)" },
    prefersLightSurface: false,
  },
  linen: {
    id: "linen",
    labelKey: "kits.linen",
    typePairing: { display: "serif", body: "serif", scale: 0.98, tracking: 0.03 },
    gradePreset: { filter: "saturate(0.94) brightness(1.04)" },
    texture: "paper",
    motifSet: ["rule", "seal"],
    scrimStyle: LIGHT_SCRIM,
    prefersLightSurface: true,
  },
  ticket: {
    id: "ticket",
    labelKey: "kits.ticket",
    typePairing: { display: "sans", body: "sans", scale: 1.08, tracking: 0.1 },
    gradePreset: { filter: "saturate(1.1) contrast(1.06)" },
    texture: "halftone",
    motifSet: ["ticketNotch", "rule", "tape"],
    scrimStyle: DARK_SCRIM,
    prefersLightSurface: false,
  },
};

export const KIT_ORDER: KitId[] = [
  "editorial",
  "bold",
  "minimal",
  "chalkboard",
  "linen",
  "ticket",
];

/**
 * Narrow a value of unknown provenance — a `creative_snapshot.kit` off the
 * wire, a URL parameter — to a `KitId` that is guaranteed to have a definition
 * in `KITS`. Exact match only: no trimming, no case folding, because the
 * backend whitelist in marketing_activity_handlers.go stores the canonical
 * lowercase id and anything else is a value this build cannot honour.
 *
 * Use this instead of `as KitId`. The cast turns a server-side typo into an
 * undefined registry lookup and a post that renders blank; the guard turns it
 * into a fall back to the legacy derivation.
 */
export function isKitId(value: unknown): value is KitId {
  return typeof value === "string" && (KIT_ORDER as string[]).includes(value);
}

/**
 * Legacy `creative_snapshot.template` values share names with the first three
 * kits, so this is an identity map with a safe default. Keeping the names
 * aligned removes a translation table that could drift from the backend
 * whitelist in marketing_activity_handlers.go.
 */
export function kitForLegacyTemplate(template: TemplateStyle): KitId {
  return (KIT_ORDER as string[]).includes(template)
    ? (template as KitId)
    : "editorial";
}
