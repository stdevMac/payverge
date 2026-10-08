/**
 * Print-shop export presets (S3-Reach / S3-E).
 *
 * Parameter packs on top of the existing FormatDef registry — no new FormatId
 * entries unless product later requires physical trim geometry that cannot be
 * expressed as a high-dpi PNG of an existing canvas.
 *
 * - table_tent_5x7 → Wave 4 print walker HTML @ 300 dpi with bleed + crop marks
 * - window_clings_square → 1:1 high-dpi PNG (2× native = 2160px) for shops
 * - flyer_half_letter → wide high-dpi PNG; shop scales to half-letter paper
 *
 * Thermal receipts stay out of scope (backend ESC/POS formatters).
 * Pure data + pure helpers. No React. No schedule fields.
 */

import type { DestinationId } from "../destinations/destinations";
import {
  FORMATS,
  isFormatId,
  type FormatDef,
  type FormatId,
  type PrintSpec,
} from "./formats";
import { pageBoxMm } from "./printGeometry";

export type PrintPresetId =
  | "table_tent_5x7"
  | "window_clings_square"
  | "flyer_half_letter";

/** How the asset is produced for the shop zip. */
type PrintPresetExportKind = "print_html" | "high_dpi_png";

export interface PrintPresetDef {
  id: PrintPresetId;
  /** i18n: printShop.presets.<id>.name */
  labelKey: string;
  /** i18n: printShop.presets.<id>.hint */
  hintKey: string;
  /** Existing FormatId — never invent ids here. */
  formatId: FormatId;
  exportKind: PrintPresetExportKind;
  /**
   * Raster multiplier for high_dpi_png (native format.px × scale).
   * Clamped 1–2 in export. Ignored for print_html (uses FormatDef.print.dpi).
   */
  rasterScale: number;
  /**
   * Destination packs that default to this preset when exporting
   * "print shop pack".
   */
  destinationIds: readonly DestinationId[];
  /**
   * English default print-instructions body (FE may pass translated override).
   * Must never mention scheduling.
   */
  defaultInstructions: string;
}

const TABLE_TENT_INSTRUCTIONS = [
  "Print shop instructions — table tent (5 × 7 in)",
  "============================================",
  "",
  "File: HTML with @page size, bleed, and crop marks.",
  "Open the HTML in Chrome/Safari/Edge and use Print → Save as PDF,",
  "or send the HTML/PDF to your print shop.",
  "",
  "Trim size: 127 mm × 177.8 mm (5 × 7 in).",
  "Bleed: 3.175 mm (1/8 in) on every side.",
  "Crop marks: outside the bleed; do not crop marks into live art.",
  "Resolution: authored at 300 dpi.",
  "",
  "Not a thermal receipt. Kitchen/POS receipt branding is separate.",
  "You place the tent when ready — Payverge does not own a publish clock.",
  "",
].join("\n");

const WINDOW_INSTRUCTIONS = [
  "Print shop instructions — window cling (square)",
  "===============================================",
  "",
  "File: high-resolution PNG (2× social square = 2160 × 2160 px).",
  "Ask the shop to size to your glass panel; common clings are 20–40 cm square.",
  "Provide a white underbase if printing on clear vinyl.",
  "",
  "Source aspect: 1:1. Do not stretch — letterbox or crop only if the panel is not square.",
  "Colour: sRGB PNG. Request CMYK conversion at the shop if needed.",
  "",
  "Not a thermal receipt.",
  "You apply the cling when ready — Payverge does not own a publish clock.",
  "",
].join("\n");

const FLYER_INSTRUCTIONS = [
  "Print shop instructions — flyer (half-letter landscape base)",
  "============================================================",
  "",
  "File: high-resolution wide PNG (2× 1920×1080 = 3840 × 2160 px).",
  "Scale to half-letter (5.5 × 8.5 in) or A5 as needed; keep margins ≥ 5 mm.",
  "Bleed: add 3 mm at the shop if full-bleed is required (this PNG is trim-safe).",
  "",
  "Source aspect: 16:9 wide. Letterbox or crop carefully for portrait half-letter.",
  "Colour: sRGB PNG.",
  "",
  "Not a thermal receipt.",
  "You distribute flyers when ready — Payverge does not own a publish clock.",
  "",
].join("\n");

export const PRINT_PRESETS: Record<PrintPresetId, PrintPresetDef> = {
  table_tent_5x7: {
    id: "table_tent_5x7",
    labelKey: "printShop.presets.table_tent_5x7.name",
    hintKey: "printShop.presets.table_tent_5x7.hint",
    formatId: "5:7",
    exportKind: "print_html",
    rasterScale: 1,
    destinationIds: ["print_tent"],
    defaultInstructions: TABLE_TENT_INSTRUCTIONS,
  },
  window_clings_square: {
    id: "window_clings_square",
    labelKey: "printShop.presets.window_clings_square.name",
    hintKey: "printShop.presets.window_clings_square.hint",
    formatId: "1:1",
    exportKind: "high_dpi_png",
    rasterScale: 2,
    destinationIds: ["print_window"],
    defaultInstructions: WINDOW_INSTRUCTIONS,
  },
  flyer_half_letter: {
    id: "flyer_half_letter",
    labelKey: "printShop.presets.flyer_half_letter.name",
    hintKey: "printShop.presets.flyer_half_letter.hint",
    formatId: "wide",
    exportKind: "high_dpi_png",
    rasterScale: 2,
    destinationIds: [],
    defaultInstructions: FLYER_INSTRUCTIONS,
  },
};

export const PRINT_PRESET_ORDER: readonly PrintPresetId[] = [
  "table_tent_5x7",
  "window_clings_square",
  "flyer_half_letter",
];

export function isPrintPresetId(value: unknown): value is PrintPresetId {
  return (
    typeof value === "string" &&
    (PRINT_PRESET_ORDER as readonly string[]).includes(value)
  );
}

export function printPresetFor(
  id: string | null | undefined,
): PrintPresetDef | null {
  return isPrintPresetId(id) ? PRINT_PRESETS[id] : null;
}

/**
 * Default print-shop preset for a destination pack, if any.
 * Non-print destinations return null.
 */
export function printPresetForDestination(
  destinationId: string | null | undefined,
): PrintPresetDef | null {
  const dest = (destinationId ?? "").trim();
  if (!dest) return null;
  for (const id of PRINT_PRESET_ORDER) {
    const preset = PRINT_PRESETS[id];
    if ((preset.destinationIds as readonly string[]).includes(dest)) {
      return preset;
    }
  }
  return null;
}

/**
 * Effective FormatDef for the preset. print_html requires format.print.
 * high_dpi_png uses the screen format (print may be null).
 */
export function formatForPrintPreset(preset: PrintPresetDef): FormatDef {
  const format = FORMATS[preset.formatId];
  if (!format) {
    // Defensive — registry is closed; tests pin formatId validity.
    return FORMATS["1:1"];
  }
  return format;
}

/**
 * Target canvas width for high_dpi_png export (native × scale).
 * Returns undefined for print_html (walker owns physical size).
 */
export function printPresetRasterWidth(preset: PrintPresetDef): number | undefined {
  if (preset.exportKind !== "high_dpi_png") return undefined;
  const format = formatForPrintPreset(preset);
  const scale = Math.min(2, Math.max(1, preset.rasterScale || 1));
  return Math.round(format.px.w * scale);
}

/**
 * Sanity: print_html presets must have physical print specs.
 * high_dpi_png presets must resolve a FormatId.
 */
export function assertPrintPresetValid(preset: PrintPresetDef): string | null {
  if (!isFormatId(preset.formatId) || !FORMATS[preset.formatId]) {
    return "unknown_format";
  }
  if (preset.exportKind === "print_html") {
    const print = FORMATS[preset.formatId].print as PrintSpec | null;
    if (!print) return "print_html_requires_print_spec";
    // pageBox must be finite (Chromium rejects auto page size).
    const page = pageBoxMm(print);
    if (!(page.wMm > 0 && page.hMm > 0)) return "invalid_page_box";
  }
  if (preset.exportKind === "high_dpi_png") {
    const w = printPresetRasterWidth(preset);
    if (!w || w < FORMATS[preset.formatId].px.w) return "invalid_raster_width";
  }
  return null;
}

/** Keys that must never appear on a PrintPresetDef (schedule guard). */
export const FORBIDDEN_PRINT_PRESET_KEYS = [
  "scheduled_at",
  "schedule",
  "due_at",
  "dueDate",
  "cadence",
  "queue",
  "best_time",
  "publish_at",
] as const;
