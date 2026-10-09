import {
  normalizeUrlParam,
  buildSearchWithParam,
  buildSearchWithParams,
  needsUrlNormalization,
} from "../urlState";

const SECTIONS = ["profile", "payments", "localization", "notifications"] as const;
const PERIODS = ["today", "week", "month", "year"] as const;

describe("normalizeUrlParam", () => {
  it("returns a valid value unchanged", () => {
    expect(normalizeUrlParam("payments", SECTIONS, "profile")).toBe("payments");
    expect(normalizeUrlParam("week", PERIODS, "today")).toBe("week");
  });

  it("falls back when raw is missing", () => {
    expect(normalizeUrlParam(null, SECTIONS, "profile")).toBe("profile");
    expect(normalizeUrlParam(undefined, SECTIONS, "profile")).toBe("profile");
    expect(normalizeUrlParam("", SECTIONS, "profile")).toBe("profile");
  });

  it("falls back when raw is invalid garbage", () => {
    expect(normalizeUrlParam("nope", SECTIONS, "profile")).toBe("profile");
    expect(normalizeUrlParam("Hoy", PERIODS, "today")).toBe("today");
    expect(normalizeUrlParam("%%%", PERIODS, "today")).toBe("today");
  });
});

describe("buildSearchWithParam", () => {
  it("writes a non-default value and preserves siblings", () => {
    const out = buildSearchWithParam(
      "tab=settings&foo=1",
      "section",
      "notifications",
      { fallback: "profile" },
    );
    const params = new URLSearchParams(out);
    expect(params.get("tab")).toBe("settings");
    expect(params.get("foo")).toBe("1");
    expect(params.get("section")).toBe("notifications");
  });

  it("omits the key when value equals fallback (default omitDefault)", () => {
    const out = buildSearchWithParam("tab=settings&section=payments", "section", "profile", {
      fallback: "profile",
    });
    const params = new URLSearchParams(out);
    expect(params.get("section")).toBeNull();
    expect(params.get("tab")).toBe("settings");
  });

  it("keeps the key when omitDefault is false even if value is fallback", () => {
    const out = buildSearchWithParam("", "period", "today", {
      fallback: "today",
      omitDefault: false,
    });
    expect(new URLSearchParams(out).get("period")).toBe("today");
  });

  it("removes the key when value is null/empty", () => {
    const out = buildSearchWithParam("tab=staff&staffSearch=Ana", "staffSearch", "");
    expect(new URLSearchParams(out).get("staffSearch")).toBeNull();
    expect(new URLSearchParams(out).get("tab")).toBe("staff");
  });
});

describe("buildSearchWithParams", () => {
  it("writes tab+sub+focus in one search string without dropping siblings (#376)", () => {
    const out = buildSearchWithParams("tab=crm&sub=segments", [
      { key: "tab", value: "crm" },
      { key: "sub", value: "customers" },
      { key: "focus", value: "at-risk" },
    ]);
    const params = new URLSearchParams(out);
    expect(params.get("tab")).toBe("crm");
    expect(params.get("sub")).toBe("customers");
    expect(params.get("focus")).toBe("at-risk");
  });

  it("clears a key when the patch value is null", () => {
    const out = buildSearchWithParams("tab=crm&sub=customers&focus=vip", [
      { key: "focus", value: null },
    ]);
    const params = new URLSearchParams(out);
    expect(params.get("focus")).toBeNull();
    expect(params.get("sub")).toBe("customers");
  });
});

describe("needsUrlNormalization", () => {
  it("is false when raw already matches normalized", () => {
    expect(needsUrlNormalization("week", "week", { fallback: "today" })).toBe(false);
  });

  it("is true when raw is garbage that normalizes to fallback", () => {
    expect(needsUrlNormalization("garbage", "today", { fallback: "today" })).toBe(true);
  });

  it("is false when key is absent and normalized is the omitted default", () => {
    expect(
      needsUrlNormalization(null, "today", { fallback: "today", omitDefault: true }),
    ).toBe(false);
  });

  it("is true when raw is a non-default that should stay (already equal)", () => {
    // Sanity: valid non-default does not need rewrite
    expect(needsUrlNormalization("week", "week", { fallback: "today" })).toBe(false);
  });
});
