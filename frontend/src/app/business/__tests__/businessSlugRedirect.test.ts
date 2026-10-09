/**
 * PG-1 — /business/<slug> Route Handler returns real 308 / 404.
 * @jest-environment node
 */

export {};

const originalFetch = global.fetch;

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

// Production reality: behind Caddy/Cloudflare, Next builds `request.url` from
// the container's internal bind address, NOT the public origin. Encoding that
// here is the whole point — the previous suite used a synthetic public origin
// (https://app.payverge.test) and therefore asserted an absolute Location that
// was only ever correct inside the test. Production shipped
// `Location: https://0.0.0.0:3000/b/<slug>`, an unreachable host.
const INTERNAL_ORIGIN = "https://0.0.0.0:3000";

function req(path: string, init?: RequestInit) {
  return new Request(`${INTERNAL_ORIGIN}${path}`, init);
}

describe("/business/<slug> route handler (PG-1)", () => {
  const prevInternal = process.env.INTERNAL_API_URL;
  const prevPublic = process.env.API_URL;

  beforeAll(() => {
    process.env.INTERNAL_API_URL = "https://api.test/api/v1";
    process.env.API_URL = "https://api.test/api/v1";
  });

  afterAll(() => {
    process.env.INTERNAL_API_URL = prevInternal;
    process.env.API_URL = prevPublic;
    global.fetch = originalFetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("308 redirects to /b/<custom_url> for a flat business shape", async () => {
    mockFetchOnce({
      id: 3,
      name: "Mara Core Kitchen",
      custom_url: "mara-core-kitchen",
    });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(req("/business/mara-core-demo"), {
      params: Promise.resolve({ businessId: "mara-core-demo" }),
    });
    expect(res.status).toBe(308);
    expect(res.headers.get("location")).toBe("/b/mara-core-kitchen");
    expect(res.headers.get("location")).not.toContain(INTERNAL_ORIGIN);
  });

  it("404 for a business without a custom_url (no numeric /b/<id> alias)", async () => {
    mockFetchOnce({ id: 66, name: "Slugless", custom_url: "" });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(req("/business/66"), {
      params: Promise.resolve({ businessId: "66" }),
    });
    expect(res.status).toBe(404);
    expect(res.headers.get("location")).toBeNull();
  });

  it("404 HTML for unknown slug (not soft-200 marketing title)", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(req("/business/zzz-not-a-real-slug-9987"), {
      params: Promise.resolve({ businessId: "zzz-not-a-real-slug-9987" }),
    });
    expect(res.status).toBe(404);
    const html = await res.text();
    expect(html).toMatch(/We couldn't find that page — Payverge/);
    expect(html).toMatch(/noindex/);
    expect(html).not.toMatch(/AI-Powered Restaurant Management System/);
    // The "Back to Payverge" link was built from url.origin and shipped
    // href="https://0.0.0.0:3000/" to production.
    expect(html).not.toContain(INTERNAL_ORIGIN);
    expect(html).toContain('href="/"');
  });

  it("uses explicit ?lang= copy before Accept-Language", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(
      req("/business/introuvable?lang=fr", {
        headers: { "Accept-Language": "ar" },
      }),
      { params: Promise.resolve({ businessId: "introuvable" }) },
    );
    const html = await res.text();

    expect(html).toContain('<html lang="fr" dir="ltr">');
    expect(html).toContain("Page introuvable");
    expect(html).not.toContain("لم نتمكن من العثور على هذه الصفحة");
  });

  it("preserves the canonical es-AR locale from Accept-Language", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(
      req("/business/no-existe", {
        headers: { "Accept-Language": "es-AR,es;q=0.9,en;q=0.8" },
      }),
      { params: Promise.resolve({ businessId: "no-existe" }) },
    );
    const html = await res.text();

    expect(html).toContain('<html lang="es-AR" dir="ltr">');
    expect(html).toContain("No pudimos encontrar esa página");
  });

  it("sets RTL document direction for Arabic Accept-Language copy", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(
      req("/business/not-found", {
        headers: { "Accept-Language": "ar,en;q=0.8" },
      }),
      { params: Promise.resolve({ businessId: "not-found" }) },
    );
    const html = await res.text();

    expect(html).toContain('<html lang="ar" dir="rtl">');
    expect(html).toContain("لم نتمكن من العثور على هذه الصفحة");
  });

  it("falls back to English copy and LTR when locale hints are absent", async () => {
    mockFetchOnce({}, { ok: false, status: 404 });
    const { GET } = await import("../[businessId]/route");
    const res = await GET(req("/business/missing"), {
      params: Promise.resolve({ businessId: "missing" }),
    });
    const html = await res.text();

    expect(html).toContain('<html lang="en" dir="ltr">');
    expect(html).toContain("We couldn't find that page");
    expect(html).toContain("Go to homepage");
  });

  it("404 when fetch throws", async () => {
    (global as { fetch?: unknown }).fetch = jest.fn(() =>
      Promise.reject(new Error("network")),
    );
    const { GET } = await import("../[businessId]/route");
    const res = await GET(req("/business/mara-core-demo"), {
      params: Promise.resolve({ businessId: "mara-core-demo" }),
    });
    expect(res.status).toBe(404);
  });

  it("aborts a hanging slug lookup and returns 404", async () => {
    const timeout = jest.spyOn(AbortSignal, "timeout").mockImplementation(() => {
      const controller = new AbortController();
      setTimeout(
        () => controller.abort(new DOMException("timeout", "TimeoutError")),
        10,
      );
      return controller.signal;
    });
    (global as { fetch?: unknown }).fetch = jest.fn(
      (_url: string, init?: RequestInit) =>
        new Promise((_resolve, reject) => {
          const signal = init?.signal;
          if (!signal) return;
          const fail = () => reject(signal.reason);
          if (signal.aborted) {
            fail();
            return;
          }
          signal.addEventListener("abort", fail);
        }),
    );
    try {
      const { GET } = await import("../[businessId]/route");
      const res = await GET(req("/business/foo"), {
        params: Promise.resolve({ businessId: "foo" }),
      });
      expect(res.status).toBe(404);
      expect(timeout).toHaveBeenCalledWith(5000);
    } finally {
      timeout.mockRestore();
    }
  });
});

export {};
