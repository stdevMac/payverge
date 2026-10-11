/**
 * L9-2: real cancel-path tests for ReservationsTodayCard data load.
 * Asserts AbortSignal is threaded into every reservationAPI call.
 */
import { loadReservationsToday, type ReservationsTodayDeps } from "./loadReservationsToday";

describe("loadReservationsToday (L9-2 cancel + no fabricate)", () => {
  it("passes the same AbortSignal into getStats, getUpcoming, and getReservations", async () => {
    const controller = new AbortController();
    const getStats = jest.fn(async (_b, _s, _e, signal?: AbortSignal) => {
      expect(signal).toBe(controller.signal);
      return { total: 4, cancelled: 1, no_show: 1, pending: 1 };
    });
    const getUpcomingReservations = jest.fn(
      async (_b, _limit?: number, signal?: AbortSignal) => {
        expect(signal).toBe(controller.signal);
        return { reservations: [], total: 0 };
      },
    );
    const getReservations = jest.fn(
      async (_b, _s, _e, _st, options?: { pageSize?: number; signal?: AbortSignal }) => {
        expect(options?.signal).toBe(controller.signal);
        return { total: 3 };
      },
    );

    const deps: ReservationsTodayDeps = {
      getStats,
      getUpcomingReservations,
      getReservations,
    };

    const result = await loadReservationsToday({
      businessId: 9,
      today: "2026-08-06",
      signal: controller.signal,
      deps,
    });

    expect(getStats).toHaveBeenCalledWith(
      9,
      "2026-08-06",
      "2026-08-06",
      controller.signal,
    );
    expect(getUpcomingReservations).toHaveBeenCalledWith(
      9,
      5,
      controller.signal,
    );
    expect(getReservations).toHaveBeenCalledWith(
      9,
      undefined,
      undefined,
      "pending",
      expect.objectContaining({ pageSize: 1, signal: controller.signal }),
    );
    expect(result).toEqual({
      ok: true,
      todayCount: 2, // 4 - 1 cancelled - 1 no_show
      pendingCount: 3,
      upcoming: [],
    });
  });

  it("returns ok:false without inventing a zero count on live failure", async () => {
    const controller = new AbortController();
    const deps: ReservationsTodayDeps = {
      getStats: jest.fn(async () => {
        throw new Error("503");
      }),
      getUpcomingReservations: jest.fn(async () => ({ reservations: [] })),
      getReservations: jest.fn(async () => ({ total: 0 })),
    };

    const result = await loadReservationsToday({
      businessId: 1,
      today: "2026-08-06",
      signal: controller.signal,
      deps,
    });

    expect(result).toEqual({ ok: false, aborted: false });
  });

  it("returns aborted:true when signal is aborted (no error UI path)", async () => {
    const controller = new AbortController();
    controller.abort();
    const deps: ReservationsTodayDeps = {
      getStats: jest.fn(async () => {
        throw Object.assign(new Error("canceled"), { name: "CanceledError" });
      }),
      getUpcomingReservations: jest.fn(async () => ({ reservations: [] })),
      getReservations: jest.fn(async () => ({ total: 0 })),
    };

    const result = await loadReservationsToday({
      businessId: 1,
      today: "2026-08-06",
      signal: controller.signal,
      deps,
    });

    expect(result).toEqual({ ok: false, aborted: true });
  });
});
