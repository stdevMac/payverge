import { physicalItemQuantity, toFulfillmentLines } from "./orderFulfillment";
import type { OrderItem } from "@/api/orders";
import { asDollars } from "@/types/money";

const item = (overrides: Partial<OrderItem>): OrderItem => ({
  id: "1", menu_item_name: "Burger", quantity: 1, price: asDollars(5),
  options: [], special_requests: "", subtotal: asDollars(5), ...overrides,
});

it("keeps only physical menu and bundle component rows for prep lines", () => {
  const lines = toFulfillmentLines([
    item({ id: "10", menu_item_id: "10", quantity: 2, item_type: "menu_item" }),
    item({ id: "11", menu_item_id: "bundle", item_type: "bundle", menu_item_name: "Combo" }),
    item({ id: "12", menu_item_id: "12", quantity: 2, item_type: "bundle_item", menu_item_name: "Fries" }),
    item({ id: "13", item_type: "discount", menu_item_name: "Promo" }),
  ]);
  expect(lines.map(({ key, menu_item_name: name, quantity, special_requests: notes }) => ({ key, name, quantity, notes }))).toEqual([
    { key: "menu_item:10", name: "Burger", quantity: 2, notes: "" },
    { key: "bundle_item:12", name: "Fries", quantity: 2, notes: "" },
  ]);
});

it("counts sellable units (menu_item + bundle), not expanded bundle children", () => {
  const lines = [
    item({ id: "10", menu_item_id: "10", quantity: 2, item_type: "menu_item" }),
    item({ id: "11", menu_item_id: "bundle", quantity: 1, item_type: "bundle", menu_item_name: "Combo" }),
    item({ id: "12", menu_item_id: "12", quantity: 2, item_type: "bundle_item", menu_item_name: "Fries" }),
    item({ id: "13", item_type: "discount", menu_item_name: "Promo" }),
  ];
  // 2 burgers + 1 combo — fries are components of the combo, not extra covers.
  expect(physicalItemQuantity(lines)).toBe(3);
});

it("does not inflate 4× Date Night into 16 component plates", () => {
  const lines = [
    item({
      id: "bundle",
      menu_item_id: "date-night",
      menu_item_name: "Date Night",
      quantity: 4,
      item_type: "bundle",
    }),
    item({ id: "steak", menu_item_name: "Steak", quantity: 4, item_type: "bundle_item" }),
    item({ id: "wine", menu_item_name: "Wine", quantity: 4, item_type: "bundle_item" }),
    item({ id: "salad", menu_item_name: "Salad", quantity: 4, item_type: "bundle_item" }),
    item({ id: "dessert", menu_item_name: "Dessert", quantity: 4, item_type: "bundle_item" }),
  ];
  expect(physicalItemQuantity(lines)).toBe(4);
  expect(
    toFulfillmentLines(lines).reduce((sum, line) => sum + line.quantity, 0),
  ).toBe(16);
});

it("parses a legacy string snapshot the same as an array (#771)", () => {
  const lines = [
    item({ id: "10", menu_item_id: "10", quantity: 2, item_type: "menu_item" }),
  ];
  expect(toFulfillmentLines(JSON.stringify(lines))).toHaveLength(1);
  expect(physicalItemQuantity(JSON.stringify(lines))).toBe(2);
  expect(physicalItemQuantity(undefined)).toBe(0);
});

it("combines identical prep lines but preserves different requests", () => {
  const lines = toFulfillmentLines([
    item({ menu_item_id: "10", quantity: 1 }),
    item({ menu_item_id: "10", quantity: 2 }),
    item({ menu_item_id: "10", quantity: 1, special_requests: "no salt" }),
  ]);
  expect(lines.map((line) => line.quantity)).toEqual([3, 1]);
});
