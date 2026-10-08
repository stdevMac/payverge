import {
  renderPlannedMenuHtml,
  type RenderPlannedMenuOptions,
} from "./renderPlannedHtml";
import type { PlannedMenuDocument, PrintMenuModel } from "./types";

/** Stable public renderer options for planned restaurant menu documents. */
export type RenderMenuPrintOptions = RenderPlannedMenuOptions;

/**
 * Escape text for historical public callers of this module.
 *
 * Apostrophes intentionally remain literal: existing consumers place this
 * output in text nodes and rely on the original `&`, `<`, `>`, `"` contract.
 * The planned menu renderer keeps its stricter internal attribute-safe helper.
 */
export function escapeHtml(value: string): string {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/**
 * Render the exact document produced by the menu planner.
 *
 * Keeping this entry point intentionally small ensures thumbnails, review, and
 * printed output all use the same page plan and renderer.
 */
export function renderMenuPrintHtml(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  options: RenderMenuPrintOptions,
): string {
  return renderPlannedMenuHtml(model, document, options);
}
