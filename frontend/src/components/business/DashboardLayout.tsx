"use client";

import React from "react";
import Link from "next/link";
import Image from "next/image";
import { Button } from "@nextui-org/react";
import { Menu, AlertCircle, LockKeyhole } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { Business } from "@/api/business";
import { StaffData } from "@/utils/staffAuth";
import DashboardSidebar from "./DashboardSidebar";
import { MobileTodayBar } from "./sidebar/MobileTodayBar";
import { Reservation } from "@/api/reservations";
import { AuthModal } from "@/components/auth/AuthModal";
import { ConnectivityProvider } from "@/contexts/ConnectivityContext";
import { OfflineBanner } from "@/components/ui/OfflineBanner";
import { principalQueueId, setActiveUserId } from "@/lib/mutationQueue";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useOfflineMutation } from "@/hooks/useOfflineMutation";
import { CommandPaletteProvider } from "./commandPalette/CommandPaletteProvider";
import CommandPaletteTrigger from "./commandPalette/CommandPaletteTrigger";
import OpsAssistantWidget from "@/components/opsAssistant/OpsAssistantWidget";
import { useInstance } from "@/hooks/useInstance";
import OpsAssistantProactiveNudge from "@/components/opsAssistant/OpsAssistantProactiveNudge";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import type { AccessState } from "./commandPalette/tabAccess";
import RecentAlertsPopover from "./operational-alerts/RecentAlertsPopover";
import DashboardShellSkeleton from "./shared/DashboardShellSkeleton";
import { setOperatorAccessError } from "@/hooks/useOperatorAccessError";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { useTabScrollRestore } from "@/hooks/useTabScrollRestore";
import { shouldReplaceDashboardWithFetchError } from "./dashboardFetchErrorPolicy";
import { brandLinks } from "@/config/brand";
import { VENUES_OVERVIEW_PATH } from "@/utils/businessUrl";

function OfflineQueueDrain({ userId }: { userId: string }) {
  useOfflineMutation(userId);
  return null;
}

function DashboardStateFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative min-h-screen overflow-hidden bg-warm-50 text-ink-950">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_16%_10%,rgba(26,107,106,0.12),transparent_30%),radial-gradient(circle_at_86%_18%,rgba(255,255,255,0.86),transparent_28%),linear-gradient(180deg,#faf9f6_0%,#f2f0ea_100%)]" />
      <div className="pointer-events-none absolute inset-0 opacity-[0.28] [background-image:linear-gradient(rgba(255,255,255,0.38)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.38)_1px,transparent_1px)] [background-size:28px_28px]" />
      <div className="relative z-[1] flex min-h-screen items-center justify-center px-6 py-10">
        <section className="relative w-full max-w-md overflow-hidden rounded-2xl border border-warm-200/90 bg-white/90 p-8 text-center shadow-[0_24px_80px_rgba(46,42,37,0.10)] backdrop-blur-xl">
          <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-white/90" />
          {children}
        </section>
      </div>
    </div>
  );
}

const CORE_TUTORIAL_TABS = [
  "overview",
  "bills",
  "kitchen",
  "reservations",
  "analytics",
] as const;

// Preview screenshot shown alongside each core tab's tour step. Images live in
// frontend/public/onboarding/ and are displayed object-contain on a warm
// backdrop, so their varying aspect ratios are letterboxed (never cropped).
// Counters is setup-only (not dinner-ready), so it is not part of the core tour.
const TUTORIAL_TAB_IMAGES: Record<(typeof CORE_TUTORIAL_TABS)[number], string> =
  {
    overview: "/onboarding/overview.webp",
    bills: "/onboarding/bills.webp",
    kitchen: "/onboarding/kds.webp",
    reservations: "/onboarding/reservations.webp",
    analytics: "/onboarding/analytics.webp",
  };

interface TutorialStep {
  type: "tab" | "support";
  tabKey?: string;
  title: string;
  description: string;
  image?: string;
}

interface DashboardLayoutProps {
  business: Business | null;
  loading: boolean;
  authLoading: boolean;
  error: string | null;
  activeTab: string;
  setActiveTab: (tab: string) => void;
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
  children: React.ReactNode;
  isConnected: boolean;
  allowedTabs?: string[];
  isStaffUser?: boolean;
  staffData?: StaffData | null;
  globalOrders?: Record<number, any[]>;
  globalOrdersLoaded?: boolean;
  occupiedTablesCount?: number;
  upcomingReservations?: Reservation[];
  venueName?: string | null;
}

export default function DashboardLayout({
  business,
  loading,
  authLoading,
  error,
  activeTab,
  setActiveTab,
  sidebarOpen,
  setSidebarOpen,
  children,
  isConnected,
  allowedTabs = [],
  isStaffUser = false,
  staffData = null,
  globalOrders = {},
  globalOrdersLoaded,
  occupiedTablesCount,
  upcomingReservations = [],
  venueName = null,
}: DashboardLayoutProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [showAuthModal, setShowAuthModal] = React.useState(false);
  const [tutorialOpen, setTutorialOpen] = React.useState(false);
  const [opsOpenSignal, setOpsOpenSignal] = React.useState(0);
  // No LLM provider on this install: the ops assistant has nothing to answer
  // with, so it has no entry point (GET /instance features.ai).
  const { isOff: instanceIsOff, instance } = useInstance();
  const aiOff = instanceIsOff("ai");
  // "Sign Up" only where this instance takes new accounts.
  const signUpOpen = instance?.registration_mode === "open";
  const accessHook = useBusinessAccess(business?.id);
  const accessState: AccessState = {
    // M7 / #773: a failed access fetch must not lock the mobile rail.
    // Treat the error like loading (never gate).
    loading: accessHook.loading || accessHook.isError,
    isSuspended: accessHook.isSuspended,
  };
  const [tutorialStepIndex, setTutorialStepIndex] = React.useState(0);
  const isMobileNav = useMediaQuery("(max-width: 1023px)");
  const mobileDrawerOpen = isMobileNav && sidebarOpen;
  const { oauthData, staffData: authStaff, walletAddress } = useAuth();
  const queueUserId = principalQueueId(
    oauthData?.userId,
    staffData?.id ?? authStaff?.id,
    walletAddress,
  );

  // M2: remember each rail's scroll position (scoped per business) so
  // returning to a tab lands where the operator left off.
  const mainRef = React.useRef<HTMLDivElement>(null);
  useTabScrollRestore(mainRef, activeTab, String(business?.id ?? "anon"));

  React.useEffect(() => {
    if (queueUserId) setActiveUserId(queueUserId);
    return () => setActiveUserId("");
  }, [queueUserId]);

  const tString = React.useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const tutorialSteps = React.useMemo<TutorialStep[]>(() => {
    const allowedTabSet = allowedTabs.length > 0 ? new Set(allowedTabs) : null;

    const coreTabSteps = CORE_TUTORIAL_TABS.filter(
      (tabKey) => !allowedTabSet || allowedTabSet.has(tabKey),
    ).map((tabKey) => ({
      type: "tab" as const,
      tabKey,
      title: tString(`tabs.${tabKey}`),
      description: tString(`tabs.${tabKey}Desc`),
      image: TUTORIAL_TAB_IMAGES[tabKey],
    }));

    if (coreTabSteps.length === 0) return [];

    const steps: TutorialStep[] = [];

    steps.push(...coreTabSteps);

    steps.push({
      type: "support" as const,
      title: tString("tutorial.supportTitle"),
      description: tString("tutorial.supportDescription"),
    });

    return steps;
  }, [allowedTabs, tString]);

  const currentTutorialStep = tutorialSteps[tutorialStepIndex] || null;
  const currentTutorialTabKey =
    currentTutorialStep?.type === "tab"
      ? currentTutorialStep.tabKey || null
      : null;
  const currentTutorialTarget = currentTutorialStep?.type || null;
  const isLastTutorialStep =
    tutorialStepIndex === tutorialSteps.length - 1 && tutorialSteps.length > 0;
  const lastTutorialSyncedTabRef = React.useRef<string | null>(null);

  const tutorialStepLabel = React.useMemo(
    () =>
      tString("tutorial.stepOf")
        .replace("{{current}}", String(tutorialStepIndex + 1))
        .replace("{{total}}", String(tutorialSteps.length)),
    [tutorialStepIndex, tutorialSteps.length, tString],
  );

  const handleTutorialBack = React.useCallback(() => {
    setTutorialStepIndex((prev) => Math.max(prev - 1, 0));
  }, []);

  const handleTutorialSkip = React.useCallback(() => {
    setTutorialOpen(false);
    setActiveTab("overview");
  }, [setActiveTab]);

  const handleTutorialFinish = React.useCallback(() => {
    setTutorialOpen(false);
    setActiveTab("overview");
  }, [setActiveTab]);

  const handleTutorialNext = React.useCallback(() => {
    if (isLastTutorialStep) {
      handleTutorialFinish();
      return;
    }

    setTutorialStepIndex((prev) =>
      Math.min(prev + 1, tutorialSteps.length - 1),
    );
  }, [handleTutorialFinish, isLastTutorialStep, tutorialSteps.length]);

  const handleTutorialRestart = React.useCallback(() => {
    if (tutorialSteps.length === 0) return;
    setTutorialStepIndex(0);
    setTutorialOpen(true);
  }, [tutorialSteps.length]);

  React.useEffect(() => {
    setTutorialOpen(false);
    setTutorialStepIndex(0);
  }, [business?.id]);

  React.useEffect(() => {
    if (tutorialStepIndex < tutorialSteps.length) return;
    setTutorialStepIndex(0);
  }, [tutorialStepIndex, tutorialSteps.length]);

  React.useEffect(() => {
    if (!tutorialOpen || !currentTutorialTabKey) {
      lastTutorialSyncedTabRef.current = null;
      return;
    }

    // Prevent re-trigger loops with URL synchronization by syncing once per step.
    if (lastTutorialSyncedTabRef.current === currentTutorialTabKey) return;

    lastTutorialSyncedTabRef.current = currentTutorialTabKey;
    if (activeTab !== currentTutorialTabKey) {
      setActiveTab(currentTutorialTabKey);
    }
  }, [activeTab, currentTutorialTabKey, setActiveTab, tutorialOpen]);

  React.useEffect(() => {
    if (!tutorialOpen) return;
    if (sidebarOpen) return;
    setSidebarOpen(true);
  }, [sidebarOpen, setSidebarOpen, tutorialOpen]);

  // Translate a stable error CODE emitted by useBusinessDashboard into an
  // operator-facing message. Branch on the code, never on message substrings —
  // production sanitizeError rewrites the underlying text.
  const isForbiddenError = error === "business:forbidden";

  React.useLayoutEffect(() => {
    setOperatorAccessError(isForbiddenError);
    return () => setOperatorAccessError(false);
  }, [isForbiddenError]);

  const isAuthErrorCode =
    error === "auth:signin-required" || error === "auth:failed";
  const errorCodeMessage = (code: string): string => {
    switch (code) {
      case "auth:signin-required":
        return tString("authentication.title");
      case "auth:failed":
        return tString("errors.authFailed");
      case "business:not-found":
        return tString("error.businessNotFound");
      case "business:forbidden":
        return tString("error.accessDenied");
      case "business:rate-limited":
        return tString("error.rateLimited");
      case "business:generic":
      default:
        return tString("error.loadFailed");
    }
  };

  // Show loading state
  if (authLoading || loading) {
    return <DashboardShellSkeleton />;
  }

  // Show authentication error
  if (error && isAuthErrorCode) {
    return (
      <DashboardStateFrame>
        <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl border border-brand/15 bg-brand/5 shadow-sm shadow-brand/10">
          <LockKeyhole className="h-8 w-8 text-brand" aria-hidden="true" />
        </div>
        <h2 className="mb-4 text-2xl font-semibold text-ink-950">
          {tString("authentication.title")}
        </h2>
        <p className="mb-8 text-sm leading-6 text-ink-600">
          {errorCodeMessage(error)}
        </p>

        {!isConnected ? (
          <div className="space-y-4">
            <p className="text-sm text-ink-500">
              {tString("authentication.connectMessage")}
            </p>
            <div className="flex justify-center">
              <Button
                onClick={() => setShowAuthModal(true)}
                className="rounded-xl bg-brand px-6 py-3 text-white transition-colors hover:bg-brand-dark"
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
              onSuccess={() => {
                setShowAuthModal(false);
                window.location.reload();
              }}
            />
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-sm text-ink-500">
              {tString("authentication.signMessage")}
            </p>
            <button
              onClick={() => window.location.reload()}
              className="rounded-xl border border-transparent bg-brand px-6 py-3 text-sm font-medium text-white transition-colors duration-200 hover:bg-brand-dark"
            >
              {tString("authentication.tryAgain")}
            </button>
          </div>
        )}

        <div className="mt-8">
          <Link
            href="/"
            className="text-sm text-ink-500 hover:text-ink-700 transition-colors duration-200"
          >
            ← {tString("authentication.backToHome")}
          </Link>
        </div>
      </DashboardStateFrame>
    );
  }

  // Show other errors. #773: a refetch flake that still has last-good
  // business data must keep the operator shell (Tables / Menu / Fiscal)
  // instead of replacing it with "Something went wrong / Failed to load
  // business data". Auth and forbidden stay fatal.
  if (
    shouldReplaceDashboardWithFetchError({
      error,
      hasLastGoodBusiness: !!business,
      isAuthError: isAuthErrorCode,
      isForbiddenError,
    })
  ) {
    const namedVenue = (venueName || business?.name || "").trim();
    if (isForbiddenError) {
      return (
        <DashboardStateFrame>
          <div
            data-testid="business-access-error"
            className="flex flex-col items-center"
          >
            <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl border border-rose-200 bg-rose-50">
              <AlertCircle className="h-8 w-8 text-rose-600" />
            </div>
            <h2 className="mb-4 text-xl font-semibold text-ink-950">
              {namedVenue || tString("error.accessDenied")}
            </h2>
            <p className="mb-6 text-sm leading-6 text-ink-600">
              {namedVenue
                ? tString("error.venueAccessDenied").replace(
                    "{name}",
                    namedVenue,
                  )
                : tString("error.accessDenied")}
            </p>
            <Link
              href={VENUES_OVERVIEW_PATH}
              className="rounded-xl bg-brand px-6 py-3 text-sm font-medium text-white transition-colors hover:bg-brand-dark"
            >
              {tString("error.backToBusinesses")}
            </Link>
          </div>
        </DashboardStateFrame>
      );
    }
    return (
      <DashboardStateFrame>
        <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl border border-rose-200 bg-rose-50">
          <AlertCircle className="h-8 w-8 text-rose-600" />
        </div>
        <h2 className="mb-4 text-xl font-semibold text-ink-950">
          {tString("error.title")}
        </h2>
        <p className="mb-6 text-sm leading-6 text-ink-600">
          {errorCodeMessage(error ?? "business:generic")}
        </p>
        <Button
          onPress={() => window.location.reload()}
          className="rounded-xl bg-brand px-6 py-3 text-white transition-colors hover:bg-brand-dark"
        >
          {tString("error.retry")}
        </Button>
      </DashboardStateFrame>
    );
  }

  // Show business not found
  if (!business) {
    return (
      <DashboardStateFrame>
        <p className="text-sm font-medium text-ink-700">
          {tString("error.businessNotFound")}
        </p>
      </DashboardStateFrame>
    );
  }

  return (
    <ConnectivityProvider>
      <CommandPaletteProvider
        business={business}
        activeTab={activeTab}
        setActiveTab={setActiveTab}
        setSidebarOpen={setSidebarOpen}
        allowedTabs={allowedTabs}
        isStaffUser={isStaffUser}
        tutorialOpen={tutorialOpen}
        onStartTutorial={
          tutorialSteps.length > 0 ? handleTutorialRestart : undefined
        }
      >
        <div
          data-testid="dashboard-shell"
          className="relative h-[100dvh] overflow-hidden bg-warm-50 text-ink-950"
        >
          <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_12%_8%,rgba(26,107,106,0.10),transparent_28%),radial-gradient(circle_at_90%_12%,rgba(255,255,255,0.78),transparent_24%),linear-gradient(180deg,#faf9f6_0%,#f3f1ec_100%)]" />
          <div className="pointer-events-none absolute inset-0 opacity-[0.32] [background-image:linear-gradient(rgba(255,255,255,0.32)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.32)_1px,transparent_1px)] [background-size:28px_28px]" />
          {/* Root A: no z-[1] stacking context here — it trapped every
              dashboard overlay (KDS/kiosk/modals) under the root TopMenu
              (fixed z-50). Backgrounds are earlier siblings and already
              paint underneath without a z-index. */}
          <div className="relative flex h-full flex-col">
            <OfflineBanner />
            {queueUserId ? <OfflineQueueDrain userId={queueUserId} /> : null}
            {/* Spacer to push content below TopMenu */}
            <div className="h-14 md:h-16 flex-shrink-0" />

            <div className="flex flex-1 overflow-hidden">
              {/* Sidebar */}
              <DashboardSidebar
                business={business}
                activeTab={activeTab}
                setActiveTab={setActiveTab}
                sidebarOpen={sidebarOpen}
                setSidebarOpen={setSidebarOpen}
                allowedTabs={allowedTabs}
                isStaffUser={isStaffUser}
                staffData={staffData}
                globalOrders={globalOrders}
                globalOrdersLoaded={globalOrdersLoaded}
                occupiedTablesCount={occupiedTablesCount}
                upcomingReservations={upcomingReservations}
                tutorialOpen={tutorialOpen}
                tutorialTabKey={currentTutorialTabKey}
                tutorialTarget={currentTutorialTarget}
                onStartTutorial={
                  tutorialSteps.length > 0 ? handleTutorialRestart : undefined
                }
                onOpenOpsAssistant={
                  aiOff ? undefined : () => setOpsOpenSignal((n) => n + 1)
                }
              />

              {/* Main Content */}
              <div
                className={`relative flex min-w-0 flex-1 flex-col overflow-hidden ${
                  tutorialOpen ? "pointer-events-none" : ""
                }`}
                aria-hidden={mobileDrawerOpen ? true : undefined}
                inert={mobileDrawerOpen || undefined}
              >
                {/* Mobile Header */}
                <div className="relative z-30 border-b border-warm-200/80 bg-white/90 px-3 py-2 shadow-sm shadow-warm-300/20 backdrop-blur-xl sm:px-4 sm:py-3 lg:hidden">
                  <div className="flex items-center justify-between">
                    <Button
                      isIconOnly
                      aria-label={tString("openMenuAria")}
                      variant="light"
                      size="sm"
                      isDisabled={tutorialOpen}
                      onMouseDown={(event) => {
                        event.preventDefault();
                      }}
                      onPress={() => setSidebarOpen(true)}
                    >
                      <Menu className="w-5 h-5" />
                    </Button>
                    {/* h2 not h1 — the main content already provides the page-level
                  h1 ("Welcome back to <business>"). Two h1s on the same route
                  was an a11y red flag even though this one is lg:hidden. */}
                    <h2 className="font-semibold text-ink-900 line-clamp-2">
                      {business.name}
                    </h2>
                    <div className="flex items-center gap-1">
                      {!tutorialOpen && (
                        <RecentAlertsPopover
                          businessId={business.id}
                          showSoundControl
                          onNavigate={setActiveTab}
                        />
                      )}
                      {/* ⌘K launcher — compact icon-only pill on mobile. */}
                      {!tutorialOpen ? (
                        <CommandPaletteTrigger />
                      ) : (
                        <div className="w-8" />
                      )}
                    </div>
                  </div>
                </div>

                {/* Content Area */}
                <div
                  // The shell root is `h-[100dvh] overflow-hidden`, so THIS is
                  // the element that scrolls on every operator tab — the window
                  // never does. It is a <div>, not a <main>: the (shop) layout
                  // owns the page's single <main id="main-content"> landmark.
                  // QA harnesses must measure `[data-dashboard-scroller]`
                  // instead of `window.scrollY` / `document.scrollHeight`,
                  // which are pinned at 0 / viewport height here and read as
                  // "page cannot scroll" on every tab.
                  data-dashboard-scroller="true"
                  ref={mainRef}
                  tabIndex={0}
                  className="min-h-0 min-w-0 flex-1 overflow-auto scroll-smooth"
                >
                  {business && activeTab === "overview" && !aiOff ? (
                    <div className="px-4 pt-4 max-w-screen-2xl mx-auto">
                      <OpsAssistantProactiveNudge
                        businessId={business.id}
                        activeTab={activeTab}
                        onOpenAssistant={() => setOpsOpenSignal((n) => n + 1)}
                      />
                    </div>
                  ) : null}
                  {children}
                </div>
                {/* M4: thumb-reachable TODAY rails on phones (sidebar is
                    behind a hamburger there); desktop keeps the rail. */}
                {business ? (
                  <MobileTodayBar
                    businessId={business.id}
                    activeTab={activeTab}
                    onNavigate={setActiveTab}
                    allowedTabs={allowedTabs}
                    accessState={accessState}
                    isStaffUser={isStaffUser}
                    occupiedTablesCount={occupiedTablesCount}
                    globalOrders={globalOrders}
                    globalOrdersLoaded={globalOrdersLoaded}
                    upcomingReservations={upcomingReservations}
                    hidden={tutorialOpen}
                  />
                ) : null}
                {business && !aiOff ? (
                  <OpsAssistantWidget
                    businessId={business.id}
                    activeTab={activeTab}
                    isStaffUser={isStaffUser}
                    openSignal={opsOpenSignal}
                    dock="content"
                    hideFab
                  />
                ) : null}
              </div>
            </div>

            {tutorialOpen && currentTutorialStep && (
              <>
                <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-[160]" />

                <div className="fixed inset-0 z-[200] pointer-events-none flex items-center justify-center p-3 sm:p-6">
                  <div className="w-full max-w-[460px] pointer-events-auto">
                    <div className="bg-white rounded-2xl border border-warm-200 shadow-2xl overflow-hidden">
                      <div className="px-4 py-3 sm:px-5 sm:py-4 border-b border-warm-100 flex items-start justify-between gap-4">
                        <p className="text-xs font-semibold text-brand pt-0.5">
                          {tutorialStepLabel}
                        </p>
                        <button
                          onClick={handleTutorialSkip}
                          className="text-sm font-medium text-ink-500 hover:text-ink-700 transition-colors"
                        >
                          {tString("tutorial.skip")}
                        </button>
                      </div>

                      <div className="p-5 space-y-4">
                        {currentTutorialStep.type === "support" ? (
                          <div className="rounded-xl border border-warm-200 bg-brand/5 p-4 space-y-3">
                            <div>
                              <h4 className="text-base font-semibold text-ink-900">
                                {currentTutorialStep.title}
                              </h4>
                              <p className="text-sm text-ink-600 mt-1">
                                {currentTutorialStep.description}
                              </p>
                            </div>
                            <div className="space-y-2">
                              <div className="text-sm font-medium text-ink-700">
                                • {tString("footer.requestHelp")}
                              </div>
                              {brandLinks.whatsappUrl ||
                              brandLinks.telegramUrl ? (
                                <div className="text-sm font-medium text-ink-700">
                                  • {tString("footer.urgentSupport")}
                                </div>
                              ) : null}
                              {brandLinks.bookCallUrl ? (
                                <div className="text-sm font-medium text-ink-700">
                                  • {tString("footer.bookCall")}
                                </div>
                              ) : null}
                            </div>
                          </div>
                        ) : (
                          <>
                            {currentTutorialStep.image ? (
                              <div className="relative aspect-[16/10] w-full overflow-hidden rounded-xl border border-warm-200 bg-warm-50">
                                <Image
                                  src={currentTutorialStep.image}
                                  alt={currentTutorialStep.title}
                                  fill
                                  sizes="460px"
                                  className="object-contain"
                                />
                              </div>
                            ) : (
                              <div className="aspect-[16/10] w-full rounded-xl border border-dashed border-warm-300 bg-warm-50 flex items-center justify-center">
                                <span className="text-sm font-medium text-ink-500">
                                  {tString("tutorial.comingSoonImage")}
                                </span>
                              </div>
                            )}

                            <div>
                              <h4 className="text-base font-semibold text-ink-900">
                                {currentTutorialStep.title}
                              </h4>
                              <p className="text-sm text-ink-600 mt-1">
                                {currentTutorialStep.description}
                              </p>
                              <p className="text-sm text-ink-500 mt-2">
                                {tString("tutorial.stepHint")}
                              </p>
                            </div>
                          </>
                        )}

                        <div className="flex items-center justify-between gap-3 pt-1">
                          <Button
                            variant="flat"
                            onPress={handleTutorialBack}
                            isDisabled={tutorialStepIndex === 0}
                          >
                            {tString("tutorial.back")}
                          </Button>
                          <Button
                            className="bg-brand text-white hover:bg-brand-dark"
                            onPress={
                              isLastTutorialStep
                                ? handleTutorialFinish
                                : handleTutorialNext
                            }
                          >
                            {isLastTutorialStep
                              ? tString("tutorial.finish")
                              : tString("tutorial.next")}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </CommandPaletteProvider>
    </ConnectivityProvider>
  );
}
