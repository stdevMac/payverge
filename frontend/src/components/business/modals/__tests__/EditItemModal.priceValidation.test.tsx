/** @jest-environment jsdom */
/**
 * L3-5: EditItemModal must match AddItemModal price/COGS input shape —
 * type=text inputMode=decimal (no native English HTML5 number bubbles) +
 * S-5 inline invalidPrice for invalid price.
 *
 * #117: COGS lives under Photos & tags; tests that need it open that section.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

jest.mock("../../MultipleImageUpload", () => ({ __esModule: true, default: () => null }));
jest.mock("../../MenuBuilder/components/AIImageTools", () => ({ AIImageTools: () => null }));
jest.mock("../../TagSelector", () => ({ __esModule: true, default: () => null }));
jest.mock("../../../../api/currency", () => ({
  formatCurrency: (n: number) => `$${(n || 0).toFixed(2)}`,
}));

import EditItemModal from "../EditItemModal";

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
  onUpdateItem: jest.fn(),
  onResetForm: jest.fn(),
  tString: (k: string) => k,
};

function saveButton() {
  return screen
    .getByText("buttons.updateItem")
    .closest("button") as HTMLButtonElement;
}

function openDetails() {
  fireEvent.click(screen.getByTestId("edit-item-section-details"));
}

describe("L3-5 EditItemModal locale decimal inputs (parity with AddItemModal)", () => {
  it("uses type=text inputMode=decimal on price (not type=number)", () => {
    render(<EditItemModal {...baseProps} itemPrice="10" />);
    const price = screen.getByTestId("edit-item-price");
    expect(price).toHaveAttribute("type", "text");
    expect(price).toHaveAttribute("inputmode", "decimal");
    expect(price).not.toHaveAttribute("step");
    expect(price).not.toHaveAttribute("min");
  });

  it("uses type=text inputMode=decimal on COGS (not type=number)", () => {
    render(<EditItemModal {...baseProps} itemCogs="2.50" />);
    openDetails();
    const cogs = screen.getByTestId("edit-item-cogs");
    expect(cogs).toHaveAttribute("type", "text");
    expect(cogs).toHaveAttribute("inputmode", "decimal");
    expect(cogs).not.toHaveAttribute("step");
    expect(cogs).not.toHaveAttribute("min");
  });

  it("shows localized invalidPrice via isInvalid for negative typed price", () => {
    render(<EditItemModal {...baseProps} itemPrice="-3" />);
    expect(screen.getByText("validation.invalidPrice")).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();
  });

  it("enables Save for a valid non-negative price", () => {
    render(<EditItemModal {...baseProps} itemPrice="12.50" itemName="Burger" />);
    expect(saveButton()).not.toBeDisabled();
  });
});

describe("Edit Item progressive disclosure (#117 / #130 / #150)", () => {
  it("opens on Basics with name/price/availability; buries COGS + AI/tags", () => {
    render(
      <EditItemModal
        {...baseProps}
        itemCogs="3.50"
        onGeneratePhoto={async () => null}
      />,
    );
    expect(screen.getByTestId("edit-item-basics-panel")).toBeInTheDocument();
    expect(screen.getByTestId("edit-item-price")).toBeInTheDocument();
    expect(screen.getByText("items.available")).toBeInTheDocument();
    expect(screen.queryByTestId("edit-item-cogs")).not.toBeInTheDocument();
    expect(screen.queryByTestId("edit-item-details-panel")).not.toBeInTheDocument();
    expect(screen.queryByTestId("edit-item-ai-tools")).not.toBeInTheDocument();
  });

  it("shows full-width food-cost helper on Details (not clipped under a 160px column)", () => {
    const tString = (key: string) =>
      (
        ({
          "items.itemCogs": "Food cost (per plate)",
          "items.itemCogsHint":
            "Optional plate cost used when no Inventory recipe is linked. A recipe cost wins when both are set.",
          "items.editSectionDetails": "Photos & tags",
        }) as Record<string, string>
      )[key] ?? key;
    render(<EditItemModal {...baseProps} itemCogs="3.50" tString={tString} />);
    openDetails();
    expect(screen.getByTestId("edit-item-details-panel")).toBeInTheDocument();
    expect(screen.getByTestId("edit-item-cogs")).toBeInTheDocument();
    expect(screen.queryByText("COGS")).not.toBeInTheDocument();
    expect(screen.getByText("Food cost (per plate)")).toBeInTheDocument();
    const hint = screen.getByTestId("edit-item-cogs-hint");
    expect(hint).toHaveTextContent(
      "Optional plate cost used when no Inventory recipe is linked. A recipe cost wins when both are set.",
    );
    // Hint is a sibling paragraph, not NextUI description crushed under the input.
    expect(hint.tagName).toBe("P");
    expect(hint.className).not.toMatch(/truncate|line-clamp|overflow-hidden/);
  });

  it("formats Price and Food cost with the same two decimals (#184)", () => {
    render(
      <EditItemModal {...baseProps} itemPrice="12.00" itemCogs="0.00" />,
    );
    expect(screen.getByTestId("edit-item-price")).toHaveValue("12.00");
    openDetails();
    expect(screen.getByTestId("edit-item-cogs")).toHaveValue("0.00");
  });
});
