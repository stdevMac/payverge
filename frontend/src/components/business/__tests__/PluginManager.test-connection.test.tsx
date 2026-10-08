/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import PluginManager from "@/components/business/PluginManager";
import { pluginAPI } from "@/api/plugins";

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    protected: { getAllPlugins: jest.fn() },
    business: {
      getBusinessPlugins: jest.fn(),
      getPluginConfig: jest.fn(),
      enablePlugin: jest.fn(),
      updatePluginConfig: jest.fn(),
      disablePlugin: jest.fn(),
      testPluginConnection: jest.fn(),
    },
    utils: {
      getTranslatedContent: (plugin: any, fieldName: string) => {
        if (fieldName === "display_name") return plugin.display_name;
        if (fieldName === "description") return plugin.description;
        if (fieldName === "features") return plugin.features || "[]";
        return plugin.description;
      },
      getTranslatedFeatures: (plugin: any) => JSON.parse(plugin.features || "[]"),
    },
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({ id: 42, name: "Cafe" }),
  },
}));

jest.mock("@/components/business/plugins/PluginConfigFactory", () => ({
  __esModule: true,
  default: () => <div>Plugin configuration</div>,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const leaf = key.split(".").pop() || key;
    const translations: Record<string, string> = {
      configure: "Configure",
      button: "Test connection",
      testing: "Testing…",
      ok: "Connection successful.",
    };
    return translations[leaf] || leaf;
  },
}));

jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  return {
    Image: ({ alt }: any) => <span aria-label={alt} />,
    Tooltip: ({ children }: any) => <>{children}</>,
    Modal: ({ children, isOpen }: any) =>
      isOpen ? <div data-testid="plugin-config-modal">{children}</div> : null,
    ModalContent: ({ children }: any) => <div>{children}</div>,
    ModalHeader: ({ children }: any) => <div>{children}</div>,
    ModalBody: ({ children }: any) => <div>{children}</div>,
    useDisclosure: () => {
      const [isOpen, setIsOpen] = React.useState(false);
      return {
        isOpen,
        onOpen: () => setIsOpen(true),
        onClose: () => setIsOpen(false),
      };
    },
  };
});

jest.mock("react-hot-toast", () => ({ success: jest.fn(), error: jest.fn() }));

const stripePlugin = {
  id: 1,
  name: "stripe",
  display_name: "Stripe",
  description: "Card payments",
  image: "",
  price: "0",
  is_active: true,
  coming_soon: false,
  category: "payment",
  version: "1.0.0",
  features: `["Cards"]`,
  config_schema: `{}`,
  created_at: "",
  updated_at: "",
};

describe("PluginManager test connection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: [stripePlugin],
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [{ plugin_id: 1, is_enabled: true, config: "{}" }],
    });
    (pluginAPI.business.getPluginConfig as jest.Mock).mockResolvedValue({
      config: {},
    });
  });

  it("tests a configured payment provider and reports success inline", async () => {
    (pluginAPI.business.testPluginConnection as jest.Mock).mockResolvedValue({
      ok: true,
      provider: "stripe",
      message: "Connection successful.",
    });

    render(<PluginManager businessId="42" />);

    const configureBtn = await screen.findByRole("button", {
      name: "Configure",
    });
    fireEvent.click(configureBtn);

    const testBtn = await screen.findByRole("button", {
      name: "Test connection",
    });
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(pluginAPI.business.testPluginConnection).toHaveBeenCalledWith(
        "42",
        "stripe",
      );
    });
    expect(await screen.findByText("Connection successful.")).toBeInTheDocument();
  });
});
