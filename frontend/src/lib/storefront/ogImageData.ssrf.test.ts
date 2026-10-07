/** @jest-environment node */

import { fetchImageDataUrl, resolveImageFetchUrl } from "./ogImageData";

const realFetch = global.fetch;
const realMediaOrigins = process.env.MEDIA_ORIGINS;

afterEach(() => {
  global.fetch = realFetch;
  if (realMediaOrigins === undefined) delete process.env.MEDIA_ORIGINS;
  else process.env.MEDIA_ORIGINS = realMediaOrigins;
});

describe("fetchImageDataUrl SSRF guards", () => {
  it("never fetches a non-allowlisted host", async () => {
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const fetchMock = jest.fn();
    global.fetch = fetchMock as unknown as typeof fetch;

    const metadata = "http://169.254.169.254/latest/meta-data";
    const evil = "https://evil.example/x.png";

    expect(resolveImageFetchUrl(metadata)).toBeNull();
    expect(resolveImageFetchUrl(evil)).toBeNull();
    await expect(fetchImageDataUrl(metadata)).resolves.toBeNull();
    await expect(fetchImageDataUrl(evil)).resolves.toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("does not follow redirects", async () => {
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const fetchMock = jest.fn().mockResolvedValue({
      ok: false,
      status: 302,
      headers: {
        get: (name: string) =>
          name === "location" ? "http://169.254.169.254/latest/meta-data" : null,
      },
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    await expect(
      fetchImageDataUrl("https://images.example.com/logo.png"),
    ).resolves.toBeNull();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://images.example.com/logo.png",
      expect.objectContaining({
        cache: "no-store",
        redirect: "manual",
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("rejects a content-length over 5MB before reading the body", async () => {
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const getReader = jest.fn();
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: {
        get: (name: string) => {
          if (name === "content-length") return "5000001";
          if (name === "content-type") return "image/png";
          return null;
        },
      },
      body: { getReader },
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    await expect(
      fetchImageDataUrl("https://images.example.com/huge.png"),
    ).resolves.toBeNull();
    expect(getReader).not.toHaveBeenCalled();
  });

  it("cancels a streamed body over 5MB and aborts the fetch", async () => {
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const chunks = [
      new Uint8Array(2_000_000),
      new Uint8Array(2_000_000),
      new Uint8Array(2_000_000),
    ];
    let index = 0;
    const cancel = jest.fn().mockResolvedValue(undefined);
    const read = jest.fn(async () => {
      if (index >= chunks.length) return { done: true, value: undefined };
      const value = chunks[index];
      index += 1;
      return { done: false, value };
    });
    let signal: AbortSignal | undefined;
    const fetchMock = jest.fn(async (_url: string, init?: RequestInit) => {
      signal = init?.signal ?? undefined;
      return {
        ok: true,
        status: 200,
        headers: {
          get: (name: string) => (name === "content-type" ? "image/png" : null),
        },
        body: { getReader: () => ({ read, cancel }) },
      };
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    await expect(
      fetchImageDataUrl("https://images.example.com/huge.png"),
    ).resolves.toBeNull();

    expect(read).toHaveBeenCalledTimes(3);
    expect(cancel).toHaveBeenCalled();
    expect(signal?.aborted).toBe(true);
  });

  it("fetches an allowlisted media origin", async () => {
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const bytes = new Uint8Array([1, 2, 3, 4]);
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: {
        get: (name: string) => (name === "content-type" ? "image/png" : null),
      },
      arrayBuffer: async () => bytes.buffer,
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const out = await fetchImageDataUrl("https://images.example.com/logo.png");

    expect(fetchMock).toHaveBeenCalledWith(
      "https://images.example.com/logo.png",
      expect.objectContaining({
        cache: "no-store",
        redirect: "manual",
        signal: expect.any(AbortSignal),
      }),
    );
    expect(out).toMatch(/^data:image\/png;base64,/);
  });
});
