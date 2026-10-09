/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

// OP-2 / LOCALE-6: HybridAuthProvider must route the backend-saved language
// into the SimpleTranslationProvider render path (the canonical `locale` key)
// and must no longer depend on the removed useLanguage / `language`-key system.
const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../HybridAuthProvider.tsx"),
  "utf-8",
);

describe("HybridAuthProvider — operator locale bridge (OP-2/LOCALE-6)", () => {
  test("no longer imports or calls the removed useLanguage system", () => {
    // No import from the deleted module and no hook invocation. (Plain-prose
    // mentions in explanatory comments are fine — they document the removal.)
    expect(SOURCE).not.toMatch(/from\s+["']@\/i18n\/useLanguage["']/);
    expect(SOURCE).not.toMatch(/useLanguage\(/);
    expect(SOURCE).not.toMatch(/\bsetLanguage\b/);
    // The old divergent localStorage["language"] write is gone.
    expect(SOURCE).not.toMatch(/localStorage\.setItem\(\s*["']language["']/);
  });

  test("consumes SimpleTranslationProvider's setLocale", () => {
    expect(SOURCE).toMatch(
      /from\s+["']@\/i18n\/OperatorLocaleProvider["']/,
    );
    expect(SOURCE).toContain("useSimpleLocale");
    expect(SOURCE).toMatch(/const\s*{\s*setLocale\s*}\s*=\s*useSimpleLocale\(\)/);
  });

  test("seeds the render locale via the guarded bridge helper", () => {
    expect(SOURCE).toMatch(
      /from\s+["']@\/i18n\/operatorLocaleBridge["']/,
    );
    expect(SOURCE).toContain("resolveBackendLocaleSeed");
    // Bridge reads the existing explicit `locale` pick to avoid clobbering it.
    expect(SOURCE).toMatch(/localStorage\.getItem\(\s*["']locale["']\s*\)/);
    // And applies the resolved seed through setLocale.
    expect(SOURCE).toMatch(/setLocale\(seedLocale\)/);
  });
});
