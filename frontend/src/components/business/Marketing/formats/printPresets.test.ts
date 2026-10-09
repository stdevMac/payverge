import { FORMATS } from "./formats";
import {
  FORBIDDEN_PRINT_PRESET_KEYS,
  PRINT_PRESET_ORDER,
  PRINT_PRESETS,
  assertPrintPresetValid,
  formatForPrintPreset,
  isPrintPresetId,
  printPresetFor,
  printPresetForDestination,
  printPresetRasterWidth,
} from "./printPresets";

describe("print presets registry", () => {
  it("keeps order in sync with the map", () => {
    expect(new Set(PRINT_PRESET_ORDER)).toEqual(
      new Set(Object.keys(PRINT_PRESETS)),
    );
  });

  it("only references known FormatIds", () => {
    PRINT_PRESET_ORDER.forEach((id) => {
      const preset = PRINT_PRESETS[id];
      expect(FORMATS[preset.formatId]).toBeDefined();
      expect(assertPrintPresetValid(preset)).toBeNull();
    });
  });

  it("never carries schedule field keys on the preset object", () => {
    PRINT_PRESET_ORDER.forEach((id) => {
      const keys = Object.keys(PRINT_PRESETS[id]);
      FORBIDDEN_PRINT_PRESET_KEYS.forEach((forbidden) => {
        expect(keys).not.toContain(forbidden);
      });
    });
  });

  it("table tent is print HTML at 5:7 with print specs", () => {
    const preset = PRINT_PRESETS.table_tent_5x7;
    expect(preset.exportKind).toBe("print_html");
    expect(preset.formatId).toBe("5:7");
    expect(formatForPrintPreset(preset).print?.dpi).toBe(300);
    expect(printPresetRasterWidth(preset)).toBeUndefined();
  });

  it("window cling is 2× square PNG", () => {
    const preset = PRINT_PRESETS.window_clings_square;
    expect(preset.exportKind).toBe("high_dpi_png");
    expect(preset.formatId).toBe("1:1");
    expect(printPresetRasterWidth(preset)).toBe(2160);
  });

  it("flyer is 2× wide PNG", () => {
    const preset = PRINT_PRESETS.flyer_half_letter;
    expect(printPresetRasterWidth(preset)).toBe(3840);
  });

  it("maps destinations to presets", () => {
    expect(printPresetForDestination("print_tent")?.id).toBe("table_tent_5x7");
    expect(printPresetForDestination("print_window")?.id).toBe(
      "window_clings_square",
    );
    expect(printPresetForDestination("ig_feed")).toBeNull();
  });

  it("isPrintPresetId / printPresetFor guard unknowns", () => {
    expect(isPrintPresetId("table_tent_5x7")).toBe(true);
    expect(isPrintPresetId("billboard")).toBe(false);
    expect(printPresetFor("billboard")).toBeNull();
    expect(printPresetFor("flyer_half_letter")?.id).toBe("flyer_half_letter");
  });

  it("instructions never mention scheduling", () => {
    PRINT_PRESET_ORDER.forEach((id) => {
      const text = PRINT_PRESETS[id].defaultInstructions.toLowerCase();
      expect(text).not.toMatch(/schedul/);
      expect(text).not.toMatch(/calendar/);
      expect(text).not.toMatch(/queue/);
    });
  });
});
