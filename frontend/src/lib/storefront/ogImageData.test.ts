/** @jest-environment node */

import {
  buildOgCardModel,
  fetchImageDataUrl,
  truncateOgText,
} from "./ogImageData";

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
});

describe("truncateOgText", () => {
  it("keeps short text intact (collapsing whitespace)", () => {
    expect(truncateOgText("  Cozy\nneighborhood   bistro ")).toBe(
      "Cozy neighborhood bistro",
    );
  });

  it("clips long text on a word boundary with an ellipsis", () => {
    const long = "word ".repeat(60).trim();
    const out = truncateOgText(long, 120);
    expect(out.length).toBeLessThanOrEqual(121);
    expect(out.endsWith("…")).toBe(true);
    expect(out).not.toMatch(/wor…$/); // not cut mid-word when a boundary exists
  });
});

describe("buildOgCardModel", () => {
  const business = {
    name: "Café Aurora",
    description: "Neighborhood bistro with a seasonal menu",
    logo: "https://cdn.example.com/logo.png",
    banner_images: JSON.stringify(["https://cdn.example.com/banner.jpg"]),
  } as any;

  it("builds the full model from a business payload", () => {
    const model = buildOgCardModel({
      business,
      customUrl: "aurora",
      baseUrl: "https://payverge.io",
      googleRating: { ratingValue: 4.7, reviewCount: 213 },
    });
    expect(model.name).toBe("Café Aurora");
    expect(model.initial).toBe("C");
    expect(model.description).toContain("bistro");
    expect(model.slugPath).toBe("payverge.io/b/aurora");
    expect(model.bannerUrl).toBe("https://cdn.example.com/banner.jpg");
    expect(model.logoUrl).toBe("https://cdn.example.com/logo.png");
    expect(model.rating).toEqual({ value: 4.7, count: 213 });
  });

  it("falls back to a branded Payverge card when the business is missing", () => {
    const model = buildOgCardModel({
      business: null,
      customUrl: "gone",
      baseUrl: "https://payverge.io",
    });
    expect(model.name).toBe("Payverge");
    expect(model.initial).toBe("P");
    expect(model.bannerUrl).toBeNull();
    expect(model.logoUrl).toBeNull();
    expect(model.rating).toBeNull();
    expect(model.slugPath).toBe("payverge.io/b/gone");
  });

  it("drops the rating when non-positive and tolerates bad banner JSON", () => {
    const model = buildOgCardModel({
      business: { ...business, banner_images: "{broken" } as any,
      customUrl: "aurora",
      baseUrl: "https://payverge.io/",
      googleRating: { ratingValue: 0, reviewCount: 12 },
    });
    expect(model.rating).toBeNull();
    expect(model.bannerUrl).toBeNull();
    expect(model.slugPath).toBe("payverge.io/b/aurora");
  });
});

describe("fetchImageDataUrl", () => {
  // Static image host from imageCspOrigins. Arbitrary CDNs are not fetched.
  const allowed = "https://images.unsplash.com/b.jpg";

  it("inlines an image response as a base64 data URL", async () => {
    const bytes = new Uint8Array([1, 2, 3, 4]);
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      headers: { get: () => "image/jpeg; charset=binary" },
      arrayBuffer: async () => bytes.buffer,
    }) as unknown as typeof fetch;
    const out = await fetchImageDataUrl(allowed);
    expect(out).toMatch(/^data:image\/jpeg;base64,/);
  });

  it("rejects non-image content types and failed fetches", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      headers: { get: () => "text/html" },
      arrayBuffer: async () => new Uint8Array([1]).buffer,
    }) as unknown as typeof fetch;
    await expect(fetchImageDataUrl(allowed)).resolves.toBeNull();

    global.fetch = jest
      .fn()
      .mockRejectedValue(new Error("down")) as unknown as typeof fetch;
    await expect(fetchImageDataUrl(allowed)).resolves.toBeNull();
  });
});
