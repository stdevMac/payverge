/** @jest-environment jsdom */
/**
 * F4: the reservation details page wrapped its content in a bare
 * <GuestTranslationProvider> with no language, so it always rendered English
 * even when the guest booked in another locale. The page now honors a ?lang=
 * query param (carried by the booking email link), validated against the guest
 * locale set, so a French guest following their "View reservation" link lands
 * on a French page.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import ReservationDetailsPage from "./page";
import { guestReservationAPI } from "@/api/reservations";

const mockGet = jest.fn();
jest.mock("next/navigation", () => ({
  useParams: () => ({ confirmationCode: "ABC123" }),
  useSearchParams: () => ({ get: mockGet }),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getReservation: jest.fn(),
  },
}));

const detailsResponse = {
  reservation: {
    id: 7,
    customer_name: "Jean Dupont",
    party_size: 2,
    reservation_time: "2026-06-20T19:00:00.000Z",
    duration: 90,
    status: "confirmed",
    confirmation_code: "ABC123",
  },
  business_name: "Le Bistro",
  business_custom_url: "le-bistro",
  can_cancel: true,
};

describe("ReservationDetailsPage locale resolution (F4)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (guestReservationAPI.getReservation as jest.Mock).mockResolvedValue(
      detailsResponse,
    );
  });

  it("renders in French when ?lang=fr is present", async () => {
    mockGet.mockImplementation((k: string) => (k === "lang" ? "fr" : null));
    render(<ReservationDetailsPage />);
    // The success title comes from reservationConfirmation.successTitle, which
    // resolves to the French bundle value when the provider starts in fr.
    expect(await screen.findByText("Votre réservation")).toBeInTheDocument();
  });

  it("falls back to English when ?lang is missing or unsupported", async () => {
    mockGet.mockImplementation((k: string) =>
      k === "lang" ? "zz-not-a-locale" : null,
    );
    render(<ReservationDetailsPage />);
    await waitFor(() => {
      expect(screen.getByText("Your Reservation")).toBeInTheDocument();
    });
  });
});
