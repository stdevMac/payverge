/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import BusinessHeroSection from "../BusinessHeroSection";

const withBanner: any = {
  business: { name: "Test Biz", banner_images: JSON.stringify(["https://example.com/banner.jpg"]) },
  designSettings: { primary_color: "#1a6b6a", secondary_color: "#2a8b8a", header_style: "banner" },
  googleRating: null,
  operatingHours: [],
  onViewMenu: () => {},
  openStatusLabel: "Open",
};

describe("BusinessHeroSection — scrim gating (BEAUTY-5)", () => {
  it("renders the legibility scrim over a banner photo (header_style=banner)", () => {
    const { container } = render(<BusinessHeroSection {...withBanner} />);
    const scrim = container.querySelector('[class*="from-black"]');
    expect(scrim).not.toBeNull();
    // The hero H1 uses white text because onDarkOverlay (same boolean as the scrim).
    const h1 = container.querySelector("h1");
    expect(h1?.className).toContain("text-white");
  });

  it("does NOT render the banner scrim for the minimal treatment (no banner background)", () => {
    const minimal = {
      ...withBanner,
      designSettings: { ...withBanner.designSettings, header_style: "minimal" },
    };
    const { container } = render(<BusinessHeroSection {...minimal} />);
    // minimal => treatment.showBanner=false => no full-bleed banner scrim.
    const scrim = container.querySelector('[class*="from-black"]');
    expect(scrim).toBeNull();
    // ...and the hero text is the light-branch charcoal, not white-over-dark.
    const h1 = container.querySelector("h1");
    expect(h1?.className).not.toContain("text-white");
  });

  it("does NOT render the banner scrim when there is no banner image", () => {
    const noBanner = {
      ...withBanner,
      business: { name: "Test Biz", banner_images: JSON.stringify([]) },
    };
    const { container } = render(<BusinessHeroSection {...noBanner} />);
    const scrim = container.querySelector('[class*="from-black"]');
    expect(scrim).toBeNull();
  });
});
