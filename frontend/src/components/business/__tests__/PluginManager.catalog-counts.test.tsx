/** @jest-environment jsdom */

import React from "react";
import { render, screen, within } from "@testing-library/react";

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
    },
    utils: {
      getTranslatedContent: (plugin: { display_name?: string; description?: string }, fieldName: string) => {
        if (fieldName === "display_name") return plugin.display_name;
        return plugin.description;
      },
      getTranslatedFeatures: () => [],
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
      title: "Plugin Marketplace",
      subtitle: "Extend your business",
      enabled: "Enabled",
      available: "Available",
      comingSoon: "Coming Soon",
      searchPlaceholder: "Search plugins, features...",
    };
    return translations[leaf] || leaf;
  },
}));

jest.mock("react-hot-toast", () => ({
  success: jest.fn(),
  error: jest.fn(),
}));

function plugin(id: number, name: string, comingSoon = false) {
  return {
    id,
    name,
    display_name: name,
    description: name,
    message: "",
    image: "",
    is_active: true,
    coming_soon: comingSoon,
    category: "integration",
    version: "1.0.0",
    features: "[]",
    config_schema: "{}",
    created_at: "",
    updated_at: "",
  };
}

describe("PluginManager catalog counts (#379)", () => {
  it("does not count the enabled plugin as Available", async () => {
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        plugin(1, "stripe"),
        plugin(2, "paypal"),
        plugin(3, "mercadopago"),
        plugin(4, "telegram"),
        plugin(5, "trustpilot"),
        plugin(6, "quickbooks", true),
        plugin(7, "tax_reports", true),
      ],
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          id: 1,
          business_id: 42,
          plugin_id: 1,
          is_enabled: true,
          created_at: "",
          updated_at: "",
        },
      ],
    });

    render(<PluginManager businessId="42" />);
    await screen.findByRole("heading", { name: "stripe" });

    const header = screen.getByRole("banner");
    expect(within(header).getByText("1")).toBeInTheDocument();
    expect(within(header).getByText("Enabled")).toBeInTheDocument();
    expect(within(header).getByText("4")).toBeInTheDocument();
    expect(within(header).getByText("Available")).toBeInTheDocument();
    expect(within(header).getByText("2")).toBeInTheDocument();
    expect(within(header).getByText("Coming Soon")).toBeInTheDocument();
  });
});
