/** @jest-environment jsdom */
import { render, act, fireEvent, cleanup } from "@testing-library/react";
import BusinessHeroSection from "../BusinessHeroSection";
import type { PublicBusiness } from "@/api/publicBusiness";

// Render next/image as a plain <img> so onError is reachable.
jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: any) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...props} />;
  },
}));

const business = {
  id: 1,
  name: "Rotation Cafe",
  description: "",
  logo: "",
  banner_images: JSON.stringify([
    "https://cdn.example.com/a.jpg",
    "https://cdn.example.com/b.jpg",
    "https://cdn.example.com/c.jpg",
  ]),
  custom_url: "rotation-cafe",
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
  show_operating_hours: true,
  created_at: "",
  updated_at: "",
} as unknown as PublicBusiness;

const settings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  background_pattern: "none",
  pattern_opacity: 0.1,
  hero_layout: "centered" as const,
};

function visibleBannerSrc(container: HTMLElement): string | null {
  // The active slide wrapper carries opacity-100; the inactive ones opacity-0.
  const active = container.querySelector(".opacity-100 img") as HTMLImageElement | null;
  return active ? active.getAttribute("src") : null;
}

describe("BusinessHeroSection banner rotation skips failed banners", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => {
    act(() => {
      cleanup();
    });
    jest.useRealTimers();
  });

  it("does not dwell on a failed middle banner during auto-rotation", () => {
    const { container } = render(
      <BusinessHeroSection
        business={business}
        designSettings={settings}
        googleRating={null}
        operatingHours={[]}
        onViewMenu={() => {}}
        openStatusLabel="Open now"
        isOpenNow
      />,
    );

    // Index 0 ("a.jpg") is shown initially.
    expect(visibleBannerSrc(container)).toContain("a.jpg");

    // Fail the middle banner (index 1, "b.jpg").
    const imgs = Array.from(container.querySelectorAll("img"));
    const middle = imgs.find((i) => i.getAttribute("src")?.includes("b.jpg"))!;
    act(() => {
      fireEvent.error(middle);
    });

    // Advance one rotation tick. With the bug, it would land on the failed
    // index 1 and show no banner; the fix must skip to the loadable index 2.
    act(() => {
      jest.advanceTimersByTime(6000);
    });

    const src = visibleBannerSrc(container);
    expect(src).not.toBeNull();
    expect(src).toContain("c.jpg");
  });
});
