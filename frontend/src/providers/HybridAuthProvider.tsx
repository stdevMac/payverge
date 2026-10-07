"use client";
import { getPublicConfig } from "@/config/publicConfig";
import {
  useEffect,
  useCallback,
  useRef,
  createContext,
  useContext,
  ReactNode,
  useState,
} from "react";
import { useUserStore } from "@/store/useUserStore";
import { UserInterface } from "@/interface";
import { getUserProfile } from "@/api/users/profile";
import { getStaffProfile } from "@/api/staffProfile";
import { getCurrentUser, authAPI } from "@/api/auth";
import { useRouter, usePathname } from "next/navigation";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { resolveBackendLocaleSeed } from "@/i18n/operatorLocaleBridge";
import {
  clearStaffSession,
  StaffData,
} from "@/utils/staffAuth";
import { axiosInstance } from "@/api";
import { useWalletBridge } from "@/providers/walletBridgeStore";
import { logError } from "@/utils/errorLogger";
import { clearSentryIdentity, setSentryIdentity } from "@/lib/sentry/reporting";
import { logger } from "@/utils/logger";
import { getSessionInfo, type SessionInfo } from "@/api/auth/sessionInfo";
import { apiCache } from "@/utils/cache";
import { buildWalletAuthMessage } from "@/utils/walletAuth";
import { isSafeRedirectUrl } from "@/utils/safeRedirect";
import { startTokenRefreshTimer } from "@/utils/tokenRefresh";
import {
  SESSION_HINT_KEY,
  refreshAuthSession,
  type RefreshAuthResult,
} from "@/utils/refreshAuth";
import { isGuestRoute } from "@/utils/guestRoute";
import {
  CLIENT_PUBLIC_PREFIXES,
  CLIENT_PROTECTED_PREFIXES,
  matchesPublicPrefix,
} from "@/utils/publicPaths";
import SessionTimeoutWarning from "@/components/auth/SessionTimeoutWarning";
import VerifyEmailBanner from "@/components/auth/VerifyEmailBanner";
import { getLaunchInviteCode } from "@/utils/launchInvite";
import { clearAllMutationQueues } from "@/lib/mutationQueue";
import { getOperatorDashboardPath } from "@/utils/businessUrl";

interface OAuthUserData {
  userId: number;
  email: string;
  role: string;
  picture?: string;
}

type AuthSource = "oauth" | "dynamic" | "staff" | "customer";

interface AuthContextType {
  isWeb3User: boolean;
  isStaffUser: boolean;
  isOAuthUser: boolean;
  oauthData: OAuthUserData | null;
  staffData: StaffData | null;
  refreshStaffData: () => Promise<void>;
  walletLinked: boolean;
  walletAddress: string | null;
  linkWallet: (address: string) => Promise<void>;
  unlinkWallet: () => Promise<void>;
  refreshOAuthUser: () => Promise<void>;
  // P0-1: re-hydrate the provider from session-info after an in-modal
  // email/password auth (AuthModal), so public-surface funnels
  // (/business/register) see the new principal without a reload.
  refreshSession: () => Promise<boolean>;
  // Re-run cold session-info bootstrap after a transport failure
  // (AuthGate retriable card). Does not force a full page reload.
  retrySessionBootstrap: () => Promise<void>;
  isLoading: boolean;
  isInitialized: boolean;
  sessionInfoError: boolean;
  // False only for an authenticated email/password user whose email is not yet
  // verified — drives the dashboard "verify your email" banner.
  emailVerified: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within a HybridAuthProvider");
  }
  return context;
};

/**
 * Compute a per-session cache scope from the SessionInfo response. The
 * fingerprint encodes the authenticated principal (type + id + business id)
 * so apiCache can isolate entries across users on shared devices.
 * Returns null for unauthenticated / customer sessions.
 */
function computeSessionFingerprint(info: {
  type?: string;
  user_id?: number;
  staff_id?: number;
  business_id?: number;
  customer_id?: number;
}): string | null {
  if (!info.type) return null;
  const id = info.staff_id ?? info.user_id ?? info.customer_id ?? 0;
  const biz = info.business_id ?? 0;
  return `${info.type}:${id}:${biz}`;
}

function hasSessionHint(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return localStorage.getItem("payverge_had_session") === "1";
  } catch {
    return false;
  }
}

function nonDeadRefreshFailure(): RefreshAuthResult {
  return {
    ok: false,
    status: 0,
    alreadyRotated: false,
    sessionDead: false,
  };
}

export function HybridAuthProvider({ children }: { children: ReactNode }) {
  // Wallet state comes through the bridge store, not wagmi hooks: this
  // provider is in the root layout, and wagmi only mounts in operator
  // layouts. `walletAvailable` is false where no WagmiProvider is mounted
  // (diner routes) or while wagmi is still reconnecting; that means "unknown",
  // never "disconnected".
  const {
    available: walletAvailable,
    address,
    isConnected,
    chainId,
    signMessageAsync,
  } = useWalletBridge();
  const { setUser, clearUser, user } = useUserStore();
  const router = useRouter();
  const pathname = usePathname();
  const isPublicSurface = matchesPublicPrefix(
    pathname ?? "/",
    CLIENT_PUBLIC_PREFIXES,
  );
  const isProtectedSurface = matchesPublicPrefix(
    pathname ?? "/",
    CLIENT_PROTECTED_PREFIXES,
  );

  // Public guest surfaces — no operator/staff features here, so the
  // session-expired modal would just startle a guest who happens to have
  // a stale token from a prior session on a different surface.
  const isGuestSurface = ((): boolean => {
    if (isPublicSurface) return true;
    if (!pathname) return false;
    if (pathname.startsWith("/b/")) return true;
    if (pathname.startsWith("/t/")) return true;
    if (pathname.startsWith("/reservations/")) return true;
    if (pathname === "/") return true;
    if (pathname.startsWith("/es/")) return true;
    if (pathname.startsWith("/es-ar/")) return true;
    if (pathname.startsWith("/refund")) return true;
    if (pathname.startsWith("/terms")) return true;
    if (pathname.startsWith("/privacy")) return true;
    return false;
  })();
  // OP-2: the operator dashboard render is driven by SimpleTranslationProvider's
  // `locale` key (read/written via useSimpleLocale). Seed it from the user's
  // backend-saved language so a Spanish-preferring operator on a fresh device
  // (empty localStorage["locale"]) gets a Spanish dashboard immediately, instead
  // of English-until-they-re-pick. resolveBackendLocaleSeed guards against
  // clobbering an explicit in-session pick and rejects non-operator codes.
  const { setLocale } = useSimpleLocale();

  // Wallet login state (formerly the Dynamic SDK hook).
  const isDynamicLoggedIn = walletAvailable && isConnected;

  // Track loading state for UI feedback
  const [isLoading, setIsLoading] = useState(true);
  const [isInitialized, setIsInitialized] = useState(false);
  const [oauthSessionData, setOAuthSessionData] = useState<OAuthUserData | null>(null);
  const oauthPictureRef = useRef<string>("");

  // Track the last fetch time and address to prevent duplicate requests
  const lastFetchRef = useRef<{ time: number; address: string | null }>({
    time: 0,
    address: null,
  });
  const fetchTimeoutRef = useRef<NodeJS.Timeout>();
  const initAttemptedRef = useRef(false);
  const dynamicAuthProcessedRef = useRef(false);
  const staffTokenProcessedRef = useRef(false);

  const [isSessionExpired, setIsSessionExpired] = useState(false);
  const [sessionInfoError, setSessionInfoError] = useState(false);
  // Bumped by retrySessionBootstrap so the cold-start effect re-runs after a
  // transport failure without a full page reload.
  const [bootstrapGeneration, setBootstrapGeneration] = useState(0);
  // Defaults verified; only an authenticated email/password user with an
  // unverified email flips this false (drives the verify-email banner).
  const [emailVerified, setEmailVerified] = useState(true);
  const refreshCleanupRef = useRef<(() => void) | null>(null);
  const crossTabEpochRef = useRef(0);
  const crossTabLoginInFlightRef = useRef(false);

  // Staff state - ref for synchronous access in callbacks, state for reactive re-renders
  const staffDataRef = useRef<StaffData | null>(null);
  const [staffDataState, setStaffDataState] = useState<StaffData | null>(null);
  const [isStaffUser, setIsStaffUser] = useState(false);

  const performRefresh = useCallback(async (): Promise<RefreshAuthResult> => {
    if (isGuestRoute()) return nonDeadRefreshFailure();
    // Skip the refresh attempt entirely for anonymous visitors. The
    // refresh_token cookie is httpOnly so JS can't see it; we use a tiny
    // non-sensitive localStorage flag set on successful auth and cleared
    // on logout/expiry so first-load doesn't fire a guaranteed-401 to
    // /auth/refresh on every marketing page view.
    if (typeof window !== "undefined" && !localStorage.getItem(SESSION_HINT_KEY)) {
      return nonDeadRefreshFailure();
    }
    // Shared with the axios 401 interceptor so concurrent refreshes coalesce
    // into one POST. Refresh outcomes do not clear the hint here: confirmed
    // death must reach the timer threshold, then terminal expiry clears it.
    const apiUrl = getPublicConfig().apiUrl;
    const result = await refreshAuthSession(apiUrl);
    if (
      !result.ok &&
      result.status != null &&
      result.status !== 401 &&
      result.status !== 429 &&
      result.status !== 0
    ) {
      void logError(
        `refresh returned ${result.status}`,
        "HybridAuthProvider",
        "performRefresh",
      );
    }
    // Preserve the dead/non-dead classification so the proactive timer can
    // retry inconclusive rotation without ever turning it into session expiry.
    return result;
  }, []);

  // auth_source is kept in a ref (not localStorage) to avoid XSS exposure.
  // The ref is canonical for synchronous reads inside callbacks. A React
  // state mirror drives context-derived booleans like isWeb3User so consumers
  // re-render when the auth source changes.
  const authSourceRef = useRef<string | null>(null);
  const [authSourceState, setAuthSourceStateReact] = useState<string | null>(null);

  const getAuthSource = useCallback((): string | null => {
    return authSourceRef.current;
  }, []);

  const setAuthSource = useCallback((source: AuthSource) => {
    authSourceRef.current = source;
    setAuthSourceStateReact(source);
  }, []);

  const clearAuthSource = useCallback(() => {
    authSourceRef.current = null;
    setAuthSourceStateReact(null);
  }, []);

  const getDynamicAuthIntent = useCallback((): boolean => {
    if (typeof window === "undefined") return false;
    const raw = localStorage.getItem("dynamic_auth_intent");
    if (!raw) return false;

    try {
      const parsed = JSON.parse(raw) as { intent?: string; timestamp?: number };
      if (parsed.intent !== "wallet" || !parsed.timestamp) {
        localStorage.removeItem("dynamic_auth_intent");
        return false;
      }
      // Expire intent after 5 minutes
      if (Date.now() - parsed.timestamp > 5 * 60 * 1000) {
        localStorage.removeItem("dynamic_auth_intent");
        return false;
      }
      return true;
    } catch {
      localStorage.removeItem("dynamic_auth_intent");
      return false;
    }
  }, []);

  const clearDynamicAuthIntent = useCallback(() => {
    if (typeof window === "undefined") return;
    localStorage.removeItem("dynamic_auth_intent");
  }, []);

  const updateStaffData = useCallback((data: StaffData | null) => {
    staffDataRef.current = data;
    setStaffDataState(data);
    setIsStaffUser(!!data);
    // NOTE: staff data is no longer stored in localStorage.
    // The backend maintains the canonical session in an httpOnly cookie.
  }, []);

  const clearHybridSentryIdentities = useCallback(() => {
    clearSentryIdentity("staff");
    clearSentryIdentity("user");
    clearSentryIdentity("web3");
  }, []);

  // Clear session by calling backend logout
  const clearSession = useCallback(async (reason: string = "unknown") => {
    const currentSource = getAuthSource();
    refreshCleanupRef.current?.();
    refreshCleanupRef.current = null;
    setIsSessionExpired(false);
    logger.debug(`[HybridAuthProvider] Clearing session: ${reason}`);
    try {
      if (currentSource === "staff") {
        await clearStaffSession();
      } else {
        await axiosInstance.post("/auth/logout");
      }
    } catch (error) {
      // Best-effort server-side clear. We still wipe local state below so the
      // user is signed out client-side; Sentry gets the server failure so
      // ops can tell "user clicked logout but server rejected" from "user
      // never signed out." Do NOT block on this.
      void logError(
        error instanceof Error ? error : String(error),
        'HybridAuthProvider',
        `clearSession:${currentSource ?? 'unknown'}`,
      );
    }
    clearUser();
    setOAuthSessionData(null);
    updateStaffData(null);
    clearAuthSource();
    apiCache.setFingerprint(null);
    if (typeof window !== "undefined") {
      localStorage.removeItem(SESSION_HINT_KEY);
      clearAllMutationQueues();
    }
  }, [clearAuthSource, clearUser, getAuthSource, updateStaffData]);

  // Fetch staff profile data
  const fetchStaffProfile = useCallback(async (): Promise<boolean> => {
    try {
      const staffSession = await getStaffProfile();
      // /staff/profile returns business.name as a sibling field; merge it
      // into staffData so the dashboard header keeps the real business
      // name across re-hydration. Without this, refreshStaffData would
      // overwrite the business_name set by session-info and the
      // 'Business N' fallback would re-appear.
      updateStaffData({
        ...staffSession.staff,
        business_name: staffSession.business?.name,
        business_slug: staffSession.business?.business_id,
      });
      return true;
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'fetchStaffProfile');
      // Do NOT tear down the staff principal on a 401 from /staff/profile.
      // The axios interceptor already attempted refresh; when the session is
      // genuinely dead it fires `auth:session-expired` (SessionTimeoutWarning
      // CTA clears + redirects). A non-dead 401 (rotation race, inconclusive
      // probe) must leave the principal from session-info in place — clearing
      // here used to POST /staff/logout and bounce AuthGate on a live session.
      return false;
    }
  }, [updateStaffData]);

  // Fetch Web3 user profile
  const fetchUserProfile = useCallback(
    async (userAddress: string, force: boolean = false) => {
      // Prevent duplicate requests within 2 seconds unless forced
      const now = Date.now();
      if (
        !force &&
        lastFetchRef.current.address === userAddress &&
        now - lastFetchRef.current.time < 2000
      ) {
        return;
      }

      // Update last fetch time and address
      lastFetchRef.current = { time: now, address: userAddress };
      const currentUser = useUserStore.getState().user;

      logger.debug("[HybridAuthProvider] fetchUserProfile starting", { userAddress, force });

      // Only fetch if conditions are met
      if (
        !force &&
        currentUser?.address?.toLowerCase() === userAddress.toLowerCase() &&
        isInitialized
      ) {
        return;
      }

      try {
        const userData = await getUserProfile(userAddress);
        if (userData) {
          // Set user data first
          setUser(userData);

          // OP-2: seed the operator render locale from the backend-saved
          // preference. This routes through SimpleTranslationProvider's
          // `setLocale` (the canonical `locale` key it actually reads) instead
          // of the old, divergent `language` key feeding the dead useLanguage
          // system. resolveBackendLocaleSeed honors an explicit in-session pick
          // (existing non-empty localStorage["locale"]) and only applies real
          // operator locales (en/es/es-AR) — guest-only codes are ignored here.
          const storedLocale =
            typeof window !== "undefined"
              ? localStorage.getItem("locale")
              : null;
          const seedLocale = resolveBackendLocaleSeed({
            saved: userData.language_selected,
            storedLocale,
          });
          if (seedLocale) {
            setLocale(seedLocale);
          }
        }
      } catch (error) {
        void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'fetchUserProfile');
        // L6-42: Do NOT tear down the web3 principal on a non-dead 401 from
        // getUserProfile. Staff/OAuth already leave the principal in place;
        // the interceptor classifies dead sessions and fires
        // `auth:session-expired` (SessionTimeoutWarning CTA clears + redirects).
        // Clearing here logged users out on recoverable rotation races.
        logger.debug(
          "[HybridAuthProvider] Web3 profile fetch failed — leaving wallet principal in place",
          error,
        );
      }
    },
    [setUser, setLocale, isInitialized],
  );

  // Fetch OAuth user profile from backend
  // sessionPicture: picture URL from the session-info response, used as fallback
  const fetchOAuthUserProfile = useCallback(async (sessionPicture?: string) => {
    try {
      logger.debug("[HybridAuthProvider] Fetching OAuth user profile from backend...");
      const userData = await getCurrentUser();
      logger.debug("[HybridAuthProvider] OAuth user data from backend:", userData);

      const backendPicture =
        userData?.picture ||
        userData?.avatar ||
        userData?.avatar_url ||
        userData?.profile_image_url;

      const resolvedPicture = backendPicture || sessionPicture || oauthPictureRef.current || "";
      if (resolvedPicture) {
        oauthPictureRef.current = resolvedPicture;
      }

      const oauthSession: OAuthUserData = {
        userId: userData.id,
        email: userData.email || "",
        role: userData.role || "user",
        picture: resolvedPicture,
      };
      setOAuthSessionData(oauthSession);

      // Map backend response to user store format
      const oauthUser = {
        username: userData.name || userData.email?.split('@')[0] || '',
        email: userData.email || '',
        address: userData.wallet_address || '',
        role: userData.role || 'user',
        joined_at: userData.created_at || new Date().toISOString(),
        language_selected: 'en',
        notifications: [],
      };
      setUser(oauthUser as UserInterface);

      return userData;
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'fetchOAuthUserProfile');

      // Do NOT tear down the OAuth principal on a 401 from /auth/me.
      // session-info already proved the user is authenticated upstream of this
      // call. The interceptor classifies dead sessions and fires
      // `auth:session-expired` (modal CTA clears + redirects). A non-dead 401
      // (rotation race) used to flip isOAuthUser false and bounce AuthGate.
      logger.debug(
        "[HybridAuthProvider] Profile fetch failed after session-info — leaving OAuth principal in place",
      );
      return null;
    }
  }, [setUser]);

  const linkWallet = useCallback(async (address: string) => {
    try {
      if (!chainId) {
        throw new Error("Unable to determine the connected wallet network.");
      }
      if (!signMessageAsync) {
        throw new Error("Connect a wallet before linking it.");
      }

      const challengeResponse = await authAPI.getWalletChallenge(address);
      if (!challengeResponse.challenge) {
        throw new Error("Backend did not return a wallet link challenge.");
      }

      const message = buildWalletAuthMessage(
        address,
        challengeResponse.challenge,
        chainId,
        "Link this wallet to your Payverge account.",
      );
      const signature = await signMessageAsync({ message });

      await authAPI.linkWallet({ address, message, signature });
      await fetchOAuthUserProfile();
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'linkWallet');
      throw error;
    }
  }, [chainId, fetchOAuthUserProfile, signMessageAsync]);

  const unlinkWallet = useCallback(async () => {
    try {
      await authAPI.unlinkWallet();
      await fetchOAuthUserProfile();
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'unlinkWallet');
      throw error;
    }
  }, [fetchOAuthUserProfile]);

  const refreshOAuthUser = useCallback(async () => {
    await fetchOAuthUserProfile();
  }, [fetchOAuthUserProfile]);

  // AuthGate retriable card: re-run cold session-info bootstrap without a
  // full page reload. Resets the one-shot guard and bumps bootstrapGeneration
  // so the initializeSession effect runs again.
  const retrySessionBootstrap = useCallback(async () => {
    // A prior session hint means this is a transport blip. Keep sessionInfoError
    // set and the shell mounted so CRM / Analytics do not flash the verify wall
    // while session-info re-runs (#621). Success clears the flag below.
    let keepChrome = false;
    if (typeof window !== "undefined") {
      try {
        keepChrome = window.localStorage.getItem(SESSION_HINT_KEY) === "1";
      } catch {
        keepChrome = false;
      }
    }
    if (!keepChrome) {
      setSessionInfoError(false);
      setIsInitialized(false);
      setIsLoading(true);
    }
    initAttemptedRef.current = false;
    setBootstrapGeneration((generation) => generation + 1);
  }, []);

  const startHybridRefreshTimer = useCallback(() => {
    refreshCleanupRef.current?.();
    refreshCleanupRef.current = startTokenRefreshTimer({
      refreshFn: performRefresh,
      onRefreshSuccess: () => {},
      onSessionExpired: () => setIsSessionExpired(true),
    });
  }, [performRefresh]);

  // Replace (do not merge) the in-memory principal from a verified
  // session-info payload. Local storage is only a wakeup hint — this
  // commit is the authorization result.
  const commitSessionFromInfo = useCallback(
    async (sessionInfo: SessionInfo): Promise<boolean> => {
      if (!sessionInfo.authenticated) return false;

      if (sessionInfo.type === "user") {
        updateStaffData(null);
        apiCache.setFingerprint(computeSessionFingerprint(sessionInfo));
        setAuthSource("oauth");
        setEmailVerified(sessionInfo.email_verified !== false);
        if (sessionInfo.picture) {
          oauthPictureRef.current = sessionInfo.picture;
        }
        setOAuthSessionData({
          userId: sessionInfo.user_id!,
          email: sessionInfo.email!,
          role: sessionInfo.role || "user",
          picture: sessionInfo.picture,
        });
        await fetchOAuthUserProfile(sessionInfo.picture);
        startHybridRefreshTimer();
        setIsInitialized(true);
        return true;
      }

      if (sessionInfo.type === "staff") {
        clearUser();
        setOAuthSessionData(null);
        oauthPictureRef.current = "";
        apiCache.setFingerprint(computeSessionFingerprint(sessionInfo));
        setAuthSource("staff");
        updateStaffData({
          id: sessionInfo.staff_id!,
          email: sessionInfo.email!,
          name: sessionInfo.staff_name || "",
          role: (sessionInfo.role || "server") as StaffData["role"],
          business_id: sessionInfo.business_id!,
          business_name: sessionInfo.business_name,
          business_slug: sessionInfo.business_slug,
          is_active: true,
        });
        startHybridRefreshTimer();
        setIsInitialized(true);
        return true;
      }

      return false;
    },
    [
      clearUser,
      fetchOAuthUserProfile,
      setAuthSource,
      startHybridRefreshTimer,
      updateStaffData,
    ],
  );

  // P0-1: after an in-modal email/password register/login the backend has set
  // the httpOnly session cookie, but on public surfaces initializeSession was
  // skipped (no session hint), so the provider stays anonymous and the
  // register funnel's checkout guard bounces back to the sign-in modal
  // forever. This re-runs session-info and hydrates the OAuth principal in
  // the same order as initializeSession's "user" branch. Never throws:
  // failures are logged and the hint key makes the next full bootstrap retry.
  const refreshSession = useCallback(async (): Promise<boolean> => {
    if (typeof window !== "undefined") {
      try {
        localStorage.setItem(SESSION_HINT_KEY, "1");
      } catch {
        // Storage unavailable — session-info below still hydrates this tab.
      }
    }
    try {
      const sessionInfo = await getSessionInfo();
      if (!sessionInfo.authenticated || sessionInfo.type !== "user") {
        return false;
      }
      return commitSessionFromInfo(sessionInfo);
    } catch (error) {
      void logError(
        error instanceof Error ? error : String(error),
        "HybridAuthProvider",
        "refreshSession",
      );
      return false;
    }
  }, [commitSessionFromInfo]);

  const applyRemoteLogout = useCallback(() => {
    refreshCleanupRef.current?.();
    refreshCleanupRef.current = null;
    setIsSessionExpired(false);
    clearUser();
    setOAuthSessionData(null);
    updateStaffData(null);
    clearAuthSource();
    apiCache.setFingerprint(null);
    clearAllMutationQueues();
    setIsInitialized(true);
  }, [clearAuthSource, clearUser, updateStaffData]);

  const reconcileCrossTabLogin = useCallback(async () => {
    if (crossTabLoginInFlightRef.current) return;
    crossTabLoginInFlightRef.current = true;
    const epoch = ++crossTabEpochRef.current;
    try {
      const sessionInfo = await getSessionInfo();
      if (epoch !== crossTabEpochRef.current) return;
      // The hint is not proof of authentication — only session-info is.
      if (!sessionInfo.authenticated) return;
      await commitSessionFromInfo(sessionInfo);
    } catch (error) {
      void logError(
        error instanceof Error ? error : String(error),
        "HybridAuthProvider",
        "reconcileCrossTabLogin",
      );
    } finally {
      if (epoch === crossTabEpochRef.current) {
        crossTabLoginInFlightRef.current = false;
      }
    }
  }, [commitSessionFromInfo]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const onStorage = (event: StorageEvent) => {
      if (event.key !== SESSION_HINT_KEY) return;
      if (!event.newValue) {
        crossTabEpochRef.current += 1;
        crossTabLoginInFlightRef.current = false;
        applyRemoteLogout();
        return;
      }
      void reconcileCrossTabLogin();
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [applyRemoteLogout, reconcileCrossTabLogin]);

  useEffect(() => {
    return () => {
      refreshCleanupRef.current?.();
    };
  }, []);

  // Handle staff OAuth callback signal.
  // Backend sets staff_token as httpOnly cookie; frontend detects via ?staff_auth=success.
  useEffect(() => {
    if (typeof window === "undefined") return;

    const url = new URL(window.location.href);
    const staffAuthSuccess = url.searchParams.get("staff_auth");

    const cleanUrl = () => {
      url.searchParams.delete("staff_auth");
      window.history.replaceState({}, document.title, url.pathname + url.search);
    };

    const handleStaffAuth = async () => {
      if (staffAuthSuccess !== "success" || staffTokenProcessedRef.current) return;
      staffTokenProcessedRef.current = true;

      // Token is already in httpOnly cookie set by backend redirect.
      setAuthSource("staff");
      cleanUrl();

      const hydrated = await fetchStaffProfile();
      if (!hydrated || !staffDataRef.current) {
        clearAuthSource();
        localStorage.removeItem("staff_auth_redirect_url");
        router.replace("/staff/login");
        return;
      }

      const storedRedirectUrl = localStorage.getItem("staff_auth_redirect_url");
      if (storedRedirectUrl) {
        localStorage.removeItem("staff_auth_redirect_url");
        if (isSafeRedirectUrl(storedRedirectUrl)) {
          router.replace(storedRedirectUrl);
          return;
        }
      }

      const staffData = staffDataRef.current;
      if (staffData?.business_id) {
        router.replace(getOperatorDashboardPath(String(staffData.business_slug || staffData.business_id)));
      } else {
        localStorage.removeItem("staff_auth_redirect_url");
        router.replace("/staff/login");
      }
    };

    if (staffAuthSuccess) {
      void handleStaffAuth();
    }
  }, [clearAuthSource, fetchStaffProfile, router, setAuthSource]);

  // Initialize session on app startup using server-driven session-info API
  useEffect(() => {
    const initializeSession = async () => {
      // Don't run if already initialized and we have user data — unless this
      // is a silent retry (initAttemptedRef cleared, chrome kept mounted).
      if (isInitialized && user && initAttemptedRef.current) {
        setIsLoading(false);
        return;
      }

      // Only run once per mount / retry
      if (initAttemptedRef.current) return;
      initAttemptedRef.current = true;

      setIsLoading(true);

      // Anonymous public/unprotected pages should not make a guaranteed
      // /auth/session-info request. The backend session cookie is httpOnly,
      // so the lightweight local marker set after a successful auth is the
      // signal that a route may need session hydration.
      if (!hasSessionHint() && (isPublicSurface || !isProtectedSurface)) {
        clearUser();
        setOAuthSessionData(null);
        updateStaffData(null);
        clearAuthSource();
        apiCache.setFingerprint(null);
        refreshCleanupRef.current?.();
        refreshCleanupRef.current = null;
        setIsSessionExpired(false);
        setIsInitialized(true);
        setIsLoading(false);
        return;
      }

      try {
        let sessionInfo = await getSessionInfo();

        // session-info returns 200 { authenticated: false } when the access
        // token cookie is missing or expired (15-min TTL). The refresh_token
        // cookie (7d TTL) may still be valid — try it once before giving up.
        // Without this, opening a new tab >15 min after login looks logged-out
        // even though the refresh token is alive.
        if (!sessionInfo.authenticated) {
          const refreshed = await performRefresh();
          if (refreshed.ok) {
            sessionInfo = await getSessionInfo();
          } else if (refreshed.sessionDead && typeof window !== "undefined") {
            // Bootstrap has no proactive timer running yet, so a confirmed
            // dead refresh session must clear the hint here. Inconclusive
            // rotation and transport failures retain it for a later retry.
            localStorage.removeItem(SESSION_HINT_KEY);
          }
        }

        if (!sessionInfo.authenticated) {
          clearUser();
          setOAuthSessionData(null);
          updateStaffData(null);
          clearAuthSource();
          apiCache.setFingerprint(null);
          refreshCleanupRef.current?.();
          refreshCleanupRef.current = null;
          setIsSessionExpired(false);
          setIsInitialized(true);
          setIsLoading(false);
          return;
        }

        // Scope the cache BEFORE any profile fetches so their responses
        // are cached under the correct principal. Calling this after the
        // switch would let fetchStaffProfile/fetchOAuthUserProfile cache
        // under the previous (or null) fingerprint, re-opening the
        // shared-device leak this whole system is meant to close.
        apiCache.setFingerprint(computeSessionFingerprint(sessionInfo));

        // Mark that a session exists so subsequent visits know to attempt
        // /auth/refresh. Cleared on logout/expiry to keep anonymous loads silent.
        if (typeof window !== "undefined") {
          localStorage.setItem("payverge_had_session", "1");
        }

        switch (sessionInfo.type) {
          case "staff":
            setAuthSource("staff");
            updateStaffData({
              id: sessionInfo.staff_id!,
              email: sessionInfo.email!,
              name: sessionInfo.staff_name || "",
              role: (sessionInfo.role || "server") as StaffData["role"],
              business_id: sessionInfo.business_id!,
              business_name: sessionInfo.business_name,
              business_slug: sessionInfo.business_slug,
              is_active: true,
            });
            await fetchStaffProfile();
            break;

          case "user":
            setAuthSource("oauth");
            // Absent (pre-update backend) is treated as verified; only an
            // explicit false shows the banner.
            setEmailVerified(sessionInfo.email_verified !== false);
            if (sessionInfo.picture) {
              oauthPictureRef.current = sessionInfo.picture;
            }
            setOAuthSessionData({
              userId: sessionInfo.user_id!,
              email: sessionInfo.email!,
              role: sessionInfo.role || "user",
              picture: sessionInfo.picture,
            });
            // Skip the secondary /auth/me fetch on public diner pages —
            // session-info already provides everything we need to render
            // the (optional) "Hi <name>" header, and the diner's first
            // network impression of a QR scan shouldn't include an extra
            // round-trip just to flesh out an OAuth profile that may
            // never be used.
            if (!isGuestRoute()) {
              await fetchOAuthUserProfile(sessionInfo.picture);
            }
            break;

          case "web3":
            setAuthSource("dynamic");
            if (sessionInfo.address) {
              await fetchUserProfile(sessionInfo.address, true);
            }
            break;

          case "customer":
            clearUser();
            setOAuthSessionData(null);
            updateStaffData(null);
            setAuthSource("customer");
            break;
        }

        if (sessionInfo.type !== "customer") {
          // Start proactive token refresh timer for hybrid auth only. The
          // refresh is silent — we never prompt the user mid-session; the
          // SessionTimeoutWarning only renders if the refresh truly fails.
          refreshCleanupRef.current?.();
          refreshCleanupRef.current = startTokenRefreshTimer({
            refreshFn: performRefresh,
            onRefreshSuccess: () => {},
            onSessionExpired: () => {
              if (typeof window !== "undefined") {
                localStorage.removeItem(SESSION_HINT_KEY);
              }
              setIsSessionExpired(true);
            },
          });
        } else {
          refreshCleanupRef.current?.();
          refreshCleanupRef.current = null;
          setIsSessionExpired(false);
        }

        setSessionInfoError(false);
        setIsInitialized(true);

        // Check for a stored redirect URL (e.g., from registration flow Google OAuth)
        try {
          const storedRedirectUrl = localStorage.getItem("auth_redirect_url");
          if (storedRedirectUrl) {
            localStorage.removeItem("auth_redirect_url");
            if (isSafeRedirectUrl(storedRedirectUrl)) {
              router.replace(storedRedirectUrl);
            }
          }
        } catch {
          // Ignore storage errors
        }
      } catch (error) {
        void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'initializeSession');
        // Transport failure while checking session-info is not proof of
        // logout. Preserve the last-known session state and let the next
        // bootstrap/refetch reconcile it.
        setSessionInfoError(true);
        setIsInitialized(true);
      } finally {
        setIsLoading(false);
      }
    };

    initializeSession().catch((err) => void logError(err instanceof Error ? err : String(err), 'HybridAuthProvider', 'initializeSession'));
  // initAttemptedRef keeps this bootstrap exactly once even when reactive
  // auth and route inputs change while the async request is in flight.
  // bootstrapGeneration intentionally re-runs after retrySessionBootstrap.
  }, [
    bootstrapGeneration,
    clearAuthSource,
    clearUser,
    fetchOAuthUserProfile,
    fetchStaffProfile,
    fetchUserProfile,
    isInitialized,
    isProtectedSurface,
    isPublicSurface,
    performRefresh,
    router,
    setAuthSource,
    updateStaffData,
    user,
  ]);

  // Surface the re-auth modal when token refresh fails beyond recovery
  // (session expired). P1-10: do NOT navigate or tear the page down here —
  // an operator 20 minutes into a form must keep their input; a background
  // poll's 401 only opens SessionTimeoutWarning, and navigation happens
  // exclusively from the modal CTA. The `alreadyHandled` flag is a defensive
  // one-shot guard so a stream of concurrent 401s (each in-flight request
  // would otherwise dispatch the event independently) opens it once.
  useEffect(() => {
    let alreadyHandled = false;
    const handleSessionExpired = () => {
      if (alreadyHandled) return;
      const currentSource = getAuthSource();
      if (
        currentSource !== "oauth" &&
        currentSource !== "dynamic" &&
        currentSource !== "staff"
      ) {
        return;
      }
      alreadyHandled = true;

      // Stop the proactive refresh timer and the session hint so we stop
      // firing guaranteed-401 refreshes while the modal is up.
      refreshCleanupRef.current?.();
      refreshCleanupRef.current = null;
      if (typeof window !== "undefined") {
        localStorage.removeItem("payverge_had_session");
      }
      setIsSessionExpired(true);
    };
    window.addEventListener("auth:session-expired", handleSessionExpired);
    return () => window.removeEventListener("auth:session-expired", handleSessionExpired);
  }, [getAuthSource]);

  // Handle Web3 disconnects and address changes
  useEffect(() => {
    let mounted = true;

    // Clear any pending fetch timeout
    if (fetchTimeoutRef.current) {
      clearTimeout(fetchTimeoutRef.current);
    }

    const handleConnection = async () => {
      // Avoid disconnect handling while initial auth bootstrap is in progress.
      if (!isInitialized) {
        return;
      }

      // If we have a staff session, don't interfere with Web3 connection logic
      if (isStaffUser) {
        return;
      }

      // No wallet stack on this route (or wagmi is still reconnecting): the
      // wallet state is unknown, so never treat it as a disconnect.
      if (!walletAvailable) {
        return;
      }

      if (!isConnected || !address) {
        // OAuth users can be fully authenticated without a connected wallet.
        if (oauthSessionData || getAuthSource() === "oauth") {
          return;
        }

        // Only clear if auth source is dynamic (Web3)
        if (getAuthSource() === "dynamic") {
          await clearSession("wallet_disconnected");
        }
        return;
      }

      // If connected with address and auth source is dynamic, fetch user profile
      if (getAuthSource() === "dynamic" && mounted) {
        if (fetchTimeoutRef.current) {
          clearTimeout(fetchTimeoutRef.current);
        }
        fetchTimeoutRef.current = setTimeout(() => {
          fetchUserProfile(address, false);
        }, 100);
      }
    };

    handleConnection().catch((err) => void logError(err instanceof Error ? err : String(err), 'HybridAuthProvider', 'handleConnection'));

    return () => {
      mounted = false;
      if (fetchTimeoutRef.current) {
        clearTimeout(fetchTimeoutRef.current);
      }
    };
  }, [
    walletAvailable,
    address,
    isConnected,
    fetchUserProfile,
    clearSession,
    oauthSessionData,
    getAuthSource,
    isInitialized,
    isStaffUser,
  ]);

  // Function to process Dynamic authentication
  const processDynamicAuth = useCallback(async () => {
    try {
      if (!getDynamicAuthIntent()) {
        logger.debug("[HybridAuthProvider] Wallet auth intent not set, skipping wallet sign-in.");
        return;
      }

      if (address && chainId && signMessageAsync) {
        logger.debug("[HybridAuthProvider] Starting wallet challenge sign-in.");

        const challengeResponse = await axiosInstance.post("/auth/challenge", {
          address,
        });
        const challenge = challengeResponse.data?.challenge;
        if (!challenge) {
          throw new Error("Backend did not return a wallet sign-in challenge.");
        }

        const message = buildWalletAuthMessage(
          address,
          challenge,
          chainId,
          "Sign in to Payverge.",
        );
        const signature = await signMessageAsync({ message });
        const response = await axiosInstance.post("/auth/signin", {
          message,
          signature,
          invite_code: getLaunchInviteCode(),
        });

        if (response.data.success && response.data.token) {
          setIsInitialized(true);
          setAuthSource("dynamic");
          clearDynamicAuthIntent();
          if (typeof window !== "undefined") {
            localStorage.setItem("payverge_had_session", "1");
          }
          // Scope apiCache to this Dynamic session before fetching the
          // profile so responses land under the correct fingerprint.
          apiCache.setFingerprint(
            computeSessionFingerprint({ type: "web3", user_id: 0, business_id: 0 }),
          );
          await fetchUserProfile(address, true);
        } else {
          void logError("Wallet sign-in response missing success or token", 'HybridAuthProvider', 'processDynamicAuth');
        }
      }
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'HybridAuthProvider', 'processDynamicAuth');
    }
  }, [
    address,
    chainId,
    clearDynamicAuthIntent,
    fetchUserProfile,
    getDynamicAuthIntent,
    setAuthSource,
    signMessageAsync,
  ]);

  // Watch Dynamic SDK login state directly (more reliable than custom events)
  useEffect(() => {
    // Wallet state unknown here (diner route / reconnecting): keep whatever
    // wallet session exists rather than reading it as a logout.
    if (!walletAvailable) return;
    if (isDynamicLoggedIn && !dynamicAuthProcessedRef.current) {
      dynamicAuthProcessedRef.current = true;
      void processDynamicAuth();
    } else if (!isDynamicLoggedIn && dynamicAuthProcessedRef.current) {
      dynamicAuthProcessedRef.current = false;
      if (getAuthSource() === "dynamic") {
        void clearSession("dynamic_logged_out");
      }
    }
  }, [walletAvailable, isDynamicLoggedIn, processDynamicAuth, clearSession, getAuthSource]);

  // Mirror the active auth source into the (scrubbed) Sentry identity layer.
  // Wallet addresses are never sent raw — reporting.ts pseudonymizes them.
  useEffect(() => {
    if (!isInitialized) {
      return;
    }

    if (authSourceState === "staff" && staffDataState) {
      setSentryIdentity({
        authSource: "staff",
        staffId: staffDataState.id,
        businessId: staffDataState.business_id,
      });
      return;
    }

    if (authSourceState === "oauth" && oauthSessionData) {
      setSentryIdentity({
        authSource: "user",
        userId: oauthSessionData.userId,
      });
      return;
    }

    if (authSourceState === "dynamic" && user?.address) {
      setSentryIdentity({
        authSource: "web3",
        walletAddress: user.address,
      });
      return;
    }

    if (authSourceState === "customer") {
      setSentryIdentity({
        authSource: "customer",
      });
      return;
    }

    clearHybridSentryIdentities();
  }, [
    authSourceState,
    clearHybridSentryIdentities,
    isInitialized,
    oauthSessionData,
    staffDataState,
    user?.address,
  ]);

  // Context value - derive from state instead of cookie-based checks
  const contextValue: AuthContextType = {
    isWeb3User: !!user && authSourceState === "dynamic",
    isStaffUser,
    isOAuthUser: !!oauthSessionData,
    oauthData: oauthSessionData,
    staffData: staffDataState,
    refreshStaffData: async () => {
      await fetchStaffProfile();
    },
    walletLinked: !!user?.address,
    walletAddress: user?.address || null,
    linkWallet,
    unlinkWallet,
    refreshOAuthUser,
    refreshSession,
    retrySessionBootstrap,
    isLoading,
    isInitialized,
    sessionInfoError,
    emailVerified,
  };

  return (
    <AuthContext.Provider value={contextValue}>
      {!isGuestSurface && <VerifyEmailBanner />}
      {children}
      <SessionTimeoutWarning
        isExpired={isSessionExpired && !isGuestSurface}
        onLogin={() => {
          setIsSessionExpired(false);
          updateStaffData(null);
          clearUser();
          setOAuthSessionData(null);
          clearAuthSource();
          if (isStaffUser) {
            router.replace("/staff/login");
          } else {
            router.replace("/");
          }
        }}
      />
    </AuthContext.Provider>
  );
}
