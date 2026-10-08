import {
  ATTR_QUERY,
  attachAttributionQuery,
  attributionFileText,
  buildAttributionTag,
  buildPromoCode,
  promoCodeFromUrl,
} from "./attribution";

describe("buildPromoCode", () => {
  it("is deterministic for the same play + suggestion", () => {
    const a = buildPromoCode({ play: "happy_hour", suggestionId: "sug-1" });
    const b = buildPromoCode({ play: "happy_hour", suggestionId: "sug-1" });
    expect(a).toBe(b);
    expect(a).toMatch(/^PV-HH-[0-9A-F]{4}$/);
  });

  it("changes when suggestion id changes", () => {
    const a = buildPromoCode({ play: "offer", suggestionId: "a" });
    const b = buildPromoCode({ play: "offer", suggestionId: "b" });
    expect(a).not.toBe(b);
    expect(a).toMatch(/^PV-OF-/);
  });

  it("falls back to GEN when play empty", () => {
    expect(buildPromoCode({ play: "", suggestionId: "" })).toMatch(
      /^PV-GEN-[0-9A-F]{4}$/,
    );
  });
});

describe("attachAttributionQuery", () => {
  it("returns null for empty or private URLs", () => {
    expect(attachAttributionQuery("", { promoCode: "PV-X-1" })).toBeNull();
    expect(
      attachAttributionQuery(
        "https://bucket.s3.amazonaws.com/x?X-Amz-Signature=abc",
        { promoCode: "PV-X-1" },
      ),
    ).toBeNull();
  });

  it("stamps pv_ref / pv_mkt / pv_dest on public storefront URLs", () => {
    const out = attachAttributionQuery(
      "https://payverge.io/b/bistro?tab=menu",
      {
        promoCode: "PV-HH-ABCD",
        play: "happy_hour",
        destinationId: "print_tent",
      },
    );
    expect(out).toBeTruthy();
    const u = new URL(out!);
    expect(u.searchParams.get(ATTR_QUERY.ref)).toBe("PV-HH-ABCD");
    expect(u.searchParams.get(ATTR_QUERY.play)).toBe("happy_hour");
    expect(u.searchParams.get(ATTR_QUERY.dest)).toBe("print_tent");
    expect(u.searchParams.get("tab")).toBe("menu");
    expect(u.pathname).toBe("/b/bistro");
  });

  it("result remains a public guest URL", () => {
    const out = attachAttributionQuery("https://payverge.io/b/cafe", {
      promoCode: "PV-FD-1111",
      play: "featured_dish",
    });
    // isPublicGuestUrl accepts query strings on /b/{slug}
    expect(out).toContain("pv_ref=PV-FD-1111");
  });
});

describe("buildAttributionTag + file text", () => {
  it("builds tag and documents limitations without schedule language", () => {
    const tag = buildAttributionTag({
      play: "featured_dish",
      suggestionId: "s-9",
      destinationId: "print_window",
    });
    expect(tag.promoCode).toMatch(/^PV-FD-/);
    const text = attributionFileText({
      tag,
      attributedUrl: "https://payverge.io/b/x?pv_ref=PV-FD-1",
    });
    expect(text).toContain(tag.promoCode);
    expect(text).toContain("Limitations");
    expect(text.toLowerCase()).not.toMatch(/schedul/);
    expect(text.toLowerCase()).not.toContain("meta ads");
  });

  it("honours promoCode override", () => {
    const tag = buildAttributionTag({
      play: "offer",
      suggestionId: "s",
      promoCode: "PV-CUSTOM-99",
    });
    expect(tag.promoCode).toBe("PV-CUSTOM-99");
  });
});

describe("promoCodeFromUrl", () => {
  it("reads pv_ref", () => {
    expect(
      promoCodeFromUrl("https://payverge.io/b/x?pv_ref=PV-HH-ABCD&tab=menu"),
    ).toBe("PV-HH-ABCD");
    expect(promoCodeFromUrl("https://payverge.io/b/x")).toBeNull();
  });

  it("returns null for garbage URLs", () => {
    expect(promoCodeFromUrl(null)).toBeNull();
    expect(promoCodeFromUrl("not-a-url")).toBeNull();
  });
});

describe("buildPromoCode play abbreviations", () => {
  it.each([
    ["happy_hour", "HH"],
    ["featured_dish", "FD"],
    ["move_item", "MV"],
    ["win_back", "WB"],
    ["combo_deal", "CD"],
    ["offer", "OF"],
  ] as const)("%s → PV-%s-", (play, abbrev) => {
    expect(buildPromoCode({ play, suggestionId: "s" })).toMatch(
      new RegExp(`^PV-${abbrev}-[0-9A-F]{4}$`),
    );
  });

  it("abbreviates unknown plays from slug characters", () => {
    expect(buildPromoCode({ play: "flash_sale", suggestionId: "1" })).toMatch(
      /^PV-FLAS-[0-9A-F]{4}$/,
    );
  });
});

describe("attributionFileText completeness", () => {
  it("includes activity id and destination when provided", () => {
    const text = attributionFileText({
      tag: {
        promoCode: "PV-FD-1111",
        play: "featured_dish",
        destinationId: "print_tent",
        suggestionId: "sug-9",
        activityId: 42,
      },
      attributedUrl: null,
    });
    expect(text).toContain("Activity id: 42");
    expect(text).toContain("Destination: print_tent");
    expect(text).toContain("Suggestion id: sug-9");
    expect(text).toContain("(none — storefront page off or no slug)");
    expect(text.toLowerCase()).not.toMatch(/schedul|calendar|queue/);
  });
});
