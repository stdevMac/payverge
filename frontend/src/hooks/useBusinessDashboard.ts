import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { useAuth } from "@/providers/HybridAuthProvider";
import { getUserProfile } from "@/api/users/profile";
import { getBusiness } from "@/api/business";
import { UserInterface as User } from "@/interface/users/users-interface";
import { logError } from "@/utils/errorLogger";
import { queryKeys } from "@/api/queryKeys";

export const useBusinessDashboard = (businessId: string) => {
  const [authLoading, setAuthLoading] = useState(true);
  const [authError, setAuthError] = useState<string | null>(null);

  const { address, isConnected } = useAccount();
  const {
    isOAuthUser: providerOAuthUser,
    isStaffUser: providerStaffUser,
    isWeb3User: providerWeb3User,
    oauthData,
    isLoading: providerLoading,
    isInitialized: providerInitialized,
  } = useAuth();
  const user = useUserStore((state) => state.user);

  const authCheckedRef = useRef(false);

  // Authentication check — identical logic to original, preserving all auth paths
  useEffect(() => {
    if (!providerInitialized || providerLoading) return;
    if (authCheckedRef.current) return;

    if (providerOAuthUser || providerStaffUser) {
      if (providerOAuthUser && oauthData?.email) {
        const currentUser = useUserStore.getState().user;
        if (!currentUser || currentUser.email !== oauthData.email) {
          useUserStore.getState().setUser({
            username: oauthData.email.split("@")[0],
            email: oauthData.email,
            address: "",
            role: oauthData.role || "user",
            joined_at: new Date().toISOString(),
            language_selected: "en",
            notifications: [],
          });
        }
      }
      authCheckedRef.current = true;
      setAuthLoading(false);
      setAuthError(null);
      return;
    }

    if (providerWeb3User && isConnected && address) {
      const loadWeb3User = async () => {
        try {
          const currentUser = useUserStore.getState().user;
          if (
            !currentUser ||
            currentUser.address.toLowerCase() !== address.toLowerCase()
          ) {
            try {
              const userData = await getUserProfile(address);
              if (userData) {
                useUserStore.getState().setUser(userData);
              } else {
                useUserStore.getState().setUser({
                  username: "",
                  email: "",
                  address: address.toLowerCase(),
                  role: "user",
                  joined_at: new Date().toISOString(),
                  language_selected: "en",
                  notifications: [],
                } as User);
              }
            } catch {
              useUserStore.getState().setUser({
                username: "",
                email: "",
                address: address.toLowerCase(),
                role: "user",
                joined_at: new Date().toISOString(),
                language_selected: "en",
                notifications: [],
              } as User);
            }
          }
          authCheckedRef.current = true;
          setAuthLoading(false);
          setAuthError(null);
        } catch (err) {
          void logError(
            err instanceof Error ? err : String(err),
            "useBusinessDashboard",
            "loadWeb3User"
          );
          setAuthError("auth:failed");
          authCheckedRef.current = true;
          setAuthLoading(false);
        }
      };
      loadWeb3User().catch((err) => console.error("loadWeb3User failed:", err));
      return;
    }

    setAuthError("auth:signin-required");
    authCheckedRef.current = true;
    setAuthLoading(false);
  }, [
    providerInitialized,
    providerLoading,
    providerOAuthUser,
    providerStaffUser,
    providerWeb3User,
    isConnected,
    address,
    oauthData,
  ]);

  // Clear auth error on successful login. Branch on the stable auth CODE, not
  // English message substrings — the hook now emits codes, not literals.
  useEffect(() => {
    if (user && isConnected && authError !== null) {
      if (authError === "auth:signin-required" || authError === "auth:failed") {
        setAuthError(null);
      }
    }
  }, [user, isConnected, authError]);

  // Business data fetch via React Query
  const {
    data: business = null,
    isLoading: businessLoading,
    error: businessQueryError,
    refetch: refreshBusiness,
  } = useQuery({
    queryKey: queryKeys.business.dashboard(businessId),
    queryFn: () => getBusiness(businessId),
    enabled: !!businessId && !authLoading && !authError,
    staleTime: 30 * 1000, // 30 seconds
    retry: (failureCount, error) => {
      // Branch on the numeric HTTP status, not the message text. In production
      // sanitizeError rewrites messages, so a "403"/"Not Found" substring check
      // never matches and 404/403 would (wrongly) retry before showing the
      // generic message.
      const status = getErrorStatus(error);
      // Don't retry on client errors we can't recover from (auth/not-found/perm).
      if (status === 403 || status === 404) return false;
      // M1 — React Query is the single retry owner now that axios no longer
      // retries GETs. Bound this to 2 total attempts (was <3 = 4 attempts, and
      // previously each RQ attempt fanned out to axios's own 3 exp-backoff
      // retries → 8-16 requests and a 30-60s lag before `isError` surfaced).
      return failureCount < 2;
    },
  });

  const businessError = businessQueryError
    ? formatBusinessError(businessQueryError)
    : null;
  const error = authError || businessError;
  const loading = authLoading || businessLoading;

  return {
    business,
    loading,
    authLoading,
    error,
    refreshBusiness: () => { refreshBusiness(); },
    user,
    isConnected,
    address,
  };
};

// Read the numeric HTTP status off whatever error shape the fetch layer threw.
// Prefer an explicit `status`, fall back to axios-style `response.status`. Do
// NOT sniff the message text — production sanitizeError rewrites it.
function getErrorStatus(error: unknown): number | undefined {
  const e = error as
    | { status?: number; response?: { status?: number } }
    | null
    | undefined;
  return e?.status ?? e?.response?.status;
}

// Map a business-fetch failure to a STABLE error CODE. The layout translates
// the code at render via tString; keep no English literals here.
function formatBusinessError(err: unknown): string {
  const status = getErrorStatus(err);
  if (status === 404) return "business:not-found";
  if (status === 403) return "business:forbidden";
  if (status === 429) return "business:rate-limited";
  return "business:generic";
}
