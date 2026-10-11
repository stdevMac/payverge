"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams, useSearchParams } from "next/navigation";
import { Button, Card, CardBody, Spinner } from "@nextui-org/react";
import {
  AlertTriangle,
  Bike,
  CheckCircle2,
  ChefHat,
  Clock,
  ExternalLink,
  PackageCheck,
  Phone,
  RefreshCcw,
  ShoppingBag,
  Store,
  UserRound,
  XCircle,
} from "lucide-react";

import { guestDeliveryApi, type PublicDeliveryTrackingDto } from "@/api/delivery";
import { getRouteParam } from "@/utils/nextRouteParams";
import {
  GuestTranslationProvider,
  useGuestTranslation,
} from "@/i18n/GuestTranslationProvider";
import { resolveDeliveryInitialLanguage } from "@/lib/deliveryLocale";
import {
  formatBusinessDateTime,
  formatBusinessTime,
} from "@/utils/businessTime";
import { PaymentBlock } from "./_PaymentBlock";
import { isSafeExternalTrackingUrl } from "@/lib/externalUrl";
import { deliveryStatusAnnouncement } from "./deliveryStatusAnnouncement";

// ── Guest stage model ─────────────────────────────────────────────────────────

type GuestStage = "received" | "accepted" | "preparing" | "out_for_delivery" | "delivered";

const STAGE_BY_STATUS: Record<string, GuestStage> = {
  pending: "received",
  confirmed: "accepted",
  preparing: "preparing",
  ready: "preparing",
  assigned: "out_for_delivery",
  picked_up: "out_for_delivery",
  in_transit: "out_for_delivery",
  nearby: "out_for_delivery",
  delivered: "delivered",
};

const STAGES: GuestStage[] = [
  "received",
  "accepted",
  "preparing",
  "out_for_delivery",
  "delivered",
];

const liveStatuses = new Set<string>([
  "pending",
  "confirmed",
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
]);

// ── Helpers ───────────────────────────────────────────────────────────────────

// Guest-selected language for month/hour names; venue timezone for the
// wall clock so ETA does not flip with the guest device TZ (FIND-023 family).
const formatTimestamp = (
  value?: string,
  locale?: string,
  timeZone?: string | null,
): string => {
  if (!value) return "";
  // Guest track keeps dateStyle:"medium" (not DATE_TIME_SHORT field-by-field).
  // For ja: medium → "2026/07/06"; year+month:short+day → "2026年7月6日".
  // The pre-L8-2 tracker used medium; preserve that slash form for CJK guests.
  return formatBusinessDateTime(value, locale || "en", timeZone ?? null, {
    dateStyle: "medium",
    timeStyle: "short",
  });
};

const formatTime = (
  value?: string,
  locale?: string,
  timeZone?: string | null,
): string => {
  if (!value) return "";
  return formatBusinessTime(value, locale || "en", timeZone ?? null, {
    hour: "numeric",
    minute: "2-digit",
  });
};

/**
 * Machine-written cancellation reasons the backend stores as English
 * constants (e.g. the payment-expiry sweeper). These must be shown as
 * localized copy, never verbatim, on the 21-locale guest surface. Operator
 * free-text reasons fall through and render behind a localized label.
 */
const MACHINE_CANCELLATION_REASON_KEYS: Record<string, string> = {
  "payment window expired": "deliveryTracking.cancelledReasonPaymentExpired",
};

const getBusinessHref = (customUrl?: string) => (customUrl ? `/b/${customUrl}` : "/");

// ── Stage icons ───────────────────────────────────────────────────────────────

function StageIcon({ stage, size = 20 }: { stage: GuestStage; size?: number }) {
  switch (stage) {
    case "received":
      return <PackageCheck size={size} />;
    case "accepted":
      return <ShoppingBag size={size} />;
    case "preparing":
      return <ChefHat size={size} />;
    case "out_for_delivery":
      return <Bike size={size} />;
    case "delivered":
      return <CheckCircle2 size={size} />;
  }
}

// ── 5-Stage Ladder ────────────────────────────────────────────────────────────

interface StageLadderProps {
  activeStage: GuestStage;
  tString: (key: string, vars?: Record<string, string | number>) => string;
}

function StageLadder({ activeStage, tString }: StageLadderProps) {
  const activeIdx = STAGES.indexOf(activeStage);

  return (
    <div className="py-2">
      <div
        className="flex flex-col gap-0"
        role="list"
        aria-label={tString("deliveryTracking.progressAria")}
      >
        {STAGES.map((stage, idx) => {
          const isCompleted = idx < activeIdx;
          const isCurrent = idx === activeIdx;
          const isUpcoming = idx > activeIdx;
          const isLast = idx === STAGES.length - 1;

          return (
            <div
              key={stage}
              className="flex items-stretch"
              role="listitem"
              aria-current={isCurrent ? "step" : undefined}
            >
              {/* Left: icon + connector */}
              <div className="flex flex-col items-center w-10 shrink-0">
                <div
                  className={`
                    flex h-9 w-9 items-center justify-center rounded-full border-2 transition-all
                    ${isCompleted ? "border-emerald-500 bg-emerald-500 text-white" : ""}
                    ${isCurrent ? "border-amber-400 bg-amber-400 text-ink-900 shadow-[0_0_12px_rgba(251,191,36,0.5)]" : ""}
                    ${isUpcoming ? "border-ink-200 bg-ink-50 text-ink-500" : ""}
                  `}
                  aria-hidden="true"
                >
                  <StageIcon stage={stage} size={16} />
                </div>
                {!isLast && (
                  <div
                    className={`w-0.5 flex-1 min-h-[1.25rem] ${isCompleted ? "bg-emerald-500" : "bg-ink-200"}`}
                    aria-hidden="true"
                  />
                )}
              </div>

              {/* Right: label */}
              <div className={`ms-3 pb-4 flex items-center ${isLast ? "pb-0" : ""}`}>
                <span
                  className={`text-sm font-medium ${
                    isCurrent
                      ? "text-amber-800"
                      : isCompleted
                      ? "text-emerald-800"
                      : "text-ink-500"
                  }`}
                >
                  {tString(`deliveryTracking.stages.${stage}`)}
                  {isCurrent && (
                    <span className="ms-2 inline-flex h-1.5 w-1.5 animate-pulse rounded-full bg-amber-400 align-middle" />
                  )}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ── Terminal State Cards ──────────────────────────────────────────────────────

interface TerminalCardProps {
  status: string;
  cancellationReason?: string;
  tString: (key: string, vars?: Record<string, string | number>) => string;
}

function TerminalCard({ status, cancellationReason, tString }: TerminalCardProps) {
  const isFailed = status === "failed";
  const isCancelled = status === "cancelled";

  return (
    <div
      className="rounded-2xl border border-rose-200 bg-rose-50 p-6 text-center"
      data-testid="terminal-card"
    >
      <div className="flex justify-center mb-3">
        {isFailed ? (
          <AlertTriangle className="h-10 w-10 text-rose-500" />
        ) : (
          <XCircle className="h-10 w-10 text-rose-500" />
        )}
      </div>
      <p className="text-lg font-semibold text-rose-700">
        {isFailed
          ? tString("deliveryTracking.failedTitle")
          : tString("deliveryTracking.cancelledTitle")}
      </p>
      <p className="mt-1 text-sm text-ink-500">
        {isFailed
          ? tString("deliveryTracking.failedMessage")
          : tString("deliveryTracking.cancelledMessage")}
      </p>
      {isCancelled && cancellationReason ? (
        <p className="mt-2 text-sm text-rose-600 font-medium">
          {(() => {
            const machineKey =
              MACHINE_CANCELLATION_REASON_KEYS[
                cancellationReason.trim().toLowerCase()
              ];
            return machineKey
              ? tString(machineKey)
              : tString("deliveryTracking.cancelledReasonFromRestaurant", {
                  reason: cancellationReason,
                });
          })()}
        </p>
      ) : null}
    </div>
  );
}

// Delivered terminal — green variant (full ladder + this card)
interface DeliveredCardProps {
  tString: (key: string, vars?: Record<string, string | number>) => string;
}

function DeliveredCard({ tString }: DeliveredCardProps) {
  return (
    <div
      className="rounded-2xl border border-emerald-200 bg-emerald-50 p-6 text-center"
      data-testid="delivered-card"
    >
      <div className="flex justify-center mb-3">
        <CheckCircle2 className="h-10 w-10 text-emerald-500" />
      </div>
      <p className="text-lg font-semibold text-emerald-700">
        {tString("deliveryTracking.deliveredTitle")}
      </p>
      <p className="mt-1 text-sm text-ink-500">{tString("deliveryTracking.deliveredMessage")}</p>
    </div>
  );
}

// ── Payment Block ─────────────────────────────────────────────────────────────

// ── Driver Card ───────────────────────────────────────────────────────────────

interface DriverCardProps {
  driver: NonNullable<PublicDeliveryTrackingDto["driver"]>;
  tString: (key: string, vars?: Record<string, string | number>) => string;
}

function DriverCard({ driver, tString }: DriverCardProps) {
  return (
    <div className="rounded-2xl border border-ink-200 bg-white p-5">
      <div className="flex items-start gap-4">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-amber-400/10 text-amber-500">
          <UserRound className="h-5 w-5" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.15em] text-ink-500 mb-1">
            {tString("deliveryTracking.driver")}
          </p>
          <p className="font-semibold text-ink-800 truncate">{driver.name}</p>
          {/* The public tracking payload carries only name + phone (see
              PublicDeliveryTrackingDto) — vehicle/rating UI was dead code. */}
          <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
            {driver.phone && (
              <span className="flex items-center gap-1.5">
                <Phone className="h-3 w-3" />
                {driver.phone}
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// ── ETA Display ───────────────────────────────────────────────────────────────

interface ETADisplayProps {
  eta?: string;
  tString: (key: string, vars?: Record<string, string | number>) => string;
  guestLocale?: string;
  timeZone?: string | null;
}

function ETADisplay({ eta, tString, guestLocale, timeZone }: ETADisplayProps) {
  const label = useMemo(() => {
    if (!eta) return tString("deliveryTracking.etaNotAvailable");
    const target = new Date(eta);
    if (Number.isNaN(target.getTime())) return tString("deliveryTracking.etaNotAvailable");
    const diffMs = target.getTime() - Date.now();
    const diffMin = Math.round(diffMs / 60000);
    if (diffMin > 0 && diffMin <= 60) {
      return tString("deliveryTracking.minutesAway", { minutes: String(diffMin) });
    }
    return tString("deliveryTracking.etaArrivingBy", {
      time: formatTime(eta, guestLocale, timeZone),
    });
  }, [eta, tString, guestLocale, timeZone]);

  return (
    <div className="flex items-start gap-3 rounded-2xl border border-ink-200 bg-white p-4">
      <Clock className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
      <div>
        <p className="text-xs text-ink-500">{tString("deliveryTracking.estimatedArrival")}</p>
        <p className="font-medium text-ink-800">{label}</p>
      </div>
    </div>
  );
}

// ── Inner Page ────────────────────────────────────────────────────────────────

const POLLING_INTERVAL_MS = 15_000;

function DeliveryTrackingInner() {
  const params = useParams();
  const deliveryNumber = getRouteParam(params, "deliveryNumber");
  const { t, currentLanguage } = useGuestTranslation();

  const tString = useCallback(
    (key: string, vars?: Record<string, string | number>): string => t(key, vars),
    [t],
  );

  const [tracking, setTracking] = useState<PublicDeliveryTrackingDto | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [lastSyncedAt, setLastSyncedAt] = useState("");
  const [statusAnnouncement, setStatusAnnouncement] = useState("");
  const prevStatusRef = useRef<string | null>(null);

  // Translation providers and test doubles are not required to preserve the
  // function identity of `t`. Read the latest translator through a ref so a
  // locale change is reflected without turning the network callback into an
  // effect dependency loop.
  const tStringRef = useRef(tString);
  tStringRef.current = tString;

  const status = tracking?.status ?? "";

  const isLiveDelivery = useMemo(() => liveStatuses.has(status), [status]);

  // Cancelled and failed never show the ladder
  const isNegativeTerminal = status === "cancelled" || status === "failed";

  // Delivered shows the ladder + a delivered card
  const isDelivered = status === "delivered";

  // Active guest stage (for the 5-stage ladder)
  const activeStage: GuestStage = STAGE_BY_STATUS[status] ?? "received";

  const hasDriver = useMemo(
    () => !!tracking?.driver?.name,
    [tracking?.driver],
  );

  // Show driver card only while out for delivery
  const showDriverCard = hasDriver && activeStage === "out_for_delivery" && tracking?.driver;

  const loadTracking = useCallback(
    async (background = false) => {
      if (!deliveryNumber) {
        setTracking(null);
        setError(tStringRef.current("deliveryTracking.incompleteLink"));
        setLoading(false);
        return;
      }

      if (background) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }

      try {
        const response = await guestDeliveryApi.track(deliveryNumber);
        setTracking(response);
        setError("");
        setLastSyncedAt(new Date().toISOString());
      } catch (requestError: unknown) {
        // Never surface raw backend English on the 21-locale guest surface —
        // map by HTTP status to localized copy instead of error.message.
        const status =
          typeof requestError === "object" && requestError !== null
            ? (requestError as { status?: unknown }).status
            : undefined;
        setError(
          tStringRef.current(
            status === 404
              ? "deliveryTracking.notFound"
              : "deliveryTracking.loadError",
          ),
        );
        if (!background) {
          setTracking(null);
        }
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [deliveryNumber],
  );

  useEffect(() => {
    void loadTracking();
  }, [loadTracking]);

  useEffect(() => {
    const nextStatus = tracking?.status ?? "";
    const prevStatus = prevStatusRef.current;
    if (nextStatus && prevStatus && prevStatus !== nextStatus) {
      setStatusAnnouncement(deliveryStatusAnnouncement(nextStatus, tString));
    }
    prevStatusRef.current = nextStatus || null;
  }, [tracking?.status, tString]);

  // Auto-refresh: poll every 15 s while delivery is live, stop on terminal.
  // Polling pauses while the tab is hidden (public tracking is a backend hot
  // path — see the Performance Gate) and refreshes immediately on return to
  // visibility, mirroring the operator DispatchConsole pattern.
  useEffect(() => {
    if (!deliveryNumber || !isLiveDelivery) return;

    let interval: number | null = null;
    const startPolling = () => {
      if (interval == null) {
        interval = window.setInterval(() => {
          void loadTracking(true);
        }, POLLING_INTERVAL_MS);
      }
    };
    const stopPolling = () => {
      if (interval != null) {
        window.clearInterval(interval);
        interval = null;
      }
    };
    const handleVisibility = () => {
      if (document.hidden) {
        stopPolling();
      } else {
        void loadTracking(true);
        startPolling();
      }
    };

    if (!document.hidden) {
      startPolling();
    }
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      stopPolling();
    };
  }, [deliveryNumber, isLiveDelivery, loadTracking]);

  return (
    <main className="min-h-screen bg-warm-50 px-4 py-12 text-ink-900 sm:px-6 sm:py-16">
      <div className="mx-auto flex max-w-lg flex-col items-center justify-center">
        <Card className="w-full border border-ink-200 bg-white shadow-2xl">
          <CardBody className="gap-6 p-6 sm:p-8">
            {/* Header */}
            <div className="space-y-2 text-center">
              <p className="text-xs font-medium uppercase tracking-[0.3em] text-ink-500">
                {tracking?.business_name ?? "Payverge Delivery"}
              </p>
              <h1 className="text-2xl font-semibold sm:text-3xl">
                {tString("deliveryTracking.pageTitle")}
              </h1>
              <p className="text-sm text-ink-500">
                {tracking
                  ? tString("deliveryTracking.orderNumber", { number: tracking.delivery_number })
                  : tString("deliveryTracking.checkingStatus")}
              </p>
            </div>

            {/* Loading */}
            {loading ? (
              <div className="flex justify-center py-8" data-testid="loading-spinner">
                <Spinner color="warning" />
              </div>
            ) : null}

            {/* Error */}
            {error ? (
              <div
                className="rounded-2xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700"
                data-testid="error-message"
                role="alert"
              >
                {error}
              </div>
            ) : null}

            <span
              className="sr-only"
              role="status"
              aria-live="polite"
              data-testid="delivery-status-live"
            >
              {statusAnnouncement}
            </span>

            {/* Tracking content */}
            {tracking && !loading ? (
              <>
                {/* Payment block — between header and ladder */}
                {tracking && (
                  <PaymentBlock
                    tracking={tracking}
                    deliveryNumber={deliveryNumber ?? ""}
                    onExpired={() => void loadTracking(true)}
                    tString={tString}
                    guestLocale={currentLanguage}
                  />
                )}

                {/* Negative terminal cards (cancelled / failed) — no ladder */}
                {isNegativeTerminal ? (
                  <TerminalCard
                    status={tracking.status}
                    cancellationReason={tracking.cancellation_reason}
                    tString={tString}
                  />
                ) : (
                  <>
                    {/* 5-stage ladder */}
                    <div
                      className="rounded-2xl border border-ink-200 bg-ink-50 p-5"
                      data-testid="status-timeline"
                    >
                      <StageLadder activeStage={activeStage} tString={tString} />
                    </div>

                    {/* Delivered confirmation card */}
                    {isDelivered ? <DeliveredCard tString={tString} /> : null}

                    {/* Driver card — only while out for delivery */}
                    {showDriverCard && tracking.driver ? (
                      <DriverCard driver={tracking.driver} tString={tString} />
                    ) : null}

                    {/* ETA — only while live. No live driver map or raw
                        coordinates: there is no real driver-GPS producer, so
                        the map/coords would imply a real-time feed that does
                        not exist. Keep the honest surfaces only. */}
                    {isLiveDelivery ? (
                      <ETADisplay
                        eta={tracking.estimated_delivery_time}
                        tString={tString}
                        guestLocale={currentLanguage}
                        timeZone={tracking.timezone}
                      />
                    ) : null}
                  </>
                )}

                {/* Auto-refresh notice + actions */}
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <span className="text-xs text-ink-500">
                    {isLiveDelivery
                      ? tString("deliveryTracking.autoRefreshActive")
                      : tracking.updated_at
                      ? tString("deliveryTracking.statusUpdated", {
                          time: formatTimestamp(
                            tracking.updated_at,
                            currentLanguage,
                            tracking.timezone,
                          ),
                        })
                      : lastSyncedAt
                      ? tString("deliveryTracking.lastSynced", {
                          time: formatTimestamp(
                            lastSyncedAt,
                            currentLanguage,
                            tracking.timezone,
                          ),
                        })
                      : tString("deliveryTracking.lastSyncedJustNow")}
                  </span>

                  <div className="flex shrink-0 flex-wrap gap-2">
                    {!isNegativeTerminal && !isDelivered ? (
                      <Button
                        size="sm"
                        variant="flat"
                        startContent={<RefreshCcw className="h-3.5 w-3.5" />}
                        isLoading={refreshing}
                        onPress={() => void loadTracking(true)}
                      >
                        {tString("deliveryTracking.refreshStatus")}
                      </Button>
                    ) : null}
                    {tracking.external_partner_links &&
                    tracking.external_partner_links.length > 0 ? (
                      <div
                        className="w-full text-sm text-ink-600"
                        data-testid="track-couriers"
                      >
                        <p>
                          {tString("deliveryTracking.couriersNamed", {
                            names: tracking.external_partner_links
                              .map((link) => link.name)
                              .filter(Boolean)
                              .join(", "),
                          })}
                        </p>
                        <ul className="mt-1 flex flex-wrap gap-2">
                          {tracking.external_partner_links.map((link) => (
                            <li key={`${link.name}-${link.url}`}>
                              {link.url && isSafeExternalTrackingUrl(link.url) ? (
                                <a
                                  href={link.url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="font-medium text-brand underline-offset-2 hover:underline"
                                >
                                  {link.name}
                                </a>
                              ) : (
                                <span className="font-medium text-ink-800">
                                  {link.name}
                                </span>
                              )}
                            </li>
                          ))}
                        </ul>
                      </div>
                    ) : null}
                    {tracking.external_tracking_url &&
                    isSafeExternalTrackingUrl(tracking.external_tracking_url) ? (
                      <Button
                        as="a"
                        size="sm"
                        href={tracking.external_tracking_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        color="warning"
                        variant="flat"
                        endContent={<ExternalLink className="h-3.5 w-3.5" />}
                      >
                        {tString("deliveryTracking.trackExternal")}
                      </Button>
                    ) : null}
                  </div>
                </div>

                {/* Back link */}
                <div className="flex justify-center pt-1">
                  {tracking.business_custom_url ? (
                    <Button
                      as={Link}
                      href={getBusinessHref(tracking.business_custom_url)}
                      variant="bordered"
                      size="sm"
                      startContent={<Store className="h-3.5 w-3.5" />}
                    >
                      {tString("deliveryTracking.backToRestaurant")}
                    </Button>
                  ) : (
                    <Button as={Link} href="/" variant="bordered" size="sm">
                      {tString("deliveryTracking.returnHome")}
                    </Button>
                  )}
                </div>
              </>
            ) : null}
          </CardBody>
        </Card>
      </div>
    </main>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export default function DeliveryTrackingPage() {
  const searchParams = useSearchParams();
  const initialLanguage = resolveDeliveryInitialLanguage(
    searchParams?.get("lang"),
  );
  return (
    <GuestTranslationProvider
      initialLanguage={initialLanguage}
      preferInitialLanguage
    >
      <DeliveryTrackingInner />
    </GuestTranslationProvider>
  );
}
