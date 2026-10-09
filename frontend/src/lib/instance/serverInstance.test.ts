/** @jest-environment node */
import {
  getServerInstanceInfo,
  INSTANCE_FAILURE_TTL_MS,
  resetServerInstanceCacheForTests,
} from "./serverInstance";

jest.mock("@/lib/serverApiUrl", () => ({
  getServerApiUrl: () => "http://backend.test/api/v1",
}));

describe("getServerInstanceInfo negative cache (F3)", () => {
  afterEach(() => resetServerInstanceCacheForTests());

  it("does not refetch while the backend is down inside the failure window", async () => {
    const fetchMock = jest.fn().mockRejectedValue(new Error("down"));
    global.fetch = fetchMock as never;
    const t0 = Date.now();
    await expect(getServerInstanceInfo(t0)).resolves.toBeNull();
    await expect(getServerInstanceInfo(t0 + 100)).resolves.toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await getServerInstanceInfo(t0 + INSTANCE_FAILURE_TTL_MS + 1000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
