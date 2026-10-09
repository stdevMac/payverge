/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import PluginManager from "@/components/business/PluginManager";
import { pluginAPI } from "@/api/plugins";
import { PLUGIN } from "@/constants/plugins";

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
      getTranslatedContent: (plugin: { display_name?: string; description?: string; message?: string }, fieldName: string) => {
        if (fieldName === "display_name") return plugin.display_name;
        if (fieldName === "message") return plugin.message || plugin.description;
        return plugin.description;
      },
      getTranslatedFeatures: () => [],
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
    const map: Record<string, string> = {
      "businessDashboard.dashboard.pluginManager.title": "Plugins",
      "businessDashboard.dashboard.pluginManager.subtitle": "Integrations",
      "businessDashboard.dashboard.pluginManager.enable": "Enable",
      "businessDashboard.dashboard.pluginManager.configure": "Configure",
      "businessDashboard.dashboard.pluginManager.plugin": "plugin",
      "businessDashboard.dashboard.pluginManager.plugins": "plugins",
      "businessDashboard.dashboard.pluginManager.categories.payment":
        "Payment Processing",
      "businessDashboard.dashboard.pluginManager.categorySubtitle":
        "Review capabilities",
      "businessDashboard.dashboard.pluginManager.showing":
        "Showing {visible} of {total}",
      "businessDashboard.dashboard.pluginManager.status.available": "Available",
      "businessDashboard.dashboard.pluginManager.status.enabled": "Enabled",
      "businessDashboard.dashboard.pluginManager.status.ready": "Ready to enable",
      "businessDashboard.dashboard.pluginManager.status.configured":
        "Configured",
      "businessDashboard.dashboard.pluginManager.cardRailCallout.title":
        "No card processor enabled",
      "businessDashboard.dashboard.pluginManager.cardRailCallout.description":
        "Guests can pay at the counter once the cash drawer is open. Enable Mercado Pago to take cards — we will not turn it on until you connect your account.",
      "businessDashboard.dashboard.pluginManager.cardRailCallout.action":
        "Enable {name}",
    };
    return map[key] ?? key;
  },
}));

jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  return {
    Image: ({ alt }: { alt?: string }) => <span aria-label={alt} />,
    Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    Modal: ({
      children,
      isOpen,
    }: {
      children: React.ReactNode;
      isOpen?: boolean;
    }) => (isOpen ? <div data-testid="plugin-config-modal">{children}</div> : null),
    ModalContent: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    ModalHeader: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    ModalBody: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
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

const paymentPlugins = [
  {
    id: 1,
    name: PLUGIN.usdcPayment,
    display_name: "USDC Payment",
    description: "Optional crypto rail",
    message: "Optional crypto rail",
    image: "",
    is_active: true,
    coming_soon: false,
    category: "payment",
    version: "1.0.0",
    features: "[]",
    config_schema: "{}",
    created_at: "",
    updated_at: "",
  },
  {
    id: 2,
    name: PLUGIN.mercadopago,
    display_name: "MercadoPago",
    description: "Cards and local checkout",
    message: "Cards and local checkout",
    image: "",
    is_active: true,
    coming_soon: false,
    category: "payment",
    version: "1.0.0",
    features: "[]",
    config_schema: "{}",
    created_at: "",
    updated_at: "",
  },
];

describe("PluginManager card rail honesty", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: paymentPlugins,
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { plugin_id: 1, is_enabled: true, config: "{}" },
        { plugin_id: 2, is_enabled: false, config: "{}" },
      ],
    });
  });

  it("lists Mercado Pago first, keeps it available, and does not auto-enable", async () => {
    render(<PluginManager businessId="42" />);

    expect(await screen.findByText("MercadoPago")).toBeInTheDocument();
    const headings = screen.getAllByRole("heading", { level: 3 });
    expect(headings[0]).toHaveTextContent("MercadoPago");
    expect(headings[1]).toHaveTextContent("USDC Payment");

    expect(screen.getByTestId("card-rail-callout")).toHaveTextContent(
      "No card processor enabled",
    );
    expect(
      screen.getByRole("button", { name: "Enable MercadoPago" }),
    ).toBeInTheDocument();

    const cardButtons = screen.getAllByRole("button", { name: "Enable" });
    expect(cardButtons.length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Enable MercadoPago" }));
    expect(await screen.findByTestId("plugin-config-modal")).toBeInTheDocument();
    expect(pluginAPI.business.enablePlugin).not.toHaveBeenCalled();
  });

  it("hides the card-rail callout once a card processor is enabled", async () => {
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { plugin_id: 1, is_enabled: true, config: "{}" },
        { plugin_id: 2, is_enabled: true, config: "{}" },
      ],
    });

    render(<PluginManager businessId="42" />);
    expect(await screen.findByText("MercadoPago")).toBeInTheDocument();
    expect(screen.queryByTestId("card-rail-callout")).not.toBeInTheDocument();
  });
});
