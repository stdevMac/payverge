/** @jest-environment node */

let mockLocaleHeader: string | null = null;

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({
    get: (name: string) =>
      name === "x-payverge-locale" ? mockLocaleHeader : null,
  }),
}));

// I18N-D1: the page BODY fetch must honor a validated ?lang= the same way
// generateMetadata already does, so non-default-language guests don't get a
// source-language flash of the hero/about prose on first paint.
import { notFound, permanentRedirect } from "next/navigation";

jest.mock("next/navigation", () => ({
  notFound: jest.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
  permanentRedirect: jest.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

// Stub the heavy client + shell imports so importing page.tsx is cheap.
jest.mock("./BusinessPageClient", () => () => null);
jest.mock(
  "@/components/customer/CustomerAuthShell",
  () => ({ children }: any) => children,
);

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  mockLocaleHeader = null;
  jest.clearAllMocks();
});

function mockBusinessFetch(payload: unknown, status = 200) {
  global.fetch = jest.fn().mockResolvedValue({
    status,
    ok: status >= 200 && status < 300,
    json: async () => payload,
  }) as unknown as typeof fetch;
}

describe("BusinessPage server component — ?lang= body fetch (I18N-D1)", () => {
  it("threads a validated ?lang= into the body fetch", async () => {
    mockBusinessFetch({ id: 1, name: "Café Aurora", custom_url: "aurora" });

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "ar" }),
    });

    const calledUrl = (global.fetch as jest.Mock).mock.calls[0][0] as string;
    expect(calledUrl).toContain("/business/aurora?language=ar");
  });

  it("normalizes case-insensitively (es-ar → es-AR), matching generateMetadata", async () => {
    mockBusinessFetch({ id: 1, name: "Café Aurora", custom_url: "aurora" });

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "es-ar" }),
    });

    const calledUrl = (global.fetch as jest.Mock).mock.calls[0][0] as string;
    expect(calledUrl).toContain("language=es-AR");
  });

  it("ignores garbage lang values (no language param leaks)", async () => {
    mockBusinessFetch({ id: 1, name: "Café Aurora", custom_url: "aurora" });

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({
      params: Promise.resolve({ customUrl: "aurora" }),
      searchParams: Promise.resolve({ lang: "zz-not-a-locale" }),
    });

    const calledUrl = (global.fetch as jest.Mock).mock.calls[0][0] as string;
    expect(calledUrl).not.toContain("language=");
  });

  it("threads path-locale /es/b/{slug} via x-payverge-locale into the body fetch (#860)", async () => {
    mockLocaleHeader = "es";
    mockBusinessFetch({
      id: 142,
      name: "Parrilla Quebracho Azul",
      custom_url: "parrilla-quebracho-azul",
      description: "Noche de asado para dos",
      default_language: "es",
    });

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({
      params: Promise.resolve({ customUrl: "parrilla-quebracho-azul" }),
    });

    const calledUrls = (global.fetch as jest.Mock).mock.calls.map(
      (call) => call[0] as string,
    );
    expect(calledUrls.some((url) => url.includes("language=es"))).toBe(true);
  });

  it("still works with no searchParams at all (existing callers unaffected)", async () => {
    mockBusinessFetch({ id: 1, name: "Café Aurora", custom_url: "aurora" });

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({ params: Promise.resolve({ customUrl: "aurora" }) });

    const calledUrl = (global.fetch as jest.Mock).mock.calls[0][0] as string;
    expect(calledUrl).not.toContain("language=");
    expect(notFound).not.toHaveBeenCalled();
    expect(permanentRedirect).not.toHaveBeenCalled();
  });
});

describe("generateMetadata — localized not-found fallback (I18N-D1b)", () => {
  it("serves the guest-tier not-found title/description for a validated lang", async () => {
    mockBusinessFetch({}, 404);
    const { generateMetadata } = await import("./page");
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "missing" }),
      searchParams: Promise.resolve({ lang: "de" }),
    });
    // de.json → businessPage.errors.notFound = "Unternehmen nicht gefunden"
    expect(meta.title).toBe("Unternehmen nicht gefunden | Payverge");
    expect(meta.title).not.toContain("Business Not Found");
  });

  it("falls back to English when no lang is present", async () => {
    mockBusinessFetch({}, 404);
    const { generateMetadata } = await import("./page");
    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: "missing" }),
    });
    expect(meta.title).toBe("Business Not Found | Payverge");
  });
});
