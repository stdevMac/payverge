import { isGuestRoute } from "./guestRoute";

describe("isGuestRoute", () => {
  it("matches storefront and table routes", () => {
    expect(isGuestRoute("/b/acme-cafe")).toBe(true);
    expect(isGuestRoute("/t/ABC123")).toBe(true);
    expect(isGuestRoute("/t/ABC123/menu")).toBe(true);
  });

  it("matches delivery tracking and reservation deep links", () => {
    // Regression: these guest routes were missing, so a 401 there triggered a
    // wasted auth refresh + apiCache.clear + auth:session-expired event.
    expect(isGuestRoute("/delivery/DLV-42/track")).toBe(true);
    expect(isGuestRoute("/reservations/CONF-9/confirm")).toBe(true);
    expect(isGuestRoute("/reservations/CONF-9")).toBe(true);
  });

  it("does not match operator / marketing routes", () => {
    expect(isGuestRoute("/dashboard")).toBe(false);
    expect(isGuestRoute("/business/1/dashboard")).toBe(false);
    // Guard against accidental prefix bleed.
    expect(isGuestRoute("/blog")).toBe(false);
  });
});
