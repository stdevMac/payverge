/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessContactTab from "../BusinessContactTab";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import type { PublicBusiness } from "@/api/publicBusiness";

const business = {
  id: 1,
  name: "Cafe Aurora",
  address: { city: "Berlin" },
  social_media: "{}",
  // The hours section only renders when the merchant has enabled it; without
  // this the row never mounts and the locale assertion would be vacuously true.
  show_operating_hours: true,
} as unknown as PublicBusiness;

// Format an HH:mm pair the same way the component does, for a given locale, so
// the assertion holds regardless of the test runner's default locale.
const fmtPair = (locale: string, openH: number, closeH: number) => {
  const f = (h: number) => {
    const d = new Date();
    d.setHours(h, 0, 0, 0);
    return d.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" });
  };
  return `${f(openH)} – ${f(closeH)}`;
};

describe("BusinessContactTab — guest-locale hours (I18N-4)", () => {
  it("formats operating hours in the active guest locale (de → 24h, no AM/PM)", () => {
    render(
      <GuestTranslationProvider initialLanguage="de">
        <BusinessContactTab
          business={business}
          designSettings={{ primary_color: "#1a6b6a" }}
          operatingHours={[
            { day_of_week: 1, is_closed: false, open_time: "17:00", close_time: "23:00" },
          ]}
          t={(k: string) => k}
        />
      </GuestTranslationProvider>,
    );
    // The hours row must render in the German convention (24h, no AM/PM), not
    // the runner's en-US default (which would render "5:00 PM – 11:00 PM").
    expect(screen.getByText(fmtPair("de", 17, 23))).toBeInTheDocument();
    expect(screen.queryByText(fmtPair("en-US", 17, 23))).not.toBeInTheDocument();
    expect(screen.queryByText(/PM/i)).not.toBeInTheDocument();
  });
});
