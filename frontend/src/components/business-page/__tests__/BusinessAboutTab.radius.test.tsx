/** @jest-environment jsdom */
import { render } from "@testing-library/react";
import BusinessAboutTab from "../BusinessAboutTab";
import type { PublicBusiness } from "@/api/publicBusiness";

const t = (k: string) => k;

// Mirror the working baseBusiness shape from BusinessAboutTab.test.tsx — the
// gallery gate is business.show_gallery && galleryImages.length > 0.
const baseBusiness = {
  id: 1,
  name: "Test",
  description: "",
  banner_images: "",
  custom_url: "test",
  social_media: "",
  address: { street: "", city: "", state: "", postal_code: "", country: "" },
  google_place_id: "",
  google_reviews_enabled: false,
  show_reviews: false,
  show_special_features: true,
  show_gallery: true,
} as unknown as PublicBusiness;

const baseSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  section_density: "comfortable",
};

describe("BusinessAboutTab — unified radius (BEAUTY-4)", () => {
  it("gallery cards route through getRadiusClass (no off-vocabulary rounded-2xl)", () => {
    // Real gallery prop: array of { id, image_url, caption } — copied from
    // BusinessAboutTab.test.tsx's "renders aspect-[4/3] gallery tiles" test, so
    // the gallery <figure>s actually render and the assertions are meaningful.
    const gallery = [
      { id: 1, image_url: "https://picsum.photos/seed/g1/800/600", caption: "" },
      { id: 2, image_url: "https://picsum.photos/seed/g2/800/600", caption: "" },
    ];
    const { container } = render(
      <BusinessAboutTab
        business={baseBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={[]}
        galleryImages={gallery}
        t={t}
      />,
    );
    // The gallery actually rendered (the assertion is not vacuous).
    expect(container.innerHTML).toMatch(/aspect-\[4\/3\]/);
    // No card surface carries the off-vocabulary rounded-2xl any more.
    expect(container.querySelectorAll('[class*="rounded-2xl"]').length).toBe(0);
    // With corner_radius="medium", getRadiusClass emits rounded-lg.
    expect(container.querySelectorAll('[class*="rounded-lg"]').length).toBeGreaterThan(0);
  });
});
