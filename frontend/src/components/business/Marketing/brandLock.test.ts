import {
  applyHashtagPreference,
  brandHandle,
  brandLockSummary,
  brandLogoUrl,
  ctaLabelForBrand,
  fontFamilyFromKit,
  kitBiasFromVisualMood,
  resolveBrandPalette,
  resolveMarketingFontFamily,
  seedKitForCreative,
  shouldSuppressHashtags,
} from "./brandLock";
import { KIT_ORDER, KITS, type KitId } from "./artDirection/kits";
import type { Business } from "@/api/business";

function business(
  overrides: Partial<Business> & {
    design_settings?: Business["design_settings"];
  } = {},
): Business {
  return {
    id: 1,
    name: "Casa Sur",
    logo: "https://cdn.example.com/logo.png",
    social_media: "@casasur",
    design_settings: {
      primary_color: "#112233",
      secondary_color: "#445566",
      font_family: "Serif",
      ...overrides.design_settings,
    },
    ...overrides,
  } as Business;
}

const t = (key: string) => {
  const map: Record<string, string> = {
    "defaults.cta.default": "VISIT US",
    "defaults.cta.featured_dish": "TRY IT",
    "defaults.cta.offer": "CLAIM OFFER",
    "defaults.ctaStyle.soft": "COME BY",
    "defaults.ctaStyle.direct": "ORDER NOW",
    "defaults.ctaStyle.urgent": "TODAY ONLY",
  };
  return map[key] ?? key;
};

describe("fontFamilyFromKit / resolveMarketingFontFamily", () => {
  it.each(KIT_ORDER)(
    "maps %s display face to wire font honestly (never design_settings)",
    (kit: KitId) => {
      const want =
        KITS[kit].typePairing.display === "serif" ? "Serif" : "Sans";
      expect(fontFamilyFromKit(kit)).toBe(want);
      expect(resolveMarketingFontFamily({ kit })).toBe(want);
    },
  );

  it("does not honour design_settings.font_family as a kit face override", () => {
    // Business is Serif in design settings, but bold kit is sans display.
    const palette = resolveBrandPalette(business(), { kit: "bold" });
    expect(palette.fontFamily).toBe("Sans");
    expect(palette.primary).toBe("#112233");
    expect(palette.secondary).toBe("#445566");
  });

  it("falls back to Sans when kit and stored font are absent", () => {
    expect(resolveMarketingFontFamily({})).toBe("Sans");
  });

  it("accepts legacy Inter/Sans/Serif stored fonts only when no kit", () => {
    expect(resolveMarketingFontFamily({ storedFontFamily: "Inter" })).toBe(
      "Sans",
    );
    expect(resolveMarketingFontFamily({ storedFontFamily: "serif" })).toBe(
      "Serif",
    );
    // Kit still wins over stored.
    expect(
      resolveMarketingFontFamily({
        kit: "linen",
        storedFontFamily: "Sans",
      }),
    ).toBe("Serif");
  });
});

describe("kitBiasFromVisualMood / seedKitForCreative", () => {
  it.each([
    ["bright", "bold"],
    ["moody", "chalkboard"],
    ["editorial", "editorial"],
    ["rustic", "linen"],
  ] as const)("biases %s → %s", (mood, kit) => {
    expect(kitBiasFromVisualMood(mood)).toBe(kit);
  });

  it("leaves natural and blank mood unbiased", () => {
    expect(kitBiasFromVisualMood("natural")).toBeNull();
    expect(kitBiasFromVisualMood("")).toBeNull();
    expect(kitBiasFromVisualMood(null)).toBeNull();
  });

  it("stored kit wins over mood bias", () => {
    expect(
      seedKitForCreative({
        storedKit: "ticket",
        visualMood: "bright",
        templateStyle: "editorial",
      }),
    ).toBe("ticket");
  });

  it("mood bias wins over play template when no stored kit", () => {
    expect(
      seedKitForCreative({
        visualMood: "moody",
        templateStyle: "minimal",
      }),
    ).toBe("chalkboard");
  });

  it("falls back to legacy template map when mood is blank", () => {
    expect(
      seedKitForCreative({
        visualMood: "",
        templateStyle: "bold",
      }),
    ).toBe("bold");
    expect(
      seedKitForCreative({
        visualMood: "natural",
        templateStyle: "minimal",
      }),
    ).toBe("minimal");
  });

  it("ignores unknown stored kit ids and re-seeds from mood/template", () => {
    expect(
      seedKitForCreative({
        storedKit: "papyrus-pro",
        visualMood: "rustic",
        templateStyle: "editorial",
      }),
    ).toBe("linen");
  });
});

describe("resolveBrandPalette", () => {
  it("uses design colour fallbacks when missing", () => {
    const palette = resolveBrandPalette(
      {
        design_settings: {
          primary_color: "",
          secondary_color: "",
          font_family: "Inter",
        },
      } as Pick<Business, "design_settings">,
      { kit: "editorial" },
    );
    expect(palette.primary).toBe("#1a6b6a");
    expect(palette.secondary).toBe("#0f3d3c");
    expect(palette.fontFamily).toBe("Serif");
  });
});

describe("ctaLabelForBrand", () => {
  it("prefers cta_style labels when set", () => {
    expect(ctaLabelForBrand(t, "featured_dish", "direct")).toBe("ORDER NOW");
    expect(ctaLabelForBrand(t, "offer", "urgent")).toBe("TODAY ONLY");
    expect(ctaLabelForBrand(t, "featured_dish", "soft")).toBe("COME BY");
  });

  it("falls back to play CTA when style blank", () => {
    expect(ctaLabelForBrand(t, "featured_dish", "")).toBe("TRY IT");
    expect(ctaLabelForBrand(t, undefined, "")).toBe("VISIT US");
  });
});

describe("handle + hashtag preferences", () => {
  it("normalizes business handle", () => {
    expect(brandHandle(business())).toBe("@casasur");
    expect(brandHandle(business({ social_media: "" }))).toBe("");
  });

  it("suppresses hashtags when behavior is none", () => {
    expect(shouldSuppressHashtags("none")).toBe(true);
    expect(shouldSuppressHashtags("light")).toBe(false);
    expect(
      applyHashtagPreference("Try our pasta #food #yummy @casasur", "none"),
    ).toBe("Try our pasta @casasur");
    expect(
      applyHashtagPreference("Try our pasta #food", "standard"),
    ).toBe("Try our pasta #food");
  });
});

describe("brandLockSummary", () => {
  it("surfaces logo, colours, mood kit, and copy prefs", () => {
    const summary = brandLockSummary(business(), {
      visual_mood: "bright",
      cta_style: "direct",
      hashtag_behavior: "none",
    });
    expect(summary.logoUrl).toBe("https://cdn.example.com/logo.png");
    expect(summary.primaryColor).toBe("#112233");
    expect(summary.moodKit).toBe("bold");
    expect(summary.fontNote).toBe("kit_owned");
    expect(summary.handle).toBe("@casasur");
    expect(summary.ctaStyle).toBe("direct");
    expect(summary.hashtagBehavior).toBe("none");
  });

  it("reports empty logo when business has none", () => {
    expect(brandLogoUrl(business({ logo: "" }))).toBe("");
    expect(
      brandLockSummary(business({ logo: "  " }), { visual_mood: "natural" })
        .moodKit,
    ).toBeNull();
  });
});
