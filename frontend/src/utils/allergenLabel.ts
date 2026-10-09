import { ALLERGENS } from "@/constants/menu-tags";
import { sentenceCaseLeaf } from "@/i18n/sentenceCaseLeaf";
import { reportMissingTranslation } from "@/i18n/missingTranslationReporter";

const OPERATOR_ALLERGEN_NAME_PREFIX =
  "businessDashboard.dashboard.menuBuilder.items";

const FALLBACK_LABEL: Record<string, string> = {
  celery: "Celery",
  crustaceans: "Crustaceans",
  dairy: "Dairy",
  eggs: "Eggs",
  fish: "Fish",
  gluten: "Gluten",
  lupin: "Lupin",
  mollusc: "Mollusc",
  mustard: "Mustard",
  peanut: "Peanut",
  sesame: "Sesame",
  so2: "Sulphur Dioxide",
  soya: "Soya",
  treenuts: "Tree Nuts",
};

function compact(value: string): string {
  return value.replace(/[\s._-]/g, "").toLowerCase();
}

/**
 * Turn a token that is NOT in the operator allergen catalog into a safe,
 * human-readable label instead of ever rendering the raw enum/extraction
 * value to an operator (#574/#584 acceptance criterion). Also reports the
 * gap once per (locale, token) session via the existing missing-translation
 * telemetry so unrecognized allergens surface for catalog follow-up.
 */
function humanizeUnknownAllergen(token: string, locale?: string): string {
  const id = token.trim();
  reportMissingTranslation({
    locale: locale ?? "unknown",
    key: `${OPERATOR_ALLERGEN_NAME_PREFIX}.allergenNames.${id.toLowerCase()}`,
    fallbackUsed: "leaf",
  });
  return sentenceCaseLeaf(id.toLowerCase()) || id;
}

export function operatorAllergenNameKey(token: string): string | null {
  const id = token.trim().toLowerCase();
  if (!ALLERGENS.some((allergen) => allergen.id === id)) return null;
  return `${OPERATOR_ALLERGEN_NAME_PREFIX}.allergenNames.${id}`;
}

/**
 * Map an extracted allergen token to a restaurant-ready operator label.
 * `locale` is optional and only used to tag telemetry when a future/unknown
 * token falls back to a humanized label — it never changes which catalog
 * entries resolve.
 */
export function allergenDisplayName(
  token: string,
  translate: (key: string) => string,
  locale?: string,
): string {
  const id = token.trim().toLowerCase();
  const key = operatorAllergenNameKey(token);
  if (!key) return humanizeUnknownAllergen(token, locale);

  const label = translate(key).trim();
  if (label && label !== key && compact(label) !== compact(id)) {
    return label;
  }
  return FALLBACK_LABEL[id] ?? humanizeUnknownAllergen(token, locale);
}
