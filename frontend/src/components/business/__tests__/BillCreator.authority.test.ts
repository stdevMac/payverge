import fs from "fs";
import path from "path";

describe("BillCreator pricing and orderability authority", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "../BillCreator.tsx"), "utf8");

  it("does not calculate promotion discounts locally", () => {
    expect(source).not.toContain("localPromotionPreview");
    expect(source).not.toMatch(/discount_value\s*\/\s*100/);
    expect(source).toContain("neutralCartSubtotal");
    expect(source).not.toContain("promotionPreview.discountedSubtotal || subtotal");
  });

  it("uses the menu item_orderability projection instead of inventory summary flags", () => {
    expect(source).toContain("menuResponse.item_orderability");
    expect(source).not.toContain("inventoryApi.getSummary");
    expect(source).not.toContain("inventoryStatuses");
  });
});
