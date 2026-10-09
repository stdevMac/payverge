import {
  buildDispatchBoardLoadPlan,
  DISPATCH_ACTIVE_STATUSES,
  DISPATCH_PAGE_SIZE,
  DISPATCH_TERMINAL_STATUSES,
  isDispatchLoadCanceled,
  mergeDispatchBoardResults,
  startOfBusinessDayISO,
} from "../dispatchBoardLoad";
import type { DeliveryListResult, DeliveryOrder } from "@/api/delivery";

describe("buildDispatchBoardLoadPlan", () => {
  it("plans a status-filtered active query and a since-bounded terminal query", () => {
    const now = new Date("2026-07-06T15:00:00Z");
    const plan = buildDispatchBoardLoadPlan({
      timeZone: "UTC",
      now,
      activeLimit: 50,
      terminalLimit: 25,
    });

    expect(plan.active.status).toEqual(DISPATCH_ACTIVE_STATUSES);
    expect(plan.active.limit).toBe(50);
    expect(plan.terminal.status).toEqual(DISPATCH_TERMINAL_STATUSES);
    expect(plan.terminal.limit).toBe(25);
    // Terminal window starts at business-day midnight, not "all time".
    expect(plan.terminal.since).toBe(startOfBusinessDayISO("UTC", now));
    expect(new Date(plan.terminal.since).getTime()).toBeLessThanOrEqual(now.getTime());
  });

  it("defaults page size and never omits status filters", () => {
    const plan = buildDispatchBoardLoadPlan();
    expect(plan.active.limit).toBe(DISPATCH_PAGE_SIZE);
    expect(plan.terminal.limit).toBe(DISPATCH_PAGE_SIZE);
    expect(plan.active.status.length).toBeGreaterThan(0);
    expect(plan.terminal.status.length).toBeGreaterThan(0);
    // Must not plan a silent unfiltered 200-row dump.
    expect(plan.active.status).not.toContain("delivered");
    expect(plan.terminal.status).not.toContain("preparing");
  });
});

describe("mergeDispatchBoardResults", () => {
  const mk = (id: number, status: string): DeliveryOrder =>
    ({
      id,
      business_id: 1,
      delivery_number: `DEL-${id}`,
      delivery_type: "in_house",
      status,
      customer_name: "X",
      customer_phone: "1",
      delivery_address: { street: "s", city: "c", country: "US" },
      delivery_fee: 1,
      contactless_delivery: false,
      leave_at_door: false,
      created_at: "2026-07-06T10:00:00Z",
      updated_at: "2026-07-06T10:00:00Z",
    }) as DeliveryOrder;

  it("merges, dedupes, and surfaces has_more from either window", () => {
    const active: DeliveryListResult = {
      deliveries: [mk(1, "preparing"), mk(2, "ready")],
      total: 150,
      has_more: true,
    };
    const terminal: DeliveryListResult = {
      deliveries: [mk(2, "ready"), mk(3, "delivered")],
      total: 3,
      has_more: false,
    };
    const merged = mergeDispatchBoardResults(active, terminal);
    expect(merged.orders.map((o) => o.id).sort()).toEqual([1, 2, 3]);
    expect(merged.activeTotal).toBe(150);
    expect(merged.terminalTotal).toBe(3);
    expect(merged.activeHasMore).toBe(true);
    expect(merged.terminalHasMore).toBe(false);
    expect(merged.hasMore).toBe(true);
  });

  it("reports no truncation when both windows fit", () => {
    const merged = mergeDispatchBoardResults(
      { deliveries: [mk(1, "preparing")], total: 1, has_more: false },
      { deliveries: [], total: 0, has_more: false },
    );
    expect(merged.hasMore).toBe(false);
    expect(merged.activeTotal).toBe(1);
  });
});

describe("startOfBusinessDayISO", () => {
  it("returns a midnight-ish instant for the business timezone", () => {
    const now = new Date("2026-07-06T18:30:00Z");
    const iso = startOfBusinessDayISO("UTC", now);
    expect(iso.startsWith("2026-07-06T00:00:00")).toBe(true);
  });
});

describe("isDispatchLoadCanceled", () => {
  it("treats abort errors and aborted signals as non-failures", () => {
    const abortErr = { name: "AbortError", code: "ERR_CANCELED" };
    expect(isDispatchLoadCanceled(abortErr)).toBe(true);
    const ac = new AbortController();
    ac.abort();
    expect(isDispatchLoadCanceled(new Error("network"), ac.signal)).toBe(true);
    expect(isDispatchLoadCanceled(new Error("network"))).toBe(false);
  });
});
