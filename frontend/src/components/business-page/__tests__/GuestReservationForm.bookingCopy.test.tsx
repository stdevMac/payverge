/** @jest-environment jsdom */
/**
 * #948 — book-a-table copy truths on the public storefront:
 *  (a) the quick-date chips and the service-period headings are separate
 *      controls and must never share a label (es/es-AR both said "Mañana");
 *  (b) es-AR is a voseo surface — the submit CTA must match the heading
 *      ("Reservá", not the neutral "Reservar");
 *  (c) an approval_mode=auto venue must never show confirmation-required
 *      chrome.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";
import { guestReservationAPI } from "@/api/reservations";
import fs from "fs";
import path from "path";
import esMessages from "@/i18n/guest-messages/es.json";
import esArMessages from "@/i18n/guest-messages/es-AR.json";

const QUICK_DATE_KEYS = ["quickToday", "quickTomorrow", "quickWeekend"] as const;
const PERIOD_KEYS = [
  "periodEarly",
  "periodLunch",
  "periodDinner",
  "periodLate",
] as const;

type ReservationCopy = Record<string, string>;

function reservationCopy(bundle: unknown): ReservationCopy {
  return (bundle as any).businessPage.info.reservationForm as ReservationCopy;
}

// Read every shipped storefront bundle from disk so a new locale is covered
// the day it lands.
const GUEST_MESSAGES_DIR = path.join(__dirname, "../../../i18n/guest-messages");

function allGuestBundles(): Array<[string, unknown]> {
  return fs
    .readdirSync(GUEST_MESSAGES_DIR)
    .filter((file) => file.endsWith(".json") && !file.startsWith("."))
    .map((file) => [
      file.replace(/\.json$/, ""),
      JSON.parse(
        fs.readFileSync(path.join(GUEST_MESSAGES_DIR, file), "utf-8"),
      ) as unknown,
    ]);
}

describe("#948 book-a-table copy", () => {
  it("never gives a quick-date chip the same label as a service period", () => {
    const bundles = allGuestBundles();
    expect(bundles.length).toBeGreaterThanOrEqual(21);
    for (const [locale, bundle] of bundles) {
      const copy = reservationCopy(bundle);
      for (const dateKey of QUICK_DATE_KEYS) {
        for (const periodKey of PERIOD_KEYS) {
          expect({
            locale,
            dateKey,
            periodKey,
            collision: copy[dateKey] === copy[periodKey],
          }).toEqual({ locale, dateKey, periodKey, collision: false });
        }
      }
    }
  });

  it("keeps the es-AR booking verbs in voseo, matching the page heading", () => {
    const esAr = reservationCopy(esArMessages);
    expect(esAr.title).toContain("Reservá");
    expect(esAr.submit).toBe("Reservá");
    expect(esAr.slotReasonBookAhead).toMatch(/^Reservá /);
    // The neutral base keeps its tuteo/infinitive wording.
    const es = reservationCopy(esMessages);
    expect(es.submit).toBe("Reservar");
    expect(es.slotReasonBookAhead).toMatch(/^Reserva /);
  });
});

// ---------------------------------------------------------------------------
// (c) approval_mode=auto must not render confirmation-required chrome.
// ---------------------------------------------------------------------------

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key.split(".").pop() as string,
    currentLanguage: "es-AR",
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

const autoSettings = {
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

describe("#948(c) approval-mode chrome", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue({
      available_slots: [],
      waitlist_available: false,
      next_available_slot: null,
    });
  });

  it("shows instant-confirmation policy copy on an auto-approval venue", async () => {
    (guestReservationAPI.getSettings as jest.Mock).mockResolvedValue(
      autoSettings,
    );
    render(
      <GuestReservationForm
        customUrl="parrilla-quebracho-azul"
        businessName="Parrilla Quebracho Azul"
        timezone="America/Argentina/Buenos_Aires"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("policyInstant")).toBeInTheDocument();
    });
    expect(screen.queryByText("policyReview")).not.toBeInTheDocument();
    expect(screen.queryByText(/confirmationRequired/)).not.toBeInTheDocument();
    expect(screen.queryByText("summaryStatusPending")).not.toBeInTheDocument();
  });

  it("shows review copy on a manual-approval venue", async () => {
    (guestReservationAPI.getSettings as jest.Mock).mockResolvedValue({
      ...autoSettings,
      approval_mode: "manual",
    });
    render(
      <GuestReservationForm
        customUrl="parrilla-quebracho-azul"
        businessName="Parrilla Quebracho Azul"
        timezone="America/Argentina/Buenos_Aires"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("policyReview")).toBeInTheDocument();
    });
    expect(screen.getByText(/confirmationRequired/)).toBeInTheDocument();
  });
});
