/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const mockToastError = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: (...a: unknown[]) => mockToastError(...a) },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

import { ItemCustomizer } from "../ItemCustomizer";

const itemWithRequired = {
  id: "i1",
  name: "Burger",
  price: 10,
  options: [
    { name: "Size", price_change: 0, is_required: true },
    { name: "Extra cheese", price_change: 1, is_required: false },
  ],
} as never;

beforeEach(() => jest.clearAllMocks());

it("blocks Add to Cart while a required option is unselected (F15)", () => {
  const onAddToCart = jest.fn();
  render(
    <ItemCustomizer
      isOpen
      onClose={jest.fn()}
      item={itemWithRequired}
      onAddToCart={onAddToCart}
    />,
  );

  // The Add button label includes itemCustomizer.addToCart.
  const addBtn = screen
    .getByText(/itemCustomizer\.addToCart/)
    .closest("button") as HTMLButtonElement;

  // Disabled because the required "Size" option is unselected.
  expect(addBtn).toBeDisabled();
});

it("allows Add once the required option is selected", () => {
  const onAddToCart = jest.fn();
  render(
    <ItemCustomizer
      isOpen
      onClose={jest.fn()}
      item={itemWithRequired}
      onAddToCart={onAddToCart}
    />,
  );

  // Select the required "Size" option checkbox.
  fireEvent.click(screen.getByText("Size"));

  const addBtn = screen
    .getByText(/itemCustomizer\.addToCart/)
    .closest("button") as HTMLButtonElement;
  expect(addBtn).not.toBeDisabled();

  fireEvent.click(addBtn);
  expect(onAddToCart).toHaveBeenCalledTimes(1);
  // The selected required option is included.
  const passedOptions = onAddToCart.mock.calls[0][2];
  expect(passedOptions.map((o: { name: string }) => o.name)).toContain("Size");
});
