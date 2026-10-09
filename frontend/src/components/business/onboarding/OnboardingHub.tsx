"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import { brandLinks } from "@/config/brand";
import {
  completeOnboarding,
  getSetupStatus,
  type SetupStatusResponse,
  type SetupStatusSteps,
} from "@/api/onboarding";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  safeActivationToken,
  trackOptionalActivationEvent,
} from "@/lib/analytics/activationEvents";

/** Required + optional hub chips (excludes layout, which is rendered separately). */
type HubStepId = Exclude<keyof SetupStatusSteps, "layout">;

const STEP_ORDER: HubStepId[] = [
  "business_profile",
  "tables",
  "menu",
  "staff",
  "payment",
];

// Chip DISPLAY order stays STEP_ORDER (stable, matches the operator's mental
// model of the card). The Continue-setup TARGET follows the backend's value
// ranking instead — services/setup_status.go FirstMissingRequiredStep puts the
// menu first because it is the single biggest blocker to a first order (the
// setup-nudge email already walks this order; the hub CTA now agrees with it).
const CTA_PRIORITY: HubStepId[] = [
  "menu",
  "tables",
  "business_profile",
  "staff",
  "payment",
];

const TAB_MAP: Record<HubStepId, string> = {
  business_profile: "settings",
  tables: "tables",
  menu: "menu",
  staff: "staff",
  payment: "plugins",
};

const OPTIONAL_STEPS = new Set<HubStepId>(["staff", "payment"]);

const HIDDEN_TTL_MS = 24 * 60 * 60 * 1000;
const CELEBRATION_WINDOW_MS = 24 * 60 * 60 * 1000;
const MAX_AUTOMATIC_RETRIES = 2;
const RETRY_BASE_DELAY_MS = 500;

const hiddenKey = (id: number) => `payverge_hub_hidden_until_${id}`;
const forceShowKey = (id: number) => `payverge_hub_force_show_${id}`;
const statusCacheKey = (id: number) => `payverge_hub_status_${id}`;
// Permanent (no TTL) opt-out of the celebration card. Once set, the card never
// re-enters "celebration" mode for that business.
const dismissedKey = (id: number) => `payverge_hub_celebration_dismissed_${id}`;

type LoadState = "loading" | "ready" | "refreshing" | "error";
type CompletionState = "idle" | "saving" | "error";

function activationDeviceClass() {
  if (typeof window === "undefined") return "unknown" as const;
  if (window.innerWidth < 768) return "mobile" as const;
  if (window.innerWidth < 1024) return "tablet" as const;
  return "desktop" as const;
}

function onboardingActivationDimensions(
  locale: string,
  step: HubStepId | "layout",
  startedAtMs: number,
) {
  return {
    locale: safeActivationToken(locale || "en", "en"),
    device_class: activationDeviceClass(),
    onboarding_step: safeActivationToken(step),
    elapsed_ms: Math.max(0, Date.now() - startedAtMs),
  };
}

function OnboardingStepView({
  locale,
  step,
  startedAtMs,
}: {
  locale: string;
  step: HubStepId;
  startedAtMs: number;
}) {
  useEffect(() => {
    void trackOptionalActivationEvent(
      "onboarding_step_viewed",
      onboardingActivationDimensions(locale, step, startedAtMs),
    ).catch(() => undefined);
  }, [locale, step, startedAtMs]);
  return null;
}

function readCachedStatus(businessId: number): SetupStatusResponse | null {
  if (typeof window === "undefined") return null;
  const raw = window.sessionStorage.getItem(statusCacheKey(businessId));
  if (!raw) return null;
  try {
    return JSON.parse(raw) as SetupStatusResponse;
  } catch {
    window.sessionStorage.removeItem(statusCacheKey(businessId));
    return null;
  }
}

export interface OnboardingHubProps {
  businessId: number;
  businessName: string;
  onboardingCompletedAt: string | null;
  refreshKey: number;
  onNavigateToTab: (tab: string) => void;
  onVisibilityChange?: (isVisible: boolean) => void;
  /** table_code of the business's first table, used for the celebration
   * card's "See what your guests see" link. Omitted/undefined hides the link. */
  firstTableCode?: string | null;
}

function renderIncomplete({
  status,
  t,
  tSpaces,
  onNavigateToTab,
  onHideForToday,
  paymentChooserOpen,
  onTogglePaymentChooser,
  locale,
  startedAtMs,
}: {
  status: SetupStatusResponse;
  t: (key: string, params?: Record<string, string | number>) => string;
  tSpaces: (key: string, params?: Record<string, string | number>) => string;
  onNavigateToTab: (tab: string) => void;
  onHideForToday: () => void;
  paymentChooserOpen: boolean;
  onTogglePaymentChooser: () => void;
  locale: string;
  startedAtMs: number;
}) {
  const firstIncompleteStep = (() => {
    for (const stepId of CTA_PRIORITY) {
      if (!status.steps[stepId].done) return stepId;
    }
    return "business_profile";
  })();
  const firstIncompleteTab = TAB_MAP[firstIncompleteStep];
  const trackStepClick = (step: HubStepId | "layout") => {
    void trackOptionalActivationEvent(
      "onboarding_step_clicked",
      onboardingActivationDimensions(locale, step, startedAtMs),
    ).catch(() => undefined);
  };

  const progressPct =
    status.total_count > 0
      ? Math.round((status.completed_count / status.total_count) * 100)
      : 0;

  const hasSettlementAddress = Boolean(
    status.steps.payment.has_settlement_address,
  );

  return (
    <div className="bg-white border border-warm-200 rounded-2xl p-5 shadow-sm">
      <OnboardingStepView
        locale={locale}
        step={firstIncompleteStep}
        startedAtMs={startedAtMs}
      />
      {/* Compact: the page header already greets the operator, so this card
          is just the checklist — one line, progress bar, steps, CTA. */}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
        <p className="text-sm font-semibold text-ink-900">
          {t("incomplete.welcomeSubtitle")}
        </p>
        <span className="text-xs font-medium text-ink-500">
          {t("incomplete.progress", {
            done: status.completed_count,
            total: status.total_count,
          })}{" "}
          · {progressPct}%
        </span>
      </div>

      <div
        className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-warm-100"
        role="progressbar"
        aria-valuenow={progressPct}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={t("incomplete.progress", {
          done: status.completed_count,
          total: status.total_count,
        })}
      >
        <div
          className="h-full rounded-full bg-brand transition-all duration-300"
          style={{ width: `${progressPct}%` }}
          aria-hidden
        />
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {STEP_ORDER.map((stepId) => {
          const isDone = status.steps[stepId].done;
          const isOptional = OPTIONAL_STEPS.has(stepId);
          const label = t(`steps.${stepId}`);

          if (isDone) {
            return (
              <span
                key={stepId}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-brand/10 text-brand-dark text-sm font-medium"
              >
                ✓ {label}
              </span>
            );
          }

          // The payment step opens an inline chooser (cash today / Stripe /
          // payout wallet) instead of dead-ending on the Plugins tab.
          const onChipClick =
            stepId === "payment"
              ? () => {
                  trackStepClick(stepId);
                  onTogglePaymentChooser();
                }
              : () => {
                  trackStepClick(stepId);
                  onNavigateToTab(TAB_MAP[stepId]);
                };

          return (
            <button
              key={stepId}
              type="button"
              onClick={onChipClick}
              aria-expanded={
                stepId === "payment" ? paymentChooserOpen : undefined
              }
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-warm-200 text-ink-700 text-sm font-medium hover:border-brand hover:text-brand transition-colors"
            >
              ○ {label}
              {isOptional && (
                <span className="text-[10px] text-ink-500 ml-0.5">
                  · {t("incomplete.optional")}
                </span>
              )}
            </button>
          );
        })}
        {/* Optional layout step: only after tables exist. Never required for go-live. */}
        {status.steps.tables.done &&
          (status.steps.layout?.done ? (
            <span
              key="layout"
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-brand/10 text-brand-dark text-sm font-medium"
              title={tSpaces("onboarding.layoutStepHint")}
            >
              ✓ {tSpaces("onboarding.layoutStep")}
            </span>
          ) : (
            <button
              key="layout"
              type="button"
              onClick={() => {
                trackStepClick("layout");
                onNavigateToTab("tables?tablesView=spaces");
              }}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-warm-200 text-ink-700 text-sm font-medium hover:border-brand hover:text-brand transition-colors"
              title={tSpaces("onboarding.layoutStepHint")}
              data-testid="onboarding-layout-step"
            >
              ○ {tSpaces("onboarding.layoutStep")}
              <span className="text-[10px] text-ink-500 ml-0.5">
                · {t("incomplete.optional")}
              </span>
            </button>
          ))}
      </div>

      {paymentChooserOpen && !status.steps.payment.done && (
        <div className="mt-4 rounded-xl border border-warm-200 bg-warm-50/60 p-4">
          <p className="text-sm font-semibold text-ink-900">
            {t("paymentChooser.title")}
          </p>
          <div className="mt-3 grid gap-3 sm:grid-cols-3">
            <div className="rounded-lg border border-warm-200 bg-white p-3">
              <p className="text-sm font-medium text-ink-900">
                {t("paymentChooser.cashTitle")}
              </p>
              <p className="mt-1 text-xs leading-5 text-ink-600">
                {t("paymentChooser.cashBody")}
              </p>
              <button
                type="button"
                onClick={() => {
                  trackStepClick("payment");
                  onNavigateToTab("bills");
                }}
                className="mt-2 text-sm font-medium text-brand hover:text-brand-dark"
              >
                {t("paymentChooser.cashCta")} →
              </button>
            </div>
            <div className="rounded-lg border border-warm-200 bg-white p-3">
              <p className="text-sm font-medium text-ink-900">
                {t("paymentChooser.stripeTitle")}
              </p>
              <p className="mt-1 text-xs leading-5 text-ink-600">
                {t("paymentChooser.stripeBody")}
              </p>
              <button
                type="button"
                onClick={() => {
                  trackStepClick("payment");
                  onNavigateToTab("plugins");
                }}
                className="mt-2 text-sm font-medium text-brand hover:text-brand-dark"
              >
                {t("paymentChooser.stripeCta")} →
              </button>
            </div>
            <div className="rounded-lg border border-warm-200 bg-white p-3">
              <p className="text-sm font-medium text-ink-900">
                {t("paymentChooser.walletTitle")}
              </p>
              <p className="mt-1 text-xs leading-5 text-ink-600">
                {t("paymentChooser.walletBody")}
              </p>
              {hasSettlementAddress ? (
                <p className="mt-2 text-sm font-medium text-brand-dark">
                  ✓ {t("paymentChooser.walletDone")}
                </p>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    trackStepClick("payment");
                    onNavigateToTab("settings");
                  }}
                  className="mt-2 text-sm font-medium text-brand hover:text-brand-dark"
                >
                  {t("paymentChooser.walletCta")} →
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={() => {
            trackStepClick(firstIncompleteStep);
            onNavigateToTab(firstIncompleteTab);
          }}
          className="inline-flex items-center gap-2 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark"
        >
          {t("incomplete.continueSetup")} →
        </button>
        <button
          type="button"
          onClick={onHideForToday}
          className="text-sm text-ink-500 hover:text-ink-700 underline-offset-2 hover:underline"
        >
          {t("incomplete.hideForToday")}
        </button>
      </div>
    </div>
  );
}

function renderCelebration({
  t,
  businessId,
  businessName,
  onNavigateToTab,
  onDismiss,
  firstTableCode,
}: {
  t: (key: string, params?: Record<string, string | number>) => string;
  businessId: number;
  businessName: string;
  onNavigateToTab: (tab: string) => void;
  onDismiss: () => void;
  firstTableCode?: string | null;
}) {
  const handlePrintSheet = () => {
    // One-shot hand-off: TableManager consumes this flag on mount and opens
    // the print-all sheet once tables have loaded (P2-16).
    if (typeof window !== "undefined") {
      window.sessionStorage.setItem(
        `payverge_print_qr_on_open_${businessId}`,
        "1",
      );
    }
    onNavigateToTab("tables");
  };

  return (
    <div className="relative bg-white border border-warm-200 rounded-2xl p-6 sm:p-8 shadow-sm">
      {/* Permanent dismiss: absolutely positioned so the heading keeps the
          card's left edge. The 10px inset + h-10/w-10 gives a ~40px hit area
          without visually crowding the corner. */}
      <button
        type="button"
        onClick={onDismiss}
        aria-label={t("celebration.dismiss")}
        className="absolute right-2.5 top-2.5 inline-flex h-10 w-10 items-center justify-center rounded-full text-lg leading-none text-ink-400 transition-colors hover:text-ink-700"
      >
        ×
      </button>
      <h2 className="text-xl font-semibold text-ink-900 leading-tight">
        {t("celebration.title")}
      </h2>
      <p className="mt-2 text-base text-ink-600 leading-relaxed">
        {t("celebration.subtitle", { businessName })}
      </p>
      <p className="mt-3 text-sm text-ink-500 leading-relaxed">
        {t("celebration.body")}
      </p>
      <p className="mt-3 text-sm text-ink-500 leading-relaxed">
        {t("celebration.testOrderHint")}
      </p>
      <div className="mt-5 flex flex-col items-start gap-3 sm:flex-row sm:items-center">
        <button
          type="button"
          onClick={handlePrintSheet}
          className="inline-flex items-center justify-center gap-2 rounded-full bg-brand px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-brand-dark w-full sm:w-auto"
        >
          {t("celebration.printCta")}
        </button>
        <button
          type="button"
          onClick={() => onNavigateToTab("tables")}
          className="inline-flex items-center justify-center gap-2 rounded-full border border-warm-200 px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:border-brand hover:text-brand w-full sm:w-auto"
        >
          {t("celebration.cta")}
        </button>
        {firstTableCode ? (
          <a
            href={`/t/${firstTableCode}`}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center justify-center gap-1.5 rounded-full px-3 py-2.5 text-sm font-medium text-brand transition-colors hover:text-brand-dark"
          >
            {t("celebration.previewCta")}
          </a>
        ) : null}
      </div>
    </div>
  );
}

function VisibilityReporter({
  isVisible,
  onVisibilityChange,
  children,
}: {
  isVisible: boolean;
  onVisibilityChange?: (visible: boolean) => void;
  children: React.ReactNode;
}) {
  useEffect(() => {
    onVisibilityChange?.(isVisible);
  }, [isVisible, onVisibilityChange]);
  return <>{children}</>;
}

export default function OnboardingHub({
  businessId,
  businessName,
  onboardingCompletedAt,
  refreshKey,
  onNavigateToTab,
  onVisibilityChange,
  firstTableCode,
}: OnboardingHubProps) {
  const { locale } = useSimpleLocale();
  const activationStartedAtRef = useRef(Date.now());
  const [status, setStatus] = useState<SetupStatusResponse | null>(() =>
    readCachedStatus(businessId),
  );
  const statusRef = useRef(status);
  const [loadState, setLoadState] = useState<LoadState>(() =>
    readCachedStatus(businessId) ? "refreshing" : "loading",
  );
  const [loadFailures, setLoadFailures] = useState(0);
  const loadFailuresRef = useRef(0);
  const [retryKey, setRetryKey] = useState(0);
  const [optimisticCompletedAt, setOptimisticCompletedAt] = useState<
    string | null
  >(null);
  const [completionState, setCompletionState] =
    useState<CompletionState>("idle");
  const completionInFlightRef = useRef(false);
  const [, setHiddenTick] = useState(0);
  const [forceShow, setForceShow] = useState(false);
  // Inline payment-path chooser (cash today / Stripe keys / payout wallet).
  const [paymentChooserOpen, setPaymentChooserOpen] = useState(false);

  // Consume the one-shot force_show override (set by the Settings "Reopen
  // setup wizard" action). Read it once on mount and clear it so the override
  // applies to exactly one visit — a later "Hide for today" or navigation is
  // then honored instead of being permanently overridden.
  useEffect(() => {
    if (typeof window === "undefined") return;
    if (window.sessionStorage.getItem(forceShowKey(businessId)) === "1") {
      setForceShow(true);
      window.sessionStorage.removeItem(forceShowKey(businessId));
    }
  }, [businessId]);

  useEffect(() => {
    let cancelled = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    setLoadState(statusRef.current ? "refreshing" : "loading");
    (async () => {
      try {
        const resp = await getSetupStatus(businessId);
        if (!cancelled) {
          statusRef.current = resp;
          setStatus(resp);
          setLoadState("ready");
          setLoadFailures(0);
          loadFailuresRef.current = 0;
          if (typeof window !== "undefined") {
            window.sessionStorage.setItem(
              statusCacheKey(businessId),
              JSON.stringify(resp),
            );
          }
        }
      } catch {
        if (!cancelled) {
          const nextFailure = Math.min(loadFailuresRef.current + 1, 3);
          loadFailuresRef.current = nextFailure;
          setLoadState("error");
          setLoadFailures(nextFailure);
          if (nextFailure <= MAX_AUTOMATIC_RETRIES) {
            const delay = RETRY_BASE_DELAY_MS * Math.pow(2, nextFailure - 1);
            retryTimer = setTimeout(() => {
              if (!cancelled) setRetryKey((key) => key + 1);
            }, delay);
          }
        }
      }
    })();
    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
    };
  }, [businessId, refreshKey, retryKey]);

  const saveCompletion = useCallback(async () => {
    if (completionInFlightRef.current) return;
    completionInFlightRef.current = true;
    setCompletionState("saving");
    try {
      const resp = await completeOnboarding(businessId);
      setOptimisticCompletedAt(resp.completed_at ?? new Date().toISOString());
      setCompletionState("idle");
    } catch (err) {
      console.error("[OnboardingHub] completeOnboarding failed", err);
      setOptimisticCompletedAt(null);
      setCompletionState("error");
    } finally {
      completionInFlightRef.current = false;
    }
  }, [businessId]);

  // The endpoint is idempotent. This effect starts the first attempt; a failed
  // request remains visible and can be retried without overlapping calls.
  useEffect(() => {
    if (!status?.required_done) return;
    if ((optimisticCompletedAt ?? onboardingCompletedAt) !== null) return;
    if (completionState !== "idle") return;
    void saveCompletion();
  }, [
    status,
    onboardingCompletedAt,
    optimisticCompletedAt,
    completionState,
    saveCompletion,
  ]);

  const t = (key: string, params?: Record<string, string | number>): string => {
    const fullKey = `businessDashboard.setupProgress.${key}`;
    const result = params
      ? getTranslation(fullKey, locale, params)
      : getTranslation(fullKey, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const tSpaces = (
    key: string,
    params?: Record<string, string | number>,
  ): string => {
    const fullKey = `spacesTables.${key}`;
    const result = params
      ? getTranslation(fullKey, locale, params)
      : getTranslation(fullKey, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  // Hide the hub for the rest of the day, and drop any active force_show
  // override so the dismissal actually takes effect on the next render.
  const handleHideForToday = () => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(
      hiddenKey(businessId),
      String(Date.now() + HIDDEN_TTL_MS),
    );
    setForceShow(false);
    setHiddenTick((n) => n + 1);
  };

  // Permanently dismiss the celebration for this business. The flag is read in
  // the mode computation below, so the tick re-renders straight to dormant.
  const handleDismissCelebration = () => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(dismissedKey(businessId), "1");
    setHiddenTick((n) => n + 1);
  };

  if (loadState === "loading" && status === null) {
    return (
      <VisibilityReporter isVisible onVisibilityChange={onVisibilityChange}>
        <div
          role="status"
          aria-busy="true"
          aria-label={t("recovery.loading")}
          className="rounded-2xl border border-warm-200 bg-white p-5 shadow-sm"
        >
          <div className="h-4 w-48 animate-pulse rounded bg-warm-200" />
          <div className="mt-4 h-2 w-full animate-pulse rounded bg-warm-100" />
          <span className="sr-only">{t("recovery.loading")}</span>
        </div>
      </VisibilityReporter>
    );
  }

  if (loadState === "error" && status === null) {
    return (
      <VisibilityReporter isVisible onVisibilityChange={onVisibilityChange}>
        <div
          role="alert"
          className="rounded-2xl border border-rose-200 bg-white p-5 shadow-sm"
        >
          <p className="font-semibold text-ink-900">
            {t("recovery.loadErrorTitle")}
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => setRetryKey((key) => key + 1)}
              className="rounded-full bg-brand px-4 py-2 text-sm font-medium text-white hover:bg-brand-dark"
            >
              {t("recovery.retry")}
            </button>
            {loadFailures >= 2 && brandLinks.contactEmail ? (
              <a
                href={`mailto:${brandLinks.contactEmail}`}
                className="text-sm font-medium text-brand hover:text-brand-dark"
              >
                {t("recovery.support")}
              </a>
            ) : null}
          </div>
        </div>
      </VisibilityReporter>
    );
  }

  if (status === null) return null;

  // "Live" = the required steps are done (profile + tables + menu). Optional
  // steps (staff, payment) do not gate the celebration, matching their label.
  const effectiveCompletedAt = optimisticCompletedAt ?? onboardingCompletedAt;
  const inCelebrationWindow =
    status.required_done &&
    effectiveCompletedAt !== null &&
    Date.now() - new Date(effectiveCompletedAt).getTime() <
      CELEBRATION_WINDOW_MS;
  let mode: "incomplete" | "celebration" | "dormant" = "dormant";
  if (!status.required_done) {
    const hiddenUntilRaw =
      typeof window !== "undefined"
        ? window.localStorage.getItem(hiddenKey(businessId))
        : null;
    const parsedHiddenUntil = hiddenUntilRaw ? Number(hiddenUntilRaw) : 0;
    // A corrupt (non-numeric) value parses to NaN, and `NaN <= now` is false,
    // which would otherwise pin the hub dormant forever. Treat any non-finite
    // value as "not hidden".
    const hiddenUntil = Number.isFinite(parsedHiddenUntil)
      ? parsedHiddenUntil
      : 0;
    if (forceShow || hiddenUntil <= Date.now()) mode = "incomplete";
  } else if (inCelebrationWindow) {
    // A permanent dismissal wins over any celebration window — fall through to
    // dormant rather than re-showing the card.
    const dismissed =
      typeof window !== "undefined" &&
      window.localStorage.getItem(dismissedKey(businessId)) === "1";
    if (!dismissed) mode = "celebration";
  }

  return (
    <VisibilityReporter
      isVisible={
        loadState === "error" ||
        completionState === "saving" ||
        completionState === "error" ||
        mode !== "dormant"
      }
      onVisibilityChange={onVisibilityChange}
    >
      {loadState === "error" ? (
        <div
          role="alert"
          className="mb-3 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-ink-700"
        >
          {t("recovery.refreshError")}{" "}
          <button
            type="button"
            onClick={() => setRetryKey((key) => key + 1)}
            className="font-medium text-brand underline-offset-2 hover:underline"
          >
            {t("recovery.retry")}
          </button>
        </div>
      ) : null}
      {completionState === "saving" && effectiveCompletedAt === null ? (
        <div
          role="status"
          aria-busy="true"
          className="rounded-2xl border border-warm-200 bg-white p-5 shadow-sm"
        >
          {t("recovery.savingCompletion")}
        </div>
      ) : completionState === "error" ? (
        <div
          role="alert"
          className="rounded-2xl border border-amber-200 bg-white p-5 shadow-sm"
        >
          <p className="font-semibold text-ink-900">
            {t("recovery.completionError")}
          </p>
          <button
            type="button"
            onClick={() => void saveCompletion()}
            className="mt-3 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white hover:bg-brand-dark"
          >
            {t("recovery.retryCompletion")}
          </button>
        </div>
      ) : mode === "incomplete" ? (
        renderIncomplete({
          status,
          t,
          tSpaces,
          onNavigateToTab,
          onHideForToday: handleHideForToday,
          paymentChooserOpen,
          onTogglePaymentChooser: () => setPaymentChooserOpen((v) => !v),
          locale,
          startedAtMs: activationStartedAtRef.current,
        })
      ) : mode === "celebration" ? (
        renderCelebration({
          t,
          businessId,
          businessName,
          onNavigateToTab,
          onDismiss: handleDismissCelebration,
          firstTableCode,
        })
      ) : null}
    </VisibilityReporter>
  );
}
