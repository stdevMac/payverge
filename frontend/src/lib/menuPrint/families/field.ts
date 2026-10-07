/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const fieldFamily = defineFamily({
  id: "field",
  nameKey: "print.family.field",
  descriptionKey: "print.familyDesc.field",
  supportedTreatments: ["type-led", "balanced", "photo-led", "compact"],
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
});
