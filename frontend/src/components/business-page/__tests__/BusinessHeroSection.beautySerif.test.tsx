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

describe("BusinessHeroSection — crisp serif H1 (BEAUTY-1)", () => {
  it("the hero name H1 uses font-title with NO faux-bold weight", () => {
    const { getByRole } = render(<BusinessHeroSection {...baseProps} />);
    const h1 = getByRole("heading", { level: 1, name: /test biz/i });
    expect(h1.className).toContain("font-title");
    expect(h1.className).not.toContain("font-semibold");
    expect(h1.className).not.toContain("font-bold");
  });
});
