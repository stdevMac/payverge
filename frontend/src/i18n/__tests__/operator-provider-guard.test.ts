import { readFileSync, readdirSync } from "fs";
import { join } from "path";

// Recursively collect .ts/.tsx source files under a directory, skipping test files.
function collectSourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...collectSourceFiles(full));
      continue;
    }
    if (!/\.(ts|tsx)$/.test(entry.name)) continue;
    if (/\.test\.tsx?$/.test(entry.name)) continue;
    out.push(full);
  }
  return out;
}

const srcRoot = join(__dirname, "..", ".."); // -> frontend/src
const operatorDirs = [
  join(srcRoot, "app", "business"),
  join(srcRoot, "components", "business"),
];
const operatorFiles = operatorDirs.flatMap((d) => collectSourceFiles(d));

// The guest-tier provider serves diner-facing storefront/menu locales
// (guest-messages/*.json). Operator dashboard surfaces must NEVER import it —
// that is the only real "wrong translation tier" downgrade direction. The
// operator tier (SimpleTranslationProvider) covers the three operatorLocale
// codes: en, es, es-AR.
const GUEST_PROVIDER_IMPORT = "@/i18n/GuestTranslationProvider";

describe("operator translation provider guard", () => {
  test("collects the operator surface", () => {
    // Sanity: the recursive collector actually found operator files. If this
    // ever hits 0 the import-checking tests below would vacuously pass.
    expect(operatorFiles.length).toBeGreaterThan(0);
  });

  test("no operator page/component imports the guest-tier provider", () => {
    const offenders = operatorFiles.filter((file) =>
      readFileSync(file, "utf8").includes(GUEST_PROVIDER_IMPORT),
    );
    expect(offenders).toEqual([]);
  });

  test("historically-flagged operator pages use the operator provider", () => {
    const flagged = [
      join(
        srcRoot,
        "app",
        "business",
        "[businessId]",
        "bills",
        "[billId]",
        "alternative-payments",
        "page.tsx",
      ),
    ];
    for (const file of flagged) {
      const source = readFileSync(file, "utf8");
      expect(source).toContain("@/i18n/SimpleTranslationProvider");
      expect(source).not.toContain(GUEST_PROVIDER_IMPORT);
    }
  });
});

describe("operator provider naming", () => {
  test("exports an OperatorTranslationProvider alias for the operator provider", () => {
    // Use require() so the assertion targets the module's runtime exports.
    const mod = require("../SimpleTranslationProvider");
    expect(mod.OperatorTranslationProvider).toBeDefined();
    expect(mod.OperatorTranslationProvider).toBe(mod.SimpleTranslationProvider);
  });
});
