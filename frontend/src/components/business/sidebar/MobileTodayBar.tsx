"use client";

/**
 * M4 — mobile TODAY bottom bar.
 *
 * On phones the sidebar hides behind a hamburger, so the three service rails
 * an operator reaches for mid-service (Overview, Kitchen, Reservations) get a
 * thumb-height bar pinned below the content. Desktop never sees it
 * (`lg:hidden`). Same rules as the rail: allowedTabs filter, access locks,
 * useRailBadges counts, registry icons — and a touchstart chunk prefetch
 * (M1) since mobile has no hover intent.
 */

import React from "react";
import { Lock } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { TAB_REGISTRY, prefetchTab } from "../tabs/tabRegistry";
import type { TabKey } from "../tabs/tabRegistry";
import { getTabLockMeta } from "../commandPalette/tabAccess";
import type { AccessState } from "../commandPalette/tabAccess";
import { AnimatedBadge } from "../premium/AnimatedBadge";
import { useRailBadges } from "./useRailBadges";
import { useOptionalOperationalAlerts } from "../operational-alerts/useOperationalAlerts";
import type { Reservation } from "@/api/reservations";

const TODAY_TABS = ["overview", "kitchen", "reservations"] as const;

const TAB_LABEL_KEYS: Record<(typeof TODAY_TABS)[number], string> = {
  overview: "tabs.overview",
  kitchen: "tabs.kitchen",
  reservations: "tabs.reservations",
};

export interface MobileTodayBarProps {
  businessId: number;
  activeTab: string;
  onNavigate: (tab: string) => void;
  allowedTabs?: string[];
  accessState: AccessState;
  isStaffUser?: boolean;
  /** Dashboard-polled badge inputs — same fidelity as the desktop rail. */
  occupiedTablesCount?: number;
  globalOrders?: Record<number, any[]>;
  globalOrdersLoaded?: boolean;
  upcomingReservations?: Reservation[];
  /** Tutorial chrome owns the viewport — the bar steps aside. */
  hidden?: boolean;
}

export function MobileTodayBar({
  businessId,
  activeTab,
  onNavigate,
  allowedTabs = [],
  accessState,
  isStaffUser = false,
  occupiedTablesCount,
  globalOrders,
  globalOrdersLoaded,
  upcomingReservations,
  hidden = false,
}: MobileTodayBarProps) {
  const { locale } = useSimpleLocale();
  const alertCounts = useOptionalOperationalAlerts()?.counts ?? null;
  const badges = useRailBadges({
    businessId,
    hasAccess: !accessState.loading && !accessState.isSuspended,
    alertCounts,
    occupiedTablesCount,
    globalOrders,
    globalOrdersLoaded,
    upcomingReservations,
  });

  const tString = React.useCallback(
    (key: string): string => {
      const result = getTranslation(`businessDashboard.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const visibleTabs = React.useMemo(
    () =>
      TODAY_TABS.filter(
        (key) => allowedTabs.length === 0 || allowedTabs.includes(key),
      ).filter((key) => !getTabLockMeta(key, accessState, isStaffUser).hidden),
    [allowedTabs, accessState, isStaffUser],
  );

  if (hidden || visibleTabs.length === 0) return null;

  return (
    <nav
      aria-label={tString("mobileNav.label")}
      className="relative z-30 flex items-stretch justify-around border-t border-warm-200/80 bg-white/95 pb-[env(safe-area-inset-bottom)] shadow-[0_-8px_24px_rgba(46,42,37,0.08)] backdrop-blur-xl lg:hidden"
    >
      {visibleTabs.map((key: TabKey) => {
        const label = tString(TAB_LABEL_KEYS[key as keyof typeof TAB_LABEL_KEYS]);
        const Icon = TAB_REGISTRY[key].icon;
        const active = activeTab === key;
        const lock = getTabLockMeta(key, accessState, isStaffUser);
        const badge = badges[key] ?? null;

        return (
          <button
            key={key}
            type="button"
            aria-current={active ? "page" : undefined}
            onClick={() => onNavigate(key)}
            onTouchStart={() => void prefetchTab(key)}
            className={`flex min-h-[56px] flex-1 flex-col items-center justify-center gap-0.5 px-2 py-1.5 text-[11px] font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand ${
              active ? "text-brand-dark" : "text-ink-500"
            }`}
          >
            <span className="relative inline-flex">
              <Icon className="h-5 w-5" aria-hidden="true" />
              {lock.locked ? (
                <Lock
                  className="absolute -right-2 -top-1 h-3 w-3 text-ink-400"
                  aria-label={tString("locked.short")}
                />
              ) : null}
              <AnimatedBadge
                count={badge}
                label={label}
                testId={`mobile-today-badge-${key}`}
                className="absolute -right-2.5 -top-1.5"
              />
            </span>
            <span className="max-w-full truncate">{label}</span>
          </button>
        );
      })}
    </nav>
  );
}
