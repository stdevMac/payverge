/** @jest-environment jsdom */
/**
 * #886 — a public booking rejected by the backend must surface localized copy.
 * The backend now returns `{ error, code: "reservation_email_required" }`; the
 * storefront form must translate the code in the guest's language instead of
 * toasting the raw English sentence.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import toast from "react-hot-toast";
import GuestReservationForm from "../GuestReservationForm";
import { guestReservationAPI } from "@/api/reservations";

const guestLocale = { current: "es-AR" };

// Only `t` is stubbed (last key segment) — `currentLanguage` is real input to
// the localization hook under test.
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      let out = key.split(".").pop() as string;
      if (params) {
        for (const [name, value] of Object.entries(params)) {
          out = out.replace(
            new RegExp(`\\{\\{?${name}\\}?\\}`, "g"),
            String(value),
          );
        }
      }
      return out;
    },
    currentLanguage: guestLocale.current,
  }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn(),
    getAvailability: jest.fn(),
    createReservation: jest.fn(),
  },
}));

const WAIT = { timeout: 15_000 } as const;

const settings = {
  enabled: true,
  min_party_size: 1,
  max_party_size: 12,
  max_advance_days: 30,
  min_advance_minutes: 0,
  approval_mode: "auto",
  allow_cancellation: true,
  allow_waitlist: false,
  default_duration: 60,
  cancellation_deadline: 24,
  external_partner_links: [],
};

function isoTomorrowAt(hours: number): string {
  const date = new Date();
  date.setDate(date.getDate() + 1);
  date.setHours(hours, 0, 0, 0);
  return date.toISOString();
}

// The shape axiosInstance rejects with (sanitized plain Error carrying the
// backend envelope on `response.data`).
function codedApiError(code: string, message: string): unknown {
  const err = new Error("Request failed with status code 400") as Error & {
    status?: number;
    response?: { status: number; data: { error: string; code: string } };
  };
  err.status = 400;
  err.response = { status: 400, data: { error: message, code } };
  return err;
}

async function renderAndSubmit() {
  const view = render(
    <GuestReservationForm
      customUrl="parrilla-quebracho-azul"
      businessName="Parrilla Quebracho Azul"
      timezone="America/Argentina/Buenos_Aires"
    />,
  );

  fireEvent.click(await screen.findByRole("button", { name: "quickTomorrow" }, WAIT));

  const slot = await screen.findByRole(
    "button",
    { name: /19:00|7:00/ },
    WAIT,
  );
  fireEvent.click(slot);

  fireEvent.change(screen.getByLabelText(/fullName/), {
    target: { value: "Federico" },
  });
  fireEvent.change(screen.getByLabelText(/phone/), {
    target: { value: "+5491122334455" },
  });
  fireEvent.change(screen.getByLabelText(/email/), {
    target: { value: "federico@example.com" },
  });

  fireEvent.click(screen.getByRole("button", { name: "submit" }));
  return view;
}

describe("GuestReservationForm coded backend errors", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    guestLocale.current = "es-AR";
    (guestReservationAPI.getSettings as jest.Mock).mockResolvedValue(settings);
    (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue({
      available_slots: [
        {
          time: isoTomorrowAt(19),
          available_tables: 3,
          recommended: true,
          reason_code: "available",
        },
      ],
      waitlist_available: false,
      next_available_slot: isoTomorrowAt(19),
    });
  });

  it("localizes reservation_email_required in the guest's language", async () => {
    (guestReservationAPI.createReservation as jest.Mock).mockRejectedValue(
      codedApiError(
        "reservation_email_required",
        "a valid email address is required to book online",
      ),
    );

    await renderAndSubmit();

    await waitFor(
      () => {
        expect(toast.error).toHaveBeenCalledWith(
          "Ingresá un email válido: te vamos a enviar la confirmación ahí.",
        );
      },
      WAIT,
    );
  });

  it("falls back to the backend string when the error carries no code", async () => {
    (guestReservationAPI.createReservation as jest.Mock).mockRejectedValue(
      codedApiError("", "something specific but uncoded"),
    );

    await renderAndSubmit();

    await waitFor(
      () => {
        expect(toast.error).toHaveBeenCalledWith(
          "something specific but uncoded",
        );
      },
      WAIT,
    );
  });
});
