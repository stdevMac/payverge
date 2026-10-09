import {
  getRadiusClass,
  getShadowClass,
  getSectionPadding,
  getMenuLayoutClass,
  getMenuCardClass,
  getMenuMediaClass,
  getHeroTreatment,
  DEFAULT_DESIGN_SETTINGS,
} from "../designClasses";
import { readFileSync } from "fs";
import { join } from "path";

const heroSkeletonSource = readFileSync(
  join(__dirname, "..", "HeroSkeleton.tsx"),
  "utf8",
);

describe("getRadiusClass", () => {
  it.each([
    ["none", "rounded-none"],
    ["small", "rounded-sm"],
    ["medium", "rounded-lg"],
    ["large", "rounded-xl"],
    [undefined, "rounded-lg"],
    ["bogus", "rounded-lg"],
  ])("maps %p -> %p", (input, expected) => {
    expect(getRadiusClass(input)).toBe(expected);
  });
});

describe("getShadowClass", () => {
  it.each([
    ["none", "shadow-none"],
    ["subtle", "shadow-sm"],
    ["medium", "shadow-md"],
    ["strong", "shadow-xl"],
    [undefined, "shadow-sm"],
    ["bogus", "shadow-sm"],
  ])("maps %p -> %p", (input, expected) => {
    expect(getShadowClass(input)).toBe(expected);
  });
});

describe("getSectionPadding", () => {
  it("compact density is tighter than comfortable", () => {
    expect(getSectionPadding("compact")).toBe("py-8 md:py-12");
    expect(getSectionPadding("comfortable")).toBe("py-12 md:py-16");
  });

  it("defaults to comfortable", () => {
    expect(getSectionPadding(undefined)).toBe("py-12 md:py-16");
    expect(getSectionPadding("bogus")).toBe("py-12 md:py-16");
  });
});

describe("DEFAULT_DESIGN_SETTINGS", () => {
  it("uses the merchant-facing tokens the plan depends on", () => {
    expect(DEFAULT_DESIGN_SETTINGS.font_family).toBe("Inter");
    expect(DEFAULT_DESIGN_SETTINGS.theme).toBe("light");
    expect(DEFAULT_DESIGN_SETTINGS.menu_layout).toBe("grid");
    expect(DEFAULT_DESIGN_SETTINGS.show_images).toBe(true);
    expect(DEFAULT_DESIGN_SETTINGS.show_descriptions).toBe(true);
    expect(DEFAULT_DESIGN_SETTINGS.header_style).toBe("banner");
    expect(DEFAULT_DESIGN_SETTINGS.corner_radius).toBe("medium");
    expect(DEFAULT_DESIGN_SETTINGS.shadow_intensity).toBe("subtle");
    expect(DEFAULT_DESIGN_SETTINGS.background_pattern).toBe("none");
    expect(DEFAULT_DESIGN_SETTINGS.pattern_opacity).toBe(0.1);
    expect(DEFAULT_DESIGN_SETTINGS.hero_layout).toBe("centered");
    expect(DEFAULT_DESIGN_SETTINGS.section_density).toBe("comfortable");
  });

  it("ships non-empty primary and secondary colors", () => {
    expect(DEFAULT_DESIGN_SETTINGS.primary_color).toMatch(/^#[0-9a-f]{6}$/i);
    expect(DEFAULT_DESIGN_SETTINGS.secondary_color).toMatch(/^#[0-9a-f]{6}$/i);
  });
});

describe("getMenuLayoutClass", () => {
  it("returns a multi-column grid for grid (or unset/bogus)", () => {
    const grid = "grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6";
    expect(getMenuLayoutClass("grid")).toBe(grid);
    expect(getMenuLayoutClass(undefined)).toBe(grid);
    expect(getMenuLayoutClass("bogus")).toBe(grid);
  });

  it("returns a single-column stack for list", () => {
    expect(getMenuLayoutClass("list")).toBe("flex flex-col gap-4");
  });
});

describe("getMenuCardClass / getMenuMediaClass", () => {
  it("grid card stacks media on top; list card goes side-by-side on md", () => {
    expect(getMenuCardClass("grid")).toBe("");
    expect(getMenuCardClass("list")).toBe("md:flex md:items-stretch");
  });

  it("list media takes a fixed left column on md; grid media is full width", () => {
    expect(getMenuMediaClass("grid")).toBe("");
    expect(getMenuMediaClass("list")).toBe("md:w-56 md:flex-shrink-0");
  });
});

describe("getHeroTreatment", () => {
  it("banner: full banner background allowed, tall hero, layout not forced", () => {
    const tr = getHeroTreatment("banner");
    expect(tr.showBanner).toBe(true);
    expect(tr.forceCentered).toBe(false);
    expect(tr.minHeightClass).toBe("min-h-[56vh] md:min-h-[68vh]");
  });

  it("minimal: no banner background, compact band, forces centered", () => {
    const tr = getHeroTreatment("minimal");
    expect(tr.showBanner).toBe(false);
    expect(tr.forceCentered).toBe(true);
    expect(tr.minHeightClass).toBe("min-h-[40vh] md:min-h-[44vh]");
  });

  it("classic: banner as a slim strip, reduced height, forces centered", () => {
    const tr = getHeroTreatment("classic");
    expect(tr.bannerAsStrip).toBe(true);
    expect(tr.forceCentered).toBe(true);
    expect(tr.minHeightClass).toBe("min-h-[48vh] md:min-h-[56vh]");
  });

  it("unknown/undefined falls back to banner", () => {
    expect(getHeroTreatment(undefined).showBanner).toBe(true);
    expect(getHeroTreatment("bogus").minHeightClass).toBe("min-h-[56vh] md:min-h-[68vh]");
  });

  it("keeps the public-page hero skeleton at the default banner height", () => {
    expect(heroSkeletonSource).toContain("min-h-[56vh] md:min-h-[68vh]");
    expect(heroSkeletonSource).not.toContain("md:min-h-[78vh]");
  });
});
