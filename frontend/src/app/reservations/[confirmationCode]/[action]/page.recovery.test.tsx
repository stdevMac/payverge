/** @jest-environment jsdom */
/**
 * Error-state recovery: a failed cancel must not dead-end. The error card
 * offers Retry (re-runs the action) and a View Reservation link built from
 * the URL confirmation code — no loaded reservation object required.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ReservationActionPage from "./page";
import { guestReservationAPI } from "@/api/reservations";

const mockGet = jest.fn();
const mockReplace = jest.fn();
jest.mock("next/navigation", () => ({
  useParams: () => ({ confirmationCode: "ABC123", action: "cancel" }),
  useSearchParams: () => ({ get: mockGet }),
  useRouter: () => ({ replace: mockReplace }),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    cancelReservation: jest.fn(),
  },
}));

describe("ReservationActionPage error recovery", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockGet.mockImplementation(() => null);
  });

  it("offers retry and a reservation link when the cancel call fails", async () => {
    (guestReservationAPI.cancelReservation as jest.Mock).mockRejectedValueOnce(
      new Error("network down"),
    );

    render(<ReservationActionPage />);
    fireEvent.click(await screen.findByText("Cancel Reservation"));

    // Error state reached; retry CTA present (guest key bill.retry).
    const retry = await screen.findByText("Try Again");

    // View Reservation link works WITHOUT a loaded reservation object.
    // NextUI Button as={Link} renders <a role="button">, so query by text.
    const viewLink = screen.getByText("View Reservation").closest("a");
    expect(viewLink).toHaveAttribute("href", "/reservations/ABC123?lang=en");

    // Retry actually re-runs the action.
    (guestReservationAPI.cancelReservation as jest.Mock).mockRejectedValueOnce(
      new Error("still down"),
    );
    fireEvent.click(retry);
    await waitFor(() => {
      expect(guestReservationAPI.cancelReservation).toHaveBeenCalledTimes(2);
    });
  });
});
