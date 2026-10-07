const OPTIONAL_MARK =
  /(?:\(|（)\s*(?:optional|opcional|opzionale|opcjonalnie|facultatif|optioneel|valfritt|valgfritt|valgfrit|tùy chọn|необязательно|任意|可选|선택|اختياري|वैकल्पिक|isteğe bağlı|ไม่บังคับ)\s*(?:\)|）)/iu;

/**
 * Append " (optional)" to a field label unless the label already carries
 * that marker. Guest reservation catalogs used to bake "(opcional)" into
 * specialRequests while the form also appended reservationForm.optional.
 */
export function appendOptionalSuffix(label: string, optional: string): string {
  const base = label.trim();
  const suffix = optional.trim();
  if (!base) return suffix ? `(${suffix})` : "";
  if (!suffix) return base;
  if (alreadyHasOptional(base, suffix)) return base;
  return `${base} (${suffix})`;
}

function alreadyHasOptional(label: string, optional: string): boolean {
  const n = label.normalize("NFKC").toLowerCase();
  const o = optional.normalize("NFKC").toLowerCase();
  if (o && (n.includes(`(${o})`) || n.includes(`（${o}）`))) return true;
  return OPTIONAL_MARK.test(label);
}
