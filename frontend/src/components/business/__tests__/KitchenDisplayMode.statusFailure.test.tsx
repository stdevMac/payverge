/** @jest-environment jsdom */
/**
 * K-3 regression: KDS status-update failures must surface a translated toast
 * (mirroring Kitchen list mode), with dedicated 409 wording for the two-cook
 * race. Pre-fix the catch was console.error-only.
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

const mockToastError = jest.fn();
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: (...a: unknown[]) => mockToastError(...a), success: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/orders", () => ({
  updateOrderStatus: jest.fn(),
  parseOrderItems: (items: string) => JSON.parse(items || "[]"),
}));

jest.mock(
  "@/components/business/operational-alerts/EnableAlertSoundButton",
  () => ({ __esModule: true, default: () => null }),
);
jest.mock(
  "@/components/business/operational-alerts/OperationalAlertClaimStatus",
  () => ({ __esModule: true, default: () => null }),
);

import KitchenDisplayMode from "@/components/business/KitchenDisplayMode";
import type { Order } from "@/api/orders";

const approvedOrder: Order = {
  id: 1,
  bill_id: 10,
  business_id: 1,
  order_number: "A-1",
  status: "approved",
  items: JSON.stringify([
    {
      id: "i1",
      menu_item_name: "Soup",
      quantity: 1,
      price: 12,
      options: [],
      special_requests: "",
      subtotal: 12,
    },
  ]),
  currency: "USD",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

const renderKds = (onOrderStatusChange: jest.Mock) =>
  render(
    <KitchenDisplayMode
      orders={[approvedOrder]}
      onExit={jest.fn()}
      onRefresh={jest.fn()}
      businessId={1}
      locale={"en" as never}
      onOrderStatusChange={onOrderStatusChange}
    />,
  );

beforeEach(() => jest.clearAllMocks());

it("shows the conflict toast on a 409 (two cooks, same ticket)", async () => {
  const onOrderStatusChange = jest
    .fn()
    .mockRejectedValue({ response: { status: 409 } });
  renderKds(onOrderStatusChange);

  fireEvent.click(screen.getByText("kitchenDisplay.actions.start"));

  await waitFor(() =>
    expect(mockToastError).toHaveBeenCalledWith(
      "kitchenDisplay.errors.conflictRefreshed",
    ),
  );
});

it("shows the generic update-failed toast on a 500", async () => {
  const onOrderStatusChange = jest
    .fn()
    .mockRejectedValue({ response: { status: 500 } });
  renderKds(onOrderStatusChange);

  fireEvent.click(screen.getByText("kitchenDisplay.actions.start"));

  await waitFor(() =>
    expect(mockToastError).toHaveBeenCalledWith(
      "kitchenDisplay.errors.updateFailed",
    ),
  );
});
