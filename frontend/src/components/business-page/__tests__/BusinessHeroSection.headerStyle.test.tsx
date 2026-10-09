/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import BusinessHeroSection from "../BusinessHeroSection";
import type { PublicBusiness } from "@/api/publicBusiness";

const business = {
  id: 1,
  name: "Cafe Aurora",
  description: "A neighborhood bistro",
  logo: "",
  banner_images: JSON.stringify(["https://picsum.photos/seed/b1/1600/900"]),
  custom_url: "aurora",
  show_operating_hours: false,
  show_reviews: false,
  google_reviews_enabled: false,
} as unknown as PublicBusiness;

function renderHero(header_style: string) {
  return render(
    <BusinessHeroSection
      business={business}
      designSettings={{
        primary_color: "#1a6b6a",
        secondary_color: "#2a8b8a",
        hero_layout: "centered",
        header_style,
      }}
      googleRating={null}
      operatingHours={[]}
      onViewMenu={jest.fn()}
      openStatusLabel="Open"
    />,
  );
}

describe("BusinessHeroSection — header_style treatment", () => {
  it("banner: tall hero with a full-bleed banner background", () => {
    const { container } = renderHero("banner");
    const section = container.querySelector("section");
    expect(section?.className).toMatch(/min-h-\[56vh\] md:min-h-\[68vh\]/);
    expect(section?.getAttribute("data-header-style")).toBe("banner");
    // Full-bleed banner <img> present.
    expect(container.querySelector("img")).not.toBeNull();
  });

  it("minimal: compact band, no banner background, forced centered", () => {
    const { container } = renderHero("minimal");
    const section = container.querySelector("section");
    expect(section?.className).toMatch(/min-h-\[40vh\] md:min-h-\[44vh\]/);
    expect(section?.getAttribute("data-header-style")).toBe("minimal");
    // The full-bleed banner background is suppressed in minimal.
    expect(container.querySelector("img")).toBeNull();
  });

  it("classic: reduced height, forced centered", () => {
    const { container } = renderHero("classic");
    const section = container.querySelector("section");
    expect(section?.className).toMatch(/min-h-\[48vh\] md:min-h-\[56vh\]/);
    expect(section?.getAttribute("data-header-style")).toBe("classic");
  });
});
