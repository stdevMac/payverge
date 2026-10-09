import { metadata } from "./layout";

describe("unsubscribe metadata", () => {
  it("owns title, canonical, OG url, and noindex", () => {
    expect(metadata.title).toBe("Unsubscribe — Payverge");
    expect(metadata.alternates?.canonical).toBe("https://payverge.io/unsubscribe");
    expect(metadata.robots).toEqual({ index: false, follow: false });
    expect(String(metadata.openGraph?.url)).toBe(
      "https://payverge.io/unsubscribe",
    );
    expect(metadata.openGraph?.title).toBe(metadata.title);
    expect(String(metadata.openGraph?.url)).not.toBe("https://payverge.io");
  });
});
