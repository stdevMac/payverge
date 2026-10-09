/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

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
        if (fieldName === "display_name") {
          return plugin._translatedName || plugin.display_name;
        }
        if (fieldName === "message") return plugin.message || plugin.description;
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
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.split(".").pop() || key;
    const translations: Record<string, string> = {
      configurePlugin: "Configurar {name}",
      title: "Plugins",
      subtitle: "Sub",
      enable: "Activar",
      configure: "Configurar",
      disable: "Desactivar",
      loading: "Cargando",
      plugins: "plugins",
      plugin: "plugin",
      searchPlaceholder: "Buscar",
      features: "Features",
      comingSoon: "Coming Soon",
      integration: "Integration",
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
    ModalHeader: ({ children }: any) => (
      <div data-testid="plugin-config-header">{children}</div>
    ),
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

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as any)} />;
  },
}));

const catalogPlugin = {
  id: 7,
  name: "telegram",
  display_name: "Telegram Notifications",
  _translatedName: "Notificaciones de Telegram",
  description: "English description",
  message: "English message",
  image: "/images/plugins/telegram-logo.png",
  category: "integration",
  version: "1.0.0",
  features: "[]",
  is_active: true,
  coming_soon: false,
  config_schema: "{}",
  created_at: "",
  updated_at: "",
};

describe("PluginManager modal header i18n (L6-31)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: [catalogPlugin],
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
    });
    (pluginAPI.business.getPluginConfig as jest.Mock).mockResolvedValue({
      config: {},
    });
  });

  it("uses getTranslatedContent for configure modal header, not raw display_name", async () => {
    render(<PluginManager businessId="42" />);

    const enable = await screen.findByRole("button", { name: "Activar" });
    fireEvent.click(enable);

    await waitFor(() => {
      expect(screen.getByTestId("plugin-config-header")).toHaveTextContent(
        "Configurar Notificaciones de Telegram",
      );
    });
    expect(screen.getByTestId("plugin-config-header")).not.toHaveTextContent(
      "Telegram Notifications",
    );
  });
});
