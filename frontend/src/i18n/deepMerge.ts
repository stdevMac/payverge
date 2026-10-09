// Deep-merge translation override trees onto a base locale tree.
//
// Used to layer the Argentine (es-AR) override files over the neutral `es`
// base so es-AR only stores the strings that actually differ (voseo verbs,
// Rioplatense vocabulary). Any key the override omits inherits the es value —
// this is what keeps es-AR DRY and structurally free of "missing key" gaps.
//
// Semantics: plain objects merge recursively; everything else (strings, arrays,
// numbers, null) is replaced wholesale by the override. Arrays are NOT merged
// by index — a translated list replaces the base list entirely. The base is
// never mutated.

type Tree = Record<string, unknown>;

function isPlainObject(value: unknown): value is Tree {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value)
  );
}

export function deepMerge<T extends Tree>(base: T, override: Tree): T {
  const result: Tree = { ...base };

  for (const [key, overrideValue] of Object.entries(override)) {
    const baseValue = result[key];
    if (isPlainObject(baseValue) && isPlainObject(overrideValue)) {
      result[key] = deepMerge(baseValue, overrideValue);
    } else {
      result[key] = overrideValue;
    }
  }

  return result as T;
}
