import type {
  LayoutBoundary,
  LayoutDocument,
  LayoutElement,
  LayoutElementType,
  LayoutRegion,
  LayoutTable,
  SpaceMeasurementUnit,
  TableShape,
} from "@/api/spaces";

export type EditorTool =
  | "select"
  | "pan"
  | "draw-rect-room"
  | "draw-polygon"
  | "draw-region"
  | "draw-wall";

export type PreviewMode = "edit" | "guest" | "staff" | "live";

export type SelectionRef =
  | { kind: "table"; key: string }
  | { kind: "element"; key: string }
  | { kind: "region"; key: string };

export interface EditorTable extends LayoutTable {
  clientKey: string;
}

export interface EditorElement extends LayoutElement {
  clientKey: string;
}

export interface EditorRegion extends LayoutRegion {
  clientKey: string;
}

export interface EditorDocument {
  schema_version: number;
  width_mm: number;
  height_mm: number;
  measurement_unit: SpaceMeasurementUnit | string;
  boundary?: LayoutBoundary;
  regions: EditorRegion[];
  elements: EditorElement[];
  tables: EditorTable[];
  meta?: Record<string, unknown>;
}

export type AutosaveStatus =
  | "idle"
  | "dirty"
  | "saving"
  | "saved"
  | "offline"
  | "failed"
  | "conflict";

type SoftWarningCode =
  | "overlap"
  | "outOfBounds"
  | "missingCapacity"
  | "duplicateLabels"
  | "emptyLayout";

export interface SoftWarning {
  code: SoftWarningCode;
  messageKey: string;
  path?: string;
}

export const ELEMENT_TYPES: LayoutElementType[] = [
  "wall",
  "door",
  "window",
  "column",
  "bar",
  "counter",
  "entrance",
  "stairs",
  "service_station",
  "restroom",
  "divider",
  "obstacle",
  "label",
];

export const TABLE_SHAPES: TableShape[] = [
  "round",
  "square",
  "rectangle",
  "oval",
  "bar",
];

export const DEFAULT_TABLE_SIZE: Record<
  string,
  { width_mm: number; height_mm: number }
> = {
  round: { width_mm: 900, height_mm: 900 },
  square: { width_mm: 900, height_mm: 900 },
  rectangle: { width_mm: 1400, height_mm: 900 },
  oval: { width_mm: 1400, height_mm: 900 },
  bar: { width_mm: 2000, height_mm: 600 },
  custom: { width_mm: 1000, height_mm: 1000 },
};

export const DEFAULT_ELEMENT_SIZE: Record<
  string,
  { width_mm: number; height_mm: number }
> = {
  wall: { width_mm: 3000, height_mm: 150 },
  door: { width_mm: 900, height_mm: 150 },
  window: { width_mm: 1200, height_mm: 120 },
  column: { width_mm: 400, height_mm: 400 },
  bar: { width_mm: 3000, height_mm: 700 },
  counter: { width_mm: 2000, height_mm: 600 },
  entrance: { width_mm: 1200, height_mm: 400 },
  stairs: { width_mm: 1200, height_mm: 2000 },
  service_station: { width_mm: 800, height_mm: 800 },
  restroom: { width_mm: 1500, height_mm: 1200 },
  divider: { width_mm: 2000, height_mm: 80 },
  obstacle: { width_mm: 600, height_mm: 600 },
  label: { width_mm: 800, height_mm: 400 },
};

let keySeq = 0;
export function nextClientKey(prefix = "k"): string {
  keySeq += 1;
  return `${prefix}_${Date.now().toString(36)}_${keySeq}`;
}

export function layoutToEditor(
  layout: LayoutDocument | null | undefined,
  fallback?: {
    width_mm?: number | null;
    height_mm?: number | null;
    measurement_unit?: string;
  },
): EditorDocument {
  const width =
    layout?.width_mm ||
    fallback?.width_mm ||
    0;
  const height =
    layout?.height_mm ||
    fallback?.height_mm ||
    0;
  return {
    schema_version: layout?.schema_version ?? 1,
    width_mm: width,
    height_mm: height,
    measurement_unit:
      layout?.measurement_unit ||
      fallback?.measurement_unit ||
      "m",
    boundary: layout?.boundary,
    regions: (layout?.regions ?? []).map((r, i) => ({
      ...r,
      clientKey: nextClientKey(`region${i}`),
    })),
    elements: (layout?.elements ?? []).map((e, i) => ({
      ...e,
      clientKey: nextClientKey(`el${i}`),
    })),
    tables: (layout?.tables ?? []).map((t, i) => ({
      ...t,
      clientKey: nextClientKey(`tbl${i}`),
    })),
    meta: layout?.meta,
  };
}

export function editorToLayout(doc: EditorDocument): LayoutDocument {
  return {
    schema_version: doc.schema_version || 1,
    width_mm: doc.width_mm,
    height_mm: doc.height_mm,
    measurement_unit: doc.measurement_unit,
    boundary: doc.boundary,
    regions: doc.regions.map(({ clientKey: _k, ...r }) => r),
    elements: doc.elements.map(({ clientKey: _k, ...e }) => e),
    tables: doc.tables.map(({ clientKey: _k, ...t }) => t),
    meta: doc.meta,
  };
}

export function cloneDoc(doc: EditorDocument): EditorDocument {
  return JSON.parse(JSON.stringify(doc)) as EditorDocument;
}
