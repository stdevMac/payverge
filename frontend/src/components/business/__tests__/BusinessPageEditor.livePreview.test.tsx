/** @jest-environment jsdom */
/**
 * Task 25 / Finding 37 — live preview must render the business's actual
 * name, hours, and contact from the editor form state (not hardcoded lorem).
 */
import React from "react";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import BusinessPageEditor from "../BusinessPageEditor";
import { businessApi } from "@/api/business";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { resetTestUrl } from "@/test/nextNavigationMock";

// #225 moved Business Page sub-tabs onto `?section=` via useUrlState — the
// static next/navigation mock cannot round-trip tab clicks.
jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: { alt?: string; src?: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img alt={props.alt || ""} src={typeof props.src === "string" ? props.src : ""} />
  ),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

// The real useToast returns stable callbacks. A fresh jest.fn() per render
// changes loadData's identity every render and reloads the page in a loop.
jest.mock("@/contexts/ToastContext", () => {
  const toast = { showSuccess: jest.fn(), showError: jest.fn() };
  return { useToast: () => toast };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn(),
    getBusinessGalleryImages: jest.fn(),
    getBusinessOperatingHours: jest.fn(),
    getBusinessSpecialFeatures: jest.fn(),
    updateBusiness: jest.fn(),
    updateBusinessOperatingHours: jest.fn(),
    updateBusinessSpecialFeatures: jest.fn(),
    updateBusinessGalleryImages: jest.fn(),
    updateBusinessDesignSettings: jest.fn(),
  },
  checkCustomURLAvailability: jest.fn().mockResolvedValue({ available: true }),
}));

// Tolerate concurrent Reviews-tab work (Task 28) that may load plugins in
// BusinessPageEditor.loadData — without this mock, an unmocked pluginAPI
// rejects and the editor never leaves the loading spinner.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));

jest.mock("../BusinessPageToggle", () => ({
  BusinessPageToggle: () => <div data-testid="business-page-toggle" />,
}));
jest.mock("../BannerImageUploader", () => function MockBanner() {
  return <div data-testid="banner-uploader" />;
});
jest.mock("../CustomURLInput", () => function MockUrl() {
  return <div data-testid="custom-url-input" />;
});
jest.mock("../GalleryImageUploader", () => function MockGallery() {
  return <div data-testid="gallery-uploader" />;
});
jest.mock("../OperatingHoursEditor", () => function MockHours() {
  return <div data-testid="hours-editor" />;
});
jest.mock("../SpecialFeaturesEditor", () => function MockFeatures() {
  return <div data-testid="features-editor" />;
});
jest.mock("../GoogleBusinessSearch", () => function MockGoogle() {
  return <div data-testid="google-business-search" />;
});
jest.mock("../DashboardLockedTabView", () => function MockLocked() {
  return <div data-testid="locked-tab" />;
});
jest.mock("../ContactEditor", () => function MockContact() {
  return <div data-testid="contact-editor" />;
});

describe("BusinessPageEditor — live preview from real page tree (Finding 37)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    resetTestUrl("/business/1/dashboard?tab=business-page");
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: true,
      loading: false,
    });
    (businessApi.getBusiness as jest.Mock).mockResolvedValue({
      id: 1,
      name: "Mara Core Kitchen",
      description: "Wood-fired plates",
      address: {
        street: "42 Harbor Ave",
        city: "Buenos Aires",
        state: "CABA",
        postal_code: "1001",
        country: "AR",
      },
      custom_url: "mara-core-kitchen",
      phone: "+54 11 5555 0101",
      website: "https://mara.example",
      business_page_enabled: true,
      show_reviews: true,
      show_operating_hours: true,
      design_settings: {
        primary_color: "#1a6b6a",
        secondary_color: "#2a8b8a",
        font_family: "Inter",
        theme: "light",
        menu_layout: "grid",
        show_images: true,
        show_descriptions: true,
        header_style: "banner",
        corner_radius: "medium",
        shadow_intensity: "subtle",
        background_pattern: "none",
        pattern_opacity: 0.1,
        hero_layout: "centered",
        section_density: "comfortable",
      },
    });
    (businessApi.getBusinessGalleryImages as jest.Mock).mockResolvedValue([]);
    (businessApi.getBusinessOperatingHours as jest.Mock).mockResolvedValue([
      {
        id: 10,
        business_id: 1,
        day_of_week: 1,
        open_time: "11:00",
        close_time: "23:00",
        is_closed: false,
        created_at: "",
        updated_at: "",
      },
    ]);
    (businessApi.getBusinessSpecialFeatures as jest.Mock).mockResolvedValue([]);
  });

  it("renders the business name, hours, and contact from form state in the live preview", async () => {
    render(<BusinessPageEditor businessId={1} />);

    // Preview lives on the Look & feel (design) tab.
    const designTab = (
      await screen.findAllByRole("tab", { name: /look|design|feel/i })
    )[0];
    fireEvent.click(designTab);

    // #600: the preview host renders inside the StorefrontPreviewFrame
    // iframe (a real 380px viewport), so it is queried from the frame's
    // document rather than the dashboard document.
    const frame = (await screen.findByTestId(
      "storefront-preview-frame",
    )) as HTMLIFrameElement;
    const preview = await waitFor(() => {
      const el = frame.contentDocument?.querySelector<HTMLElement>(
        '[data-testid="business-page-live-preview"]',
      );
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });

    // The preview is aria-hidden (visual mock; must not pollute the operator
    // a11y tree), so query with { hidden: true }. Assert against textContent
    // so we prove the real form state is painted, not i18n lorem.
    await waitFor(() => {
      expect(preview.textContent).toContain("Mara Core Kitchen");
    });

    const text = preview.textContent || "";

    // Contact values from loaded form state (not i18n lorem placeholders).
    expect(text).toContain("+54 11 5555 0101");
    expect(text).toMatch(/42 Harbor Ave/);

    // Operating hours from form state — day label + open/close window.
    expect(text).toMatch(/monday/i);
    // Locale-formatted times still carry the hour digits from form state.
    expect(text).toMatch(/11:00/);
    expect(text).toMatch(/23:00|11:00\s*PM/i);

    // Hardcoded DesignCustomization lorem must not appear.
    expect(text).not.toContain("Business Name");
    expect(text).not.toContain("Tasty food & drinks");
    expect(text).not.toMatch(/Delicious Item/i);

    // And the real public-tree pieces are mounted (hero h1 + contact panel).
    expect(
      within(preview).getByRole("heading", { level: 1, hidden: true }),
    ).toHaveTextContent("Mara Core Kitchen");
  });
});
