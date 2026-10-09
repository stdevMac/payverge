/** @jest-environment jsdom */
import { recalledVenueName, rememberOpenVenue } from "./openVenueMemory";

describe("openVenueMemory", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it("recalls the venue name only for the matching identifier", () => {
    rememberOpenVenue({
      id: 2,
      business_id: "demo-admin-1-ai-pro",
      name: "Payverge AI Pro Demo Lounge",
    });
    expect(recalledVenueName("demo-admin-1-ai-pro")).toBe(
      "Payverge AI Pro Demo Lounge",
    );
    expect(recalledVenueName("demo-admin-1-core")).toBeNull();
  });
});
