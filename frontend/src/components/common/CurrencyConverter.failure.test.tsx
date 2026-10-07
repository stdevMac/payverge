/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import CurrencyConverter, { CurrencyPrice } from "./CurrencyConverter";

// Code-echoing formatCurrency so we can tell which currency a value is labeled
// with (the shared CurrencyConverter.test.tsx mock collapses all non-USDC codes
// to "$" and can't distinguish fromCurrency vs displayCurrency).
jest.mock("../../api/currency", () => ({
  convertAmount: jest.fn(),
  formatCurrency: (amount: number, code: string) => `${code} ${amount.toFixed(2)}`,
}));

import { convertAmount } from "../../api/currency";

describe("CurrencyConverter conversion-failure labeling (D-01)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("CurrencyPrice labels the unconverted amount in fromCurrency, not displayCurrency", async () => {
    (convertAmount as jest.Mock).mockRejectedValue(new Error("rate outage"));

    render(
      <CurrencyPrice
        amount={50}
        fromCurrency="AED"
        displayCurrency="USD"
        locale="en-US"
      />,
    );

    // Must show the magnitude in its true source currency (AED), never mislabel
    // 50 as USD.
    await waitFor(() => expect(screen.getByText("AED 50.00")).toBeInTheDocument());
    expect(screen.queryByText("USD 50.00")).toBeNull();
  });

  it("CurrencyConverter falls back to fromCurrency and hides the USDC estimate on failure", async () => {
    (convertAmount as jest.Mock).mockRejectedValue(new Error("rate outage"));

    render(
      <CurrencyConverter
        amount={50}
        fromCurrency="AED"
        displayCurrency="USD"
        showUSDCConversion
        locale="en-US"
      />,
    );

    await waitFor(() => expect(screen.getByText("AED 50.00")).toBeInTheDocument());
    // The uncomputable USDC estimate must not be shown as a real number.
    expect(screen.queryByText(/USDC/)).toBeNull();
    expect(screen.queryByText("USD 50.00")).toBeNull();
  });
});

describe("CurrencyConverter stale-response guard (D-02)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("a slow earlier conversion cannot overwrite a newer one", async () => {
    let resolveOld!: (v: { converted_amount: number }) => void;
    let resolveNew!: (v: { converted_amount: number }) => void;
    (convertAmount as jest.Mock)
      .mockReturnValueOnce(new Promise((r) => (resolveOld = r)))
      .mockReturnValueOnce(new Promise((r) => (resolveNew = r)));

    const { rerender } = render(
      <CurrencyPrice
        amount={10}
        fromCurrency="AED"
        displayCurrency="USD"
        locale="en-US"
      />,
    );
    // New amount before the first conversion resolves -> effect cleanup cancels
    // the stale call.
    rerender(
      <CurrencyPrice
        amount={20}
        fromCurrency="AED"
        displayCurrency="USD"
        locale="en-US"
      />,
    );

    // Resolve the NEWER call first, then the stale OLDER one.
    resolveNew({ converted_amount: 40 });
    await waitFor(() => expect(screen.getByText("USD 40.00")).toBeInTheDocument());
    resolveOld({ converted_amount: 999 });

    // The stale value must never be shown.
    await waitFor(() => expect(screen.getByText("USD 40.00")).toBeInTheDocument());
    expect(screen.queryByText("USD 999.00")).toBeNull();
  });
});
