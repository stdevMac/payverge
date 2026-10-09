/** @jest-environment node */
import { resolveReservationEmptyKind } from "./reservationEmptyState";

describe("resolveReservationEmptyKind (L1-8)", () => {
  const clear = {
    search: "",
    attentionFilter: "none",
    statusFilter: "all",
  };

  it("treats default today as a narrowing filter, not first-run", () => {
    expect(
      resolveReservationEmptyKind({ ...clear, dateFilter: "today" }),
    ).toBe("today_empty");
  });

  it("uses filtered empty for upcoming/past/custom windows", () => {
    for (const dateFilter of ["upcoming", "past", "custom"]) {
      expect(resolveReservationEmptyKind({ ...clear, dateFilter })).toBe(
        "filtered",
      );
    }
  });

  it("uses first-run only when the window is fully open and unfiltered", () => {
    expect(
      resolveReservationEmptyKind({ ...clear, dateFilter: "all_time" }),
    ).toBe("first_run");
    expect(resolveReservationEmptyKind({ ...clear, dateFilter: "all" })).toBe(
      "first_run",
    );
  });

  it("uses filtered empty when search/status/attention narrow", () => {
    expect(
      resolveReservationEmptyKind({
        search: "Maria",
        attentionFilter: "none",
        statusFilter: "all",
        dateFilter: "all_time",
      }),
    ).toBe("filtered");
    expect(
      resolveReservationEmptyKind({
        search: "",
        attentionFilter: "none",
        statusFilter: "confirmed",
        dateFilter: "all_time",
      }),
    ).toBe("filtered");
  });
});
