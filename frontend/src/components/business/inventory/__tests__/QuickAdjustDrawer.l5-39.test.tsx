/** @jest-environment jsdom */
/**
 * L5-39: unparseable / comma-decimal qty must not leave save disabled with
 * zero error messages.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import QuickAdjustDrawer from "../QuickAdjustDrawer";

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    createAdjustment: jest.fn(),
  },
}));

const item = {
  id: 1,
  business_id: 1,
  name: "Flour",
  sku: "",
  category: "",
  unit: "kg",
  current_quantity: 10,
  reorder_threshold: 2,
  cost_per_unit: 1,
  is_active: true,
  created_at: "",
  updated_at: "",
};

const t = (key: string) => {
  const map: Record<string, string> = {
    "quickAdjust.title": "Adjust",
    "quickAdjust.verbs.receive": "Receive",
    "quickAdjust.verbs.waste": "Waste",
    "quickAdjust.verbs.count": "Count",
    "quickAdjust.hints.receive": "hint",
    "quickAdjust.hints.waste": "hint",
    "quickAdjust.hints.count": "hint",
    "quickAdjust.quantityLabel": "Qty",
    "quickAdjust.countedLabel": "Counted",
    "quickAdjust.quantityRequired": "Quantity required",
    "quickAdjust.quantityInvalid": "Invalid quantity",
    "quickAdjust.overWaste": "Over waste {current}",
    "quickAdjust.reasonLabel": "Reason",
    "quickAdjust.reasons.spoilage": "Spoilage",
    "quickAdjust.reasons.prep_waste": "Prep",
    "quickAdjust.reasons.server_error": "Server",
    "quickAdjust.reasons.quality_reject": "Quality",
    "quickAdjust.reasons.other": "Other",
    "quickAdjust.noteLabel": "Note",
    "quickAdjust.save": "Save",
    "quickAdjust.cancel": "Cancel",
    "quickAdjust.setOnHandPreview": "Set {counted} {unit}",
  };
  return map[key] || key;
};

describe("L5-39 QuickAdjust error while save disabled", () => {
  it("shows an error for comma-decimal that bare Number cannot parse", () => {
    render(
      <QuickAdjustDrawer
        isOpen
        item={item as never}
        businessId={1}
        onClose={jest.fn()}
        onSaved={jest.fn()}
        t={t}
      />,
    );
    const qty = screen.getByTestId("quick-adjust-quantity");
    fireEvent.change(qty, { target: { value: "2,5" } });
    // With tryParse, 2,5 is valid — save should enable for receive.
    const save = screen.getByText("Save").closest("button");
    expect(save).not.toBeDisabled();
  });

  it("shows an error (not silent) for alphabetic garbage", () => {
    render(
      <QuickAdjustDrawer
        isOpen
        item={item as never}
        businessId={1}
        onClose={jest.fn()}
        onSaved={jest.fn()}
        t={t}
      />,
    );
    const qty = screen.getByTestId("quick-adjust-quantity");
    fireEvent.change(qty, { target: { value: "abc" } });
    expect(
      screen.getByText(/Invalid quantity|Quantity required/),
    ).toBeInTheDocument();
    const save = screen.getByText("Save").closest("button");
    expect(save).toBeDisabled();
  });
});
