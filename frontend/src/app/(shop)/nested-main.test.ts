import fs from "fs";
import path from "path";

const shopRoot = __dirname;

function walk(dir: string, acc: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, acc);
    else if (/\.(tsx|ts|jsx|js)$/.test(entry.name)) acc.push(full);
  }
  return acc;
}

// Source without comments, so prose that names <main> is not a landmark.
function stripComments(file: string): string {
  return fs
    .readFileSync(file, "utf8")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
}

describe("shop route group landmarks (#454)", () => {
  it("keeps a single shell main and no nested page mains", () => {
    const layout = fs.readFileSync(path.join(shopRoot, "layout.tsx"), "utf8");
    expect(layout).toMatch(/<main[\s\S]{0,80}id="main-content"/);

    const nested = walk(shopRoot).filter((file) => {
      if (path.basename(file) === "layout.tsx") return false;
      if (file.endsWith(".test.ts") || file.endsWith(".test.tsx")) return false;
      return /<main[\s>]/.test(fs.readFileSync(file, "utf8"));
    });
    expect(nested).toEqual([]);
  });
  // The (shop) shell already owns the page's <main id="main-content">, so a
  // shared component that renders <main> nests a second landmark on every
  // (shop) route that uses it (the operator dashboard did). Only components
  // that ARE the page outside (shop) may render one.
  it("keeps shared components free of a second <main> landmark", () => {
    const componentsRoot = path.join(shopRoot, "..", "..", "components");
    const pageOwners = new Set([
      // /b/<slug> storefront and the "/" storefront (outside (shop)).
      path.join("business-page", "ConvertingBusinessLandingPage.tsx"),
      // "/" venue directory (outside (shop)).
      path.join("venue-directory", "VenueDirectory.tsx"),
    ]);
    const offenders = walk(componentsRoot)
      .filter((file) => !/\.test\.(ts|tsx)$/.test(file))
      .filter((file) => !file.includes(`${path.sep}__tests__${path.sep}`))
      .filter((file) => !pageOwners.has(path.relative(componentsRoot, file)))
      .filter((file) => /<main[\s>]/.test(stripComments(file)))
      .map((file) => path.relative(componentsRoot, file));
    expect(offenders).toEqual([]);
  });
});
