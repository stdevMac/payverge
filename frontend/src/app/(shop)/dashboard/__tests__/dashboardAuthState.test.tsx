/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getMyBusinesses } from "@/api/business";
import { getBusinessDashboardPath } from "@/utils/businessUrl";

// --- Provider harness (mirrors business/register wallet-auth-guard.test.tsx) ---

const mockRouterPush = jest.fn();
const mockRouterReplace = jest.fn();
let mockSearchParams = new URLSearchParams();

jest.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: mockRouterReplace,
    back: jest.fn(),
    push: mockRouterPush,
  }),
  useSearchParams: () => mockSearchParams,
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: null, isConnected: false }),
}));

// Signed-in OAuth user: the dashboard effect will call loadBusinesses().
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isOAuthUser: true,
    isWeb3User: false,
    isStaffUser: false,
    isInitialized: true,
    oauthData: { email: "owner@example.com" },
    sessionInfoError: null,
  }),
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: Object.assign(
    () => ({ user: { id: 1 } }),
    { getState: () => ({ setUser: jest.fn() }) },
  ),
}));

// getTranslation passthrough → rendered text is the raw i18n key. We match
// against those keys (stable regardless of locale bundle contents).
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

// Empty-state signals (raw i18n keys under the passthrough translation mock).
const EMPTY_STATE_TITLE = "dashboard.businesses.noneTitle";
const EMPTY_STATE_CTA = "dashboard.businesses.createFirst";
// Sign-in / authentication UI signal — the "Authentication Required" heading
// renders on BOTH the OAuth ("Try Again") and not-signed-in ("Sign In / Sign
// Up") variants of the auth branch.
const SIGN_IN_CTA = "dashboard.authentication.title";

describe("dashboard auth states", () => {
  beforeEach(() => {
    mockGetMyBusinesses.mockReset();
    mockRouterPush.mockReset();
    mockRouterReplace.mockReset();
    mockSearchParams = new URLSearchParams();
  });

  it("sends a signed-in operator with exactly one venue straight to its dashboard", async () => {
    const business = { id: 42, business_id: "acme-bistro", name: "Acme Bistro" };
    mockGetMyBusinesses.mockResolvedValue([business]);

    renderDashboard();

    await waitFor(() =>
      expect(mockRouterReplace).toHaveBeenCalledWith(
        getBusinessDashboardPath(business),
      ),
    );
    // The picker never flashes on the way through.
    expect(screen.queryByText("dashboard.overview.openBusiness")).toBeNull();
    expect(screen.queryByText(SIGN_IN_CTA)).toBeNull();
  });

  it("keeps the picker for an operator with several venues", async () => {
    mockGetMyBusinesses.mockResolvedValue([
      { id: 42, business_id: "acme-bistro", name: "Acme Bistro" },
      { id: 43, business_id: "acme-cafe", name: "Acme Cafe" },
    ]);

    renderDashboard();

    expect(
      (await screen.findAllByText("dashboard.overview.openBusiness")).length,
    ).toBe(2);
    expect(mockRouterReplace).not.toHaveBeenCalled();
  });

  it("shows ONLY the sign-in prompt on 401 — never the empty state CTA", async () => {
    mockGetMyBusinesses.mockRejectedValue({
      response: { status: 401 },
      status: 401,
      message: "Request failed with status code 401",
    });

    renderDashboard();

    await waitFor(() =>
      expect(screen.getByText(SIGN_IN_CTA)).toBeInTheDocument(),
    );

    expect(screen.queryByText(EMPTY_STATE_TITLE)).toBeNull();
    expect(screen.queryByText(EMPTY_STATE_CTA)).toBeNull();
  });

  it("shows ONLY the sign-in prompt on 403 — never the empty state CTA", async () => {
    mockGetMyBusinesses.mockRejectedValue({
      response: { status: 403 },
      status: 403,
      message: "Request failed with status code 403",
    });

    renderDashboard();

    await waitFor(() =>
      expect(screen.getByText(SIGN_IN_CTA)).toBeInTheDocument(),
    );

    expect(screen.queryByText(EMPTY_STATE_TITLE)).toBeNull();
    expect(screen.queryByText(EMPTY_STATE_CTA)).toBeNull();
  });

  it("shows the empty state only after a successful empty fetch", async () => {
    mockGetMyBusinesses.mockResolvedValue([]);

    renderDashboard();

    expect(await screen.findByText(EMPTY_STATE_TITLE)).toBeInTheDocument();
    expect(screen.getByText(EMPTY_STATE_CTA)).toBeInTheDocument();
    // The sign-in UI must NOT appear on a successful (if empty) load.
    expect(screen.queryByText(SIGN_IN_CTA)).toBeNull();
  });

  it("Open business navigates via router.push through the slug-aware helper (audit L2)", async () => {
    const business = { id: 42, business_id: "acme-bistro", name: "Acme Bistro" };
    mockGetMyBusinesses.mockResolvedValue([business]);
    // ?venues=all keeps the overview for a single-venue operator.
    mockSearchParams = new URLSearchParams("venues=all");

    renderDashboard();

    expect(mockRouterReplace).not.toHaveBeenCalled();
    const openBtn = await screen.findByText("dashboard.overview.openBusiness");

    // Keyboard accessibility: it must remain a real <button>, not an <a>.
    expect(openBtn.closest("button")).not.toBeNull();

    fireEvent.click(openBtn);

    // Navigation goes through router.push using the slug-aware helper — no
    // hand-built path — so the pending state (useTransition) can gate re-taps.
    expect(mockRouterPush).toHaveBeenCalledTimes(1);
    expect(mockRouterPush).toHaveBeenCalledWith(
      getBusinessDashboardPath(business),
    );
  });

  it("combined All-businesses view labels today and does not ask to select a business", async () => {
    const { analyticsApi } = jest.requireMock("@/api/analytics") as {
      analyticsApi: {
        getDashboardSummaries: jest.Mock;
      };
    };
    analyticsApi.getDashboardSummaries.mockResolvedValue({
      "42": {
        today: { revenue: 0, tips: 0, transactions: 0, bills: 0 },
        week: {
          revenue: 5000,
          tips: 0,
          transactions: 10,
          bills: 10,
          unique_customers: 1,
          average_ticket: 500,
        },
        live: { active_bills: 0 },
        top_items: [],
      },
    });
    mockGetMyBusinesses.mockResolvedValue([
      {
        id: 42,
        business_id: "acme-bistro",
        name: "Acme Bistro",
        default_currency: "USD",
        is_active: true,
      },
    ]);
    mockSearchParams = new URLSearchParams("venues=all");

    renderDashboard();

    expect(
      await screen.findByText("dashboard.overview.grainToday"),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("dashboard.overview.noActivityToday"),
    ).toBeInTheDocument();
    expect(screen.getByText("dashboard.overview.weekRevenue")).toBeInTheDocument();
    expect(
      screen.queryByText("dashboard.overview.selectBusinessHint"),
    ).toBeNull();
  });
});
