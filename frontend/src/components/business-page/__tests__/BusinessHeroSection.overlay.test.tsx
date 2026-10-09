/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import BusinessHeroSection from "../BusinessHeroSection";

const baseProps = {
  business: {
    name: "Test Biz",
    banner_images: JSON.stringify(["https://example.com/banner.jpg"]),
  } as any,
  designSettings: { primary_color: "#1a6b6a", secondary_color: "#2a8b8a" },
  googleRating: null,
  operatingHours: [],
  onViewMenu: () => {},
  openStatusLabel: "Open",
};

describe("BusinessHeroSection — banner overlay", () => {
  it("uses a bottom-weighted scrim instead of flooding the whole hero", () => {
    const { container } = render(<BusinessHeroSection {...baseProps} />);
    const overlay = container.querySelector('[class*="from-black"]');
    expect(overlay).not.toBeNull();
    expect(overlay!.className).toContain("from-black/70");
    expect(overlay!.className).toContain("via-black/30");
    expect(overlay!.className).toContain("to-black/10");
  });

  it("does not render the banner scrim for a non-banner treatment", () => {
    const { container } = render(
      <BusinessHeroSection
        {...baseProps}
        designSettings={{ ...baseProps.designSettings, header_style: "minimal" }}
      />
    );
    expect(container.querySelector('[class*="from-black"]')).toBeNull();
  });
});
