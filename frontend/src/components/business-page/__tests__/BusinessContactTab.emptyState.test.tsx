/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessContactTab from "../BusinessContactTab";
import type { PublicBusiness } from "@/api/publicBusiness";

const t = (k: string) => k;

// A business with NO contact channels: empty address, no phone, no website,
// no socials, hours hidden. Drives the plan-1.4 empty-state predicate.
const bareBusiness = {
  id: 1,
  name: "Bare",
  description: "",
  logo: "",
  banner_images: "",
  custom_url: "bare",
  website: "",
  phone: "",
  address: { street: "", city: "", state: "", postal_code: "", country: "" },
  social_media: "",
  default_currency: "USD",
  display_currency: "USD",
  google_business_name: "",
  google_business_url: "",
  google_place_id: "",
  google_review_link: "",
  google_reviews_enabled: false,
  show_reviews: false,
  show_operating_hours: false,
  created_at: "",
  updated_at: "",
} as unknown as PublicBusiness;

const baseSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  corner_radius: "medium",
};

const hours = [
  { day_of_week: 1, open_time: "09:00", close_time: "17:00", is_closed: false } as never,
];

describe("BusinessContactTab — empty-state predicate (plan 1.4)", () => {
  it("renders the friendly empty state when nothing is shared", () => {
    render(
      <BusinessContactTab
        business={bareBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(
      screen.getByText("businessPage.contactEmptyTitle"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("businessPage.contactEmptyBody"),
    ).toBeInTheDocument();
    // No contact rows / hours list leak through.
    expect(screen.queryByText("businessPage.address")).not.toBeInTheDocument();
    expect(
      screen.queryByText("businessPage.openingHours"),
    ).not.toBeInTheDocument();
  });

  it("does NOT render the empty state when only a phone exists", () => {
    render(
      <BusinessContactTab
        business={{ ...bareBusiness, phone: "+54 11 5555 5555" } as PublicBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(
      screen.queryByText("businessPage.contactEmptyTitle"),
    ).not.toBeInTheDocument();
  });

  it("does NOT render the empty state when only a website exists", () => {
    render(
      <BusinessContactTab
        business={{ ...bareBusiness, website: "https://example.com" } as PublicBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(
      screen.queryByText("businessPage.contactEmptyTitle"),
    ).not.toBeInTheDocument();
  });

  it("does NOT render the empty state when only an address exists", () => {
    render(
      <BusinessContactTab
        business={
          {
            ...bareBusiness,
            address: { street: "", city: "Buenos Aires", state: "", postal_code: "", country: "" },
          } as PublicBusiness
        }
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(
      screen.queryByText("businessPage.contactEmptyTitle"),
    ).not.toBeInTheDocument();
  });

  it("does NOT render the empty state when only socials exist", () => {
    render(
      <BusinessContactTab
        business={
          {
            ...bareBusiness,
            social_media: JSON.stringify({ instagram: "bare" }),
          } as PublicBusiness
        }
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(
      screen.queryByText("businessPage.contactEmptyTitle"),
    ).not.toBeInTheDocument();
  });

  it("does NOT render the empty state when only hours are shown", () => {
    render(
      <BusinessContactTab
        business={{ ...bareBusiness, show_operating_hours: true } as PublicBusiness}
        designSettings={baseSettings}
        operatingHours={hours}
        t={t}
      />,
    );
    expect(
      screen.queryByText("businessPage.contactEmptyTitle"),
    ).not.toBeInTheDocument();
    expect(screen.getByText("businessPage.openingHours")).toBeInTheDocument();
  });

  it("hidden hours (show_operating_hours off) do not count as content", () => {
    render(
      <BusinessContactTab
        business={bareBusiness}
        designSettings={baseSettings}
        operatingHours={hours}
        t={t}
      />,
    );
    expect(
      screen.getByText("businessPage.contactEmptyTitle"),
    ).toBeInTheDocument();
  });
});

describe("BusinessContactTab — map embed (plan 3.3)", () => {
  const withAddress = {
    ...bareBusiness,
    address: {
      street: "123 Main St",
      city: "Chicago",
      state: "IL",
      postal_code: "60601",
      country: "US",
    },
  } as unknown as PublicBusiness;

  it("renders a lazy Google Maps iframe titled for a11y when an address exists", () => {
    render(
      <BusinessContactTab
        business={withAddress}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    const iframe = screen.getByTitle("businessPage.mapTitle");
    expect(iframe).toBeInTheDocument();
    expect(iframe).toHaveAttribute("loading", "lazy");
    expect(iframe.getAttribute("src")).toContain(
      `https://www.google.com/maps?q=${encodeURIComponent("123 Main St, Chicago, IL, 60601, US")}&output=embed`,
    );
  });

  it("renders a Get directions link targeting Google Maps search", () => {
    render(
      <BusinessContactTab
        business={withAddress}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByRole("link", {
      name: /businessPage\.getDirections/,
    });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
    expect(link.getAttribute("href")).toContain(
      `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent("123 Main St, Chicago, IL, 60601, US")}`,
    );
  });

  it("renders no map and no directions CTA without an address", () => {
    render(
      <BusinessContactTab
        business={{ ...bareBusiness, phone: "+1 555 555 5555" } as PublicBusiness}
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    expect(screen.queryByTitle("businessPage.mapTitle")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /businessPage\.getDirections/ }),
    ).not.toBeInTheDocument();
  });
});

describe("BusinessContactTab — social chip hover (plan 1.9)", () => {
  it("uses CSS var-driven hover classes, not JS mouse handlers", () => {
    render(
      <BusinessContactTab
        business={
          {
            ...bareBusiness,
            social_media: JSON.stringify({ instagram: "bare" }),
          } as PublicBusiness
        }
        designSettings={baseSettings}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByRole("link", { name: /instagram/i });
    expect(link.className).toMatch(/hover:bg-\[var\(--chip-fg\)\]/);
    expect(link.className).toMatch(/hover:text-white/);
    expect(link.className).toMatch(/focus-visible:bg-\[var\(--chip-fg\)\]/);
    // No imperative inline-style mutation leftovers.
    expect(link.getAttribute("onmouseenter")).toBeNull();
    expect(link.getAttribute("onmouseleave")).toBeNull();
  });
});
