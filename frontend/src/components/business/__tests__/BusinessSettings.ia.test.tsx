/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

// Mutable locale so we can flip it and re-render (reproduces the freeze bug).
let mockLocale = "en";
let mockTierAccess = true;
let mockTierSuspended = false;
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: mockLocale, setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

// Stable toast fns — a fresh jest.fn() every render would recreate
// loadBusinessProfile (showError is in its dep array) and re-fetch mid-edit,
// wiping the dirty baseline useDirtyForm just captured (Task 30).
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

// Wave 3 (P1-12): BusinessSettings reads isStaffUser to lock owner-only fields.
// Shell tests run without a real HybridAuthProvider.
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
    hasAccess: mockTierAccess,
    isSuspended: mockTierSuspended,
    loading: false,
  }),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest
      .fn()
      .mockResolvedValue({ name: "Demo", custom_url: "demo" }),
    updateBusiness: jest.fn().mockResolvedValue({}),
    updateBusinessDesignSettings: jest.fn().mockResolvedValue({}),
  },
}));

// Stub next/navigation (used by BusinessProfileTab sub-component). useParams is
// required by the useBusinessUrlId hook the reopen-wizard button calls — a
// file-level mock fully overrides jest.setup.js's global mock, so it must be
// declared here too or the hook throws "useParams is not a function".
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
  useParams: () => ({}),
  // BusinessSettings reads `?section=` to seed its initial sub-tab (notifications
  // deep link). A file-level mock fully overrides jest.setup.js, so include it.
  useSearchParams: () => new URLSearchParams(),
  // useUrlState (?section= sync) reads the pathname to rebuild the URL.
  usePathname: () => "/business/1/dashboard",
}));

// Stub next/dynamic — BusinessSettings lazy-loads PaymentSettingsTab and
// NotificationPreferencesTab via dynamic(); return lightweight stubs so the
// shell renders in isolation.
jest.mock("next/dynamic", () => ({
  __esModule: true,
  default: () => () => <div data-testid="dynamic-stub" />,
}));

// Stub heavy child sub-tabs so the shell renders in isolation.
jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design" />,
}));
// CurrencySettings uses forwardRef; jest.mock factories are hoisted before
// imports so React isn't in scope. Use jest.requireActual's React inside.
jest.mock("@/components/business/CurrencySettings", () => {
  const r = jest.requireActual("react") as typeof import("react");
  const Stub = r.forwardRef<
    { save: () => Promise<void> },
    { businessId: number }
  >((_props, _ref) => r.createElement("div", { "data-testid": "currency" }));
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

async function editBusinessNameAndSave(name: string) {
  const input = await screen.findByDisplayValue("Demo");
  fireEvent.change(input, { target: { value: name } });
  await waitFor(() => expect(input).toHaveValue(name));
  // SaveBar disables while clean (Finding 44) — wait for the edit to dirty.
  const save = await screen.findByRole("button", { name: /save/i });
  await waitFor(() => expect(save).toBeEnabled());
  fireEvent.click(save);
}

describe("BusinessSettings locale reactivity", () => {
  beforeEach(() => {
    mockLocale = "en";
    mockTierAccess = true;
    mockTierSuspended = false;
    const { businessApi } = require("@/api/business");
    businessApi.updateBusiness.mockClear();
    businessApi.updateBusinessDesignSettings.mockClear();
  });

  it("re-translates tab labels when the locale changes without remount", async () => {
    const { rerender } = render(<BusinessSettings businessId={1} />);
    // en: businessSettings.tabs.profile = "Profile & Contact"
    // The tab bar renders this in both the desktop button and the mobile select;
    // use findAllByText so multiple matches don't throw.
    const enLabels = await screen.findAllByText("Profile & Contact");
    expect(enLabels.length).toBeGreaterThan(0);

    mockLocale = "es";
    rerender(<BusinessSettings businessId={1} />);

    // es: businessSettings.tabs.profile = "Perfil y Contacto"
    const esLabels = await screen.findAllByText("Perfil y Contacto");
    expect(esLabels.length).toBeGreaterThan(0);
    // The English label must be gone after locale switch.
    expect(screen.queryAllByText("Profile & Contact")).toHaveLength(0);
  });

  it("does not render a description editor and never blanks description on save", async () => {
    const { businessApi } = require("@/api/business");
    businessApi.getBusiness.mockResolvedValue({
      name: "Demo",
      description: "Existing copy",
      custom_url: "demo",
    });

    render(<BusinessSettings businessId={1} />);
    await screen.findAllByText("Profile & Contact");

    expect(screen.queryByLabelText(/description/i)).not.toBeInTheDocument();

    await editBusinessNameAndSave("Demo Description");
    await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
    const payload = businessApi.updateBusiness.mock.calls.at(-1)[1];
    // P2-23: partial PUT omits fields this screen does not edit — backend
    // leaves description untouched (never re-sent as empty).
    expect(payload).not.toHaveProperty("description");
  });

  it("keeps timezone off the profile form so profile saves never clobber it", async () => {
    const { businessApi } = require("@/api/business");
    businessApi.getBusiness.mockResolvedValue({
      name: "Demo",
      timezone: "America/New_York",
      custom_url: "demo",
    });

    render(<BusinessSettings businessId={1} />);
    await screen.findAllByText("Profile & Contact");
    // Timezone lives on Localization (CurrencySettings), not Profile & Contact.
    expect(screen.queryByTestId("venue-timezone-select")).not.toBeInTheDocument();

    await editBusinessNameAndSave("Demo Timezone");
    await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
    const payload = businessApi.updateBusiness.mock.calls.at(-1)[1];
    // P2-23: timezone is not on the profile form; omit so the backend preserves it.
    expect(payload).not.toHaveProperty("timezone");
  });

  it("does not render an operating-hours editor in Settings", async () => {
    const { businessApi } = require("@/api/business");
    businessApi.getBusiness.mockResolvedValue({
      name: "Demo",
      custom_url: "demo",
    });
    render(<BusinessSettings businessId={1} />);
    await screen.findAllByText("Profile & Contact");
    expect(screen.queryByText(/operating hours/i)).not.toBeInTheDocument();
  });

  it("does not show unsaved changes on first load (#224)", async () => {
    render(<BusinessSettings businessId={1} />);
    await screen.findByDisplayValue("Demo");
    expect(screen.getByTestId("save-bar-all-saved")).toHaveTextContent(
      /No unsaved changes/i,
    );
    expect(screen.queryByText(/saved automatically/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/You have unsaved changes/i)).not.toBeInTheDocument();
  });

  it("locks profile mutations and removes the save action when the business is suspended", async () => {
    mockTierAccess = false;
    mockTierSuspended = true;

    render(<BusinessSettings businessId={1} />);

    expect(await screen.findByTestId("locked")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Demo")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /save/i }),
    ).not.toBeInTheDocument();
  });
});
