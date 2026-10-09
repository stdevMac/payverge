// Sentence-case the leaf segment of a dotted i18n key as a last-resort
// fallback. `error.notFoundEyebrow` -> "Not found"; `cart.empty` -> "Empty".
// Splits on camelCase and capitalises the first word only — anything more
// elaborate (Title Case, locale-specific casing) belongs in real
// translations, not in a fallback. Keep it boring and predictable.
//
// This lives in a server-safe module (no "use client") so the operator-tier
// getTranslation lookup can run during server `generateMetadata()`.
export function sentenceCaseLeaf(key: string): string {
  const leaf = key.split(".").pop() || key;
  // Split snake_case/kebab-case and camelCase / PascalCase into
  // space-separated words ("payment_refund_review" -> "Payment refund review").
  const words = leaf
    .replace(/[_-]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean);
  if (words.length === 0) return leaf;
  words[0] = words[0].charAt(0).toUpperCase() + words[0].slice(1);
  return words.join(" ");
}
