import {
  outwardSeatPositions,
  seatPositions,
  seatRenderRadius,
} from "./seatLayout";

describe("seatLayout", () => {
  it("returns empty for invalid seat counts", () => {
    expect(seatPositions("round", 0, 1000, 1000)).toEqual([]);
    expect(seatPositions("round", 4, 0, 1000)).toEqual([]);
  });

  it("places round seats starting at 12 o'clock", () => {
    const seats = seatPositions("round", 4, 1000, 1000);
    expect(seats).toHaveLength(4);
    // First seat near top (y smaller than center)
    expect(seats[0].y).toBeLessThan(500);
    expect(seats[0].x).toBeCloseTo(500, 0);
  });

  it("places rectangle seats around perimeter", () => {
    const seats = seatPositions("rectangle", 4, 2000, 1000);
    expect(seats).toHaveLength(4);
    // All on perimeter
    for (const s of seats) {
      const onEdge =
        Math.abs(s.y) < 1e-6 ||
        Math.abs(s.y - 1000) < 1e-6 ||
        Math.abs(s.x) < 1e-6 ||
        Math.abs(s.x - 2000) < 1e-6;
      expect(onEdge).toBe(true);
    }
  });

  it("places bar seats along the long edge", () => {
    const seats = seatPositions("bar", 3, 3000, 400);
    expect(seats).toHaveLength(3);
    expect(seats.every((s) => s.y === 0)).toBe(true);
  });

  it("offsets seats outward without collapsing to center", () => {
    const raw = seatPositions("square", 4, 1000, 1000);
    const out = outwardSeatPositions("square", 4, 1000, 1000, 100);
    expect(out).toHaveLength(4);
    const cx = 500;
    const cy = 500;
    for (let i = 0; i < 4; i++) {
      const dRaw = Math.hypot(raw[i].x - cx, raw[i].y - cy);
      const dOut = Math.hypot(out[i].x - cx, out[i].y - cy);
      expect(dOut).toBeGreaterThanOrEqual(dRaw);
    }
  });

  it("computes a reasonable seat render radius", () => {
    expect(seatRenderRadius(900, 900)).toBeGreaterThanOrEqual(80);
    expect(seatRenderRadius(900, 900)).toBeLessThanOrEqual(140);
  });
});
