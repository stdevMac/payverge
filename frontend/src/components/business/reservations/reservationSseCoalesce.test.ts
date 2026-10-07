/** @jest-environment node */
import {
  decideReservationRefreshCoalesce,
  RESERVATION_SSE_COALESCE_MS,
} from "./reservationSseCoalesce";

describe("decideReservationRefreshCoalesce", () => {
  it("runs immediately when outside the coalesce window", () => {
    expect(
      decideReservationRefreshCoalesce(2000, 0, false, 1500),
    ).toEqual({ action: "run_now" });
  });

  it("schedules trailing when inside the window", () => {
    expect(
      decideReservationRefreshCoalesce(500, 0, false, 1500),
    ).toEqual({ action: "schedule_trailing", delayMs: 1000 });
  });

  it("collapses further events while a trailing timer is pending", () => {
    expect(
      decideReservationRefreshCoalesce(800, 0, true, 1500),
    ).toEqual({ action: "covered_by_pending_trailing" });
  });

  it("defaults the window to RESERVATION_SSE_COALESCE_MS", () => {
    expect(RESERVATION_SSE_COALESCE_MS).toBe(1500);
    const decision = decideReservationRefreshCoalesce(100, 0, false);
    expect(decision).toEqual({
      action: "schedule_trailing",
      delayMs: 1400,
    });
  });

  it("a burst of 5 events yields at most one run_now + one trailing schedule", () => {
    let lastLoad = 0;
    let pending = false;
    let runNow = 0;
    let trailingSchedules = 0;
    const windowMs = 1500;

    for (let i = 0; i < 5; i++) {
      const now = 10 + i; // all inside the first window after t=0 lastLoad
      // First event at t=0 with lastLoad=0 → elapsed 0 < window → trailing
      // unless lastLoad is far in the past.
      const decision = decideReservationRefreshCoalesce(
        now,
        lastLoad,
        pending,
        windowMs,
      );
      if (decision.action === "run_now") {
        runNow += 1;
        lastLoad = now;
      } else if (decision.action === "schedule_trailing") {
        trailingSchedules += 1;
        pending = true;
      }
    }

    // With lastLoad=0 and now≈10, first event schedules trailing; rest covered.
    expect(runNow).toBe(0);
    expect(trailingSchedules).toBe(1);

    // After a quiet period past the window, next event runs immediately.
    const later = decideReservationRefreshCoalesce(5000, lastLoad, false, windowMs);
    expect(later.action).toBe("run_now");
  });
});
