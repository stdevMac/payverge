/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessAboutTab from "../BusinessAboutTab";
import type { PublicBusiness } from "@/api/publicBusiness";

// The reviews section header ("What customers say") is rendered by
// BusinessAboutTab itself; the slider is a child that needs the guest
// translation provider. Stub it so these gate assertions can run the
// reviews-on path without wiring the full GuestTranslationProvider.
jest.mock("../GoogleReviewsSlider", () => ({
  __esModule: true,
  default: () => <div data-testid="google-reviews-slider" />,
}));

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
  google_place_id: "place-123",
  google_review_link: "",
  google_reviews_enabled: true,
  show_reviews: true,
  show_gallery: false,
  show_special_features: true,
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

const features = [{ id: 1, title: "Patio", description: "Outdoor seating", icon: "sun", is_active: true }];

function renderTab(overrides: Partial<PublicBusiness>) {
  return render(
    <BusinessAboutTab
      business={{ ...baseBusiness, ...overrides } as PublicBusiness}
      customUrl="test"
      designSettings={baseSettings}
      specialFeatures={features}
      galleryImages={[]}
      t={t}
      onViewMenu={jest.fn()}
    />,
  );
}

describe("BusinessAboutTab — visibility gates (PARITY-3, PARITY-5)", () => {
  it("hides the features section header when show_special_features is off", () => {
    renderTab({ show_special_features: false });
    expect(screen.queryByText("businessPage.whyChooseUs")).not.toBeInTheDocument();
  });

  it("shows the features section when show_special_features is on", () => {
    renderTab({ show_special_features: true });
    expect(screen.getByText("businessPage.whyChooseUs")).toBeInTheDocument();
  });

  it("hides the entire reviews section (no orphan header) when show_reviews is off", () => {
    renderTab({ show_reviews: false });
    expect(screen.queryByText("businessPage.whatCustomersSay")).not.toBeInTheDocument();
  });

  it("shows the reviews section header when show_reviews is on and Google is linked", () => {
    renderTab({ show_reviews: true });
    expect(screen.getByText("businessPage.whatCustomersSay")).toBeInTheDocument();
  });
});
