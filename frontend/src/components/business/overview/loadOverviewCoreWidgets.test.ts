/**
 * L9-2: real cancel-path tests for overview core widgets.
 * Asserts AbortSignal is threaded into getDashboardSummary / getBusinessTables
 * and that live table failures do not fabricate a successful empty board.
 */
import {
  loadOverviewCoreWidgets,
  type OverviewCoreWidgetDeps,
} from "./loadOverviewCoreWidgets";

describe("loadOverviewCoreWidgets (L9-2 cancel + no fabricate)", () => {
  it("passes the same AbortSignal into getDashboardSummary and getBusinessTables", async () => {
    const controller = new AbortController();
    const getDashboardSummary = jest.fn(async (_id: string, signal?: AbortSignal) => {
      expect(signal).toBe(controller.signal);
      return { today: { revenue: 10, bills: 1 } };
    });
    const getBusinessTables = jest.fn(async (_id: number, signal?: AbortSignal) => {
      expect(signal).toBe(controller.signal);
      return { tables: [{ id: 1 }] };
    });

    const deps: OverviewCoreWidgetDeps = {
      getDashboardSummary,
      getBusinessTables,
    };

    const result = await loadOverviewCoreWidgets({
      businessId: 42,
      canSeeAnalytics: true,
      canSeeTables: true,
      isRestricted: false,
      signal: controller.signal,
      deps,
    });

    expect(getDashboardSummary).toHaveBeenCalledWith("42", controller.signal);
    expect(getBusinessTables).toHaveBeenCalledWith(42, controller.signal);
    expect(result.analyticsError).toBe(false);
    expect(result.tablesError).toBe(false);
    expect(result.tables).toHaveLength(1);
  });

  it("marks tablesError on live failure without treating empty as success data", async () => {
    const controller = new AbortController();
    const deps: OverviewCoreWidgetDeps = {
      getDashboardSummary: jest.fn(async () => ({ today: { revenue: 0, bills: 0 } })),
      getBusinessTables: jest.fn(async () => {
        throw new Error("network down");
      }),
    };

    const result = await loadOverviewCoreWidgets({
      businessId: 7,
      canSeeAnalytics: true,
      canSeeTables: true,
      isRestricted: false,
      signal: controller.signal,
      deps,
    });

    expect(result.tablesError).toBe(true);
    expect(result.tables).toEqual([]);
    // Analytics still ok
    expect(result.analyticsError).toBe(false);
    expect(result.dashboard).not.toBeNull();
  });

  it("marks analyticsError when summary fails without inventing a zero-sales day object", async () => {
    const controller = new AbortController();
    const deps: OverviewCoreWidgetDeps = {
      getDashboardSummary: jest.fn(async () => {
        throw new Error("503");
      }),
      getBusinessTables: jest.fn(async () => ({ tables: [{ id: 2 }] })),
    };

    const result = await loadOverviewCoreWidgets({
      businessId: 7,
      canSeeAnalytics: true,
      canSeeTables: true,
      isRestricted: false,
      signal: controller.signal,
      deps,
    });

    expect(result.analyticsError).toBe(true);
    expect(result.dashboard).toBeNull();
    expect(result.tablesError).toBe(false);
    expect(result.tables).toHaveLength(1);
  });

  it("does not report errors when the signal was aborted mid-flight", async () => {
    const controller = new AbortController();
    const deps: OverviewCoreWidgetDeps = {
      getDashboardSummary: jest.fn(async (_id, signal) => {
        controller.abort();
        const err = Object.assign(new Error("canceled"), {
          name: "CanceledError",
          code: "ERR_CANCELED",
        });
        // Signal is aborted — callers typically reject with CanceledError
        void signal;
        throw err;
      }),
      getBusinessTables: jest.fn(async () => {
        throw Object.assign(new Error("canceled"), { name: "CanceledError" });
      }),
    };

    const result = await loadOverviewCoreWidgets({
      businessId: 7,
      canSeeAnalytics: true,
      canSeeTables: true,
      isRestricted: false,
      signal: controller.signal,
      deps,
    });

    expect(result.analyticsError).toBe(false);
    expect(result.tablesError).toBe(false);
  });
});
