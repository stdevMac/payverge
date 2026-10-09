"use client";

import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { BarChart3, Clock, DollarSign, Package, Users, SearchX } from "lucide-react";
import dynamic from "next/dynamic";
import TodayLivePanel from "./panels/TodayLivePanel";
import MenuPanel from "./panels/MenuPanel";
import PaymentHistory from "./PaymentHistory";
import CostAnalyticsSection from "./CostAnalyticsSection";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import DashboardLockedTabView from "../business/DashboardLockedTabView";
import DashboardTabShell from "../business/shared/DashboardTabShell";
import DashboardTabLoadingSkeleton from "../business/shared/DashboardTabLoadingSkeleton";
import { DashboardTabTransition } from "../business/premium";
import { EmptyState } from "@/components/ui/EmptyState";
import { useSubTab } from "@/lib/subTabs";
import { useUrlState } from "@/hooks/useUrlState";

/** Shared analytics/finance period keys (L6-2 / L6-5). Invalid values normalize. */
export const ANALYTICS_PERIOD_VALUES = [
  "today",
  "yesterday",
  "week",
  "month",
  "quarter",
  "year",
] as const;
export type AnalyticsPeriod = (typeof ANALYTICS_PERIOD_VALUES)[number];

// RevenuePanel + ServiceTipsPanel statically import chart.js (TrendChart /
// HourOfDayChart). Defer them so the 6.2 MB engine loads on the analytics tab,
// not the operator dashboard's first paint — same pattern as the page-level
// AiWaiterDashboard/DirectorConsoleDashboard dynamic loads. (P-4)
const RevenuePanel = dynamic(() => import("./panels/RevenuePanel"), {
  ssr: false,
  loading: () => (
    <div className="animate-pulse h-64 bg-default-100 rounded-lg" />
  ),
});
const ServiceTipsPanel = dynamic(() => import("./panels/ServiceTipsPanel"), {
  ssr: false,
  loading: () => (
    <div className="animate-pulse h-64 bg-default-100 rounded-lg" />
  ),
});

interface DashboardProps {
  businessId: string;
  // M5b: currency + IANA timezone threaded down from the dashboard page (which
  // already holds the resolved business) so TodayLivePanel/LiveBills don't each
  // re-fetch getBusiness just to read these two fields. Optional — a caller
  // without the business in scope leaves them undefined and the panels fall
  // back to their own single fetch.
  currency?: string;
  businessTimezone?: string | null;
  /** Business address ISO country; threaded to bill drawers for fiscal capture. */
  country?: string | null;
  /** Wave 4: cross-tab drill-in (e.g. analytics item → Menu Builder). */
  onNavigateToTab?: (tab: string) => void;
}

const TAB_KEYS = ["today", "payments", "revenue", "menu", "service"] as const;
type AnalyticsSub = (typeof TAB_KEYS)[number];

export default function Dashboard({
  businessId,
  currency,
  businessTimezone,
  country,
  onNavigateToTab,
}: DashboardProps) {
  const { locale } = useSimpleLocale();

  const tString = (key: string, params?: Record<string, string | number>): string => {
    const fullKey = `businessDashboard.dashboard.${key}`;
    const result = getTranslation(fullKey, locale, params);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const tDash = (key: string, params?: Record<string, string | number>): string => {
    const fullKey = `businessDashboard.${key}`;
    const result = getTranslation(fullKey, locale, params);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const { hasAccess, loading: accessLoading } = useBusinessAccess(businessId);

  // Deep-link the active panel via &sub= only (Task 31). Unknown keys
  // surface not-found.
  const { sub, unknownSub, setSub } = useSubTab<AnalyticsSub>(
    "analytics",
    TAB_KEYS,
    "today",
  );
  const activeTab: AnalyticsSub = sub ?? "today";

  // Fix 4 + L6-2/L6-5: shared analytics period is URL-authoritative via
  // useUrlState. Invalid ?period= garbage normalizes to "today" and rewrites
  // the URL (replace). Cross-rail carry of period is handled in tabParams
  // (CROSS_RAIL_SHARED_PARAMS) for accounting.
  const [sharedPeriod, setSharedPeriod] = useUrlState({
    key: "period",
    valid: ANALYTICS_PERIOD_VALUES,
    fallback: "today" as AnalyticsPeriod,
    // Keep the default in the URL so a shared link is reproducible (mirrors
    // the sub-tab default-in-URL policy). omitDefault false.
    omitDefault: false,
  });

  const selectTab = (key: string) => {
    if (!(TAB_KEYS as readonly string[]).includes(key)) return;
    if (key === activeTab && !unknownSub) return;
    setSub(key as AnalyticsSub);
  };

  const handlePeriodChange = (key: string) => {
    if ((ANALYTICS_PERIOD_VALUES as readonly string[]).includes(key)) {
      setSharedPeriod(key as AnalyticsPeriod);
    }
  };
  const effectivePeriod = sharedPeriod;

  const tabs = [
    {
      key: "today",
      title: tString("tabs.todayLive"),
      icon: Clock,
      component: (
        <TodayLivePanel
          businessId={businessId}
          currency={currency}
          businessTimezone={businessTimezone}
          country={country}
        />
      ),
    },
    {
      key: "payments",
      title: tString("tabs.paymentHistory"),
      icon: Users,
      component: (
        <PaymentHistory
          businessId={businessId}
          currency={currency}
          businessTimezone={businessTimezone}
          country={country}
          // L6-2: pass shared period through (including yesterday). Dropping
          // yesterday as undefined used to render the local month default
          // while the URL still said period=yesterday.
          period={
            effectivePeriod === undefined
              ? undefined
              : (effectivePeriod as
                  | "today"
                  | "yesterday"
                  | "week"
                  | "month"
                  | "quarter"
                  | "year")
          }
          onPeriodChange={handlePeriodChange}
        />
      ),
    },
    {
      key: "revenue",
      title: tString("tabs.revenue"),
      icon: BarChart3,
      component: (
        <RevenuePanel
          businessId={businessId}
          currency={currency}
          period={effectivePeriod}
          onPeriodChange={handlePeriodChange}
        />
      ),
    },
    {
      key: "menu",
      title: tString("tabs.menu"),
      icon: Package,
      component: (
        <MenuPanel
          businessId={businessId}
          currency={currency}
          period={effectivePeriod}
          onPeriodChange={handlePeriodChange}
          onNavigateToTab={onNavigateToTab}
        />
      ),
    },
    {
      key: "service",
      title: tString("tabs.serviceTips"),
      icon: DollarSign,
      component: (
        <ServiceTipsPanel
          businessId={businessId}
          currency={currency}
          period={effectivePeriod}
          onPeriodChange={handlePeriodChange}
        />
      ),
    },
  ];

  if (unknownSub) {
    return (
      <DashboardTabShell
        header={{
          title: tString("title"),
          subtitle: tString("subtitle"),
        }}
      >
        <EmptyState
          panel
          icon={SearchX}
          title={tDash("error.subNotFound")}
          subtitle={tDash("error.subNotFoundBody", { key: unknownSub })}
          actionLabel={tDash("error.goToDefaultSub")}
          onAction={() => setSub("today")}
          data-testid="analytics-sub-not-found"
        />
      </DashboardTabShell>
    );
  }

  return (
    <DashboardTabShell
      locked={
        !accessLoading && !hasAccess ? (
          <DashboardLockedTabView
            title={tString("title")}
            subtitle={tString("subtitle")}
            businessId={businessId}
          />
        ) : null
      }
      loading={
        accessLoading ? (
          <DashboardTabLoadingSkeleton
            labelKey="loadingAnalytics"
            withPageChrome={false}
          />
        ) : null
      }
      header={{
        title: tString("title"),
        subtitle: tString("subtitle"),
      }}
      tabs={{
        items: tabs.map((tab) => ({
          key: tab.key,
          label: tab.title,
          icon: tab.icon,
        })),
        activeKey: activeTab,
        onChange: selectTab,
        ariaLabel: tString("title"),
      }}
    >
      <div className="min-w-0">
        <DashboardTabTransition tabKey={activeTab}>
          {tabs.find((tab) => tab.key === activeTab)?.component}
        </DashboardTabTransition>
        {/* Cost health (food / labor / waste) — revenue tab only (L6-4). */}
        {activeTab === "revenue" && (
          <CostAnalyticsSection
            businessId={businessId}
            currency={currency}
          />
        )}
      </div>
    </DashboardTabShell>
  );
}
