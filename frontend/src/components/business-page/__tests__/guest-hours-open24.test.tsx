/** @jest-environment jsdom */
/**
 * Demo venues are seeded open 00:00–00:00, which the backend and
 * useBusinessOpenStatus read as a full wrapping day. Storefront hour lists and
 * the open/closed pill must say "Open 24 hours", never "00:00 – 00:00" or
 * "Open until 00:00".
 */
import { render, screen, waitFor } from "@testing-library/react";
import BusinessContactTab from "../BusinessContactTab";
import BusinessFooter from "../BusinessFooter";
import OpenClosedPill from "@/components/guest/OpenClosedPill";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import type { PublicBusiness } from "@/api/publicBusiness";
import { isAllDayWindow } from "@/utils/guestClockTime";
import en from "@/i18n/guest-messages/en.json";
import es from "@/i18n/guest-messages/es.json";

const business = {
  id: 1,
  name: "Demo Bistro",
  address: { city: "Buenos Aires" },
  social_media: "{}",
  show_operating_hours: true,
} as unknown as PublicBusiness;

const allDay = [0, 1, 2, 3, 4, 5, 6].map((day) => ({
  day_of_week: day,
  is_closed: false,
  open_time: "00:00",
  close_time: "00:00",
}));

const guestT = (k: string) =>
  k === "businessPage.open24Hours" ? en.businessPage.open24Hours : k;

describe("isAllDayWindow", () => {
  it("is true only for equal parseable open/close times", () => {
    expect(isAllDayWindow("00:00", "00:00")).toBe(true);
    expect(isAllDayWindow("09:00:00", "09:00")).toBe(true);
    expect(isAllDayWindow("18:00", "02:00")).toBe(false);
    expect(isAllDayWindow("09:00", "17:00")).toBe(false);
    expect(isAllDayWindow("", "")).toBe(false);
    expect(isAllDayWindow(undefined, undefined)).toBe(false);
  });
});

describe("storefront renders 00:00–00:00 as Open 24 hours", () => {
  it("BusinessContactTab", () => {
    render(
      <GuestTranslationProvider initialLanguage="en">
        <BusinessContactTab
          business={business}
          designSettings={{ primary_color: "#1a6b6a" }}
          operatingHours={allDay}
          t={guestT}
        />
      </GuestTranslationProvider>,
    );
    expect(screen.getAllByText("Open 24 hours")).toHaveLength(7);
    expect(screen.queryByText(/12:00\s*AM|00:00/)).not.toBeInTheDocument();
  });

  it("BusinessFooter", () => {
    render(
      <GuestTranslationProvider initialLanguage="en">
        <BusinessFooter
          business={business}
          operatingHours={allDay}
          designSettings={{
            primary_color: "#1a6b6a",
            secondary_color: "#2a8b8a",
          }}
          t={guestT}
        />
      </GuestTranslationProvider>,
    );
    expect(screen.getAllByText("Open 24 hours")).toHaveLength(7);
    expect(screen.queryByText(/12:00\s*AM|00:00/)).not.toBeInTheDocument();
  });

  it("keeps ordinary ranges as times", () => {
    render(
      <GuestTranslationProvider initialLanguage="es-AR">
        <BusinessFooter
          business={business}
          operatingHours={[
            {
              day_of_week: 1,
              is_closed: false,
              open_time: "17:00",
              close_time: "23:00",
            },
          ]}
          designSettings={{
            primary_color: "#1a6b6a",
            secondary_color: "#2a8b8a",
          }}
          t={guestT}
        />
      </GuestTranslationProvider>,
    );
    expect(screen.getByText(/17:00\s*–\s*23:00/)).toBeInTheDocument();
    expect(screen.queryByText("Open 24 hours")).not.toBeInTheDocument();
  });

  it("OpenClosedPill says Open 24 hours in the guest locale", async () => {
    render(
      <GuestTranslationProvider initialLanguage="es">
        <OpenClosedPill
          hours={allDay}
          timezone="UTC"
          now={new Date("2024-06-03T12:00:00Z")}
        />
      </GuestTranslationProvider>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("landing-open-pill")).toHaveTextContent(
        es.businessPage.open24Hours,
      ),
    );
    expect(screen.getByTestId("landing-open-pill")).not.toHaveTextContent(
      /00:00/,
    );
  });
});
