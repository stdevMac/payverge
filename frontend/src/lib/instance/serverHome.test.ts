/** @jest-environment node */
import {
  getServerHome,
  HOME_FAILURE_TTL_MS,
  HOME_TTL_MS,
  isPrimaryVenueSlug,
  parseHomeInfo,
  resetServerHomeCacheForTests,
} from "./serverHome";

jest.mock("@/lib/serverApiUrl", () => ({
  getServerApiUrl: () => "http://backend.test/api/v1",
}));

const realFetch = global.fetch;
const alpha = { id: 7, name: "Alpha", logo: "", custom_url: "alpha", city: "Rosario" };

afterEach(() => {
  global.fetch = realFetch;
  resetServerHomeCacheForTests();
});

function okFetch(body: unknown) {
  return jest.fn().mockResolvedValue({ ok: true, json: async () => body });
}

describe("parseHomeInfo", () => {
  it("accepts the three modes", () => {
    expect(parseHomeInfo({ mode: "venue", primary: alpha, venues: null })).toEqual({
      mode: "venue",
      primary: alpha,
      venues: [],
    });
    expect(parseHomeInfo({ mode: "directory", venues: [alpha] })?.mode).toBe(
      "directory",
    );
    expect(parseHomeInfo({ mode: "empty" })).toEqual({
      mode: "empty",
      primary: null,
      venues: [],
    });
  });

  it("rejects unusable payloads", () => {
    expect(parseHomeInfo(null)).toBeNull();
    expect(parseHomeInfo({ storefronts: [] })).toBeNull();
    expect(parseHomeInfo({ mode: "venue", primary: { id: 1 } })).toBeNull();
    expect(parseHomeInfo({ mode: "directory", venues: [] })).toBeNull();
  });

  it("drops directory rows without a slug", () => {
    const info = parseHomeInfo({
      mode: "directory",
      venues: [alpha, { id: 2, name: "No slug", custom_url: "" }],
    });
    expect(info?.venues).toEqual([alpha]);
  });
});

describe("getServerHome", () => {
  it("fetches /home and caches a good answer for the TTL", async () => {
    const fetchMock = okFetch({ mode: "venue", primary: alpha, venues: [] });
    global.fetch = fetchMock as never;
    const t0 = Date.now();
    await expect(getServerHome(t0)).resolves.toMatchObject({ mode: "venue" });
    await getServerHome(t0 + 100);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("http://backend.test/api/v1/home");
    await getServerHome(t0 + HOME_TTL_MS + 1000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("keeps the last good answer when a refresh fails", async () => {
    global.fetch = okFetch({ mode: "venue", primary: alpha, venues: [] }) as never;
    const t0 = Date.now();
    await getServerHome(t0);
    global.fetch = jest.fn().mockRejectedValue(new Error("down")) as never;
    await expect(getServerHome(t0 + HOME_TTL_MS + 1000)).resolves.toMatchObject({
      mode: "venue",
    });
  });

  it("returns null and negative-caches when the backend never answered", async () => {
    const fetchMock = jest.fn().mockRejectedValue(new Error("down"));
    global.fetch = fetchMock as never;
    const t0 = Date.now();
    await expect(getServerHome(t0)).resolves.toBeNull();
    await expect(getServerHome(t0 + 100)).resolves.toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await getServerHome(t0 + HOME_FAILURE_TTL_MS + 1000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe("isPrimaryVenueSlug", () => {
  it("matches the root venue case-insensitively and nothing else", () => {
    const home = { mode: "venue" as const, primary: alpha, venues: [] };
    expect(isPrimaryVenueSlug(home, "ALPHA")).toBe(true);
    expect(isPrimaryVenueSlug(home, "beta")).toBe(false);
    expect(
      isPrimaryVenueSlug({ mode: "directory", primary: null, venues: [alpha] }, "alpha"),
    ).toBe(false);
    expect(isPrimaryVenueSlug(null, "alpha")).toBe(false);
  });
});
