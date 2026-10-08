import {
  PAPER_DIMENSIONS_MM,
  type MenuOutputFormat,
  type PaperFormat,
  type PrintGeometry,
} from "./types";

const FORMAT_PROFILE: Record<
  MenuOutputFormat,
  {
    safeAreaMm: number;
    trimGuideInsetMm: number;
    minimumPages: number;
    panelCount: number;
    orientation: "portrait" | "landscape";
  }
> = {
  "single-sheet": {
    safeAreaMm: 16,
    trimGuideInsetMm: 0,
    minimumPages: 1,
    panelCount: 1,
    orientation: "portrait",
  },
  "two-page-spread": {
    safeAreaMm: 16,
    trimGuideInsetMm: 0,
    minimumPages: 2,
    panelCount: 1,
    orientation: "portrait",
  },
  "folded-booklet": {
    safeAreaMm: 14,
    trimGuideInsetMm: 3,
    minimumPages: 2,
    panelCount: 2,
    orientation: "landscape",
  },
  "takeaway-trifold": {
    safeAreaMm: 9,
    trimGuideInsetMm: 3,
    minimumPages: 2,
    panelCount: 3,
    orientation: "landscape",
  },
  "drinks-card": {
    safeAreaMm: 12,
    trimGuideInsetMm: 0,
    minimumPages: 1,
    panelCount: 1,
    orientation: "portrait",
  },
  "counter-menu": {
    safeAreaMm: 10,
    trimGuideInsetMm: 0,
    minimumPages: 1,
    panelCount: 1,
    orientation: "landscape",
  },
};

const roundMm = (value: number): number => Number(value.toFixed(2));

export function resolvePrintGeometry(
  outputFormat: MenuOutputFormat,
  paperFormat: PaperFormat,
): PrintGeometry {
  const paper = PAPER_DIMENSIONS_MM[paperFormat];
  const profile = FORMAT_PROFILE[outputFormat];
  const widthMm =
    profile.orientation === "landscape" ? paper.heightMm : paper.widthMm;
  const heightMm =
    profile.orientation === "landscape" ? paper.widthMm : paper.heightMm;
  const foldsMm =
    profile.panelCount === 3
      ? [roundMm(widthMm / 3), roundMm((widthMm * 2) / 3)]
      : profile.panelCount === 2
        ? [roundMm(widthMm / 2)]
        : [];

  return {
    paperFormat,
    outputFormat,
    orientation: profile.orientation,
    widthMm,
    heightMm,
    safeArea: {
      top: profile.safeAreaMm,
      right: profile.safeAreaMm,
      bottom: profile.safeAreaMm,
      left: profile.safeAreaMm,
    },
    trimGuideInsetMm: profile.trimGuideInsetMm,
    foldsMm,
    minimumPages: profile.minimumPages,
    panelCount: profile.panelCount,
  };
}
