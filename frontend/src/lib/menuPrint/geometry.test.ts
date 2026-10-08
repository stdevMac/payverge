import { resolvePrintGeometry } from "./geometry";
import {
  MENU_OUTPUT_FORMATS,
  PAPER_DIMENSIONS_MM,
  type MenuOutputFormat,
  type PaperFormat,
} from "./types";

it.each([
  ["single-sheet", "a4", 1, []],
  ["two-page-spread", "a4", 2, []],
  ["folded-booklet", "a4", 2, [148.5]],
  ["takeaway-trifold", "letter", 2, [93.13, 186.27]],
  ["drinks-card", "a5", 1, []],
  ["counter-menu", "letter", 1, []],
])(
  "resolves %s geometry",
  (outputFormat, paperFormat, minimumPages, foldsMm) => {
    const geometry = resolvePrintGeometry(
      outputFormat as MenuOutputFormat,
      paperFormat as PaperFormat,
    );

    expect(geometry.minimumPages).toBe(minimumPages);
    expect(geometry.foldsMm).toEqual(foldsMm);
    expect(geometry.safeArea.left).toBeGreaterThan(0);
    expect(geometry.orientation).toBe(
      ["folded-booklet", "takeaway-trifold", "counter-menu"].includes(
        outputFormat,
      )
        ? "landscape"
        : "portrait",
    );
  },
);

describe("resolvePrintGeometry", () => {
  const paperFormats: PaperFormat[] = ["letter", "a4", "half-letter", "a5"];
  const profiles: Record<
    MenuOutputFormat,
    {
      safeAreaMm: number;
      trimGuideInsetMm: number;
      minimumPages: number;
      panelCount: number;
    }
  > = {
    "single-sheet": {
      safeAreaMm: 16,
      trimGuideInsetMm: 0,
      minimumPages: 1,
      panelCount: 1,
    },
    "two-page-spread": {
      safeAreaMm: 16,
      trimGuideInsetMm: 0,
      minimumPages: 2,
      panelCount: 1,
    },
    "folded-booklet": {
      safeAreaMm: 14,
      trimGuideInsetMm: 3,
      minimumPages: 2,
      panelCount: 2,
    },
    "takeaway-trifold": {
      safeAreaMm: 9,
      trimGuideInsetMm: 3,
      minimumPages: 2,
      panelCount: 3,
    },
    "drinks-card": {
      safeAreaMm: 12,
      trimGuideInsetMm: 0,
      minimumPages: 1,
      panelCount: 1,
    },
    "counter-menu": {
      safeAreaMm: 10,
      trimGuideInsetMm: 0,
      minimumPages: 1,
      panelCount: 1,
    },
  };

  it.each(
    MENU_OUTPUT_FORMATS.flatMap((outputFormat) =>
      paperFormats.map((paperFormat) => [outputFormat, paperFormat] as const),
    ),
  )("preserves the %s profile on %s paper", (outputFormat, paperFormat) => {
    const geometry = resolvePrintGeometry(outputFormat, paperFormat);
    const paper = PAPER_DIMENSIONS_MM[paperFormat];
    const profile = profiles[outputFormat];

    const landscape = [
      "folded-booklet",
      "takeaway-trifold",
      "counter-menu",
    ].includes(outputFormat);
    expect(geometry).toMatchObject({
      paperFormat,
      outputFormat,
      orientation: landscape ? "landscape" : "portrait",
      widthMm: landscape ? paper.heightMm : paper.widthMm,
      heightMm: landscape ? paper.widthMm : paper.heightMm,
      safeArea: {
        top: profile.safeAreaMm,
        right: profile.safeAreaMm,
        bottom: profile.safeAreaMm,
        left: profile.safeAreaMm,
      },
      trimGuideInsetMm: profile.trimGuideInsetMm,
      minimumPages: profile.minimumPages,
      panelCount: profile.panelCount,
    });
    expect(geometry.foldsMm).toHaveLength(profile.panelCount - 1);
    expect(
      geometry.foldsMm.every((fold) => fold > 0 && fold < geometry.widthMm),
    ).toBe(true);
    expect(
      geometry.foldsMm.every(
        (fold, index) => index === 0 || fold > geometry.foldsMm[index - 1],
      ),
    ).toBe(true);
  });

  it.each([
    ["letter", [93.13, 186.27]],
    ["a4", [99, 198]],
    ["half-letter", [71.97, 143.93]],
    ["a5", [70, 140]],
  ] as const)(
    "rounds trifold positions for %s paper",
    (paperFormat, foldsMm) => {
      expect(
        resolvePrintGeometry("takeaway-trifold", paperFormat).foldsMm,
      ).toEqual(foldsMm);
    },
  );

  it("returns deterministic results without sharing mutable geometry", () => {
    const first = resolvePrintGeometry("takeaway-trifold", "a4");
    const second = resolvePrintGeometry("takeaway-trifold", "a4");

    expect(second).toEqual(first);
    expect(second).not.toBe(first);
    expect(second.safeArea).not.toBe(first.safeArea);
    expect(second.foldsMm).not.toBe(first.foldsMm);
  });
});
