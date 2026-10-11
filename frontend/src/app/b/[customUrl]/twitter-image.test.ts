/** @jest-environment node */

import fs from "node:fs";
import path from "node:path";

describe("storefront twitter-image (#654)", () => {
  it("re-exports the working opengraph-image file-convention card", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "twitter-image.tsx"),
      "utf8",
    );
    expect(src).toContain("./opengraph-image");
    expect(src).toMatch(/export\s*\{[^}]*default/);
    expect(src).toMatch(/export\s*\{[^}]*size/);
    expect(src).toMatch(/export\s*\{[^}]*contentType/);
  });

  it("declares segment config as literals (Next.js ignores re-exported runtime/dynamic)", () => {
    const read = (f: string) =>
      fs.readFileSync(path.join(__dirname, f), "utf8");
    const src = read("twitter-image.tsx");
    const og = read("opengraph-image.tsx");
    for (const key of ["runtime", "dynamic"]) {
      const literal = new RegExp(`export const ${key} = "([^"]+)";`);
      expect(src).not.toMatch(new RegExp(`export\\s*\\{[^}]*\\b${key}\\b`));
      expect(src.match(literal)?.[1]).toBeDefined();
      expect(src.match(literal)?.[1]).toBe(og.match(literal)?.[1]);
    }
  });
});
