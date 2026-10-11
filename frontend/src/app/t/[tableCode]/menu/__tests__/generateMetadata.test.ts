/** @jest-environment node */
/**
 * /t/[tableCode]/menu must not inherit the marketing root robots (index, follow).
 * Honest per-page metadata; noindex on both resolved and unresolved lookups.
 */
jest.mock("next/headers", () => ({
  headers: async () => ({ get: () => null }),
}));

import { generateMetadata } from "../layout";

const originalFetch = global.fetch;
const originalPublicApi = process.env.API_URL;

beforeEach(() => {
  process.env.API_URL = "http://api.test/api/v1";
});

function mockFetchOnce(
  payload: unknown,
  init: { ok?: boolean; status?: number } = {},
) {
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

function expectNoIndexNoFollow(meta: { robots?: unknown }) {
  const robots = meta.robots;
  if (typeof robots === "object" && robots !== null) {
    expect((robots as { index?: boolean }).index).toBe(false);
    expect((robots as { follow?: boolean }).follow).toBe(false);
  } else {
    expect(robots).toMatch(/noindex/i);
    expect(robots).toMatch(/nofollow/i);
  }
}

describe("generateMetadata for /t/[tableCode]/menu", () => {
  afterAll(() => {
    global.fetch = originalFetch;
    if (originalPublicApi === undefined) delete process.env.API_URL;
    else process.env.API_URL = originalPublicApi;
  });

  it("marks a resolved menu page noindex, nofollow", async () => {
    mockFetchOnce({
      business: { id: 3, name: "Mara AI Lounge", custom_url: "mara-ai-lounge" },
    });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "AI-T01" }),
    });
    expect(meta.title).toBe("Mara AI Lounge – Menu | Payverge");
    expect(String(meta.openGraph?.url)).toContain("/t/AI-T01/menu");
    expect(meta.openGraph?.title).toContain("Mara AI Lounge");
    expect(String(meta.openGraph?.url)).not.toBe("https://payverge.io");
    expectNoIndexNoFollow(meta);
  });

  it("marks an unresolved menu page noindex, nofollow (not bare Payverge)", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "MISSING" }),
    });
    expect(meta.title).toBe("Menu | Payverge");
    expect(meta.title).not.toBe("Payverge");
    expectNoIndexNoFollow(meta);
  });
});
