/**
 * @jest-environment jsdom
 *
 * L6-42: web3 fetchUserProfile must not clearSession / clearUser on a non-dead
 * 401 from getUserProfile. Exercises the real HybridAuthProvider bootstrap
 * path (session type "web3") — not a source-file grep.
 */
import React from "react";
import { render, waitFor } from "@testing-library/react";
import { HybridAuthProvider, useAuth } from "../HybridAuthProvider";

const mockGetSessionInfo = jest.fn();
const mockGetUserProfile = jest.fn();
const mockClearUser = jest.fn();
const mockSetUser = jest.fn();
const mockRefreshAuthSession = jest.fn();
const mockAxiosPost = jest.fn();

jest.mock("@/utils/refreshAuth", () => ({
  SESSION_HINT_KEY: "payverge_had_session",
  refreshAuthSession: (...args: unknown[]) => mockRefreshAuthSession(...args),
}));

jest.mock("@/api/auth/sessionInfo", () => ({
  getSessionInfo: (...args: unknown[]) => mockGetSessionInfo(...args),
}));

jest.mock("@/api/auth", () => ({
  getCurrentUser: jest.fn(),
  authAPI: {
    getWalletChallenge: jest.fn(),
    linkWallet: jest.fn(),
    unlinkWallet: jest.fn(),
  },
}));

jest.mock("@/api/staffProfile", () => ({
  getStaffProfile: jest.fn(),
}));

jest.mock("@/utils/staffAuth", () => ({
  clearStaffSession: jest.fn(),
}));

jest.mock("@/store/useUserStore", () => {
  const useUserStore = Object.assign(
    () => ({ setUser: mockSetUser, clearUser: mockClearUser, user: null }),
    { getState: () => ({ user: null }) },
  );
  return { useUserStore };
});

jest.mock("@/providers/walletBridgeStore", () => ({
  // Keep wallet connected so the disconnect effect does not clearSession
  // after web3 bootstrap (that path is unrelated to L6-42).
  useWalletBridge: () => ({
    available: true,
    address: "0xabc123" as `0x${string}`,
    isConnected: true,
    chainId: 1,
    signMessageAsync: jest.fn(),
  }),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn() }),
  usePathname: () => "/business/1/dashboard",
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ setLocale: jest.fn() }),
}));

jest.mock("@/i18n/operatorLocaleBridge", () => ({
  resolveBackendLocaleSeed: () => null,
}));

jest.mock("@/api/users/profile", () => ({
  getUserProfile: (...args: unknown[]) => mockGetUserProfile(...args),
}));

jest.mock("@/api", () => ({
  axiosInstance: {
    post: (...args: unknown[]) => mockAxiosPost(...args),
  },
}));

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { post: jest.fn() },
}));
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
jest.mock("@/utils/tokenRefresh", () => ({
  startTokenRefreshTimer: () => () => undefined,
}));
jest.mock("@/components/auth/VerifyEmailBanner", () => () => null);
jest.mock("@/components/auth/SessionTimeoutWarning", () => () => null);

function AuthProbe({
  onReady,
}: {
  onReady: (auth: ReturnType<typeof useAuth>) => void;
}) {
  const auth = useAuth();
  React.useEffect(() => {
    onReady(auth);
  }, [auth, onReady]);
  return null;
}

describe("HybridAuthProvider L6-42 web3 non-dead 401 (real path)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("payverge_had_session", "1");
    mockRefreshAuthSession.mockResolvedValue({ ok: true });
    mockAxiosPost.mockResolvedValue({ data: {} });
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("does not clearSession / logout when getUserProfile returns a transient 401", async () => {
    mockGetSessionInfo.mockResolvedValue({
      authenticated: true,
      type: "web3",
      address: "0xabc123",
    });
    const err = Object.assign(new Error("Unauthorized"), {
      response: { status: 401 },
    });
    mockGetUserProfile.mockRejectedValue(err);

    const latest: { current: ReturnType<typeof useAuth> | null } = {
      current: null,
    };
    render(
      <HybridAuthProvider>
        <AuthProbe
          onReady={(a) => {
            latest.current = a;
          }}
        />
      </HybridAuthProvider>,
    );

    await waitFor(() => expect(latest.current?.isInitialized).toBe(true));
    await waitFor(() =>
      expect(mockGetUserProfile).toHaveBeenCalledWith("0xabc123"),
    );

    // L6-42 enforcement: recoverable web3 profile 401 must NOT tear down the
    // session (clearSession posts /auth/logout and drops the session hint).
    expect(mockAxiosPost).not.toHaveBeenCalledWith(
      "/auth/logout",
      expect.anything(),
    );
    expect(localStorage.getItem("payverge_had_session")).toBe("1");
    // Provider finished bootstrap; profile fetch failed without killing session.
    expect(latest.current?.isInitialized).toBe(true);
    // clearSession always clears the user store — must not happen on this path.
    // (wallet_disconnected is suppressed by connected wagmi mock above.)
    expect(mockClearUser).not.toHaveBeenCalled();
  });
});


