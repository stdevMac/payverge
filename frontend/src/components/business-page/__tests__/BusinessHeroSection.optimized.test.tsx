/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import BusinessHeroSection from "../BusinessHeroSection";
import type { PublicBusiness } from "@/api/publicBusiness";

const imgProps: Record<string, unknown>[] = [];
jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    imgProps.push(props);
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as any)} />;
  },
}));

const business = {
  id: 1,
  name: "Cafe Aurora",
  description: "A neighborhood bistro",
  logo: "",
  // Same-origin upload served through the /media proxy.
  banner_images: JSON.stringify(["/media/banners/aurora.jpg"]),
  custom_url: "aurora",
  show_operating_hours: false,
  show_reviews: false,
  google_reviews_enabled: false,
} as unknown as PublicBusiness;

describe("BusinessHeroSection — optimized banner (PERF-2)", () => {
  beforeEach(() => {
    imgProps.length = 0;
  });

  const renderHero = (b: PublicBusiness) =>
    render(
      <BusinessHeroSection
        business={b}
        designSettings={{
          primary_color: "#1a6b6a",
          secondary_color: "#2a8b8a",
          hero_layout: "centered",
          header_style: "banner",
        }}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={jest.fn()}
        openStatusLabel="Open"
      />,
    );

  it("renders the banner image WITHOUT unoptimized (default banner treatment)", () => {
    renderHero(business);
    // The banner NextImage is present and the priority LCP one is optimized.
    const banner = imgProps.find((p) => p.fill && p.priority);
    expect(banner).toBeDefined();
    expect(banner?.unoptimized).toBeFalsy();
    // The failed-slide-skipping onError handler (protected bar) is preserved.
    expect(typeof banner?.onError).toBe("function");
  });

  it("serves a runtime-only upload host unoptimized instead of a broken optimizer URL", () => {
    renderHero({
      ...business,
      banner_images: JSON.stringify(["https://bucket.example.test/banners/aurora.jpg"]),
    } as PublicBusiness);
    const banner = imgProps.find((p) => p.fill && p.priority);
    expect(banner?.unoptimized).toBe(true);
  });
});
