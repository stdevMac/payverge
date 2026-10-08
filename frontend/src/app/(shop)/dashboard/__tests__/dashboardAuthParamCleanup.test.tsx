/** @jest-environment jsdom */
import React from "react";
import { render, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getMyBusinesses } from "@/api/business";

// `?auth=signin` (the demo "Enter" link, /login alias) only asks for the
// sign-in dialog. Once the visitor has a session it must leave the URL, or a
// reload reopens the dialog over the dashboard they just reached.

const mockRouterReplace = jest.fn();
let mockSearch = "auth=signin";

jest.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: mockRouterReplace,
    back: jest.fn(),
    push: jest.fn(),
  }),
  useSearchParams: () => new URLSearchParams(mockSearch),
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

const ownerSession = {
  isOAuthUser: true,
  isWeb3User: false,
  isStaffUser: false,
  isInitialized: true,
  oauthData: { email: "owner@example.com" },
  sessionInfoError: null,
  staffData: null,
};

const twoVenues = [
  { id: 1, name: "Uno", custom_url: "uno" },
  { id: 2, name: "Dos", custom_url: "dos" },
];

describe("dashboard ?auth= cleanup", () => {
  beforeEach(() => {
    mockRouterReplace.mockReset();
    mockGetMyBusinesses.mockReset().mockResolvedValue(twoVenues);
  });

  it("drops ?auth=signin once the owner is signed in", async () => {
    mockSearch = "auth=signin";
    mockAuth = ownerSession;
    renderDashboard();
    await waitFor(() =>
      expect(mockRouterReplace).toHaveBeenCalledWith("/dashboard"),
    );
  });

  it("keeps the other query params", async () => {
    mockSearch = "auth=signin&all=1";
    mockAuth = ownerSession;
    renderDashboard();
    await waitFor(() =>
      expect(mockRouterReplace).toHaveBeenCalledWith("/dashboard?all=1"),
    );
  });

  it("leaves the URL alone while signed out", async () => {
    mockSearch = "auth=signin";
    mockAuth = { ...ownerSession, isOAuthUser: false, oauthData: null };
    renderDashboard();
    await new Promise((r) => setTimeout(r, 50));
    expect(mockRouterReplace).not.toHaveBeenCalled();
  });
});
