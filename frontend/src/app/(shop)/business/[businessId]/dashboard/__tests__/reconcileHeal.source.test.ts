/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);

/**
 * Wiring only: the board contract lives in reconcileFailures.test.ts.
 * These checks exist so page.tsx cannot silently drop applyReconcileCycle
 * or go back to OR-ing per-leg 429 flags.
 */
describe("dashboard live-reconcile page wiring (#623)", () => {
  it("feeds every probed leg through applyReconcileCycle", () => {
    expect(SOURCE).toMatch(/applyReconcileCycle\(/);
    expect(SOURCE).toMatch(/bills:\s*billsLeg/);
    expect(SOURCE).toMatch(/orders:\s*ordersLeg/);
    expect(SOURCE).toMatch(/reservations:\s*reservationsLeg/);
    expect(SOURCE).toMatch(/crm:\s*crmLeg/);
    expect(SOURCE).toMatch(/Promise\.allSettled/);
    expect(SOURCE).toMatch(
      /setReconcileFailureCount\(\s*\(prev\)\s*=>\s*applyReconcileCycle\(prev,\s*cycleLegs\)\.nextCount/,
    );
  });

  it("does not OR per-leg 429 flags in page.tsx", () => {
    expect(SOURCE).not.toMatch(/rateLimited\s*=\s*rateLimited\s*\|\|/);
    expect(SOURCE).not.toMatch(/ordersLeg\.rateLimited\s*\|\|/);
  });

  it("keeps polling while a pinned counter is still up", () => {
    expect(SOURCE).toMatch(
      /shouldScheduleGlobalBillPoll\s*\(\s*visibleActiveTab\s*\)\s*\|\|/,
    );
    expect(SOURCE).toMatch(/reconcileFailureCount\s*>\s*0/);
  });
});
