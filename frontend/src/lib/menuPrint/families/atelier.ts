/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const atelierFamily = defineFamily({
  id: "atelier",
  nameKey: "print.family.atelier",
  descriptionKey: "print.familyDesc.atelier",
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
});
