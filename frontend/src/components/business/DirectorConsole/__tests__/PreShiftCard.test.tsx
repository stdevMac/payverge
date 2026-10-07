/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { PackageX } from "lucide-react";
import PreShiftCard from "../PreShiftCard";
import type { PreShiftCardModel } from "../insightCopy";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _locale: string, params?: Record<string, unknown>) =>
    params ? `${key} ${JSON.stringify(params)}` : key,
}));

const model: PreShiftCardModel = {
  id: "a",
  icon: PackageX,
  tone: "urgent",
  copyKey: "outOfStock",
  params: { count: 3, names: "Branzino" },
  tab: "inventory",
};

describe("PreShiftCard", () => {
  it("renders the GM-voice line via the card's copy key + params", () => {
    render(<PreShiftCard model={model} onOpen={jest.fn()} currency="USD" />);
    expect(
      screen.getByText(/directorConsole\.preShift\.cards\.outOfStock/),
    ).toBeInTheDocument();
  });

  it("calls onOpen with the destination tab when the action is clicked", () => {
    const onOpen = jest.fn();
    render(<PreShiftCard model={model} onOpen={onOpen} currency="USD" />);
    fireEvent.click(screen.getByTestId("preshift-card-action"));
    expect(onOpen).toHaveBeenCalledWith("inventory");
  });

  it("formats the money amount with the business currency, not a literal $", () => {
    // Real formatCurrency + intlLocaleFor run here (not mocked). The mocked
    // getTranslation echoes the params it received, so a EUR-formatted amount
    // proves the card threaded the business currency through the formatter
    // (Accounting-tab parity) instead of rendering a separator-less integer.
    const wasteModel: PreShiftCardModel = {
      id: "w",
      icon: PackageX,
      tone: "watch",
      copyKey: "wasteHigh",
      params: { amount: 1234, ingredient: "salmon" },
      tab: "accounting",
    };
    render(<PreShiftCard model={wasteModel} onOpen={jest.fn()} currency="EUR" />);
    const text = screen.getByText(/directorConsole\.preShift\.cards\.wasteHigh/).textContent || "";
    expect(text).toContain("€");
    expect(text).toContain("1,234");
  });
});
