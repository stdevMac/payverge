import fs from "node:fs";
import path from "node:path";

// R2-7: `insightCopy.ts` emits the `outOfStockMixed` key for an inventory set
// that is part oversold and part exactly zero. A copy key with no catalog entry
// renders the raw key to the operator, so every operator locale must ship it —
// and every placeholder the card model passes must appear in the template, or
// the sentence silently drops a group.
const MESSAGES_ROOT = path.resolve(__dirname, "..");

function loadDirectorConsole(folder: string): Record<string, unknown> {
  return JSON.parse(
    fs.readFileSync(
      path.join(MESSAGES_ROOT, folder, "directorConsole.json"),
      "utf8",
    ),
  ) as Record<string, unknown>;
}

function cards(folder: string): Record<string, unknown> {
  const tree = loadDirectorConsole(folder) as {
    preShift?: { cards?: Record<string, unknown> };
  };
  return tree.preShift?.cards ?? {};
}

const MIXED_PLACEHOLDERS = [
  "{count}",
  "{oversoldCount}",
  "{oversoldNames}",
  "{zeroCount}",
  "{zeroNames}",
];

describe("mixed oversold/at-zero inventory card copy (R2-7)", () => {
  for (const folder of ["en", "es"]) {
    describe(`${folder}`, () => {
      it("ships the outOfStockMixed template", () => {
        expect(typeof cards(folder).outOfStockMixed).toBe("string");
      });

      it("interpolates both groups' counts and names", () => {
        const template = String(cards(folder).outOfStockMixed ?? "");
        for (const placeholder of MIXED_PLACEHOLDERS) {
          expect(template).toContain(placeholder);
        }
      });
    });
  }

  // es-AR is an override layer: it only carries strings that diverge from es.
  // The mixed sentence ends in an imperative, which is exactly where Argentine
  // voseo diverges, so the override must be present.
  it("es-AR overrides the mixed template with the same placeholders", () => {
    const template = String(cards("es-ar").outOfStockMixed ?? "");
    expect(template.length).toBeGreaterThan(0);
    for (const placeholder of MIXED_PLACEHOLDERS) {
      expect(template).toContain(placeholder);
    }
    expect(template).not.toBe(String(cards("es").outOfStockMixed ?? ""));
  });
});

// R2-8: base `es` is the neutral tuteo tier; voseo belongs in the es-AR
// override layer ONLY (locales/LANGUAGES.md, ES_AR_OVERRIDE_LAYER.md). The
// L1-22 oversold strings shipped Argentine imperatives ("Contá y corregí") into
// base es, so every non-Argentine Spanish operator read Rioplatense copy.
describe("inventory card copy register (R2-8)", () => {
  // NOTE: `\b` does not fire after an accented vowel (JS \w is ASCII-only), so
  // these patterns must not anchor a trailing word boundary on "contá"/"corregí".
  const VOSEO_IMPERATIVE = /cont[áa]\s+y\s+correg[íi]/i;
  const TUTEO_IMPERATIVE = /cuenta\s+y\s+corrige/i;

  const inventoryKeys = (folder: string): string[] => {
    const c = cards(folder) as Record<string, unknown>;
    const oversold = (c.oversold ?? {}) as Record<string, unknown>;
    return [
      String(oversold.one ?? ""),
      String(oversold.other ?? ""),
      String(c.outOfStockMixed ?? ""),
    ];
  };

  it("base es keeps the neutral tuteo imperative", () => {
    for (const line of inventoryKeys("es")) {
      expect(line).not.toMatch(VOSEO_IMPERATIVE);
      expect(line).toMatch(TUTEO_IMPERATIVE);
    }
  });

  it("es-AR carries the voseo imperative for the same strings", () => {
    const c = cards("es-ar") as Record<string, unknown>;
    const oversold = (c.oversold ?? {}) as Record<string, unknown>;
    for (const line of [
      String(oversold.one ?? ""),
      String(oversold.other ?? ""),
      String(c.outOfStockMixed ?? ""),
    ]) {
      expect(line).toMatch(VOSEO_IMPERATIVE);
    }
  });
});
