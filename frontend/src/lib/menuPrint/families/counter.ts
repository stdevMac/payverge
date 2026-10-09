/* eslint-disable no-restricted-syntax -- print palette data for generated documents, not Tailwind UI */

import { defineFamily } from "./registry";

export const counterFamily = defineFamily({
  id: "counter",
  nameKey: "print.family.counter",
  descriptionKey: "print.familyDesc.counter",
  supportedTreatments: ["type-led", "balanced", "photo-led", "compact"],
  supportedFormats: [
    "single-sheet",
    "takeaway-trifold",
    "drinks-card",
    "counter-menu",
  ],
  typography: {
    display: "Oswald",
    body: "DM Sans",
    displayScale: 1.25,
    bodyFloorPt: 9.5,
    align: "start",
  },
  paletteFallback: {
    ground: "#f5f3ee",
    ink: "#22221f",
    accent: "#d2693f",
    muted: "#67594b",
  },
  compositions: ["modular-counter"],
  photography: {
    roles: ["category-opener", "compact-tile", "full-width"],
    maxFeatureImagesPerPage: 3,
  },
  ornament: "block",
});
