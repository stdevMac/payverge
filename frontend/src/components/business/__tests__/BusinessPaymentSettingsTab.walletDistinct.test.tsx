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

const SAME = "0x1111111111111111111111111111111111111111";

describe("BusinessPaymentSettingsTab — tip wallet ≠ settlement", () => {
  it("leads with fees & taxes before optional crypto wallets", () => {
    const { container } = render(
      <BusinessPaymentSettingsTab
        profile={{
          settlement_address: "",
          tipping_address: "",
          tax_rate: 10,
          service_fee_rate: 5,
          tax_inclusive: false,
          service_inclusive: false,
        }}
        handleInputChange={jest.fn()}
      />,
    );

    const feesHeading = screen.getByText(/feesAndTaxesTitle/);
    const cryptoHeading = screen.getByText(/cryptoAddressesTitle/);
    expect(
      feesHeading.compareDocumentPosition(cryptoHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(container.textContent).toContain("checkoutRailsNotice");
  });

  it("flags identical settlement and tip wallets", () => {
    render(
      <BusinessPaymentSettingsTab
        profile={{
          settlement_address: SAME,
          tipping_address: SAME,
          tax_rate: 10,
          service_fee_rate: 5,
          tax_inclusive: false,
          service_inclusive: false,
        }}
        handleInputChange={jest.fn()}
      />,
    );

    expect(screen.getAllByText(/tipWalletMustDiffer/).length).toBeGreaterThan(0);
  });
});
