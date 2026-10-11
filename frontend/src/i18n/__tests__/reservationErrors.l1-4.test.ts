/**
 * D1 / L1-4: reservation domain codes must resolve to localized product copy
 * (not only backend English envelopes). Spanish operators get Spanish past-time
 * errors when the API returns code=reservation_in_past.
 */
import { translateApiError } from "@/i18n/apiErrors";

describe("L1-4 reservation error codes localize (D1)", () => {
  it("maps reservation_in_past to Spanish product copy", () => {
    const msg = translateApiError(
      {
        code: "reservation_in_past",
        error: "reservation time is in the past",
      },
      "es",
    );
    expect(msg).toMatch(/pasado|reserva/i);
    expect(msg).not.toBe("reservation time is in the past");
    expect(msg.toLowerCase()).not.toMatch(/^reservation time is in the past$/);
  });

  it("maps reservation_min_advance in Spanish", () => {
    const msg = translateApiError(
      {
        code: "reservation_min_advance",
        error: "selected time is too soon to book",
      },
      "es",
    );
    expect(msg).toMatch(/pronto|hora|horario/i);
    expect(msg).not.toMatch(/too soon to book/i);
  });

  it("keeps English product copy for en", () => {
    const msg = translateApiError(
      {
        code: "reservation_in_past",
        error: "reservation time is in the past",
      },
      "en",
    );
    expect(msg.toLowerCase()).toMatch(/past/);
  });
});
