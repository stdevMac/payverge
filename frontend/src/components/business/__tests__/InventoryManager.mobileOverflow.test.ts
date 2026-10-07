import fs from "fs";
import path from "path";

const SOURCE = fs.readFileSync(
  path.join(__dirname, "../InventoryManager.tsx"),
  "utf8",
);

describe("InventoryManager mobile overflow (#461)", () => {
  it("wraps the items header action cluster instead of locking it to one row", () => {
    expect(SOURCE).toMatch(/data-testid="inventory-items-header-actions"/);
    expect(SOURCE).toMatch(
      /data-testid="inventory-items-header-actions"[\s\S]{0,220}flex-wrap/,
    );
    expect(SOURCE).toMatch(
      /data-testid="inventory-items-header-actions"[\s\S]{0,220}min-w-0/,
    );
  });

  it("puts the items table in a local horizontal scroller instead of overflow-hidden", () => {
    expect(SOURCE).toMatch(/data-testid="inventory-items-table-scroller"/);
    expect(SOURCE).toMatch(
      /data-testid="inventory-items-table-scroller"[\s\S]{0,280}overflow-x-auto/,
    );
    expect(SOURCE).not.toMatch(
      /overflow-hidden rounded-2xl border border-warm-200\/80 bg-white shadow-sm shadow-warm-900\/5/,
    );
  });
});
