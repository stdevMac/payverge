import type { MenuPrintAudit } from "./audit";
import { getMenuDesignFamily } from "./families";
import type {
  MenuDesignFamilyId,
  MenuDirectionRecommendation,
  MenuOutputFormat,
  MenuTreatment,
} from "./types";

export interface RecommendMenuDirectionsInput {
  audit: MenuPrintAudit;
  businessType?: string | null;
}

interface DirectionCandidate {
  familyId: MenuDesignFamilyId;
  outputFormat: MenuOutputFormat;
  reasonKey: string;
  score: number;
}

const SPACIOUS_FORMAT_BY_FAMILY: Partial<
  Record<MenuDesignFamilyId, MenuOutputFormat>
> = {
  atelier: "two-page-spread",
  counter: "takeaway-trifold",
  field: "folded-booklet",
  gallery: "two-page-spread",
  maison: "folded-booklet",
  "night-house": "folded-booklet",
  osteria: "folded-booklet",
  street: "takeaway-trifold",
};

/**
 * The roomiest non-spread format a family hands off to when a compact
 * format runs out of page. Exposed so surfaces that assemble directions
 * outside recommendMenuDirections (the look picker's non-recommended family
 * cards) can recover from a blocked plan with the same escalation path.
 */
export function spaciousPrintFormat(
  familyId: MenuDesignFamilyId,
): MenuOutputFormat | undefined {
  return SPACIOUS_FORMAT_BY_FAMILY[familyId];
}

const DENSE_FAMILY_FALLBACKS: readonly MenuDesignFamilyId[] = [
  "atelier",
  "field",
  "osteria",
  "maison",
  "night-house",
  "gallery",
];

const GENERAL_CANDIDATES: readonly DirectionCandidate[] = [
  {
    familyId: "atelier",
    outputFormat: "single-sheet",
    reasonKey: "print.recommendation.versatileRestaurant",
    score: 100,
  },
  {
    familyId: "gallery",
    outputFormat: "two-page-spread",
    reasonKey: "print.recommendation.editorialAlternative",
    score: 80,
  },
  {
    familyId: "osteria",
    outputFormat: "folded-booklet",
    reasonKey: "print.recommendation.warmAlternative",
    score: 80,
  },
];

const CANDIDATES_BY_BUSINESS_TYPE: Readonly<
  Record<string, readonly DirectionCandidate[]>
> = {
  cafe: [
    {
      familyId: "counter",
      outputFormat: "single-sheet",
      reasonKey: "print.recommendation.cafeCounter",
      score: 100,
    },
    {
      familyId: "atelier",
      outputFormat: "single-sheet",
      reasonKey: "print.recommendation.cleanAlternative",
      score: 80,
    },
    {
      familyId: "gallery",
      outputFormat: "folded-booklet",
      reasonKey: "print.recommendation.editorialAlternative",
      score: 80,
    },
  ],
  fine_dining: [
    {
      familyId: "maison",
      outputFormat: "two-page-spread",
      reasonKey: "print.recommendation.fineDiningMaison",
      score: 100,
    },
    {
      familyId: "atelier",
      outputFormat: "two-page-spread",
      reasonKey: "print.recommendation.cleanAlternative",
      score: 80,
    },
    {
      familyId: "osteria",
      outputFormat: "folded-booklet",
      reasonKey: "print.recommendation.warmAlternative",
      score: 80,
    },
  ],
  bar: [
    {
      familyId: "night-house",
      outputFormat: "drinks-card",
      reasonKey: "print.recommendation.barNightHouse",
      score: 100,
    },
    {
      familyId: "counter",
      outputFormat: "drinks-card",
      reasonKey: "print.recommendation.casualAlternative",
      score: 80,
    },
    {
      familyId: "maison",
      outputFormat: "drinks-card",
      reasonKey: "print.recommendation.editorialAlternative",
      score: 80,
    },
  ],
  quick_service: [
    {
      familyId: "street",
      outputFormat: "counter-menu",
      reasonKey: "print.recommendation.quickServiceStreet",
      score: 100,
    },
    {
      familyId: "counter",
      outputFormat: "counter-menu",
      reasonKey: "print.recommendation.counterAlternative",
      score: 80,
    },
    {
      familyId: "osteria",
      outputFormat: "takeaway-trifold",
      reasonKey: "print.recommendation.warmAlternative",
      score: 80,
    },
  ],
  bakery: [
    {
      familyId: "counter",
      outputFormat: "single-sheet",
      reasonKey: "print.recommendation.bakeryCounter",
      score: 100,
    },
    ...GENERAL_CANDIDATES.slice(1),
  ],
  food_truck: [
    {
      familyId: "street",
      outputFormat: "takeaway-trifold",
      reasonKey: "print.recommendation.foodTruckStreet",
      score: 100,
    },
    {
      familyId: "atelier",
      outputFormat: "single-sheet",
      reasonKey: "print.recommendation.cleanAlternative",
      score: 80,
    },
    {
      familyId: "gallery",
      outputFormat: "single-sheet",
      reasonKey: "print.recommendation.editorialAlternative",
      score: 80,
    },
  ],
  restaurant: GENERAL_CANDIDATES,
  other: GENERAL_CANDIDATES,
};

function treatmentForAudit(audit: MenuPrintAudit): MenuTreatment {
  const dense = audit.itemCount > 36 || audit.averageDescriptionLength > 110;
  const photoRich = audit.photoCoverage >= 0.6;
  return dense
    ? "compact"
    : photoRich
      ? "photo-led"
      : audit.photoCoverage >= 0.2
        ? "balanced"
        : "type-led";
}

function formatForAudit(
  candidate: DirectionCandidate,
  audit: MenuPrintAudit,
  treatment: MenuTreatment,
): MenuOutputFormat {
  const spacious = SPACIOUS_FORMAT_BY_FAMILY[candidate.familyId];
  if (!spacious) return candidate.outputFormat;
  const items = audit.itemCount;
  const copy = audit.averageDescriptionLength;
  // A letter/A4 sheet comfortably seats a dozen-plus dishes even with a
  // feature photo, so a small menu stays a single confident page instead of
  // spilling onto a half-empty companion sheet. The compact card formats top
  // out far earlier (a drinks card blocks near 13 plain items, a counter card
  // near 11, both around 8 once descriptions appear), so they hand off to the
  // family's roomier folded format well before the planner would block.
  const needsMoreRoom =
    candidate.outputFormat === "single-sheet"
      ? items >= 18 ||
        (items >= 13 && copy > 90) ||
        (treatment === "photo-led" && items >= 12)
      : candidate.outputFormat === "drinks-card"
        ? items >= 11 ||
          (items >= 8 && copy > 50) ||
          (items >= 7 && copy > 90) ||
          (treatment === "photo-led" && items >= 8)
        : candidate.outputFormat === "counter-menu"
          ? items >= 10 ||
            (items >= 7 && copy > 50) ||
            (treatment === "photo-led" && items >= 7)
          : false;
  return needsMoreRoom ? spacious : candidate.outputFormat;
}

function candidateForDenseMenu(
  candidate: DirectionCandidate,
  usedFamilies: ReadonlySet<MenuDesignFamilyId>,
): DirectionCandidate {
  const currentFamily = getMenuDesignFamily(candidate.familyId);
  const familyId =
    currentFamily.supportedFormats.includes("two-page-spread") &&
    !usedFamilies.has(candidate.familyId)
      ? candidate.familyId
      : DENSE_FAMILY_FALLBACKS.find(
          (fallbackId) => !usedFamilies.has(fallbackId),
        );

  if (!familyId) return candidate;
  return {
    ...candidate,
    familyId,
    outputFormat: "two-page-spread",
    reasonKey:
      familyId === candidate.familyId
        ? candidate.reasonKey
        : "print.recommendation.balanced",
  };
}

export function recommendMenuDirections(
  input: RecommendMenuDirectionsInput,
): MenuDirectionRecommendation[] {
  const businessType = input.businessType?.trim().toLowerCase() ?? "";
  const candidates = Object.prototype.hasOwnProperty.call(
    CANDIDATES_BY_BUSINESS_TYPE,
    businessType,
  )
    ? CANDIDATES_BY_BUSINESS_TYPE[businessType]
    : GENERAL_CANDIDATES;
  const treatment = treatmentForAudit(input.audit);
  // Only genuinely large menus get pushed onto a spread; formatForAudit
  // already grants mid-size menus a roomier format within their own family.
  const spreadVolume =
    input.audit.itemCount >= 26 ||
    (input.audit.itemCount >= 20 &&
      input.audit.averageDescriptionLength >= 90);
  const usedFamilies = new Set<MenuDesignFamilyId>();

  return candidates
    .map((candidate) => {
      const compatibleCandidate = spreadVolume
        ? candidateForDenseMenu(candidate, usedFamilies)
        : candidate;
      usedFamilies.add(compatibleCandidate.familyId);
      const family = getMenuDesignFamily(compatibleCandidate.familyId);
      const compatibleTreatment = family.supportedTreatments.includes(treatment)
        ? treatment
        : "balanced";
      return {
        ...compatibleCandidate,
        treatment: compatibleTreatment,
        outputFormat: formatForAudit(
          compatibleCandidate,
          input.audit,
          compatibleTreatment,
        ),
      };
    })
    .sort(
      (first, second) =>
        second.score - first.score ||
        first.familyId.localeCompare(second.familyId),
    );
}
