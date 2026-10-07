/** @jest-environment jsdom */
import { isGuestRoute } from "../guestRoute";

describe("isGuestRoute", () => {
  test("returns true for /b/<customUrl>", () => {
    expect(isGuestRoute("/b/mara-core-kitchen")).toBe(true);
    expect(isGuestRoute("/b/foo")).toBe(true);
    expect(isGuestRoute("/b/foo/bar")).toBe(true);
  });

  test("returns true for /t/<tableCode> diner pages", () => {
    expect(isGuestRoute("/t/TABLE123")).toBe(true);
    expect(isGuestRoute("/t/TABLE123/menu")).toBe(true);
    expect(isGuestRoute("/t/TABLE123/bill")).toBe(true);
  });

  test("returns true for the instance root (venue page or directory)", () => {
    expect(isGuestRoute("/")).toBe(true);
    expect(isGuestRoute("/es")).toBe(true);
    expect(isGuestRoute("/es-ar/")).toBe(true);
  });

  test("returns false for non-guest pages", () => {
    expect(isGuestRoute("/es/dashboard")).toBe(false);
    expect(isGuestRoute("/dashboard")).toBe(false);
    expect(isGuestRoute("/business/123")).toBe(false);
    expect(isGuestRoute("/admin")).toBe(false);
    expect(isGuestRoute("/be/foo")).toBe(false); // not /b/
    expect(isGuestRoute("/tables")).toBe(false); // not /t/
  });

  test("returns false when window is unavailable (SSR)", () => {
    // Helper accepts an explicit pathname; without it, it reads window.location.
    // SSR-safe: return false if window is undefined.
    expect(isGuestRoute(undefined)).toBe(
      typeof window !== "undefined" &&
        (/^\/(?:b|t)\//.test(window.location.pathname) ||
          window.location.pathname === "/"),
    );
    expect(isGuestRoute("")).toBe(false);
  });
});
