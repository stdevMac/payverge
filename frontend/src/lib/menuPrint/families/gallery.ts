/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const galleryFamily = defineFamily({
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
});
