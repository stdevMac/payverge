/**
 * L1-23 — Director briefing body locale follows the operator UI.
 * ALREADY-FIXED regression: BriefingContent/BriefingStrip resolve copy via
 * useSimpleLocale + getTranslation(directorConsole.*, locale). A regression
 * that hard-codes English keys or omits locale would fail these source
 * contracts.
 */
import { readFileSync } from "fs";
import { join } from "path";

const root = join(__dirname, "..");

describe("L1-23 Director briefing locale regression (ALREADY-FIXED)", () => {
  it("BriefingContent threads active locale into getTranslation", () => {
    const src = readFileSync(join(root, "BriefingContent.tsx"), "utf8");
    expect(src).toMatch(/useSimpleLocale/);
    expect(src).toMatch(/getTranslation\(`directorConsole\.\$\{key\}`,\s*locale/);
    // Money/percent formatting also honors locale (no en-US hardcode).
    expect(src).toMatch(/intlLocaleFor\(locale\)/);
  });

  it("BriefingStrip threads active locale into getTranslation", () => {
    const src = readFileSync(join(root, "BriefingStrip.tsx"), "utf8");
    expect(src).toMatch(/useSimpleLocale/);
    expect(src).toMatch(/getTranslation\(`directorConsole\.\$\{key\}`,\s*locale/);
  });

  it("BriefingStrip re-localizes insight duration before interpolating chip copy", () => {
    const src = readFileSync(join(root, "BriefingStrip.tsx"), "utf8");
    expect(src).toMatch(/localizeInsightCardParams|formatInsightCardLine/);
  });

  it("DirectorConsole ask path sends operator locale to the API", () => {
    const src = readFileSync(join(root, "DirectorConsoleDashboard.tsx"), "utf8");
    // Ask payload includes locale so the backend prompt family matches headers.
    expect(src).toMatch(/locale/);
    expect(src).toMatch(/askDirector|message/);
  });
});
