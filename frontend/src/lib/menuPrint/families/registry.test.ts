import { PRINT_FONT_FAMILIES } from "../fonts";
import {
  contrastRatio,
  isPrintSafePalette,
  resolvePrintPalette,
} from "../palette";
import {
  MENU_DESIGN_FAMILY_IDS,
  MENU_OUTPUT_FORMATS,
  MENU_TREATMENTS,
  type CompositionStrategyId,
  type ImageRole,
  type MenuDesignFamily,
  type MenuDesignFamilyId,
  type MenuOutputFormat,
  type MenuTreatment,
  type PrintPalette,
} from "../types";
import {
  ALL_MENU_DESIGN_FAMILIES,
  atelierFamily,
  counterFamily,
  defineFamily,
  fieldFamily,
  galleryFamily,
  getMenuDesignFamily,
  listMenuDesignFamilies,
  maisonFamily,
  nightHouseFamily,
  osteriaFamily,
  registerMenuDesignFamily,
  streetFamily,
} from ".";

const ALL_TREATMENTS = ["type-led", "balanced", "photo-led", "compact"];

const EXPECTED_FAMILIES = [
  {
    id: "atelier",
    nameKey: "print.family.atelier",
    descriptionKey: "print.familyDesc.atelier",
    supportedTreatments: ALL_TREATMENTS,
    supportedFormats: [
      "single-sheet",
      "two-page-spread",
      "folded-booklet",
      "drinks-card",
    ],
    typography: {
      display: "DM Serif Display",
      body: "DM Sans",
      displayScale: 1.45,
      bodyFloorPt: 10,
      align: "start",
    },
    paletteFallback: {
      ground: "#f4f2ed",
      ink: "#20211f",
      accent: "#1a6b6a",
      muted: "#696964",
    },
    compositions: ["editorial-asymmetric"],
    photography: {
      roles: ["full-width", "editorial-crop", "paired"],
      maxFeatureImagesPerPage: 2,
    },
    ornament: "rule",
  },
  {
    id: "maison",
    nameKey: "print.family.maison",
    descriptionKey: "print.familyDesc.maison",
    supportedTreatments: ALL_TREATMENTS,
    supportedFormats: [
      "single-sheet",
      "two-page-spread",
      "folded-booklet",
      "drinks-card",
    ],
    typography: {
      display: "EB Garamond",
      body: "EB Garamond",
      displayScale: 1.55,
      bodyFloorPt: 10,
      align: "center",
    },
    paletteFallback: {
      ground: "#f7f4ed",
      ink: "#272421",
      accent: "#756143",
      muted: "#65584d",
    },
    compositions: ["formal-course"],
    photography: {
      roles: ["cover", "half-page"],
      maxFeatureImagesPerPage: 1,
    },
    ornament: "double-rule",
  },
  {
    id: "osteria",
    nameKey: "print.family.osteria",
    descriptionKey: "print.familyDesc.osteria",
    supportedTreatments: ALL_TREATMENTS,
    supportedFormats: [
      "single-sheet",
      "two-page-spread",
      "folded-booklet",
      "takeaway-trifold",
    ],
    typography: {
      display: "EB Garamond",
      body: "DM Sans",
      displayScale: 1.35,
      bodyFloorPt: 10,
      align: "start",
    },
    paletteFallback: {
      ground: "#f2eee5",
      ink: "#29231e",
      accent: "#8b4134",
      muted: "#5f5548",
    },
    compositions: ["lively-columns"],
    photography: {
      roles: ["category-opener", "paired", "compact-tile"],
      maxFeatureImagesPerPage: 2,
    },
    ornament: "rule",
  },
  {
    id: "night-house",
    nameKey: "print.family.nightHouse",
    descriptionKey: "print.familyDesc.nightHouse",
    supportedTreatments: ALL_TREATMENTS,
    supportedFormats: [
      "single-sheet",
      "two-page-spread",
      "folded-booklet",
      "drinks-card",
    ],
    typography: {
      display: "Cormorant Garamond",
      body: "DM Sans",
      displayScale: 1.6,
      bodyFloorPt: 10,
      align: "center",
    },
    paletteFallback: {
      ground: "#171816",
      ink: "#ece7dc",
      accent: "#b89b64",
      muted: "#c7bda9",
    },
    compositions: ["cinematic-sections"],
    photography: {
      roles: ["cover", "full-width", "editorial-crop"],
      maxFeatureImagesPerPage: 2,
    },
    ornament: "frame",
  },
  {
    id: "counter",
    nameKey: "print.family.counter",
    descriptionKey: "print.familyDesc.counter",
    supportedTreatments: ALL_TREATMENTS,
    supportedFormats: [
      "single-sheet",
      "takeaway-trifold",
      "drinks-card",
      "counter-menu",
    ],
    typography: {
      display: "Oswald",
      body: "DM Sans",
      displayScale: 1.25,
      bodyFloorPt: 9.5,
      align: "start",
    },
    paletteFallback: {
      ground: "#f5f3ee",
      ink: "#22221f",
      accent: "#d2693f",
      muted: "#67594b",
    },
    compositions: ["modular-counter"],
    photography: {
      roles: ["category-opener", "compact-tile", "full-width"],
      maxFeatureImagesPerPage: 3,
    },
    ornament: "block",
  },
  {
    id: "street",
    nameKey: "print.family.street",
    descriptionKey: "print.familyDesc.street",
    supportedTreatments: ["balanced", "photo-led", "compact"],
    supportedFormats: ["single-sheet", "takeaway-trifold", "counter-menu"],
    typography: {
      display: "Oswald",
      body: "DM Sans",
      displayScale: 1.4,
      bodyFloorPt: 9.5,
      align: "start",
    },
    paletteFallback: {
      ground: "#f1f0eb",
      ink: "#20201f",
      accent: "#d04f38",
      muted: "#665047",
    },
    compositions: ["bold-scan"],
    photography: {
      roles: ["full-width", "paired", "compact-tile"],
      maxFeatureImagesPerPage: 3,
    },
    ornament: "block",
  },
  {
    id: "field",
    nameKey: "print.family.field",
    descriptionKey: "print.familyDesc.field",
    supportedTreatments: ALL_TREATMENTS,
    supportedFormats: [
      "single-sheet",
      "two-page-spread",
      "folded-booklet",
      "drinks-card",
    ],
    typography: {
      display: "DM Serif Display",
      body: "DM Sans",
      displayScale: 1.4,
      bodyFloorPt: 10,
      align: "start",
    },
    paletteFallback: {
      ground: "#f1f3ee",
      ink: "#243027",
      accent: "#55735a",
      muted: "#566459",
    },
    compositions: ["ingredient-air"],
    photography: {
      roles: ["half-page", "editorial-crop", "category-opener"],
      maxFeatureImagesPerPage: 2,
    },
    ornament: "rule",
  },
  {
    id: "gallery",
    nameKey: "print.family.gallery",
    descriptionKey: "print.familyDesc.gallery",
    supportedTreatments: ["balanced", "photo-led"],
    supportedFormats: ["single-sheet", "two-page-spread", "folded-booklet"],
    typography: {
      display: "Cormorant Garamond",
      body: "DM Sans",
      displayScale: 1.65,
      bodyFloorPt: 10,
      align: "start",
    },
    paletteFallback: {
      ground: "#eeeeeb",
      ink: "#1e1e1c",
      accent: "#404846",
      muted: "#5d5d5d",
    },
    compositions: ["image-gallery"],
    photography: {
      roles: ["cover", "full-width", "half-page", "editorial-crop"],
      maxFeatureImagesPerPage: 2,
    },
    ornament: "rule",
  },
] as const;

describe("menu design family collection", () => {
  it("registers the eight approved families in presentation order", () => {
    expect(listMenuDesignFamilies().map((family) => family.id)).toEqual(
      MENU_DESIGN_FAMILY_IDS,
    );
    expect(ALL_MENU_DESIGN_FAMILIES).toEqual(EXPECTED_FAMILIES);
  });

  it("exports each named family in the same approved order", () => {
    expect(ALL_MENU_DESIGN_FAMILIES).toEqual([
      atelierFamily,
      maisonFamily,
      osteriaFamily,
      nightHouseFamily,
      counterFamily,
      streetFamily,
      fieldFamily,
      galleryFamily,
    ]);
  });

  it("covers every treatment and output format across the collection", () => {
    const families = listMenuDesignFamilies();
    for (const treatment of MENU_TREATMENTS) {
      expect(
        families.some((family) =>
          family.supportedTreatments.includes(treatment),
        ),
      ).toBe(true);
    }
    for (const format of MENU_OUTPUT_FORMATS) {
      expect(
        families.some((family) => family.supportedFormats.includes(format)),
      ).toBe(true);
    }
  });

  it("gives every family a distinct structural signature", () => {
    const signatures = listMenuDesignFamilies().map((family) =>
      JSON.stringify([
        family.compositions,
        family.typography,
        family.photography.roles,
      ]),
    );
    expect(new Set(signatures).size).toBe(8);
  });

  it("uses only registered self-hosted print fonts", () => {
    for (const family of listMenuDesignFamilies()) {
      expect(PRINT_FONT_FAMILIES[family.typography.display]).toBeDefined();
      expect(PRINT_FONT_FAMILIES[family.typography.body]).toBeDefined();
      for (const faces of [
        PRINT_FONT_FAMILIES[family.typography.display],
        PRINT_FONT_FAMILIES[family.typography.body],
      ]) {
        expect(faces.length).toBeGreaterThan(0);
        for (const face of faces) {
          expect(face.path).toMatch(/^\/fonts\/.+\.woff2$/);
        }
      }
    }
  });

  it("deeply freezes every registered family and the public collection", () => {
    expect(Object.isFrozen(ALL_MENU_DESIGN_FAMILIES)).toBe(true);
    for (const family of listMenuDesignFamilies()) {
      expect(Object.isFrozen(family)).toBe(true);
      expect(Object.isFrozen(family.supportedTreatments)).toBe(true);
      expect(Object.isFrozen(family.supportedFormats)).toBe(true);
      expect(Object.isFrozen(family.typography)).toBe(true);
      expect(Object.isFrozen(family.paletteFallback)).toBe(true);
      expect(Object.isFrozen(family.compositions)).toBe(true);
      expect(Object.isFrozen(family.photography.roles)).toBe(true);
      expect(Object.isFrozen(family.photography)).toBe(true);
    }
  });

  it("keeps every resolved family palette text-safe", () => {
    for (const family of listMenuDesignFamilies()) {
      const resolved = resolvePrintPalette({ familyId: family.id });

      expect(resolved.source).toBe("family-fallback");
      expect(isPrintSafePalette(resolved)).toBe(true);
    }
  });

  it.each(["counter", "street"] as const)(
    "keeps %s decorative accent separate from its text-safe palette",
    (familyId) => {
      const family = getMenuDesignFamily(familyId);
      const resolved = resolvePrintPalette({ familyId });

      expect(
        contrastRatio(
          family.paletteFallback.accent,
          family.paletteFallback.ground,
        ),
      ).toBeLessThan(4.5);
      expect(isPrintSafePalette(resolved)).toBe(true);
      expect(resolved.accent).not.toBe(family.paletteFallback.accent);
      expect(resolved).not.toEqual(family.paletteFallback);
    },
  );
});

describe("menu design family registry", () => {
  it("rejects a family without a supported treatment", () => {
    expect(() =>
      defineFamily({
        ...EXPECTED_FAMILIES[0],
        supportedTreatments: [],
      } as unknown as MenuDesignFamily),
    ).toThrow("Menu family atelier must support a treatment and format");
  });

  it("rejects a family without a supported format", () => {
    expect(() =>
      defineFamily({
        ...EXPECTED_FAMILIES[0],
        supportedFormats: [],
      } as unknown as MenuDesignFamily),
    ).toThrow("Menu family atelier must support a treatment and format");
  });

  it("throws for an unknown family id", () => {
    expect(() => getMenuDesignFamily("missing" as MenuDesignFamilyId)).toThrow(
      "Unknown menu design family: missing",
    );
  });

  it("defensively clones caller-owned nested values", () => {
    const supportedTreatments: MenuTreatment[] = ["balanced"];
    const supportedFormats: MenuOutputFormat[] = ["single-sheet"];
    const typography: MenuDesignFamily["typography"] = {
      display: "DM Serif Display",
      body: "DM Sans",
      displayScale: 1.2,
      bodyFloorPt: 10,
      align: "start",
    };
    const paletteFallback: PrintPalette = {
      ground: "#ffffff",
      ink: "#000000",
      accent: "#111111",
      muted: "#222222",
    };
    const compositions: CompositionStrategyId[] = ["editorial-asymmetric"];
    const roles: ImageRole[] = ["cover"];
    const photography: MenuDesignFamily["photography"] = {
      roles,
      maxFeatureImagesPerPage: 1,
    };
    const registered = defineFamily({
      id: "atelier",
      nameKey: "print.family.aliasTest",
      descriptionKey: "print.familyDesc.aliasTest",
      supportedTreatments,
      supportedFormats,
      typography,
      paletteFallback,
      compositions,
      photography,
      ornament: "none",
    });

    expect(Object.isFrozen(supportedTreatments)).toBe(false);
    expect(Object.isFrozen(supportedFormats)).toBe(false);
    expect(Object.isFrozen(typography)).toBe(false);
    expect(Object.isFrozen(paletteFallback)).toBe(false);
    expect(Object.isFrozen(compositions)).toBe(false);
    expect(Object.isFrozen(roles)).toBe(false);
    expect(Object.isFrozen(photography)).toBe(false);

    supportedTreatments.push("compact");
    supportedFormats.push("drinks-card");
    typography.displayScale = 99;
    paletteFallback.ink = "#333333";
    compositions.push("image-gallery");
    roles.push("full-width");
    photography.maxFeatureImagesPerPage = 99;

    expect(registered.supportedTreatments).toEqual(["balanced"]);
    expect(registered.supportedFormats).toEqual(["single-sheet"]);
    expect(registered.typography.displayScale).toBe(1.2);
    expect(registered.paletteFallback.ink).toBe("#000000");
    expect(registered.compositions).toEqual(["editorial-asymmetric"]);
    expect(registered.photography).toEqual({
      roles: ["cover"],
      maxFeatureImagesPerPage: 1,
    });
  });

  it("rejects nested mutation of a registered family", () => {
    const registered = defineFamily({
      ...EXPECTED_FAMILIES[0],
      supportedTreatments: ["balanced"],
      typography: { ...EXPECTED_FAMILIES[0].typography },
    } as MenuDesignFamily);

    expect(() =>
      (registered.supportedTreatments as unknown as MenuTreatment[]).push(
        "compact",
      ),
    ).toThrow(TypeError);
    expect(() => {
      (
        registered.typography as unknown as { displayScale: number }
      ).displayScale = 99;
    }).toThrow(TypeError);
    expect(registered.supportedTreatments).toEqual(["balanced"]);
    expect(registered.typography.displayScale).toBe(1.45);
  });

  it("allows same-instance registration and returns a fresh ordered list", () => {
    const first = listMenuDesignFamilies();
    registerMenuDesignFamily(atelierFamily);
    registerMenuDesignFamily(maisonFamily);
    const second = listMenuDesignFamilies();

    expect(second).not.toBe(first);
    expect(second.map((family) => family.id)).toEqual(MENU_DESIGN_FAMILY_IDS);
    expect(second).toHaveLength(8);
  });

  it("rejects a conflicting registration without replacing the original", () => {
    const conflictingAtelier = defineFamily({
      ...atelierFamily,
      nameKey: "print.family.conflictingAtelier",
      supportedTreatments: [...atelierFamily.supportedTreatments],
      supportedFormats: [...atelierFamily.supportedFormats],
      typography: { ...atelierFamily.typography },
      paletteFallback: { ...atelierFamily.paletteFallback },
      compositions: [...atelierFamily.compositions],
      photography: {
        ...atelierFamily.photography,
        roles: [...atelierFamily.photography.roles],
      },
    });

    expect(() => registerMenuDesignFamily(conflictingAtelier)).toThrow(
      "Menu design family already registered: atelier",
    );
    expect(getMenuDesignFamily("atelier")).toBe(atelierFamily);
  });
});
