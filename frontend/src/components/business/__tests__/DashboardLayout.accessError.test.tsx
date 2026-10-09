/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import DashboardLayout from "../DashboardLayout";

function renderShell(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

jest.mock("../DashboardSidebar", () => ({
  __esModule: true,
  default: () => <aside data-testid="sidebar" />,
}));

jest.mock("../commandPalette/CommandPaletteProvider", () => ({
  CommandPaletteProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  useCommandPalette: () => ({
    open: false,
    openPalette: () => {},
    closePalette: () => {},
    toggle: () => {},
    ready: false,
  }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(() => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: true,
    refetch: async () => {},
  })),
}));

jest.mock("@/components/auth/AuthModal", () => ({
  AuthModal: () => null,
  __esModule: true,
  default: () => null,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    oauthData: { userId: 9, email: "qa@example.com", role: "user" },
    staffData: null,
    walletAddress: null,
  }),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

const layoutProps = {
  business: null,
  loading: false,
  authLoading: false,
  activeTab: "overview",
  setActiveTab: () => {},
  sidebarOpen: false,
  setSidebarOpen: () => {},
  isConnected: true,
};

describe("DashboardLayout forbidden state", () => {
  afterEach(() => {
    delete document.documentElement.dataset.operatorAccessError;
  });

  it("shows the named venue and Back to businesses instead of a marketing crash card", () => {
    renderShell(
      <DashboardLayout
        {...layoutProps}
        error="business:forbidden"
        venueName="Payverge AI Pro Demo Lounge"
      >
        <div />
      </DashboardLayout>,
    );

    expect(screen.getByTestId("business-access-error")).toBeInTheDocument();
    expect(
      screen.getAllByText("Payverge AI Pro Demo Lounge").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByText(
        "businessDashboard.error.venueAccessDenied".replace(
          "{name}",
          "Payverge AI Pro Demo Lounge",
        ),
      ),
    ).toBeInTheDocument();
    const back = screen.getByRole("link", {
      name: "businessDashboard.error.backToBusinesses",
    });
    // Plain /dashboard would bounce a single-venue operator straight back.
    expect(back).toHaveAttribute("href", "/dashboard?venues=all");
    expect(screen.queryByText("businessDashboard.error.title")).toBeNull();
    expect(screen.queryByText("businessDashboard.error.retry")).toBeNull();
    expect(screen.queryByRole("link", { name: /home/i })).toBeNull();
    expect(screen.queryByText(/install/i)).toBeNull();
  });
});
