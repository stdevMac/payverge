/**
 * @jest-environment jsdom
 *
 * L6-31: USDC (and CrossChain) must not hardcode t("enable") when already enabled.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import USDCPaymentConfig from "../USDCPaymentConfig";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const leaf = key.split(".").pop() || key;
    if (leaf === "enable") return "Enable";
    if (leaf === "save") return "Save";
    if (leaf === "cancel") return "Cancel";
    return leaf;
  },
}));

jest.mock("@/providers/DynamicProvider", () => ({
  useDynamicContext: () => ({ setShowAuthFlow: jest.fn(), user: null }),
}));

jest.mock("@/api/business", () => ({ businessApi: {} }));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: { alt?: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img alt={props.alt || ""} />
  ),
}));

const basePlugin = {
  id: 1,
  name: "usdc_payment",
  display_name: "USDC",
  description: "d",
  image: "/x.png",
  category: "payment",
  version: "1",
  features: "",
  is_enabled: false,
  config: "{}",
};

describe("USDCPaymentConfig L6-31 enable/save CTA", () => {
  it("shows Enable when plugin is disabled", () => {
    render(
      <USDCPaymentConfig
        plugin={{ ...basePlugin, is_enabled: false }}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Enable" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Save" }),
    ).not.toBeInTheDocument();
  });

  it("shows Save when plugin is already enabled", () => {
    render(
      <USDCPaymentConfig
        plugin={{ ...basePlugin, is_enabled: true }}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Enable" }),
    ).not.toBeInTheDocument();
  });
});
