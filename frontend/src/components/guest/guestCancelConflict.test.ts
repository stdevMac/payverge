import {
  guestCancelConflictFromError,
  guestCancelConflictKey,
} from "./guestCancelConflict";

describe("guestCancelConflictKey (#528)", () => {
  it("maps kitchen-accepted 409 to alreadyAccepted", () => {
    expect(
      guestCancelConflictKey(409, "order_already_accepted", "This order has already been accepted by the kitchen"),
    ).toBe("orders.alreadyAccepted");
  });

  it("maps accepted English body without a code", () => {
    expect(
      guestCancelConflictKey(409, undefined, "This order has already been accepted by the kitchen"),
    ).toBe("orders.alreadyAccepted");
  });

  it("maps cancelled 409 to alreadyCancelled", () => {
    expect(
      guestCancelConflictKey(409, "order_already_cancelled", "This order has already been cancelled"),
    ).toBe("orders.alreadyCancelled");
  });

  it("maps generic 409 to alreadyCancelled (legacy)", () => {
    expect(guestCancelConflictKey(409, undefined, "conflict")).toBe("orders.alreadyCancelled");
  });

  it("maps non-409 to cancelFailed", () => {
    expect(guestCancelConflictKey(500, undefined, "boom")).toBe("orders.cancelFailed");
  });
});

describe("guestCancelConflictFromError (#528)", () => {
  it("reads a sanitized axios 409 with nested backend code", () => {
    const err = Object.assign(new Error("request failed"), {
      status: 409,
      code: "order_already_accepted",
      response: {
        status: 409,
        data: {
          error: "This order has already been accepted by the kitchen",
          code: "order_already_accepted",
        },
      },
    });
    expect(guestCancelConflictFromError(err)).toBe("orders.alreadyAccepted");
  });

  it("reads a top-level backend code when response.data is missing", () => {
    const err = Object.assign(
      new Error("This action conflicts with existing data. Please refresh and try again."),
      {
        status: 409,
        code: "order_already_accepted",
      },
    );
    expect(guestCancelConflictFromError(err)).toBe("orders.alreadyAccepted");
  });

  it("ignores axios transport codes on a 409", () => {
    const err = Object.assign(new Error("Request failed with status code 409"), {
      status: 409,
      code: "ERR_BAD_REQUEST",
    });
    expect(guestCancelConflictFromError(err)).toBe("orders.alreadyCancelled");
  });

  it("maps an already-cancelled envelope", () => {
    const err = Object.assign(new Error("request failed"), {
      status: 409,
      response: {
        status: 409,
        data: {
          error: "This order has already been cancelled",
          code: "order_already_cancelled",
        },
      },
    });
    expect(guestCancelConflictFromError(err)).toBe("orders.alreadyCancelled");
  });

  it("maps a non-409 throw to cancelFailed", () => {
    expect(guestCancelConflictFromError(new Error("boom"))).toBe(
      "orders.cancelFailed",
    );
  });
});
