/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import TrustpilotConfig from "./TrustpilotConfig";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: { alt?: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img alt={props.alt || ""} />
  ),
}));

const plugin = {
  id: 1,
  name: "trustpilot",
  display_name: "Trustpilot",
  description: "Reviews",
  image: "/images/plugins/trustpilot.png",
  category: "marketing",
  version: "1.0",
  features: "",
  is_enabled: false,
  config: "{}",
};

describe("TrustpilotConfig required-field validation (L6-30)", () => {
  it("disables save and does not call onSave when required fields are empty", () => {
    const onSave = jest.fn();
    render(
      <TrustpilotConfig
        plugin={plugin}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={onSave}
        onCancel={jest.fn()}
      />,
    );

    const saveBtn = screen.getByRole("button", {
      name: "businessDashboard.dashboard.pluginManager.config.save",
    });
    expect(saveBtn).toBeDisabled();
    fireEvent.click(saveBtn);
    expect(onSave).not.toHaveBeenCalled();
  });

  it("calls onSave when business name and URL are filled", () => {
    const onSave = jest.fn();
    render(
      <TrustpilotConfig
        plugin={plugin}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={onSave}
        onCancel={jest.fn()}
      />,
    );

    const inputs = screen.getAllByRole("textbox");
    fireEvent.change(inputs[0], { target: { value: "Cafe Test" } });
    fireEvent.change(inputs[1], {
      target: { value: "https://www.trustpilot.com/review/example.com" },
    });

    const saveBtn = screen.getByRole("button", {
      name: "businessDashboard.dashboard.pluginManager.config.save",
    });
    expect(saveBtn).not.toBeDisabled();
    fireEvent.click(saveBtn);
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});
