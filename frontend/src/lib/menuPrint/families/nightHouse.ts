/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const nightHouseFamily = defineFamily({
  id: "night-house",
  nameKey: "print.family.nightHouse",
  descriptionKey: "print.familyDesc.nightHouse",
  supportedTreatments: ["type-led", "balanced", "photo-led", "compact"],
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
});
