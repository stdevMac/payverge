import { detectOrderTransitions } from "./orderTransitions";

describe("detectOrderTransitions", () => {
  it("detects cancelled and ready transitions only from a known prior status", () => {
    const known = new Map<number, string>([
      [1, "in_kitchen"],
      [2, "pending"],
      [3, "ready"],
    ]);
    const result = detectOrderTransitions(
      [
        { id: 1, status: "ready" },
        { id: 2, status: "cancelled" },
        { id: 3, status: "ready" }, // unchanged — no event
        { id: 4, status: "ready" }, // first sighting — no event
      ],
      known,
    );
    expect(result.becameReady).toEqual([1]);
    expect(result.staffCancelled).toEqual([2]);
  });

  it("returns empty transitions for an empty poll", () => {
    expect(detectOrderTransitions([], new Map())).toEqual({
      staffCancelled: [],
      becameReady: [],
    });
  });
});
