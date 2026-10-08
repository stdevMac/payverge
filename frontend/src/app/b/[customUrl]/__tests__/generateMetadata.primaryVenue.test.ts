/** @jest-environment node */
// The primary venue is served at "/" too; /b/<primary> must canonicalize to
// "/" so search engines see one URL, while OG/Twitter images keep the /b
// file-convention routes (the root has none of its own).
jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));
jest.mock("@/lib/instance/serverHome", () => ({
  ...jest.requireActual("@/lib/instance/serverHome"),
  getServerHome: async () => ({
    mode: "venue",
    primary: { id: 1, name: "Cafe Aurora", logo: "", custom_url: "aurora", city: "" },
    venues: [],
  }),
}));

import { generateMetadata } from "../page";
import { storefrontMetadata } from "../storefrontRender";
import { getSiteUrl } from "@/config/publicConfig";
import { clearStorefrontBusinessSsrCache } from "@/lib/storefront/serverData";

const realFetch = global.fetch;
beforeEach(() => {
  global.fetch = jest.fn().mockResolvedValue({
    status: 200,
    ok: true,
    json: async () => ({
      id: 1,
      name: "Cafe Aurora",
      description: "Neighborhood bistro",
      supported_languages: [{ code: "en" }, { code: "es" }],
    }),
  }) as unknown as typeof fetch;
});
afterEach(() => {
  global.fetch = realFetch;
  clearStorefrontBusinessSsrCache();
});

describe("primary venue canonical", () => {
  it("canonicalizes /b/<primary> to the site root", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    const base = getSiteUrl();
    expect(meta.alternates?.canonical).toBe(`${base}/`);
    expect((meta.openGraph as { url?: string }).url).toBe(`${base}/`);
    expect(meta.alternates?.languages?.["es"]).toBe(`${base}/es`);
  });

  it("keeps a non-primary venue on /b/<slug>", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "other" }),
    });
    expect(meta.alternates?.canonical).toBe(`${getSiteUrl()}/b/other`);
  });

  it("points root OG/Twitter images at the /b image routes", async () => {
    const meta = await storefrontMetadata({ customUrl: "aurora", pagePath: "/" });
    const base = getSiteUrl();
    expect(JSON.stringify(meta.openGraph)).toContain(`${base}/b/aurora/opengraph-image`);
    expect(JSON.stringify(meta.twitter)).toContain(`${base}/b/aurora/twitter-image`);
  });
});
