import {
  MENU_DESIGN_FAMILY_IDS,
  MENU_OUTPUT_FORMATS,
  MENU_TREATMENTS,
  PAPER_DIMENSIONS_MM,
  type MenuArtDirection,
  type MenuDesignFamilyId,
  type MenuOutputFormat,
  type MenuTreatment,
  type PaperFormat,
  type PlannedMenuDocument,
  type ResolvedPrintPalette,
} from "./types";

const PALETTE_CONTRACT_IS_RESOLVED: MenuArtDirection["palette"] extends ResolvedPrintPalette
  ? true
  : false = true;

describe("PAPER_DIMENSIONS_MM", () => {
  const formats: PaperFormat[] = ["letter", "a4", "half-letter", "a5"];

  it("defines all four paper formats", () => {
    for (const f of formats) {
      expect(PAPER_DIMENSIONS_MM[f].widthMm).toBeGreaterThan(0);
      expect(PAPER_DIMENSIONS_MM[f].heightMm).toBeGreaterThan(0);
    }
  });

  it("matches ISO / ANSI portrait dimensions", () => {
    expect(PAPER_DIMENSIONS_MM.letter).toEqual({
      widthMm: 215.9,
      heightMm: 279.4,
    });
    expect(PAPER_DIMENSIONS_MM.a4).toEqual({ widthMm: 210, heightMm: 297 });
    expect(PAPER_DIMENSIONS_MM["half-letter"]).toEqual({
      widthMm: 139.7,
      heightMm: 215.9,
    });
    expect(PAPER_DIMENSIONS_MM.a5).toEqual({ widthMm: 148, heightMm: 210 });
  });
});

describe("menu art-direction and planned-document contracts", () => {
  it("requires art-direction palettes to come from the text-safe resolver", () => {
    expect(PALETTE_CONTRACT_IS_RESOLVED).toBe(true);
  });

  it("exports the approved design families, treatments, and output formats", () => {
    expect(MENU_DESIGN_FAMILY_IDS).toEqual([
      "atelier",
      "maison",
      "osteria",
      "night-house",
      "counter",
      "street",
      "field",
      "gallery",
    ]);
    expect(MENU_TREATMENTS).toEqual([
      "type-led",
      "balanced",
      "photo-led",
      "compact",
    ]);
    expect(MENU_OUTPUT_FORMATS).toEqual([
      "single-sheet",
      "two-page-spread",
      "folded-booklet",
      "takeaway-trifold",
      "drinks-card",
      "counter-menu",
    ]);
  });

  it("represents a coherent two-page A4 spread", () => {
    const direction: MenuArtDirection = {
      familyId: "atelier" satisfies MenuDesignFamilyId,
      treatment: "balanced" satisfies MenuTreatment,
      outputFormat: "two-page-spread" satisfies MenuOutputFormat,
      paperFormat: "a4",
      palette: {
        ground: "#f4f2ed",
        ink: "#20211f",
        accent: "#1a6b6a",
        muted: "#66645f",
        source: "family-fallback",
      },
      imageSelections: [],
      imageFocalPoints: {},
      logoTreatment: "contained",
      coverMode: "none",
      ornamentIntensity: "restrained",
      typographyPersonality: "editorial",
      contactPlacement: "footer",
    };
    const doc: PlannedMenuDocument = {
      direction,
      geometry: {
        paperFormat: "a4",
        outputFormat: "two-page-spread",
        orientation: "portrait",
        widthMm: 210,
        heightMm: 297,
        safeArea: { top: 16, right: 16, bottom: 16, left: 16 },
        trimGuideInsetMm: 0,
        foldsMm: [],
        minimumPages: 2,
        panelCount: 1,
      },
      pages: [
        { index: 0, blocks: [] },
        { index: 1, blocks: [] },
      ],
      diagnostics: [],
      readiness: "ready",
    };

    expect(doc.pages).toHaveLength(2);
  });
});
