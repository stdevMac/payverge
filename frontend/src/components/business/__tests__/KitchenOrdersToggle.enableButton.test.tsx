/** @jest-environment jsdom */
/**
 * Button variant when ordering is disabled must show an activate control
 * that opens the enable modal and calls toggle with true.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const mockToggle = jest.fn(() => Promise.resolve());
jest.mock("@/api/kitchenOrders", () => ({
  getKitchenOrdersStatus: jest.fn(() =>
    Promise.resolve({ kitchen_enabled: true, orders_enabled: true }),
  ),
  toggleKitchenAndOrders: (...a: unknown[]) => mockToggle(...(a as [])),
}));

import { KitchenOrdersToggle } from "@/components/business/KitchenOrdersToggle";

describe("KitchenOrdersToggle button variant when ordering is disabled", () => {
  it("shows an activate button that opens the enable modal and enables ordering", async () => {
    render(
      <KitchenOrdersToggle
        businessId={1}
        isLocked={false}
        variant="button"
        externalEnabled={false}
        externalLoading={false}
      />,
    );

    fireEvent.click(screen.getByText("kitchenOrdersToggle.button.enableOrdering"));
    fireEvent.click(
      await screen.findByText("kitchenOrdersToggle.enableModal.buttons.confirm"),
    );

    await waitFor(() => expect(mockToggle).toHaveBeenCalledWith(1, true));
  });
});
