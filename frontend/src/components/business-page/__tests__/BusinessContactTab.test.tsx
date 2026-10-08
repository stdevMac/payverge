/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessContactTab from "../BusinessContactTab";
import type { PublicBusiness } from "@/api/publicBusiness";

const t = (k: string) => k;

const baseBusiness = {
  id: 1,
  name: "Test",
  description: "",
  logo: "",
  banner_images: "",
  custom_url: "test",
  website: "https://example.com",
  phone: "+1 312 847 1928",
  address: { street: "123 Main", city: "Chicago", state: "IL", postal_code: "60601", country: "US" },
  social_media: JSON.stringify({ instagram: "test" }),
  default_currency: "USD",
  display_currency: "USD",
  google_business_name: "",
  google_business_url: "",
  google_place_id: "",
  google_review_link: "",
  google_reviews_enabled: false,
  show_reviews: false,
  show_operating_hours: true,
  created_at: "",
  updated_at: "",
} as unknown as PublicBusiness;

 
const baseSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  corner_radius: "medium",
};
 

describe("BusinessContactTab", () => {
  it("does not repeat Contact as both eyebrow and heading (#508)", () => {
    render(
      <BusinessContactTab
        business={baseBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(screen.getAllByText("businessPage.contact")).toHaveLength(1);
  });

  it("renders address, phone, and website rows", () => {
    render(
      <BusinessContactTab
        business={baseBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(screen.getByText(/123 Main/)).toBeInTheDocument();
    expect(screen.getByText(/\+1 312 847 1928/)).toBeInTheDocument();
  });

  it("does not use the rainbow Instagram gradient or hover:scale-110", () => {
    const { container } = render(
      <BusinessContactTab
        business={baseBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(container.innerHTML).not.toMatch(/from-brand via-pink-500 to-orange-500/);
    expect(container.innerHTML).not.toMatch(/\bhover:scale-110\b/);
  });

  it("normalizes a bare-domain website into an absolute https href (F6)", () => {
    render(
      <BusinessContactTab
        business={{ ...baseBusiness, website: "myrestaurant.com" } as PublicBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByText("businessPage.visitWebsite").closest("a");
    expect(link).toHaveAttribute("href", "https://myrestaurant.com");
  });

  it("leaves an already-absolute website href untouched (F6)", () => {
    render(
      <BusinessContactTab
        business={{ ...baseBusiness, website: "https://example.com" } as PublicBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByText("businessPage.visitWebsite").closest("a");
    expect(link).toHaveAttribute("href", "https://example.com");
  });

  it("formats operating hours via the locale time formatter, not raw 24h (F7)", () => {
    render(
      <BusinessContactTab
        business={baseBusiness}
        designSettings={baseSettings}
        operatingHours={[
          { day_of_week: 1, open_time: "17:00", close_time: "22:00", is_closed: false } as never,
        ]}
        t={t}
      />,
    );
    // Compute the expected formatted string the same way the component does,
    // so the assertion holds regardless of the test runner's locale.
    const fmt = (h: number) => {
      const d = new Date();
      d.setHours(h, 0, 0, 0);
      return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
    };
    const expected = `${fmt(17)} – ${fmt(22)}`;
    expect(screen.getByText(expected)).toBeInTheDocument();
    // The raw 24h concatenation must no longer appear.
    expect(screen.queryByText("17:00 – 22:00")).not.toBeInTheDocument();
  });

  it("does not wrap content in symmetric dual cards with gradient stripes", () => {
    const { container } = render(
      <BusinessContactTab
        business={baseBusiness}
        designSettings={baseSettings}
        operatingHours={[{ day_of_week: 0, open_time: "09:00", close_time: "17:00", is_closed: false } as never]}
        t={t}
      />,
    );
    // The old layout used <Card> wrappers with `h-2 w-full` gradient stripes inside.
    // After the rebuild, no gradient-stripe stripe element should remain.
    const stripes = container.querySelectorAll('div.h-2.w-full');
    expect(stripes.length).toBe(0);
  });
});
