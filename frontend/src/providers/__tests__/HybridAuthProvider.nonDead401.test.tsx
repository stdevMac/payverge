/**
 * @jest-environment jsdom
 *
 * Defect L6-42 / #5: profile-fetch 401s that are NOT session-dead must leave
 * the principal in place. Only a genuinely dead session may clear state
 * (via the auth:session-expired → SessionTimeoutWarning CTA path, or an
 * explicit dead-session signal). User-initiated logout must still POST
 * /staff/logout.
 */
import React from "react";
import { act, render, waitFor } from "@testing-library/react";
import { HybridAuthProvider, useAuth } from "../HybridAuthProvider";

const mockGetSessionInfo = jest.fn();
const mockGetCurrentUser = jest.fn();
const mockGetStaffProfile = jest.fn();
const mockClearStaffSession = jest.fn();
const mockAxiosPost = jest.fn();
const mockSetUser = jest.fn();
const mockClearUser = jest.fn();
const mockRefreshAuthSession = jest.fn();

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

jest.mock("@/api/staffProfile", () => ({
  getStaffProfile: (...args: unknown[]) => mockGetStaffProfile(...args),
}));

jest.mock("@/utils/staffAuth", () => ({
  clearStaffSession: (...args: unknown[]) => mockClearStaffSession(...args),
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
  usePathname: () => "/business/1/dashboard",
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ setLocale: jest.fn() }),
}));

jest.mock("@/i18n/operatorLocaleBridge", () => ({
  resolveBackendLocaleSeed: () => null,
}));

jest.mock("@/api/users/profile", () => ({ getUserProfile: jest.fn() }));
jest.mock("@/api", () => ({
  axiosInstance: {
    post: (...args: unknown[]) => mockAxiosPost(...args),
  },
}));
// staffAuth's real implementation posts through the shared instance module,
// not "@/api" — mock it so requireActual(staffAuth) below hits an observable
// seam instead of a live axios instance.
const mockInstancePost = jest.fn();
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: (...args: unknown[]) => mockInstancePost(...args),
  },
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

describe("HybridAuthProvider — non-dead 401 must not clear principal (#5)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("payverge_had_session", "1");
    mockClearStaffSession.mockResolvedValue(undefined);
    mockAxiosPost.mockResolvedValue({ data: {} });
    mockRefreshAuthSession.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("does not call clearStaffSession when staff profile returns a transient 401", async () => {
    mockGetSessionInfo.mockResolvedValue({
      authenticated: true,
      type: "staff",
      staff_id: 9,
      email: "mgr@example.com",
      staff_name: "Mgr",
      role: "manager",
      business_id: 1,
      business_name: "Cafe",
      business_slug: "cafe",
    });
    // Interceptor rejected with original 401 after non-dead refresh failure
    // (rotation not yet recovered). Session is still alive server-side.
    const err = Object.assign(new Error("Unauthorized"), { status: 401 });
    mockGetStaffProfile.mockRejectedValue(err);

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
    await waitFor(() => expect(mockGetStaffProfile).toHaveBeenCalled());

    expect(mockClearStaffSession).not.toHaveBeenCalled();
    // Principal from session-info must remain.
    expect(latest.current?.isStaffUser).toBe(true);
    expect(latest.current?.staffData?.email).toBe("mgr@example.com");
  });

  it("does not clear OAuth principal when /auth/me returns a transient 401", async () => {
    mockGetSessionInfo.mockResolvedValue({
      authenticated: true,
      type: "user",
      user_id: 42,
      email: "owner@example.com",
      role: "user",
      email_verified: true,
    });
    const err = { response: { status: 401 } };
    mockGetCurrentUser.mockRejectedValue(err);

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
    await waitFor(() => expect(mockGetCurrentUser).toHaveBeenCalled());

    // session-info already set the OAuth principal — a non-dead 401 on
    // /auth/me must not flip isOAuthUser to false (AuthGate would bounce).
    expect(latest.current?.isOAuthUser).toBe(true);
    expect(latest.current?.oauthData?.email).toBe("owner@example.com");
  });

  it("still surfaces SessionTimeoutWarning path on auth:session-expired (dead session)", async () => {
    mockGetSessionInfo.mockResolvedValue({
      authenticated: true,
      type: "user",
      user_id: 42,
      email: "owner@example.com",
      role: "user",
      email_verified: true,
    });
    mockGetCurrentUser.mockResolvedValue({
      id: 42,
      email: "owner@example.com",
      role: "user",
      name: "Owner",
    });

    // Spy SessionTimeoutWarning via a stateful probe that reads nothing about
    // isSessionExpired directly (it's not on context). Instead verify the
    // session-expired handler does not recurse into clearSession/logout and
    // does not clear the principal immediately (P1-10), while still running.
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

    await waitFor(() => expect(latest.current?.isOAuthUser).toBe(true));

    await act(async () => {
      window.dispatchEvent(new CustomEvent("auth:session-expired"));
      // Second concurrent event must be a no-op (no recursion / double-handle).
      window.dispatchEvent(new CustomEvent("auth:session-expired"));
    });

    // Principal stays mounted until the modal CTA (P1-10). Dead-path logout
    // is user-driven via SessionTimeoutWarning, not auto-clear here.
    expect(latest.current?.isOAuthUser).toBe(true);
    expect(mockAxiosPost).not.toHaveBeenCalledWith(
      "/auth/logout",
      expect.anything(),
    );
    expect(mockClearStaffSession).not.toHaveBeenCalled();
  });

  it("user-initiated clearSession for staff still POSTs /staff/logout", async () => {
    // The user-initiated path is clearSession → clearStaffSession (provider
    // staff branch, unchanged). Prove the REAL clearStaffSession still posts
    // /staff/logout: requireActual bypasses the module mock above while its
    // axios import resolves to the mocked instance seam.
    mockInstancePost.mockResolvedValue({ data: {} });
    const { clearStaffSession: actualClearStaffSession } = jest.requireActual(
      "@/utils/staffAuth",
    ) as { clearStaffSession: () => Promise<void> };

    await actualClearStaffSession();

    expect(mockInstancePost).toHaveBeenCalledWith("/staff/logout");
  });
});
