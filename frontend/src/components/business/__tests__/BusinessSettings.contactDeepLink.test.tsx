/** @jest-environment jsdom */
/**
 * Related to #225 — Settings Contact is a working deep-link to Business Page
 * → Contact, and Disable Ordering lives in a service/danger section, not
 * among profile fields under the logo.
 */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { getTestUrl, resetTestUrl, setTestUrl } from "@/test/nextNavigationMock";

jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => mockToast,
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isStaffUser: false,
    isOAuthUser: true,
    isWeb3User: false,
    isInitialized: true,
  }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo",
      custom_url: "demo",
      phone: "+15550001111",
      website: "https://demo.example",
      address: {
        street: "1 Main",
        city: "Austin",
        state: "TX",
        postal_code: "78701",
        country: "US",
      },
    }),
    updateBusiness: jest.fn().mockResolvedValue({}),
    updateBusinessDesignSettings: jest.fn().mockResolvedValue({}),
  },
}));

jest.mock("next/dynamic", () => ({
  __esModule: true,
  default: () => () => <div data-testid="dynamic-stub" />,
}));

jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design" />,
}));

jest.mock("@/components/business/CurrencySettings", () => {
  const r = jest.requireActual("react") as typeof import("react");
  const Stub = r.forwardRef(() =>
    r.createElement("div", { "data-testid": "currency" }),
  );
  return { __esModule: true, default: Stub };
});

jest.mock("@/components/business/DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked" />,
}));

jest.mock("@/components/business/SimpleImageUpload", () => ({
  __esModule: true,
  default: () => <div data-testid="image-upload" />,
}));

jest.mock("@/components/business/KitchenOrdersToggle", () => ({
  KitchenOrdersToggle: () => <div data-testid="kitchen-toggle" />,
}));

jest.mock("@/components/business/SettingsSkeleton", () => ({
  SettingsSkeleton: () => <div data-testid="skeleton" />,
}));

jest.mock("@/utils/businessDataParsers", () => ({
  parseSocialMedia: () => ({}),
  parseDesignSettings: (x: unknown) => x,
}));

import BusinessSettings from "@/components/business/BusinessSettings";

describe("BusinessSettings — contact deep-link + guest ordering (#225)", () => {
  beforeEach(() => {
    resetTestUrl("/business/demo/dashboard?tab=settings");
  });

  it("Edit on Business Page is a real link to Business Page → Contact", async () => {
    render(<BusinessSettings businessId={1} />);
    const link = await screen.findByTestId("edit-contact-on-business-page");
    expect(link.tagName.toLowerCase()).toBe("a");
    expect(link).toHaveAttribute(
      "href",
      "/business/1/dashboard?tab=business-page&section=contact",
    );
  });

  it("does not strip section=contact when the URL moves to Business Page (#225)", async () => {
    render(<BusinessSettings businessId={1} />);
    await screen.findByTestId("edit-contact-on-business-page");

    act(() => {
      setTestUrl("/business/1/dashboard?tab=business-page&section=contact");
    });

    await waitFor(() => {
      expect(getTestUrl()).toContain("tab=business-page");
      expect(getTestUrl()).toContain("section=contact");
    });
  });

  it("does not raise unsaved changes when switching Settings sub-tabs (#224 / #268)", async () => {
    render(<BusinessSettings businessId={1} />);
    await screen.findByDisplayValue("Demo");
    expect(screen.getByTestId("save-bar-all-saved")).toHaveTextContent(
      /No unsaved changes/i,
    );
    expect(screen.queryByText(/saved automatically/i)).not.toBeInTheDocument();

    act(() => {
      setTestUrl("/business/demo/dashboard?tab=settings&section=localization");
    });
    expect(getTestUrl()).toContain("section=localization");
    expect(await screen.findByTestId("currency")).toBeInTheDocument();
    expect(screen.getByTestId("save-bar-all-saved")).toHaveTextContent(
      /No unsaved changes/i,
    );
    expect(screen.queryByText(/You have unsaved changes/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/saved automatically/i)).not.toBeInTheDocument();

    act(() => {
      setTestUrl("/business/demo/dashboard?tab=settings&section=notifications");
    });
    await waitFor(() => {
      expect(screen.queryByText(/You have unsaved changes/i)).not.toBeInTheDocument();
    });
    // Notifications is a real autosave surface — that claim is honest here.
    expect(screen.getByText(/Changes saved automatically/i)).toBeInTheDocument();
  });

  it("places Disable Ordering in a service section, not under the logo", async () => {
    render(<BusinessSettings businessId={1} />);
    const service = await screen.findByTestId("settings-guest-ordering");
    expect(service).toContainElement(screen.getByTestId("kitchen-toggle"));
    expect(screen.getByTestId("image-upload")).toBeInTheDocument();
    // Logo uploader and the service switch are siblings of different sections,
    // not stacked as two profile cards.
    expect(screen.getByTestId("image-upload").closest("div")?.parentElement).not.toBe(
      service,
    );
  });
});
