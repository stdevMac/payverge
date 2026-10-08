import {
  DEFAULT_BRAND_LINKS,
  PROJECT_REPO_URL,
  brandLinks,
  resolveBrandLinks,
  sanitizeBrandEmail,
  sanitizeBrandUrl,
} from "./brand";

describe("brand links defaults", () => {
  it("routes support to the public GitHub repository", () => {
    expect(PROJECT_REPO_URL).toBe("https://github.com/stdevMac/payverge");
    expect(brandLinks.repoUrl).toBe(PROJECT_REPO_URL);
    expect(brandLinks.supportUrl).toBe(`${PROJECT_REPO_URL}/issues`);
  });

  it("ships no phone, chat handle or booking link by default", () => {
    expect(brandLinks.whatsappUrl).toBeUndefined();
    expect(brandLinks.telegramUrl).toBeUndefined();
    expect(brandLinks.bookCallUrl).toBeUndefined();
    const serialized = JSON.stringify(DEFAULT_BRAND_LINKS);
    expect(serialized).not.toMatch(/wa\.me|calendar\.app\.google|calendly|\+?971/);
  });

  it("ships no contact mailbox or social profile by default", () => {
    // A zero-config self-host must not route its guests and staff to the
    // upstream project's inbox.
    expect(brandLinks.contactEmail).toBeUndefined();
    expect(Object.keys(DEFAULT_BRAND_LINKS).sort()).toEqual([
      "repoUrl",
      "supportUrl",
    ]);
    expect(JSON.stringify(brandLinks)).not.toMatch(/payverge\.io|x\.com|twitter\.com/);
  });
});

describe("resolveBrandLinks", () => {
  it("accepts valid http(s) overrides", () => {
    const links = resolveBrandLinks({
      bookCallUrl: " https://cal.example.com/support ",
      telegramUrl: "https://t.me/example",
    });
    expect(links.bookCallUrl).toBe("https://cal.example.com/support");
    expect(links.telegramUrl).toBe("https://t.me/example");
  });

  it("clears optional entries with an empty or null override", () => {
    const links = resolveBrandLinks({ telegramUrl: "", contactEmail: null });
    expect(links.telegramUrl).toBeUndefined();
    expect(links.contactEmail).toBeUndefined();
  });

  it("never clears the repository or support link", () => {
    const links = resolveBrandLinks({ repoUrl: "", supportUrl: null });
    expect(links.repoUrl).toBe(DEFAULT_BRAND_LINKS.repoUrl);
    expect(links.supportUrl).toBe(DEFAULT_BRAND_LINKS.supportUrl);
  });

  it("ignores malformed values and keeps the default", () => {
    const links = resolveBrandLinks({
      supportUrl: "javascript:alert(1)",
      bookCallUrl: "not a url",
      contactEmail: "nope",
      whatsappUrl: "ftp://wa.example.com/handle",
    });
    expect(links.supportUrl).toBe(DEFAULT_BRAND_LINKS.supportUrl);
    expect(links.bookCallUrl).toBeUndefined();
    expect(links.contactEmail).toBeUndefined();
    expect(links.whatsappUrl).toBeUndefined();
  });

  it("accepts an operator contact mailbox", () => {
    const links = resolveBrandLinks({
      contactEmail: " help@restaurant.example ",
    });
    expect(links.contactEmail).toBe("help@restaurant.example");
  });
});

describe("sanitizers", () => {
  it("rejects non-http schemes and embedded credentials", () => {
    expect(sanitizeBrandUrl("mailto:a@b.co")).toBeUndefined();
    expect(sanitizeBrandUrl("https://user:pw@example.com")).toBeUndefined();
    expect(sanitizeBrandUrl(42)).toBeUndefined();
    expect(sanitizeBrandUrl("https://example.com/help")).toBe(
      "https://example.com/help",
    );
  });

  it("accepts one plain mailbox only", () => {
    expect(sanitizeBrandEmail("help@example.com")).toBe("help@example.com");
    expect(sanitizeBrandEmail("a@b.co?subject=x")).toBeUndefined();
    expect(sanitizeBrandEmail("two words@example.com")).toBeUndefined();
  });
});

describe("brandLinks contact mailbox follows runtime SUPPORT_EMAIL", () => {
  const original = process.env.SUPPORT_EMAIL;

  afterEach(() => {
    if (original === undefined) delete process.env.SUPPORT_EMAIL;
    else process.env.SUPPORT_EMAIL = original;
  });

  function loadBrandLinks(): typeof brandLinks {
    let links: typeof brandLinks | undefined;
    jest.isolateModules(() => {
      links = (require("./brand") as typeof import("./brand")).brandLinks;
    });
    return links as typeof brandLinks;
  }

  it("shows the deployment's SUPPORT_EMAIL as the contact mailbox", () => {
    process.env.SUPPORT_EMAIL = "help@restaurant.example";
    expect(loadBrandLinks().contactEmail).toBe("help@restaurant.example");
  });

  it("keeps the mailbox hidden when SUPPORT_EMAIL is unset or malformed", () => {
    delete process.env.SUPPORT_EMAIL;
    expect(loadBrandLinks().contactEmail).toBeUndefined();
    process.env.SUPPORT_EMAIL = "not-an-address";
    expect(loadBrandLinks().contactEmail).toBeUndefined();
  });
});
