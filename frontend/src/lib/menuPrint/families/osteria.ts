/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const osteriaFamily = defineFamily({
  id: "osteria",
  nameKey: "print.family.osteria",
  descriptionKey: "print.familyDesc.osteria",
  supportedTreatments: ["type-led", "balanced", "photo-led", "compact"],
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
});
