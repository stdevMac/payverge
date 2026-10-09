import {
  shouldPollStorefrontMenu,
  storefrontMenuRefetchInterval,
  storefrontMenuRefreshOptions,
} from "./useBusinessPageData";

describe("shouldPollStorefrontMenu", () => {
  it("polls only the visible menu surface", () => {
    expect(shouldPollStorefrontMenu("menu")).toBe(true);
    expect(shouldPollStorefrontMenu("about")).toBe(false);
    expect(shouldPollStorefrontMenu("reservations")).toBe(false);
    expect(shouldPollStorefrontMenu("contact")).toBe(false);
    expect(shouldPollStorefrontMenu(null)).toBe(false);
  });

  it("polls delivery only when a cart still needs the menu", () => {
    expect(shouldPollStorefrontMenu("delivery")).toBe(false);
    expect(shouldPollStorefrontMenu("delivery", true)).toBe(true);
    expect(shouldPollStorefrontMenu("about", true)).toBe(false);
  });
});

describe("storefrontMenuRefetchInterval", () => {
  it("disables the 60s interval when the menu is not on screen", () => {
    expect(storefrontMenuRefetchInterval(false)).toBe(false);
    expect(storefrontMenuRefetchInterval(true)).toBe(60_000);
  });
});

describe("storefrontMenuRefreshOptions", () => {
  it("refetches on the 60s interval and on focus while the menu is polling", () => {
    expect(storefrontMenuRefreshOptions(true)).toEqual({
      refetchInterval: 60_000,
      refetchOnWindowFocus: true,
      refetchIntervalInBackground: false,
    });
  });

  it("disables the interval and focus refetch when the menu is not polling", () => {
    expect(storefrontMenuRefreshOptions(false)).toEqual({
      refetchInterval: false,
      refetchOnWindowFocus: false,
      refetchIntervalInBackground: false,
    });
  });
});
