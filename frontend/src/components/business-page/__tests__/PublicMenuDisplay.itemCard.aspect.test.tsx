/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../PublicMenuDisplay.tsx"),
  "utf-8",
);

describe("PublicMenuDisplay item-card image area", () => {
  test("does not use MenuCarousel inside the item card image area", () => {
    // Isolate just the card grid: everything after the menu-items container
    // marker but before the "Item Details Modal" section (the modal still
    // uses MenuCarousel for multi-image swipe). The container comment carries
    // the menu_layout note since the grid/list wiring (Phase 2).
    const afterGrid = SOURCE.split("grid or list per design_settings")[1] ?? "";
    const itemCardSection = afterGrid.split("Item Details Modal")[0] ?? "";
    expect(itemCardSection).not.toMatch(/<MenuCarousel\b/);
    expect(itemCardSection).toMatch(/<MenuItemMedia\b/);
  });

  test("imports MenuItemMedia", () => {
    expect(SOURCE).toMatch(/from\s+["']\.\/MenuItemMedia["']/);
  });
});
