import { countCookQueue, countPendingApprovals } from "./orderQueueCounts";

/**
 * #792/#793 — rail badges must match the tab data, not alert-ping counts.
 * Fixture mirrors the QA night: Approved=2 (both Bill #1143), In Kitchen=3
 * (leftover T2/T3/T5), plus terminal history rows the badge must ignore.
 */
const RUSH_ORDERS = {
  1143: [{ status: "approved" }, { status: "approved" }],
  847: [{ status: "in_kitchen" }],
  848: [{ status: "in_kitchen" }],
  850: [{ status: "in_kitchen" }],
  761: [{ status: "delivered" }, { status: "cancelled" }],
};

describe("countCookQueue (#792 Kitchen rail badge)", () => {
  it("counts pending + approved — never in_kitchen/ready/terminal", () => {
    expect(countCookQueue(RUSH_ORDERS)).toBe(2);
    expect(
      countCookQueue({
        1: [{ status: "pending" }, { status: "approved" }],
        2: [{ status: "ready" }],
      }),
    ).toBe(2);
  });

  it("is 0 on an empty or missing feed", () => {
    expect(countCookQueue({})).toBe(0);
    expect(countCookQueue(null)).toBe(0);
    expect(countCookQueue(undefined)).toBe(0);
  });
});

describe("countPendingApprovals (#793 Bills rail badge)", () => {
  it("counts only needs-approval tickets — open checks are not a pip", () => {
    // Approval queue empty ⇒ 0, even with 5 live kitchen tickets on the board.
    expect(countPendingApprovals(RUSH_ORDERS)).toBe(0);
    expect(
      countPendingApprovals({
        1143: [{ status: "pending" }, { status: "pending" }],
        761: [{ status: "in_kitchen" }],
      }),
    ).toBe(2);
  });

  it("is 0 on an empty or missing feed", () => {
    expect(countPendingApprovals({})).toBe(0);
    expect(countPendingApprovals(null)).toBe(0);
  });
});
