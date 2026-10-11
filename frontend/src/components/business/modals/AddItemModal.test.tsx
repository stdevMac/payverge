/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import AddItemModal from "./AddItemModal";

// Passthrough translator: returns the i18n key verbatim so we can assert the
// component wires up the new helper keys (matches the repo's modal-test convention).
const tString = (key: string) => key;

function renderModal(overrides: Partial<React.ComponentProps<typeof AddItemModal>> = {}) {
  return render(
    <AddItemModal
      isOpen
      onOpenChange={jest.fn()}
      selectedCategoryIndex={null}
      menu={[]}
      itemName=""
      setItemName={jest.fn()}
      itemDescription=""
      setItemDescription={jest.fn()}
      itemPrice=""
      setItemPrice={jest.fn()}
      itemCogs=""
      setItemCogs={jest.fn()}
      defaultCurrency="USD"
      itemImages={[]}
      setItemImages={jest.fn()}
      itemAvailable
      setItemAvailable={jest.fn()}
      itemSortOrder={0}
      setItemSortOrder={jest.fn()}
      businessId={1}
      itemOptions={[]}
      newOptionName=""
      setNewOptionName={jest.fn()}
      newOptionPrice=""
      setNewOptionPrice={jest.fn()}
      onAddOption={jest.fn()}
      onRemoveOption={jest.fn()}
      itemAllergens={[]}
      newAllergen=""
      setNewAllergen={jest.fn()}
      onAddAllergen={jest.fn()}
      onRemoveAllergen={jest.fn()}
      itemDietaryTags={[]}
      newDietaryTag=""
      setNewDietaryTag={jest.fn()}
      onAddDietaryTag={jest.fn()}
      onRemoveDietaryTag={jest.fn()}
      onAddItem={jest.fn()}
      onResetForm={jest.fn()}
      tString={tString}
      {...overrides}
    />
  );
}

const addOptionButton = () =>
  screen.getByRole("button", { name: "items.addOptionShort" });

describe("AddItemModal helper guidance", () => {
  it("explains what each section is for (options, allergens, dietary tags)", () => {
    renderModal();
    // Each bare section now carries a one-line explainer keyed under items.*Help.
    expect(screen.getByText("items.optionsHelp")).toBeInTheDocument();
    expect(screen.getByText("items.allergensHelp")).toBeInTheDocument();
    expect(screen.getByText("items.dietaryTagsHelp")).toBeInTheDocument();
  });
});

describe("AddItemModal option-price validation (F7)", () => {
  it("disables Add Option when the price is missing", () => {
    renderModal({ newOptionName: "Large", newOptionPrice: "" });
    expect(addOptionButton()).toBeDisabled();
  });

  it("keeps Add Option disabled for a negative price and surfaces feedback", () => {
    renderModal({ newOptionName: "Large", newOptionPrice: "-5" });
    expect(addOptionButton()).toBeDisabled();
    // Operator now gets a localized reason instead of a silent no-op.
    expect(screen.getByText("validation.invalidPrice")).toBeInTheDocument();
  });

  it("enables Add Option for a valid comma-decimal price (5,50 -> 5.5)", () => {
    renderModal({ newOptionName: "Large", newOptionPrice: "5,50" });
    expect(addOptionButton()).toBeEnabled();
    // A valid value must not trip the inline error.
    expect(screen.queryByText("validation.invalidPrice")).not.toBeInTheDocument();
  });

  it("enables Add Option for a plain dot-decimal price", () => {
    renderModal({ newOptionName: "Large", newOptionPrice: "3.00" });
    expect(addOptionButton()).toBeEnabled();
  });

  it("does not show the inline error on a pristine (empty) price field", () => {
    renderModal({ newOptionName: "Large", newOptionPrice: "" });
    expect(screen.queryByText("validation.invalidPrice")).not.toBeInTheDocument();
  });
});
