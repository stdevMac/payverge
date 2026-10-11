import {
  computeAllowedImageHosts,
  isAllowedImageUrl,
  isTrustedEntityImageUrl,
} from "../aiImageOrigins";

describe("aiImageOrigins", () => {
  const menu = [
    {
      items: [
        { name: "Burger", image: "https://cdn.payverge.io/biz/1/burger.jpg" },
        {
          name: "Fries",
          images: ["https://media-bucket.s3.amazonaws.com/x.png", "not-a-url"],
        },
      ],
    },
    { items: [{ name: "Soda" }] },
  ];

  it("derives the set of hosts from menu image fields", () => {
    const hosts = computeAllowedImageHosts(menu);
    expect(hosts.has("cdn.payverge.io")).toBe(true);
    expect(hosts.has("media-bucket.s3.amazonaws.com")).toBe(true);
  });

  it("ignores non-URL image values without throwing", () => {
    expect(() => computeAllowedImageHosts(menu)).not.toThrow();
  });

  it("merges NEXT_PUBLIC_MEDIA_ORIGINS fallback hosts", () => {
    const hosts = computeAllowedImageHosts(
      [],
      "https://assets.payverge.io, https://img.example.com",
    );
    expect(hosts.has("assets.payverge.io")).toBe(true);
    expect(hosts.has("img.example.com")).toBe(true);
  });

  it("allows a URL whose host is in the allowlist", () => {
    const hosts = computeAllowedImageHosts(menu);
    expect(
      isAllowedImageUrl("https://cdn.payverge.io/biz/1/promo.jpg", hosts),
    ).toBe(true);
  });

  it("rejects a hostile off-allowlist URL (exfil/tracking pixel)", () => {
    const hosts = computeAllowedImageHosts(menu);
    expect(
      isAllowedImageUrl(
        "https://evil.example.com/pixel.gif?leak=secret",
        hosts,
      ),
    ).toBe(false);
  });

  it("rejects non-http(s) schemes outright", () => {
    const hosts = computeAllowedImageHosts(menu);
    expect(isAllowedImageUrl("javascript:alert(1)", hosts)).toBe(false);
    expect(isAllowedImageUrl("data:image/png;base64,AAAA", hosts)).toBe(false);
  });

  it("rejects malformed URLs", () => {
    const hosts = computeAllowedImageHosts(menu);
    expect(isAllowedImageUrl("http://", hosts)).toBe(false);
    expect(isAllowedImageUrl("", hosts)).toBe(false);
  });

  it("allows structured entity media from the canonical image-origin manifest", () => {
    expect(
      isTrustedEntityImageUrl(
        "https://images.unsplash.com/photo-1568901346375-23c9450c58cd",
      ),
    ).toBe(true);
  });

  it("trusts this deployment's own /media uploads, relative or absolute", () => {
    // jest.setup.js pins PUBLIC_URL to https://payverge.io.
    expect(isTrustedEntityImageUrl("/media/business/1/burger.webp")).toBe(true);
    expect(
      isTrustedEntityImageUrl("https://payverge.io/media/business/1/burger.webp"),
    ).toBe(true);
  });

  it.each([
    "/media/../api/v1/admin",
    "/media/%2e%2e/api/v1/admin",
    "/media/",
    "/api/v1/media/x.png",
    "https://payverge.io.evil.example/media/x.png",
    "https://evil.example/media/x.png",
    "http://payverge.io/media/x.png",
  ])("does not treat %s as a same-origin upload", (url) => {
    expect(isTrustedEntityImageUrl(url)).toBe(false);
  });

  it("allows an explicitly configured structured media origin", () => {
    expect(
      isTrustedEntityImageUrl(
        "https://img.restaurant.example/burger.jpg",
        "https://img.restaurant.example",
      ),
    ).toBe(true);
  });

  it("does not trust a foreign structured-media host merely because a live menu row uses it", () => {
    expect(isTrustedEntityImageUrl("https://evil.example/track.gif")).toBe(
      false,
    );
  });

  it.each([
    "http://images.unsplash.com/burger.jpg",
    "https://guest:secret@images.unsplash.com/burger.jpg",
    "//images.unsplash.com/burger.jpg",
    "https://images.unsplash.com.evil/burger.jpg",
    "https://images.unsplash.com:444/burger.jpg",
    "https://images.unsplash.com\\@evil.example/burger.jpg",
    " https://images.unsplash.com/burger.jpg",
    "https://images.unsplash.com/burger.jpg\n",
  ])("rejects adversarial structured-media URL %s", (url) => {
    expect(isTrustedEntityImageUrl(url)).toBe(false);
  });

  it("requires explicitly configured origins to use HTTPS without credentials", () => {
    const image = "https://img.restaurant.example/burger.jpg";
    expect(
      isTrustedEntityImageUrl(image, "http://img.restaurant.example"),
    ).toBe(false);
    expect(
      isTrustedEntityImageUrl(
        image,
        "https://guest:secret@img.restaurant.example",
      ),
    ).toBe(false);
  });

  it("matches an explicitly configured port exactly", () => {
    expect(
      isTrustedEntityImageUrl(
        "https://img.restaurant.example:8443/burger.jpg",
        "https://img.restaurant.example:8443",
      ),
    ).toBe(true);
    expect(
      isTrustedEntityImageUrl(
        "https://img.restaurant.example/burger.jpg",
        "https://img.restaurant.example:8443",
      ),
    ).toBe(false);
  });
});
