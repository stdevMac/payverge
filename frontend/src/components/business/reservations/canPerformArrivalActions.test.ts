/** @jest-environment node */
import {
  EARLY_SEATING_GRACE_MINUTES,
  canMarkNoShowNow,
  canPerformArrivalActions,
  canSeatNow,
  isArrivalActionStatus,
} from "./canPerformArrivalActions";

describe("canPerformArrivalActions (L1-13)", () => {
  const now = new Date("2026-06-01T15:00:00.000Z");

  it("allows arrival actions at or after reservation start", () => {
    expect(
      canPerformArrivalActions(
        { reservation_time: "2026-06-01T14:00:00.000Z" },
        now,
      ),
    ).toBe(true);
    expect(
      canPerformArrivalActions(
        { reservation_time: "2026-06-01T15:00:00.000Z" },
        now,
      ),
    ).toBe(true);
  });

  it("blocks arrival actions on future-dated rows", () => {
    expect(
      canPerformArrivalActions(
        { reservation_time: "2026-06-03T20:00:00.000Z" },
        now,
      ),
    ).toBe(false);
  });

  it("honors a pre-arrival grace window", () => {
    // 30 min before start with 60 min grace → allowed
    expect(
      canPerformArrivalActions(
        { reservation_time: "2026-06-01T15:30:00.000Z" },
        now,
        60,
      ),
    ).toBe(true);
    // 90 min before start with 60 min grace → blocked
    expect(
      canPerformArrivalActions(
        { reservation_time: "2026-06-01T16:30:00.000Z" },
        now,
        60,
      ),
    ).toBe(false);
  });

  it("combines status + time for seat affordances", () => {
    expect(isArrivalActionStatus("confirmed")).toBe(true);
    expect(isArrivalActionStatus("seated")).toBe(false);
    expect(
      canSeatNow(
        {
          status: "confirmed",
          reservation_time: "2026-06-03T20:00:00.000Z",
        },
        now,
      ),
    ).toBe(false);
    expect(
      canSeatNow(
        {
          status: "confirmed",
          reservation_time: "2026-06-01T14:00:00.000Z",
        },
        now,
      ),
    ).toBe(true);
  });
});

describe("early-seating grace (R2-B5)", () => {
  const now = new Date("2026-06-01T15:00:00.000Z");

  it("uses a shared 30-minute default", () => {
    expect(EARLY_SEATING_GRACE_MINUTES).toBe(30);
  });

  it("seats a guest who arrives exactly at start − 30 min", () => {
    expect(
      canSeatNow(
        { status: "confirmed", reservation_time: "2026-06-01T15:30:00.000Z" },
        now,
      ),
    ).toBe(true);
  });

  it("does not seat at start − 31 min", () => {
    expect(
      canSeatNow(
        { status: "confirmed", reservation_time: "2026-06-01T15:31:00.000Z" },
        now,
      ),
    ).toBe(false);
  });

  it("never allows no-show before the reservation start", () => {
    // Inside the seating grace, but the guest is not late yet.
    expect(
      canMarkNoShowNow(
        { status: "confirmed", reservation_time: "2026-06-01T15:30:00.000Z" },
        now,
      ),
    ).toBe(false);
    // One minute in the future is still not a no-show.
    expect(
      canMarkNoShowNow(
        { status: "confirmed", reservation_time: "2026-06-01T15:01:00.000Z" },
        now,
      ),
    ).toBe(false);
    // At start, and past it, no-show is allowed.
    expect(
      canMarkNoShowNow(
        { status: "confirmed", reservation_time: "2026-06-01T15:00:00.000Z" },
        now,
      ),
    ).toBe(true);
    expect(
      canMarkNoShowNow(
        { status: "confirmed", reservation_time: "2026-06-01T14:30:00.000Z" },
        now,
      ),
    ).toBe(true);
  });
});
