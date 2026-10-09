import { InventoryItem } from "@/api/inventory";
import { buildInventoryCsv } from "./inventoryCsv";

function item(p: Partial<InventoryItem>): InventoryItem {
  return {
    id: p.id ?? 1,
    business_id: 42,
    name: p.name ?? "Item",
    sku: p.sku,
    category: p.category,
    unit: p.unit ?? "unit",
    current_quantity: p.current_quantity ?? 0,
    reorder_threshold: p.reorder_threshold ?? 0,
    cost_per_unit: p.cost_per_unit ?? 0,
    is_active: true,
    created_at: "",
    updated_at: "",
  };
}

describe("buildInventoryCsv", () => {
  it("emits a header row and one row per item", () => {
    const csv = buildInventoryCsv(
      [item({ name: "Flour", sku: "F1", category: "Pantry", unit: "kg", current_quantity: 8, reorder_threshold: 2, cost_per_unit: 3 })],
      { statusLabel: () => "Healthy" },
    );
    const lines = csv.trim().split("\n");
    expect(lines[0]).toBe("Name,SKU,Category,Unit,On hand,Reorder at,Unit cost,Value,Status");
    expect(lines[1]).toBe("Flour,F1,Pantry,kg,8,2,3.00,24.00,Healthy");
  });

  it("quotes and escapes values containing commas or quotes", () => {
    const csv = buildInventoryCsv(
      [item({ name: 'Oil, "Extra" Virgin', current_quantity: 1, cost_per_unit: 2 })],
      { statusLabel: () => "Healthy" },
    );
    expect(csv.split("\n")[1]).toContain('"Oil, ""Extra"" Virgin"');
  });

  it("neutralizes formula-injection characters at the start of a cell", () => {
    const injectionNames = [
      "=SUM(A1:B1)",
      "+cmd|' /C calc'!A0",
      "-2+3+cmd|' /C calc'!A0",
      "@SUM(1+1)*cmd|' /C calc'!A0",
    ];
    for (const name of injectionNames) {
      const csv = buildInventoryCsv(
        [item({ name, current_quantity: 1, cost_per_unit: 1 })],
        { statusLabel: () => "Healthy" },
      );
      const firstCell = csv.split("\n")[1].split(",")[0];
      // Must NOT start with the dangerous character directly (after any surrounding quotes)
      const raw = firstCell.replace(/^"|"$/g, "");
      expect(raw.charAt(0)).not.toMatch(/[=+\-@\t\r]/);
    }
  });
});

it("localizes Spanish headers for es / es-AR", () => {
  const csv = buildInventoryCsv([], {
    statusLabel: () => "ok",
    locale: "es-AR",
  });
  expect(csv.split("\n")[0]).toContain("Nombre");
  expect(csv.split("\n")[0]).toContain("En stock");
});
