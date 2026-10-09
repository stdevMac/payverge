import { sortOrdersFifo } from "@/utils/orderSorting";

describe("sortOrdersFifo (K-7)", () => {
  it("sorts oldest ticket first and does not mutate the input", () => {
    const input = [
      { created_at: "2026-06-12T12:30:00Z" },
      { created_at: "2026-06-12T12:00:00Z" },
      { created_at: "2026-06-12T12:15:00Z" },
    ];
    const sorted = sortOrdersFifo(input);
    expect(sorted.map((o) => o.created_at)).toEqual([
      "2026-06-12T12:00:00Z",
      "2026-06-12T12:15:00Z",
      "2026-06-12T12:30:00Z",
    ]);
    expect(input[0].created_at).toBe("2026-06-12T12:30:00Z");
  });
});
