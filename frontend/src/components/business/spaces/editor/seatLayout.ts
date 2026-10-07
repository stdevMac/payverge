/**
 * Seat placement around table shapes (client mirror of geometry.SeatPositions).
 * Coordinates are millimeters relative to the table top-left (unrotated).
 */

import type { Point } from "./geometry";

function perimeterSeats(seats: number, w: number, h: number): Point[] {
  const perim = 2 * (w + h);
  if (perim <= 0) return [];
  const out: Point[] = [];
  for (let i = 0; i < seats; i++) {
    // Start mid-top, go clockwise (matches backend).
    let d = ((i + 0.5) / seats) * perim;
    d = (d + w / 2) % perim;
    if (d <= w) {
      out.push({ x: d, y: 0 });
    } else if (d <= w + h) {
      out.push({ x: w, y: d - w });
    } else if (d <= 2 * w + h) {
      out.push({ x: w - (d - w - h), y: h });
    } else {
      out.push({ x: 0, y: h - (d - 2 * w - h) });
    }
  }
  return out;
}

/**
 * Approximate seat anchor points around a table perimeter.
 * First seat for round/oval is at 12 o'clock, then clockwise in Y-down space.
 */
export function seatPositions(
  shape: string,
  seats: number,
  widthMm: number,
  heightMm: number,
): Point[] {
  if (seats <= 0 || widthMm <= 0 || heightMm <= 0) return [];
  const cx = widthMm / 2;
  const cy = heightMm / 2;

  switch (shape) {
    case "round":
    case "oval": {
      const rx = widthMm / 2;
      const ry = heightMm / 2;
      const out: Point[] = [];
      for (let i = 0; i < seats; i++) {
        const angle = -Math.PI / 2 + (i / seats) * 2 * Math.PI;
        out.push({
          x: cx + rx * Math.cos(angle),
          y: cy + ry * Math.sin(angle),
        });
      }
      return out;
    }
    case "bar": {
      const out: Point[] = [];
      for (let i = 0; i < seats; i++) {
        const t = (i + 0.5) / seats;
        if (widthMm >= heightMm) {
          out.push({ x: t * widthMm, y: 0 });
        } else {
          out.push({ x: 0, y: t * heightMm });
        }
      }
      return out;
    }
    default:
      return perimeterSeats(seats, widthMm, heightMm);
  }
}

/** Seat radius in mm for rendering (scales slightly with table size). */
export function seatRenderRadius(widthMm: number, heightMm: number): number {
  const m = Math.min(widthMm, heightMm);
  return Math.max(80, Math.min(140, m * 0.12));
}

/**
 * Offset seat anchors slightly outward from the table edge so markers
 * don't heavily overlap the table body.
 */
export function outwardSeatPositions(
  shape: string,
  seats: number,
  widthMm: number,
  heightMm: number,
  offsetMm = 120,
): Point[] {
  const raw = seatPositions(shape, seats, widthMm, heightMm);
  const cx = widthMm / 2;
  const cy = heightMm / 2;
  return raw.map((p) => {
    const dx = p.x - cx;
    const dy = p.y - cy;
    const len = Math.hypot(dx, dy) || 1;
    // Edge points already on perimeter; push outward along radial (or normal).
    if (shape === "round" || shape === "oval") {
      return {
        x: p.x + (dx / len) * offsetMm,
        y: p.y + (dy / len) * offsetMm,
      };
    }
    // For rect/bar, push away from center along axis of greater offset.
    if (Math.abs(dx) >= Math.abs(dy)) {
      return { x: p.x + Math.sign(dx || 1) * offsetMm, y: p.y };
    }
    return { x: p.x, y: p.y + Math.sign(dy || 1) * offsetMm };
  });
}
