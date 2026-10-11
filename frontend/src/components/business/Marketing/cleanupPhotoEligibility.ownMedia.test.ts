/** @jest-environment jsdom */
import { isCleanupEligiblePhotoUrl } from "./cleanupPhotoEligibility";

describe("isCleanupEligiblePhotoUrl on the default storage driver", () => {
  it("accepts relative /media uploads (backend reads them from its own store)", () => {
    expect(
      isCleanupEligiblePhotoUrl(
        "/media/businesses/1/menu_items/0123456789abcdef_dish.png",
      ),
    ).toBe(true);
  });

  it("accepts ${PUBLIC_URL}/media uploads on this origin even over http", () => {
    expect(
      isCleanupEligiblePhotoUrl(
        `${window.location.origin}/media/businesses/1/a.png`,
      ),
    ).toBe(true);
  });

  it("still rejects other relative paths and traversal", () => {
    expect(isCleanupEligiblePhotoUrl("/api/v1/inside/me")).toBe(false);
    expect(isCleanupEligiblePhotoUrl("/media/../api/v1/x")).toBe(false);
  });
});
