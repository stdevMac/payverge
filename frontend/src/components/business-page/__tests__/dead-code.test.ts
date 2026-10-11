import fs from "fs";
import path from "path";
import { walkSrcFiles, fileContains, FRONTEND_ROOT } from "./_walkSrc";

const REMOVED_FILES = [
  "src/components/business-page/BusinessInfo.tsx",
  "src/components/business-page/BusinessHeader.tsx",
  "src/components/business-page/BusinessMenu.tsx",
  "src/components/business-page/LanguageSelector.tsx",
];

describe("business-page dead code", () => {
  it.each(REMOVED_FILES)("file %s no longer exists", (rel) => {
    expect(fs.existsSync(path.join(FRONTEND_ROOT, rel))).toBe(false);
  });

  it.each(REMOVED_FILES)("symbol from %s is not imported anywhere", (rel) => {
    const noExt = path.basename(rel).replace(/\.tsx?$/, "");
    const importPattern = new RegExp(
      `from\\s+["'][^"']*business-page/${noExt}["']`,
    );
    const offenders = walkSrcFiles().filter((f) => fileContains(f, importPattern));
    expect(offenders).toEqual([]);
  });
});

describe("orchestrator dedupe", () => {
  it("does not render the duplicate '-mt-10' delivery/reservation strip", () => {
    const cblp = path.join(
      FRONTEND_ROOT,
      "src/components/business-page/ConvertingBusinessLandingPage.tsx",
    );
    const src = fs.readFileSync(cblp, "utf8");
    expect(src).not.toContain("-mt-10");
    expect(src).not.toMatch(
      /<p className="text-xs uppercase tracking-\[0\.2em\] text-gray-500 mb-2">Delivery<\/p>/,
    );
    expect(src).not.toMatch(
      /<p className="text-xs uppercase tracking-\[0\.2em\] text-gray-500 mb-2">Reservations<\/p>/,
    );
  });
});

describe("orchestrator mobile-nav dedupe", () => {
  it("does not render the duplicate fixed-bottom mobile button row", () => {
    const cblp = path.join(
      FRONTEND_ROOT,
      "src/components/business-page/ConvertingBusinessLandingPage.tsx",
    );
    const src = fs.readFileSync(cblp, "utf8");
    expect(src).not.toContain("fixed inset-x-0 bottom-4 z-40");
  });
});
