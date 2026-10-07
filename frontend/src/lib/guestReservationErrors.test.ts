import {
  presentGuestReservationCancelError,
  presentGuestReservationCancelSuccess,
  presentGuestReservationLoadError,
} from "./guestReservationErrors";

function sanitizedHttpError(
  status: number,
  message: string,
  data: Record<string, unknown> = {},
) {
  return Object.assign(new Error(message), {
    status,
    response: { status, data },
  });
}

describe("presentGuestReservationCancelSuccess", () => {
  it("maps already-cancelled English message to a guest key", () => {
    expect(
      presentGuestReservationCancelSuccess({
        message: "Reservation was already cancelled",
      }),
    ).toBe("reservationConfirmation.action.alreadyCancelledBody");
  });

  it("defaults success to cancelledBody without raw message preference", () => {
    expect(presentGuestReservationCancelSuccess({ message: "" })).toBe(
      "reservationConfirmation.action.cancelledBody",
    );
  });

  it("honors stable codes when present", () => {
    expect(
      presentGuestReservationCancelSuccess({ code: "reservation_cancelled" }),
    ).toBe("reservationConfirmation.action.cancelledBody");
  });
});

describe("presentGuestReservationCancelError", () => {
  it("maps never-open cancel copy to windowNeverOpenBody", () => {
    expect(
      presentGuestReservationCancelError(
        Object.assign(new Error("this reservation cannot be cancelled online"), {
          response: {
            data: {
              error: "this reservation cannot be cancelled online",
              code: "reservation_cancel_never_open",
            },
          },
        }),
      ),
    ).toBe("reservationConfirmation.action.windowNeverOpenBody");
  });

  it("maps known English error strings to guest keys", () => {
    expect(
      presentGuestReservationCancelError(
        Object.assign(new Error("cancellation window has closed"), {
          response: {
            data: { error: "cancellation window has closed" },
          },
        }),
      ),
    ).toBe("reservationConfirmation.action.windowClosedBody");
  });

  it("falls back to generic cancel error for unknown prose", () => {
    expect(
      presentGuestReservationCancelError(new Error("something exploded")),
    ).toBe("reservationConfirmation.action.cancelErrorBody");
  });
});

describe("presentGuestReservationLoadError (#383)", () => {
  it("maps a sanitized 404 to the guest not-found key, not English prose", () => {
    expect(
      presentGuestReservationLoadError(
        sanitizedHttpError(404, "The requested resource was not found.", {
          error: "Reservation not found",
        }),
      ),
    ).toBe("reservationConfirmation.notFoundFallback");
  });

  it("maps reservation_not_found / not-found English strings to the same key", () => {
    expect(
      presentGuestReservationLoadError(
        sanitizedHttpError(404, "The requested resource was not found.", {
          code: "reservation_not_found",
        }),
      ),
    ).toBe("reservationConfirmation.notFoundFallback");
    expect(
      presentGuestReservationLoadError(
        new Error("The requested resource was not found."),
      ),
    ).toBe("reservationConfirmation.notFoundFallback");
  });

  it("maps network failures to the guest network key", () => {
    expect(
      presentGuestReservationLoadError(
        Object.assign(new Error("Network Error"), { code: "ERR_NETWORK" }),
      ),
    ).toBe("errors.networkErrorDescription");
  });

  it("maps 5xx / unknown failures to the guest server key, never raw English", () => {
    expect(
      presentGuestReservationLoadError(
        sanitizedHttpError(500, "Something went wrong on our end. Please try again later."),
      ),
    ).toBe("errors.serverErrorDescription");
    expect(
      presentGuestReservationLoadError(new Error("dial tcp: connection refused")),
    ).toBe("errors.serverErrorDescription");
  });
});
