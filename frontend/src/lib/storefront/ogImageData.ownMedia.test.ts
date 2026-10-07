/** @jest-environment node */

import { fetchImageDataUrl, resolveImageFetchUrl } from "./ogImageData";

const realFetch = global.fetch;
const realInternalApiUrl = process.env.INTERNAL_API_URL;
const realMediaOrigins = process.env.MEDIA_ORIGINS;
const realPublicUrl = process.env.PUBLIC_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (realInternalApiUrl === undefined) delete process.env.INTERNAL_API_URL;
  else process.env.INTERNAL_API_URL = realInternalApiUrl;
  if (realMediaOrigins === undefined) delete process.env.MEDIA_ORIGINS;
  else process.env.MEDIA_ORIGINS = realMediaOrigins;
  if (realPublicUrl === undefined) delete process.env.PUBLIC_URL;
  else process.env.PUBLIC_URL = realPublicUrl;
});

describe("resolveImageFetchUrl", () => {
  it("resolves relative /media uploads against the backend origin", () => {
    expect(
      resolveImageFetchUrl(
        "/media/businesses/1/logo.png",
        "http://backend:8080/api/v1",
      ),
    ).toBe("http://backend:8080/media/businesses/1/logo.png");
    expect(
      resolveImageFetchUrl(
        "/media/businesses/1/banner.webp",
        "https://pos.example.com/api/v1",
      ),
    ).toBe("https://pos.example.com/media/businesses/1/banner.webp");
  });

  it("returns an allowlisted https origin and rejects anything else", () => {
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const cdn = "https://images.example.com/businesses/1/logo.png";
    expect(resolveImageFetchUrl(cdn, "http://backend:8080/api/v1")).toBe(cdn);
    expect(resolveImageFetchUrl(cdn, "")).toBe(cdn);
    expect(
      resolveImageFetchUrl(
        "https://cdn.example.com/businesses/1/logo.png",
        "http://backend:8080/api/v1",
      ),
    ).toBeNull();
    expect(
      resolveImageFetchUrl("http://images.example.com/logo.png", ""),
    ).toBeNull();
    expect(
      resolveImageFetchUrl(
        "https://user:pass@images.example.com/logo.png",
        "",
      ),
    ).toBeNull();
    expect(resolveImageFetchUrl("file:///etc/passwd", "")).toBeNull();
    expect(
      resolveImageFetchUrl("https://images.unsplash.com/photo.jpg", ""),
    ).toBe("https://images.unsplash.com/photo.jpg");
  });

  it("allows absolute own-media on the site origin only", () => {
    process.env.PUBLIC_URL = "https://pos.example.com";
    process.env.MEDIA_ORIGINS = "https://images.example.com";
    const logo = "https://pos.example.com/media/businesses/1/logo.png";
    expect(resolveImageFetchUrl(logo, "http://backend:8080/api/v1")).toBe(logo);
    expect(
      resolveImageFetchUrl(
        "https://pos.example.com/api/v1/admin",
        "http://backend:8080/api/v1",
      ),
    ).toBeNull();
  });

  it("gives null without an absolute http(s) API base", () => {
    expect(resolveImageFetchUrl("/media/businesses/1/logo.png", "")).toBeNull();
    expect(
      resolveImageFetchUrl("/media/businesses/1/logo.png", "/api/v1"),
    ).toBeNull();
    expect(
      resolveImageFetchUrl("/media/businesses/1/logo.png", "file:///api/v1"),
    ).toBeNull();
  });

  it("never rewrites paths outside the strict /media/<key> form", () => {
    // Not own media and not an allowlisted absolute URL: null, so fetch is
    // never asked to reach another backend path.
    expect(
      resolveImageFetchUrl(
        "/media/../api/v1/admin",
        "http://backend:8080/api/v1",
      ),
    ).toBeNull();
    expect(
      resolveImageFetchUrl("/api/v1/x.png", "http://backend:8080/api/v1"),
    ).toBeNull();
  });
});

describe("fetchImageDataUrl with a relative /media logo", () => {
  it("fetches it from the server-side API origin", async () => {
    process.env.INTERNAL_API_URL = "http://backend:8080/api/v1";
    const bytes = new Uint8Array([137, 80, 78, 71]);
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      headers: { get: () => "image/png" },
      arrayBuffer: async () => bytes.buffer,
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const out = await fetchImageDataUrl("/media/businesses/1/logo.png");

    expect(fetchMock).toHaveBeenCalledWith(
      "http://backend:8080/media/businesses/1/logo.png",
      expect.objectContaining({
        cache: "no-store",
        redirect: "manual",
        signal: expect.any(AbortSignal),
      }),
    );
    expect(out).toMatch(/^data:image\/png;base64,/);
  });
});
