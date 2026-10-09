/** @jest-environment jsdom */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

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

// Echo the full key so `customPlugin.*` is unambiguous — the leaf-key mocks used
// by the sibling suites collide (`customPlugin.title` and the page title both
// end in "title").
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
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
      return { isOpen, onOpen: () => setIsOpen(true), onClose: () => setIsOpen(false) };
    },
  };
});

jest.mock("react-hot-toast", () => ({ success: jest.fn(), error: jest.fn() }));

// The card only renders when the install configured a booking link; a stock
// self-hosted build ships none. Getter keeps the value switchable per test.
jest.mock("@/config/brand", () => {
  const actual = jest.requireActual("@/config/brand");
  let current = actual.resolveBrandLinks({
    bookCallUrl: "https://booking.example.com/custom-plugin",
  });
  return {
    ...actual,
    get brandLinks() {
      return current;
    },
    __setBrandOverrides: (overrides: Record<string, string | null>) => {
      current = actual.resolveBrandLinks(overrides);
    },
  };
});

const setBrandOverrides = (overrides: Record<string, string | null>) =>
  (
    jest.requireMock("@/config/brand") as {
      __setBrandOverrides: (o: Record<string, string | null>) => void;
    }
  ).__setBrandOverrides(overrides);

const KEY = "businessDashboard.dashboard.pluginManager";
const CUSTOM_TITLE = `${KEY}.customPlugin.title`;
const SEARCH_PLACEHOLDER = `${KEY}.searchPlaceholder`;

const catalog = [
  {
    id: 10,
    name: "stripe",
    display_name: "Stripe",
    description: "Card payments",
    message: "Card payments",
    image: "",
    is_active: true,
    coming_soon: false,
    category: "payment",
    version: "1.0.0",
    features: `["Cards"]`,
    config_schema: `{}`,
    created_at: "",
    updated_at: "",
  },
];

// L6-34 / decision #12. "Need a custom plugin?" was hidden whenever a search was
// active — including the one moment it is most relevant, when the operator
// searched for an integration the catalog does not have. A zero-result search is
// the strongest signal of unmet need the marketplace ever gets.
describe("PluginManager — custom-plugin card on an empty search", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setBrandOverrides({
      bookCallUrl: "https://booking.example.com/custom-plugin",
    });
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: catalog,
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
    });
  });

  const search = (value: string) =>
    fireEvent.change(screen.getByPlaceholderText(SEARCH_PLACEHOLDER), {
      target: { value },
    });

  it("offers the custom-plugin card when a search returns nothing", async () => {
    render(<PluginManager businessId="42" />);
    expect(await screen.findByText("Stripe")).toBeInTheDocument();

    search("zoho");

    await waitFor(() =>
      expect(screen.queryByText("Stripe")).not.toBeInTheDocument(),
    );
    expect(screen.getByText(CUSTOM_TITLE)).toBeInTheDocument();
  });

  it("stays hidden while a search is actually matching plugins", async () => {
    render(<PluginManager businessId="42" />);
    expect(await screen.findByText("Stripe")).toBeInTheDocument();

    search("stri");

    await waitFor(() => expect(screen.getByText("Stripe")).toBeInTheDocument());
    // A filtered view should stay tight — the pitch belongs at the end of the
    // full catalog or in place of nothing, not underneath a hit.
    expect(screen.queryByText(CUSTOM_TITLE)).not.toBeInTheDocument();
  });

  it("still shows below the full catalog when no search is active", async () => {
    render(<PluginManager businessId="42" />);
    expect(await screen.findByText("Stripe")).toBeInTheDocument();
    expect(screen.getByText(CUSTOM_TITLE)).toBeInTheDocument();
  });

  it("opens the configured booking link", async () => {
    const open = jest.spyOn(window, "open").mockImplementation(() => null);
    render(<PluginManager businessId="42" />);
    expect(await screen.findByText("Stripe")).toBeInTheDocument();

    fireEvent.click(screen.getByText(`${KEY}.customPlugin.bookCall`));

    expect(open).toHaveBeenCalledWith(
      "https://booking.example.com/custom-plugin",
      "_blank",
      "noopener,noreferrer",
    );
    open.mockRestore();
  });

  it("stays hidden when no booking link is configured", async () => {
    setBrandOverrides({});
    render(<PluginManager businessId="42" />);
    expect(await screen.findByText("Stripe")).toBeInTheDocument();
    expect(screen.queryByText(CUSTOM_TITLE)).not.toBeInTheDocument();

    search("zoho");
    await waitFor(() =>
      expect(screen.queryByText("Stripe")).not.toBeInTheDocument(),
    );
    expect(screen.queryByText(CUSTOM_TITLE)).not.toBeInTheDocument();
  });

  it("stays hidden when the catalog failed to load", async () => {
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockRejectedValue(
      new Error("boom"),
    );
    render(<PluginManager businessId="42" />);

    // Nothing loaded is not the same as nothing available: pitching a custom
    // build on top of a failed request would be guessing at the operator's need.
    await waitFor(() =>
      expect(screen.getByText("Couldn't load integrations")).toBeInTheDocument(),
    );
    expect(screen.queryByText(CUSTOM_TITLE)).not.toBeInTheDocument();
  });
});
