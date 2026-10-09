/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);

describe("dashboard Cuentas badge is the approval queue (#793)", () => {
  it("does not feed open-check counts into the sidebar badge", () => {
    // #92/#619 made the badge count open checks; #793 redefines it as
    // pending-approval tickets, derived inside useRailBadges from the same
    // globalOrders feed the Bills approval strip renders. Open-check totals
    // stay as tab copy, never a rail pip.
    expect(SOURCE).not.toMatch(/activeBillsCount=/);
    expect(SOURCE).not.toMatch(/countOpenChecks\(/);
    expect(SOURCE).toMatch(/globalOrdersLoaded=\{globalOrdersLoaded\}/);
  });
});

describe("dashboard Mesas badge uses floor occupancy (#774)", () => {
  it("loads occupancy from /tables/status, not the kitchen activeBillsOnly feed", () => {
    expect(SOURCE).toMatch(/fetchOccupiedTablesCount\(/);
    expect(SOURCE).toMatch(/occupiedTablesCount=\{occupiedTablesCount\}/);
    expect(SOURCE).not.toMatch(/occupiedTablesCount=\{countOccupiedTables\(/);
    expect(SOURCE).toMatch(/activeBillsOnly:\s*true/);
  });
});
