/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("../../MultipleImageUpload", () => ({ __esModule: true, default: () => null }));
jest.mock("../../MenuBuilder/components/AIImageTools", () => ({ AIImageTools: () => null }));
jest.mock("../../TagSelector", () => ({ __esModule: true, default: () => null }));
jest.mock("../../../../api/currency", () => ({
  formatCurrency: (n: number) => `$${(n || 0).toFixed(2)}`,
}));

import AddItemModal from "../AddItemModal";

const baseProps = {
  isOpen: true,
  onOpenChange: jest.fn(),
  selectedCategoryIndex: 0,
  menu: [{ id: "c1", name: "Mains", description: "", items: [] }],
  itemName: "Burger",
  setItemName: jest.fn(),
  itemDescription: "",
  setItemDescription: jest.fn(),
  itemPrice: "10",
  setItemPrice: jest.fn(),
  itemCogs: "",
  setItemCogs: jest.fn(),
  defaultCurrency: "USD",
  itemImages: [],
  setItemImages: jest.fn(),
  itemAvailable: true,
  setItemAvailable: jest.fn(),
  itemSortOrder: 0,
  setItemSortOrder: jest.fn(),
  businessId: 1,
  itemOptions: [],
  newOptionName: "",
  setNewOptionName: jest.fn(),
  newOptionPrice: "",
  setNewOptionPrice: jest.fn(),
  onAddOption: jest.fn(),
  onRemoveOption: jest.fn(),
  itemAllergens: [],
  newAllergen: "",
  setNewAllergen: jest.fn(),
  onAddAllergen: jest.fn(),
  onRemoveAllergen: jest.fn(),
  itemDietaryTags: [],
  newDietaryTag: "",
  setNewDietaryTag: jest.fn(),
  onAddDietaryTag: jest.fn(),
  onRemoveDietaryTag: jest.fn(),
  onAddItem: jest.fn(),
  onResetForm: jest.fn(),
  tString: (k: string) => k,
};

function createButton() {
  return screen
    .getByText("buttons.createItem")
    .closest("button") as HTMLButtonElement;
}

it("disables Create when the price is negative (F24)", () => {
  render(<AddItemModal {...baseProps} itemPrice="-5" />);
  expect(createButton()).toBeDisabled();
});

it("disables Create when the price is empty", () => {
  render(<AddItemModal {...baseProps} itemPrice="" />);
  expect(createButton()).toBeDisabled();
});

it("enables Create for a valid non-negative price", () => {
  render(<AddItemModal {...baseProps} itemPrice="12.50" />);
  expect(createButton()).not.toBeDisabled();
});

it("allows a zero price (free item)", () => {
  render(<AddItemModal {...baseProps} itemPrice="0" />);
  expect(createButton()).not.toBeDisabled();
});

describe("L3-2 no native English HTML5 number bubbles", () => {
  it("uses type=text inputMode=decimal on price (controlled, not type=number)", () => {
    render(<AddItemModal {...baseProps} itemPrice="10" />);
    const price = screen.getByTestId("add-item-price");
    expect(price).toHaveAttribute("type", "text");
    expect(price).toHaveAttribute("inputmode", "decimal");
    expect(price).not.toHaveAttribute("step");
    expect(price).not.toHaveAttribute("min");
  });

  it("shows localized invalidPrice via isInvalid for negative typed price", () => {
    render(<AddItemModal {...baseProps} itemPrice="-3" />);
    expect(screen.getByText("validation.invalidPrice")).toBeInTheDocument();
    expect(createButton()).toBeDisabled();
  });
});

describe("A3b COGS optional decimal with visible error", () => {
  it("empty COGS keeps Create enabled when price is valid", () => {
    render(<AddItemModal {...baseProps} itemPrice="10" itemCogs="" />);
    expect(createButton()).not.toBeDisabled();
  });

  it("garbage COGS shows invalidPrice and disables Create", () => {
    render(<AddItemModal {...baseProps} itemPrice="10" itemCogs="abc" />);
    const cogs = screen.getByTestId("add-item-cogs");
    expect(cogs).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("validation.invalidPrice")).toBeInTheDocument();
    expect(createButton()).toBeDisabled();
  });

  it("negative COGS disables Create with visible error", () => {
    render(<AddItemModal {...baseProps} itemPrice="10" itemCogs="-1" />);
    expect(screen.getByTestId("add-item-cogs")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(createButton()).toBeDisabled();
  });
});
