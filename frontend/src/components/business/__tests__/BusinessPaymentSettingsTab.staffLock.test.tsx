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
  settlement_address: "0x1111111111111111111111111111111111111111",
  tipping_address: "",
  tax_rate: 10,
  service_fee_rate: 5,
  tax_inclusive: false,
  service_inclusive: false,
};

describe("BusinessPaymentSettingsTab — staff wallet lock (P1-12)", () => {
  it("disables both payout wallet inputs for staff sessions", () => {
    render(
      <BusinessPaymentSettingsTab
        profile={profile}
        handleInputChange={jest.fn()}
        walletFieldsDisabled
      />,
    );
    expect(
      screen.getByLabelText(/settlementAddressLabel/),
    ).toBeDisabled();
    expect(screen.getByLabelText(/tippingAddressLabel/)).toBeDisabled();
    // The rate fields stay editable — only wallets are owner-only.
    expect(screen.getByLabelText(/taxRateLabel/)).not.toBeDisabled();
  });

  it("keeps wallet inputs editable for owner sessions", () => {
    render(
      <BusinessPaymentSettingsTab
        profile={profile}
        handleInputChange={jest.fn()}
      />,
    );
    expect(
      screen.getByLabelText(/settlementAddressLabel/),
    ).not.toBeDisabled();
    expect(screen.getByLabelText(/tippingAddressLabel/)).not.toBeDisabled();
  });
});
