/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import BusinessPaymentSettingsTab from "../BusinessPaymentSettingsTab";

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

const profile = {
  settlement_address: "",
  tipping_address: "",
  tax_rate: 10,
  service_fee_rate: 5,
  tax_inclusive: false,
  service_inclusive: false,
};

describe("BusinessPaymentSettingsTab — sample check preview (#266)", () => {
  it("renders effective-date disclosure and a $100 sample check total", () => {
    render(
      <BusinessPaymentSettingsTab
        profile={profile}
        handleInputChange={jest.fn()}
      />,
    );

    expect(screen.getByTestId("fee-effective-date")).toBeInTheDocument();
    expect(screen.getByTestId("sample-check-preview")).toBeInTheDocument();
    // $100 + 10% tax + 5% service = $115.00
    expect(screen.getByText("$115.00")).toBeInTheDocument();
    expect(screen.getByText("$100.00")).toBeInTheDocument();
  });
});
