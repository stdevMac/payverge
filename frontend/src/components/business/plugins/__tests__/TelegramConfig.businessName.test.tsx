/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import TelegramConfig from "../TelegramConfig";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

jest.mock("react-hot-toast", () => ({
  success: jest.fn(),
  error: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key.split(".").pop() || key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as any)} />;
  },
}));

const mockAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const plugin = {
  id: 1,
  name: "telegram",
  display_name: "Telegram",
  description: "Telegram notifications",
  image: "/images/plugins/telegram.png",
  category: "integration",
  version: "1.0.0",
  features: "",
  is_enabled: false,
  config: "{}",
};

describe("TelegramConfig business_name (L6-29)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAxios.get.mockResolvedValue({
      data: { connected: false, plugin_enabled: false, health: "not_connected" },
    });
  });

  it("enables Save when business_name is absent from config but derived from businessName", async () => {
    const onSave = jest.fn();
    const onConfigChange = jest.fn();

    render(
      <TelegramConfig
        businessId="42"
        plugin={plugin}
        config={{}}
        onConfigChange={onConfigChange}
        onSave={onSave}
        onCancel={jest.fn()}
        businessName="Demo Bistro"
      />,
    );

    // Primary CTA is Enable when plugin is not yet enabled.
    const saveBtn = await screen.findByRole("button", { name: "enable" });
    expect(saveBtn).not.toBeDisabled();

    await waitFor(() => {
      const last = onConfigChange.mock.calls.at(-1)?.[0];
      expect(last?.business_name).toBe("Demo Bistro");
    });

    await userEvent.click(saveBtn);
    expect(onSave).toHaveBeenCalled();
  });

  it("does not block Save when config has no business_name and no businessName prop", async () => {
    render(
      <TelegramConfig
        businessId="42"
        plugin={plugin}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );

    const saveBtn = await screen.findByRole("button", { name: "enable" });
    // business_name is not a form field operators can fill — never gate save on it.
    expect(saveBtn).not.toBeDisabled();
  });
});
