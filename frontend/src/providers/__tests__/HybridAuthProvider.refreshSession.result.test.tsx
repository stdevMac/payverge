/**
 * @jest-environment jsdom
 */
import React, { useEffect } from "react";
import { act, render, waitFor } from "@testing-library/react";
import { HybridAuthProvider, useAuth } from "../HybridAuthProvider";
import { SESSION_HINT_KEY } from "@/utils/refreshAuth";
import { apiCache } from "@/utils/cache";

const mockGetSessionInfo = jest.fn();
const mockGetCurrentUser = jest.fn();
const mockGetStaffProfile = jest.fn();
const mockSetUser = jest.fn();
const mockClearUser = jest.fn();
const mockReplace = jest.fn();
const mockIsSafeRedirectUrl = jest.fn((..._args: unknown[]) => false);

jest.mock("@/utils/refreshAuth", () => ({
  SESSION_HINT_KEY: "payverge_had_session",
  refreshAuthSession: jest.fn().mockResolvedValue({ ok: true, sessionDead: false }),
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
  useRouter: () => ({ replace: mockReplace }),
  usePathname: () => "/dashboard",
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ setLocale: jest.fn() }),
}));

jest.mock("@/i18n/operatorLocaleBridge", () => ({
  resolveBackendLocaleSeed: () => null,
}));

jest.mock("@/api/users/profile", () => ({ getUserProfile: jest.fn() }));
jest.mock("@/api/staffProfile", () => ({
  getStaffProfile: (...args: unknown[]) => mockGetStaffProfile(...args),
}));
jest.mock("@/api", () => ({ axiosInstance: { post: jest.fn() } }));
jest.mock("@/utils/staffAuth", () => ({ clearStaffSession: jest.fn() }));
jest.mock("@/utils/guestRoute", () => ({ isGuestRoute: () => false }));
jest.mock("@/utils/safeRedirect", () => ({
  isSafeRedirectUrl: (...args: unknown[]) => mockIsSafeRedirectUrl(...args),
}));
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

function Probe({
  onReady,
}: {
  onReady: (api: ReturnType<typeof useAuth>) => void;
}) {
  const auth = useAuth();
  useEffect(() => {
    onReady(auth);
  }, [auth, onReady]);
  return null;
}

const userSession = {
  authenticated: true,
  type: "user" as const,
  user_id: 42,
  email: "owner@example.com",
  role: "user",
};

const staffSession = {
  authenticated: true,
  type: "staff" as const,
  staff_id: 7,
  email: "server@example.com",
  staff_name: "Server",
  role: "server",
  business_id: 99,
  business_name: "Cafe",
  business_slug: "cafe",
};

const ownerProfile = {
  id: 42,
  email: "owner@example.com",
  role: "user",
  name: "Owner",
};

describe("HybridAuthProvider refreshSession result", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockGetSessionInfo.mockResolvedValue({ authenticated: false });
    mockIsSafeRedirectUrl.mockReturnValue(false);
    mockGetStaffProfile.mockReset();
    window.history.replaceState({}, "", "/dashboard");
  });

  it("returns false and leaves the provider anonymous when session-info rejects", async () => {
    mockGetSessionInfo.mockRejectedValue(new Error("session-info down"));
    let api: ReturnType<typeof useAuth> | null = null;
    render(
      <HybridAuthProvider>
        <Probe onReady={(next) => {
          api = next;
        }} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(api).not.toBeNull());
    const ok = await act(async () => api!.refreshSession());
    expect(ok).toBe(false);
    expect(mockSetUser).not.toHaveBeenCalled();
  });

  it("returns false when session-info is unauthenticated", async () => {
    mockGetSessionInfo.mockResolvedValue({ authenticated: false });
    let api: ReturnType<typeof useAuth> | null = null;
    render(
      <HybridAuthProvider>
        <Probe onReady={(next) => {
          api = next;
        }} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(api).not.toBeNull());
    const ok = await act(async () => api!.refreshSession());
    expect(ok).toBe(false);
    expect(mockSetUser).not.toHaveBeenCalled();
  });

  it("returns false for a staff realm instead of treating email login as hydrated", async () => {
    mockGetSessionInfo.mockResolvedValue(staffSession);
    let api: ReturnType<typeof useAuth> | null = null;
    render(
      <HybridAuthProvider>
        <Probe onReady={(next) => {
          api = next;
        }} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(api).not.toBeNull());
    const ok = await act(async () => api!.refreshSession());
    expect(ok).toBe(false);
    expect(mockSetUser).not.toHaveBeenCalled();
  });

  it("retries successfully after a failed hydration", async () => {
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    let api: ReturnType<typeof useAuth> | null = null;
    render(
      <HybridAuthProvider>
        <Probe onReady={(next) => {
          api = next;
        }} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(api).not.toBeNull());
    mockGetSessionInfo.mockRejectedValueOnce(new Error("session-info down"));
    const first = await act(async () => api!.refreshSession());
    expect(first).toBe(false);
    mockGetSessionInfo.mockResolvedValue(userSession);
    const second = await act(async () => api!.refreshSession());
    expect(second).toBe(true);
    expect(mockSetUser).toHaveBeenCalled();
  });

  it("still hydrates when localStorage writes fail", async () => {
    const setItem = jest
      .spyOn(Storage.prototype, "setItem")
      .mockImplementation(() => {
        throw new Error("quota");
      });
    mockGetSessionInfo.mockResolvedValue(userSession);
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    let api: ReturnType<typeof useAuth> | null = null;
    try {
      render(
        <HybridAuthProvider>
          <Probe onReady={(next) => {
            api = next;
          }} />
        </HybridAuthProvider>,
      );
      await waitFor(() => expect(api).not.toBeNull());
      const ok = await act(async () => api!.refreshSession());
      expect(ok).toBe(true);
      expect(mockSetUser).toHaveBeenCalled();
    } finally {
      setItem.mockRestore();
    }
  });

  it("returns true after a user session hydrates", async () => {
    mockGetSessionInfo.mockResolvedValue(userSession);
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    let api: ReturnType<typeof useAuth> | null = null;
    render(
      <HybridAuthProvider>
        <Probe onReady={(next) => {
          api = next;
        }} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(api).not.toBeNull());
    const ok = await act(async () => api!.refreshSession());
    expect(ok).toBe(true);
    expect(mockSetUser).toHaveBeenCalled();
  });

  it("clears the principal when a sibling tab removes the session hint", async () => {
    mockGetSessionInfo.mockResolvedValue(userSession);
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    render(
      <HybridAuthProvider>
        <Probe onReady={() => {}} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(mockGetSessionInfo).toHaveBeenCalled());
    mockClearUser.mockClear();
    await act(async () => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: SESSION_HINT_KEY, newValue: null }),
      );
    });
    await waitFor(() => expect(mockClearUser).toHaveBeenCalled());
    expect(apiCache.setFingerprint).toHaveBeenCalledWith(null);
  });

  it("probes session-info once when a sibling tab sets the session hint", async () => {
    mockGetSessionInfo.mockResolvedValue({ authenticated: false });
    render(
      <HybridAuthProvider>
        <Probe onReady={() => {}} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(mockGetSessionInfo).toHaveBeenCalled());
    mockGetSessionInfo.mockClear();
    mockGetSessionInfo.mockResolvedValue(userSession);
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    await act(async () => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: SESSION_HINT_KEY, newValue: "1" }),
      );
    });
    await waitFor(() => expect(mockGetSessionInfo).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(mockSetUser).toHaveBeenCalled());
  });

  it("serializes duplicate sibling-tab login hints into one session-info probe", async () => {
    render(
      <HybridAuthProvider>
        <Probe onReady={() => {}} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(mockGetSessionInfo).toHaveBeenCalled());
    mockGetSessionInfo.mockClear();
    let resolveInfo: (value: unknown) => void = () => {};
    mockGetSessionInfo.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveInfo = resolve;
        }),
    );
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    await act(async () => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: SESSION_HINT_KEY, newValue: "1" }),
      );
      window.dispatchEvent(
        new StorageEvent("storage", { key: SESSION_HINT_KEY, newValue: "1" }),
      );
    });
    expect(mockGetSessionInfo).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolveInfo(userSession);
    });
    await waitFor(() => expect(mockSetUser).toHaveBeenCalled());
  });

  it("replaces an owner principal with staff after a sibling-tab realm change", async () => {
    mockGetSessionInfo.mockResolvedValue(userSession);
    mockGetCurrentUser.mockResolvedValue(ownerProfile);
    let api: ReturnType<typeof useAuth> | null = null;
    render(
      <HybridAuthProvider>
        <Probe onReady={(next) => {
          api = next;
        }} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(api).not.toBeNull());
    await act(async () => {
      await api!.refreshSession();
    });
    await waitFor(() => expect(api!.oauthData?.userId).toBe(42));
    mockClearUser.mockClear();
    mockGetSessionInfo.mockResolvedValue(staffSession);
    await act(async () => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: SESSION_HINT_KEY, newValue: "1" }),
      );
    });
    await waitFor(() => expect(api!.staffData?.id).toBe(7));
    expect(api!.oauthData).toBeNull();
    expect(mockClearUser).toHaveBeenCalled();
    expect(apiCache.setFingerprint).toHaveBeenCalledWith("staff:7:99");
  });

  it("does not treat unrelated storage keys as a session hint", async () => {
    render(
      <HybridAuthProvider>
        <Probe onReady={() => {}} />
      </HybridAuthProvider>,
    );
    await waitFor(() => expect(mockGetSessionInfo).toHaveBeenCalled());
    mockGetSessionInfo.mockClear();
    mockClearUser.mockClear();
    await act(async () => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: "unrelated", newValue: "1" }),
      );
    });
    expect(mockGetSessionInfo).not.toHaveBeenCalled();
    expect(mockClearUser).not.toHaveBeenCalled();
  });

  it("staff OAuth callback does not follow a stored redirect when profile hydration fails", async () => {
    window.history.replaceState({}, "", "/dashboard?staff_auth=success");
    localStorage.setItem("staff_auth_redirect_url", "/business/99/dashboard");
    mockIsSafeRedirectUrl.mockReturnValue(true);
    mockGetStaffProfile.mockRejectedValue(new Error("profile down"));
    render(
      <HybridAuthProvider>
        <Probe onReady={() => {}} />
      </HybridAuthProvider>,
    );
    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/staff/login"),
    );
    expect(mockReplace).not.toHaveBeenCalledWith("/business/99/dashboard");
  });
});
