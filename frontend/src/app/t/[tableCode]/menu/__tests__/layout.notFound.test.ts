/** @jest-environment node */
/**
 * Unknown /t/[tableCode]/menu links must be real HTTP 404s via notFound().
 * The menu page is a client component, so the server layout owns the lookup.
 */
const mockGetServerApiUrl = jest.fn(() => "http://api.test/api/v1");
jest.mock("@/lib/serverApiUrl", () => ({
  getServerApiUrl: () => mockGetServerApiUrl(),
}));

import { notFound } from "next/navigation";

jest.mock("next/navigation", () => ({
  notFound: jest.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  mockGetServerApiUrl.mockReset();
  mockGetServerApiUrl.mockReturnValue("http://api.test/api/v1");
  jest.clearAllMocks();
});

describe("MenuLayout — unknown table code 404", () => {
  it("calls notFound() when the backend returns a non-OK lookup", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    const { default: MenuLayout } = await import("../layout");
    await expect(
      MenuLayout({
        children: null,
        params: Promise.resolve({ tableCode: "MISSING" }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFound).toHaveBeenCalledTimes(1);
  });

  it("calls notFound() when the SSR lookup cannot run (empty API URL)", async () => {
    mockGetServerApiUrl.mockReturnValue("");
    global.fetch = jest.fn() as unknown as typeof fetch;

    const { default: MenuLayout } = await import("../layout");
    await expect(
      MenuLayout({
        children: null,
        params: Promise.resolve({ tableCode: "MISSING" }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFound).toHaveBeenCalledTimes(1);
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("calls notFound() when a 200 lookup body is null", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => null,
    }) as unknown as typeof fetch;

    const { default: MenuLayout } = await import("../layout");
    await expect(
      MenuLayout({
        children: null,
        params: Promise.resolve({ tableCode: "MISSING" }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFound).toHaveBeenCalledTimes(1);
  });

  it("does not call notFound() on a transient (5xx) failure", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    const { default: MenuLayout } = await import("../layout");
    const result = await MenuLayout({
      children: null,
      params: Promise.resolve({ tableCode: "BLIP" }),
    });
    expect(notFound).not.toHaveBeenCalled();
    expect(result).toBeTruthy();
  });

  it("does not call notFound() when the table business resolves", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ business: { name: "Mesa Sur" } }),
    }) as unknown as typeof fetch;

    const { default: MenuLayout } = await import("../layout");
    const result = await MenuLayout({
      children: null,
      params: Promise.resolve({ tableCode: "TABLE-42" }),
    });
    expect(notFound).not.toHaveBeenCalled();
    expect(result).toBeTruthy();
  });
});
