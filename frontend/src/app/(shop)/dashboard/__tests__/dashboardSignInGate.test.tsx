/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getMyBusinesses } from "@/api/business";

// --- Provider harness (copied from dashboardAuthState.test.tsx) ---

const mockRouterPush = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn(), back: jest.fn(), push: mockRouterPush }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: null, isConnected: false }),
}));

// No auth surface active: OAuth/staff/web3 all false, initialized true.
// Matches a logged-out visitor so the dashboard effect hits the fallthrough
// that sets the connect-wallet error key (raw under passthrough translations).
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isOAuthUser: false,
    isWeb3User: false,
    isStaffUser: false,
    isInitialized: true,
    oauthData: null,
    sessionInfoError: null,
  }),
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: Object.assign(
    () => ({ user: null }),
    { getState: () => ({ setUser: jest.fn() }) },
  ),
}));

// getTranslation passthrough → rendered text is the raw i18n key. We match
// against those keys (stable regardless of locale bundle contents).
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    if (key === "dashboard.authentication.valueProp.bullets") {
      return [
        { title: "Director Console", description: "Owner-level decisions.", feature: "ai" },
        { title: "AI Waiter", description: "Menu-aware guest service.", feature: "ai" },
        { title: "Payments go straight to you", description: "Nothing sits in between." },
      ];
    }
    return key;
  },
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
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import { CONSENT_STORAGE_KEY, writeConsent } from "@/lib/analytics/consentGate";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

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

describe("dashboard sign-in gate", () => {
  beforeEach(() => {
    mockGetMyBusinesses.mockReset();
    mockRouterPush.mockReset();
    resetInstanceCacheForTests();
  });

  afterEach(() => window.localStorage.removeItem(CONSENT_STORAGE_KEY));

  afterAll(() => resetInstanceCacheForTests());

  it("shows the sign-in UI when no auth surface is active, independent of locale strings", async () => {
    renderDashboard();
    // Passthrough translations make the error text the raw key
    // ("dashboard.errors.connectWallet"), which contains none of the English
    // substrings the old gate matched — exactly like a translated locale.
    expect(
      await screen.findByText("dashboard.authentication.title"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("dashboard.authentication.subtitle"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "dashboard.authentication.signInOnly",
      }),
    ).toBeInTheDocument();
    // No instance info: never promise sign-up.
    expect(
      screen.queryByRole("button", {
        name: "dashboard.authentication.signInSignUp",
      }),
    ).not.toBeInTheDocument();
    // Non-OAuth branch must not repeat connectMessage under the subtitle.
    expect(
      screen.queryByText("dashboard.authentication.connectMessage"),
    ).not.toBeInTheDocument();
    // 390px: one Director / AI / fees line sits under the CTA.
    expect(screen.getByTestId("auth-value-prop-compact")).toHaveTextContent(
      "Director Console · AI Waiter · Payments go straight to you",
    );
  });

  it("drops Google and the AI bullets on an instance without them", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "invite",
        features: { google_oauth: false, ai: false },
      }),
    );
    renderDashboard();
    expect(
      await screen.findByText("dashboard.authentication.subtitleEmail"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("dashboard.authentication.subtitle"),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId("auth-value-prop-compact")).toHaveTextContent(
      /^Payments go straight to you$/,
    );
  });

  it("offers sign-up only when the instance takes open registration", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "open",
        features: {},
      }),
    );
    renderDashboard();
    expect(
      await screen.findByRole("button", {
        name: "dashboard.authentication.signInSignUp",
      }),
    ).toBeInTheDocument();
  });

  it("labels the button sign-in only on an invite instance", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "invite",
        features: {},
      }),
    );
    renderDashboard();
    expect(
      await screen.findByRole("button", {
        name: "dashboard.authentication.signInOnly",
      }),
    ).toBeInTheDocument();
  });

  it("keeps the full gate when the instance has everything on", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "invite",
        features: { google_oauth: true, ai: true },
      }),
    );
    renderDashboard();
    expect(
      await screen.findByText("dashboard.authentication.subtitle"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("auth-value-prop-compact")).toHaveTextContent(
      "Director Console · AI Waiter · Payments go straight to you",
    );
  });

  it("holds the demo dialog until the cookie banner is answered", async () => {
    window.localStorage.removeItem(CONSENT_STORAGE_KEY);
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "closed",
        features: {},
        demo: { enabled: true, mode: true, reset_utc: "03:00" },
      }),
    );
    renderDashboard();
    expect(
      await screen.findByText("dashboard.authentication.title"),
    ).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByTestId("demo-login")).not.toBeInTheDocument();
  });

  it("opens the sign-in dialog with the one-click demo buttons on the public demo", async () => {
    writeConsent({ analytics: false, marketing: false });
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "closed",
        features: {},
        demo: { enabled: true, mode: true, reset_utc: "03:00" },
      }),
    );
    renderDashboard();
    expect(await screen.findByTestId("demo-login")).toBeInTheDocument();
  });

  it("keeps the dialog closed on a normal install", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "invite",
        features: {},
        demo: { enabled: true, mode: false },
      }),
    );
    renderDashboard();
    expect(
      await screen.findByText("dashboard.authentication.title"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("demo-login")).not.toBeInTheDocument();
  });
});
