import {
  generatePatternCSS,
  getBusinessByCustomUrl,
  isNonIndexableStorefront,
  normalizePublicBusiness,
  sanitizeCssColor,
} from "@/api/publicBusiness";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

describe("public business api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("normalizes public business payload aliases and supported languages", () => {
    const business = normalizePublicBusiness({
      id: 42,
      name: "Demo Bistro",
      address: {
        street: "123 Main St",
        city: "Dubai",
        state: "Dubai",
        postal_code: "00000",
        country: "UAE",
      },
      business_page_enabled: false,
      supported_languages: [
        {
          id: 1,
          code: "en",
          name: "English",
          native_name: "English",
          is_active: true,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
    });

    expect(business.page_enabled).toBe(false);
    expect(business.address.city).toBe("Dubai");
    expect(business.supported_languages).toEqual([
      expect.objectContaining({ code: "en", name: "English" }),
    ]);
  });

  it("normalizes the public business endpoint response", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        id: 42,
        name: "Demo Bistro",
        business_page_enabled: true,
        supported_languages: ["en", "es"],
      },
    });

    const business = await getBusinessByCustomUrl("demo-bistro");

    expect(axiosInstance.get).toHaveBeenCalledWith("/business/demo-bistro", { params: {} });
    expect(business.page_enabled).toBe(true);
    expect(business.supported_languages?.map((language) => language.code)).toEqual(["en", "es"]);
  });
});

describe("isNonIndexableStorefront", () => {
  it("noindexes kind=test fixtures but keeps published demo showrooms indexable (#637)", () => {
    expect(isNonIndexableStorefront({ kind: "test" })).toBe(true);
    expect(isNonIndexableStorefront({ is_demo: true })).toBe(false);
    expect(isNonIndexableStorefront({ kind: "demo" })).toBe(false);
    expect(isNonIndexableStorefront({ is_demo: false, kind: "real" })).toBe(
      false,
    );
    expect(isNonIndexableStorefront({ custom_url: "aurora" } as any)).toBe(
      false,
    );
  });
});

describe("sanitizeCssColor", () => {
  it("passes valid hex through", () => {
    expect(sanitizeCssColor("#1a6b6a")).toBe("#1a6b6a");
    expect(sanitizeCssColor("#fff")).toBe("#fff");
    expect(sanitizeCssColor("#11223344")).toBe("#11223344");
  });
  it("falls back on injection payloads", () => {
    expect(sanitizeCssColor("red; background-image: url(evil)")).toBe("#000000");
    expect(sanitizeCssColor("url(javascript:alert(1))")).toBe("#000000");
    expect(sanitizeCssColor("")).toBe("#000000");
  });
});

describe("generatePatternCSS", () => {
  it("never emits an unsanitized color", () => {
    const css = generatePatternCSS("stripes", "red; background-image: url(evil)");
    expect(css).not.toContain("url(evil)");
    // The sanitized fallback color is percent-encoded inside the SVG data URI.
    expect(css).toContain(encodeURIComponent("#000000"));
  });

  it("returns an empty string for unknown patterns", () => {
    expect(generatePatternCSS("does-not-exist", "#1a6b6a")).toBe("");
    expect(generatePatternCSS("none", "#1a6b6a")).toBe("");
  });

  it("builds background-image + opacity from the shared storefront artwork", () => {
    const css = generatePatternCSS("dots", "#1a6b6a", 0.3);
    expect(css).toContain("background-image:");
    expect(css).toContain("data:image/svg+xml");
    expect(css).toContain("background-repeat: repeat");
    expect(css).toContain("opacity: 0.3");
    // Same artwork the public page's <pattern> defs use (dots tile body).
    expect(css).toContain(encodeURIComponent('<circle cx="10" cy="10" r="2"'));
  });
});
