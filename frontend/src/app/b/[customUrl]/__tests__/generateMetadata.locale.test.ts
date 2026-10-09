/** @jest-environment node */

let mockLocaleHeader: string | null = null;

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({
    get: (name: string) =>
      name === "x-payverge-locale" ? mockLocaleHeader : null,
  }),
}));

import { generateMetadata } from "../page";
import { getSiteUrl } from "@/config/publicConfig";
import { clearStorefrontBusinessSsrCache } from "@/lib/storefront/serverData";

const realFetch = global.fetch;
afterEach(() => {
  global.fetch = realFetch;
  mockLocaleHeader = null;
  jest.clearAllMocks();
  clearStorefrontBusinessSsrCache();
});

// The backend returns the (already-translated) name/description for the
// requested language. Capture the URL the server fetch hit so we can assert
// the validated ?language= param was threaded through.
function mockTranslatedBusiness() {
  return jest.fn().mockResolvedValue({
    status: 200,
    ok: true,
    json: async () => ({
      id: 1,
      name: "Café Aurora",
      description: "Bistrot de quartier", // FR copy the backend returned
      logo: "https://production.j5f7.c18.e2-3.dev/logos/aurora.png",
      banner_images: JSON.stringify([
        "https://production.j5f7.c18.e2-3.dev/banners/aurora-wide.jpg",
      ]),
      supported_languages: [{ code: "en" }, { code: "fr" }],
      default_language: "en",
    }),
  }) as unknown as typeof fetch;
}

describe("generateMetadata — per-language localization (I18N-3)", () => {
  it("threads a validated ?lang= into the server fetch so the snippet is localized", async () => {
    const fetchMock = mockTranslatedBusiness();
    global.fetch = fetchMock;
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "fr" }),
    });
    // The fetch carried ?language=fr.
    const calledUrl = (fetchMock as jest.Mock).mock.calls[0][0] as string;
    expect(calledUrl).toContain("language=fr");
    // The localized description flows into the metadata + OG.
    expect(meta.description).toBe("Bistrot de quartier");
    // Images come from the route's opengraph-image file convention.
    expect(meta.openGraph?.images).toBeUndefined();
  });

  it("ignores an unsupported/garbage lang and fetches the default language", async () => {
    const fetchMock = jest.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: 1,
        name: "Café Aurora",
        description: "Neighborhood bistro",
        supported_languages: [{ code: "en" }],
        default_language: "en",
      }),
    }) as unknown as typeof fetch;
    global.fetch = fetchMock;
    await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "zz-not-a-locale" }),
    });
    const calledUrl = (fetchMock as jest.Mock).mock.calls[0][0] as string;
    // No bogus language param leaks into the fetch.
    expect(calledUrl).not.toContain("language=");
  });

  it("does not leak Arabic into EN metadata (#686)", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: 1,
        name: "Payverge AI Pro Demo Lounge",
        description: "مطعم نموذجي في قلب المدينة",
        supported_languages: [{ code: "en" }, { code: "ar" }],
        default_language: "en",
      }),
    }) as unknown as typeof fetch;
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "payverge-ai-pro-demo-lounge" }),
      searchParams: Promise.resolve({ lang: "en" }),
    });
    expect(meta.description).not.toMatch(/[\u0600-\u06FF]/);
    expect(String(meta.openGraph?.description ?? "")).not.toMatch(
      /[\u0600-\u06FF]/,
    );
    expect(String((meta.twitter as { description?: string })?.description ?? "")).not.toMatch(
      /[\u0600-\u06FF]/,
    );
  });

  it("does not leak Arabic into zh metadata (#642)", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: 1,
        name: "Payverge AI Pro Demo Lounge",
        description: "示范餐厅，提供现代菜单",
        supported_languages: [{ code: "zh" }, { code: "ar" }],
        default_language: "en",
      }),
    }) as unknown as typeof fetch;
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "payverge-ai-pro-demo-lounge" }),
      searchParams: Promise.resolve({ lang: "zh" }),
    });
    expect(meta.description).not.toMatch(/[\u0600-\u06FF]/);
    expect(meta.description).toContain("示范");
  });

  it("threads path-locale /es/b/{slug} via x-payverge-locale (#644)", async () => {
    mockLocaleHeader = "es";
    const fetchMock = mockTranslatedBusiness();
    global.fetch = fetchMock;
    await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    const calledUrl = (fetchMock as jest.Mock).mock.calls[0][0] as string;
    expect(calledUrl).toContain("language=es");
  });

  it("self-canonicalizes /es/b/{slug} with es_ES og:locale (#864)", async () => {
    mockLocaleHeader = "es";
    global.fetch = mockTranslatedBusiness();
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "parrilla-quebracho-azul" }),
    });
    const publicBaseUrl =
      getSiteUrl();
    expect(meta.alternates?.canonical).toBe(
      `${publicBaseUrl}/es/b/parrilla-quebracho-azul`,
    );
    expect(String(meta.openGraph?.url)).toBe(
      `${publicBaseUrl}/es/b/parrilla-quebracho-azul`,
    );
    expect(meta.openGraph?.locale).toBe("es_ES");
  });

  it("keeps the canonical lang-less even when ?lang= is set (SEO-3 not regressed)", async () => {
    global.fetch = mockTranslatedBusiness();
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "fr" }),
    });
    const publicBaseUrl =
      getSiteUrl();
    expect(meta.alternates?.canonical).toBe(`${publicBaseUrl}/b/aurora`);
  });
});
