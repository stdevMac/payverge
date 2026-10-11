/** @jest-environment jsdom */
/**
 * K-12: actionLoading must reset after a SUCCESSFUL toggle too (it was only
 * reset in the catch, leaving the button permanently disabled when the
 * component stayed mounted).
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

it("re-enables the disable button after a successful toggle", async () => {
  render(
    <KitchenOrdersToggle
      businessId={1}
      isLocked={false}
      variant="button"
      externalEnabled
      externalLoading={false}
    />,
  );

  const disableButton = screen.getByText(
    "kitchenOrdersToggle.button.disableOrdering",
  );
  fireEvent.click(disableButton);

  const confirm = await screen.findByText(
    "kitchenOrdersToggle.disableModal.buttons.confirm",
  );
  fireEvent.click(confirm);

  await waitFor(() => expect(mockToggle).toHaveBeenCalledWith(1, false));

  await waitFor(() =>
    expect(
      (screen.getByText("kitchenOrdersToggle.button.disableOrdering")
        .closest("button") as HTMLButtonElement).disabled,
    ).toBe(false),
  );
});
