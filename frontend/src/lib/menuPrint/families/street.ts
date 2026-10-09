/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const streetFamily = defineFamily({
  id: "street",
  nameKey: "print.family.street",
  descriptionKey: "print.familyDesc.street",
  supportedTreatments: ["balanced", "photo-led", "compact"],
  supportedFormats: ["single-sheet", "takeaway-trifold", "counter-menu"],
  typography: {
    display: "Oswald",
    body: "DM Sans",
    displayScale: 1.4,
    bodyFloorPt: 9.5,
    align: "start",
  },
  paletteFallback: {
    ground: "#f1f0eb",
    ink: "#20201f",
    accent: "#d04f38",
    muted: "#665047",
  },
  compositions: ["bold-scan"],
  photography: {
    roles: ["full-width", "paired", "compact-tile"],
    maxFeatureImagesPerPage: 3,
  },
  ornament: "block",
});
