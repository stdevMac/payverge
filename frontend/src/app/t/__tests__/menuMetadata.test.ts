// #864: generateMetadata reads x-payverge-locale via next/headers, which
// throws outside a request scope. Null header falls back to defaultLocale.
jest.mock("next/headers", () => ({
  headers: async () => ({ get: () => null }),
}));

import { generateMetadata } from "../[tableCode]/menu/layout";

const originalFetch = global.fetch;
const originalPublicApi = process.env.API_URL;

beforeEach(() => {
  process.env.API_URL = "http://api.test/api/v1";
});

function mockFetchOnce(payload: unknown, init: { ok?: boolean; status?: number } = {}) {
  const ok = init.ok ?? true;
  const status = init.status ?? (ok ? 200 : 404);
  (global as { fetch?: unknown }).fetch = jest.fn(() =>
    Promise.resolve({
      ok,
      status,
      json: () => Promise.resolve(payload),
    } as Response),
  );
}

describe("generateMetadata for /t/{tableCode}/menu", () => {
  afterAll(() => {
    global.fetch = originalFetch;
    if (originalPublicApi === undefined) delete process.env.API_URL;
    else process.env.API_URL = originalPublicApi;
  });

  it("unwraps the `business` envelope and returns a business-scoped title", async () => {
    // Production response shape from /api/v1/guest/table/:code/business is
    // { business: { name, ... }, business_languages, ... } — not flat.
    // This test pins that contract so we never regress to the generic title.
    mockFetchOnce({
      business: { id: 3, name: "Mara AI Lounge", custom_url: "mara-ai-lounge" },
      business_languages: [],
      supported_languages: [],
    });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "AI-T01" }),
    });
    expect(meta.title).toBe("Mara AI Lounge – Menu | Payverge");
    expect(meta.robots).toEqual({ index: false, follow: false });
  });

  it("falls back to a generic title on non-OK response", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "MISSING" }),
    });
    expect(meta.title).toBe("Menu | Payverge");
    expect(meta.robots).toEqual({ index: false, follow: false });
  });

  it("falls back to a generic title when fetch throws", async () => {
    (global as { fetch?: unknown }).fetch = jest.fn(() =>
      Promise.reject(new Error("network")),
    );
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "AI-T01" }),
    });
    expect(meta.title).toBe("Menu | Payverge");
  });

  it("bounds metadata lookup with an abort signal so the route cannot hang", async () => {
    const fetchMock = jest.fn(
      (_input: RequestInfo | URL, _init?: RequestInit) =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ business: { name: "Demo" } }),
        } as Response),
    );
    (global as { fetch?: unknown }).fetch = fetchMock;
    await generateMetadata({
      params: Promise.resolve({ tableCode: "AI-T01" }),
    });
    expect(fetchMock).toHaveBeenCalled();
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit | undefined;
    expect(init?.signal).toBeDefined();
  });
});
