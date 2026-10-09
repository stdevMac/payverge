/** @jest-environment node */
/**
 * Unknown /t/[tableCode] links must be real HTTP 404s via notFound(),
 * not a 200 shell that inherits the marketing homepage title.
 */
jest.mock("next/dynamic", () => () => () => null);
const mockGetServerApiUrl = jest.fn(() => "http://api.test/api/v1");
jest.mock("@/lib/serverApiUrl", () => ({
  getServerApiUrl: () => mockGetServerApiUrl(),
}));
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

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  mockGetServerApiUrl.mockReset();
  mockGetServerApiUrl.mockReturnValue("http://api.test/api/v1");
  jest.clearAllMocks();
});

describe("TablePage — unknown table code 404", () => {
  it("calls notFound() when the backend returns a non-OK lookup", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    const { default: TablePage } = await import("../page");
    await expect(
      TablePage({
        params: Promise.resolve({ tableCode: "MISSING" }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    expect(notFound).toHaveBeenCalledTimes(1);
  });

  it("calls notFound() when the SSR lookup returns null (empty API URL)", async () => {
    mockGetServerApiUrl.mockReturnValue("");
    global.fetch = jest.fn() as unknown as typeof fetch;

    const { default: TablePage } = await import("../page");
    await expect(
      TablePage({
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

    const { default: TablePage } = await import("../page");
    await expect(
      TablePage({
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

    const { default: TablePage } = await import("../page");
    const result = await TablePage({
      params: Promise.resolve({ tableCode: "BLIP" }),
    });
    expect(notFound).not.toHaveBeenCalled();
    expect(result).toBeTruthy();
  });

  it("does not call notFound() when the table resolves", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ business: { name: "Mesa Sur" } }),
    }) as unknown as typeof fetch;

    const { default: TablePage } = await import("../page");
    const result = await TablePage({
      params: Promise.resolve({ tableCode: "TABLE-42" }),
    });
    expect(notFound).not.toHaveBeenCalled();
    expect(result).toBeTruthy();
  });
});
