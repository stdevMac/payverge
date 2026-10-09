/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import CurrencyConverter, { CurrencyPrice } from "./CurrencyConverter";

jest.mock("../../api/currency", () => ({
  // Deferred promise so the loading branch is observable.
  convertAmount: jest.fn(),
  // Mirror the real shape closely enough for assertions.
  formatCurrency: (amount: number, code: string) => {
    if (code === "USDC") return `USDC ${amount.toFixed(2)}`;
    return `$${amount.toFixed(2)}`;
  },
}));

import { convertAmount } from "../../api/currency";

describe("CurrencyPrice loading skeleton", () => {
  it("renders the skeleton as an inline (non-block) element valid inside <p>", () => {
    // Never resolves -> stays in loading state.
    (convertAmount as jest.Mock).mockReturnValue(new Promise(() => {}));

    const { container } = render(
      <p data-testid="row">
        <CurrencyPrice
          amount={10}
          fromCurrency="EUR"
          displayCurrency="USD"
          locale="en-US"
        />
      </p>,
    );

    // No <div> may exist inside the <p> (invalid HTML / hydration mismatch).
    expect(container.querySelector("p div")).toBeNull();
    const skeleton = container.querySelector("p span.animate-pulse");
    expect(skeleton).not.toBeNull();
    expect(skeleton?.className).toContain("inline-block");
  });
});

describe("CurrencyConverter USDC chip", () => {
  it("formats the USDC equivalent via formatCurrency, with no hardcoded $", async () => {
    (convertAmount as jest.Mock).mockResolvedValue({ converted_amount: 12.34 });

    render(
      <CurrencyConverter
        amount={12.34}
        fromCurrency="USD"
        displayCurrency="USD"
        showUSDCConversion
        locale="en-US"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText(/USDC 12\.34/)).toBeInTheDocument();
    });
    // The old "$12.34 USDC" form must be gone.
    expect(screen.queryByText(/\$12\.34 USDC/)).toBeNull();
  });
});
