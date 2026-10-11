/** @jest-environment node */
import {
  clearStorefrontBusinessSsrCache,
  fetchStorefrontBusiness,
  fetchStorefrontGoogleRating,
  fetchStorefrontMenuPayload,
  STOREFRONT_GOOGLE_RATING_REVALIDATE_SECONDS,
  STOREFRONT_MENU_FETCH_TIMEOUT_MS,
} from "./serverData";

const realFetch = global.fetch;
const originalPublic = process.env.API_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (originalPublic === undefined) delete process.env.API_URL;
  else process.env.API_URL = originalPublic;
  clearStorefrontBusinessSsrCache();
});

describe("fetchStorefrontBusiness", () => {
  it("retries a 503 and returns the live venue instead of unavailable", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 503, json: async () => ({}) })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          id: 86,
          name: "Payverge AI Pro Demo Lounge",
          custom_url: "payverge-ai-pro-demo-lounge",
        }),
      }) as unknown as typeof fetch;

    const result = await fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    expect(result.business?.name).toBe("Payverge AI Pro Demo Lounge");
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("coalesces concurrent SSR fetches for the same live slug (#685)", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    let resolveFetch: ((value: unknown) => void) | undefined;
    global.fetch = jest.fn().mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    ) as unknown as typeof fetch;

    const first = fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    const second = fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    expect(global.fetch).toHaveBeenCalledTimes(1);

    resolveFetch?.({
      ok: true,
      status: 200,
      json: async () => ({
        id: 86,
        name: "Payverge AI Pro Demo Lounge",
        custom_url: "payverge-ai-pro-demo-lounge",
      }),
    });

    const [a, b] = await Promise.all([first, second]);
    expect(a.business?.name).toBe("Payverge AI Pro Demo Lounge");
    expect(b.business?.name).toBe("Payverge AI Pro Demo Lounge");
    expect(global.fetch).toHaveBeenCalledTimes(1);
  });

  it("serves last-known-good after a later 503 instead of unavailable (#685)", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          id: 86,
          name: "Payverge AI Pro Demo Lounge",
          custom_url: "payverge-ai-pro-demo-lounge",
        }),
      })
      .mockResolvedValue({
        ok: false,
        status: 503,
        json: async () => ({}),
      }) as unknown as typeof fetch;

    const first = await fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    expect(first.business?.name).toBe("Payverge AI Pro Demo Lounge");

    const later = await fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    expect(later.business?.name).toBe("Payverge AI Pro Demo Lounge");
    expect("reason" in later ? later.reason : undefined).toBeUndefined();
  });

  it("drops last-known-good on a confirmed 404", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          id: 86,
          name: "Payverge AI Pro Demo Lounge",
          custom_url: "payverge-ai-pro-demo-lounge",
        }),
      })
      .mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({}),
      }) as unknown as typeof fetch;

    const first = await fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    expect(first.business?.name).toBe("Payverge AI Pro Demo Lounge");

    const missing = await fetchStorefrontBusiness("payverge-ai-pro-demo-lounge");
    expect(missing.business).toBeNull();
    expect("reason" in missing ? missing.reason : undefined).toBe("not_found");
  });
});

describe("fetchStorefrontMenuPayload", () => {
  it("returns null when the menu fetch times out", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    const timeout = jest.spyOn(AbortSignal, "timeout").mockImplementation(() => {
      const controller = new AbortController();
      setTimeout(
        () => controller.abort(new DOMException("timeout", "TimeoutError")),
        10,
      );
      return controller.signal;
    });
    global.fetch = jest.fn().mockImplementation((_url: string, init?: RequestInit) => {
      return new Promise((_resolve, reject) => {
        const signal = init?.signal;
        if (!signal) return;
        const fail = () => reject(signal.reason);
        if (signal.aborted) {
          fail();
          return;
        }
        signal.addEventListener("abort", fail);
      });
    }) as unknown as typeof fetch;

    try {
      await expect(fetchStorefrontMenuPayload("demo-venue")).resolves.toBeNull();
      expect(timeout).toHaveBeenCalledWith(STOREFRONT_MENU_FETCH_TIMEOUT_MS);
    } finally {
      timeout.mockRestore();
    }
  });
});

describe("fetchStorefrontGoogleRating", () => {
  it("revalidates the Google rating instead of fetching it on every render", async () => {
    process.env.NEXT_PUBLIC_API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        place_details: { rating: 4.6, user_ratings_total: 120 },
      }),
    }) as unknown as typeof fetch;

    const rating = await fetchStorefrontGoogleRating("demo-venue");
    expect(rating).toEqual({ ratingValue: 4.6, reviewCount: 120 });

    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe(
      "http://api.test/api/v1/business/demo-venue/google/details",
    );
    expect(init).toEqual({
      next: { revalidate: STOREFRONT_GOOGLE_RATING_REVALIDATE_SECONDS },
    });
    expect(init).not.toHaveProperty("cache");
    expect(STOREFRONT_GOOGLE_RATING_REVALIDATE_SECONDS).toBeGreaterThanOrEqual(
      6 * 60 * 60,
    );
    expect(STOREFRONT_GOOGLE_RATING_REVALIDATE_SECONDS).toBeLessThanOrEqual(
      24 * 60 * 60,
    );
  });

  it("returns null when the backend reports no rating", async () => {
    process.env.NEXT_PUBLIC_API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        place_details: null,
        error_code: "upstream_unavailable",
      }),
    }) as unknown as typeof fetch;

    await expect(fetchStorefrontGoogleRating("demo-venue")).resolves.toBeNull();
  });
});
