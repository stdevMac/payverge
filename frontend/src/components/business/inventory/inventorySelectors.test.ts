// frontend/src/components/business/inventory/inventorySelectors.test.ts
import { InventoryItem } from "@/api/inventory";
import {
  itemStatus,
  selectItemValue,
  selectTotalStockValue,
} from "./inventorySelectors";

function item(partial: Partial<InventoryItem>): InventoryItem {
  return {
    id: 1,
    business_id: 42,
    name: "Olive Oil",
    unit: "liter",
    current_quantity: 0,
    reorder_threshold: 0,
    cost_per_unit: 0,
    is_active: true,
    created_at: "",
    updated_at: "",
    ...partial,
  };
}

describe("itemStatus", () => {
  it("is out_of_stock at or below zero", () => {
    expect(itemStatus(item({ current_quantity: 0 }))).toBe("out_of_stock");
    expect(itemStatus(item({ current_quantity: -2 }))).toBe("out_of_stock");
  });
  it("is low_stock at or below a positive threshold", () => {
    expect(
      itemStatus(item({ current_quantity: 3, reorder_threshold: 5 })),
    ).toBe("low_stock");
    expect(
      itemStatus(item({ current_quantity: 5, reorder_threshold: 5 })),
    ).toBe("low_stock");
  });
  it("ignores a zero threshold (untracked reorder point)", () => {
    expect(
      itemStatus(item({ current_quantity: 1, reorder_threshold: 0 })),
    ).toBe("ok");
  });
  it("is ok above threshold", () => {
    expect(
      itemStatus(item({ current_quantity: 10, reorder_threshold: 5 })),
    ).toBe("ok");
  });
});

describe("value selectors", () => {
  it("multiplies quantity by unit cost (no cents re-division)", () => {
    expect(selectItemValue(item({ current_quantity: 4, cost_per_unit: 2.5 }))).toBe(10);
  });
  it("sums total stock value", () => {
    const items = [
      item({ current_quantity: 4, cost_per_unit: 2.5 }),
      item({ current_quantity: 2, cost_per_unit: 3 }),
    ];
    expect(selectTotalStockValue(items)).toBe(16);
  });
  it("treats missing numbers as zero", () => {
    expect(
      selectItemValue(item({ current_quantity: 5, cost_per_unit: undefined as unknown as number })),
    ).toBe(0);
  });
  it("clamps a negative (oversold) quantity to zero value (INV-L3)", () => {
    // Warn-mode oversell drives current_quantity negative; an oversold item
    // can't represent negative shelf value, so its value contribution is 0.
    expect(
      selectItemValue(item({ current_quantity: -5, cost_per_unit: 2 })),
    ).toBe(0);
  });
  it("a negative item does not subtract from the total stock value (INV-L3)", () => {
    const items = [
      item({ current_quantity: 4, cost_per_unit: 2.5 }), // 10
      item({ current_quantity: -3, cost_per_unit: 2 }), // would be -6, clamped to 0
    ];
    expect(selectTotalStockValue(items)).toBe(10);
  });
  it("rounds stock value to cents (FIND-043 float residue)", () => {
    // 36 * 4.2 → 151.20000000000002 without rounding
    expect(selectItemValue(item({ current_quantity: 36, cost_per_unit: 4.2 }))).toBe(151.2);
    const items = [
      item({ current_quantity: 36, cost_per_unit: 4.2 }),
      item({ current_quantity: 22, cost_per_unit: 21.5 }),
      item({ current_quantity: 29.92, cost_per_unit: 2.1 }),
    ];
    expect(selectTotalStockValue(items)).toBe(687.03);
  });
});
