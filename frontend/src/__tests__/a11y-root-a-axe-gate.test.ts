/**
 * Root A (Session O) — structural gate for the release axe specs.
 *
 * Playwright + a live stack cannot run at commit time. This suite freezes the
 * three holes the audit walked through so a future edit cannot re-open them:
 *   1. Coverage: operator rail loop must include `overview`
 *   2. Severity: named rule-id tier (or moderate floor) — not serious-only
 *   3. Mechanism: jsdom guards live under a11y-axe-blind-guards (separate file)
 */

import fs from "fs";
import path from "path";

const FRONTEND_ROOT = path.resolve(__dirname, "..", "..");

function read(rel: string): string {
  return fs.readFileSync(path.join(FRONTEND_ROOT, rel), "utf8");
}

describe("Root A — release axe gate structure", () => {
  const operatorSpec = "tests/release/operator-accessibility.spec.ts";
  const publicSpec = "tests/release/public-business-accessibility.spec.ts";

  let operatorSrc: string;
  let publicSrc: string;

  beforeAll(() => {
    operatorSrc = read(operatorSpec);
    publicSrc = read(publicSpec);
  });

  it("operator rail loop includes overview (hole 1: coverage)", () => {
    // Must be an actual tab loop entry, not a comment.
    expect(operatorSrc).toMatch(
      /for\s*\(\s*const\s+tab\s+of\s*\[[^\]]*"overview"[^\]]*\]/,
    );
  });

  it("operator rail loop still covers menu, bills, reservations", () => {
    expect(operatorSrc).toMatch(
      /for\s*\(\s*const\s+tab\s+of\s*\[[^\]]*\]/,
    );
    for (const tab of ["menu", "bills", "reservations"] as const) {
      expect(operatorSrc).toMatch(new RegExp(`"${tab}"`));
    }
  });

  it("operator + public specs assert a wave-O named-rule tier (hole 2: severity)", () => {
    // serious/critical alone is insufficient — require the named-id constant.
    for (const src of [operatorSrc, publicSrc]) {
      expect(src).toMatch(/WAVE_O_NAMED_RULE_IDS/);
      expect(src).toMatch(/"button-name"/);
      expect(src).toMatch(/"svg-img-alt"/);
      expect(src).toMatch(/"image-alt"/);
      // Must actually filter violations by those ids, not just declare them.
      expect(src).toMatch(/WAVE_O_NAMED_RULE_IDS[\s\S]{0,200}includes\(violation\.id\)/);
    }
  });

  it("still fails on serious/critical impact", () => {
    expect(operatorSrc).toMatch(/impact === "serious"/);
    expect(operatorSrc).toMatch(/impact === "critical"/);
  });
});
