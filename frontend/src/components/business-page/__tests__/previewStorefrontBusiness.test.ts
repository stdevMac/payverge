/**
 * Draft → storefront payload adapter (#591).
 *
 * The Business Page live preview mounts the REAL storefront tree, so this
 * adapter is the only thing standing between the editor's operator-shaped
 * draft state and the `PublicBusiness` wire object the storefront consumes.
 * Every mismatch here shows up as preview drift, which is exactly the class of
 * bug #591 filed.
 */

import {
  buildPreviewStorefrontBusiness,
  resolvePreviewDesignSettings,
  resolvePreviewLanguage,
  type BusinessPageLivePreviewModel,
} from "../previewStorefrontBusiness";
import { DEFAULT_DESIGN_SETTINGS } from "../designClasses";
import type { BusinessDesignSettings } from "@/api/business";

const baseModel: BusinessPageLivePreviewModel = {
  businessId: 85,
  name: "Core Kitchen",
  operatingHours: [],
};

describe("resolvePreviewLanguage", () => {
  it("prefers the storefront default language over the operator locale", () => {
    expect(resolvePreviewLanguage("es", "en")).toBe("es");
  });

  it("falls back to the operator dashboard locale when the storefront has none", () => {
    expect(resolvePreviewLanguage(undefined, "es")).toBe("es");
    expect(resolvePreviewLanguage("", "es")).toBe("es");
  });

  it("keeps a regional storefront locale that ships its own guest bundle", () => {
    expect(resolvePreviewLanguage("es-AR", "en")).toBe("es-AR");
  });

  it("falls back to the base language for an unshipped regional tag", () => {
    expect(resolvePreviewLanguage("es-419")).toBe("es");
    expect(resolvePreviewLanguage("pt_BR")).toBe("pt");
  });

  it("falls back to English when nothing resolves", () => {
    expect(resolvePreviewLanguage(undefined, undefined)).toBe("en");
    expect(resolvePreviewLanguage("klingon", "xx")).toBe("en");
  });
});

describe("resolvePreviewDesignSettings", () => {
  it("fills every storefront theme field from the shipped defaults", () => {
    const resolved = resolvePreviewDesignSettings(undefined);
    expect(resolved.primary_color).toBe(DEFAULT_DESIGN_SETTINGS.primary_color);
    expect(resolved.secondary_color).toBe(
      DEFAULT_DESIGN_SETTINGS.secondary_color,
    );
    expect(resolved.hero_layout).toBe(DEFAULT_DESIGN_SETTINGS.hero_layout);
    expect(resolved.section_density).toBe(
      DEFAULT_DESIGN_SETTINGS.section_density,
    );
    expect(typeof resolved.pattern_opacity).toBe("number");
  });

  it("lets the operator's unsaved draft win over the defaults", () => {
    const resolved = resolvePreviewDesignSettings({
      primary_color: "#ff5722",
      corner_radius: "large",
      show_images: false,
    } as Partial<BusinessDesignSettings>);
    expect(resolved.primary_color).toBe("#ff5722");
    expect(resolved.corner_radius).toBe("large");
    expect(resolved.show_images).toBe(false);
    // Untouched fields still come from the defaults, so the preview is never
    // rendered with half a theme while the editor is still loading.
    expect(resolved.secondary_color).toBe(
      DEFAULT_DESIGN_SETTINGS.secondary_color,
    );
  });

  it("keeps a non-numeric pattern_opacity from reaching the CSS variable", () => {
    const resolved = resolvePreviewDesignSettings({
      pattern_opacity: undefined,
    } as Partial<BusinessDesignSettings>);
    expect(typeof resolved.pattern_opacity).toBe("number");
  });
});

describe("buildPreviewStorefrontBusiness — wire contract", () => {
  it("serializes banner_images and social_media as JSON strings", () => {
    const business = buildPreviewStorefrontBusiness({
      ...baseModel,
      banner_images: ["https://cdn.test/a.jpg", "https://cdn.test/b.jpg"],
      social_media: { instagram: "corekitchen" },
    });

    // The public payload ships these as strings; the storefront parses them.
    expect(typeof business.banner_images).toBe("string");
    expect(typeof business.social_media).toBe("string");
    expect(JSON.parse(business.banner_images as unknown as string)).toEqual([
      "https://cdn.test/a.jpg",
      "https://cdn.test/b.jpg",
    ]);
    expect(JSON.parse(business.social_media as unknown as string)).toEqual({
      instagram: "corekitchen",
    });
  });

  it("emits parseable empty JSON when the draft has no banners or socials", () => {
    const business = buildPreviewStorefrontBusiness(baseModel);
    expect(JSON.parse(business.banner_images as unknown as string)).toEqual([]);
    expect(JSON.parse(business.social_media as unknown as string)).toEqual({});
  });

  it("carries the real business id so the preview reads the real menu", () => {
    expect(buildPreviewStorefrontBusiness(baseModel).id).toBe(85);
  });

  it("falls back to id 0 before the editor has loaded a business", () => {
    const business = buildPreviewStorefrontBusiness({
      name: "",
      operatingHours: [],
    });
    expect(business.id).toBe(0);
  });
});

describe("buildPreviewStorefrontBusiness — money and locale", () => {
  it("mirrors the saved storefront currencies", () => {
    const business = buildPreviewStorefrontBusiness({
      ...baseModel,
      defaultCurrency: "ARS",
      displayCurrency: "USD",
    });
    expect(business.default_currency).toBe("ARS");
    expect(business.display_currency).toBe("USD");
  });

  it("falls back the display currency to the default currency", () => {
    const business = buildPreviewStorefrontBusiness({
      ...baseModel,
      defaultCurrency: "ARS",
    });
    expect(business.display_currency).toBe("ARS");
  });

  it("falls back to USD when the business has no currency yet", () => {
    const business = buildPreviewStorefrontBusiness(baseModel);
    expect(business.default_currency).toBe("USD");
    expect(business.display_currency).toBe("USD");
  });

  it("passes tax and service fee rates through for menu pricing", () => {
    const business = buildPreviewStorefrontBusiness({
      ...baseModel,
      taxRate: 21,
      serviceFeeRate: 10,
    });
    expect(business.tax_rate).toBe(21);
    expect(business.service_fee_rate).toBe(10);
  });
});

describe("buildPreviewStorefrontBusiness — section visibility", () => {
  it("mirrors the draft show_* toggles onto the public payload", () => {
    const business = buildPreviewStorefrontBusiness({
      ...baseModel,
      showWelcomeMessage: true,
      showAboutStory: true,
      showGallery: true,
      showSpecialFeatures: true,
      showOperatingHours: true,
      welcomeMessage: "Bienvenidos",
      aboutStory: "Since 1998",
    });
    expect(business.show_welcome_message).toBe(true);
    expect(business.show_about_story).toBe(true);
    expect(business.show_gallery).toBe(true);
    expect(business.show_special_features).toBe(true);
    expect(business.show_operating_hours).toBe(true);
    expect(business.welcome_message).toBe("Bienvenidos");
    expect(business.about_story).toBe("Since 1998");
  });

  it("treats an unset toggle as hidden, never as undefined", () => {
    const business = buildPreviewStorefrontBusiness(baseModel);
    expect(business.show_welcome_message).toBe(false);
    expect(business.show_about_story).toBe(false);
    expect(business.show_gallery).toBe(false);
    expect(business.show_special_features).toBe(false);
    expect(business.show_operating_hours).toBe(false);
  });

  it("keeps reviews visible by default (matches the public API default)", () => {
    expect(buildPreviewStorefrontBusiness(baseModel).show_reviews).toBe(true);
    expect(
      buildPreviewStorefrontBusiness({ ...baseModel, showReviews: false })
        .show_reviews,
    ).toBe(false);
  });
});

describe("buildPreviewStorefrontBusiness — hours, exceptions, theme", () => {
  it("passes operating hours and exceptions through for open status + announcements", () => {
    const hours = [
      {
        id: 1,
        business_id: 85,
        day_of_week: 1,
        open_time: "11:00",
        close_time: "23:00",
        is_closed: false,
        created_at: "",
        updated_at: "",
      },
    ];
    const exceptions = [
      {
        id: 9,
        business_id: 85,
        exception_date: "2026-12-25",
        is_closed: true,
        label: "Christmas",
        created_at: "",
        updated_at: "",
      },
    ];
    const business = buildPreviewStorefrontBusiness({
      ...baseModel,
      operatingHours: hours,
      operatingExceptions: exceptions,
      timezone: "America/Argentina/Buenos_Aires",
    });
    expect(business.operating_hours).toEqual(hours);
    expect(business.operating_exceptions).toEqual(exceptions);
    expect(business.timezone).toBe("America/Argentina/Buenos_Aires");
  });

  it("embeds the resolved draft design settings", () => {
    const business = buildPreviewStorefrontBusiness(baseModel, {
      primary_color: "#ff5722",
    } as Partial<BusinessDesignSettings>);
    expect(business.design_settings?.primary_color).toBe("#ff5722");
  });

  it("never advertises the AI waiter — the preview must not open a session", () => {
    const business = buildPreviewStorefrontBusiness(baseModel);
    expect(business.ai_settings?.ai_enabled).toBe(false);
    expect(business.ai_settings?.business_page_ai_enabled).toBe(false);
  });

  it("marks the page enabled so the preview never renders the disabled state", () => {
    const business = buildPreviewStorefrontBusiness(baseModel);
    expect(business.page_enabled).toBe(true);
    expect(business.is_active).toBe(true);
  });
});
