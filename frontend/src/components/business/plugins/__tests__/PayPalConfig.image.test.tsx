/** @jest-environment jsdom */

import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as React.ImgHTMLAttributes<HTMLImageElement>)} />;
  },
}));

import PayPalConfig from "../PayPalConfig";

const basePlugin = {
  id: 2,
  name: "paypal",
  display_name: "PayPal",
  description: "PayPal payments",
  image: "",
  category: "payments",
  version: "1.0.0",
  features: "",
  is_enabled: false,
  config: "{}",
};

describe("PayPalConfig logo fallback", () => {
  it("uses the shipped paypal logo when plugin.image is empty", () => {
    render(
      <PayPalConfig
        plugin={basePlugin}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );

    expect(screen.getByRole("img", { name: "PayPal" })).toHaveAttribute(
      "src",
      expect.stringContaining("paypal-logo.png"),
    );
  });
});
