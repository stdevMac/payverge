import { getGuestSlotState, pickRecommendedSlotTime } from "../guestSlotState";

describe("getGuestSlotState", () => {
  it("is open when tables are available", () => {
    expect(
      getGuestSlotState({ available_tables: 2, reason_code: "available" }, true),
    ).toBe("open");
  });

  it("offers the waitlist for fully-booked slots when enabled", () => {
    expect(
      getGuestSlotState(
        { available_tables: 0, reason_code: "covers_limit" },
        true,
      ),
    ).toBe("waitlist");
  });

  it("never offers the waitlist for slots inside the advance window", () => {
    expect(
      getGuestSlotState(
        { available_tables: 0, reason_code: "outside_advance_window" },
        true,
      ),
    ).toBe("unavailable");
  });

  it("never offers the waitlist for slots too close to closing — the backend rejects them outright", () => {
    // outside_operating_window slots fail window validation BEFORE the
    // waitlist fallback, so offering them as waitlist-selectable would
    // guarantee a rejected submission.
    expect(
      getGuestSlotState(
        { available_tables: 0, reason_code: "outside_operating_window" },
        true,
      ),
    ).toBe("unavailable");
  });

  it("is unavailable when the waitlist is off", () => {
    expect(
      getGuestSlotState(
        { available_tables: 0, reason_code: "covers_limit" },
        false,
      ),
    ).toBe("unavailable");
  });
});

describe("pickRecommendedSlotTime", () => {
  const slot = (
    time: string,
    available_tables: number,
    extras: { recommended?: boolean; reason_code?: string } = {},
  ) => ({
    time,
    available_tables,
    recommended: extras.recommended ?? true,
    reason_code: extras.reason_code ?? "available",
  });

  it("returns null when every open slot has the same capacity (no ranking)", () => {
    expect(
      pickRecommendedSlotTime(
        [
          slot("2026-06-20T17:00:00.000Z", 10),
          slot("2026-06-20T17:30:00.000Z", 10),
          slot("2026-06-20T18:00:00.000Z", 10),
        ],
        false,
      ),
    ).toBeNull();
  });

  it("returns null for a fully open day even when the backend marks every slot", () => {
    const slots = Array.from({ length: 8 }, (_, index) =>
      slot(`2026-06-20T${String(17 + index).padStart(2, "0")}:00:00.000Z`, 9),
    );
    expect(pickRecommendedSlotTime(slots, false)).toBeNull();
  });

  it("recommends only the soonest max-capacity slot when capacities differ", () => {
    expect(
      pickRecommendedSlotTime(
        [
          slot("2026-06-20T17:00:00.000Z", 9),
          slot("2026-06-20T17:30:00.000Z", 10),
          slot("2026-06-20T18:00:00.000Z", 10),
          slot("2026-06-20T18:30:00.000Z", 9),
        ],
        false,
      ),
    ).toBe("2026-06-20T17:30:00.000Z");
  });

  it("honors a sparse backend mark among max-capacity slots", () => {
    expect(
      pickRecommendedSlotTime(
        [
          slot("2026-06-20T17:00:00.000Z", 10, { recommended: false }),
          slot("2026-06-20T18:00:00.000Z", 10, { recommended: true }),
          slot("2026-06-20T19:00:00.000Z", 8, { recommended: true }),
        ],
        false,
      ),
    ).toBe("2026-06-20T18:00:00.000Z");
  });

  it("never recommends waitlist-only or constrained (unbookable) windows", () => {
    expect(
      pickRecommendedSlotTime(
        [
          slot("2026-06-20T16:00:00.000Z", 0, {
            recommended: false,
            reason_code: "covers_limit",
          }),
          slot("2026-06-20T16:30:00.000Z", 0, {
            recommended: false,
            reason_code: "outside_operating_window",
          }),
          slot("2026-06-20T17:00:00.000Z", 2),
        ],
        true,
      ),
    ).toBeNull();
  });

  it("returns null when there are no open slots", () => {
    expect(
      pickRecommendedSlotTime(
        [
          slot("2026-06-20T17:00:00.000Z", 0, { reason_code: "covers_limit" }),
          slot("2026-06-20T18:00:00.000Z", 0, { reason_code: "covers_limit" }),
        ],
        true,
      ),
    ).toBeNull();
  });
});
