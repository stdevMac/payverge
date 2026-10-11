/** @jest-environment node */

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));

import { generateMetadata } from "../page";
import { getSiteUrl } from "@/config/publicConfig";
import { clearStorefrontBusinessSsrCache } from "@/lib/storefront/serverData";

const realFetch = global.fetch;
afterEach(() => {
  global.fetch = realFetch;
  jest.clearAllMocks();
  clearStorefrontBusinessSsrCache();
});

function mockBusiness(over: Record<string, unknown> = {}) {
  global.fetch = jest.fn().mockResolvedValue({
    status: 200,
    ok: true,
    json: async () => ({
      id: 1,
      name: "Cafe Aurora",
      description: "Neighborhood bistro",
      logo: "https://production.j5f7.c18.e2-3.dev/logos/aurora.png",
      banner_images: JSON.stringify([
        "https://production.j5f7.c18.e2-3.dev/banners/aurora-wide.jpg",
      ]),
      supported_languages: [{ code: "en" }, { code: "es" }],
      ...over,
    }),
  }) as unknown as typeof fetch;
}

describe("generateMetadata — SEO (canonical + OG banner)", () => {
  it("sets an absolute self-referencing canonical (SEO-3)", async () => {
    mockBusiness();
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    const publicBaseUrl =
      getSiteUrl();
    expect(meta.alternates?.canonical).toBe(`${publicBaseUrl}/b/aurora`);
    // hreflang languages still present.
    expect(meta.alternates?.languages?.["en"]).toContain("/b/aurora");
  });

  it("delegates OG/Twitter images to the opengraph-image file convention (SEO-4)", async () => {
    mockBusiness();
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    // The route's opengraph-image.tsx generates the branded per-business card;
    // metadata must NOT pin raw banner/logo URLs or it would override it.
    const og = meta.openGraph as
      | { type?: string; url?: string; images?: unknown }
      | undefined;
    expect(og?.images).toBeUndefined();
    expect(og?.type).toBe("website");
    expect(og?.url).toContain("/b/aurora");
    const twitter = meta.twitter as { card?: string; images?: unknown };
    expect(twitter.card).toBe("summary_large_image");
    expect(twitter.images).toBeUndefined();
  });

  it("noindexes the transient 'Loading business…' path so a backend blip is never indexed", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      status: 503,
      ok: false,
      json: async () => ({}),
    }) as unknown as typeof fetch;
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    expect(meta.title).toContain("Loading business");
    expect(meta.robots).toEqual(
      expect.objectContaining({ index: false, follow: false }),
    );
  });

  it("indexes published demo showrooms so converting storefronts are crawlable (#637)", async () => {
    mockBusiness({
      is_demo: true,
      kind: "demo",
      description:
        "A Payverge showcase restaurant with realistic operational demo data.",
    });
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "payverge-ai-pro-demo-lounge" }),
    });
    expect(meta.robots).toEqual(
      expect.objectContaining({ index: true, follow: true }),
    );
  });

  it("noindexes kind=test fixtures", async () => {
    mockBusiness({ kind: "test" });
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "ci-test-kitchen" }),
    });
    expect(meta.robots).toEqual(
      expect.objectContaining({ index: false, follow: false }),
    );
  });

  it("keeps real tenant storefronts explicitly indexable", async () => {
    mockBusiness({ is_demo: false, kind: "real" });
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    expect(meta.robots).toEqual(
      expect.objectContaining({ index: true, follow: true }),
    );
  });
});
