/** @jest-environment jsdom */
import { act, fireEvent, render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BusinessHeroSection from "../BusinessHeroSection";
import type { PublicBusiness } from "@/api/publicBusiness";

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
  logo: "https://cdn.example.com/logo.png",
  banner_images: JSON.stringify([
    "https://cdn.example.com/a.jpg",
    "https://cdn.example.com/b.jpg",
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

function mockMatchMedia(reduce: boolean) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((query: string) => ({
      matches: reduce && query.includes("prefers-reduced-motion"),
      media: query,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      addListener: jest.fn(),
      removeListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
}

function renderHero(overrides: Partial<React.ComponentProps<typeof BusinessHeroSection>> = {}) {
  return render(
    <BusinessHeroSection
      business={business}
      designSettings={settings}
      googleRating={null}
      operatingHours={[]}
      onViewMenu={() => {}}
      openStatusLabel="Open now"
      isOpenNow
      {...overrides}
    />,
  );
}

function slideWrappers(container: HTMLElement) {
  return Array.from(container.querySelectorAll("[data-hero-slide]"));
}

describe("BusinessHeroSection motion and slide semantics", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockMatchMedia(false);
  });

  afterEach(() => {
    act(() => {
      cleanup();
    });
    jest.useRealTimers();
  });

  it("autoplays multiple banners and exposes a pause/play control", async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    const { container } = renderHero();

    const slides = slideWrappers(container);
    expect(slides).toHaveLength(2);
    expect(slides[0]).toHaveAttribute("data-hero-slide", "active");
    expect(slides[1]).toHaveAttribute("data-hero-slide", "inactive");

    act(() => {
      jest.advanceTimersByTime(6000);
    });
    expect(slideWrappers(container)[1]).toHaveAttribute("data-hero-slide", "active");

    const toggle = screen.getByRole("button", { name: /pause banner/i });
    expect(toggle).toBeVisible();
    await user.click(toggle);

    act(() => {
      jest.advanceTimersByTime(6000);
    });
    expect(slideWrappers(container)[1]).toHaveAttribute("data-hero-slide", "active");

    await user.click(screen.getByRole("button", { name: /play banner/i }));
    act(() => {
      jest.advanceTimersByTime(6000);
    });
    expect(slideWrappers(container)[0]).toHaveAttribute("data-hero-slide", "active");
  });

  it("does not autoplay or crossfade under prefers-reduced-motion", () => {
    mockMatchMedia(true);
    const setIntervalSpy = jest.spyOn(global, "setInterval");
    const { container } = renderHero();

    expect(setIntervalSpy).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /pause banner|play banner/i })).not.toBeInTheDocument();
    expect(slideWrappers(container)).toHaveLength(1);
    expect(container.querySelector(".duration-1000")).toBeNull();

    act(() => {
      jest.advanceTimersByTime(12000);
    });
    expect(slideWrappers(container)[0]).toHaveAttribute("data-hero-slide", "active");
    setIntervalSpy.mockRestore();
  });

  it("does not render a motion control when only one banner exists", () => {
    renderHero({
      business: {
        ...business,
        banner_images: JSON.stringify(["https://cdn.example.com/only.jpg"]),
      } as PublicBusiness,
    });
    expect(screen.queryByRole("button", { name: /pause banner|play banner/i })).not.toBeInTheDocument();
  });

  it("hides inactive slides from the accessibility tree and keeps banners decorative", () => {
    const { container } = renderHero();
    const slides = slideWrappers(container);

    expect(slides[0]).toHaveAttribute("aria-hidden", "true");
    expect(slides[1]).toHaveAttribute("aria-hidden", "true");
    expect(slides[1].hasAttribute("inert")).toBe(true);

    const bannerImgs = Array.from(container.querySelectorAll("[data-hero-slide] img"));
    bannerImgs.forEach((img) => {
      expect(img).toHaveAttribute("alt", "");
    });
    expect(screen.queryByRole("img", { name: /rotation cafe —/i })).not.toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Rotation Cafe" })).toBeInTheDocument();
  });

  it("keeps failed slides out of the tree and continues rotating remaining banners", () => {
    const { container } = renderHero();
    const failed = container.querySelector('img[src*="b.jpg"]') as HTMLImageElement;
    act(() => {
      fireEvent.error(failed);
    });

    expect(container.querySelector('img[src*="b.jpg"]')).toBeNull();
    expect(screen.queryByRole("button", { name: /pause banner/i })).not.toBeInTheDocument();
  });
});
