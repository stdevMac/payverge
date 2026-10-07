/** @jest-environment jsdom */
/**
 * Edit Item layout — Name/Price/Availability first; AI + tags on Details.
 * Related to issue 117.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

jest.mock("../../MultipleImageUpload", () => ({
  __esModule: true,
  default: () => <div data-testid="edit-item-photos" />,
}));
jest.mock("../../TagSelector", () => ({
  __esModule: true,
  default: ({ type }: { type: string }) => (
    <div data-testid={`edit-item-tags-${type}`} />
  ),
}));
jest.mock("../../../../api/currency", () => ({
  formatCurrency: (n: number) => `$${(n || 0).toFixed(2)}`,
}));

import EditItemModal from "../EditItemModal";

const baseProps = {
  isOpen: true,
  onOpenChange: jest.fn(),
  selectedCategoryIndex: 0,
  menu: [{ id: "c1", name: "Mains", description: "", items: [] }],
  itemName: "Steak Plate",
  setItemName: jest.fn(),
  itemDescription: "Grilled",
  setItemDescription: jest.fn(),
  itemPrice: "12.00",
  setItemPrice: jest.fn(),
  itemCogs: "3.50",
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
  itemAllergens: [] as string[],
  newAllergen: "",
  setNewAllergen: jest.fn(),
  onAddAllergen: jest.fn(),
  onRemoveAllergen: jest.fn(),
  itemDietaryTags: [] as string[],
  newDietaryTag: "",
  setNewDietaryTag: jest.fn(),
  onAddDietaryTag: jest.fn(),
  onRemoveDietaryTag: jest.fn(),
  onUpdateItem: jest.fn(),
  onResetForm: jest.fn(),
  tString: (k: string) => k,
  onGeneratePhoto: async () => null,
  onGenerateBreakdown: async () => null,
  onEnhancePhoto: async () => null,
};

describe("EditItemModal tab order (#117)", () => {
  it("keeps AI image tools off the first tab so a price change is not a 12-block wall", () => {
    render(<EditItemModal {...baseProps} />);

    expect(screen.getByTestId("edit-item-basics-panel")).toBeInTheDocument();
    expect(screen.getByTestId("edit-item-price")).toBeInTheDocument();
    expect(screen.getByText("items.available")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Steak Plate")).toBeInTheDocument();
    expect(screen.queryByTestId("edit-item-ai-tools")).not.toBeInTheDocument();
    expect(screen.queryByTestId("edit-item-tags-allergens")).not.toBeInTheDocument();
    expect(screen.queryByText("items.aiImageTools.title")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("edit-item-section-details"));

    expect(screen.getByTestId("edit-item-ai-tools")).toBeInTheDocument();
    expect(screen.getByText("items.aiImageTools.title")).toBeInTheDocument();
    expect(screen.getByTestId("edit-item-tags-allergens")).toBeInTheDocument();
    expect(screen.getByTestId("edit-item-cogs")).toBeInTheDocument();
  });
});
