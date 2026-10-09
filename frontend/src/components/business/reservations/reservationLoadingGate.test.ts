/** @jest-environment node */
import {
  shouldShowReservationsRefreshing,
  shouldShowReservationsSkeleton,
} from "./reservationLoadingGate";

describe("reservationLoadingGate", () => {
  it("shows skeleton until the active view's data source first loads", () => {
    // Never loaded: skeleton regardless of whether the load effect has
    // already flipped `loading` (covers the pre-effect frame on view switch).
    expect(shouldShowReservationsSkeleton(false)).toBe(true);
    // Active source loaded at least once: never fall back to the skeleton.
    expect(shouldShowReservationsSkeleton(true)).toBe(false);
  });

  it("shows a light refreshing indicator only after the active source has loaded", () => {
    expect(shouldShowReservationsRefreshing(true, true)).toBe(true);
    expect(shouldShowReservationsRefreshing(true, false)).toBe(false);
    expect(shouldShowReservationsRefreshing(false, true)).toBe(false);
  });
});
