/** @jest-environment node */

import {
  lookupStorefront,
  matchStorefrontRoute,
} from "./lookupStorefront";

const realFetch = global.fetch;
const originalInternal = process.env.INTERNAL_API_URL;
const originalPublic = process.env.API_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (originalInternal === undefined) delete process.env.INTERNAL_API_URL;
  else process.env.INTERNAL_API_URL = originalInternal;
  if (originalPublic === undefined) delete process.env.API_URL;
  else process.env.API_URL = originalPublic;
});

describe("matchStorefrontRoute", () => {
  it("matches /b/{slug} and locale-prefixed storefronts", () => {
    expect(matchStorefrontRoute("/b/demo-75-ai-pro")).toEqual({
      slug: "demo-75-ai-pro",
    });
    expect(matchStorefrontRoute("/es/b/payverge-ai-pro-demo-lounge")).toEqual({
      slug: "payverge-ai-pro-demo-lounge",
    });
    expect(matchStorefrontRoute("/es-ar/b/aurora")).toEqual({ slug: "aurora" });
  });

  it("does not intercept OG/Twitter image file-convention paths", () => {
    expect(matchStorefrontRoute("/b/aurora/opengraph-image")).toBeNull();
    expect(matchStorefrontRoute("/b/aurora/twitter-image")).toBeNull();
  });
});

describe("lookupStorefront", () => {
  it("treats an empty API URL as not_found without fetching", async () => {
    delete process.env.INTERNAL_API_URL;
    delete process.env.API_URL;
    global.fetch = jest.fn() as unknown as typeof fetch;

    await expect(lookupStorefront("missing")).resolves.toEqual({
      kind: "not_found",
    });
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("treats HTTP 404 as not_found", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: "Business not found" }),
    }) as unknown as typeof fetch;

    await expect(lookupStorefront("demo-75-ai-pro")).resolves.toEqual({
      kind: "not_found",
    });
  });

  it("treats 5xx as unavailable so a blip is not a 404", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    await expect(lookupStorefront("aurora")).resolves.toEqual({
      kind: "unavailable",
    });
  });

  it("returns found for a live published storefront", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 1, name: "Lounge", custom_url: "aurora" }),
    }) as unknown as typeof fetch;

    await expect(lookupStorefront("aurora")).resolves.toEqual({
      kind: "found",
    });
  });

  it("exposes venue default_language so bare /b/ can honor it (#861)", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        id: 142,
        custom_url: "parrilla-quebracho-azul",
        default_language: "es",
      }),
    }) as unknown as typeof fetch;

    await expect(lookupStorefront("parrilla-quebracho-azul")).resolves.toEqual({
      kind: "found",
      defaultLanguage: "es",
    });
  });
});
