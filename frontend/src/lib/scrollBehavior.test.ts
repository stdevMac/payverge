/** @jest-environment jsdom */
import { preferredScrollBehavior } from "./scrollBehavior";

describe("preferredScrollBehavior", () => {
  const originalMatchMedia = window.matchMedia;

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  it("returns auto when the user prefers reduced motion", () => {
    window.matchMedia = jest.fn().mockImplementation((query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      media: query,
    })) as unknown as typeof window.matchMedia;

    expect(preferredScrollBehavior()).toBe("auto");
  });

  it("returns smooth when the user has no reduced-motion preference", () => {
    window.matchMedia = jest.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
    })) as unknown as typeof window.matchMedia;

    expect(preferredScrollBehavior()).toBe("smooth");
  });

  it("returns smooth when matchMedia is missing", () => {
    // jest.setup defines matchMedia as writable but not configurable.
    window.matchMedia = undefined as unknown as typeof window.matchMedia;
    expect(preferredScrollBehavior()).toBe("smooth");
  });
});
