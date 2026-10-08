/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { InventoryItem } from "@/api/inventory";

const mockUpdateItem = jest.fn().mockResolvedValue({});
const mockCreateItem = jest.fn().mockResolvedValue({});
jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    updateItem: (...a: unknown[]) => mockUpdateItem(...a),
    createItem: (...a: unknown[]) => mockCreateItem(...a),
  },
}));

import InventoryItemModal from "./InventoryItemModal";

const existingItem: InventoryItem = {
  id: 5,
  business_id: 42,
  name: "Tomatoes",
  sku: "TOM-01",
  category: "Produce",
  unit: "kg",
  current_quantity: 22,
  reorder_threshold: 5,
  cost_per_unit: 1.5,
  is_active: true,
  created_at: "",
  updated_at: "",
};

const t = (k: string) => k; // key-echo

function setup(overrides: Partial<React.ComponentProps<typeof InventoryItemModal>> = {}) {
  const onClose = jest.fn();
  const onSaved = jest.fn().mockResolvedValue(undefined);
  render(
    <InventoryItemModal
      isOpen
      onClose={onClose}
      mode="edit"
      initial={existingItem}
      businessId={42}
      categories={["Produce"]}
      onSaved={onSaved}
      t={t}
      {...overrides}
    />,
  );
  return { onClose, onSaved };
}

describe("InventoryItemModal – edit mode dirty-tracking", () => {
  beforeEach(() => {
    mockUpdateItem.mockClear();
    mockCreateItem.mockClear();
  });

  it("omits current_quantity from the update payload when the operator did not touch the quantity field", async () => {
    // Regression: the modal was unconditionally sending the loaded
    // current_quantity on every edit save, overwriting concurrent server-side
    // deductions (e.g., an order was processed between load and save).
    setup();

    // Operator changes only the name — does NOT touch the quantity field.
    const nameInput = screen.getByDisplayValue("Tomatoes");
    fireEvent.change(nameInput, { target: { value: "Cherry Tomatoes" } });

    fireEvent.click(screen.getByText("itemForm.updateButton"));

    await waitFor(() => expect(mockUpdateItem).toHaveBeenCalledTimes(1));

    const [, , payload] = mockUpdateItem.mock.calls[0] as [number, number, Record<string, unknown>];
    expect(payload.current_quantity).toBeUndefined();
    // Other fields are still sent
    expect(payload.name).toBe("Cherry Tomatoes");
  });

  it("sends current_quantity when the operator explicitly changes it", async () => {
    setup();

    const quantityInput = screen.getByDisplayValue("22");
    fireEvent.change(quantityInput, { target: { value: "30" } });

    fireEvent.click(screen.getByText("itemForm.updateButton"));

    await waitFor(() => expect(mockUpdateItem).toHaveBeenCalledTimes(1));

    const [, , payload] = mockUpdateItem.mock.calls[0] as [number, number, Record<string, unknown>];
    expect(payload.current_quantity).toBe(30);
  });
});

describe("InventoryItemModal – L5-38 whitespace name + L5-40 no step bubble", () => {
  beforeEach(() => {
    mockUpdateItem.mockClear();
    mockCreateItem.mockClear();
  });

  it("L5-38: whitespace-only name shows error and blocks save (button still pressable)", async () => {
    setup({ mode: "create", initial: null });
    const name = screen.getByTestId("inventory-item-name");
    fireEvent.change(name, { target: { value: "   " } });
    expect(screen.getByText("errors.itemNameWhitespace")).toBeInTheDocument();
    // Save is not silently disabled solely for whitespace — click surfaces error.
    fireEvent.click(screen.getByText("itemForm.createButton"));
    await waitFor(() => {
      expect(mockCreateItem).not.toHaveBeenCalled();
    });
    expect(
      screen.getAllByText("errors.itemNameWhitespace").length,
    ).toBeGreaterThan(0);
  });

  it("L5-40: quantity fields are type=text (no native step-mismatch bubble)", () => {
    setup();
    expect(screen.getByTestId("inventory-current-quantity")).toHaveAttribute(
      "type",
      "text",
    );
    expect(screen.getByTestId("inventory-reorder-threshold")).toHaveAttribute(
      "type",
      "text",
    );
    expect(screen.getByTestId("inventory-current-quantity")).toHaveAttribute(
      "inputmode",
      "decimal",
    );
  });
});

describe("InventoryItemModal – L5-36 locale cost-per-unit", () => {
  beforeEach(() => {
    mockUpdateItem.mockClear();
    mockCreateItem.mockClear();
    mockUpdateItem.mockResolvedValue({ ...existingItem, cost_per_unit: 12.34 });
  });

  it("submits cost_per_unit 12.34 for comma-decimal input \"12,34\" (not 1234)", async () => {
    setup();

    const costInput = screen.getByTestId("inventory-cost-per-unit");
    fireEvent.change(costInput, { target: { value: "12,34" } });
    fireEvent.click(screen.getByText("itemForm.updateButton"));

    await waitFor(() => expect(mockUpdateItem).toHaveBeenCalledTimes(1));

    const [, , payload] = mockUpdateItem.mock.calls[0] as [
      number,
      number,
      Record<string, unknown>,
    ];
    expect(payload.cost_per_unit).toBe(12.34);
    expect(payload.cost_per_unit).not.toBe(1234);
  });

  it("does not use native type=number on cost (preserves comma paste)", () => {
    setup();
    const costInput = screen.getByTestId("inventory-cost-per-unit");
    expect(costInput).toHaveAttribute("type", "text");
    expect(costInput).toHaveAttribute("inputmode", "decimal");
  });
});

describe("InventoryItemModal – L5-37 unit default + L5-42 edit subtitle", () => {
  beforeEach(() => {
    mockUpdateItem.mockClear();
    mockCreateItem.mockClear();
  });

  it("L5-37: create mode seeds unit from localized itemForm.unitDefault", async () => {
    const tLocal = (k: string) => {
      if (k === "itemForm.unitDefault") return "unidad";
      return k;
    };
    setup({ mode: "create", initial: null, t: tLocal });
    const unit = screen.getByTestId("inventory-item-unit") as HTMLInputElement;
    expect(unit.value).toBe("unidad");
    expect(unit).toHaveAttribute("placeholder", "unidad");
  });

  it("L5-42: edit mode shows editSubtitle, not the create subtitle", () => {
    const tLocal = (k: string) => {
      if (k === "itemForm.editSubtitle") return "Edit this ingredient";
      if (k === "itemForm.subtitle") return "Add a new ingredient";
      return k;
    };
    setup({ mode: "edit", t: tLocal });
    expect(screen.getByText("Edit this ingredient")).toBeInTheDocument();
    expect(screen.queryByText("Add a new ingredient")).not.toBeInTheDocument();
  });
});
