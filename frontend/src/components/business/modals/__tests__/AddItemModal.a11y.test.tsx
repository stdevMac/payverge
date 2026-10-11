/** @jest-environment jsdom */
/**
 * #389 — Add Item dialog and core fields must expose accessible names.
 *
 * Production Chrome a11y snapshot: unnamed dialog + unnamed name/price/
 * description/cost/option textboxes. Edit Item is the labeled positive control.
 *
 * Assert the real dialog by role/name (not data-testid or source greps).
 */
import React, { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

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

import type { MenuItemOption } from "@/api/business";
import AddItemModal from "../AddItemModal";
import en from "@/i18n/messages/en/businessDashboard.json";

const copy = en.dashboard.menuBuilder;
const tString = (key: string) => {
  const [ns, ...rest] = key.split(".");
  const leaf = rest.join(".");
  if (ns === "items") {
    const value = (copy.items as Record<string, unknown>)[leaf];
    if (typeof value === "string") return value;
  }
  if (ns === "buttons") {
    const value = (copy.buttons as Record<string, unknown>)[leaf];
    if (typeof value === "string") return value;
  }
  if (ns === "validation" && leaf === "invalidPrice") {
    return copy.validation.invalidPrice;
  }
  return key;
};

/** Visible label, or NextUI's aria-label+label concatenation of the same words. */
function namedTextbox(name: string, root: HTMLElement = document.body) {
  return within(root).getByRole("textbox", {
    name: (accessible) =>
      accessible === name || accessible === `${name} ${name}`,
  });
}

type Props = React.ComponentProps<typeof AddItemModal>;

function baseProps(overrides: Partial<Props> = {}): Props {
  return {
    isOpen: true,
    onOpenChange: jest.fn(),
    selectedCategoryIndex: 0,
    menu: [{ id: "c1", name: "Mains", description: "", items: [] }],
    itemName: "",
    setItemName: jest.fn(),
    itemDescription: "",
    setItemDescription: jest.fn(),
    itemPrice: "",
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
    itemOptions: [] as MenuItemOption[],
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
    tString,
    ...overrides,
  };
}

function namedDialog() {
  return screen.getByRole("dialog", { name: copy.items.addItem });
}

function Harness({ extras = {} }: { extras?: Partial<Props> }) {
  const [isOpen, setIsOpen] = useState(true);
  return (
    <>
      <button type="button" onClick={() => setIsOpen(true)}>
        reopen-add-item
      </button>
      <AddItemModal
        {...baseProps({
          isOpen,
          onOpenChange: () => setIsOpen((open) => !open),
          ...extras,
        })}
      />
    </>
  );
}

function expectLabeledTextbox(field: HTMLElement, visibleLabel: string) {
  const id = field.getAttribute("id");
  expect(id).toBeTruthy();
  const htmlLabel = document.querySelector(`label[for="${id}"]`);
  const ariaLabel = field.getAttribute("aria-label");
  expect(
    htmlLabel?.textContent?.includes(visibleLabel) || ariaLabel === visibleLabel,
  ).toBe(true);
}

describe("AddItemModal accessible names (#389)", () => {
  it("names the dialog Add Item and labels every core field", () => {
    render(<AddItemModal {...baseProps()} />);

    const dialog = namedDialog();
    const labelledBy = dialog.getAttribute("aria-labelledby");
    expect(labelledBy).toBeTruthy();
    const title = document.getElementById(labelledBy!);
    expect(title).toBeTruthy();
    expect(title).toHaveTextContent(copy.items.addItem);

    const name = namedTextbox(copy.items.itemName, dialog);
    const price = namedTextbox(copy.items.itemPrice, dialog);
    const description = namedTextbox(copy.items.itemDescription, dialog);
    const cogs = namedTextbox(copy.items.itemCogs, dialog);
    const optionName = namedTextbox(copy.items.optionName, dialog);
    const optionPrice = namedTextbox(copy.items.optionPrice, dialog);

    expect(name).toBeRequired();
    expect(price).toBeRequired();
    expectLabeledTextbox(name, copy.items.itemName);
    expectLabeledTextbox(price, copy.items.itemPrice);
    expectLabeledTextbox(description, copy.items.itemDescription);
    expectLabeledTextbox(cogs, copy.items.itemCogs);
    expectLabeledTextbox(optionName, copy.items.optionName);
    expectLabeledTextbox(optionPrice, copy.items.optionPrice);
  });

  it("announces invalid price on the labeled Price field", () => {
    render(<AddItemModal {...baseProps({ itemPrice: "-5" })} />);

    const price = namedTextbox(copy.items.itemPrice);
    expect(price).toHaveAttribute("aria-invalid", "true");
    expect(price).toHaveAccessibleDescription(/valid price/i);
  });

  it("keeps the dialog and field names after the keyed remount", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    expect(namedDialog()).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^Close$/i }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "reopen-add-item" }));
    const dialog = namedDialog();
    expect(namedTextbox(copy.items.itemName, dialog)).toBeInTheDocument();
    expect(namedTextbox(copy.items.itemPrice, dialog)).toBeInTheDocument();
    const labelledBy = dialog.getAttribute("aria-labelledby");
    expect(document.getElementById(labelledBy!)).toHaveTextContent(
      copy.items.addItem,
    );
  });
});
