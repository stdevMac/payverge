import type { CSSProperties } from "react";

/** Reserved chrome; canvas keeps the leftover width. */
const EDITOR_PALETTE_WIDTH_PX = 52;
const EDITOR_PROPERTIES_WIDTH_PX = 224;
export const EDITOR_CANVAS_MIN_HEIGHT_PX = 320;

const OVERLAY_POSITIONS = new Set(["absolute", "fixed", "sticky"]);

export const EDITOR_WORKSPACE_EDIT_STYLE: CSSProperties = {
  display: "flex",
  flexDirection: "row",
  flexGrow: 1,
  flexShrink: 1,
  flexBasis: "0%",
  minWidth: 0,
  minHeight: 0,
  overflow: "hidden",
};

export const EDITOR_WORKSPACE_PREVIEW_STYLE: CSSProperties = {
  display: "flex",
  flexDirection: "row",
  flexGrow: 1,
  flexShrink: 1,
  flexBasis: "0%",
  minWidth: 0,
  minHeight: 0,
  overflow: "hidden",
};

export const EDITOR_PALETTE_COLUMN_STYLE: CSSProperties = {
  flexGrow: 0,
  flexShrink: 0,
  flexBasis: EDITOR_PALETTE_WIDTH_PX,
  width: EDITOR_PALETTE_WIDTH_PX,
  minWidth: EDITOR_PALETTE_WIDTH_PX,
  maxWidth: EDITOR_PALETTE_WIDTH_PX,
  minHeight: 0,
  position: "relative",
  overflow: "hidden",
};

export const EDITOR_CANVAS_COLUMN_STYLE: CSSProperties = {
  flexGrow: 1,
  flexShrink: 1,
  flexBasis: "0%",
  minWidth: 0,
  minHeight: EDITOR_CANVAS_MIN_HEIGHT_PX,
  position: "relative",
  overflow: "hidden",
};

export const EDITOR_PROPERTIES_COLUMN_STYLE: CSSProperties = {
  flexGrow: 0,
  flexShrink: 0,
  flexBasis: EDITOR_PROPERTIES_WIDTH_PX,
  width: EDITOR_PROPERTIES_WIDTH_PX,
  minWidth: EDITOR_PROPERTIES_WIDTH_PX,
  maxWidth: EDITOR_PROPERTIES_WIDTH_PX,
  minHeight: 0,
  position: "relative",
  overflow: "auto",
};

export const EDITOR_TOOLBAR_STYLE: CSSProperties = {
  display: "flex",
  flexWrap: "nowrap",
  alignItems: "center",
  overflowX: "auto",
};

function usedPosition(el: HTMLElement): string {
  return (el.style.position || "").trim().toLowerCase();
}

/**
 * True when Properties would paint over the floor plan: nested in the canvas
 * column, absolutely positioned, or not in a row flex sibling layout where
 * the canvas is the growing hero.
 *
 * Reads inline styles on purpose — Tailwind `lg:` utilities do not apply in
 * jsdom, so a class-string check cannot catch an overlay regression.
 */
export function propertiesCoversCanvas(
  workspace: HTMLElement,
  canvas: HTMLElement,
  properties: HTMLElement | null,
): boolean {
  if (!properties) return false;
  if (canvas.contains(properties) || properties.contains(canvas)) return true;
  if (OVERLAY_POSITIONS.has(usedPosition(properties))) return true;
  if (OVERLAY_POSITIONS.has(usedPosition(canvas))) return true;
  if (workspace.style.display !== "flex") return true;
  if (workspace.style.flexDirection !== "row") return true;
  const canvasGrow = Number.parseFloat(canvas.style.flexGrow || "0");
  if (!(canvasGrow >= 1)) return true;
  const propertiesGrow = Number.parseFloat(properties.style.flexGrow || "0");
  if (propertiesGrow >= 1) return true;
  return false;
}
