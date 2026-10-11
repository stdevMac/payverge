import fs from "fs";
import path from "path";

const SOURCE = fs.readFileSync(path.join(__dirname, "ReportsTab.tsx"), "utf8");

describe("ReportsTab mobile comparison overflow (#463)", () => {
  it("puts the P&L table in a local horizontal scroller", () => {
    expect(SOURCE).toMatch(/data-testid="pnl-table-scroller"/);
    expect(SOURCE).toMatch(
      /data-testid="pnl-table-scroller"[\s\S]{0,220}overflow-x-auto/,
    );
  });
});
