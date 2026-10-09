/** @jest-environment jsdom */
/**
 * #35 — Marketing footer "Dashboard" must not flash the authenticated
 * "Loading your businesses…" shell for anonymous visitors while session
 * bootstrap settles / before the sign-in gate paints.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getMyBusinesses } from "@/api/business";

const mockRouterPush = jest.fn();
const authState = {
  isOAuthUser: false,
  isWeb3User: false,
  isStaffUser: false,
  isInitialized: false,
  oauthData: null as null | { email: string },
  sessionInfoError: null as null | Error,
};

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn(), back: jest.fn(), push: mockRouterPush }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: null, isConnected: false }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ ...authState }),
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: Object.assign(
    () => ({ user: null }),
    { getState: () => ({ setUser: jest.fn() }) },
  ),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ fill, priority, ...props }: { fill?: boolean; priority?: boolean; alt?: string }) =>
    require("react").createElement("img", { ...props, alt: props.alt ?? "" }),
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

describe("dashboard anonymous footer flash (#35)", () => {
  beforeEach(() => {
    mockGetMyBusinesses.mockReset();
    mockRouterPush.mockReset();
    authState.isInitialized = false;
    authState.isOAuthUser = false;
    authState.isWeb3User = false;
    authState.isStaffUser = false;
  });

  it("does not show Loading your businesses while session is bootstrapping", () => {
    renderDashboard();
    expect(screen.queryByText("dashboard.loading")).toBeNull();
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
  });

  it("shows the sign-in UI immediately once initialized with no auth surface", async () => {
    authState.isInitialized = true;
    renderDashboard();
    expect(
      await screen.findByText("dashboard.authentication.title"),
    ).toBeInTheDocument();
    expect(screen.queryByText("dashboard.loading")).toBeNull();
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
  });
});
