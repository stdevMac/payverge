/** @jest-environment node */

import { lookupGuestTable, matchGuestTableRoute } from "./lookupGuestTable";

const realFetch = global.fetch;
const originalInternal = process.env.INTERNAL_API_URL;
const originalPublic = process.env.API_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (originalInternal === undefined) delete process.env.INTERNAL_API_URL;
  else process.env.INTERNAL_API_URL = originalInternal;
  if (originalPublic === undefined) delete process.env.API_URL;
  else process.env.API_URL = originalPublic;
});

describe("lookupGuestTable", () => {
  it("treats an empty API URL as not_found without fetching", async () => {
    delete process.env.INTERNAL_API_URL;
    delete process.env.API_URL;
    global.fetch = jest.fn() as unknown as typeof fetch;

    await expect(lookupGuestTable("MISSING")).resolves.toEqual({
      kind: "not_found",
    });
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("treats HTTP 404 as not_found", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: "Table not found" }),
    }) as unknown as typeof fetch;

    await expect(lookupGuestTable("zzzz-missing")).resolves.toEqual({
      kind: "not_found",
    });
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("treats a null 200 body as not_found", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => null,
    }) as unknown as typeof fetch;

    await expect(lookupGuestTable("MISSING")).resolves.toEqual({
      kind: "not_found",
    });
  });

  it("treats 5xx as unavailable", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    await expect(lookupGuestTable("BLIP")).resolves.toEqual({
      kind: "unavailable",
    });
  });

  it("retries a one-shot 404 then resolves the live table", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: async () => ({ error: "Table not found" }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ business: { name: "AI Pro Lounge" } }),
      }) as unknown as typeof fetch;

    await expect(lookupGuestTable("M03Y18GB3P")).resolves.toEqual({
      kind: "found",
      businessName: "AI Pro Lounge",
    });
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("retries a 503 then resolves the live table", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 503,
        json: async () => ({}),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ business: { name: "AI Pro Lounge" } }),
      }) as unknown as typeof fetch;

    await expect(lookupGuestTable("M03Y18GB3P")).resolves.toEqual({
      kind: "found",
      businessName: "AI Pro Lounge",
    });
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("treats a network failure as unavailable", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockRejectedValue(new Error("Failed to fetch"));

    await expect(lookupGuestTable("BLIP")).resolves.toEqual({
      kind: "unavailable",
    });
  });

  it("returns the business name when the table resolves", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ business: { name: "Mesa Sur" } }),
    }) as unknown as typeof fetch;

    await expect(lookupGuestTable("EFDJQQ9J5B")).resolves.toEqual({
      kind: "found",
      businessName: "Mesa Sur",
    });
  });

  it("exposes venue default_language so bare /t/{code} can honor it (#877)", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        business: { name: "Parrilla Quebracho Azul", default_language: "es" },
      }),
    }) as unknown as typeof fetch;

    await expect(lookupGuestTable("FV214XU12D")).resolves.toEqual({
      kind: "found",
      businessName: "Parrilla Quebracho Azul",
      defaultLanguage: "es",
    });
  });
});

describe("matchGuestTableRoute", () => {
  it("matches table and menu surfaces", () => {
    expect(matchGuestTableRoute("/t/EFDJQQ9J5B")).toEqual({
      tableCode: "EFDJQQ9J5B",
      surface: "table",
    });
    expect(matchGuestTableRoute("/t/EFDJQQ9J5B/menu")).toEqual({
      tableCode: "EFDJQQ9J5B",
      surface: "menu",
    });
    expect(matchGuestTableRoute("/t/EFDJQQ9J5B/bill/")).toEqual({
      tableCode: "EFDJQQ9J5B",
      surface: "table",
    });
    expect(matchGuestTableRoute("/t/EFDJQQ9J5B/profile")).toEqual({
      tableCode: "EFDJQQ9J5B",
      surface: "table",
    });
    expect(matchGuestTableRoute("/t/EFDJQQ9J5B/signin")).toEqual({
      tableCode: "EFDJQQ9J5B",
      surface: "table",
    });
  });

  it("ignores unrelated paths", () => {
    expect(matchGuestTableRoute("/tools")).toBeNull();
    expect(matchGuestTableRoute("/b/demo")).toBeNull();
    expect(matchGuestTableRoute("/t")).toBeNull();
  });
});
