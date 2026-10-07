import {
  isBusinessClosedFromOrderability,
  isGuestOrderingEnabled,
} from "./guestBusinessClosed";

describe("isBusinessClosedFromOrderability", () => {
  it("returns false for empty or missing maps", () => {
    expect(isBusinessClosedFromOrderability(undefined)).toBe(false);
    expect(isBusinessClosedFromOrderability(null)).toBe(false);
    expect(isBusinessClosedFromOrderability({})).toBe(false);
  });

  it("returns true when every item is business_closed", () => {
    expect(
      isBusinessClosedFromOrderability({
        a: { state: "business_closed", orderable: false },
        b: { state: "business_closed", orderable: false },
      }),
    ).toBe(true);
  });

  it("returns true when any item is business_closed (even with available peers)", () => {
    // When open, BE never emits business_closed — so any is safe.
    expect(
      isBusinessClosedFromOrderability({
        a: { state: "business_closed", orderable: false },
        b: { state: "available", orderable: true },
      }),
    ).toBe(true);
  });

  it("returns false for inventory_out / ordering_disabled only (not hours)", () => {
    expect(
      isBusinessClosedFromOrderability({
        a: { state: "inventory_out", orderable: false },
        b: { state: "inventory_out", orderable: false },
      }),
    ).toBe(false);
    expect(
      isBusinessClosedFromOrderability({
        a: { state: "ordering_disabled", orderable: false },
      }),
    ).toBe(false);
    expect(
      isBusinessClosedFromOrderability({
        a: { state: "inventory_warning", orderable: true },
        b: { state: "available", orderable: true },
      }),
    ).toBe(false);
  });

  it("ignores nullish map entries", () => {
    expect(
      isBusinessClosedFromOrderability({
        a: { state: "business_closed", orderable: false },
        b: undefined,
      }),
    ).toBe(true);
  });
});

describe("isGuestOrderingEnabled", () => {
  it("requires kitchen + orders and not closed", () => {
    expect(
      isGuestOrderingEnabled({
        kitchenEnabled: true,
        ordersEnabled: true,
        businessClosed: false,
      }),
    ).toBe(true);
    expect(
      isGuestOrderingEnabled({
        kitchenEnabled: true,
        ordersEnabled: true,
        businessClosed: true,
      }),
    ).toBe(false);
    expect(
      isGuestOrderingEnabled({
        kitchenEnabled: true,
        ordersEnabled: false,
        businessClosed: false,
      }),
    ).toBe(false);
    expect(
      isGuestOrderingEnabled({
        kitchenEnabled: false,
        ordersEnabled: true,
        businessClosed: false,
      }),
    ).toBe(false);
  });
});
