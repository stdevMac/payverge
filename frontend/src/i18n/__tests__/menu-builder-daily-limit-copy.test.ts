import { getTranslation } from "../getTranslation";
import { sentenceCaseLeaf } from "../sentenceCaseLeaf";
import enDashboard from "../messages/en/businessDashboard.json";
import esDashboard from "../messages/es/businessDashboard.json";
import esArDashboard from "../messages/es-ar/businessDashboard.json";

/**
 * Regression: Task 8 wired AIImageTools to dailyLimit.* keys but only added
 * them to component-test mocks. Missing production keys fall through to
 * sentenceCaseLeaf and render "Title Resets Contact". This suite asserts the
 * real message files ship the copy the component reads.
 */
describe("Menu Builder daily-limit copy (production message files)", () => {
  const PREFIX =
    "businessDashboard.dashboard.menuBuilder.items.aiImageTools.dailyLimit";
  const LEAVES = [
    "title",
    "resets",
    "resetsHour",
    "resetsSoon",
    "contact",
  ] as const;

  it("ships the Menu Builder daily-limit copy in every operator locale", () => {
    for (const locale of ["en", "es", "es-AR"] as const) {
      for (const leaf of LEAVES) {
        const key = `${PREFIX}.${leaf}`;
        const v = getTranslation(key, locale);
        expect(typeof v).toBe("string");
        expect(v).toBeTruthy();
        // Compare against sentenceCaseLeaf itself, not a leaf-name regex: it
        // splits camelCase, so the real fallback is "Resets hour" and a
        // single-token /ResetsHour/ pattern never matches it.
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
      (
        bundle as {
          dashboard?: {
            menuBuilder?: {
              items?: {
                aiImageTools?: { dailyLimit?: Record<string, string> };
              };
            };
          };
        }
      ).dashboard?.menuBuilder?.items?.aiImageTools?.dailyLimit ?? {};

    for (const [locale, bundle] of [
      ["en", enDashboard],
      ["es", esDashboard],
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
    const esAr = rawDailyLimit(esArDashboard);
    expect(Object.keys(esAr).sort()).toEqual(["contact", "title"]);
  });
});
