import {
  buildGuestLink,
  guestUrlToQrDataUrl,
  isPublicGuestUrl,
  resolveGuestLink,
} from "./guestLinks";

jest.mock("qrcode", () => ({
  __esModule: true,
  default: {
    toDataURL: jest.fn(async (url: string) => `data:image/png;base64,QR(${url})`),
  },
}));

describe("buildGuestLink", () => {
  it.each([
    {
      name: "page off",
      input: { customUrl: "bistro", pageEnabled: false, play: "featured_dish" },
      want: null,
    },
    {
      name: "empty slug",
      input: { customUrl: "", pageEnabled: true, play: "featured_dish" },
      want: null,
    },
    {
      name: "path injection",
      input: { customUrl: "foo/bar", pageEnabled: true, play: "featured_dish" },
      want: null,
    },
    {
      name: "featured dish → menu",
      input: { customUrl: "bistro", pageEnabled: true, play: "featured_dish" },
      want: {
        url: "https://payverge.io/b/bistro?tab=menu",
        kind: "menu" as const,
      },
    },
    {
      name: "win_back → reservations",
      input: { customUrl: "bistro", pageEnabled: true, play: "win_back" },
      want: {
        url: "https://payverge.io/b/bistro?tab=reservations",
        kind: "reservations" as const,
      },
    },
    {
      name: "local origin",
      input: {
        customUrl: "cafe",
        pageEnabled: true,
        play: "offer",
        origin: "http://127.0.0.1:3000",
      },
      want: {
        url: "http://127.0.0.1:3000/b/cafe?tab=menu",
        kind: "menu" as const,
      },
    },
  ])("$name", ({ input, want }) => {
    expect(buildGuestLink(input)).toEqual(want);
  });
});

describe("isPublicGuestUrl", () => {
  it("accepts public storefront URLs", () => {
    expect(isPublicGuestUrl("https://payverge.io/b/bistro?tab=menu")).toBe(true);
  });

  it("rejects empty and private-looking URLs", () => {
    expect(isPublicGuestUrl("")).toBe(false);
    expect(isPublicGuestUrl(null)).toBe(false);
    expect(
      isPublicGuestUrl(
        "https://bucket.s3.amazonaws.com/x.jpg?X-Amz-Signature=abc",
      ),
    ).toBe(false);
    expect(isPublicGuestUrl("javascript:alert(1)")).toBe(false);
    expect(isPublicGuestUrl("https://payverge.io/inside/secret")).toBe(false);
  });
});

describe("resolveGuestLink", () => {
  it("prefers server guest_url when public", () => {
    expect(
      resolveGuestLink({
        guestUrl: "https://payverge.io/b/from-server?tab=menu",
        guestUrlKind: "menu",
        customUrl: "ignored",
        pageEnabled: true,
        play: "offer",
      }),
    ).toEqual({
      url: "https://payverge.io/b/from-server?tab=menu",
      kind: "menu",
    });
  });

  it("falls back to business fields when server url missing", () => {
    expect(
      resolveGuestLink({
        customUrl: "bistro",
        pageEnabled: true,
        play: "featured_dish",
      }),
    ).toEqual({
      url: "https://payverge.io/b/bistro?tab=menu",
      kind: "menu",
    });
  });

  it("ignores signed private guest_url and rebuilds when possible", () => {
    expect(
      resolveGuestLink({
        guestUrl: "https://cdn.example.com/x?X-Amz-Signature=1",
        customUrl: "bistro",
        pageEnabled: true,
        play: "offer",
      }),
    ).toEqual({
      url: "https://payverge.io/b/bistro?tab=menu",
      kind: "menu",
    });
  });
});

describe("guestUrlToQrDataUrl", () => {
  it("returns null when url empty", async () => {
    expect(await guestUrlToQrDataUrl("")).toBeNull();
    expect(await guestUrlToQrDataUrl(null)).toBeNull();
  });

  it("returns a data URL for a public guest link", async () => {
    const data = await guestUrlToQrDataUrl(
      "https://payverge.io/b/bistro?tab=menu",
    );
    expect(data).toMatch(/^data:image\/png;base64,/);
  });

  it("returns null for private signed URLs (no QR)", async () => {
    expect(
      await guestUrlToQrDataUrl(
        "https://bucket.s3.amazonaws.com/x.jpg?X-Amz-Signature=abc",
      ),
    ).toBeNull();
  });
});
