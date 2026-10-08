import {
  contrastRatio,
  isPrintSafePalette,
  resolvePrintPalette,
} from "./palette";
import { MENU_DESIGN_FAMILY_IDS, type PrintPalette } from "./types";

describe("print palette resolution", () => {
  it("computes the known WCAG contrast ratio for black and white", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 10);
    expect(contrastRatio("#FFFFFF", "#000000")).toBeCloseTo(21, 10);
  });

  it.each(["#fff", "ffffff", "#gggggg", "#00000000", " #000000", "#000000\n"])(
    "strictly rejects invalid full hex color %s",
    (color) => {
      expect(() => contrastRatio(color, "#ffffff")).toThrow(TypeError);
    },
  );

  it("enforces the 4.5:1 text contrast floor", () => {
    const palette = (ink: string): PrintPalette => ({
      ground: "#ffffff",
      ink,
      accent: "#000000",
      muted: "#000000",
    });

    expect(isPrintSafePalette(palette("#767676"))).toBe(true);
    expect(isPrintSafePalette(palette("#777777"))).toBe(false);
    expect(isPrintSafePalette(palette("not-a-color"))).toBe(false);
  });

  it("uses safe restaurant colors and rejects low-contrast input", () => {
    expect(
      resolvePrintPalette({
        familyId: "atelier",
        primaryColor: "#145C55",
        secondaryColor: "#D9A441",
      }),
    ).toEqual(
      expect.objectContaining({ accent: "#145c55", source: "restaurant" }),
    );
    expect(
      resolvePrintPalette({
        familyId: "atelier",
        primaryColor: "#fefefe",
        secondaryColor: "#ffffff",
      }),
    ).toEqual(expect.objectContaining({ source: "family-fallback" }));
  });

  it("preserves a light restaurant accent on a contrasting dark ground", () => {
    const palette = resolvePrintPalette({
      familyId: "night-house",
      primaryColor: "#ffffff",
    });

    expect(palette).toEqual(
      expect.objectContaining({ accent: "#ffffff", source: "restaurant" }),
    );
    expect(isPrintSafePalette(palette)).toBe(true);
  });

  it("falls back for malformed restaurant colors", () => {
    expect(
      resolvePrintPalette({
        familyId: "maison",
        primaryColor: "#123",
        secondaryColor: "#ffffff",
      }),
    ).toEqual(expect.objectContaining({ source: "family-fallback" }));
  });

  it("preserves a safe primary when the optional secondary is malformed", () => {
    const fallback = resolvePrintPalette({ familyId: "atelier" });
    const palette = resolvePrintPalette({
      familyId: "atelier",
      primaryColor: "#145c55",
      secondaryColor: "bad",
    });

    expect(palette).toEqual(
      expect.objectContaining({
        accent: "#145c55",
        muted: fallback.muted,
        source: "restaurant",
      }),
    );
    expect(isPrintSafePalette(palette)).toBe(true);
  });

  it.each(MENU_DESIGN_FAMILY_IDS)(
    "provides a print-safe fallback for the %s family",
    (familyId) => {
      const palette = resolvePrintPalette({ familyId });

      expect(palette.source).toBe("family-fallback");
      expect(isPrintSafePalette(palette)).toBe(true);
      expect(Object.keys(palette).sort()).toEqual(
        ["accent", "ground", "ink", "muted", "source"].sort(),
      );
    },
  );

  it("returns the same result for repeated calls without sharing objects", () => {
    const input = {
      familyId: "gallery" as const,
      primaryColor: "#145c55",
      secondaryColor: "#d9a441",
    };

    const first = resolvePrintPalette(input);
    const second = resolvePrintPalette(input);

    expect(second).toEqual(first);
    expect(second).not.toBe(first);
  });
});
