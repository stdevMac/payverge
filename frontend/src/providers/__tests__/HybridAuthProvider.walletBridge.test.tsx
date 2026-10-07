/**
 * @jest-environment jsdom
 *
 * wagmi only mounts in the (shop) layout, so HybridAuthProvider reads wallet
 * state from the wallet bridge store. Where the bridge is unavailable (diner
 * routes, wagmi still reconnecting) a wallet session must survive: unknown is
 * not "disconnected". Where the bridge reports a real disconnect, the wallet
 * session is still torn down.
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

let mockBridge = {
  available: false,
  address: undefined as `0x${string}` | undefined,
  isConnected: false,
  chainId: undefined as number | undefined,
  signMessageAsync: null as null | jest.Mock,
};
jest.mock("@/providers/walletBridgeStore", () => ({
  useWalletBridge: () => mockBridge,
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

async function bootWalletSession() {
  mockGetSessionInfo.mockResolvedValue({
    authenticated: true,
    type: "web3",
    address: "0xabc123",
  });
  mockGetUserProfile.mockResolvedValue({ id: 7, address: "0xabc123" });
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
  return latest;
}

describe("HybridAuthProvider wallet bridge", () => {
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

  it("keeps a wallet session when no wallet stack is mounted", async () => {
    mockBridge = {
      available: false,
      address: undefined,
      isConnected: false,
      chainId: undefined,
      signMessageAsync: null,
    };
    await bootWalletSession();
    // Let the disconnect and login-state effects settle.
    await new Promise((resolve) => setTimeout(resolve, 200));

    expect(mockAxiosPost).not.toHaveBeenCalledWith(
      "/auth/logout",
      expect.anything(),
    );
    expect(mockClearUser).not.toHaveBeenCalled();
    expect(localStorage.getItem("payverge_had_session")).toBe("1");
  });

  it("still clears a wallet session on a real wallet disconnect", async () => {
    mockBridge = {
      available: true,
      address: undefined,
      isConnected: false,
      chainId: 1,
      signMessageAsync: jest.fn(),
    };
    await bootWalletSession();

    await waitFor(() => expect(mockClearUser).toHaveBeenCalled());
  });
});
