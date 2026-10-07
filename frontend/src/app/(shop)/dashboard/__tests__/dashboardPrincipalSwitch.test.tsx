/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getMyBusinesses } from "@/api/business";

// A principal switch on the same tab (sign out, then another owner signs in)
// must refetch the venue list and never keep rendering the previous owner's.

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn(), back: jest.fn(), push: jest.fn() }),
  useSearchParams: () => new URLSearchParams("venues=all"),
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: null, isConnected: false }),
}));

type MockAuth = {
  isOAuthUser: boolean;
  isWeb3User: boolean;
  isStaffUser: boolean;
  isInitialized: boolean;
  oauthData: { userId: number; email: string } | null;
  sessionInfoError: null;
};
let mockAuth: MockAuth;
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

function signedInAs(userId: number): MockAuth {
  return {
    isOAuthUser: true,
    isWeb3User: false,
    isStaffUser: false,
    isInitialized: true,
    oauthData: { userId, email: `owner${userId}@example.com` },
    sessionInfoError: null,
  };
}

function signedOut(): MockAuth {
  return {
    isOAuthUser: false,
    isWeb3User: false,
    isStaffUser: false,
    isInitialized: true,
    oauthData: null,
    sessionInfoError: null,
  };
}

const ownerOneVenues = [
  { id: 1, business_id: "alpha-bistro", name: "Alpha Bistro" },
  { id: 2, business_id: "alpha-cafe", name: "Alpha Cafe" },
];
const ownerTwoVenues = [
  { id: 3, business_id: "beta-grill", name: "Beta Grill" },
  { id: 4, business_id: "beta-bar", name: "Beta Bar" },
];

describe("dashboard principal switch", () => {
  const queryClient = new QueryClient();
  const ui = () => (
    <QueryClientProvider client={queryClient}>
      <DashboardPage />
    </QueryClientProvider>
  );

  beforeEach(() => {
    mockGetMyBusinesses.mockReset();
  });

  it("refetches for the new principal and drops the previous list", async () => {
    mockAuth = signedInAs(1);
    mockGetMyBusinesses.mockResolvedValueOnce(ownerOneVenues);
    const { rerender } = render(ui());
    expect(await screen.findByText("Alpha Bistro")).toBeInTheDocument();
    expect(mockGetMyBusinesses).toHaveBeenCalledTimes(1);

    let resolveSecond: (v: typeof ownerTwoVenues) => void = () => {};
    mockGetMyBusinesses.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSecond = resolve;
      }),
    );
    mockAuth = signedInAs(2);
    rerender(ui());

    // The old owner's venues are gone before the new list arrives.
    await waitFor(() => expect(mockGetMyBusinesses).toHaveBeenCalledTimes(2));
    expect(screen.queryByText("Alpha Bistro")).toBeNull();
    expect(screen.queryByText("Alpha Cafe")).toBeNull();

    await act(async () => {
      resolveSecond(ownerTwoVenues);
    });
    expect(await screen.findByText("Beta Grill")).toBeInTheDocument();
    expect(screen.queryByText("Alpha Bistro")).toBeNull();
  });

  it("clears the list when the principal signs out", async () => {
    mockAuth = signedInAs(1);
    mockGetMyBusinesses.mockResolvedValueOnce(ownerOneVenues);
    const { rerender } = render(ui());
    expect(await screen.findByText("Alpha Bistro")).toBeInTheDocument();

    mockAuth = signedOut();
    rerender(ui());

    await waitFor(() =>
      expect(
        screen.getByText("dashboard.authentication.title"),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByText("Alpha Bistro")).toBeNull();
    expect(mockGetMyBusinesses).toHaveBeenCalledTimes(1);
  });

  it("drops a load that resolves after the principal changed", async () => {
    mockAuth = signedInAs(1);
    let resolveFirst: (v: typeof ownerOneVenues) => void = () => {};
    mockGetMyBusinesses.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFirst = resolve;
      }),
    );
    const { rerender } = render(ui());
    await waitFor(() => expect(mockGetMyBusinesses).toHaveBeenCalledTimes(1));

    mockGetMyBusinesses.mockResolvedValueOnce(ownerTwoVenues);
    mockAuth = signedInAs(2);
    rerender(ui());
    expect(await screen.findByText("Beta Grill")).toBeInTheDocument();

    await act(async () => {
      resolveFirst(ownerOneVenues);
    });
    expect(screen.queryByText("Alpha Bistro")).toBeNull();
    expect(screen.getByText("Beta Grill")).toBeInTheDocument();
  });
});
