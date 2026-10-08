import {
  nextBookableSlotLocal,
  clampPartySize,
  isRawValidatorDump,
  operatingWindowResolver,
  reservationErrorMessage,
  buildReservationUpdatePayload,
} from "../reservationFormHelpers";
import type { Reservation } from "@/api/reservations";

// R2-11: the create form seeded the picker with the next OPEN day's hours but
// kept TODAY's date, so on a closed day the backend rejected it with
// outside_operating_window. The resolver keys hours by candidate day instead.
describe("operatingWindowResolver (R2-11)", () => {
  const row = (
    day_of_week: number,
    open_time: string,
    close_time: string,
    is_closed = false,
  ) => ({ day_of_week, open_time, close_time, is_closed });

  it("returns each day's own window, and null for a closed day", () => {
    // 2026-07-18 is a Saturday (6), 2026-07-19 a Sunday (0).
    const resolve = operatingWindowResolver([
      row(6, "11:00:00", "15:00:00", true),
      row(0, "17:00:00", "23:00:00"),
    ]);
    expect(resolve).not.toBeNull();
    expect(resolve!("2026-07-18")).toBeNull();
    expect(resolve!("2026-07-19")).toEqual({
      openHHMM: "17:00",
      closeHHMM: "23:00",
    });
  });

  it("treats a day with no row as closed", () => {
    const resolve = operatingWindowResolver([row(0, "17:00", "23:00")]);
    expect(resolve!("2026-07-18")).toBeNull();
  });

  it("treats blank open/close times as closed", () => {
    const resolve = operatingWindowResolver([
      row(6, "", ""),
      row(0, "17:00", "23:00"),
    ]);
    expect(resolve!("2026-07-18")).toBeNull();
  });

  it("returns null when nothing is bookable so callers skip the clamp", () => {
    expect(operatingWindowResolver([])).toBeNull();
    expect(
      operatingWindowResolver([row(6, "11:00", "15:00", true), row(0, "", "")]),
    ).toBeNull();
  });
});

describe("nextBookableSlotLocal", () => {
  // 2026-06-10 14:03 local
  const now = new Date(2026, 5, 10, 14, 3, 0);

  it("rounds now + min advance up to the next slot interval", () => {
    // 14:03 + 30min advance = 14:33 → next 30-min boundary = 15:00
    expect(nextBookableSlotLocal(now, 30, 30)).toBe("2026-06-10T15:00");
  });

  it("keeps an exact boundary instead of skipping a slot", () => {
    const onBoundary = new Date(2026, 5, 10, 14, 0, 0);
    // 14:00 + 30 = 14:30, already on a boundary
    expect(nextBookableSlotLocal(onBoundary, 30, 30)).toBe("2026-06-10T14:30");
  });

  it("handles zero min advance by rounding up to the next interval", () => {
    // 14:03 → next 15-min boundary = 14:15
    expect(nextBookableSlotLocal(now, 0, 15)).toBe("2026-06-10T14:15");
  });

  it("defaults a missing/absurd interval to 30 minutes", () => {
    expect(nextBookableSlotLocal(now, 0, 0)).toBe("2026-06-10T14:30");
  });

  it("rolls over to the next day when the advance window crosses midnight", () => {
    const lateNight = new Date(2026, 5, 10, 23, 50, 0);
    expect(nextBookableSlotLocal(lateNight, 30, 30)).toBe("2026-06-11T00:30");
  });
});

describe("clampPartySize", () => {
  it("clamps below the business minimum", () => {
    expect(clampPartySize(1, 2, 12)).toBe(2);
  });

  it("clamps above the business maximum", () => {
    expect(clampPartySize(50, 2, 12)).toBe(12);
  });

  it("passes through in-range values", () => {
    expect(clampPartySize(6, 2, 12)).toBe(6);
  });

  it("survives missing settings with sane defaults", () => {
    expect(clampPartySize(0)).toBe(1);
    expect(clampPartySize(7)).toBe(7);
  });
});

describe("isRawValidatorDump", () => {
  it("detects gin Key:/Error:Field validation dumps", () => {
    expect(
      isRawValidatorDump(
        "Key: 'CreateReservationInput.CustomerName' Error:Field validation for 'CustomerName' failed on the 'required' tag",
      ),
    ).toBe(true);
  });

  it("detects binding tag fragments", () => {
    expect(isRawValidatorDump(`binding:"required"`)).toBe(true);
  });

  it("leaves human product copy alone", () => {
    expect(
      isRawValidatorDump(
        "reservations must be made at least 30 minutes in advance",
      ),
    ).toBe(false);
    expect(isRawValidatorDump("Customer name is required.")).toBe(false);
  });
});

describe("reservationErrorMessage", () => {
  const fallback = "Failed to create reservation";

  const axiosLikeError = (status: number, error?: string) => {
    const err = new Error(error || "Request failed") as Error & {
      isAxiosError: boolean;
      response: { status: number; data: { error?: string } };
    };
    err.isAxiosError = true;
    err.response = { status, data: { error } };
    return err;
  };

  it("surfaces the backend validation reason on 400", () => {
    const error = axiosLikeError(
      400,
      "reservations must be made at least 30 minutes in advance",
    );
    expect(reservationErrorMessage(error, fallback)).toBe(
      "reservations must be made at least 30 minutes in advance",
    );
  });

  it("surfaces conflict reasons on 409", () => {
    const error = axiosLikeError(409, "the selected time is no longer available");
    expect(reservationErrorMessage(error, fallback)).toBe(
      "the selected time is no longer available",
    );
  });

  it("does not surface raw gin validator dumps on 400 (FIND-030)", () => {
    const ginDump =
      "Key: 'CreateReservationInput.CustomerName' Error:Field validation for 'CustomerName' failed on the 'required' tag";
    const error = axiosLikeError(400, ginDump);
    const message = reservationErrorMessage(error, fallback);
    expect(message).not.toContain("Key:");
    expect(message).not.toContain("CreateReservationInput");
    expect(message).not.toContain("Field validation");
    expect(message.length).toBeGreaterThan(0);
  });

  it("falls back for server errors instead of echoing internals", () => {
    const error = axiosLikeError(500, "pq: relation does not exist");
    const message = reservationErrorMessage(error, fallback);
    expect(message).not.toContain("pq:");
    expect(message.length).toBeGreaterThan(0);
  });

  it("falls back for non-axios errors", () => {
    expect(reservationErrorMessage(new TypeError("boom"), fallback)).not.toBe(
      "boom",
    );
  });
});

describe("buildReservationUpdatePayload", () => {
  const reservation = {
    id: 7,
    business_id: 1,
    table_id: 4,
    customer_name: "Alice",
    customer_phone: "5551112222",
    customer_email: "alice@example.com",
    party_size: 2,
    reservation_time: "2026-06-12T22:00:00Z",
    duration: 90,
    status: "confirmed",
    special_requests: "Window",
    notes: "VIP",
    created_at: "",
    updated_at: "",
  } as unknown as Reservation;

  const baseForm = {
    table_id: 4 as number | null | undefined,
    customer_name: "Alice",
    customer_phone: "5551112222",
    customer_email: "alice@example.com",
    party_size: 2,
    // datetime-local string for the same instant, in this machine's zone
    reservation_time: ((): string => {
      const date = new Date("2026-06-12T22:00:00Z");
      const offset = date.getTimezoneOffset();
      return new Date(date.getTime() - offset * 60_000)
        .toISOString()
        .slice(0, 16);
    })(),
    duration: 90,
    special_requests: "Window",
    notes: "VIP",
  };

  it("sends nothing when nothing changed", () => {
    expect(buildReservationUpdatePayload(baseForm, reservation)).toEqual({});
  });

  it("sends only the fields that changed", () => {
    const payload = buildReservationUpdatePayload(
      { ...baseForm, customer_phone: "5559998888" },
      reservation,
    );
    expect(payload).toEqual({ customer_phone: "5559998888" });
  });

  it("converts a changed time to ISO and includes it", () => {
    const payload = buildReservationUpdatePayload(
      { ...baseForm, reservation_time: "2026-06-13T19:00" },
      reservation,
    );
    expect(payload.reservation_time).toBe(
      new Date("2026-06-13T19:00").toISOString(),
    );
    expect(Object.keys(payload)).toEqual(["reservation_time"]);
  });

  it("clears the table when it was assigned and the form unsets it", () => {
    const payload = buildReservationUpdatePayload(
      { ...baseForm, table_id: undefined },
      reservation,
    );
    expect(payload).toEqual({ clear_table: true });
  });

  it("reassigns the table when the form picks a different one", () => {
    const payload = buildReservationUpdatePayload(
      { ...baseForm, table_id: 9 },
      reservation,
    );
    expect(payload).toEqual({ table_id: 9 });
  });

  it("does not send clear_table when no table was assigned to begin with", () => {
    const unassigned = { ...reservation, table_id: null } as Reservation;
    const payload = buildReservationUpdatePayload(
      { ...baseForm, table_id: undefined },
      unassigned,
    );
    expect(payload).toEqual({});
  });
});
