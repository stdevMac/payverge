import {
  MIN_FEATURE_PRINT_PX,
  MIN_TILE_PRINT_PX,
  type MenuPrintAudit,
} from "./audit";
import type {
  FeaturedImageSelection,
  ImageRole,
  MenuTreatment,
  PrintMenuModel,
  RegisteredMenuDesignFamily,
} from "./types";

/**
 * Default photography leads with roles that sit beside the copy — a side crop
 * composes the page, while a full-width band letterboxes the photo into a
 * smear and pushes every dish down. Banner roles stay available as manual
 * picks; they just stop being the out-of-the-box look.
 */
const DEFAULT_ROLE_PREFERENCE: readonly ImageRole[] = [
  "editorial-crop",
  "paired",
  "category-opener",
  "half-page",
  "compact-tile",
  "full-width",
];

export interface SelectDefaultPrintImagesInput {
  family: RegisteredMenuDesignFamily;
  model: PrintMenuModel;
  audit: MenuPrintAudit;
  treatment: MenuTreatment;
}

/** Chooses a section-diverse, print-safe starting photography set. */
export function selectDefaultPrintImages({
  family,
  model,
  audit,
  treatment,
}: SelectDefaultPrintImagesInput): FeaturedImageSelection[] {
  if (treatment === "type-led" || treatment === "compact") return [];

  const failedUrls = new Set(audit.failedUrls);
  const lowResolutionUrls = new Set(audit.lowResolutionUrls);
  const roles = family.photography.roles
    .filter((role) => role !== "cover")
    .slice()
    .sort(
      (first, second) =>
        DEFAULT_ROLE_PREFERENCE.indexOf(first) -
        DEFAULT_ROLE_PREFERENCE.indexOf(second),
    );
  if (!roles.length) return [];
  const selectionLimit =
    treatment === "photo-led" ? family.photography.maxFeatureImagesPerPage : 1;
  const bySection = model.sections.map((section) =>
    section.items.flatMap((item) => {
      const url = item.imageCandidates?.find((candidate) => {
        if (failedUrls.has(candidate) || lowResolutionUrls.has(candidate)) {
          return false;
        }
        const measurement = audit.measurements?.[candidate];
        if (!measurement) return false;
        if (
          measurement.status !== "ready" ||
          !measurement.width ||
          !measurement.height
        ) {
          return false;
        }
        const minimum =
          roles[0] === "compact-tile"
            ? MIN_TILE_PRINT_PX
            : MIN_FEATURE_PRINT_PX;
        return Math.min(measurement.width, measurement.height) >= minimum;
      });
      return url ? [{ itemId: item.id, url }] : [];
    }),
  );
  const candidates = [
    ...bySection.flatMap((usable) => (usable[0] ? [usable[0]] : [])),
    ...bySection.flatMap((usable) => usable.slice(1)),
  ];
  const seenItems = new Set<string>();
  return candidates
    .filter(({ itemId }) => {
      if (seenItems.has(itemId)) return false;
      seenItems.add(itemId);
      return true;
    })
    .slice(0, selectionLimit)
    .map((candidate, index) => ({
      ...candidate,
      role: roles[index % roles.length],
    }));
}
