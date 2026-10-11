import { parseBillItems } from "@/api/bills";
import { parseOrderItems } from "@/api/orders";

describe("parseBillItems (#771)", () => {
  it("returns a line array as-is", () => {
    const lines = [{ name: "Steak", quantity: 1 }];
    expect(parseBillItems(lines)).toEqual(lines);
  });

  it("parses a legacy JSON string into lines", () => {
    expect(parseBillItems('[{"name":"Wine","quantity":1}]')).toEqual([
      { name: "Wine", quantity: 1 },
    ]);
  });

  it("does not treat string length as the item count", () => {
    const snapshot = '[{"name":"Steak"},{"name":"Wine"}]';
    expect(snapshot.length).toBeGreaterThan(2);
    expect(parseBillItems(snapshot)).toHaveLength(2);
  });

  it("returns [] for omitted, empty, or invalid snapshots", () => {
    expect(parseBillItems(undefined)).toEqual([]);
    expect(parseBillItems("")).toEqual([]);
    expect(parseBillItems("[]")).toEqual([]);
    expect(parseBillItems("not-json")).toEqual([]);
    expect(parseBillItems({ name: "nope" })).toEqual([]);
  });
});

describe("parseOrderItems (#771)", () => {
  it("returns a line array as-is", () => {
    const lines = [{ menu_item_name: "Steak", quantity: 1 }];
    expect(parseOrderItems(lines)).toEqual(lines);
  });

  it("parses a legacy JSON string into lines", () => {
    expect(parseOrderItems('[{"menu_item_name":"Soup","quantity":2}]')).toEqual([
      { menu_item_name: "Soup", quantity: 2 },
    ]);
  });

  it("returns [] for omitted or invalid snapshots", () => {
    expect(parseOrderItems(undefined)).toEqual([]);
    expect(parseOrderItems("")).toEqual([]);
    expect(parseOrderItems("not-json")).toEqual([]);
  });
});
