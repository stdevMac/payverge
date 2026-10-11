/** @jest-environment node */
import { NextRequest } from "next/server";
import { middleware } from "@/middleware";
import { resetServerHomeCacheForTests } from "@/lib/instance/serverHome";
import { resetServerInstanceCacheForTests } from "@/lib/instance/serverInstance";

// The instance root ("/") is the venue page, the venue directory, or — with
// nothing published — a redirect to the operator sign-in (/dashboard).

type HomePayload = Record<string, unknown> | null;

function mockBackend(home: HomePayload, venueDefaultLanguage?: string) {
  global.fetch = jest.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith("/home") && home) {
      return { ok: true, status: 200, json: async () => home } as Response;
    }
    if (url.includes("/business/")) {
      return {
        ok: true,
        status: 200,
        json: async () => ({ id: 1, default_language: venueDefaultLanguage }),
      } as Response;
    }
    return { ok: false, status: 503, json: async () => null } as Response;
  }) as unknown as typeof fetch;
}

const VENUE = { id: 1, name: "Trattoria", logo: "", custom_url: "trattoria", city: "" };
const OTHER = { id: 2, name: "Osteria", logo: "", custom_url: "osteria", city: "" };

const realFetch = global.fetch;
const savedApi = process.env.INTERNAL_API_URL;

beforeEach(() => {
  resetServerHomeCacheForTests();
  resetServerInstanceCacheForTests();
  process.env.INTERNAL_API_URL = "http://backend.internal:8080/api/v1";
});

afterEach(() => {
  global.fetch = realFetch;
  if (savedApi === undefined) delete process.env.INTERNAL_API_URL;
  else process.env.INTERNAL_API_URL = savedApi;
});

describe("middleware instance root", () => {
  it("sends visitors to /dashboard when no venue is published, keeping the query", async () => {
    mockBackend({ mode: "empty", primary: null, venues: [] });
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/?invite_code=abc123"),
    );
    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://pos.restaurant.example/dashboard?invite_code=abc123",
    );
  });

  it("sends an invite link to /dashboard even when a venue is served at /", async () => {
    mockBackend({ mode: "venue", primary: VENUE, venues: [VENUE] }, "es");
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/?invite_code=abc123"),
    );
    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://pos.restaurant.example/dashboard?invite_code=abc123",
    );
  });

  it("serves the primary venue at / in the venue's default language", async () => {
    mockBackend({ mode: "venue", primary: VENUE, venues: [VENUE] }, "es");
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/"),
    );
    expect(response.headers.get("location")).toBeNull();
    expect(response.headers.get("x-middleware-request-x-payverge-locale")).toBe("es");
    expect(response.headers.get("Content-Security-Policy")).toBeTruthy();
  });

  it("lets an explicit ?lang= win over the venue default", async () => {
    mockBackend({ mode: "venue", primary: VENUE, venues: [VENUE] }, "es");
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/?lang=fr"),
    );
    expect(response.headers.get("x-middleware-request-x-payverge-locale")).toBe("fr");
  });

  it("renders the directory in place for several venues", async () => {
    mockBackend({ mode: "directory", primary: null, venues: [VENUE, OTHER] });
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/"),
    );
    expect(response.headers.get("location")).toBeNull();
  });

  it("rewrites /es to the instance home with the Spanish locale", async () => {
    mockBackend({ mode: "directory", primary: null, venues: [VENUE, OTHER] });
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/es"),
    );
    // The locale rides in the rewritten URL as well as the request header:
    // when Next proxies the rewrite (origin mismatch) the header is dropped
    // and only ?lang= reaches the page and the root layout's second pass.
    expect(response.headers.get("x-middleware-rewrite")).toBe(
      "https://pos.restaurant.example/?lang=es",
    );
    expect(response.headers.get("x-middleware-request-x-payverge-locale")).toBe("es");
  });

  it("rewrites /es-ar with lang=es-ar and keeps an explicit ?lang=", async () => {
    mockBackend({ mode: "directory", primary: null, venues: [VENUE, OTHER] });
    const esAr = await middleware(
      new NextRequest("https://pos.restaurant.example/es-ar"),
    );
    expect(esAr.headers.get("x-middleware-rewrite")).toBe(
      "https://pos.restaurant.example/?lang=es-ar",
    );
    const explicit = await middleware(
      new NextRequest("https://pos.restaurant.example/es?lang=fr"),
    );
    expect(explicit.headers.get("x-middleware-rewrite")).toBe(
      "https://pos.restaurant.example/?lang=fr",
    );
  });

  it("does not redirect when the backend never answered (the page decides)", async () => {
    mockBackend(null);
    const response = await middleware(
      new NextRequest("https://pos.restaurant.example/"),
    );
    expect(response.headers.get("location")).toBeNull();
  });
});
