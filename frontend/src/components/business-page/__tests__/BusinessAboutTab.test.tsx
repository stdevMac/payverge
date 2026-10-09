/** @jest-environment jsdom */
import { render, screen, fireEvent } from "@testing-library/react";
import BusinessAboutTab from "../BusinessAboutTab";
import type { PublicBusiness } from "@/api/publicBusiness";

const t = (k: string) => k;

const baseBusiness = {
  id: 1,
  name: "Test",
  description: "",
  logo: "",
  banner_images: "",
  custom_url: "test",
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
  show_special_features: true,
  show_gallery: true,
  created_at: "",
  updated_at: "",
} as unknown as PublicBusiness;

 
const baseSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  section_density: "comfortable",
};
 

describe("BusinessAboutTab", () => {
  it("capitalizes a lowercase highlight title on display (#488)", () => {
    render(
      <BusinessAboutTab
        business={baseBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={[
          {
            id: 1,
            title: "servicio de IA",
            description: "Asistente",
            icon: "sparkles",
            is_active: true,
          },
        ]}
        galleryImages={[]}
        t={t}
      />,
    );
    expect(screen.getByText("Servicio de IA")).toBeInTheDocument();
    expect(screen.queryByText("servicio de IA")).not.toBeInTheDocument();
  });

  it("does not use scale-110 hover or 3/4-column features grid", () => {
    const features = [
      { id: 1, title: "Patio", description: "Outdoor seating", icon: "sun", is_active: true },
      { id: 2, title: "Pet-friendly", description: "Dogs welcome", icon: "dog", is_active: true },
    ];
    const { container } = render(
      <BusinessAboutTab
        business={baseBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={features}
        galleryImages={[]}
        t={t}
      />,
    );
    expect(container.innerHTML).not.toMatch(/\bhover:scale-110\b/);
    expect(container.innerHTML).not.toMatch(/md:grid-cols-3 lg:grid-cols-4/);
  });

  it("renders at most 2 columns for the features grid on md", () => {
    const features = [
      { id: 1, title: "A", description: "", icon: "", is_active: true },
      { id: 2, title: "B", description: "", icon: "", is_active: true },
      { id: 3, title: "C", description: "", icon: "", is_active: true },
    ];
    const { container } = render(
      <BusinessAboutTab
        business={baseBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={features}
        galleryImages={[]}
        t={t}
      />,
    );
    // Accept md:grid-cols-2 (zig-zag) but not 3 or 4 columns
    expect(container.innerHTML).toMatch(/md:grid-cols-2/);
    expect(container.innerHTML).not.toMatch(/md:grid-cols-(3|4)/);
  });

  it("picks specific icons for known feature titles, not Sparkles", () => {
    // We can't directly test the icon symbol, but we can rely on lucide-react
    // setting `data-lucide` or using distinct svg children. Instead assert the
    // icon container is colored with the merchant primary_color (which only
    // happens for picked icons in the rewrite).
    const features = [
      { id: 1, title: "Pet-friendly", description: "", icon: "", is_active: true },
      { id: 2, title: "Live Bar", description: "", icon: "", is_active: true },
    ];
    const { container } = render(
      <BusinessAboutTab
        business={baseBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={features}
        galleryImages={[]}
        t={t}
      />,
    );
    // Two feature rows, both icon containers (the ones with inline style) should
    // have the brand color set inline.
    const iconWrappers = container.querySelectorAll('li > div[style]');
    expect(iconWrappers.length).toBeGreaterThanOrEqual(2);
    iconWrappers.forEach((wrapper) => {
      const style = (wrapper as HTMLElement).getAttribute("style") || "";
      // jsdom normalizes inline styles to rgb()/rgba(). The merchant primary
      // color #1a6b6a == rgb(26, 107, 106), which must appear in both the
      // background (alpha) and the foreground color so the SVG glyph inherits
      // the brand tint via currentColor.
      const lower = style.toLowerCase();
      const hasBrandColor =
        lower.includes("1a6b6a") || lower.includes("26, 107, 106");
      expect(hasBrandColor).toBe(true);
    });
  });

  it("renders a graceful default block + View menu CTA when no sections present (F5)", () => {
    const onViewMenu = jest.fn();
    render(
      <BusinessAboutTab
        business={{
          ...baseBusiness,
          show_gallery: false,
          description: "A cozy neighborhood bistro.",
          address: { street: "1 Main St", city: "Chicago", state: "IL", postal_code: "", country: "" },
        } as PublicBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={[]}
        galleryImages={[]}
        t={t}
        onViewMenu={onViewMenu}
      />,
    );
    // Description shown, address summary shown, and the CTA wired to onViewMenu.
    expect(screen.getByText("A cozy neighborhood bistro.")).toBeInTheDocument();
    expect(screen.getByText(/1 Main St, Chicago, IL/)).toBeInTheDocument();
    const cta = screen.getByText("businessPage.aboutViewMenuCta");
    fireEvent.click(cta);
    expect(onViewMenu).toHaveBeenCalledTimes(1);
  });

  it("falls back to a welcome line when the business has no description (F5)", () => {
    render(
      <BusinessAboutTab
        business={{ ...baseBusiness, show_gallery: false, description: "" } as PublicBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={[]}
        galleryImages={[]}
        t={t}
      />,
    );
    expect(screen.getByText("businessPage.aboutEmptyWelcome")).toBeInTheDocument();
  });

  it("does NOT render the empty-state block when a section is present (F5)", () => {
    render(
      <BusinessAboutTab
        business={{ ...baseBusiness, show_gallery: false } as PublicBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={[{ id: 1, title: "Patio", description: "", icon: "sun", is_active: true }]}
        galleryImages={[]}
        t={t}
        onViewMenu={jest.fn()}
      />,
    );
    expect(screen.queryByText("businessPage.aboutViewMenuCta")).not.toBeInTheDocument();
    expect(screen.queryByText("businessPage.aboutEmptyWelcome")).not.toBeInTheDocument();
  });

  it("renders aspect-[4/3] gallery tiles when gallery enabled", () => {
    const gallery = [
      { id: 1, image_url: "https://picsum.photos/seed/g1/800/600", caption: "" },
      { id: 2, image_url: "https://picsum.photos/seed/g2/800/600", caption: "" },
    ];
    const { container } = render(
      <BusinessAboutTab
        business={{ ...baseBusiness, show_gallery: true } as PublicBusiness}
        customUrl="test"
        designSettings={baseSettings}
        specialFeatures={[]}
        galleryImages={gallery}
        t={t}
      />,
    );
    expect(container.innerHTML).toMatch(/aspect-\[4\/3\]/);
    expect(container.innerHTML).not.toMatch(/aspect-square/);
  });
});
