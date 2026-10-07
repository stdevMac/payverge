import { FORMATS, type FormatId } from "./formats";
import {
  HERO_GEN_ASPECTS,
  heroGenAspectFor,
  heroGenAspectIsNative,
  isHeroGenAspect,
} from "./heroGenAspect";

describe("heroGenAspectFor", () => {
  it.each([
    ["1:1", "1:1"],
    ["4:5", "4:5"],
    ["9:16", "9:16"],
  ] as const)("passes native %s through as %s", (format, expected) => {
    expect(heroGenAspectFor(format)).toBe(expected);
    expect(heroGenAspectIsNative(format)).toBe(true);
  });

  it("maps print tent 5:7 → 4:5 (intentional re-crop into bleed)", () => {
    expect(heroGenAspectFor("5:7")).toBe("4:5");
    expect(heroGenAspectIsNative("5:7")).toBe(false);
  });

  it("maps wide and strip → 1:1 (intentional horizontal re-crop)", () => {
    expect(heroGenAspectFor("wide")).toBe("1:1");
    expect(heroGenAspectFor("strip")).toBe("1:1");
    expect(heroGenAspectIsNative("wide")).toBe(false);
    expect(heroGenAspectIsNative("strip")).toBe(false);
  });

  it("defaults unknown / empty to feed 4:5", () => {
    expect(heroGenAspectFor("")).toBe("4:5");
    expect(heroGenAspectFor(undefined)).toBe("4:5");
    expect(heroGenAspectFor("16:9")).toBe("4:5");
    expect(heroGenAspectFor("   ")).toBe("4:5");
  });

  it("covers every FormatId in the registry without inventing gen aspects", () => {
    const ids = Object.keys(FORMATS) as FormatId[];
    for (const id of ids) {
      const gen = heroGenAspectFor(id);
      expect(HERO_GEN_ASPECTS).toContain(gen);
      expect(isHeroGenAspect(gen)).toBe(true);
    }
  });

  it("never returns print/wide/strip as a gen aspect (whitelist pin)", () => {
    for (const id of Object.keys(FORMATS) as FormatId[]) {
      expect(["wide", "strip", "5:7"]).not.toContain(heroGenAspectFor(id));
    }
  });
});
