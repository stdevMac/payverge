/** @jest-environment jsdom */
/**
 * F4: the reservation action page wrapped its content in a bare
 * <GuestTranslationProvider> with no language, so it always rendered English.
 * It now honors a ?lang= query param (carried by the booking email link) and,
 * after the action resolves, adopts the booking's business so the guest's saved
 * per-business language preference is also respected.
 *
 * Guest self-confirmation was removed (approval belongs to the business):
 * the only supported action is "cancel"; stale "confirm" links redirect to the
 * read-only details page.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ReservationActionPage from "./page";
import { guestReservationAPI } from "@/api/reservations";

const mockGet = jest.fn();
const mockReplace = jest.fn();
let mockAction = "cancel";
jest.mock("next/navigation", () => ({
  useParams: () => ({ confirmationCode: "ABC123", action: mockAction }),
  useSearchParams: () => ({ get: mockGet }),
  useRouter: () => ({ replace: mockReplace }),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    cancelReservation: jest.fn(),
  },
}));

const actionResponse = {
  message: "",
  reservation: {
    id: 9,
    business_id: 42,
    customer_name: "Jean Dupont",
    party_size: 2,
    reservation_time: "2026-06-20T19:00:00.000Z",
    duration: 90,
    status: "cancelled",
    confirmation_code: "ABC123",
    created_at: new Date().toISOString(),
  },
  business_name: "Le Bistro",
  business_custom_url: "le-bistro",
};

describe("ReservationActionPage locale resolution (F4)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockAction = "cancel";
    (guestReservationAPI.cancelReservation as jest.Mock).mockResolvedValue(
      actionResponse,
    );
  });

  it("renders the cancel prompt in French when ?lang=fr is present", async () => {
    mockGet.mockImplementation((k: string) => (k === "lang" ? "fr" : null));
    render(<ReservationActionPage />);
    // cancelCta -> French bundle value when provider starts in fr.
    expect(
      await screen.findByText("Annuler la réservation"),
    ).toBeInTheDocument();
  });

  it("PG-12: honors payverge_guest_locale cookie when ?lang= is absent", async () => {
    document.cookie = "payverge_guest_locale=fr";
    mockGet.mockImplementation(() => null);
    render(<ReservationActionPage />);
    expect(
      await screen.findByText("Annuler la réservation"),
    ).toBeInTheDocument();
    document.cookie = "payverge_guest_locale=; Max-Age=0; Path=/";
  });

  it("adopts the booking's business after the action resolves (F4)", async () => {
    // Saved preference for business 42 is Spanish; with no ?lang= the page
    // should switch to it once the action returns business_id 42.
    localStorage.setItem("guest-language-42", "es");
    mockGet.mockImplementation(() => null);

    render(<ReservationActionPage />);
    const cancelBtn = await screen.findByText("Cancel Reservation");
    fireEvent.click(cancelBtn);

    await waitFor(() => {
      // cancelledTitle in Spanish proves the saved business preference was
      // adopted via setBusinessId(reservation.business_id).
      expect(screen.getByText("Reserva cancelada")).toBeInTheDocument();
    });
  });

  it("redirects legacy confirm links to the read-only details page", async () => {
    mockAction = "confirm";
    mockGet.mockImplementation((k: string) => (k === "lang" ? "fr" : null));

    render(<ReservationActionPage />);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/reservations/ABC123?lang=fr");
    });
    expect(guestReservationAPI.cancelReservation).not.toHaveBeenCalled();
  });
});
