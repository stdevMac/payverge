"use client";
import {
  useState,
  useEffect,
  useCallback,
  useRef,
  useTransition,
  Suspense,
} from "react";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { useAuth } from "@/providers/HybridAuthProvider";

import { Business, getMyBusinesses } from "@/api/business";

import { analyticsApi, DashboardSummary } from "@/api/analytics";
import { BusinessOverviewList } from "@/components/dashboard/BusinessOverviewList";
import { LayoutDashboard, AlertCircle } from "lucide-react";
import Link from "next/link";
import { Button, Card, CardBody } from "@nextui-org/react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { usePageTracking, useClickTracking } from "@/hooks/useAnalytics";
import { AuthModal } from "@/components/auth/AuthModal";
import {
  getBusinessDashboardPath,
  getOperatorDashboardPath,
} from "@/utils/businessUrl";
import { rememberOpenVenue } from "@/utils/openVenueMemory";
import { errMessage, getApiErrorStatus } from "@/utils/apiError";
import {
  AuthValueProp,
  AuthValuePropCompact,
} from "@/components/dashboard/AuthValueProp";
import { useRouter, useSearchParams } from "next/navigation";
import { resolvePostLoginRedirect } from "@/utils/safeRedirect";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";
import { useInstance } from "@/hooks/useInstance";
import { isPublicDemo } from "@/lib/instance/instanceInfo";
import { useCookieConsent } from "@/contexts/CookieConsentContext";
import { readConsent } from "@/lib/analytics/consentGate";

function DashboardInner() {
  const { isConnected } = useAccount();
  const { user } = useUserStore();
  const {
    isOAuthUser,
    isStaffUser,
    isWeb3User,
    isInitialized,
    oauthData,
    sessionInfoError,
    staffData,
  } = useAuth();
  const { locale } = useSimpleLocale();
  const searchParams = useSearchParams();
  // Self-hosted instances without Google sign-in must not offer it from the
  // sign-in gate.
  const { isOff: instanceOff, instance } = useInstance();
  // "Sign Up" only where this instance takes new accounts.
  const signUpOpen = instance?.registration_mode === "open";

  // Dashboard state. Invite links (`/?invite_code=…`, redirected here when no
  // venue is published yet) and `?auth=signin|signup` open the auth dialog
  // straight away, on the sign-up tab for an invite.
  const linkedInvite = searchParams?.get("invite_code") || "";
  const authParam = searchParams?.get("auth");
  const authModalTab: "signin" | "signup" =
    linkedInvite || authParam === "signup" ? "signup" : "signin";
  const [showAuthModal, setShowAuthModal] = useState(
    Boolean(linkedInvite) || authParam === "signin" || authParam === "signup",
  );
  // On the public demo (DEMO_MODE) the dialog opens straight away, once: its
  // one-click owner/staff buttons are the way in. The instance can arrive
  // after the first render, so this follows it rather than the initial state.
  const publicDemo = isPublicDemo(instance);
  const [demoDialogOpened, setDemoDialogOpened] = useState(false);
  // The cookie banner is outside the modal, so an open dialog would make it
  // unreachable (the modal hides and blocks everything outside it). The
  // dialog waits until the visitor has answered the banner. Before the
  // provider has read storage, read it directly so the dialog cannot win the
  // race on a cached instance.
  const { consent: cookieConsent, ready: cookieReady } = useCookieConsent();
  const cookieDecisionPending = cookieReady
    ? cookieConsent === null
    : readConsent() === null;
  useEffect(() => {
    if (!publicDemo || demoDialogOpened || cookieDecisionPending) return;
    setDemoDialogOpened(true);
    setShowAuthModal(true);
  }, [publicDemo, demoDialogOpened, cookieDecisionPending]);

  // OAuth token handling is centralized in HybridAuthProvider

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `dashboard.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [businessStats, setBusinessStats] = useState<
    Record<number, DashboardSummary>
  >({});
  const [loading, setLoading] = useState(true);
  const [statsLoading, setStatsLoading] = useState(false);
  // #832: the batch/per-business stats fetch records failures here and the
  // portfolio list renders them as an inline alert with a retry — previously
  // the value binding was dropped and a failed load silently posed as "—" /
  // "No data yet".
  const [statsError, setStatsError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Auth failure detected by HTTP status (401/403), not string matching. When
  // true the sign-in UI takes over and the "no businesses" empty state is
  // suppressed — a logged-out user must never be told they have no businesses.
  const [authError, setAuthError] = useState(false);
  // Explicit sign-in gate for pre-fetch paths (no auth surface / wallet not
  // connected). Must not rely on English substrings in error text — translated
  // locales never match those heuristics.
  const [needsSignIn, setNeedsSignIn] = useState(false);
  // Only after a *successful* fetch may the empty state render.
  const [loadedOk, setLoadedOk] = useState(false);
  const [authChecked, setAuthChecked] = useState(false);
  // Bumped on every principal switch; a load only applies its result while
  // its generation is current.
  const loadGeneration = useRef(0);

  // Analytics tracking
  usePageTracking();
  const trackClick = useClickTracking();

  // Navigation feedback: "Administrar" shows a pending/loading state while the
  // business dashboard route loads, so users don't double-tap (audit L2).
  const router = useRouter();
  const [pendingBusinessId, setPendingBusinessId] = useState<number | null>(
    null,
  );
  const [isNavigating, startNavigation] = useTransition();

  // An operator with exactly one venue goes straight to its dashboard; the
  // picker only earns its place with two or more. `?venues=all` keeps the
  // overview reachable (e.g. to add a second venue from it).
  const showAllVenues = searchParams?.get("venues") === "all";
  const [redirectingToVenue, setRedirectingToVenue] = useState(false);

  const handleManageBusiness = useCallback(
    (business: Business) => {
      trackClick(
        "dashboard-manage-business",
        "navigation",
        `business-${business.id}`,
      );
      setPendingBusinessId(business.id);
      rememberOpenVenue(business);
      startNavigation(() => router.push(getBusinessDashboardPath(business)));
    },
    [router, trackClick],
  );

  const retryLoadUser = async () => {
    setError(null);
    setLoading(true);
    setAuthChecked(false);
    window.location.reload();
  };

  const retrySession = () => {
    window.location.reload();
  };

  const applyBusinessStats = useCallback(
    (
      _businessList: Business[],
      statsMap: Record<number, DashboardSummary>,
      generation: number,
    ) => {
      if (generation !== loadGeneration.current) return;
      setBusinessStats(statsMap);
    },
    [],
  );

  const loadStatsPerBusiness = useCallback(async (businessList: Business[]) => {
    const statsMap: Record<number, DashboardSummary> = {};
    await Promise.all(
      businessList.map(async (b) => {
        try {
          statsMap[b.id] = await analyticsApi.getDashboardSummary(
            b.id.toString(),
          );
        } catch (err) {
          console.error(`Failed to load stats for business ${b.id}:`, err);
        }
      }),
    );
    return statsMap;
  }, []);

  const loadStatsForBusinesses = useCallback(
    async (businessList: Business[]) => {
      const generation = loadGeneration.current;
      setStatsLoading(true);
      setStatsError(null);
      try {
        const statsMap: Record<number, DashboardSummary> = {};
        let summaries: Record<string, DashboardSummary>;

        try {
          summaries = await analyticsApi.getDashboardSummaries(
            businessList.map((b) => b.id.toString()),
          );
        } catch (batchErr) {
          console.warn(
            "Batch dashboard summaries failed, falling back per business:",
            batchErr,
          );
          const fallbackMap = await loadStatsPerBusiness(businessList);
          if (Object.keys(fallbackMap).length === 0) {
            throw batchErr;
          }
          applyBusinessStats(businessList, fallbackMap, generation);
          return;
        }

        businessList.forEach((b) => {
          const summary = summaries[b.id.toString()];
          if (summary) {
            statsMap[b.id] = summary;
          }
        });

        applyBusinessStats(businessList, statsMap, generation);
      } catch (err) {
        if (generation !== loadGeneration.current) return;
        console.error("Failed to load aggregate stats:", err);
        setStatsError(tString("stats.loadError"));
      } finally {
        if (generation === loadGeneration.current) setStatsLoading(false);
      }
    },
    [applyBusinessStats, loadStatsPerBusiness, tString],
  );

  const loadBusinesses = useCallback(async () => {
    const generation = loadGeneration.current;
    const stale = () => generation !== loadGeneration.current;
    try {
      setLoading(true);
      setAuthError(false);
      const data = await getMyBusinesses();
      if (stale()) return;
      if (data.length === 1 && !showAllVenues) {
        rememberOpenVenue(data[0]);
        setRedirectingToVenue(true);
        router.replace(getBusinessDashboardPath(data[0]));
        return;
      }
      setBusinesses(data);
      setLoadedOk(true);

      // Load stats for each business
      if (data.length > 0) {
        loadStatsForBusinesses(data);
      }
    } catch (err: unknown) {
      if (stale()) return;
      console.error("[Dashboard] loadBusinesses - error:", err);
      // Detect an expired/insufficient session by HTTP status (the sanitized
      // axios error copies both a top-level `.status` and `.response.status`).
      // On 401/403 we render the sign-in UI in place — never a client redirect
      // (see auth:session-expired recursion bug); global session-expired
      // handling continues to run.
      const status = getApiErrorStatus(err);
      if (status === 401 || status === 403) {
        setAuthError(true);
        return;
      }
      setError(errMessage(err) ?? null);
    } finally {
      if (!stale()) setLoading(false);
    }
  }, [loadStatsForBusinesses, router, showAllVenues]);

  // The signed-in principal this page loaded for. The orchestration below
  // latches on `authChecked`, so without this a principal switch (sign out,
  // another owner or staff member signs in on the same tab) would keep
  // rendering the previous principal's venues and stats.
  const principalKey: string | null = isOAuthUser
    ? oauthData?.userId != null
      ? `oauth:${oauthData.userId}`
      : null
    : isStaffUser
      ? staffData?.id != null
        ? `staff:${staffData.id}`
        : null
      : isWeb3User && user?.address
        ? `web3:${user.address.toLowerCase()}`
        : null;
  const lastPrincipalKey = useRef<string | null | undefined>(undefined);

  // Must run before the orchestration effect so a switch never renders the
  // old list after authChecked is cleared.
  useEffect(() => {
    if (!isInitialized) return;
    const previous = lastPrincipalKey.current;
    lastPrincipalKey.current = principalKey;
    if (previous === undefined || previous === principalKey) return;
    // Results of loads started for the previous principal are dropped.
    loadGeneration.current += 1;
    setBusinesses([]);
    setBusinessStats({});
    setStatsError(null);
    setStatsLoading(false);
    setLoadedOk(false);
    setError(null);
    setAuthError(false);
    setNeedsSignIn(false);
    setRedirectingToVenue(false);
    setLoading(true);
    setAuthChecked(false);
  }, [isInitialized, principalKey]);

  // No longer needed - auth state comes from HybridAuthProvider

  // Copy address function

  // Main orchestration effect — driven by HybridAuthProvider's isInitialized
  // so we never poll/timeout/reload waiting for auth state to settle. Any of
  // the three auth surfaces (OAuth, staff, web3) being true means the user
  // is signed in; for web3 we still need wagmi's isConnected and the
  // zustand user profile loaded.
  useEffect(() => {
    if (authChecked) return;
    if (!isInitialized) return;

    // A staff session belongs to exactly one venue, and the owner-only
    // venue list (/inside/businesses) refuses it, so /dashboard sends staff
    // straight to their venue instead of rendering the sign-in gate.
    if (isStaffUser && !isOAuthUser && staffData?.business_id) {
      setAuthChecked(true);
      setRedirectingToVenue(true);
      router.replace(
        getOperatorDashboardPath(
          String(staffData.business_slug || staffData.business_id),
        ),
      );
      return;
    }

    if (isOAuthUser || isStaffUser) {
      setAuthChecked(true);
      setError(null);
      setNeedsSignIn(false);
      loadBusinesses();
      return;
    }

    if (isWeb3User) {
      if (!isConnected) {
        setError(tString("errors.connectWallet"));
        setNeedsSignIn(true);
        setLoading(false);
        setAuthChecked(true);
        return;
      }
      // Wait for HybridAuthProvider to finish fetching the wallet profile.
      // The effect will re-run when `user` populates.
      if (!user) return;

      setAuthChecked(true);
      setError(null);
      setNeedsSignIn(false);
      loadBusinesses();
      return;
    }

    // Initialized but no auth surface is active — surface the sign-in UI.
    setError(tString("errors.connectWallet"));
    setNeedsSignIn(true);
    setLoading(false);
    setAuthChecked(true);
  }, [
    isInitialized,
    isConnected,
    user,
    authChecked,
    isOAuthUser,
    isStaffUser,
    isWeb3User,
    loadBusinesses,
    router,
    staffData,
    tString,
  ]);

  // Anonymous / unauthenticated visitors must never see the authenticated
  // "Loading your businesses…" shell (footer Dashboard deep-link flash).
  // Derive the gate synchronously once HybridAuthProvider has settled so the
  // first paint after init is the sign-in UI, not a businesses spinner.
  const hasAuthSurface = isOAuthUser || isStaffUser || isWeb3User;

  // `?auth=signin|signup` only asks for the dialog. Once a session exists it
  // has done its job; leaving it in the URL would reopen the dialog on reload
  // or share a sign-in link instead of the dashboard.
  // A staff-only session is leaving for its venue (the orchestration effect
  // above), which drops the query anyway; a replace to /dashboard here would
  // race that redirect and could win, leaving the venue spinner up for good.
  const staffGoesToVenue =
    isStaffUser && !isOAuthUser && Boolean(staffData?.business_id);
  useEffect(() => {
    if (!authParam || !isInitialized || !hasAuthSurface) return;
    if (staffGoesToVenue) return;
    const next = new URLSearchParams(searchParams?.toString() ?? "");
    next.delete("auth");
    const query = next.toString();
    setShowAuthModal(false);
    router.replace(query ? `/dashboard?${query}` : "/dashboard");
  }, [
    authParam,
    isInitialized,
    hasAuthSurface,
    staffGoesToVenue,
    searchParams,
    router,
  ]);
  const showSignIn =
    authError || needsSignIn || (isInitialized && !hasAuthSurface);

  // Session still bootstrapping — neutral spinner, not the businesses label.
  if (!isInitialized && !showSignIn) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-warm-50">
        <div className="text-center">
          <div
            role="status"
            aria-label={
              (getTranslation("common.loadingSession", locale) as string) ||
              "Loading session"
            }
            className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-brand/20 border-t-brand motion-safe:animate-spin"
          />
        </div>
      </div>
    );
  }

  if (
    (redirectingToVenue || (loading && !businesses.length && hasAuthSurface)) &&
    !showSignIn
  ) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-warm-50">
        <div className="text-center">
          <div
            role="status"
            aria-label={tString("loading")}
            className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-brand/20 border-t-brand motion-safe:animate-spin"
          />
          <p className="mt-4 text-sm text-ink-600">{tString("loading")}</p>

          {/* Action buttons */}
          {error === tString("errors.loadingUserData") && (
            <div className="mt-6">
              <button
                onClick={retryLoadUser}
                className="px-4 py-2 text-sm font-medium text-white bg-ink-900 border border-transparent rounded-md hover:bg-ink-800 transition-colors duration-200"
              >
                {tString("authentication.retryLoadingUser")}
              </button>
            </div>
          )}
        </div>
      </div>
    );
  }

  // Show authentication UI — on a status-detected auth failure (401/403) OR
  // an explicit pre-fetch needsSignIn flag (no auth surface / wallet not
  // connected). Do not string-match error text: translated locales fail that.
  if (showSignIn) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Network-error banner — sticky top-of-fold */}
        {sessionInfoError && (
          <div className="sticky top-0 z-40 bg-rose-50 border-b border-rose-200 text-rose-900 px-4 py-3 text-sm flex items-center justify-center gap-3">
            <AlertCircle className="w-4 h-4 flex-shrink-0" />
            <span>{tString("errors.unreachable")}</span>
            <button
              onClick={retrySession}
              className="underline font-semibold hover:text-rose-700"
            >
              {getTranslation("common.retry", locale) as string}
            </button>
          </div>
        )}
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-brand/5 to-brand/5 rounded-full blur-3xl opacity-30 motion-safe:animate-pulse"></div>
          <div
            className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-brand/5 rounded-full blur-3xl opacity-20 motion-safe:animate-pulse"
            style={{ animationDelay: "2s" }}
          ></div>
        </div>

        <div className="relative z-10 grid min-h-screen grid-cols-1 md:grid-cols-2 max-w-7xl mx-auto">
          <div className="flex items-center justify-center px-6 py-16">
            <div className="text-center max-w-md mx-auto">
              <div className="w-16 h-16 bg-ink-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-ink-100">
                <svg
                  className="w-8 h-8 text-brand"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                  />
                </svg>
              </div>
              <h1 className="font-title text-4xl md:text-5xl text-ink-900 mb-4">
                {tString("authentication.title")}
              </h1>
              <p className="text-ink-600 tracking-wide mb-8">
                {tString(
                  instanceOff("google_oauth")
                    ? "authentication.subtitleEmail"
                    : "authentication.subtitle",
                )}
              </p>

              {isOAuthUser ? (
                <div className="space-y-4">
                  <p className="text-sm text-gray-700">
                    {tString("authentication.welcomeBack").replace(
                      "{email}",
                      oauthData?.email ?? "",
                    )}
                  </p>
                  <button
                    onClick={() => window.location.reload()}
                    className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
                  >
                    {tString("authentication.tryAgain")}
                  </button>
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="flex justify-center">
                    <Button
                      onClick={() => setShowAuthModal(true)}
                      className="bg-brand text-white px-6 py-3 rounded-full hover:bg-brand-dark font-semibold"
                    >
                      {tString(
                        signUpOpen
                          ? "authentication.signInSignUp"
                          : "authentication.signInOnly",
                      )}
                    </Button>
                  </div>
                  <AuthModal
                    isOpen={showAuthModal}
                    onClose={() => setShowAuthModal(false)}
                    defaultTab={authModalTab}
                    redirectUrl={searchParams?.get("redirect") ?? undefined}
                    onSuccess={() => {
                      setShowAuthModal(false);
                      const dest = resolvePostLoginRedirect(
                        searchParams?.get("redirect") ?? null,
                      );
                      if (dest) {
                        window.location.assign(dest);
                      } else {
                        window.location.reload();
                      }
                    }}
                  />
                </div>
              )}

              <div className="mt-6">
                <AuthValuePropCompact />
              </div>
            </div>
          </div>
          <AuthValueProp />
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-warm-50 relative overflow-hidden font-sans">
      {/* Network-error banner — sticky top-of-fold */}
      {sessionInfoError && (
        <div className="sticky top-0 z-40 bg-rose-50 border-b border-rose-200 text-rose-900 px-4 py-3 text-sm flex items-center justify-center gap-3">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{tString("errors.unreachable")}</span>
          <button
            onClick={retrySession}
            className="underline font-semibold hover:text-rose-700"
          >
            {getTranslation("common.retry", locale) as string}
          </button>
        </div>
      )}

      <div className="relative z-10 max-w-screen-2xl mx-auto px-4 sm:px-6 lg:px-8 py-12 lg:py-16">
        {/* Header Section — flattened to the shared PageHeader idiom (L17):
            serif font-title, left-aligned, quiet subtitle. The twin ~500px blur
            orbs and the pulse-dot marketing hero read as an over-designed
            landing page, not an operator surface (don't-overdesign-dashboards).
            The badge keeps a quiet glass chrome (bg-white/70 backdrop-blur-md
            replaces the undefined `transparent-glass` class) with no pulse. */}
        <header className="mb-14">
          <div className="flex flex-col gap-3">
            <span className="inline-flex w-fit items-center gap-2 px-3 py-1 bg-white/70 backdrop-blur-md border border-warm-200 rounded-full shadow-sm">
              <span className="text-xs font-semibold text-ink-600 uppercase tracking-widest">
                {tString("badge")}
              </span>
            </span>
            <h1 className="font-title text-2xl tracking-tight text-ink-900 sm:text-3xl">
              {tString("title")}
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-ink-500">
              {tString("subtitle")}
            </p>
          </div>
        </header>

        {/* Error Alert */}
        {error && (
          <div className="mb-8 p-4 bg-rose-50 border border-rose-200 rounded-2xl flex items-center gap-3 text-rose-800">
            <div className="p-2 bg-rose-100 rounded-xl">
              <svg
                className="w-5 h-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <div>
              <p className="font-semibold">{tString("errors.error")}</p>
              <p className="text-sm opacity-90">{error}</p>
            </div>
          </div>
        )}

        {/* Businesses Section */}
        <section>
          {loadedOk && businesses.length === 0 ? (
            <Card className="border border-ink-300 shadow-sm p-12 lg:p-24 text-center">
              <CardBody className="flex flex-col items-center">
                <div className="w-20 h-20 bg-ink-50 rounded-3xl flex items-center justify-center mb-8 border border-ink-100 text-ink-400">
                  <LayoutDashboard className="w-10 h-10" />
                </div>
                <h3 className="text-2xl font-bold text-ink-900 tracking-tight mb-4">
                  {tString("businesses.noneTitle")}
                </h3>
                <p className="text-ink-700 max-w-md mx-auto mb-10 text-lg leading-relaxed">
                  {tString("businesses.noneDescription")}
                </p>
                <Button
                  as={Link}
                  href="/business/register"
                  className="bg-ink-900 text-white font-semibold px-10 py-7 rounded-2xl shadow-lg hover:shadow-xl hover:bg-ink-800 transition-all duration-300 text-lg"
                >
                  {tString("businesses.createFirst")}
                </Button>
              </CardBody>
            </Card>
          ) : (
            <BusinessOverviewList
              businesses={businesses}
              stats={businessStats}
              statsLoading={statsLoading}
              statsError={statsError}
              onRetryStats={() => {
                if (businesses.length > 0) {
                  void loadStatsForBusinesses(businesses);
                }
              }}
              onManage={handleManageBusiness}
              navigatingBusinessId={isNavigating ? pendingBusinessId : null}
              t={tString}
              viewerUserId={oauthData?.userId ?? null}
              canAddBusiness={!publicDemo}
            />
          )}
        </section>
      </div>
    </div>
  );
}

// useSearchParams() must sit under a Suspense boundary or Next.js fails to
// statically render this route at build time — mirrors the /account pattern.
export default function Dashboard() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <DashboardInner />
    </Suspense>
  );
}
