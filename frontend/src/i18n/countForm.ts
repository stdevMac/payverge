/**
 * Operator copy has no ICU plural engine. Callers pick `_one` vs `_other`
 * at the use site: singular only when count === 1.
 */
function countForm(count: number): "one" | "other" {
  return count === 1 ? "one" : "other";
}

export function countKey(base: string, count: number): string {
  return `${base}_${countForm(count)}`;
}
