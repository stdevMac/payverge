/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import DashboardLayout from "../DashboardLayout";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

// DashboardLayout calls useBusinessAccess (React Query) directly, so the shell
// must render inside a QueryClientProvider. Disable retries so the stubbed
// access fetch never schedules background work during the smoke render.
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

// The ⌘K provider calls useBusinessAccess (React Query); this shell smoke test
// doesn't stand up a QueryClient. Stub it to a passthrough — the palette has
// its own dedicated test suite. The trigger pill still resolves its (no-op)
// context via the stubbed useCommandPalette export.
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

// The M4 MobileTodayBar derives rail badges via useRailBadges → these two
// hooks. The chat one opens an SSE EventSource that jsdom lacks — stub both
// (same pattern as the DashboardSidebar suites).
jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

// DashboardLayout calls useBusinessAccess, whose queryFn would otherwise fire a
// real fetch (and log an error) under jsdom. Stub it to a settled active business so
// the shell renders deterministically without leaking a background request.
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
  getTranslation: (_key: string, _locale: string) => _key,
}));

// DashboardLayout now mounts an OfflineQueueDrain that calls useOfflineMutation,
// which reads useToast. Provide a toast stub so the shell renders without a
// ToastProvider wrapper.
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

describe("DashboardLayout", () => {
  const mockUseBusinessAccess = useBusinessAccess as jest.Mock;
  const business = {
    id: 1,
    name: "Test",
    custom_url: "test",
  } as any;

  beforeEach(() => {
    mockUseBusinessAccess.mockClear();
  });

  it("renders children and shell when business is present", () => {
    const { container, getByTestId, getByText } = renderShell(
      <DashboardLayout
        business={business}
        loading={false}
        authLoading={false}
        error={null}
        activeTab="overview"
        setActiveTab={() => {}}
        sidebarOpen={false}
        setSidebarOpen={() => {}}
        isConnected
      >
        <div>tab content</div>
      </DashboardLayout>,
    );
    expect(getByTestId("sidebar")).toBeTruthy();
    expect(getByText("tab content")).toBeTruthy();
    expect(getByTestId("dashboard-shell")).toBeTruthy();
    // The (shop) layout owns the page's one <main id="main-content">; the
    // shell must not nest a second landmark or duplicate the id.
    expect(container.querySelector("main")).toBeNull();
    expect(container.querySelector("#main-content")).toBeNull();
    const content = container.querySelector("[data-dashboard-scroller]");
    expect(content).toHaveAttribute("tabindex", "0");
    expect(content?.className).toMatch(/min-w-0/);
    expect(mockUseBusinessAccess).toHaveBeenCalledWith(1);
    expect(screen.queryByTestId("ops-assistant-fab")).not.toBeInTheDocument();
  });

  // Cross-lane note from the #657 strip: the shell root is
  // `h-[100dvh] overflow-hidden` and the content <div> is the real scroller, so
  // `window.scrollY` is pinned at 0 and `document.scrollHeight` is ~viewport on
  // EVERY operator tab. A QA harness measuring the window will report "page
  // cannot scroll" for every tab forever — an artifact of the fixed-sidebar
  // shell, not a per-tab defect. `#main-content` is not a usable hook either:
  // it is the (shop) layout's outer, non-scrolling <main> landmark.
  // `[data-dashboard-scroller]` is the unambiguous handle; it is a data
  // attribute, so it changes no layout.
  it("exposes the real scroll container behind a stable hook", () => {
    const { container, getByTestId } = renderShell(
      <DashboardLayout
        business={business}
        loading={false}
        authLoading={false}
        error={null}
        activeTab="overview"
        setActiveTab={() => {}}
        sidebarOpen={false}
        setSidebarOpen={() => {}}
        isConnected
      >
        <div>tab content</div>
      </DashboardLayout>,
    );

    const scrollers = container.querySelectorAll("[data-dashboard-scroller]");
    expect(scrollers).toHaveLength(1);

    const scroller = scrollers[0];
    expect(scroller.tagName).toBe("DIV");
    expect(scroller.className).toMatch(/overflow-auto/);
    expect(scroller.className).toMatch(/min-h-0/);

    // The window-pinning half of the artifact, asserted as intended shell
    // behavior so nobody "fixes" it to satisfy a probe.
    const shell = getByTestId("dashboard-shell");
    expect(shell.className).toMatch(/overflow-hidden/);
    expect(shell.className).toMatch(/h-\[100dvh\]/);
    expect(shell.contains(scroller)).toBe(true);
  });

  it("keeps the last-good shell when a business refetch flakes (#773)", () => {
    const { getByTestId, getByText, queryByText } = renderShell(
      <DashboardLayout
        business={business}
        loading={false}
        authLoading={false}
        error="business:generic"
        activeTab="tables"
        setActiveTab={() => {}}
        sidebarOpen={false}
        setSidebarOpen={() => {}}
        isConnected
      >
        <div>tables content</div>
      </DashboardLayout>,
    );

    expect(getByTestId("dashboard-shell")).toBeTruthy();
    expect(getByText("tables content")).toBeTruthy();
    expect(queryByText("businessDashboard.error.title")).toBeNull();
    expect(queryByText("businessDashboard.error.loadFailed")).toBeNull();
  });

  it("renders without crashing when business=null and loading=true", () => {
    const { container } = renderShell(
      <DashboardLayout
        business={null}
        loading
        authLoading={false}
        error={null}
        activeTab="overview"
        setActiveTab={() => {}}
        sidebarOpen={false}
        setSidebarOpen={() => {}}
        isConnected={false}
      >
        <div />
      </DashboardLayout>,
    );
    expect(container).toBeTruthy();
    expect(mockUseBusinessAccess).toHaveBeenCalledWith(undefined);
  });
});
