import {
  normalize,
  socialProfileUrl,
  type SocialNetwork,
} from "./socialHandle";

describe("normalize(network, raw)", () => {
  describe.each([
    // [network, raw, expectedHandle]
    ["facebook", "payverge", "payverge"],
    ["facebook", "@payverge", "payverge"],
    ["facebook", "facebook.com/payverge", "payverge"],
    ["facebook", "https://facebook.com/payverge", "payverge"],
    ["facebook", "https://www.facebook.com/payverge/", "payverge"],
    ["instagram", "mycafe", "mycafe"],
    ["instagram", "@mycafe", "mycafe"],
    ["instagram", "instagram.com/mycafe", "mycafe"],
    ["instagram", "https://www.instagram.com/mycafe/", "mycafe"],
    ["twitter", "payverge", "payverge"],
    ["twitter", "@payverge", "payverge"],
    ["twitter", "https://x.com/payverge", "payverge"],
    ["twitter", "https://twitter.com/payverge/", "payverge"],
    ["linkedin", "payverge", "payverge"],
    ["linkedin", "linkedin.com/company/payverge", "payverge"],
    ["linkedin", "https://www.linkedin.com/company/payverge/", "payverge"],
    ["youtube", "payverge", "payverge"],
    ["youtube", "@payverge", "payverge"],
    ["youtube", "https://youtube.com/@payverge", "payverge"],
    ["youtube", "https://www.youtube.com/@payverge/", "payverge"],
    ["tiktok", "payverge", "payverge"],
    ["tiktok", "@payverge", "payverge"],
    ["tiktok", "tiktok.com/@payverge", "payverge"],
    ["tiktok", "https://www.tiktok.com/@payverge/", "payverge"],
  ] as const)("%s %s → %s", (network, raw, expected) => {
    it("returns one canonical handle", () => {
      const result = normalize(network, raw);
      expect(result).toEqual({ ok: true, handle: expected });
    });
  });

  it("returns empty handle for blank input", () => {
    expect(normalize("facebook", "")).toEqual({ ok: true, handle: "" });
    expect(normalize("instagram", "   ")).toEqual({ ok: true, handle: "" });
  });

  it("strips query strings and fragments", () => {
    expect(
      normalize("facebook", "https://www.facebook.com/payverge?ref=page"),
    ).toEqual({ ok: true, handle: "payverge" });
    expect(
      normalize("instagram", "https://instagram.com/mycafe#reels"),
    ).toEqual({ ok: true, handle: "mycafe" });
  });

  it("rejects a URL for the wrong network", () => {
    const cases: Array<[SocialNetwork, string]> = [
      ["facebook", "https://instagram.com/payverge"],
      ["instagram", "https://facebook.com/payverge"],
      ["twitter", "https://tiktok.com/@payverge"],
      ["tiktok", "https://www.instagram.com/payverge"],
      ["linkedin", "https://youtube.com/@payverge"],
      ["youtube", "https://linkedin.com/company/payverge"],
    ];
    for (const [network, raw] of cases) {
      const result = normalize(network, raw);
      expect(result.ok).toBe(false);
      if (!result.ok) {
        expect(result.error).toMatch(/network|wrong|belong|mismatch|expected/i);
      }
    }
  });

  it("rejects handles that fail the network charset", () => {
    const result = normalize("instagram", "bad handle!!");
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.error).toMatch(/invalid|charset|character/i);
    }
  });

  it("rejects facebook.com bare host with empty path as empty (ok)", () => {
    // Host only with trailing slash → nothing left after strip.
    expect(normalize("facebook", "https://facebook.com/")).toEqual({
      ok: true,
      handle: "",
    });
  });
});

describe("socialProfileUrl", () => {
  it("builds the canonical profile URL from a bare handle", () => {
    expect(socialProfileUrl("instagram", "mycafe")).toBe(
      "https://instagram.com/mycafe",
    );
    expect(socialProfileUrl("facebook", "payverge")).toBe(
      "https://facebook.com/payverge",
    );
    expect(socialProfileUrl("twitter", "payverge")).toBe(
      "https://twitter.com/payverge",
    );
    expect(socialProfileUrl("linkedin", "payverge")).toBe(
      "https://linkedin.com/company/payverge",
    );
    expect(socialProfileUrl("youtube", "payverge")).toBe(
      "https://youtube.com/@payverge",
    );
    expect(socialProfileUrl("tiktok", "payverge")).toBe(
      "https://tiktok.com/@payverge",
    );
  });

  it("returns empty string for empty handle", () => {
    expect(socialProfileUrl("instagram", "")).toBe("");
  });
});
