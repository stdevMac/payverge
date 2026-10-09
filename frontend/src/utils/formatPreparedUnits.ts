/**
 * L8-1: pluralize prepared-unit labels for operator bill/table surfaces.
 * English: 1 → "unit", else "units". Spanish: 1 → "unidad", else "unidades".
 *
 * Callers pass a tString that resolves keys under their local namespace
 * (billManager.preparedUnitsOne / display.preparedUnitsOne / tables… etc.).
 * Keys fall back: if *One/*Other missing, the legacy preparedUnits string is
 * used so partially-updated locales still render something.
 */
export type PreparedUnitsTranslator = (key: string) => string;

export function formatPreparedUnits(
  count: number,
  tString: PreparedUnitsTranslator,
  /**
   * Key prefix without the One/Other/legacy suffix.
   * e.g. "preparedUnits" or "display.preparedUnits"
   */
  baseKey = "preparedUnits",
): string {
  const n = Number.isFinite(count) ? count : 0;
  const pluralKey = n === 1 ? `${baseKey}One` : `${baseKey}Other`;
  const template =
    tString(pluralKey) !== pluralKey
      ? tString(pluralKey)
      : tString(baseKey) !== baseKey
        ? tString(baseKey)
        : n === 1
          ? "{count} item"
          : "{count} items";
  return template.replace(/\{count\}/g, String(n));
}
