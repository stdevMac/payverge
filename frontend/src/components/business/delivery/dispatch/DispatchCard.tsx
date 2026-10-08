import React, { useEffect, useState } from "react";
import { Clock, AlertTriangle, MapPin, Phone, Package } from "lucide-react";
import { formatCurrency } from "@/api/currency";
import OperationalAlertClaimStatus from "../../operational-alerts/OperationalAlertClaimStatus";
import { getOperatorNextStatus } from "./OrderRow";
import { DriverAssignment } from "./DriverAssignment";
import { formatBusinessTime } from "@/utils/businessTime";
import type { DeliveryDriver } from "@/api/delivery";

/** Human fallback for a status when no translation is supplied (en default). */
function humanizeStatus(status: string): string {
  return status
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

export interface DispatchOrder {
  id: number;
  code: string;
  status: string;
  customer_name: string;
  customer_phone?: string;
  address?: string;
  /** Guest ETA (ISO). Prefer this for the primary clock — never hours cutoff. */
  eta?: string;
  /** Deliver-by cutoff (ISO). Used for urgency only; shown separately from ETA. */
  cutoff?: string;
  fee: number;
  /** Order subtotal (dollars) when known from quote metadata. */
  order_total?: number;
  /** Bag size when known from quote metadata. */
  item_count?: number;
  driver?: { name: string; live: boolean; eta?: string } | null;
  /** Dispatcher claim holder (one operator at a time). */
  claimed_by_staff_id?: number | null;
  claimed_by_name?: string;
  claimed_by_role?: string;
  claimed_at?: string | null;
  /** Prepay window still open (confirmed + future payment_expires_at). */
  awaiting_payment?: boolean;
  payment_expires_at?: string;
}

interface DispatchCardProps {
  order: DispatchOrder;
  /** May return a promise — the card re-enables its advance button once the
   *  advance settles (success OR failure). */
  onAdvance: (o: DispatchOrder) => void | Promise<void>;
  onCancel: (o: DispatchOrder) => void;
  /** Open the order detail drawer. */
  onOpenDetail?: (o: DispatchOrder) => void;
  /** Called when a driver is selected from the inline assignment control. */
  onAssignDriver?: (orderId: number, driverId: number) => Promise<void>;
  /** Claim / release / steal handlers for the multi-operator lock. */
  onClaim?: (o: DispatchOrder, opts?: { steal?: boolean }) => void | Promise<void>;
  onRelease?: (o: DispatchOrder) => void | Promise<void>;
  /** Current operator staff id (null for owner principal). */
  myStaffId?: number | null;
  /** When true, show take-over for claims held by someone else. */
  canSteal?: boolean;
  /** Available drivers list for inline assignment (reuses DriverAssignment). */
  drivers?: DeliveryDriver[];
  /** Translation helper from the dispatch namespace (parent passes one in). */
  tString?: (key: string) => string;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /** Operator locale (BCP-47); falls back to "en". */
  locale?: string;
  /** Business IANA timezone; null/invalid falls back to UTC (never device TZ). */
  businessTimezone?: string | null;
}

export default function DispatchCard({
  order,
  onAdvance,
  onCancel,
  onOpenDetail,
  onAssignDriver,
  onClaim,
  onRelease,
  myStaffId = null,
  canSteal = false,
  drivers = [],
  tString,
  currency = "USD",
  locale,
  businessTimezone = null,
}: DispatchCardProps) {
  /** Format an ISO instant as a short clock time in the business timezone,
   *  matching the cutoff display on the same card. */
  const formatEta = (iso?: string): string =>
    iso
      ? formatBusinessTime(iso, locale ?? "en", businessTimezone, {
          hour: "numeric",
          minute: "2-digit",
        })
      : "";
  // Primary clock is guest ETA. Cutoff only drives the urgent flag — using
  // hours-cutoff as "ETA" made delivered/near-close cards read as order time.
  const hasEta = Boolean(order.eta);
  const hasCutoff = Boolean(order.cutoff);
  const minutesLeft = hasCutoff
    ? Math.floor((new Date(order.cutoff!).getTime() - Date.now()) / 60000)
    : Infinity;
  // Terminal orders (delivered/cancelled/failed) are done — the deliver-by
  // cutoff is irrelevant, so never flag them as urgent. Otherwise a delivered
  // card's long-past cutoff always reads as a red "late" warning.
  const isTerminal = ["delivered", "cancelled", "failed"].includes(order.status);
  const isIntake = order.status === "pending" || order.status === "confirmed";
  const urgent = hasCutoff && !isTerminal && !isIntake && minutesLeft < 15;
  const t = (key: string, fallback: string) => {
    const v = tString?.(key);
    return v && v !== key ? v : fallback;
  };
  // R13: ready-without-driver → primary action is "Assign driver".
  // ready WITH a driver → advance button (which will move to picked_up directly).
  const isReady = order.status === "ready";
  const hasDriver = Boolean(order.driver);
  const showAssignPrimary = isReady && !hasDriver && onAssignDriver != null;

  // Operator advance skips pending/confirmed (Accept lives in Bills; payment
  // advance is system-owned). Using getNextStatus here would offer a broken
  // "Advance to preparing" on intake cards.
  const nextStatus =
    isReady && hasDriver
      ? ("picked_up" as const)
      : getOperatorNextStatus(order.status);
  const nextLabel = nextStatus
    ? t(`filters.${nextStatus}`, humanizeStatus(nextStatus))
    : "";
  const advanceTemplate = t("actions.advance", "Advance to {next}");
  const advanceLabel = nextStatus
    ? advanceTemplate.replace("{next}", nextLabel)
    : "";

  const [showAssignPanel, setShowAssignPanel] = useState(false);
  const [claimBusy, setClaimBusy] = useState(false);
  // In-flight guard: a double-click (or slow request) must not fire the advance
  // twice. The guard MUST reset when the advance settles: the board keys cards
  // by id and collapses the driver-leg statuses into one column, so in-column
  // advances re-render the SAME instance — a one-way latch would permanently
  // disable the button after one advance or any failed request (DEL-OP-1).
  const [advancing, setAdvancing] = useState(false);

  const claimFresh =
    !!order.claimed_at &&
    Date.now() - new Date(order.claimed_at).getTime() < 5 * 60 * 1000;
  const heldByMe =
    claimFresh &&
    ((myStaffId != null && order.claimed_by_staff_id === myStaffId) ||
      (myStaffId == null &&
        order.claimed_by_staff_id == null &&
        order.claimed_by_role === "owner"));
  const heldByOther =
    claimFresh && !heldByMe && Boolean(order.claimed_by_name || order.claimed_at);
  // Belt-and-braces: a status change from any source (SSE refresh, polling)
  // also clears the guard, covering parents whose onAdvance returns void.
  useEffect(() => {
    setAdvancing(false);
  }, [order.status]);
  const handleAdvance = () => {
    if (advancing) return;
    setAdvancing(true);
    // Errors are surfaced by the parent (toast in DispatchConsole.handleAdvance);
    // here we only need the button back once the request settles — swallow the
    // rejection so it doesn't escape as an unhandled promise rejection.
    void Promise.resolve(onAdvance(order))
      .catch(() => {})
      .finally(() => setAdvancing(false));
  };

  return (
    <div className="border border-warm-100 rounded-xl p-3 bg-white space-y-2">
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions -- role/tabIndex/keyboard are attached together with onClick when onOpenDetail exists */}
      <div
        className={
          onOpenDetail
            ? "cursor-pointer rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            : undefined
        }
        role={onOpenDetail ? "button" : undefined}
        tabIndex={onOpenDetail ? 0 : undefined}
        data-testid="dispatch-card-open-detail"
        onClick={onOpenDetail ? () => onOpenDetail(order) : undefined}
        onKeyDown={
          onOpenDetail
            ? (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onOpenDetail(order);
                }
              }
            : undefined
        }
      >
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate font-mono text-sm text-ink-700">{order.code}</span>
        {/* shrink-0 + nowrap: long delivery codes otherwise squeeze the cutoff
            time until it wraps mid-token off the card edge. */}
        <span data-testid="dispatch-eta" className={`inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-xs ${urgent ? "text-rose-500" : "text-ink-500"} tabular-nums`}>
          {urgent ? <AlertTriangle className="w-3 h-3" /> : <Clock className="w-3 h-3" />}
          {hasEta
            ? `${t("card.eta", "ETA")} ${formatEta(order.eta)}`
            : t("order.noEta", "No ETA")}
        </span>
      </div>
      <p className="text-sm text-ink-900">{order.customer_name}</p>
      {order.customer_phone && (
        <p className="flex items-center gap-1 text-xs text-ink-600">
          <Phone className="h-3 w-3 shrink-0" aria-hidden="true" />
          <span className="truncate">{order.customer_phone}</span>
        </p>
      )}
      {order.address && (
        <p className="flex items-start gap-1 text-xs text-ink-600">
          <MapPin className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />
          <span className="line-clamp-2">{order.address}</span>
        </p>
      )}
      </div>
      <OperationalAlertClaimStatus
        resourceType="delivery"
        resourceId={order.id}
      />
      {order.awaiting_payment && (
        <p
          data-testid="dispatch-awaiting-payment"
          className="rounded-lg bg-amber-50 px-2 py-1 text-xs font-medium text-amber-800"
        >
          {t("card.awaitingPayment", "Awaiting guest payment")}
        </p>
      )}
      {isIntake && !order.awaiting_payment && order.status === "pending" && (
        <p className="rounded-lg bg-amber-50 px-2 py-1 text-xs font-medium text-amber-800">
          {t("card.awaitingAcceptance", "Awaiting acceptance in Bills")}
        </p>
      )}
      {/* Multi-operator dispatch claim lock (L4-8). */}
      {!isTerminal && !isIntake && (onClaim || onRelease) && (
        <div
          data-testid="dispatch-claim-row"
          className="flex flex-wrap items-center gap-2 text-xs"
        >
          {heldByMe ? (
            <>
              <span className="rounded-full bg-brand/10 px-2 py-0.5 text-brand">
                {t("claim.heldByYou", "You hold this order")}
              </span>
              {onRelease && (
                <button
                  type="button"
                  data-testid="dispatch-release-btn"
                  disabled={claimBusy}
                  onClick={(e) => {
                    e.stopPropagation();
                    if (claimBusy) return;
                    setClaimBusy(true);
                    void Promise.resolve(onRelease(order))
                      .catch(() => {})
                      .finally(() => setClaimBusy(false));
                  }}
                  className="text-ink-600 underline-offset-2 hover:underline disabled:opacity-50"
                >
                  {t("claim.release", "Release")}
                </button>
              )}
            </>
          ) : heldByOther ? (
            <>
              <span className="rounded-full bg-amber-50 px-2 py-0.5 text-amber-800">
                {t("claim.heldBy", "Held by {name}").replace(
                  "{name}",
                  order.claimed_by_name || t("claim.unclaimed", "Unclaimed"),
                )}
              </span>
              {canSteal && onClaim && (
                <button
                  type="button"
                  data-testid="dispatch-steal-btn"
                  disabled={claimBusy}
                  onClick={(e) => {
                    e.stopPropagation();
                    if (claimBusy) return;
                    setClaimBusy(true);
                    void Promise.resolve(onClaim(order, { steal: true }))
                      .catch(() => {})
                      .finally(() => setClaimBusy(false));
                  }}
                  className="text-amber-800 underline-offset-2 hover:underline disabled:opacity-50"
                >
                  {t("claim.steal", "Take over")}
                </button>
              )}
            </>
          ) : (
            onClaim && (
              <button
                type="button"
                data-testid="dispatch-claim-btn"
                disabled={claimBusy}
                onClick={(e) => {
                  e.stopPropagation();
                  if (claimBusy) return;
                  setClaimBusy(true);
                  void Promise.resolve(onClaim(order))
                    .catch(() => {})
                    .finally(() => setClaimBusy(false));
                }}
                className="rounded-lg border border-warm-200 px-2 py-0.5 text-ink-700 hover:bg-warm-50 disabled:opacity-50"
              >
                {t("claim.claim", "Claim")}
              </button>
            )
          )}
        </div>
      )}
      {order.driver && (
        <p className="text-xs text-ink-600">
          {order.driver.live ? `● ${t("card.live", "Live")} · ` : ""}
          {order.driver.name}
          {order.driver.eta && order.driver.eta !== order.eta
            ? ` · ${t("card.eta", "ETA")} ${formatEta(order.driver.eta)}`
            : ""}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-600">
        {typeof order.item_count === "number" && order.item_count > 0 && (
          <span className="inline-flex items-center gap-1" data-testid="dispatch-item-count">
            <Package className="h-3 w-3" aria-hidden="true" />
            {t("card.items", "{count} items").replace(
              "{count}",
              String(order.item_count),
            )}
          </span>
        )}
        {typeof order.order_total === "number" && (
          <span data-testid="dispatch-order-total" className="tabular-nums">
            {formatCurrency(order.order_total, currency)}
          </span>
        )}
        <span data-testid="dispatch-fee" className="tabular-nums">
          {t("order.fee", "Fee")} {formatCurrency(order.fee, currency)}
        </span>
      </div>

      {/* R13: ready + no driver → Assign driver is the primary action */}
      {showAssignPrimary && !showAssignPanel && (
        <button
          data-testid="assign-driver-btn"
          onClick={() => setShowAssignPanel(true)}
          className="w-full text-sm bg-brand text-white rounded-lg py-1.5"
        >
          {t("assignDriver", "Assign driver")}
        </button>
      )}

      {/* Inline driver assignment panel (reuses DriverAssignment — same as mobile) */}
      {showAssignPrimary && showAssignPanel && (
        <div className="space-y-1">
          <DriverAssignment
            drivers={drivers}
            onAssign={async (driverId) => {
              // Close only on success — a failed assignment (handleAssignDriver
              // rejects after toasting) must keep the panel open so the
              // operator can retry (DEL-OP-4). Rethrow so DriverAssignment
              // preserves its selection too.
              await onAssignDriver!(order.id, driverId);
              setShowAssignPanel(false);
            }}
            tString={(key) => t(key.replace(/^dispatch\./, ""), key)}
          />
          <button
            onClick={() => setShowAssignPanel(false)}
            className="w-full text-xs text-ink-500"
          >
            {t("card.cancel", "Cancel")}
          </button>
        </div>
      )}

      {/* Advance button: shown when there is a next status AND it's not a ready-no-driver case */}
      {nextStatus && !showAssignPrimary && (
        <button
          onClick={handleAdvance}
          disabled={advancing}
          className="w-full text-sm bg-brand text-white rounded-lg py-1.5 disabled:opacity-60 disabled:cursor-not-allowed"
        >
          {advanceLabel}
        </button>
      )}

      {/* Cancel only for non-terminal orders; a delivered/cancelled/failed order
          can't be cancelled. Only one cancel rendered (hidden while assigning). */}
      {!showAssignPanel &&
        !["delivered", "cancelled", "failed"].includes(order.status) && (
          <button onClick={() => onCancel(order)} className="w-full text-sm text-rose-600">{t("card.cancel", "Cancel")}</button>
        )}
    </div>
  );
}
