/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import ReservationManager from "../ReservationManager";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  // ReservationsSkeleton now routes its sr-only label through getTranslation
  // (common.loadingReservations). Resolve that one key to its English copy so
  // the "loading reservations" assertion still exercises the real wiring; any
  // other key falls through to its identifier (sufficient for this render gate).
  getTranslation: (key: string) =>
    key === "common.loadingReservations" ? "Loading reservations…" : key,
}));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    loading: false,
    access: null,
    error: null,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));
// Keep reservations "status" loading forever so the first-load gate stays open.
// `useReservationStatus` is re-exported from ./ReservationToggle (verified at
// ReservationManager.tsx:78), and ReservationToggle is the default export — keep
// it a render-safe stub so importing the named hook doesn't pull the real component.
jest.mock("../ReservationToggle", () => ({
  __esModule: true,
  default: () => null,
  useReservationStatus: () => ({
    enabled: false,
    loading: true,
    setEnabled: jest.fn(),
  }),
}));
// useAuth comes from the HybridAuthProvider (verified at ReservationManager.tsx:66).
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

describe("ReservationManager — first-load skeleton", () => {
  it("renders the ReservationsSkeleton content skeleton, not a bare spinner", () => {
    const { container } = render(<ReservationManager businessId={42} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText(/loading reservations/i)).toBeTruthy();
    expect(container.querySelector(".animate-spin")).toBeNull();
  });
});
