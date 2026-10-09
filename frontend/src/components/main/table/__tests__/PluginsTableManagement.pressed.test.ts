import fs from "fs";
import path from "path";

const SOURCE = fs.readFileSync(
  path.join(__dirname, "../PluginsTableManagement.tsx"),
  "utf8",
);

describe("admin plugins view modes (#452)", () => {
  it("exposes aria-pressed for table/list/grid", () => {
    expect(SOURCE).toMatch(/aria-pressed=\{viewMode === "table"\}/);
    expect(SOURCE).toMatch(/aria-pressed=\{viewMode === "list"\}/);
    expect(SOURCE).toMatch(/aria-pressed=\{viewMode === "grid"\}/);
    expect(SOURCE).toMatch(/aria-label=\{t\("views.group"\)\}/);
  });
});
