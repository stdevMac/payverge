/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import USDCPaymentConfig from "@/components/business/plugins/USDCPaymentConfig";
import { businessApi } from "@/api/business";

jest.mock("@/api/business", () => ({
  businessApi: { updateBusiness: jest.fn() },
}));

jest.mock("@/providers/DynamicProvider", () => ({
  useDynamicContext: () => ({ setShowAuthFlow: jest.fn(), user: null }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

const plugin = {
  id: 1,
  name: "usdc",
  display_name: "USDC Payments",
  description: "Accept USDC",
  image: "",
  category: "payments",
  version: "1",
  features: "",
  is_enabled: false,
  config: "{}",
};

function renderConfig() {
  return render(
    <USDCPaymentConfig
      plugin={plugin}
      // enabled (not === false) + no settlement_address on the profile is what
      // makes the Enable action open the address-requirement modal under test.
      config={{ enabled: true }}
      onConfigChange={jest.fn()}
      onSave={jest.fn()}
      onCancel={jest.fn()}
      businessProfile={{ id: 42 }}
    />,
  );
}

const VALID = "0x1234567890123456789012345678901234567890";

describe("USDCPaymentConfig — settlement address validation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (businessApi.updateBusiness as jest.Mock).mockResolvedValue({});
  });

  function openAddressModal() {
    // The Enable action opens the address modal when no settlement address set.
    fireEvent.click(screen.getByRole("button", { name: /enable/i }));
  }

  it("keeps Save disabled and does not persist a malformed address", async () => {
    renderConfig();
    openAddressModal();

    const input = await screen.findByLabelText("Wallet Address");
    fireEvent.change(input, { target: { value: "not-an-address" } });

    const saveBtn = screen.getByRole("button", { name: "Save Address" });
    expect(saveBtn).toBeDisabled();

    // Even if forced, the save guard must refuse a malformed value.
    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(businessApi.updateBusiness).not.toHaveBeenCalled();
    });
  });

  it("enables Save and persists a valid EVM address", async () => {
    renderConfig();
    openAddressModal();

    const input = await screen.findByLabelText("Wallet Address");
    fireEvent.change(input, { target: { value: VALID } });

    const saveBtn = screen.getByRole("button", { name: "Save Address" });
    expect(saveBtn).not.toBeDisabled();

    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(businessApi.updateBusiness).toHaveBeenCalledWith(42, {
        settlement_address: VALID,
      });
    });
    expect(businessApi.updateBusiness).not.toHaveBeenCalledWith(
      42,
      expect.objectContaining({ tipping_address: VALID }),
    );
  });
});
