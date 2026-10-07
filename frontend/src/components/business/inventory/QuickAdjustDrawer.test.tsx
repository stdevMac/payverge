/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { InventoryItem } from "@/api/inventory";

const mockCreateAdjustment = jest.fn().mockResolvedValue({});
jest.mock("@/api/inventory", () => ({
  inventoryApi: { createAdjustment: (...a: unknown[]) => mockCreateAdjustment(...a) },
}));

import QuickAdjustDrawer from "./QuickAdjustDrawer";

const item: InventoryItem = {
  id: 7,
  business_id: 42,
  name: "Tomatoes",
  unit: "kg",
  current_quantity: 22,
  reorder_threshold: 5,
  cost_per_unit: 1,
  is_active: true,
  created_at: "",
  updated_at: "",
};

const t = (k: string) => k; // key-echo

function setup() {
  const onClose = jest.fn();
  const onSaved = jest.fn().mockResolvedValue(undefined);
  render(
    <QuickAdjustDrawer
      isOpen
      item={item}
      businessId={42}
      onClose={onClose}
      onSaved={onSaved}
      t={t}
    />,
  );
  return { onClose, onSaved };
}

describe("QuickAdjustDrawer", () => {
  beforeEach(() => mockCreateAdjustment.mockClear());

  it("Receive posts a positive restock delta", async () => {
    const { onSaved } = setup();
    fireEvent.click(screen.getByText("quickAdjust.verbs.receive"));
    fireEvent.change(screen.getByTestId("quick-adjust-quantity"), {
      target: { value: "10" },
    });
    fireEvent.click(screen.getByTestId("quick-adjust-save"));
    await waitFor(() => expect(mockCreateAdjustment).toHaveBeenCalledTimes(1));
    expect(mockCreateAdjustment).toHaveBeenCalledWith(42, {
      inventory_item_id: 7,
      movement_type: "restock",
      quantity_change: 10,
      reason: undefined,
    });
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });

  describe("L5-39 over-waste / empty quantity explain disable", () => {
    it("shows quantityRequired when waste qty is empty", () => {
      setup();
      fireEvent.click(screen.getByText("quickAdjust.verbs.waste"));
      expect(
        screen.getByText("quickAdjust.quantityRequired"),
      ).toBeInTheDocument();
      expect(screen.getByTestId("quick-adjust-save")).toBeDisabled();
      expect(screen.getByTestId("quick-adjust-quantity")).toHaveAttribute(
        "type",
        "text",
      );
    });

    it("shows overWaste when waste exceeds on-hand and does not save", async () => {
      setup();
      fireEvent.click(screen.getByText("quickAdjust.verbs.waste"));
      fireEvent.change(screen.getByTestId("quick-adjust-quantity"), {
        target: { value: "999" },
      });
      // key-echo t leaves {current} placeholder — message still surfaces.
      expect(screen.getByText(/quickAdjust\.overWaste/)).toBeInTheDocument();
      expect(screen.getByTestId("quick-adjust-save")).toBeDisabled();
      fireEvent.click(screen.getByTestId("quick-adjust-save"));
      expect(mockCreateAdjustment).not.toHaveBeenCalled();
    });
  });

  it("Waste posts a negative delta with the STABLE reason enum key (not a translated label)", async () => {
    // R3-AC: the ledger's reason must be locale-independent, so the drawer
    // persists the enum key ("spoilage") and the Activity list translates it at
    // render time — never the operator's translated label.
    setup();
    fireEvent.click(screen.getByText("quickAdjust.verbs.waste"));
    fireEvent.change(screen.getByTestId("quick-adjust-quantity"), {
      target: { value: "3" },
    });
    fireEvent.click(screen.getByText("quickAdjust.reasons.spoilage"));
    fireEvent.click(screen.getByTestId("quick-adjust-save"));
    await waitFor(() => expect(mockCreateAdjustment).toHaveBeenCalledTimes(1));
    expect(mockCreateAdjustment).toHaveBeenCalledWith(42, {
      inventory_item_id: 7,
      movement_type: "waste",
      quantity_change: -3,
      reason: "spoilage",
    });
  });

  it("Count posts the absolute counted value as target_quantity (not a client-side delta)", async () => {
    // INV-M1: a physical count must reconcile to the operator's observed
    // absolute value. The backend computes the delta against the LIVE locked
    // row, so the client sends target_quantity and NO quantity_change — a stale
    // page-load snapshot can no longer corrupt the count.
    setup();
    fireEvent.click(screen.getByText("quickAdjust.verbs.count"));
    fireEvent.change(screen.getByTestId("quick-adjust-counted"), {
      target: { value: "18" },
    });
    fireEvent.click(screen.getByTestId("quick-adjust-save"));
    await waitFor(() => expect(mockCreateAdjustment).toHaveBeenCalledTimes(1));
    const [, payload] = mockCreateAdjustment.mock.calls[0];
    expect(payload).toMatchObject({
      inventory_item_id: 7,
      movement_type: "correction",
      target_quantity: 18,
    });
    expect(payload).not.toHaveProperty("quantity_change");
  });

  it("Count remains savable when the count equals the displayed value (it may still differ from live)", async () => {
    // The displayed current is a page-load snapshot; counted == displayed is
    // NOT necessarily a no-op against the live quantity, so Save must stay
    // enabled and post the absolute target.
    setup();
    fireEvent.click(screen.getByText("quickAdjust.verbs.count"));
    fireEvent.change(screen.getByTestId("quick-adjust-counted"), {
      target: { value: "22" }, // equals item.current_quantity
    });
    expect(screen.getByTestId("quick-adjust-save")).not.toBeDisabled();
    fireEvent.click(screen.getByTestId("quick-adjust-save"));
    await waitFor(() => expect(mockCreateAdjustment).toHaveBeenCalledTimes(1));
    expect(mockCreateAdjustment.mock.calls[0][1]).toMatchObject({
      target_quantity: 22,
    });
  });

  it("disables save when the delta is zero", () => {
    setup();
    fireEvent.click(screen.getByText("quickAdjust.verbs.receive"));
    expect(screen.getByTestId("quick-adjust-save")).toBeDisabled();
  });

  it("Count: clearing the counted field disables save (empty string must not be treated as zero)", () => {
    // Regression: Number("") === 0, so a blank count field was computing
    // delta = 0 - current = -22, which enabled the Save button and would post
    // a phantom correction driving stock to zero.
    setup();
    fireEvent.click(screen.getByText("quickAdjust.verbs.count"));
    // The field is pre-filled with the item's current_quantity; clear it.
    fireEvent.change(screen.getByTestId("quick-adjust-counted"), {
      target: { value: "" },
    });
    expect(screen.getByTestId("quick-adjust-save")).toBeDisabled();
  });
});
