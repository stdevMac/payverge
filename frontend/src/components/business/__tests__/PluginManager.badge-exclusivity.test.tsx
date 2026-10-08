/** @jest-environment jsdom */

import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

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
      getTranslatedContent: (plugin: any, fieldName: string) => {
        if (fieldName === "message") return plugin.message || plugin.description;
        if (fieldName === "display_name") return plugin.display_name;
        if (fieldName === "description") return plugin.description;
        if (fieldName === "features") return plugin.features || "[]";
        return plugin.description;
      },
      getTranslatedFeatures: (plugin: any) =>
        JSON.parse(plugin.features || "[]"),
    },
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    loading: false,
  }),
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
      loading: "Loading plugins...",
      plugins: "plugins",
      plugin: "plugin",
      integration: "Third-party Integration",
      reporting: "Tax & Compliance",
      comingSoon: "Coming Soon",
      enabled: "Enabled",
      available: "Available",
      error: "Needs review",
      waitlist: "Roadmap",
      configured: "Configured",
      ready: "Ready to enable",
      enable: "Enable",
      configure: "Configure",
      disable: "Disable",
      features: "Features",
      searchPlaceholder: "Search plugins, features...",
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

jest.mock("react-hot-toast", () => ({
  success: jest.fn(),
  error: jest.fn(),
}));

/**
 * Finding 46: coming_soon and is_enabled must produce exactly one status.
 * QuickBooks/Tax Reports historically badged as both Active/Enabled and
 * Coming Soon when the two flags were rendered independently.
 */
describe("PluginManager badge exclusivity", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows Coming Soon only when coming_soon even if is_enabled is true", async () => {
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          id: 10,
          name: "quickbooks",
          display_name: "QuickBooks",
          description: "Accounting sync",
          message: "QuickBooks accounting sync is coming soon.",
          image: "",
          is_active: true,
          coming_soon: true,
          category: "integration",
          version: "1.0.0",
          features: `["Accounting Sync"]`,
          config_schema: `{}`,
          created_at: "",
          updated_at: "",
        },
      ],
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          id: 1,
          business_id: 42,
          plugin_id: 10,
          is_enabled: true,
          created_at: "",
          updated_at: "",
        },
      ],
    });

    render(<PluginManager businessId="42" />);

    expect(await screen.findByText("QuickBooks")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getAllByText("Coming Soon").length).toBeGreaterThanOrEqual(
        1,
      );
    });

    // Single derived status on the card: Coming Soon wins over is_enabled.
    // "Enabled" appears as a shell signal *label* (column header) so we do not
    // assert its global absence — assert card actions and secondary status.
    expect(screen.queryByText("Configured")).not.toBeInTheDocument();
    expect(screen.queryByText("Ready to enable")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Enable" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Configure" }),
    ).not.toBeInTheDocument();
    // Roadmap is the footer label for comingSoon via getPluginStatus.
    expect(screen.getByText("Roadmap")).toBeInTheDocument();
  });
});
