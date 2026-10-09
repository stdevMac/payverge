import { NextRequest } from "next/server";
import { config, middleware } from "@/middleware";
import { getRequestLocale } from "@/utils/requestLocale";

describe("middleware", () => {
  it("builds the CSP from runtime config: MEDIA_ORIGINS in img-src, no baked-in hosts", async () => {
    const saved = process.env.MEDIA_ORIGINS;
    process.env.MEDIA_ORIGINS = "https://media.restaurant.example";
    try {
      const response = await middleware(
        new NextRequest("https://payverge.io/business/42/dashboard"),
      );
      const csp = response.headers.get("Content-Security-Policy") ?? "";
      expect(csp).toMatch(/img-src[^;]*https:\/\/media\.restaurant\.example/);
      expect(csp).not.toContain("api.payverge.io");
      expect(csp).not.toContain("images.payverge.io");
      expect(csp).not.toContain("cloudflareinsights");
    } finally {
      if (saved === undefined) delete process.env.MEDIA_ORIGINS;
      else process.env.MEDIA_ORIGINS = saved;
    }
  });

  it("runs on the Node.js runtime and keeps the API/media proxies out of the matcher", () => {
    expect(config.runtime).toBe("nodejs");
    const matcher = new RegExp(`^${config.matcher[0]}$`);
    expect(matcher.test("/business/42/dashboard")).toBe(true);
    expect(matcher.test("/t/demo-table/menu")).toBe(true);
    expect(matcher.test("/apiary")).toBe(true);
    expect(matcher.test("/api/v1/orders")).toBe(false);
    expect(matcher.test("/media/business/1/a.webp")).toBe(false);
    expect(matcher.test("/_next/static/chunk.js")).toBe(false);
  });

  it("allows images.unsplash.com in img-src so guest offer photos load (issue 346)", async () => {
    const response = await middleware(
      new NextRequest("https://payverge.io/t/demo-table/menu"),
    );
    const csp = response.headers.get("Content-Security-Policy") ?? "";
    expect(csp).toMatch(/img-src[^;]*https:\/\/images\.unsplash\.com/);
  });

  it("passes public routes through without a session cookie", async () => {
    const request = new NextRequest("https://app.payverge.test/staff/login");
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("allows Next's local dev runtime to hydrate under the nonce CSP", async () => {
    const request = new NextRequest("https://app.payverge.test/");
    const response = await middleware(request);

    const csp = response.headers.get("Content-Security-Policy") ?? "";
    expect(csp).toContain("script-src");
    expect(csp).toContain("'strict-dynamic'");
    expect(csp).toContain("'unsafe-eval'");
  });

  it("passes public image assets through without a session cookie", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/images/PayvergeLogo.png",
    );
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  // #288 redesign / Chopper merge blocker with #314: edge must NOT require
  // frontend-visible session cookies. Host-only COOKIE_DOMAIN (api.payverge.io)
  // means payverge.io never sees session_token — a cookie bounce 307s admins.
  it("passes /admin through without a frontend-host session cookie (SEC-5 host-only)", async () => {
    const request = new NextRequest("https://payverge.io/admin");
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
    expect(response.status).not.toBe(307);
  });

  it("passes nested /admin/* through without a frontend-host session cookie", async () => {
    const request = new NextRequest("https://payverge.io/admin/users");
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("strips absolute next= open-redirect bait at the edge (#296)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/register?next=https://evil.example",
    );
    const response = await middleware(request);

    expect(response.status).toBe(307);
    const location = response.headers.get("location") ?? "";
    expect(location).toContain("/register");
    expect(location).not.toContain("evil.example");
    expect(location).not.toContain("next=");
  });

  it("keeps style-src unsafe-inline exception documented for NextUI (#289)", async () => {
    const response = await middleware(
      new NextRequest("https://payverge.io/business/42/dashboard"),
    );
    const csp = response.headers.get("Content-Security-Policy") ?? "";
    expect(csp).toMatch(/style-src[^;]*'unsafe-inline'/);
    expect(csp).not.toMatch(/style-src[^;]*'nonce-/);
  });

  it("passes /dashboard through without a session cookie because AuthGate handles it", async () => {
    const request = new NextRequest("https://app.payverge.test/dashboard");
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  // Presence of a (stale) frontend cookie must also not redirect — edge is
  // not an auth gate for /admin either way.
  it("passes /admin through even when a session_token cookie is present", async () => {
    const request = new NextRequest("https://payverge.io/admin", {
      headers: { cookie: "session_token=abc" },
    });
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  // PG-1: a blind edge 308 of /business/<segment> → /b/<segment> 404s whenever
  // the path segment is not the canonical custom_url (e.g. an operator alias or
  // legacy demo slug). The Route Handler at app/business/[businessId]/route.ts
  // does the SSR fetch and returns 308/404 — middleware must pass single-segment
  // /business/* through so that handler can run.
  it("passes /business/<slug> through (no edge 308) so the route can resolve custom_url (PG-1)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/business/mara-core-demo",
    );
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
    expect(response.status).not.toBe(308);
  });

  it("also passes a slug that happens to equal custom_url (page still owns the redirect)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/business/mara-core-kitchen",
    );
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("does NOT redirect /business/register (reserved slug)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/business/register",
    );
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("does NOT redirect nested /business/register routes", async () => {
    const successRequest = new NextRequest(
      "https://app.payverge.test/business/register/success?session_id=cs_test_123",
    );
    const successResponse = await middleware(successRequest);

    expect(successResponse.headers.get("location")).toBeNull();
    expect(successResponse.headers.get("x-middleware-next")).toBe("1");

    const legacyRequest = new NextRequest(
      "https://app.payverge.test/business/register/dashboard?session_id=cs_test_123&tab=success",
    );
    const legacyResponse = await middleware(legacyRequest);

    expect(legacyResponse.headers.get("location")).toBeNull();
    expect(legacyResponse.headers.get("x-middleware-next")).toBe("1");
  });

  it("still rewrites /business/<id>/<segment> to the dashboard with ?tab=<segment>", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/business/mara-core-demo/menu",
    );
    const response = await middleware(request);

    expect(response.status).toBe(307);
    const location = response.headers.get("location") ?? "";
    expect(location).toContain("/business/mara-core-demo/dashboard");
    expect(location).toContain("tab=menu");
  });

  it("rewrites the legacy /business/<id>/crm page to the dashboard CRM tab", async () => {
    // The standalone CRM page was removed; crm is no longer a passthrough
    // segment, so it must rewrite like any other in-page tab.
    const request = new NextRequest(
      "https://app.payverge.test/business/mara-core-demo/crm",
    );
    const response = await middleware(request);

    expect(response.status).toBe(307);
    const location = response.headers.get("location") ?? "";
    expect(location).toContain("/business/mara-core-demo/dashboard");
    expect(location).toContain("tab=crm");
  });

  it("derives the request locale from the URL path", async () => {
    expect(getRequestLocale("/es/refund")).toBe("es");
    expect(getRequestLocale("/refund")).toBe("en");
  });

  it("ignores the persisted locale cookie on unprefixed legal routes (#37)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/privacy-policy",
      { headers: { cookie: "payverge_locale=es-AR" } },
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("prefers an explicit route locale over the persisted locale cookie", async () => {
    const request = new NextRequest("https://app.payverge.test/es/refund", {
      headers: { cookie: "payverge_locale=es-AR" },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("es");
  });

  it("keeps unprefixed public page URLs English despite Accept-Language (#37)", async () => {
    const request = new NextRequest("https://app.payverge.test/privacy-policy", {
      headers: { "accept-language": "es-AR,es;q=0.9,en;q=0.8" },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("keeps unprefixed /terms-and-conditions English despite a Spanish locale cookie (#37)", async () => {
    const request = new NextRequest("https://app.payverge.test/terms-and-conditions", {
      headers: { cookie: "payverge_locale=es" },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("does not let Accept-Language pick the operator dashboard locale (#617)", async () => {
    const request = new NextRequest("https://app.payverge.test/dashboard", {
      headers: { "accept-language": "es-AR,es;q=0.9,en;q=0.8" },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("does not clobber payverge_locale when prefetching /es (#617)", async () => {
    const request = new NextRequest("https://app.payverge.test/es", {
      headers: {
        cookie: "payverge_locale=en",
        "Next-Router-Prefetch": "1",
      },
    });
    const response = await middleware(request);
    expect(response.cookies.get("payverge_locale")).toBeUndefined();
    expect(forwardedLocale(response)).toBe("es");
  });

  it("ignores ?lang= on unprefixed public page URLs (path is source of truth)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/privacy-policy?lang=ES-ar",
      {
        headers: {
          cookie: "payverge_locale=en",
          "accept-language": "en;q=1",
        },
      },
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it.each([
    "payverge_locale=ES",
    "payverge_locale=es",
  ])("normalizes locale cookie on dashboard routes: %s", async (cookie) => {
    const request = new NextRequest("https://app.payverge.test/dashboard", {
      headers: { cookie },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("es");
  });

  it("serves bare /es as the Spanish marketing homepage (no blog redirect) (#12)", async () => {
    const request = new NextRequest("https://app.payverge.test/es");
    const response = await middleware(request);

    expect(response.status).toBe(200);
    expect(forwardedLocale(response)).toBe("es");
    // Dedicated es/page.tsx — no rewrite/redirect away from /es.
    expect(response.headers.get("location")).toBeNull();
  });

  it("persists payverge_locale when entering through /es or /es-ar (#402)", async () => {
    const es = await middleware(new NextRequest("https://app.payverge.test/es"));
    expect(es.cookies.get("payverge_locale")?.value).toBe("es");

    const esAr = await middleware(
      new NextRequest("https://app.payverge.test/es-ar"),
    );
    expect(esAr.cookies.get("payverge_locale")?.value).toBe("es-AR");
  });

  it.each([
    "/business/register",
    "/forgot-password",
    "/staff/login",
  ])("honors payverge_locale on unprefixed auth route %s (#402)", async (path) => {
    const request = new NextRequest(`https://app.payverge.test${path}`, {
      headers: { cookie: "payverge_locale=es" },
    });
    expect(forwardedLocale(await middleware(request))).toBe("es");
  });

  it("rewrites /es/terms-and-conditions to the English tools page while keeping locale es (#11)", async () => {
    const request = new NextRequest("https://app.payverge.test/es/terms-and-conditions");
    const response = await middleware(request);

    expect(response.status).toBe(200);
    expect(forwardedLocale(response)).toBe("es");
  });

  it("redirects bare /business to /business/register so the URL is not a 404", async () => {
    const request = new NextRequest("https://app.payverge.test/business");
    const response = await middleware(request);

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://app.payverge.test/business/register",
    );
  });

  it("redirects bare /staff to /staff/login so the URL is not a 404", async () => {
    const request = new NextRequest("https://app.payverge.test/staff");
    const response = await middleware(request);

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://app.payverge.test/staff/login",
    );
  });

  it("does NOT redirect /staff/login (existing public route)", async () => {
    const request = new NextRequest("https://app.payverge.test/staff/login");
    const response = await middleware(request);

    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("no longer redirects the retired /profile path", async () => {
    for (const path of ["/profile", "/profile?tab=notifications", "/profile/42"]) {
      const response = await middleware(
        new NextRequest(`https://app.payverge.test${path}`),
      );
      expect(response.headers.get("location")).toBeNull();
    }
  });

  // NextResponse.next({ request: { headers } }) surfaces forwarded request
  // header overrides on the response as x-middleware-request-<name>.
  const forwardedLocale = (response: { headers: { get: (name: string) => string | null } }): string | null =>
    response.headers.get("x-middleware-request-x-payverge-locale");

  it("rewrites /es/app and /es-ar/app instead of 404 (#914)", async () => {
    const es = await middleware(new NextRequest("https://payverge.io/es/app"));
    expect(es.status).not.toBe(404);
    expect(es.headers.get("x-middleware-request-x-payverge-locale")).toBe("es");
    expect(es.headers.get("x-middleware-rewrite") ?? "").toMatch(/\/app\?lang=es$/);

    const esAr = await middleware(
      new NextRequest("https://payverge.io/es-ar/app"),
    );
    expect(esAr.status).not.toBe(404);
    expect(esAr.headers.get("x-middleware-request-x-payverge-locale")).toBe(
      "es-AR",
    );
    expect(esAr.headers.get("x-middleware-rewrite") ?? "").toMatch(
      /\/app\?lang=es-ar$/,
    );
  });

  it("forwards es and es-AR on marketing locale prefixes (#26)", async () => {
    expect(
      forwardedLocale(
        await middleware(new NextRequest("https://app.payverge.test/es")),
      ),
    ).toBe("es");
    expect(
      forwardedLocale(
        await middleware(new NextRequest("https://app.payverge.test/es-ar/privacy-policy")),
      ),
    ).toBe("es-AR");
  });

  it("sets x-payverge-locale to the canonical storefront code for /b/<slug>?lang=ar (RTL SSR)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/b/mara-core-kitchen?lang=ar",
    );
    const response = await middleware(request);

    // No redirect — the guest page renders; only the SSR locale header changes.
    expect(response.headers.get("location")).toBeNull();
    expect(forwardedLocale(response)).toBe("ar");
  });

  it("normalises a lowercased guest ?lang=es-ar to the canonical es-AR header on /b/", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/b/mara-core-kitchen?lang=es-ar",
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("es-AR");
  });

  it("sets the canonical RTL locale on /t/<tableCode>?lang=ar before hydration", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/t/table-42?lang=ar",
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("ar");
  });

  it("leaves the guest locale at en when /b/<slug> has no ?lang=, guest cookie, or Accept-Language", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/b/mara-core-kitchen",
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it.each([
    "/b/mara-core-kitchen",
    "/t/table-42",
  ])("ignores operator payverge_locale on %s (guest uses its own cookie)", async (path) => {
    // Operator dashboard cookie must not leak into guest SSR chrome (PG-21).
    const request = new NextRequest(`https://app.payverge.test${path}`, {
      headers: {
        cookie: "payverge_locale=es-AR",
      },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("uses guest cookie for SSR when ?lang= is absent (PG-21)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/b/mara-core-kitchen",
      { headers: { cookie: "payverge_guest_locale=ja" } },
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("ja");
  });

  it("uses guest cookie on /scan before Accept-Language (#390)", async () => {
    const request = new NextRequest("https://app.payverge.test/scan", {
      headers: {
        cookie: "payverge_guest_locale=es",
        "accept-language": "en-US,en;q=0.9",
      },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("es");
  });

  it("prefers ?lang= over the guest cookie on /scan", async () => {
    const request = new NextRequest("https://app.payverge.test/scan?lang=fr", {
      headers: {
        cookie: "payverge_guest_locale=es",
        "accept-language": "en-US,en;q=0.9",
      },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("fr");
  });

  it("ignores operator payverge_locale on /scan", async () => {
    const request = new NextRequest("https://app.payverge.test/scan", {
      headers: {
        cookie: "payverge_locale=es",
        "accept-language": "en-US,en;q=0.9",
      },
    });
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("uses Accept-Language for guest SSR when no ?lang= or guest cookie (PG-21)", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/t/table-42",
      { headers: { "accept-language": "es-AR,es;q=0.9,en;q=0.8" } },
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("es-AR");
  });

  it("prefers ?lang= over guest cookie on guest routes", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/b/mara-core-kitchen?lang=fr",
      { headers: { cookie: "payverge_guest_locale=ja" } },
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("fr");
  });

  it("ignores an unknown guest ?lang= value and keeps en", async () => {
    const request = new NextRequest(
      "https://app.payverge.test/b/mara-core-kitchen?lang=zz",
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });

  it("does NOT honour ?lang= on a non-guest (operator) route", async () => {
    // ?lang= is a guest-storefront affordance only; operator locale comes from
    // the path prefix, so /dashboard?lang=ar must stay en.
    const request = new NextRequest(
      "https://app.payverge.test/dashboard?lang=ar",
    );
    const response = await middleware(request);

    expect(forwardedLocale(response)).toBe("en");
  });
});

describe("middleware — unknown storefront 404 (#609)", () => {
  const realFetch = global.fetch;
  const originalPublicApi = process.env.API_URL;
  const originalInternalApi = process.env.INTERNAL_API_URL;

  afterEach(() => {
    global.fetch = realFetch;
    if (originalPublicApi === undefined) delete process.env.API_URL;
    else process.env.API_URL = originalPublicApi;
    if (originalInternalApi === undefined) delete process.env.INTERNAL_API_URL;
    else process.env.INTERNAL_API_URL = originalInternalApi;
  });

  it("returns HTTP 404 HTML for an unknown /b/{slug}", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: "Business not found" }),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/b/demo-75-ai-pro"),
    );
    expect(response.status).toBe(404);
    expect(response.headers.get("x-robots-tag")).toMatch(/noindex/i);
    const body = await response.text();
    expect(body).toContain("Business Not Found | Payverge");
  });

  it("does not 404 a resolved storefront", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 1, custom_url: "aurora" }),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/b/aurora"),
    );
    expect(response.status).not.toBe(404);
  });

  it("forwards venue default_language on bare /b/{slug} (#861)", async () => {
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

    const response = await middleware(
      new NextRequest("https://payverge.io/b/parrilla-quebracho-azul"),
    );
    expect(response.status).not.toBe(404);
    expect(response.headers.get("x-middleware-request-x-payverge-locale")).toBe(
      "es",
    );
  });

  it("does not let venue default override /es/b/{slug} or guest cookie (#860/#861)", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        id: 142,
        custom_url: "parrilla-quebracho-azul",
        default_language: "en",
      }),
    }) as unknown as typeof fetch;

    const pathResponse = await middleware(
      new NextRequest("https://payverge.io/es/b/parrilla-quebracho-azul"),
    );
    expect(
      pathResponse.headers.get("x-middleware-request-x-payverge-locale"),
    ).toBe("es");

    const cookieResponse = await middleware(
      new NextRequest("https://payverge.io/b/parrilla-quebracho-azul", {
        headers: { cookie: "payverge_guest_locale=ja" },
      }),
    );
    expect(
      cookieResponse.headers.get("x-middleware-request-x-payverge-locale"),
    ).toBe("ja");
  });

  it("forwards es on /es/b/{slug} so path locale is not ignored (#644)", async () => {
    delete process.env.API_URL;
    delete process.env.INTERNAL_API_URL;
    const request = new NextRequest(
      "https://payverge.io/es/b/payverge-ai-pro-demo-lounge",
    );
    const response = await middleware(request);
    expect(response.status).not.toBe(404);
    expect(response.headers.get("x-middleware-request-x-payverge-locale")).toBe(
      "es",
    );
  });
});

describe("middleware — unknown guest table 404", () => {
  const realFetch = global.fetch;
  const originalPublicApi = process.env.API_URL;

  afterEach(() => {
    global.fetch = realFetch;
    if (originalPublicApi === undefined) delete process.env.API_URL;
    else process.env.API_URL = originalPublicApi;
  });

  it("returns HTTP 404 HTML for an unknown /t/:code", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: "Table not found" }),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/t/zzzz-nonexistent-9999"),
    );
    expect(response.status).toBe(404);
    expect(response.headers.get("x-robots-tag")).toMatch(/noindex/i);
    const body = await response.text();
    expect(body).toContain("Table | Payverge");
    expect(body).toContain("noindex");
  });

  it("returns HTTP 404 HTML for an unknown /t/:code/menu", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: "Table not found" }),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/t/zzzz-nonexistent-9999/menu"),
    );
    expect(response.status).toBe(404);
    expect(await response.text()).toContain("Menu | Payverge");
  });

  it("does not 404 a resolved table", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ business: { name: "Demo Kitchen" } }),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/t/EFDJQQ9J5B"),
    );
    expect(response.status).not.toBe(404);
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("forwards venue default_language on bare /t/{code} (#877)", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        business: { name: "Parrilla Quebracho Azul", default_language: "es" },
      }),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/t/FV214XU12D"),
    );
    expect(response.status).not.toBe(404);
    expect(response.headers.get("x-middleware-request-x-payverge-locale")).toBe(
      "es",
    );
  });

  it("does not 404 when the table API is 5xx", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    const response = await middleware(
      new NextRequest("https://payverge.io/t/EFDJQQ9J5B"),
    );
    expect(response.status).not.toBe(404);
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });
});


describe("uppercase / mixed-case locale prefixes alias to lowercase (#789)", () => {
  it.each([
    ["https://payverge.io/es-AR", "/es-ar"],
    ["https://payverge.io/es-AR/", "/es-ar/"],
    ["https://payverge.io/es-AR/privacy-policy", "/es-ar/privacy-policy"],
    ["https://payverge.io/es-AR/refund", "/es-ar/refund"],
    ["https://payverge.io/es-AR/business/register", "/es-ar/business/register"],
    ["https://payverge.io/ES/privacy-policy", "/es/privacy-policy"],
    ["https://payverge.io/Es-Ar/business/register", "/es-ar/business/register"],
    ["https://payverge.io/ES-AR", "/es-ar"],
  ])("308-redirects %s to the canonical lowercase path", async (input, expectedPath) => {
    const response = await middleware(new NextRequest(input));
    expect(response.status).toBe(308);
    const location = new URL(response.headers.get("location") ?? "", "https://payverge.io");
    expect(location.pathname).toBe(expectedPath);
  });

  it("preserves the query string across the case-canonicalizing redirect", async () => {
    const response = await middleware(
      new NextRequest("https://payverge.io/es-AR/privacy-policy?utm_source=share"),
    );
    expect(response.status).toBe(308);
    const location = new URL(response.headers.get("location") ?? "");
    expect(location.pathname).toBe("/es-ar/privacy-policy");
    expect(location.searchParams.get("utm_source")).toBe("share");
  });

  it("does not redirect non-locale segments that merely start with es", async () => {
    const response = await middleware(
      new NextRequest("https://payverge.io/escape-room"),
    );
    expect(response.status).not.toBe(308);
  });

  it("leaves canonical lowercase prefixes untouched", async () => {
    const response = await middleware(
      new NextRequest("https://payverge.io/es-ar/privacy-policy"),
    );
    expect(response.status).not.toBe(308);
  });
});
