"use client";

/**
 * M3 — rail badge derivation, consolidated.
 *
 * Every sidebar rail badge follows one of five rules, previously spread
 * across ~90 lines of the sidebar body (and the Bills/Kitchen split was the
 * #92 regression):
 *
 *   bills         tickets waiting for approval (#793) — pending orders from
 *                 the dashboard globalOrders feed once loaded; open-check
 *                 totals stay as table copy inside the tab, never a red pip
 *   kitchen       tickets waiting on the cook (#792) — pending + approved from
 *                 the same globalOrders feed the Kitchen tab renders (the
 *                 kitchen_order_ready alert count measured "ready" pings and
 *                 disagreed with the board)
 *   reservations  seated window — alertCounts.reservations, else local
 *                 -15/+30 min filter over upcoming reservations
 *   tables        floor occupancy — dashboard-polled occupiedTablesCount wins
 *                 so leftover-kitchen tables stay visible (#774); quiet floors
 *                 fall back to service-call alerts
 *   delivery      alerts-only (no local fallback)
 *   accounting/fiscal failed-invoice count (tier-gated fetch)
 *   staff         unread staff-chat count
 *
 * Returns a Partial<Record<TabKey, number | null>> — tabs not listed are
 * badge-less; a 0 count collapses to null so the rail never renders "0".
 */

import React from "react";
import type { Reservation } from "@/api/reservations";
import { useFailedInvoiceCount } from "@/hooks/useFailedInvoiceCount";
import { useChatUnreadCount } from "@/hooks/useChatUnreadCount";
import { countCookQueue, countPendingApprovals } from "./orderQueueCounts";
import type { TabKey } from "../tabs/tabRegistry";

export interface RailAlertCounts {
  bills: number;
  kitchen: number;
  reservations: number;
  delivery: number;
  tables: number;
}

export interface UseRailBadgesInput {
  businessId: number;
  /** Access gate for the failed-invoice fetch (non-fiscal businesses skip it). */
  hasAccess: boolean;
  /** OperationalAlerts context counts, or null when outside the provider. */
  alertCounts: RailAlertCounts | null;
  /** Dashboard-polled occupied-table count — Tables badge source of truth. */
  occupiedTablesCount?: number;
  /** Dashboard-polled orders by bill — Bills/Kitchen badge source of truth. */
  globalOrders?: Record<number, any[]>;
  /**
   * True once the dashboard's first orders reconcile has landed (K-1). Until
   * then the Bills/Kitchen badges fall back to alert counts so a cold paint
   * is not a false zero.
   */
  globalOrdersLoaded?: boolean;
  /** Upcoming reservations — Reservations badge local fallback. */
  upcomingReservations?: Reservation[];
}

export type RailBadges = Partial<Record<TabKey, number | null>>;

const orNull = (n: number): number | null => (n > 0 ? n : null);

export function useRailBadges({
  businessId,
  hasAccess,
  alertCounts,
  occupiedTablesCount,
  globalOrders = {},
  globalOrdersLoaded,
  upcomingReservations = [],
}: UseRailBadgesInput): RailBadges {
  // Compliance/revenue signal must be visible BEFORE the operator enters
  // Accounting — gated on tier access so non-fiscal businesses never fetch.
  const failedInvoiceCount = useFailedInvoiceCount(businessId, hasAccess);
  // The comms/chat view lives inside Team and had no signal outside it.
  const chatUnreadCount = useChatUnreadCount(businessId);

  // Bills badge = tickets waiting for approval (#793). A host chasing the red
  // pip must land on the approval queue, not "how many open checks" — those
  // stay as table copy ("Active Bills (2)") inside the tab. Alert counts only
  // bridge the gap before the first orders reconcile lands.
  const bills = React.useMemo(() => {
    if (globalOrdersLoaded) return countPendingApprovals(globalOrders);
    if (alertCounts && typeof alertCounts.bills === "number") {
      return alertCounts.bills;
    }
    return countPendingApprovals(globalOrders);
  }, [globalOrdersLoaded, globalOrders, alertCounts]);

  // Kitchen badge = tickets waiting on the cook right now (#792): needs
  // approval + approved/Start Cooking, from the same feed the tab renders.
  // Never show 1 (or blank) while two Start Cooking cards sit on the board.
  const kitchen = React.useMemo(() => {
    if (globalOrdersLoaded) return countCookQueue(globalOrders);
    if (alertCounts) return alertCounts.kitchen;
    return countCookQueue(globalOrders);
  }, [globalOrdersLoaded, globalOrders, alertCounts]);

  // Active reservations: 15 min before (late arrivals) to 30 min ahead.
  const reservations = React.useMemo(() => {
    if (alertCounts) return alertCounts.reservations;
    if (!upcomingReservations || upcomingReservations.length === 0) return 0;

    const now = new Date();
    const fifteenMinutesAgo = new Date(now.getTime() - 15 * 60 * 1000);
    const thirtyMinutesFromNow = new Date(now.getTime() + 30 * 60 * 1000);

    return upcomingReservations.filter((reservation) => {
      const reservationTime = new Date(reservation.reservation_time);
      const isInWindow =
        reservationTime >= fifteenMinutesAgo &&
        reservationTime <= thirtyMinutesFromNow;
      const hasValidStatus =
        reservation.status === "confirmed" || reservation.status === "pending";
      return hasValidStatus && isInWindow;
    }).length;
  }, [alertCounts, upcomingReservations]);

  // Occupancy (open checks + leftover kitchen) is the host-stand number.
  // Service-call alerts stay as the fallback so a quiet floor still surfaces
  // waiter calls without implying those calls are occupancy.
  const tables = React.useMemo(() => {
    if (typeof occupiedTablesCount === "number" && occupiedTablesCount > 0) {
      return occupiedTablesCount;
    }
    return alertCounts?.tables ?? 0;
  }, [occupiedTablesCount, alertCounts]);

  return React.useMemo(
    () => ({
      bills: orNull(bills),
      kitchen: orNull(kitchen),
      reservations: orNull(reservations),
      tables: orNull(tables),
      delivery: orNull(alertCounts?.delivery ?? 0),
      accounting: orNull(failedInvoiceCount),
      fiscal: orNull(failedInvoiceCount),
      staff: orNull(chatUnreadCount),
    }),
    [
      bills,
      kitchen,
      reservations,
      tables,
      alertCounts?.delivery,
      failedInvoiceCount,
      chatUnreadCount,
    ],
  );
}
