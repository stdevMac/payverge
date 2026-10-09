/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, fireEvent } from "@testing-library/react";

const mockRouter = { replace: jest.fn(), back: jest.fn(), push: jest.fn() };
jest.mock("next/navigation", () => ({
  useRouter: () => mockRouter,
  useSearchParams: () =>
    new URLSearchParams("step=auth"),
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: undefined, isConnected: false }),
}));

// ANONYMOUS visitor: neither OAuth nor Web3 principal resolved. This is the
// exact state an email/password signup left the funnel in before AuthModal
// hydrated the provider (P0-1) — the guard must bounce to the auth modal,
// never call checkout. The existing page.submitGuard.test.tsx only covers
// isOAuthUser: true.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isOAuthUser: false,
    isWeb3User: false,
    isStaffUser: false,
    oauthData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

jest.mock("@/store/useUserStore", () => {
  const setUser = jest.fn();
  return {
    useUserStore: Object.assign(() => ({ user: null }), {
      getState: () => ({ setUser }),
    }),
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useAnalytics", () => ({
  usePageTracking: () => undefined,
  useClickTracking: () => jest.fn(),
  useConversionTracking: () => jest.fn(),
}));

jest.mock("@/api/users/profile", () => ({
  getUserProfile: jest.fn().mockResolvedValue(null),
}));

// Business-step validation is covered by page.submitGuard.test.tsx; let the
// submit reach the auth guard.
jest.mock("../_stepValidation", () => ({
  isBusinessStepComplete: () => true,
  isValidEmail: () => true,
}));

// Stub AuthModal: the real one mounts a NextUI Modal (slow in jsdom) and its
// internals are covered by AuthModal.funnelSession.test.tsx. Here we only
// observe that the guard OPENED it.
jest.mock("@/components/auth/AuthModal", () => ({
  AuthModal: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="auth-modal-open" /> : null,
}));

const mockCreateBusiness = jest.fn();
const mockGetMyBusinesses = jest.fn().mockResolvedValue([]);
jest.mock("@/api/business", () => ({
  createBusiness: (...args: unknown[]) => mockCreateBusiness(...args),
  // MIN-3: empty list keeps anonymous/register-guard tests on the wizard.
  getMyBusinesses: (...args: unknown[]) => mockGetMyBusinesses(...args),
}));

import BusinessRegisterPage from "../page";

beforeEach(() => {
  jest.clearAllMocks();
  try {
    localStorage.clear();
    sessionStorage.clear();
  } catch {}
});

describe("business/register — anonymous workspace creation is guarded", () => {
  it("requires account creation without creating a workspace or checkout", async () => {
    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    // The account step is the final pre-workspace boundary for a visitor.
    expect(
      screen.getByText("businessRegister.authStep.title"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByText("businessRegister.authStep.cta"));

    expect(mockCreateBusiness).not.toHaveBeenCalled();
    expect(screen.getByTestId("auth-modal-open")).toBeInTheDocument();
  });
});
