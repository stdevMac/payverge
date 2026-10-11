/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
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
  show_gallery: false,
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

function renderTab(overrides: Partial<PublicBusiness>) {
  return render(
    <BusinessAboutTab
      business={{ ...baseBusiness, ...overrides } as PublicBusiness}
      customUrl="test"
      designSettings={baseSettings}
      specialFeatures={[]}
      galleryImages={[]}
      t={t}
      onViewMenu={jest.fn()}
    />,
  );
}

describe("BusinessAboutTab — Welcome & Story (IA-1)", () => {
  it("renders the welcome message as a lead paragraph when show_welcome_message is on", () => {
    renderTab({ welcome_message: "A warm hello from us", show_welcome_message: true });
    expect(screen.getByText("A warm hello from us")).toBeInTheDocument();
  });

  it("hides the welcome message when show_welcome_message is off", () => {
    renderTab({ welcome_message: "A warm hello from us", show_welcome_message: false });
    expect(screen.queryByText("A warm hello from us")).not.toBeInTheDocument();
  });

  it("renders Our story (heading + prose) when show_about_story is on", () => {
    renderTab({ about_story: "Founded in 1998 by two cousins.", show_about_story: true });
    expect(screen.getByText("Founded in 1998 by two cousins.")).toBeInTheDocument();
    expect(screen.getByText("businessPage.ourStory")).toBeInTheDocument();
  });

  it("hides Our story when show_about_story is off", () => {
    renderTab({ about_story: "Founded in 1998.", show_about_story: false });
    expect(screen.queryByText("Founded in 1998.")).not.toBeInTheDocument();
    expect(screen.queryByText("businessPage.ourStory")).not.toBeInTheDocument();
  });

  it("does NOT show the empty-state fallback when only a story is present (isEmpty flips)", () => {
    renderTab({ about_story: "Our story here.", show_about_story: true });
    // The graceful empty-state CTA must not render when story content exists.
    expect(screen.queryByText("businessPage.aboutViewMenuCta")).not.toBeInTheDocument();
    expect(screen.queryByText("businessPage.aboutEmptyWelcome")).not.toBeInTheDocument();
  });
});
