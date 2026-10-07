import { shouldReplaceDashboardWithFetchError } from "../dashboardFetchErrorPolicy";

describe("shouldReplaceDashboardWithFetchError (#773)", () => {
  it("keeps the last-good shell on a generic business refetch flake", () => {
    expect(
      shouldReplaceDashboardWithFetchError({
        error: "business:generic",
        hasLastGoodBusiness: true,
        isAuthError: false,
        isForbiddenError: false,
      }),
    ).toBe(false);
  });

  it("shows the full-page retry when there is no last-good business", () => {
    expect(
      shouldReplaceDashboardWithFetchError({
        error: "business:generic",
        hasLastGoodBusiness: false,
        isAuthError: false,
        isForbiddenError: false,
      }),
    ).toBe(true);
  });

  it("still replaces the shell on auth and forbidden errors", () => {
    expect(
      shouldReplaceDashboardWithFetchError({
        error: "auth:signin-required",
        hasLastGoodBusiness: true,
        isAuthError: true,
        isForbiddenError: false,
      }),
    ).toBe(true);
    expect(
      shouldReplaceDashboardWithFetchError({
        error: "business:forbidden",
        hasLastGoodBusiness: true,
        isAuthError: false,
        isForbiddenError: true,
      }),
    ).toBe(true);
  });

  it("does not replace when there is no error", () => {
    expect(
      shouldReplaceDashboardWithFetchError({
        error: null,
        hasLastGoodBusiness: false,
        isAuthError: false,
        isForbiddenError: false,
      }),
    ).toBe(false);
  });
});
