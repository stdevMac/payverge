import { isOwnMediaUrl, isRelativeOwnMediaUrl } from "./ownMedia";

const SITE = "https://pos.example.com";

describe("isOwnMediaUrl", () => {
  it("accepts the relative /media/<key> form the backend persists by default", () => {
    expect(
      isOwnMediaUrl(
        "/media/businesses/1/menu_items/0123456789abcdef_dish.png",
        SITE,
      ),
    ).toBe(true);
    expect(isOwnMediaUrl("/media/demo/hero.webp", undefined)).toBe(true);
    expect(isRelativeOwnMediaUrl("/media/businesses/1/icons/x.png")).toBe(true);
  });

  it("accepts ${PUBLIC_URL}/media/<key> on the site's own origin, http included", () => {
    expect(isOwnMediaUrl(`${SITE}/media/businesses/1/a.png`, SITE)).toBe(true);
    expect(
      isOwnMediaUrl(
        "http://192.168.1.20:3000/media/businesses/1/a.png",
        "http://192.168.1.20:3000",
      ),
    ).toBe(true);
  });

  it("rejects absolute URLs on other origins or without a known origin", () => {
    expect(
      isOwnMediaUrl("https://evil.example/media/businesses/1/a.png", SITE),
    ).toBe(false);
    expect(isOwnMediaUrl("http://pos.example.com/media/a.png", SITE)).toBe(
      false,
    );
    expect(isOwnMediaUrl(`${SITE}/media/a.png`, undefined)).toBe(false);
    expect(isRelativeOwnMediaUrl(`${SITE}/media/a.png`)).toBe(false);
  });

  it("rejects traversal, encoded dot segments and non-media paths", () => {
    for (const raw of [
      "/media/../api/v1/inside/me",
      "/media/%2e%2e/api/v1/x",
      "/media/businesses/../../api",
      "/media/.hidden/a.png",
      "/media/businesses//a.png",
      "/media/",
      "/media",
      "/api/v1/media/a.png",
      "/mediax/a.png",
      `${SITE}/media/../api/x`,
    ]) {
      expect(isOwnMediaUrl(raw, SITE)).toBe(false);
    }
  });

  it("rejects protocol-relative, credentials, query, fragment and odd characters", () => {
    for (const raw of [
      "//evil.example/media/a.png",
      "/media/a.png?x=1",
      "/media/a.png#frag",
      `${SITE}/media/a.png?x=1`,
      "https://user:pw@pos.example.com/media/a.png",
      "/media/a b.png",
      "/media/a%20b.png",
      "/media/a\\b.png",
      " /media/a.png",
      "javascript:/media/a.png",
      "",
    ]) {
      expect(isOwnMediaUrl(raw, SITE)).toBe(false);
    }
  });
});
