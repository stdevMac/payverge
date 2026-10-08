/** @jest-environment node */

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));

import { notFound } from "next/navigation";

jest.mock("next/navigation", () => ({
  notFound: jest.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

// Stub the heavy client + shell imports so importing page.tsx is cheap.
jest.mock("./BusinessPageClient", () => () => null);
jest.mock("@/components/customer/CustomerAuthShell", () => ({ children }: any) => children);

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  jest.clearAllMocks();
});

describe("BusinessPage server component — 404 (PARITY-4)", () => {
  it("calls notFound() when the backend returns 404 for the slug", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      status: 404,
      ok: false,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    const { default: BusinessPage } = await import("./page");
    await expect(
      BusinessPage({ params: Promise.resolve({ customUrl: "missing" }) }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFound).toHaveBeenCalledTimes(1);
  });

  it("does NOT call notFound() on a transient (5xx) failure", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      status: 503,
      ok: false,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    const { default: BusinessPage } = await import("./page");
    await BusinessPage({ params: Promise.resolve({ customUrl: "blip" }) });
    expect(notFound).not.toHaveBeenCalled();
  });
});
