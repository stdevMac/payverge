/**
 * REV-5: pin og:image + og:site_name so a wholesale openGraph replace
 * cannot silently drop them again.
 */
import {
  OG_IMAGE_URL,
  SITE_OG_SITE_NAME,
  siteOpenGraphDefaults,
} from "@/lib/seo/openGraphImages";

function expectSiteOgDefaults(og: {
  url?: string | URL;
  siteName?: string;
  type?: string;
  images?: unknown;
} | undefined) {
  expect(og).toBeDefined();
  expect(og?.siteName).toBe(SITE_OG_SITE_NAME);
  expect(og?.type).toBe("website");
  const images = og?.images as Array<{ url?: string }> | undefined;
  expect(Array.isArray(images) && images.length > 0).toBe(true);
  expect(images?.[0]?.url).toBe(OG_IMAGE_URL);
}

describe("site openGraph defaults (REV-5)", () => {
  it("siteOpenGraphDefaults always includes siteName and images", () => {
    const og = siteOpenGraphDefaults({ url: "https://payverge.io/business/register" });
    expectSiteOgDefaults(og);
    expect(og.url).toBe("https://payverge.io/business/register");
  });
});
