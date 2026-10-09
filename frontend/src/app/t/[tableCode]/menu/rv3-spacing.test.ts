import fs from "fs";
import path from "path";

describe("RV-3 guest menu search spacing", () => {
  it("search/filters row uses gap-3 or larger, not gap-2", () => {
    const src = fs.readFileSync(path.join(__dirname, "page.tsx"), "utf8");
    const bad = [...src.matchAll(/flex items-center gap-2\b/g)].filter((m) => {
      const window = src.slice(Math.max(0, m.index! - 80), m.index! + 200);
      return /search|filter|Input/i.test(window);
    });
    expect(bad.length).toBe(0);
    expect(src).toMatch(/flex items-center gap-3/);
  });
});
