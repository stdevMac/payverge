"use client";

import React, { useState, useEffect, useRef, useCallback } from "react";
import { Button, Chip } from "@nextui-org/react";
import {
  Maximize,
  Minimize,
  X,
  Clock,
  CheckCircle,
  AlertCircle,
  ChefHat,
  Utensils,
  Bell,
} from "lucide-react";
import { useDialogKeyboard } from "@/components/business/schedule/useDialogKeyboard";
import { Order, updateOrderStatus } from "../../api/orders";
import { toFulfillmentLines } from "@/utils/orderFulfillment";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import { Locale } from "@/i18n/config";
import { logger } from "@/utils/logger";
import toast from "react-hot-toast";
import { sanitizeErrorKey } from "@/utils/errorMessages";
import { localizedErrorMessage } from "@/utils/localizedError";
import { sortOrdersFifo } from "@/utils/orderSorting";
import { getBillLocationLabel } from "@/lib/tableLabel";
import EnableAlertSoundButton from "./operational-alerts/EnableAlertSoundButton";
import OperationalAlertClaimStatus from "./operational-alerts/OperationalAlertClaimStatus";
import { useOptionalOperationalAlerts } from "./operational-alerts/useOperationalAlerts";
import { formatKdsFireTime, kdsElapsedMinutes } from "@/utils/kdsTicketTime";
import { KITCHEN_TICKET_IDENTITY_SEP } from "./kitchenTicketIdentity";
import {
  humanizeDurationMinutes,
  elapsedUrgency,
} from "@/utils/humanizeDuration";
import { KitchenAllergenChips } from "./KitchenAllergenChips";
import {
  kitchenNoteLabels,
  localizeKitchenOrderNote,
} from "@/utils/kitchenTicketCopy";

function sameOrderItems(a: Order["items"], b: Order["items"]): boolean {
  if (a === b) return true;
  if (typeof a === "string" && typeof b === "string") return a === b;
  return JSON.stringify(a) === JSON.stringify(b);
}

// ── K-2: module-scope, memoized order card ───────────────────────────────────
// Declared OUTSIDE the component so the 1s clock tick re-renders (not
// remounts) cards, and the memo comparator skips even the re-render unless
// something card-relevant changed. Elapsed time is minute-granularity by
// design: cards only repaint when their displayed age actually changes.

const renderElapsed = (
  elapsedMinutes: number,
  t: (key: string) => string,
): React.ReactNode => {
  if (elapsedMinutes < 1)
    return (
      <span className="text-emerald-700 font-medium">{t("time.justNow")}</span>
    );

  // stale (≥24h) neutralizes rose/pulse so multi-day zombies are not urgent-red.
  const tone = elapsedUrgency(elapsedMinutes);
  let colorClass = "text-ink-600";
  if (tone === "warn") colorClass = "text-amber-700 font-bold";
  if (tone === "critical")
    colorClass = "text-rose-700 font-bold motion-safe:animate-pulse";
  if (tone === "stale") colorClass = "text-ink-500";

  const duration = humanizeDurationMinutes(elapsedMinutes);
  return (
    <span className={colorClass}>
      {t("time.elapsedAgo").replace("{{duration}}", duration)}
    </span>
  );
};

interface KDSOrderCardProps {
  order: Order;
  elapsedMinutes: number;
  fireTime: string;
  isLoading: boolean;
  onStatusUpdate: (orderId: number, status: string) => void;
  t: (key: string) => string;
  locale: Locale;
  allergensByItemId: Record<string, string[]>;
}

const OrderCard = React.memo<KDSOrderCardProps>(
  function OrderCard({
    order,
    elapsedMinutes,
    fireTime,
    isLoading,
    onStatusUpdate,
    t,
    locale,
    allergensByItemId,
  }) {
    const items = toFulfillmentLines(order.items);
    const noteText = order.notes
      ? localizeKitchenOrderNote(order.notes, kitchenNoteLabels(locale))
      : "";
    // Location-aware label: a named table wins, then counter, then delivery.
    // Replaces the old raw "T-{table_id}" literal, which leaked the internal
    // DB id and rendered nothing for counter/delivery bills.
    const locationLabel = order.bill
      ? getBillLocationLabel(order.bill, {
          table: t("location.table"),
          counter: t("location.counter"),
          delivery: t("location.delivery"),
        })
      : null;

    return (
      <article className="mb-3 w-full overflow-hidden rounded-2xl border border-warm-200/80 bg-white/95 shadow-[0_16px_38px_rgba(46,42,37,0.10)]">
        <div className="h-1 bg-[linear-gradient(90deg,rgba(26,107,106,0.82),rgba(26,107,106,0.14))]" />
        <header className="flex items-start justify-between gap-3 px-3.5 pb-2 pt-3">
          <div>
            <div
              className="flex flex-wrap items-center"
              data-testid="kds-ticket-identity"
            >
              <h3 className="text-lg font-bold tabular-nums text-ink-950">
                #{order.order_number}
              </h3>
              <span aria-hidden="true">{KITCHEN_TICKET_IDENTITY_SEP}</span>
              <span className="rounded-full bg-warm-100 px-2 py-0.5 text-xs font-medium text-ink-600">
                {t("billPrefix").replace("{billId}", String(order.bill_id))}
              </span>
            </div>
            <OperationalAlertClaimStatus
              resourceType="order"
              resourceId={order.id}
              className="mt-1"
            />
            <div className="mt-1 flex items-center gap-1 text-xs">
              <Clock className="h-3 w-3 text-ink-400" aria-hidden="true" />
              <span data-testid="kds-ticket-fire-time">{fireTime}</span>
              <span aria-hidden="true">·</span>
              <span data-testid="kds-ticket-elapsed">
                {renderElapsed(elapsedMinutes, t)}
              </span>
            </div>
          </div>
          {locationLabel && (
            <Chip size="sm" variant="flat" color="primary">
              {locationLabel}
            </Chip>
          )}
        </header>
        <div className="px-3.5 pb-3 pt-0">
          {noteText && (
            <div
              className="mb-2 flex gap-2 rounded-xl border border-amber-200 bg-amber-50/90 p-2 text-xs text-amber-900"
              data-testid="kds-ticket-note"
            >
              <AlertCircle
                className="mt-0.5 h-3 w-3 flex-shrink-0"
                aria-hidden="true"
              />
              <span className="font-medium">{noteText}</span>
            </div>
          )}

          <div className="my-2 divide-y divide-warm-100">
            {items.map((item, idx) => {
              const allergens =
                (item.menu_item_id && allergensByItemId[item.menu_item_id]) ||
                [];
              return (
                <div key={idx} className="flex gap-2 py-2 text-sm">
                  <span className="min-w-[24px] font-bold tabular-nums text-ink-950">
                    {item.quantity}x
                  </span>
                  <div className="flex-1">
                    <span className="font-medium text-ink-900">
                      {item.menu_item_name}
                    </span>
                    {item.options && item.options.length > 0 && (
                      <div className="pl-1 text-xs text-ink-600">
                        {item.options.map((opt, i) => (
                          <span key={i} className="block">
                            + {opt.name}
                          </span>
                        ))}
                      </div>
                    )}
                    {item.special_requests && (
                      <span className="block text-xs italic text-amber-700">
                        &quot;{item.special_requests}&quot;
                      </span>
                    )}
                    <KitchenAllergenChips
                      allergens={allergens}
                      locale={locale}
                      className="mt-1 flex flex-wrap gap-1 pl-1"
                    />
                  </div>
                </div>
              );
            })}
          </div>

          <div className="mt-3">
            {order.status === "approved" && (
              <Button
                fullWidth
                color="primary"
                size="lg"
                onPress={() => onStatusUpdate(order.id, "in_kitchen")}
                isLoading={isLoading}
                startContent={!isLoading && <ChefHat className="w-5 h-5" />}
                className="rounded-xl bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
              >
                {t("actions.start")}
              </Button>
            )}
            {order.status === "in_kitchen" && (
              <Button
                fullWidth
                color="success"
                size="lg"
                variant="shadow"
                onPress={() => onStatusUpdate(order.id, "ready")}
                isLoading={isLoading}
                startContent={!isLoading && <CheckCircle className="w-5 h-5" />}
                className="rounded-xl font-semibold text-white"
              >
                {t("actions.ready")}
              </Button>
            )}
            {order.status === "ready" && (
              <Button
                fullWidth
                color="secondary"
                size="lg"
                variant="flat"
                onPress={() => onStatusUpdate(order.id, "delivered")}
                isLoading={isLoading}
                startContent={!isLoading && <Utensils className="w-5 h-5" />}
                className="rounded-xl font-semibold"
              >
                {t("actions.serve")}
              </Button>
            )}
          </div>
        </div>
      </article>
    );
  },
  (prev, next) =>
    prev.order.id === next.order.id &&
    prev.order.status === next.order.status &&
    prev.order.updated_at === next.order.updated_at &&
    sameOrderItems(prev.order.items, next.order.items) &&
    prev.order.notes === next.order.notes &&
    prev.elapsedMinutes === next.elapsedMinutes &&
    prev.fireTime === next.fireTime &&
    prev.isLoading === next.isLoading &&
    prev.onStatusUpdate === next.onStatusUpdate &&
    prev.t === next.t &&
    prev.locale === next.locale &&
    prev.allergensByItemId === next.allergensByItemId,
);

interface KitchenDisplayModeProps {
    orders: Order[];
    onExit: () => void;
    onRefresh: () => void;
    businessId: number;
    locale: Locale;
  onOrderStatusChange?: (
    orderId: number,
    newStatus: string,
    approvedBy?: string,
    reason?: string,
  ) => Promise<void>;
    // IMP-13: menu_item_id → allergen tag list. Optional; KDS falls back to
    // modifiers + notes only when this is empty.
    allergensByItemId?: Record<string, string[]>;
  businessTimezone?: string | null;
}

const KitchenDisplayMode: React.FC<KitchenDisplayModeProps> = ({
    orders,
    onExit,
    onRefresh,
    businessId,
    locale,
    onOrderStatusChange,
    allergensByItemId = {},
  businessTimezone = null,
}) => {
    const operationalAlerts = useOptionalOperationalAlerts();
    const [isFullscreen, setIsFullscreen] = useState(false);
    const [currentTime, setCurrentTime] = useState(new Date());
    const [actionLoading, setActionLoading] = useState<number | null>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const wakeLockRef = useRef<WakeLockSentinel | null>(null);
    const [isLargeScreen, setIsLargeScreen] = useState(false);

    useEffect(() => {
        if (typeof window === "undefined" || !window.matchMedia) return;
        const mq = window.matchMedia("(min-width: 1024px)");
        const update = () => setIsLargeScreen(mq.matches);
        update();
        mq.addEventListener("change", update);
        return () => mq.removeEventListener("change", update);
    }, []);

    // Translation helper
    const t = useCallback(
        (key: string): string => {
            const fullKey = `kitchenDisplay.${key}`;
            const result = getTranslation(fullKey, locale);
            return Array.isArray(result) ? result[0] || key : (result as string);
        },
    [locale],
    );

    // Clock update
    useEffect(() => {
        const timer = setInterval(() => setCurrentTime(new Date()), 1000);
        return () => clearInterval(timer);
    }, []);

    // Wake Lock handling
    useEffect(() => {
        const requestWakeLock = async () => {
            try {
                if ("wakeLock" in navigator) {
                    wakeLockRef.current = await navigator.wakeLock.request("screen");
                    logger.debug("Wake Lock is active");
                }
            } catch (err) {
                console.error(`${err}`);
            }
        };

        requestWakeLock();

        const handleVisibilityChange = () => {
            if (document.visibilityState === "visible" && !wakeLockRef.current) {
                requestWakeLock();
            }
        };

        document.addEventListener("visibilitychange", handleVisibilityChange);

        return () => {
            if (wakeLockRef.current) {
                // release() returns a promise; an unhandled rejection on
                // teardown (lock already released by the UA) is noise.
                wakeLockRef.current.release().catch(() => {});
                wakeLockRef.current = null;
            }
            document.removeEventListener("visibilitychange", handleVisibilityChange);
        };
    }, []);

    // Fullscreen handling
    const toggleFullscreen = () => {
        if (!document.fullscreenElement) {
            containerRef.current?.requestFullscreen().catch((err) => {
                console.error(
          `Error attempting to enable full-screen mode: ${err.message} (${err.name})`,
                );
            });
        } else {
            document.exitFullscreen();
        }
    };

    // L1-5: leave browser fullscreen when exiting KDS so the operator is not stranded.
    const handleExit = useCallback(() => {
        if (document.fullscreenElement) {
            document.exitFullscreen().catch(() => {
                /* already left fullscreen */
            });
        }
        onExit();
    }, [onExit]);

    // L1-5 / Root C: Escape exits KDS (and exits fullscreen via handleExit).
    useDialogKeyboard(containerRef, handleExit);

    useEffect(() => {
        const handleFullscreenChange = () => {
            setIsFullscreen(!!document.fullscreenElement);
        };

        document.addEventListener("fullscreenchange", handleFullscreenChange);
        return () => {
            document.removeEventListener("fullscreenchange", handleFullscreenChange);
        };
    }, []);

    // Status updates — use parent's optimistic callback when available
    const handleStatusUpdate = useCallback(
      async (orderId: number, status: string) => {
        void operationalAlerts?.claimByResource(
          "order",
          orderId,
          "kds_status_change",
        );
        setActionLoading(orderId);
        try {
          if (onOrderStatusChange) {
            await onOrderStatusChange(orderId, status, "kitchen");
          } else {
            await updateOrderStatus(businessId, orderId, {
              status: status as any,
              approved_by: "kitchen",
            });
            onRefresh();
          }
        } catch (error) {
          // K-3: list mode already toasts (Kitchen.tsx) — KDS was silent. The
          // parent's onOrderStatusChange refetches on failure before
          // re-throwing, so by the time this toast shows, the board has been
          // rolled back to server truth.
          logger.error("KDS status update failed", error);
          const sanitized = sanitizeErrorKey(error);
          if (sanitized.status === 409) {
            toast.error(t("errors.conflictRefreshed"));
          } else {
            toast.error(
              t("errors.updateFailed").replace(
                "{message}",
                localizedErrorMessage(error, locale),
              ),
            );
          }
        } finally {
          setActionLoading(null);
        }
      },
      [businessId, onOrderStatusChange, onRefresh, operationalAlerts, t, locale],
    );

    // Group orders by status
    const approvedOrders = sortOrdersFifo(
        orders.filter((o) => o.status === "approved"),
    );
    const inKitchenOrders = sortOrdersFifo(
        orders.filter((o) => o.status === "in_kitchen"),
    );
    const readyOrders = sortOrdersFifo(
        orders.filter((o) => o.status === "ready"),
    );

    const elapsedMinutesFor = (createdAt: string): number =>
      kdsElapsedMinutes(createdAt, currentTime);

    const renderOrderCard = (order: Order) => (
      <OrderCard
        key={order.id}
        order={order}
        elapsedMinutes={elapsedMinutesFor(order.created_at)}
        fireTime={formatKdsFireTime(
          order.created_at,
          locale,
          businessTimezone,
        )}
        isLoading={actionLoading === order.id}
        onStatusUpdate={handleStatusUpdate}
        t={t}
        locale={locale}
        allergensByItemId={allergensByItemId}
      />
    );

    const statusGroups = [
    {
      key: "approved",
      label: t("columns.approved"),
      icon: <Bell className="w-4 h-4 text-amber-600" />,
      orders: approvedOrders,
      color: "warning" as const,
    },
    {
      key: "in_kitchen",
      label: t("columns.in_kitchen"),
      icon: <ChefHat className="w-4 h-4 text-brand" />,
      orders: inKitchenOrders,
      color: "primary" as const,
    },
    {
      key: "ready",
      label: t("columns.ready"),
      icon: <CheckCircle className="w-4 h-4 text-emerald-600" />,
      orders: readyOrders,
      color: "success" as const,
    },
    ];

    return (
        <div
            ref={containerRef}
            // L1-5: z-[100] above TopMenu (z-50) once Root A no longer traps stacking;
            // [&:fullscreen] fills the UA fullscreen layer without the thin-strip
            // collapse. The `&` is required — without it the arbitrary variant
            // is silently dropped by Tailwind (emits no CSS).
            className="fixed inset-0 z-[100] flex flex-col overflow-hidden bg-warm-50 p-0 text-ink-950 [&:fullscreen]:h-screen [&:fullscreen]:w-screen"
            tabIndex={-1}
        >
            <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_12%_8%,rgba(26,107,106,0.18),transparent_26%),radial-gradient(circle_at_90%_8%,rgba(255,255,255,0.72),transparent_22%),linear-gradient(180deg,#faf9f6_0%,#ece9df_100%)]" />
            <div className="pointer-events-none absolute inset-0 opacity-[0.3] [background-image:linear-gradient(rgba(255,255,255,0.38)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.38)_1px,transparent_1px)] [background-size:32px_32px]" />
            {/* KDS Header */}
            <div className="relative z-10 flex items-center justify-between border-b border-ink-900/20 bg-ink-950 px-3 py-3 text-white shadow-[0_18px_45px_rgba(46,42,37,0.24)] sm:p-4">
                <div className="flex items-center gap-4">
                    <Button
                        isIconOnly
                        variant="light"
                        aria-label={t("exitDisplayAria")}
                        className="rounded-xl text-white hover:bg-white/15"
                        onPress={handleExit}
                    >
                        <X className="w-6 h-6" />
                    </Button>
                    <div>
                        <h1 className="text-base sm:text-xl font-bold tracking-wide flex items-center gap-2">
                            <ChefHat className="w-6 h-6" />
                            {t("title")}
                        </h1>
                        <p className="flex items-center gap-1 text-xs text-white/60">
                            <span className="w-2 h-2 rounded-full bg-emerald-500 motion-safe:animate-pulse"></span>
              {t("liveUpdates")} •{" "}
              {formatKdsFireTime(currentTime, locale, businessTimezone)}
                        </p>
                    </div>
                </div>
                <div className="flex items-center gap-2">
          <EnableAlertSoundButton
            compact
            className="bg-white/10 text-white hover:bg-white/15"
          />
                    <Button
                        isIconOnly
                        variant="flat"
            aria-label={
              isFullscreen ? t("exitFullscreenAria") : t("enterFullscreenAria")
            }
                        aria-pressed={isFullscreen}
                        className="rounded-xl bg-white/10 text-white hover:bg-white/15"
                        onPress={toggleFullscreen}
                    >
                        {isFullscreen ? (
                            <Minimize className="w-5 h-5" />
                        ) : (
                            <Maximize className="w-5 h-5" />
                        )}
                    </Button>
                </div>
            </div>

            {/* Kanban Board (desktop) / Stacked List (mobile) */}
            {isLargeScreen ? (
                <div className="relative z-[1] flex-1 overflow-x-auto overflow-y-hidden p-4">
                    <div className="flex h-full gap-4 min-w-[1024px]">
                        {/* Column 1: Approved (To Prepare) */}
                        <div className="flex min-w-[320px] flex-1 flex-col overflow-hidden rounded-3xl border border-amber-200/80 bg-amber-50/70 shadow-[inset_0_1px_0_rgba(255,255,255,0.74),0_18px_45px_rgba(146,64,14,0.08)]">
                            <div className="sticky top-0 z-10 flex items-center justify-between border-b border-amber-200/80 bg-white/86 p-3 backdrop-blur-xl">
                                <h2 className="flex items-center gap-2 font-bold text-amber-900">
                                    <Bell className="w-5 h-5 text-amber-600" />
                                    {t("columns.approved")}
                                </h2>
                                <Chip size="sm" color="warning" variant="flat">
                                    {approvedOrders.length}
                                </Chip>
                            </div>
                            <div className="flex-1 overflow-y-auto p-3 custom-scrollbar">
                                {approvedOrders.length === 0 ? (
                                    <div className="flex h-full flex-col items-center justify-center text-amber-900/45">
                                        <ChefHat className="mb-2 h-12 w-12" />
                                        <p>{t("noOrders")}</p>
                                    </div>
                                ) : (
                                    approvedOrders.map(renderOrderCard)
                                )}
                            </div>
                        </div>

                        {/* Column 2: In Kitchen (Cooking) */}
                        <div className="flex min-w-[320px] flex-1 flex-col overflow-hidden rounded-3xl border border-brand/20 bg-brand/10 shadow-[inset_0_1px_0_rgba(255,255,255,0.74),0_18px_45px_rgba(26,107,106,0.10)]">
                            <div className="sticky top-0 z-10 flex items-center justify-between border-b border-brand/15 bg-white/86 p-3 backdrop-blur-xl">
                                <h2 className="flex items-center gap-2 font-bold text-brand-dark">
                                    <ChefHat className="w-5 h-5 text-brand" />
                                    {t("columns.in_kitchen")}
                                </h2>
                                <Chip size="sm" color="primary" variant="flat">
                                    {inKitchenOrders.length}
                                </Chip>
                            </div>
                            <div className="flex-1 overflow-y-auto p-3 custom-scrollbar">
                                {inKitchenOrders.length === 0 ? (
                                    <div className="flex h-full flex-col items-center justify-center text-brand-dark/45">
                                        <ChefHat className="mb-2 h-12 w-12" />
                                        <p>{t("noOrders")}</p>
                                    </div>
                                ) : (
                                    inKitchenOrders.map(renderOrderCard)
                                )}
                            </div>
                        </div>

                        {/* Column 3: Ready (To Serve) */}
                        <div className="flex min-w-[320px] flex-1 flex-col overflow-hidden rounded-3xl border border-emerald-200/80 bg-emerald-50/70 shadow-[inset_0_1px_0_rgba(255,255,255,0.74),0_18px_45px_rgba(6,78,59,0.08)]">
                            <div className="sticky top-0 z-10 flex items-center justify-between border-b border-emerald-200/80 bg-white/86 p-3 backdrop-blur-xl">
                                <h2 className="flex items-center gap-2 font-bold text-emerald-900">
                                    <CheckCircle className="w-5 h-5 text-emerald-600" />
                                    {t("columns.ready")}
                                </h2>
                                <Chip size="sm" color="success" variant="flat">
                                    {readyOrders.length}
                                </Chip>
                            </div>
                            <div className="flex-1 overflow-y-auto p-3 custom-scrollbar">
                                {readyOrders.length === 0 ? (
                                    <div className="flex h-full flex-col items-center justify-center text-emerald-900/45">
                                        <CheckCircle className="mb-2 h-12 w-12" />
                                        <p>{t("noOrders")}</p>
                                    </div>
                                ) : (
                                    readyOrders.map(renderOrderCard)
                                )}
                            </div>
                        </div>
                    </div>
                </div>
            ) : (
                <div className="relative z-[1] flex-1 space-y-4 overflow-y-auto p-3">
                    {statusGroups.map((group) => (
                        <section key={group.key}>
                            <div className="sticky top-0 z-10 mb-2 flex items-center justify-between rounded-2xl border border-warm-200/80 bg-white/90 px-3 py-2 shadow-sm shadow-warm-300/30 backdrop-blur-xl">
                                <h2 className="flex items-center gap-2 text-sm font-bold text-ink-800">
                                    {group.icon}
                                    {group.label}
                                </h2>
                                <Chip size="sm" color={group.color} variant="flat">
                                    {group.orders.length}
                                </Chip>
                            </div>
                            {group.orders.length === 0 ? (
                <p className="py-4 text-center text-sm text-ink-500">
                  {t("noOrders")}
                </p>
                            ) : (
                                <div className="space-y-2">
                                    {group.orders.map(renderOrderCard)}
                                </div>
                            )}
                        </section>
                    ))}
                </div>
            )}

            <style jsx global>{`
        .custom-scrollbar::-webkit-scrollbar {
          width: 6px;
        }
        .custom-scrollbar::-webkit-scrollbar-track {
          background: transparent;
        }
        .custom-scrollbar::-webkit-scrollbar-thumb {
          background-color: rgba(0, 0, 0, 0.1);
          border-radius: 20px;
        }
        .custom-scrollbar:hover::-webkit-scrollbar-thumb {
          background-color: rgba(0, 0, 0, 0.2);
        }
      `}</style>
        </div>
    );
};

export default KitchenDisplayMode;
