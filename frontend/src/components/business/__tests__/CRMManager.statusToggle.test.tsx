/** @jest-environment jsdom */
/**
 * Regression for the CRM pause/resume flow: the toggle and the manager must
 * share ONE status (the manager's useCRMStatus copy). Confirming the modal
 * updates that shared state in place — no window.location.reload() — and the
 * status endpoint is hit exactly once per mount.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const I18N_STUB: Record<string, string> = {
  "businessDashboard.crm.tabs.customers": "Customers",
  "businessDashboard.crm.tabs.segments": "Segments",
  "businessDashboard.crm.tabs.loyalty": "Loyalty",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => I18N_STUB[key] ?? key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/api/crm", () => ({
  businessCRMAPI: {
    getCustomers: jest.fn(() =>
      Promise.resolve({ customers: [], total_pages: 1 }),
    ),
    getCustomerDetails: jest.fn(() => Promise.resolve({ customer: null })),
    updateCustomerNotes: jest.fn(() => Promise.resolve({})),
    updateCustomerTags: jest.fn(() => Promise.resolve({})),
    exportCustomers: jest.fn(() => Promise.resolve(new Blob())),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    toggleCRM: jest.fn(() => Promise.resolve({})),
  },
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

import CRMManager from "@/components/business/CRMManager";
import { businessCRMAPI } from "@/api/crm";

const getCRMStatus = businessCRMAPI.getCRMStatus as jest.Mock;
const toggleCRM = businessCRMAPI.toggleCRM as jest.Mock;

beforeEach(() => {
  jest.clearAllMocks();
  getCRMStatus.mockImplementation(() => Promise.resolve({ enabled: true }));
});

describe("CRMManager status toggle (shared state, no reload)", () => {
  it("fetches CRM status exactly once per mount", async () => {
    render(<CRMManager businessId={1} />);
    await screen.findByText("businessDashboard.crm.shell.activeMode");

    expect(getCRMStatus).toHaveBeenCalledTimes(1);
  });

  it("disabling CRM swaps to the activation card in place", async () => {
    render(<CRMManager businessId={1} />);
    await screen.findByText("businessDashboard.crm.shell.activeMode");

    fireEvent.click(screen.getByText("crmToggle.button.disableCRM"));
    fireEvent.click(
      await screen.findByText("crmToggle.disableModal.buttons.confirm"),
    );

    await waitFor(() => expect(toggleCRM).toHaveBeenCalledWith(1, false));
    // The manager's shared status flips: paused chip + activation card,
    // CRM sub-tabs unmounted — all without a page reload.
    expect(
      await screen.findByText("crmToggle.activationCard.title"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("businessDashboard.crm.shell.pausedMode"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Customers" })).toBeNull();
  });

  it("enabling CRM from the activation card mounts the CRM tabs in place", async () => {
    getCRMStatus.mockImplementation(() => Promise.resolve({ enabled: false }));
    render(<CRMManager businessId={1} />);
    await screen.findByText("crmToggle.activationCard.title");

    fireEvent.click(screen.getByText("crmToggle.activationCard.button"));
    fireEvent.click(
      await screen.findByText("crmToggle.enableModal.buttons.confirm"),
    );

    await waitFor(() => expect(toggleCRM).toHaveBeenCalledWith(1, true));
    expect(
      await screen.findByRole("tab", { name: "Customers" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("businessDashboard.crm.shell.activeMode"),
    ).toBeInTheDocument();
  });
});
