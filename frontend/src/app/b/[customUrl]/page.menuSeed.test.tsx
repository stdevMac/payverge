/** @jest-environment node */

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));

// SEO-0.1: the server component fetches the guest menu alongside the business,
// seeds the React Query cache (exact useBusinessPageData key/shape), and hands
// the normalized categories to BusinessPageClient for the crawlable menu block.

jest.mock("next/navigation", () => ({
  notFound: jest.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
  permanentRedirect: jest.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

jest.mock("./BusinessPageClient", () => () => null);
jest.mock(
  "@/components/customer/CustomerAuthShell",
  () => ({ children }: any) => children,
);

// Keeps this file a module. Without a top-level import/export, TS treats it as
// a global script and `realFetch` collides with the same name in sibling tests.
export {};

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  jest.clearAllMocks();
  jest.resetModules();
});

type MockResponse = { status?: number; body?: unknown } | "reject";

// URL-aware router: the menu endpoint URL contains the business URL as a
// prefix, so order matters — most specific first.
function mockFetchRouter(routes: Array<[string, MockResponse]>) {
  global.fetch = jest.fn(async (url: unknown) => {
    const u = String(url);
    for (const [needle, response] of routes) {
      if (!u.includes(needle)) continue;
      if (response === "reject") throw new Error("boom");
      const status = response.status ?? 200;
      return {
        status,
        ok: status >= 200 && status < 300,
        json: async () => response.body,
      };
    }
    throw new Error(`unexpected fetch: ${u}`);
  }) as unknown as typeof fetch;
}

const businessPayload = {
  id: 7,
  name: "Café Aurora",
  custom_url: "aurora",
  business_page_enabled: true,
  is_active: true,
};

const menuPayload = {
  parsed_categories: [
    {
      id: "cat-1",
      name: "Pizzas",
      description: "Wood fired",
      items: [
        {
          id: "item-1",
          name: "Margherita",
          description: "San Marzano tomatoes",
          price: 12.5,
          is_available: true,
        },
      ],
    },
  ],
  offers: [],
  bundles: [],
  item_orderability: {
    "item-1": { state: "available", orderable: true },
  },
};

function shellChildren(el: any): any {
  const children = el.props.children;
  const shell = Array.isArray(children)
    ? children.find((c: any) => c && c.props && c.props.children)
    : children;
  return shell.props.children;
}

describe("BusinessPage — SSR menu seed (SEO-0.1)", () => {
  it("fetches the menu in parallel and seeds categories + the exact RQ cache key", async () => {
    mockFetchRouter([
      ["/google/details", { body: { place_details: null } }],
      ["/menu", { body: menuPayload }],
      ["/business/aurora", { body: businessPayload }],
    ]);

    const { default: BusinessPage } = await import("./page");
    const el = await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
    });

    const urls = (global.fetch as jest.Mock).mock.calls.map((c) => String(c[0]));
    // Business fetch stays the first issued request (existing tests rely on it).
    expect(urls[0]).toContain("/business/aurora");
    expect(urls.some((u) => u.includes("/business/aurora/menu"))).toBe(true);

    const inner = shellChildren(el);
    // Seeded → wrapped in HydrationBoundary carrying the dehydrated query.
    const queries = inner.props.state.queries;
    expect(queries).toHaveLength(1);
    expect(queries[0].queryKey).toEqual([
      "business",
      "7",
      "menu",
      "aurora",
      "en",
    ]);
    // Dehydrated data matches the useBusinessPageData queryFn shape exactly.
    expect(queries[0].state.data.categories[0].name).toBe("Pizzas");
    expect(queries[0].state.data.itemOrderability["item-1"]).toEqual({
      state: "available",
      orderable: true,
    });

    // The client component receives the same categories for the sr-only block.
    const clientEl = inner.props.children;
    expect(clientEl.props.initialMenuCategories).toHaveLength(1);
    expect(clientEl.props.initialMenuCategories[0].items[0].name).toBe(
      "Margherita",
    );
  });

  it("threads a validated ?lang= into the menu fetch (same language as the body fetch)", async () => {
    mockFetchRouter([
      ["/google/details", { body: { place_details: null } }],
      ["/menu", { body: menuPayload }],
      ["/business/aurora", { body: businessPayload }],
    ]);

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "ar" }),
    });

    const urls = (global.fetch as jest.Mock).mock.calls.map((c) => String(c[0]));
    expect(urls[0]).toContain("/business/aurora?language=ar");
    expect(
      urls.some((u) => u.includes("/business/aurora/menu?language=ar")),
    ).toBe(true);
  });

  it("never fails the page when the menu fetch fails — renders without a seed", async () => {
    mockFetchRouter([
      ["/google/details", { body: { place_details: null } }],
      ["/menu", "reject"],
      ["/business/aurora", { body: businessPayload }],
    ]);

    const { default: BusinessPage } = await import("./page");
    const el = await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
    });

    const inner = shellChildren(el);
    // No HydrationBoundary wrapper; the client element renders directly with
    // a null menu seed (the client query refetches on mount as before).
    expect(inner.props.initialMenuCategories).toBeNull();
    expect(inner.props.initialBusiness?.name).toBe("Café Aurora");
  });

  it("tolerates a malformed categories string without failing the page", async () => {
    mockFetchRouter([
      ["/google/details", { body: { place_details: null } }],
      ["/menu", { body: { categories: "{broken json" } }],
      ["/business/aurora", { body: businessPayload }],
    ]);

    const { default: BusinessPage } = await import("./page");
    const el = await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
    });
    expect(shellChildren(el).props.initialMenuCategories).toBeNull();
  });

  it("fetches Google details only when reviews are enabled, and emits aggregateRating", async () => {
    mockFetchRouter([
      [
        "/google/details",
        { body: { place_details: { rating: 4.7, user_ratings_total: 213 } } },
      ],
      ["/menu", { body: menuPayload }],
      [
        "/business/aurora",
        {
          body: {
            ...businessPayload,
            google_reviews_enabled: true,
            google_place_id: "ChIJx",
          },
        },
      ],
    ]);

    const { default: BusinessPage } = await import("./page");
    const el = await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
    });

    const urls = (global.fetch as jest.Mock).mock.calls.map((c) => String(c[0]));
    expect(urls.some((u) => u.includes("/business/aurora/google/details"))).toBe(
      true,
    );

    const scripts = el.props.children.filter(
      (c: any) => c && c.type === "script",
    );
    const restaurant = JSON.parse(
      scripts[0].props.dangerouslySetInnerHTML.__html,
    );
    expect(restaurant["@type"]).toBe("Restaurant");
    expect(restaurant.aggregateRating).toEqual({
      "@type": "AggregateRating",
      ratingValue: 4.7,
      reviewCount: 213,
    });
    const { getSiteUrl } = await import("@/config/publicConfig");
    const baseUrl = getSiteUrl();
    expect(restaurant.menu).toBe(`${baseUrl}/b/aurora?tab=menu`);

    const breadcrumb = JSON.parse(
      scripts[1].props.dangerouslySetInnerHTML.__html,
    );
    expect(breadcrumb["@type"]).toBe("BreadcrumbList");
    expect(breadcrumb.itemListElement.map((i: any) => i.name)).toEqual([
      "Home",
      "Café Aurora",
    ]);
  });

  it("skips the Google details fetch when reviews are not advertised", async () => {
    mockFetchRouter([
      ["/google/details", { body: { place_details: null } }],
      ["/menu", { body: menuPayload }],
      ["/business/aurora", { body: businessPayload }],
    ]);

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({ params: Promise.resolve({ customUrl: "aurora" }) });

    const urls = (global.fetch as jest.Mock).mock.calls.map((c) => String(c[0]));
    expect(urls.some((u) => u.includes("/google/details"))).toBe(false);
  });
});
