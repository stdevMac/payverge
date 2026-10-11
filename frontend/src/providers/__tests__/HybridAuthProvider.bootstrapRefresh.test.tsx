/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, waitFor } from "@testing-library/react";
import { HybridAuthProvider } from "../HybridAuthProvider";
import { SESSION_HINT_KEY, type RefreshAuthResult } from "@/utils/refreshAuth";

const mockRefreshAuthSession = jest.fn();
const mockGetSessionInfo = jest.fn();
const mockGetCurrentUser = jest.fn();
const mockSetUser = jest.fn();
const mockClearUser = jest.fn();

jest.mock("@/utils/refreshAuth", () => ({
  SESSION_HINT_KEY: "payverge_had_session",
  refreshAuthSession: (...args: unknown[]) => mockRefreshAuthSession(...args),
}));

jest.mock("@/api/auth/sessionInfo", () => ({
  getSessionInfo: (...args: unknown[]) => mockGetSessionInfo(...args),
}));

jest.mock("@/api/auth", () => ({
  getCurrentUser: (...args: unknown[]) => mockGetCurrentUser(...args),
  authAPI: {
    getWalletChallenge: jest.fn(),
    linkWallet: jest.fn(),
    unlinkWallet: jest.fn(),
  },
}));

jest.mock("@/store/useUserStore", () => {
  const useUserStore = Object.assign(
    () => ({ setUser: mockSetUser, clearUser: mockClearUser, user: null }),
    { getState: () => ({ user: null }) },
  );
  return { useUserStore };
});

jest.mock("@/providers/walletBridgeStore", () => ({
  useWalletBridge: () => ({
    available: true,
    address: undefined,
    isConnected: false,
    chainId: 1,
    signMessageAsync: jest.fn(),
  }),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn() }),
  usePathname: () => "/dashboard",
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ setLocale: jest.fn() }),
}));

jest.mock("@/i18n/operatorLocaleBridge", () => ({
  resolveBackendLocaleSeed: () => null,
}));

jest.mock("@/api/users/profile", () => ({ getUserProfile: jest.fn() }));
jest.mock("@/api/staffProfile", () => ({ getStaffProfile: jest.fn() }));
jest.mock("@/api", () => ({ axiosInstance: { post: jest.fn() } }));
jest.mock("@/utils/staffAuth", () => ({ clearStaffSession: jest.fn() }));
jest.mock("@/utils/guestRoute", () => ({ isGuestRoute: () => false }));
jest.mock("@/utils/safeRedirect", () => ({ isSafeRedirectUrl: () => false }));
jest.mock("@/utils/walletAuth", () => ({ buildWalletAuthMessage: jest.fn() }));
jest.mock("@/utils/errorLogger", () => ({ logError: jest.fn() }));
jest.mock("@/utils/logger", () => ({ logger: { debug: jest.fn() } }));
jest.mock("@/lib/sentry/reporting", () => ({
  clearSentryIdentity: jest.fn(),
  setSentryIdentity: jest.fn(),
}));
jest.mock("@/utils/cache", () => ({
  apiCache: { setFingerprint: jest.fn(), clear: jest.fn(), endSession: jest.fn() },
}));

jest.mock("@/components/auth/VerifyEmailBanner", () => () => null);
jest.mock("@/components/auth/SessionTimeoutWarning", () => () => null);

const deadRefresh: RefreshAuthResult = {
  ok: false,
  status: 401,
  code: "AUTH_TOKEN_INVALID",
  alreadyRotated: false,
  sessionDead: true,
};

const rotatedRefresh: RefreshAuthResult = {
  ok: false,
  status: 401,
  code: "AUTH_REFRESH_ROTATED",
  alreadyRotated: true,
  sessionDead: false,
};

const transientRefresh: RefreshAuthResult = {
  ok: false,
  status: 503,
  alreadyRotated: false,
  sessionDead: false,
};

describe("HybridAuthProvider bootstrap refresh", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    localStorage.setItem(SESSION_HINT_KEY, "1");
    mockGetSessionInfo.mockResolvedValue({ authenticated: false });
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("clears the session hint when bootstrap confirms the refresh session is dead", async () => {
    mockRefreshAuthSession.mockResolvedValue(deadRefresh);

    render(
      <HybridAuthProvider>
        <div>child</div>
      </HybridAuthProvider>,
    );

    await waitFor(() => expect(mockClearUser).toHaveBeenCalled());

    expect(mockRefreshAuthSession).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem(SESSION_HINT_KEY)).toBeNull();
  });

  it.each([
    ["already-rotated", rotatedRefresh],
    ["transient", transientRefresh],
  ])("preserves the session hint after a %s bootstrap result", async (_label, result) => {
    mockRefreshAuthSession.mockResolvedValue(result);

    render(
      <HybridAuthProvider>
        <div>child</div>
      </HybridAuthProvider>,
    );

    await waitFor(() => expect(mockClearUser).toHaveBeenCalled());

    expect(mockRefreshAuthSession).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem(SESSION_HINT_KEY)).toBe("1");
  });
});
