/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const mockAdjustBillItem = jest.fn();
const mockGetMenu = jest.fn();
const mockToastError = jest.fn();
const mockToastSuccess = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: (...a: unknown[]) => mockToastError(...a),
  },
}));

jest.mock("@/api/bills", () => ({
  adjustBillItem: (...a: unknown[]) => mockAdjustBillItem(...a),
}));

jest.mock("@/api/business", () => ({
  getMenu: (...a: unknown[]) => mockGetMenu(...a),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

// withManagerPin transparently calls the action; no PIN gate in this test.
jest.mock("../managerPin", () => ({
  useWithManagerPin: () => ({
    withManagerPin: (fn: (pin?: string) => Promise<unknown>) => fn(undefined),
  }),
}));

// ItemCustomizer is irrelevant here.
jest.mock("../ItemCustomizer", () => ({
  ItemCustomizer: () => null,
}));

import { BillItemEditor } from "../BillItemEditor";

const tString = (k: string) => k;

const baseItems = [
  {
    id: "item-1",
    name: "Burger",
    price: 10,
    quantity: 1,
    subtotal: 10,
    item_type: "menu_item",
  },
];

beforeEach(() => {
  jest.clearAllMocks();
  mockGetMenu.mockResolvedValue({ categories: [] });
});

it("keeps the void-reason modal open (preserving the reason) when the void fails", async () => {
  mockAdjustBillItem.mockRejectedValue(new Error("server boom"));
  const onItemsChanged = jest.fn();

  render(
    <BillItemEditor
      billId={99}
      businessId={42}
      currentItems={baseItems as never}
      onItemsChanged={onItemsChanged}
      tString={tString}
      currency="USD"
    />,
  );

  // Open the void prompt for the burger.
  const voidBtn = await screen.findByText("editItems.voidItem");
  fireEvent.click(voidBtn);

  // Type a reason.
  const reason = await screen.findByLabelText("editItems.voidReasonLabel");
  fireEvent.change(reason, { target: { value: "wrong order" } });

  // Confirm the void (which will reject).
  const confirm = await screen.findByText("editItems.voidItemConfirm");
  fireEvent.click(confirm);

  await waitFor(() => expect(mockAdjustBillItem).toHaveBeenCalled());
  await waitFor(() =>
    expect(mockToastError).toHaveBeenCalledWith("editItems.itemVoidFailed"),
  );

  // The modal must remain open with the typed reason intact (not discarded).
  await waitFor(() => {
    const stillThere = screen.getByLabelText(
      "editItems.voidReasonLabel",
    ) as HTMLTextAreaElement;
    expect(stillThere.value).toBe("wrong order");
  });
  // No success and no item-change callback fired.
  expect(mockToastSuccess).not.toHaveBeenCalled();
  expect(onItemsChanged).not.toHaveBeenCalled();
});

it("closes the void-reason modal on a successful void", async () => {
  mockAdjustBillItem.mockResolvedValue({});
  const onItemsChanged = jest.fn();

  render(
    <BillItemEditor
      billId={99}
      businessId={42}
      currentItems={baseItems as never}
      onItemsChanged={onItemsChanged}
      tString={tString}
      currency="USD"
    />,
  );

  fireEvent.click(await screen.findByText("editItems.voidItem"));
  const reason = await screen.findByLabelText("editItems.voidReasonLabel");
  fireEvent.change(reason, { target: { value: "comped" } });
  fireEvent.click(await screen.findByText("editItems.voidItemConfirm"));

  await waitFor(() => expect(mockAdjustBillItem).toHaveBeenCalled());
  await waitFor(() => expect(onItemsChanged).toHaveBeenCalled());
  // The reason textarea should be gone (modal closed) on success.
  await waitFor(() =>
    expect(screen.queryByLabelText("editItems.voidReasonLabel")).toBeNull(),
  );
});
