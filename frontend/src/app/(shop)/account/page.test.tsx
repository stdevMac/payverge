/** @jest-environment jsdom */
/**
 * /account @ 390 clipped the third tab to "Privacy &" because the strip
 * was a nowrap row with overflow-x-auto. The tablist now wraps so the
 * full "Privacy & security" label stays readable.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import AccountPage from "./page";

const mockReplace = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace, push: jest.fn(), back: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    oauthData: { email: "owner@example.com" },
    isOAuthUser: true,
    isStaffUser: false,
    isWeb3User: false,
    isInitialized: true,
  }),
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: () => ({ user: { email: "owner@example.com" } }),
}));

jest.mock("@/api/business", () => ({
  getMyBusinesses: jest.fn().mockResolvedValue([]),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const labels: Record<string, string> = {
      "account.title": "Account",
      "account.subtitle": "Manage your account settings and privacy.",
      "account.backToDashboard": "Back to dashboard",
      "account.tabs.account": "Account",
      "account.tabs.notifications": "Notifications",
      "account.tabs.privacy": "Privacy & security",
      "account.info.title": "Your information",
      "account.info.email": "Email",
    };
    return labels[key] ?? key;
  },
}));

jest.mock("@/components/account/AccountPrivacyPanel", () => ({
  AccountPrivacyPanel: () => <div data-testid="privacy-panel" />,
}));
jest.mock("@/components/account/AccountNotificationsSection", () => ({
  AccountNotificationsSection: () => (
    <div data-testid="notifications-section" />
  ),
}));
jest.mock("@/components/account/AccountEmailPreferences", () => ({
  AccountEmailPreferences: () => <div data-testid="email-prefs" />,
}));
jest.mock("@/components/ui/AsyncState", () => ({
  RouteLoadingFallback: () => <div data-testid="account-loading" />,
}));

describe("account page tabs", () => {
  it("renders the full Privacy & security tab label", async () => {
    render(<AccountPage />);

    const privacy = await screen.findByTestId("account-tab-privacy");
    expect(privacy).toHaveTextContent("Privacy & security");
    expect(privacy).not.toHaveTextContent(/^Privacy &$/);
  });

  it("wraps the tablist instead of clipping the third tab off-canvas", async () => {
    render(<AccountPage />);

    const tablist = await screen.findByTestId("account-tablist");
    expect(tablist.className).toMatch(/flex-wrap/);
    expect(tablist.className).not.toMatch(/overflow-x-auto/);
    expect(screen.getByTestId("account-tab-privacy").className).toMatch(
      /shrink-0/,
    );
    expect(screen.getByRole("tab", { name: "Account" })).toBeInTheDocument();
    expect(
      screen.getByRole("tab", { name: "Notifications" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("tab", { name: "Privacy & security" }),
    ).toBeInTheDocument();
  });
});
