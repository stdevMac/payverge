/** @jest-environment jsdom */
/**
 * #949 — every guest-facing clock string on the storefront follows one
 * hour-cycle rule. The open/closed pill was fixed first; these are its three
 * siblings, which all rendered "11:00 p. m." to an es-AR diner.
 */
import { render, screen } from "@testing-library/react";
import BusinessContactTab from "../BusinessContactTab";
import BusinessFooter from "../BusinessFooter";
import { formatTimeSlot } from "../GuestReservationForm";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import type { PublicBusiness } from "@/api/publicBusiness";

const business = {
  id: 1,
  name: "Parrilla Rivadavia",
  address: { city: "Buenos Aires" },
  social_media: "{}",
  show_operating_hours: true,
} as unknown as PublicBusiness;

const hours = [
  {
    day_of_week: 1,
    is_closed: false,
    open_time: "17:00",
    close_time: "23:00",
  },
];

const MERIDIEM = /[ap]\.?\s*m\.?/i;

describe("es-AR storefront hours render 24-hour (#949)", () => {
  it("BusinessContactTab shows 17:00 – 23:00, never p. m.", () => {
    render(
      <GuestTranslationProvider initialLanguage="es-AR">
        <BusinessContactTab
          business={business}
          designSettings={{ primary_color: "#1a6b6a" }}
          operatingHours={hours}
          t={(k: string) => k}
        />
      </GuestTranslationProvider>,
    );
    expect(screen.getByText(/17:00\s*–\s*23:00/)).toBeInTheDocument();
    expect(screen.queryByText(MERIDIEM)).not.toBeInTheDocument();
  });

  it("BusinessFooter shows 17:00 – 23:00, never p. m.", () => {
    render(
      <GuestTranslationProvider initialLanguage="es-AR">
        <BusinessFooter
          business={business}
          operatingHours={hours}
          designSettings={{
            primary_color: "#1a6b6a",
            secondary_color: "#2a8b8a",
          }}
          t={(k: string) => k}
        />
      </GuestTranslationProvider>,
    );
    expect(screen.getByText(/17:00\s*–\s*23:00/)).toBeInTheDocument();
    expect(screen.queryByText(MERIDIEM)).not.toBeInTheDocument();
  });

  it("GuestReservationForm slot times are 24-hour for es-AR", () => {
    const slot = formatTimeSlot("2024-06-03T23:00:00Z", "UTC", "es-AR");
    expect(slot).toMatch(/23:00/);
    expect(slot).not.toMatch(MERIDIEM);
  });

  it("leaves 12-hour locales alone", () => {
    const slot = formatTimeSlot("2024-06-03T23:00:00Z", "UTC", "en-US");
    expect(slot).toMatch(/11:00/);
    expect(slot).toMatch(/PM/i);
  });
});
