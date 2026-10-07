/** @jest-environment node */
/**
 * PG-19: /t/[code]/bill must not inherit the marketing root <title>.
 * Honest per-page metadata (business-scoped when resolvable), no guest PII.
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

describe("generateMetadata for /t/[tableCode]/bill (PG-19)", () => {
  afterAll(() => {
    global.fetch = originalFetch;
    if (originalPublicApi === undefined) delete process.env.API_URL;
    else process.env.API_URL = originalPublicApi;
  });

  it("returns a business-scoped bill title from the guest table business envelope", async () => {
    mockFetchOnce({
      business: { id: 3, name: "Mara AI Lounge", custom_url: "mara-ai-lounge" },
    });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "AI-T01" }),
    });
    expect(meta.title).toBe("Mara AI Lounge – Bill | Payverge");
    expect(String(meta.title)).not.toMatch(/AI-Powered Restaurant Management/i);
    expect(String(meta.openGraph?.url)).toContain("/t/AI-T01/bill");
    expect(meta.openGraph?.title).toContain("Mara AI Lounge");
    expect(String(meta.openGraph?.url)).not.toBe("https://payverge.io");
  });

  it("falls back to a generic bill title on non-OK response", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "MISSING" }),
    });
    expect(meta.title).toBe("Bill | Payverge");
  });

  it("does not embed the table code as a substitute for a title", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const meta = await generateMetadata({
      params: Promise.resolve({ tableCode: "TABLE-SECRET" }),
    });
    expect(String(meta.title)).not.toContain("TABLE-SECRET");
  });
});
