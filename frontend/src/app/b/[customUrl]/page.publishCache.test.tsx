/** @jest-environment node */

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));

// Publish-gate integrity: /b/[customUrl] must never ISR-cache a published
// storefront snapshot. After business_page_enabled=false the API 404s within
// seconds; SSR must not keep seeding full HTML/JSON-LD from revalidate:60.

// Keeps this file a module. Without a top-level import/export, TS treats it as a
// global script and `realFetch` collides with the same name in sibling tests.
export {};

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
  () =>
    ({ children }: { children: unknown }) =>
      children,
);

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  jest.clearAllMocks();
  jest.resetModules();
});

function mockBusinessFetch(payload: unknown, status = 200) {
  global.fetch = jest.fn().mockResolvedValue({
    status,
    ok: status >= 200 && status < 300,
    json: async () => payload,
  }) as unknown as typeof fetch;
}

describe("BusinessPage server fetch — publish-gate no-store", () => {
  it("fetches the public business with cache: no-store (not ISR revalidate)", async () => {
    mockBusinessFetch({
      id: 50,
      name: "Core Demo Kitchen",
      custom_url: "demo-admin-8-core-demo-kitchen",
      business_page_enabled: true,
      is_active: true,
    });

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({
      params: Promise.resolve({
        customUrl: "demo-admin-8-core-demo-kitchen",
      }),
    });

    expect(global.fetch).toHaveBeenCalled();
    const [, init] = (global.fetch as jest.Mock).mock.calls[0] as [
      string,
      RequestInit & { next?: { revalidate?: number } },
    ];
    expect(init).toEqual(expect.objectContaining({ cache: "no-store" }));
    expect(init?.next?.revalidate).toBeUndefined();
  });

  it("exports force-dynamic / revalidate=0 so the route is not statically cached", async () => {
    const page = await import("./page");
    expect(page.dynamic).toBe("force-dynamic");
    expect(page.revalidate).toBe(0);
  });

  it("calls notFound() immediately when the backend reports unpublished (404)", async () => {
    const { notFound } = await import("next/navigation");
    mockBusinessFetch({}, 404);

    const { default: BusinessPage } = await import("./page");
    await expect(
      BusinessPage({
        params: Promise.resolve({
          customUrl: "demo-admin-8-core-demo-kitchen",
        }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFound).toHaveBeenCalled();
  });

  it("generateMetadata also uses no-store (share/SERP cannot lag unpublish)", async () => {
    mockBusinessFetch({}, 404);

    const { generateMetadata } = await import("./page");
    await generateMetadata({
      params: Promise.resolve({ customUrl: "demo-admin-8-core-demo-kitchen" }),
    });

    const [, init] = (global.fetch as jest.Mock).mock.calls[0] as [
      string,
      RequestInit & { next?: { revalidate?: number } },
    ];
    expect(init).toEqual(expect.objectContaining({ cache: "no-store" }));
    expect(init?.next?.revalidate).toBeUndefined();
  });
});
