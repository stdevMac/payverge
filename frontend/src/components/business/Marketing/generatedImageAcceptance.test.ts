/** @jest-environment jsdom */

import {
  acceptGeneratedMarketingImage,
  applyGeneratedImageDrafts,
  browserReadablePhotoUrl,
  generationRetryWarning,
  isProxyableImageUrl,
  loadGeneratedImageDraft,
  proxyableImageHosts,
  persistGeneratedImageDraft,
  preflightBrowserImage,
  toSameOriginMediaUrl,
  type ImagePreflightResult,
} from "./generatedImageAcceptance";
import type { CampaignSuggestion } from "@/api/marketing";

// The deployment's own object-storage CDN, declared via MEDIA_ORIGINS.
const PROVIDER_URL = "https://media.example.test/menu_items/ai_generated/combo.jpg";
const FOREIGN_URL = "https://cdn.example/generated.png";

const savedMediaOrigins = process.env.MEDIA_ORIGINS;
beforeAll(() => {
  process.env.MEDIA_ORIGINS = "https://media.example.test";
});
afterAll(() => {
  if (savedMediaOrigins === undefined) delete process.env.MEDIA_ORIGINS;
  else process.env.MEDIA_ORIGINS = savedMediaOrigins;
});

describe("proxyableImageHosts", () => {
  it("is the stock host plus MEDIA_ORIGINS, never a vendor CDN by default", () => {
    expect(proxyableImageHosts("")).toEqual(["images.unsplash.com"]);
    expect(
      proxyableImageHosts(
        "https://cdn.a.test, b.test, http://plain.test, https://u:p@creds.test, https://port.test:8443, ::bad",
      ),
    ).toEqual(["images.unsplash.com", "cdn.a.test", "b.test"]);
  });

  it("follows the runtime MEDIA_ORIGINS", () => {
    expect(isProxyableImageUrl(PROVIDER_URL)).toBe(true);
    expect(isProxyableImageUrl("https://images.payverge.io/x.jpg")).toBe(false);
    expect(isProxyableImageUrl("https://media.example.test:444/x.jpg")).toBe(false);
  });
});

function readable(
  url: string,
  width = 800,
  height = 1000,
): ImagePreflightResult {
  return { status: "readable", url, width, height };
}

describe("toSameOriginMediaUrl", () => {
  it("rewrites a first-party CDN URL onto the same-origin media proxy", () => {
    expect(toSameOriginMediaUrl(PROVIDER_URL)).toBe(
      `/api/marketing-media?u=${encodeURIComponent(PROVIDER_URL)}`,
    );
  });

  it("does not rewrite an untrusted cross-origin host", () => {
    expect(toSameOriginMediaUrl(FOREIGN_URL)).toBeNull();
    expect(isProxyableImageUrl(FOREIGN_URL)).toBe(false);
  });
});

describe("preflightBrowserImage", () => {
  it("reports decode_failed when the image loads but decode() rejects", async () => {
    const result = await preflightBrowserImage(PROVIDER_URL, async () => ({
      naturalWidth: 800,
      naturalHeight: 1000,
      decode: async () => {
        throw new Error("broken image");
      },
    }));
    expect(result).toEqual({ status: "decode_failed", url: PROVIDER_URL });
  });

  it("reports readable dimensions when load and decode succeed", async () => {
    const result = await preflightBrowserImage(PROVIDER_URL, async () => ({
      naturalWidth: 640,
      naturalHeight: 800,
      decode: async () => undefined,
    }));
    expect(result).toEqual({
      status: "readable",
      url: PROVIDER_URL,
      width: 640,
      height: 800,
    });
  });

  it("reports cors_blocked when the browser cannot load the image", async () => {
    const result = await preflightBrowserImage(PROVIDER_URL, async () => null);
    expect(result).toEqual({ status: "cors_blocked", url: PROVIDER_URL });
  });
});

describe("acceptGeneratedMarketingImage", () => {
  it("does not announce success when decode fails", async () => {
    const persist = jest.fn();
    const accepted = await acceptGeneratedMarketingImage({
      providerUrl: PROVIDER_URL,
      businessId: "42",
      suggestionId: "1:combo_deal:date-night",
      preflight: async (url) => ({ status: "decode_failed", url }),
      persist,
    });
    expect(accepted).toEqual({ ok: false, reason: "decode_failed" });
    expect(persist).not.toHaveBeenCalled();
  });

  it("proxies a CORS-blocked first-party URL, then persists the readable asset", async () => {
    const persist = jest.fn();
    const preflight = jest.fn(async (url: string): Promise<ImagePreflightResult> => {
      if (url.startsWith("/api/marketing-media")) {
        return readable(url, 1024, 1280);
      }
      return { status: "cors_blocked", url };
    });

    const accepted = await acceptGeneratedMarketingImage({
      providerUrl: PROVIDER_URL,
      businessId: "42",
      suggestionId: "1:combo_deal:date-night",
      preflight,
      persist,
    });

    expect(accepted.ok).toBe(true);
    if (!accepted.ok) return;
    expect(accepted.draft.url).toBe(toSameOriginMediaUrl(PROVIDER_URL));
    expect(accepted.draft.providerUrl).toBe(PROVIDER_URL);
    expect(accepted.draft.imageSource).toBe("generated");
    expect(accepted.draft.width).toBe(1024);
    expect(accepted.draft.height).toBe(1280);
    expect(accepted.draft.status).toBe("ready");
    expect(persist).toHaveBeenCalledWith(accepted.draft);
  });

  it("does not announce success when persist fails after a readable decode", async () => {
    const accepted = await acceptGeneratedMarketingImage({
      providerUrl: PROVIDER_URL,
      businessId: "42",
      suggestionId: "1:combo_deal:date-night",
      preflight: async (url) => readable(url),
      persist: () => {
        throw new Error("quota");
      },
    });
    expect(accepted).toEqual({ ok: false, reason: "persist_failed" });
  });
});

describe("generated image draft persist + reload", () => {
  it("restores the exact generated asset after reload without another generation", () => {
    persistGeneratedImageDraft({
      suggestionId: "1:combo_deal:date-night",
      businessId: "42",
      url: `/api/marketing-media?u=${encodeURIComponent(PROVIDER_URL)}`,
      providerUrl: PROVIDER_URL,
      imageSource: "generated",
      width: 800,
      height: 1000,
      status: "ready",
      persistedAt: 1,
    });

    const restored = loadGeneratedImageDraft("42", "1:combo_deal:date-night");
    expect(restored?.url).toBe(
      `/api/marketing-media?u=${encodeURIComponent(PROVIDER_URL)}`,
    );
    expect(restored?.imageSource).toBe("generated");

    const suggestions = applyGeneratedImageDrafts(
      [
        {
          id: "1:combo_deal:date-night",
          play: "combo_deal",
          title: "Promote Date Night",
          why_data: "",
          why_factors: [{ key: "photo_ready", value: "1" }],
          source: "bundles",
          copy_angle: "",
          rank: 1,
          image_url: "https://images.unsplash.com/stale.jpg",
          image_source: "bundle",
        } as CampaignSuggestion,
      ],
      "42",
    );

    expect(suggestions[0].image_url).toBe(restored?.url);
    expect(suggestions[0].image_source).toBe("generated");
  });
});

describe("generationRetryWarning", () => {
  it("warns before another paid generation when a ready generated draft exists", () => {
    expect(
      generationRetryWarning({
        hasReadyGeneratedDraft: true,
        lastAcceptanceFailed: false,
        imageSource: "generated",
      }),
    ).toBe("already_ready");
  });

  it("warns when the last paid generation could not be shown", () => {
    expect(
      generationRetryWarning({
        hasReadyGeneratedDraft: false,
        lastAcceptanceFailed: true,
        imageSource: "bundle",
      }),
    ).toBe("failed_delivery");
  });

  it("does not warn on the first unpaid-alternative generation", () => {
    expect(
      generationRetryWarning({
        hasReadyGeneratedDraft: false,
        lastAcceptanceFailed: false,
        imageSource: "bundle",
      }),
    ).toBe("none");
  });
});

describe("browserReadablePhotoUrl", () => {
  it("keeps a same-origin proxy URL as-is and rewrites a first-party CDN URL", () => {
    const proxied = toSameOriginMediaUrl(PROVIDER_URL)!;
    expect(browserReadablePhotoUrl(proxied)).toBe(proxied);
    expect(browserReadablePhotoUrl(PROVIDER_URL)).toBe(proxied);
  });
});
