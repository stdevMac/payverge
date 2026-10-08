/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getMyBusinesses } from "@/api/business";

// A staff session (login code, PIN or accepted invitation) belongs to one
// venue. /inside/businesses is owner-only and refuses it, so /dashboard must
// send staff to their venue instead of the "Authentication Required" gate.

const mockRouterReplace = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: mockRouterReplace,
    back: jest.fn(),
    push: jest.fn(),
  }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: null, isConnected: false }),
}));

let mockAuth: Record<string, unknown> = {};
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockAuth,
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: Object.assign(() => ({ user: null }), {
    getState: () => ({ setUser: jest.fn() }),
  }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useAnalytics", () => ({
  usePageTracking: () => undefined,
  useClickTracking: () => jest.fn(),
  useConversionTracking: () => jest.fn(),
}));

jest.mock("@/api/business", () => ({ getMyBusinesses: jest.fn() }));

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getDashboardSummary: jest.fn().mockResolvedValue({}),
    getDashboardSummaries: jest.fn().mockResolvedValue({}),
    getTimeseries: jest.fn().mockResolvedValue({ buckets: [] }),
  },
}));

import DashboardPage from "../page";

const mockGetMyBusinesses = getMyBusinesses as jest.Mock;

function renderDashboard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardPage />
    </QueryClientProvider>,
  );
}

const staffBase = {
  isOAuthUser: false,
  isWeb3User: false,
  isStaffUser: true,
  isInitialized: true,
  oauthData: null,
  sessionInfoError: null,
};

describe("dashboard for a staff session", () => {
  beforeEach(() => {
    mockRouterReplace.mockReset();
    mockGetMyBusinesses.mockReset();
  });

  it("goes straight to the staff member's venue by slug", async () => {
    mockAuth = {
      ...staffBase,
      staffData: {
        id: 7,
        business_id: 2,
        business_slug: "parrilla-quebracho-azul",
      },
    };

    renderDashboard();

    await waitFor(() =>
      expect(mockRouterReplace).toHaveBeenCalledWith(
        "/business/parrilla-quebracho-azul/dashboard",
      ),
    );
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
    expect(screen.queryByText("dashboard.authentication.title")).toBeNull();
  });

  it("falls back to the numeric venue id without a slug", async () => {
    mockAuth = { ...staffBase, staffData: { id: 7, business_id: 2 } };

    renderDashboard();

    await waitFor(() =>
      expect(mockRouterReplace).toHaveBeenCalledWith("/business/2/dashboard"),
    );
  });

  it("keeps the venue list when the person is also a signed-in owner", async () => {
    mockAuth = {
      ...staffBase,
      isOAuthUser: true,
      oauthData: { email: "owner@example.com" },
      staffData: { id: 7, business_id: 2 },
    };
    mockGetMyBusinesses.mockResolvedValue([
      { id: 1, business_id: "a", name: "A" },
      { id: 2, business_id: "b", name: "B" },
    ]);

    renderDashboard();

    await waitFor(() => expect(mockGetMyBusinesses).toHaveBeenCalled());
    expect(mockRouterReplace).not.toHaveBeenCalled();
  });
});
