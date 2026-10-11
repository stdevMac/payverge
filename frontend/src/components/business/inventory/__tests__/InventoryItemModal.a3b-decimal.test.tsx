/**
 * A3b: inventory qty/cost fields keep intermediate decimals and show invalid
 * for negatives/garbage (not silent clamp on change).
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import InventoryItemModal from "../InventoryItemModal";

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    createItem: jest.fn(),
    updateItem: jest.fn(),
  },
}));

const t = (key: string) => {
  const map: Record<string, string> = {
    "itemForm.fields.currentQuantity": "Quantity",
    "itemForm.fields.reorderThreshold": "Reorder",
    "itemForm.fields.costPerUnit": "Cost",
    "itemForm.fields.sku": "SKU",
    "itemForm.fields.unit": "Unit",
    "itemForm.fields.category": "Category",
    "itemForm.unitDefault": "unit",
    "itemForm.active.title": "Active",
    "itemForm.active.description": "desc",
    "itemForm.cancel": "Cancel",
    "itemForm.updateButton": "Update",
    "itemForm.createButton": "Create",
    "itemForm.fields.name": "Name",
    "categoryAutocomplete.placeholder": "Cat",
    "errors.invalidCurrentQuantity": "Qty invalid",
    "errors.invalidReorderThreshold": "Reorder invalid",
    "errors.invalidCostPerUnit": "Cost invalid",
    "errors.invalidNumber": "Bad number",
    "errors.itemNameRequired": "Name required",
    "errors.itemNameWhitespace": "Name whitespace",
  };
  return map[key] || key;
};

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

describe("A3b InventoryItemModal DecimalInput fields", () => {
  it("keeps 2,5 while typing cost and clamps negative on blur", async () => {
    render(
      <InventoryItemModal
        isOpen
        onClose={jest.fn()}
        mode="create"
        initial={null}
        businessId={1}
        categories={[]}
        onSaved={jest.fn()}
        t={t}
      />,
    );
    const cost = await screen.findByTestId("inventory-cost-per-unit");
    fireEvent.change(cost, { target: { value: "" } });
    typeChars(cost, "2");
    typeChars(cost, ",");
    typeChars(cost, "5");
    expect((cost as HTMLInputElement).value).toBe("2,5");

    fireEvent.change(cost, { target: { value: "" } });
    typeChars(cost, "-3");
    await waitFor(() => {
      expect(cost).toHaveAttribute("aria-invalid", "true");
    });
    fireEvent.blur(cost);
    await waitFor(() => {
      expect((cost as HTMLInputElement).value).toBe("0");
    });
  });

  it("quantity rejects garbage with aria-invalid", async () => {
    render(
      <InventoryItemModal
        isOpen
        onClose={jest.fn()}
        mode="create"
        initial={null}
        businessId={1}
        categories={[]}
        onSaved={jest.fn()}
        t={t}
      />,
    );
    const qty = await screen.findByTestId("inventory-current-quantity");
    fireEvent.change(qty, { target: { value: "" } });
    typeChars(qty, "abc");
    await waitFor(() => {
      expect(qty).toHaveAttribute("aria-invalid", "true");
    });
    expect((qty as HTMLInputElement).value).toBe("abc");
  });
});
