/** @jest-environment jsdom */
/**
 * #383: invalid public reservation links must render fully localized
 * recovery copy. The page used to prefer sanitizeError().message, which
 * inserted the English "The requested resource was not found." into an
 * otherwise Spanish heading + CTA.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import ReservationDetailsPage from "./page";
import { guestReservationAPI } from "@/api/reservations";

const mockGet = jest.fn();
jest.mock("next/navigation", () => ({
  useParams: () => ({ confirmationCode: "qa-invalid-reservation-code-r5" }),
  useSearchParams: () => ({ get: mockGet }),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getReservation: jest.fn(),
  },
}));

const RAW_ENGLISH_NOT_FOUND = "The requested resource was not found.";

function sanitizedNotFound() {
  return Object.assign(new Error(RAW_ENGLISH_NOT_FOUND), {
    status: 404,
    response: {
      status: 404,
      data: { error: "Reservation not found" },
    },
  });
}

describe("ReservationDetailsPage invalid-code recovery (#383)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (guestReservationAPI.getReservation as jest.Mock).mockRejectedValue(
      sanitizedNotFound(),
    );
  });

  it("renders English guest not-found copy, never the raw backend string", async () => {
    mockGet.mockImplementation((k: string) => (k === "lang" ? "en" : null));
    render(<ReservationDetailsPage />);

    expect(
      await screen.findByText("Unable to Load Reservation"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("We could not find this reservation."),
    ).toBeInTheDocument();
    expect(screen.getByText("Return Home")).toBeInTheDocument();
    expect(screen.queryByText(RAW_ENGLISH_NOT_FOUND)).not.toBeInTheDocument();
  });

  it("renders Spanish recovery copy for ?lang=es, never the English 404 string", async () => {
    mockGet.mockImplementation((k: string) => (k === "lang" ? "es" : null));
    render(<ReservationDetailsPage />);

    expect(
      await screen.findByText("No se pudo cargar la reserva"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("No pudimos encontrar esta reserva."),
    ).toBeInTheDocument();
    expect(screen.getByText("Volver al inicio")).toBeInTheDocument();
    expect(screen.queryByText(RAW_ENGLISH_NOT_FOUND)).not.toBeInTheDocument();
    expect(screen.queryByText("Reservation not found")).not.toBeInTheDocument();
  });

  it("renders localized transient copy for a 500, never the English backend string", async () => {
    (guestReservationAPI.getReservation as jest.Mock).mockRejectedValue(
      Object.assign(new Error("Something went wrong on our end. Please try again later."), {
        status: 500,
        response: { status: 500, data: { error: "internal server error" } },
      }),
    );
    mockGet.mockImplementation((k: string) => (k === "lang" ? "es" : null));
    render(<ReservationDetailsPage />);

    expect(
      await screen.findByText("No se pudo cargar la reserva"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Algo salió mal de nuestro lado. Por favor intenta más tarde",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("internal server error")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Something went wrong on our end. Please try again later."),
    ).not.toBeInTheDocument();
  });

  it("renders Spanish (Argentina) recovery copy for ?lang=es-AR", async () => {
    mockGet.mockImplementation((k: string) => (k === "lang" ? "es-AR" : null));
    render(<ReservationDetailsPage />);

    expect(
      await screen.findByText("No se pudo cargar la reserva"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("No pudimos encontrar esta reserva."),
    ).toBeInTheDocument();
    expect(screen.getByText("Volver al inicio")).toBeInTheDocument();
    expect(screen.queryByText(RAW_ENGLISH_NOT_FOUND)).not.toBeInTheDocument();
  });
});
