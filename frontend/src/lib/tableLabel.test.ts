import { formatEntityName, getBillLocationLabel } from "./tableLabel";

/**
 * L1-25 + PG-29 — ONE test file asserting BOTH directions on formatEntityName:
 * - L1-25: never "Table Table 1" (double prefix)
 * - PG-29: never English "Table 1" when the active label is Spanish "Mesa"
 */
describe("formatEntityName (L1-25 + PG-29 dual direction)", () => {
  it("L1-25: does not double-prefix when the name already starts with the label", () => {
    expect(formatEntityName("Table", "Table 1")).toBe("Table 1");
    expect(formatEntityName("Counter", "Counter 1")).toBe("Counter 1");
    expect(formatEntityName("Table", "Table 1")).not.toBe("Table Table 1");
  });

  it("matches the label case-insensitively", () => {
    expect(formatEntityName("Table", "table 2")).toBe("table 2");
  });

  it("prepends the label for a bare custom name (keeps orientation)", () => {
    expect(formatEntityName("Table", "Patio A")).toBe("Table Patio A");
  });

  it("PG-29: localizes English seed names instead of leaking English type words", () => {
    // Seed names embed the English type word; a localized UI label ("Mesa")
    // must translate the type, not leave English "Table 9" in the guest header.
    expect(formatEntityName("Mesa", "Table 9")).toBe("Mesa 9");
    expect(formatEntityName("Mostrador", "Counter 3")).toBe("Mostrador 3");
    expect(formatEntityName("Mesa", "Table 9")).not.toBe("Table 9");
    expect(formatEntityName("Mesa", "Table 9")).not.toBe("Mesa Table 9");
  });

  it("both directions stay green together (no antagonistic fix)", () => {
    // English operator + English seed → single English word
    expect(formatEntityName("Table", "Table 1")).toBe("Table 1");
    // Spanish guest + English seed → Spanish type + number
    expect(formatEntityName("Mesa", "Table 1")).toBe("Mesa 1");
    // Spanish guest + already-Spanish name → no double prefix
    expect(formatEntityName("Mesa", "Mesa 1")).toBe("Mesa 1");
  });

  it("still prefixes a bare custom name with a localized label", () => {
    expect(formatEntityName("Mesa", "Patio A")).toBe("Mesa Patio A");
  });

  it("does not treat longer words starting with table/counter as seeds", () => {
    expect(formatEntityName("Mesa", "Tablecloth Corner")).toBe(
      "Mesa Tablecloth Corner",
    );
  });

  it("falls back to the label alone for an empty name", () => {
    expect(formatEntityName("Table", "")).toBe("Table");
    expect(formatEntityName("Table", "   ")).toBe("Table");
  });
});

describe("getBillLocationLabel", () => {
  const labels = { table: "Table", counter: "Counter", delivery: "Delivery" };

  it("uses a named table without double-prefixing", () => {
    expect(getBillLocationLabel({ table: { name: "Table 4" } }, labels)).toBe(
      "Table 4",
    );
    expect(getBillLocationLabel({ table_name: "Table 4" }, labels)).toBe(
      "Table 4",
    );
  });

  it("localizes English seeds when the table label is localized", () => {
    const es = { table: "Mesa", counter: "Mostrador", delivery: "Entrega" };
    expect(getBillLocationLabel({ table: { name: "Table 4" } }, es)).toBe(
      "Mesa 4",
    );
  });

  it("returns the counter label for a counter bill (table_id 0)", () => {
    expect(
      getBillLocationLabel({ table_id: 0, counter_id: 3 }, labels),
    ).toBe("Counter");
  });

  it("returns the delivery label for a tableless, counterless bill", () => {
    expect(getBillLocationLabel({ table_id: 0 }, labels)).toBe("Delivery");
  });

  it("never surfaces a raw Table 0", () => {
    const out = getBillLocationLabel({ table_id: 0, counter_id: 1 }, labels);
    expect(out).not.toMatch(/table\s+0/i);
  });

  it("falls back to table_id when no name is present but a table is set", () => {
    expect(getBillLocationLabel({ table_id: 7 }, labels)).toBe("Table 7");
  });
});
