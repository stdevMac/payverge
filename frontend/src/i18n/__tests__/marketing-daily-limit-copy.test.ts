import { getTranslation } from "../getTranslation";
import { sentenceCaseLeaf } from "../sentenceCaseLeaf";
import enMarketing from "../messages/en/marketingDashboard.json";
import esMarketing from "../messages/es/marketingDashboard.json";
import esArMarketing from "../messages/es-ar/marketingDashboard.json";

/**
 * Regression: Marketing PostEditorDrawer renders editor.dailyLimit.* keys when
 * the daily fair-use limit is hit. Missing production keys fall through to
 * sentenceCaseLeaf and render e.g. "Resets hour". Menu Builder already has a
 * production-file pin; Marketing must too — a missing marketing key previously
 * left the entire i18n suite green.
 *
 * Assert against sentenceCaseLeaf (not a hand-rolled leaf-name regex): camelCase
 * leaves become multi-word ("resetsHour" → "Resets hour"), so a single-token
 * regex cannot detect the fallback.
 */
describe("Marketing daily-limit copy (production message files)", () => {
  const PREFIX = "marketingDashboard.editor.dailyLimit";
  const LEAVES = [
    "title",
    "resets",
    "resetsHour",
    "resetsSoon",
    "contact",
  ] as const;

  it("ships the Marketing daily-limit copy in every operator locale", () => {
    for (const locale of ["en", "es", "es-AR"] as const) {
      for (const leaf of LEAVES) {
        const key = `${PREFIX}.${leaf}`;
        const v = getTranslation(key, locale);
        expect(typeof v).toBe("string");
        expect(v).toBeTruthy();
        // Missing key → sentenceCaseLeaf(key). A value equal to that fallback
        // means the real message file is missing the string.
        expect(v).not.toBe(sentenceCaseLeaf(key));
      }
    }
  });

  it("uses Rioplatense title/contact in es-AR while resets* inherit es", () => {
    expect(getTranslation(`${PREFIX}.title`, "es-AR")).toContain("Generaste");
    expect(getTranslation(`${PREFIX}.title`, "es")).toContain("Has generado");
    expect(getTranslation(`${PREFIX}.contact`, "es-AR")).toMatch(
      /Necesitás|Escribinos/,
    );
    // es-ar owns only title/contact; resets* must inherit es (override-layer).
    expect(getTranslation(`${PREFIX}.resets`, "es-AR")).toBe(
      getTranslation(`${PREFIX}.resets`, "es"),
    );
    expect(getTranslation(`${PREFIX}.resetsHour`, "es-AR")).toBe(
      getTranslation(`${PREFIX}.resetsHour`, "es"),
    );
    expect(getTranslation(`${PREFIX}.resetsSoon`, "es-AR")).toBe(
      getTranslation(`${PREFIX}.resetsSoon`, "es"),
    );
  });

  /**
   * getTranslation falls back to English, so a Spanish-only key loss resolves to
   * English copy and slips past every assertion above. Read the raw bundles so
   * losing `es` alone fails here instead of shipping English to Spanish
   * operators.
   */
  it("owns the copy in each locale's own message file", () => {
    const rawDailyLimit = (bundle: Record<string, unknown>) =>
      (bundle as { editor?: { dailyLimit?: Record<string, string> } }).editor
        ?.dailyLimit ?? {};

    for (const [locale, bundle] of [
      ["en", enMarketing],
      ["es", esMarketing],
    ] as const) {
      const block = rawDailyLimit(bundle);
      for (const leaf of LEAVES) {
        expect(`${locale}:${leaf}:${typeof block[leaf]}`).toBe(
          `${locale}:${leaf}:string`,
        );
        expect(block[leaf]).toBeTruthy();
      }
    }

    // es-ar is a deltas-only override layer: it owns the voseo strings and
    // inherits the rest. Owning resets* here would duplicate its es parent.
    const esAr = rawDailyLimit(esArMarketing);
    expect(Object.keys(esAr).sort()).toEqual(["contact", "title"]);
  });
});
