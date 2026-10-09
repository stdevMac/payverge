import { readFileSync } from "fs";
import { join } from "path";

// F16: Settings › Currency & Languages must not show a "Discard changes" /
// dirty state on load with no user edits. The prior code initialized both
// `settings` and `originalSettings` to {USD,USD}; a non-USD business briefly
// diffed against that placeholder during loadData → false dirty state.
//
// Fix: both start null (not loaded) and the currency dirty check no-ops until
// data has loaded. CurrencySettings is a large NextUI component with many API
// deps; per the established source-lock convention we assert the invariants.
describe("CurrencySettings load-time dirty state (F16)", () => {
  const source = readFileSync(
    join(__dirname, "..", "CurrencySettings.tsx"),
    "utf8",
  );

  it("initializes settings and originalSettings to null, not a USD placeholder", () => {
    expect(source).toMatch(
      /useState<BusinessCurrencySettings \| null>\(\s*null,?\s*\)/,
    );
    expect(source).toMatch(
      /useState<BusinessCurrencySettings \| null>\(null\)/,
    );
    // The old {USD,USD} double-init must be gone.
    expect(source).not.toMatch(
      /useState<BusinessCurrencySettings>\(\{\s*default_currency: "USD"/,
    );
  });

  it("treats a not-yet-loaded state as not dirty", () => {
    // hasCurrencyChanges must short-circuit to false when either half is null.
    expect(source).toMatch(
      /if \(!settings \|\| !originalSettings\) \{\s*return false;/,
    );
  });

  it("treats languages as not dirty until loadData stamps a baseline (#224)", () => {
    expect(source).toMatch(/const \[languagesLoaded, setLanguagesLoaded\]/);
    expect(source).toMatch(
      /if \(!languagesLoaded\) \{\s*return false;/,
    );
    // Empty baseline — not the old ["en"] vs [] false dirty.
    expect(source).toMatch(
      /useState<string\[\]>\(\[\]\);\s*\n\s*const \[originalDefaultLanguage/,
    );
    expect(source).toMatch(/onDirtyChange\?\.\(false\)/);
    // Loading settings… must force the parent SaveBar clean (#224).
    expect(source).toMatch(/if \(loading\) \{\s*onDirtyChange\?\.\(false\);/);
  });

  it("keeps imperative save callbacks fresh without hook suppressions", () => {
    expect(source).toMatch(/const handleSaveCurrency = useCallback\(async \(\) =>/);
    expect(source).toMatch(/const handleSaveLanguage = useCallback\(async \(\) =>/);
    expect(source).toMatch(/const handleSaveVenue = useCallback\(async \(\) =>/);
    expect(source).toMatch(
      /\[\s*handleSaveCurrency,\s*handleSaveLanguage,\s*handleSaveVenue,\s*hasCurrencyChanges,\s*hasLanguageChanges,\s*hasVenueChanges,\s*\]/,
    );
    expect(source).not.toMatch(/react-hooks\/exhaustive-deps/);
  });
});
