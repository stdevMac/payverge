/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const maisonFamily = defineFamily({
  id: "maison",
  nameKey: "print.family.maison",
  descriptionKey: "print.familyDesc.maison",
  supportedTreatments: ["type-led", "balanced", "photo-led", "compact"],
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
});
