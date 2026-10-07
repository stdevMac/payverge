/**
 * Root C / L3-31 — operator messages must not embed ICU plural/select blocks.
 *
 * getTranslation.applyParams only does `{var}` / `{{var}}` substitution; ICU
 * `{count, plural, ...}` passes through literally. Prefer one/other keys
 * resolved at the call site rather than a half-ICU runtime.
 */
import esMessages from "../messages/es";
import enMessages from "../messages/en";

const flatten = (
  o: unknown,
  prefix = "",
  out: Record<string, string> = {},
): Record<string, string> => {
  if (Array.isArray(o)) {
    o.forEach((v, i) => flatten(v, `${prefix}.${i}`, out));
  } else if (o && typeof o === "object") {
    for (const [k, v] of Object.entries(o as Record<string, unknown>)) {
      flatten(v, prefix ? `${prefix}.${k}` : k, out);
    }
  } else if (typeof o === "string") {
    out[prefix] = o;
  }
  return out;
};

const ICU_SHAPE = /,\s*plural\s*,|,\s*select\s*,/;

function icuLeaks(flat: Record<string, string>): string[] {
  return Object.entries(flat)
    .filter(([, v]) => ICU_SHAPE.test(v))
    .map(([k, v]) => `${k}: ${v}`);
}

describe("operator messages have no ICU plural/select shape (Root C / L3-31)", () => {
  test("en operator tree has no , plural, or , select, message values", () => {
    expect(icuLeaks(flatten(enMessages))).toEqual([]);
  });

  test("es operator tree has no , plural, or , select, message values", () => {
    expect(icuLeaks(flatten(esMessages))).toEqual([]);
  });
});
