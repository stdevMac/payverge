import type { BillItem } from "@/api/bills";
import { asDollars } from "@/types/money";
import {
  buildBundleGroupKeys,
  countGuestFacingBillItems,
  isDiscountBillLine,
  isIncludedBundleChild,
  isOperatorFoodBillLine,
  resolveBundleGroupKey,
} from "./billBundleGrouping";

function item(partial: Partial<BillItem> & Pick<BillItem, "id" | "name">): BillItem {
  return {
    menu_item_id: partial.menu_item_id || partial.id,
    price: asDollars(partial.price ?? 0),
    quantity: partial.quantity ?? 1,
    options: [],
    subtotal: asDollars(partial.subtotal ?? 0),
    ...partial,
  } as BillItem;
}

describe("billBundleGrouping", () => {
  it("nests each occurrence's children under that occurrence only", () => {
    const items = [
      item({
        id: "bundle-a",
        name: "Date Night",
        item_type: "bundle",
        bundle_id: 10,
        bundle_occurrence_id: "occ-a",
        price: asDollars(68),
        subtotal: asDollars(68),
      }),
      item({
        id: "child-a1",
        name: "Steak",
        item_type: "bundle_item",
        parent_bundle_id: 10,
        bundle_occurrence_id: "occ-a",
      }),
      item({
        id: "bundle-b",
        name: "Date Night",
        item_type: "bundle",
        bundle_id: 10,
        bundle_occurrence_id: "occ-b",
        price: asDollars(68),
        subtotal: asDollars(68),
      }),
      item({
        id: "child-b1",
        name: "Spritz",
        item_type: "bundle_item",
        parent_bundle_id: 10,
        bundle_occurrence_id: "occ-b",
        quantity: 2,
      }),
    ];

    const keys = buildBundleGroupKeys(items);
    const byParent: Record<string, string[]> = {};
    for (const child of items.filter((i) => i.item_type === "bundle_item")) {
      const key = resolveBundleGroupKey(child, keys);
      byParent[key] = byParent[key] || [];
      byParent[key].push(child.name);
    }

    expect(byParent["occurrence:occ-a"]).toEqual(["Steak"]);
    expect(byParent["occurrence:occ-b"]).toEqual(["Spritz"]);
    expect(countGuestFacingBillItems(items)).toBe(2);
  });

  it("reconstructs legacy sequential occurrences without occurrence ids", () => {
    const items = [
      item({
        id: "p1",
        name: "Combo",
        item_type: "bundle",
        bundle_id: 7,
        price: asDollars(20),
        subtotal: asDollars(20),
      }),
      item({
        id: "c1",
        name: "Burger",
        item_type: "bundle_item",
        parent_bundle_id: 7,
      }),
      item({
        id: "p2",
        name: "Combo",
        item_type: "bundle",
        bundle_id: 7,
        price: asDollars(20),
        subtotal: asDollars(20),
      }),
      item({
        id: "c2",
        name: "Salad",
        item_type: "bundle_item",
        parent_bundle_id: 7,
      }),
    ];

    const keys = buildBundleGroupKeys(items);
    expect(resolveBundleGroupKey(items[0], keys)).toBe("legacy:7:1");
    expect(resolveBundleGroupKey(items[1], keys)).toBe("legacy:7:1");
    expect(resolveBundleGroupKey(items[2], keys)).toBe("legacy:7:2");
    expect(resolveBundleGroupKey(items[3], keys)).toBe("legacy:7:2");
  });

  it("detects included (zero-priced) bundle children", () => {
    expect(
      isIncludedBundleChild(
        item({ id: "x", name: "Fries", price: asDollars(0), subtotal: asDollars(0) }),
      ),
    ).toBe(true);
    expect(
      isIncludedBundleChild(
        item({
          id: "steak",
          name: "Steak Plate",
          item_type: "bundle_item",
          price: asDollars(42),
          subtotal: asDollars(0),
        }),
      ),
    ).toBe(true);
    expect(
      isIncludedBundleChild(
        item({ id: "y", name: "Upgrade", price: asDollars(3), subtotal: asDollars(3) }),
      ),
    ).toBe(false);
  });

  it("separates operator food lines from discount lines (#107)", () => {
    const food = item({ id: "f", name: "Burger", item_type: "menu_item" });
    const child = item({ id: "c", name: "Side", item_type: "bundle_item" });
    const discount = item({
      id: "d",
      name: "Promo",
      item_type: "discount",
      price: asDollars(-2),
      subtotal: asDollars(-2),
    });
    expect(isOperatorFoodBillLine(food)).toBe(true);
    expect(isOperatorFoodBillLine(child)).toBe(false);
    expect(isOperatorFoodBillLine(discount)).toBe(false);
    expect(isDiscountBillLine(discount)).toBe(true);
    expect(isDiscountBillLine(food)).toBe(false);
  });
});
