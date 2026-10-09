/** @jest-environment node */
import { fetchWithTransientRetry } from "./fetchWithTransientRetry";

const realFetch = global.fetch;
afterEach(() => {
  global.fetch = realFetch;
});

describe("fetchWithTransientRetry", () => {
  it("retries 503 then returns the successful response", async () => {
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 503 })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({}) });

    const res = await fetchWithTransientRetry("http://api.test/business/lounge");
    expect(res.ok).toBe(true);
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("retries a one-shot 404 then returns the live table", async () => {
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 404 })
      .mockResolvedValueOnce({ ok: true, status: 200 });
    const res = await fetchWithTransientRetry("http://api.test/guest/table/M03Y18GB3P");
    expect(res.ok).toBe(true);
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("returns a confirmed 404 after one retry", async () => {
    global.fetch = jest.fn().mockResolvedValue({ ok: false, status: 404 });
    const res = await fetchWithTransientRetry("http://api.test/business/missing");
    expect(res.status).toBe(404);
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });
});
