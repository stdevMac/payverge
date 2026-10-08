/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessHeroSection from "../BusinessHeroSection";
import type { PublicBusiness } from "@/api/publicBusiness";

const baseBusiness = {
  id: 1,
  name: "Mara Core Kitchen",
  description: "Fire-cooked Mediterranean.",
  logo: "",
  banner_images: "",
  custom_url: "mara-core-kitchen",
  website: "",
  phone: "+1 312 847 1928",
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
  show_operating_hours: true,
  created_at: "",
  updated_at: "",
} as unknown as PublicBusiness;

 
const baseSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  background_pattern: "none",
  pattern_opacity: 0.1,
  hero_layout: "centered" as const,
};
 

describe("BusinessHeroSection", () => {
  it("renders the business name as h1", () => {
    render(
      <BusinessHeroSection
        business={baseBusiness}
        designSettings={baseSettings}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Mara Core Kitchen",
    );
  });

  it("does not apply inline text-shadow", () => {
    const { container } = render(
      <BusinessHeroSection
        business={baseBusiness}
        designSettings={baseSettings}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    expect(container.innerHTML).not.toMatch(/text-shadow/);
  });

  it("does not use text-7xl/8xl on the H1", () => {
    const { container } = render(
      <BusinessHeroSection
        business={baseBusiness}
        designSettings={baseSettings}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    const h1 = container.querySelector("h1")!;
    expect(h1.className).not.toMatch(/\btext-(7xl|8xl)\b/);
  });

  it("renders split layout left when hero_layout='split-left'", () => {
    const { container } = render(
      <BusinessHeroSection
        business={
          { ...baseBusiness, logo: "https://example.com/logo.png" } as PublicBusiness
        }
        designSettings={{ ...baseSettings, hero_layout: "split-left" }}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    expect(
      container.querySelector('[data-hero-layout="split-left"]'),
    ).not.toBeNull();
  });

  it("uses logical start alignment in the split hero so RTL inherits correctly", () => {
    const { container } = render(
      <BusinessHeroSection
        business={
          { ...baseBusiness, logo: "https://example.com/logo.png" } as PublicBusiness
        }
        designSettings={{ ...baseSettings, hero_layout: "split-left" }}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    expect(container.innerHTML).toMatch(/\bitems-start\b/);
    expect(container.innerHTML).toMatch(/\btext-start\b/);
    expect(container.innerHTML).not.toMatch(/\btext-left\b/);
  });

  it("status chip does not pulse and does not use neon glow shadow", () => {
    const { container } = render(
      <BusinessHeroSection
        business={baseBusiness}
        designSettings={baseSettings}
        googleRating={null}
        operatingHours={[{ day_of_week: 0 } as never]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    expect(container.querySelector(".motion-safe\\:animate-pulse")).toBeNull();
    expect(container.innerHTML).not.toMatch(/shadow-(green|red)-500\/\d+/);
  });

  it("falls back to centered when split layout requested without a logo", () => {
    const { container } = render(
      <BusinessHeroSection
        business={{ ...baseBusiness, logo: "" } as PublicBusiness}
        designSettings={{ ...baseSettings, hero_layout: "split-left" }}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    // The data attribute should reflect the effective (fallback) layout.
    expect(
      container.querySelector('[data-hero-layout="centered"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-hero-layout="split-left"]'),
    ).toBeNull();
  });

  it("status chip carries role=status for screen readers", () => {
    render(
      <BusinessHeroSection
        business={baseBusiness}
        designSettings={baseSettings}
        googleRating={null}
        operatingHours={[{ day_of_week: 0 } as never]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Open now");
  });
});
