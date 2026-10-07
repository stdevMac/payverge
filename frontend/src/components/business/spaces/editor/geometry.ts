/**
 * Client-side layout geometry helpers (mirrors backend/internal/spaces/geometry).
 * Units: millimeters. Origin top-left; X right, Y down. Rotation: degrees CW.
 */

export interface Point {
  x: number;
  y: number;
}

interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface OrientedRect {
  x: number;
  y: number;
  w: number;
  h: number;
  rotationDeg: number;
}

export const DEFAULT_GRID_MM = 250;
const GUIDE_THRESHOLD_MM = 80;
export const DEFAULT_ROOM_WIDTH_MM = 12000;
export const DEFAULT_ROOM_HEIGHT_MM = 8000;

function rotatePoint(p: Point, origin: Point, degreesCW: number): Point {
  const rad = (degreesCW * Math.PI) / 180;
  const cos = Math.cos(rad);
  const sin = Math.sin(rad);
  const dx = p.x - origin.x;
  const dy = p.y - origin.y;
  return {
    x: origin.x + dx * cos - dy * sin,
    y: origin.y + dx * sin + dy * cos,
  };
}

function rectCenter(r: Rect | OrientedRect): Point {
  return { x: r.x + r.w / 2, y: r.y + r.h / 2 };
}

function orientedCorners(r: OrientedRect): Point[] {
  const origin = rectCenter(r);
  const corners: Point[] = [
    { x: r.x, y: r.y },
    { x: r.x + r.w, y: r.y },
    { x: r.x + r.w, y: r.y + r.h },
    { x: r.x, y: r.y + r.h },
  ];
  if (Math.abs(r.rotationDeg) < 1e-9) return corners;
  return corners.map((c) => rotatePoint(c, origin, r.rotationDeg));
}

function project(poly: Point[], axis: Point): [number, number] {
  let min = poly[0].x * axis.x + poly[0].y * axis.y;
  let max = min;
  for (let i = 1; i < poly.length; i++) {
    const v = poly[i].x * axis.x + poly[i].y * axis.y;
    if (v < min) min = v;
    if (v > max) max = v;
  }
  return [min, max];
}

function satOverlap(poly: Point[], other: Point[]): boolean {
  const n = poly.length;
  for (let i = 0; i < n; i++) {
    const j = (i + 1) % n;
    const edge = { x: poly[j].x - poly[i].x, y: poly[j].y - poly[i].y };
    const axis = { x: -edge.y, y: edge.x };
    const [minA, maxA] = project(poly, axis);
    const [minB, maxB] = project(other, axis);
    if (maxA < minB - 1e-9 || maxB < minA - 1e-9) return false;
  }
  return true;
}

export function tableIntersects(a: OrientedRect, b: OrientedRect): boolean {
  const ca = orientedCorners(a);
  const cb = orientedCorners(b);
  return satOverlap(ca, cb) && satOverlap(cb, ca);
}

function snapToGrid(value: number, gridMm = DEFAULT_GRID_MM): number {
  if (gridMm <= 0) return value;
  return Math.round(value / gridMm) * gridMm;
}

export interface SnapResult {
  point: Point;
  guides: AlignmentGuide[];
}

export interface AlignmentGuide {
  orientation: "v" | "h";
  /** Position in mm (x for vertical, y for horizontal). */
  position: number;
}

/**
 * Snap a dragged top-left (or center) point toward grid and other element
 * centers / edges. Returns adjusted point plus guide lines to render.
 */
export function snapPosition(
  raw: Point,
  size: { w: number; h: number },
  others: OrientedRect[],
  opts?: {
    gridMm?: number;
    snapGrid?: boolean;
    snapElements?: boolean;
    thresholdMm?: number;
  },
): SnapResult {
  const gridMm = opts?.gridMm ?? DEFAULT_GRID_MM;
  const threshold = opts?.thresholdMm ?? GUIDE_THRESHOLD_MM;
  let x = raw.x;
  let y = raw.y;
  const guides: AlignmentGuide[] = [];

  if (opts?.snapGrid !== false) {
    x = snapToGrid(x, gridMm);
    y = snapToGrid(y, gridMm);
  }

  if (opts?.snapElements !== false && others.length > 0) {
    const cx = x + size.w / 2;
    const cy = y + size.h / 2;
    let bestDx = threshold + 1;
    let bestDy = threshold + 1;
    let guideV: number | null = null;
    let guideH: number | null = null;

    for (const o of others) {
      const ocx = o.x + o.w / 2;
      const ocy = o.y + o.h / 2;
      const edgesX = [o.x, ocx, o.x + o.w];
      const edgesY = [o.y, ocy, o.y + o.h];
      const selfX = [x, cx, x + size.w];
      const selfY = [y, cy, y + size.h];

      for (const sx of selfX) {
        for (const ex of edgesX) {
          const d = Math.abs(sx - ex);
          if (d < bestDx && d <= threshold) {
            bestDx = d;
            guideV = ex;
            x = x + (ex - sx);
          }
        }
      }
      for (const sy of selfY) {
        for (const ey of edgesY) {
          const d = Math.abs(sy - ey);
          if (d < bestDy && d <= threshold) {
            bestDy = d;
            guideH = ey;
            y = y + (ey - sy);
          }
        }
      }
    }
    if (guideV != null) guides.push({ orientation: "v", position: guideV });
    if (guideH != null) guides.push({ orientation: "h", position: guideH });
  }

  return { point: { x, y }, guides };
}

export function polygonPath(points: Point[]): string {
  if (points.length === 0) return "";
  return (
    points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ") +
    " Z"
  );
}

export function hitTestRect(
  p: Point,
  r: OrientedRect,
  pad = 0,
): boolean {
  if (Math.abs(r.rotationDeg) < 1e-6) {
    return (
      p.x >= r.x - pad &&
      p.x <= r.x + r.w + pad &&
      p.y >= r.y - pad &&
      p.y <= r.y + r.h + pad
    );
  }
  const center = rectCenter(r);
  const local = rotatePoint(p, center, -r.rotationDeg);
  return (
    local.x >= r.x - pad &&
    local.x <= r.x + r.w + pad &&
    local.y >= r.y - pad &&
    local.y <= r.y + r.h + pad
  );
}

/** Convert display units (m or ft) to millimeters. */
export function toMm(value: number, unit: "m" | "ft" | "mm"): number {
  if (unit === "m") return value * 1000;
  if (unit === "ft") return value * 304.8;
  return value;
}

export function fromMm(mm: number, unit: "m" | "ft" | "mm"): number {
  if (unit === "m") return mm / 1000;
  if (unit === "ft") return mm / 304.8;
  return mm;
}
