import {
  kitchenBoardGate,
  kitchenEnabledFromBusiness,
} from "./kitchenEnabled";

describe("kitchenEnabledFromBusiness (#663 cold-load seed)", () => {
  it("returns null until both flags are present as booleans", () => {
    expect(kitchenEnabledFromBusiness(null)).toBeNull();
    expect(kitchenEnabledFromBusiness(undefined)).toBeNull();
    expect(kitchenEnabledFromBusiness({})).toBeNull();
    expect(
      kitchenEnabledFromBusiness({
        kitchen_enabled: true,
      }),
    ).toBeNull();
    expect(
      kitchenEnabledFromBusiness({
        orders_enabled: true,
      }),
    ).toBeNull();
  });

  it("only seeds ON — a false pair stays unknown so first paint cannot lie", () => {
    expect(
      kitchenEnabledFromBusiness({
        kitchen_enabled: true,
        orders_enabled: true,
      }),
    ).toBe(true);
    expect(
      kitchenEnabledFromBusiness({
        kitchen_enabled: true,
        orders_enabled: false,
      }),
    ).toBeNull();
    expect(
      kitchenEnabledFromBusiness({
        kitchen_enabled: false,
        orders_enabled: true,
      }),
    ).toBeNull();
    expect(
      kitchenEnabledFromBusiness({
        kitchen_enabled: false,
        orders_enabled: false,
      }),
    ).toBeNull();
  });
});

describe("kitchenBoardGate (#663 first-paint)", () => {
  it("treats null as loading, never as off", () => {
    expect(
      kitchenBoardGate({ kitchenEnabled: null, kitchenStatusLoading: true }),
    ).toBe("loading");
    expect(
      kitchenBoardGate({ kitchenEnabled: null, kitchenStatusLoading: false }),
    ).toBe("loading");
  });

  it("keeps a false flag as loading until kitchen-orders-status settles", () => {
    expect(
      kitchenBoardGate({ kitchenEnabled: false, kitchenStatusLoading: true }),
    ).toBe("loading");
  });

  it("trusts off only after the status fetch settles", () => {
    expect(
      kitchenBoardGate({ kitchenEnabled: false, kitchenStatusLoading: false }),
    ).toBe("off");
  });

  it("renders the live board when known-on, even while status is refetching", () => {
    expect(
      kitchenBoardGate({ kitchenEnabled: true, kitchenStatusLoading: true }),
    ).toBe("on");
    expect(
      kitchenBoardGate({ kitchenEnabled: true, kitchenStatusLoading: false }),
    ).toBe("on");
  });
});
