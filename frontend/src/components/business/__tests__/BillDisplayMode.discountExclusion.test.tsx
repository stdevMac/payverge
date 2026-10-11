/**
 * #107 — discounts must not appear as orderable food lines in display/detail.
 * Pure helper coverage (BillDisplayMode / BillDetailsModal both use these).
 */
import type { BillItem } from "@/api/bills";
import { asDollars } from "@/types/money";
import {
  isDiscountBillLine,
  isOperatorFoodBillLine,
} from "@/lib/billBundleGrouping";

function item(
  partial: Partial<BillItem> & Pick<BillItem, "id" | "name">,
): BillItem {
  return {
    menu_item_id: partial.menu_item_id || partial.id,
    price: asDollars(partial.price ?? 0),
    quantity: partial.quantity ?? 1,
    options: [],
    subtotal: asDollars(partial.subtotal ?? 0),
    ...partial,
  } as BillItem;
}

describe("operator food vs discount line filters (#107)", () => {
  const lines = [
    item({
      id: "food",
      name: "Burger",
      item_type: "menu_item",
      price: asDollars(10),
      subtotal: asDollars(10),
    }),
    item({
      id: "child",
      name: "Fries",
      item_type: "bundle_item",
    }),
    item({
      id: "promo",
      name: "Weekday Lunch 15% Off",
      item_type: "discount",
      price: asDollars(-1.5),
      subtotal: asDollars(-1.5),
      order_id: 9,
    }),
  ];

  it("keeps only sellable food/bundle parents in the food list", () => {
    expect(lines.filter(isOperatorFoodBillLine).map((l) => l.name)).toEqual([
      "Burger",
    ]);
  });

  it("classifies promo lines as discounts for the emerald band", () => {
    expect(lines.filter(isDiscountBillLine).map((l) => l.name)).toEqual([
      "Weekday Lunch 15% Off",
    ]);
  });
});
