/** @jest-environment jsdom */
/**
 * L3-2: garbage price must NOT enable Create (parseLocaleDecimal returned 0
 * for "abc" so priceValid was true and a $0.00 item saved).
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("../../MultipleImageUpload", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../MenuBuilder/components/AIImageTools", () => ({
  AIImageTools: () => null,
}));
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

describe("L3-2 garbage price rejection", () => {
  it("disables Create for alphabetic garbage (abc)", () => {
    render(<AddItemModal {...baseProps} itemPrice="abc" />);
    expect(createButton()).toBeDisabled();
    expect(screen.getByText("validation.invalidPrice")).toBeInTheDocument();
  });

  it("disables Create for currency-code prefix garbage (USD12.50)", () => {
    render(<AddItemModal {...baseProps} itemPrice="USD12.50" />);
    expect(createButton()).toBeDisabled();
  });

  it("enables Create for a real locale decimal", () => {
    render(<AddItemModal {...baseProps} itemPrice="12,50" />);
    expect(createButton()).not.toBeDisabled();
  });
});
