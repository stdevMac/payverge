/** @jest-environment jsdom */
import { renderHook } from "@testing-library/react";
import { InventoryItem } from "@/api/inventory";
import {
  InventoryFilterState,
  useInventoryFilter,
} from "./useInventoryFilter";

function item(p: Partial<InventoryItem>): InventoryItem {
  return {
    id: p.id ?? 1,
    business_id: 42,
    name: p.name ?? "Item",
    sku: p.sku,
    category: p.category,
    unit: "unit",
    current_quantity: p.current_quantity ?? 10,
    reorder_threshold: p.reorder_threshold ?? 0,
    cost_per_unit: p.cost_per_unit ?? 1,
    is_active: true,
    created_at: "",
    updated_at: "",
  };
}

const items: InventoryItem[] = [
  item({ id: 1, name: "Olive Oil", sku: "OIL-1", category: "Pantry", current_quantity: 0 }),
  item({ id: 2, name: "Tomatoes", category: "Produce", current_quantity: 2, reorder_threshold: 5 }),
  item({ id: 3, name: "Basil", category: "Produce", current_quantity: 20, reorder_threshold: 5 }),
  item({ id: 4, name: "Flour", category: "Pantry", current_quantity: 8, cost_per_unit: 3 }),
];

const base: InventoryFilterState = {
  search: "",
  category: "",
  status: "all",
  sort: "attention",
};

function run(state: Partial<InventoryFilterState>) {
  return renderHook(() => useInventoryFilter(items, { ...base, ...state })).result.current;
}

describe("useInventoryFilter", () => {
  it("computes counts independent of active filters", () => {
    const r = run({ status: "out" });
    expect(r.counts).toEqual({ all: 4, healthy: 2, low: 1, out: 1 });
  });

  it("lists distinct categories sorted", () => {
    expect(run({}).categories).toEqual(["Pantry", "Produce"]);
  });

  it("searches name and sku, case-insensitive", () => {
    expect(run({ search: "oil" }).visibleItems.map((i) => i.id)).toEqual([1]);
    expect(run({ search: "oil-1" }).visibleItems.map((i) => i.id)).toEqual([1]);
  });

  it("filters by category", () => {
    expect(run({ category: "Produce" }).visibleItems.map((i) => i.id).sort()).toEqual([2, 3]);
  });

  it("filters by status", () => {
    expect(run({ status: "out" }).visibleItems.map((i) => i.id)).toEqual([1]);
    expect(run({ status: "low" }).visibleItems.map((i) => i.id)).toEqual([2]);
  });

  it("default 'attention' sort floats out then low then ok", () => {
    expect(run({}).visibleItems.map((i) => i.id)).toEqual([1, 2, 3, 4]);
  });

  it("sorts by value high to low", () => {
    // values: Flour 24, Basil 20, Tomatoes 2, Oil 0
    expect(run({ sort: "value" }).visibleItems.map((i) => i.id)).toEqual([4, 3, 2, 1]);
  });

  it("sorts by name A-Z", () => {
    expect(run({ sort: "name" }).visibleItems.map((i) => i.name)).toEqual([
      "Basil",
      "Flour",
      "Olive Oil",
      "Tomatoes",
    ]);
  });
});
