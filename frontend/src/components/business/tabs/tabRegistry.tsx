"use client";

/**
 * Operator dashboard tab registry — the single FE definition point for a rail.
 *
 * One entry per tab owns: the code-split loader (used by next/dynamic AND by
 * hover/idle prefetch), the rail icon, and the deep-link entitlement gate.
 * Membership/permissions stay in config/dashboard-tabs.json (backend source of
 * truth, mirrored at src/config/dashboard-tabs.json); presentation grouping and
 * ordering stay in sidebar/sidebarConfig.ts. Adding a tab = JSON row (+ mirror
 * copy) + one entry here + i18n keys. TabKey derives from this table so the
 * page switch, the sidebar map, and this registry cannot drift — #64 (fiscal
 * declared in the JSON but never wired to a rail) is the bug class this kills.
 */

import dynamic from "next/dynamic";
import {
  Banknote,
  BarChart3,
  Bot,
  Boxes,
  Calendar,
  CalendarClock,
  ChefHat,
  Coffee,
  CreditCard,
  FileText,
  Globe,
  Home,
  Megaphone,
  Printer as PrinterIcon,
  QrCode,
  Receipt,
  Settings,
  Stamp,
  TrendingUp,
  Truck,
  UserCircle,
  Users,
  Utensils,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import DashboardTabLoadingSkeleton from "@/components/business/shared/DashboardTabLoadingSkeleton";

type TabLoader = () => Promise<{ default: React.ComponentType<any> }>;

function TabSkeleton() {
  return <DashboardTabLoadingSkeleton />;
}

// ---------------------------------------------------------------------------
// Code-split rail panels. Each dynamic() keeps literal options inline — the
// Next SWC transform requires an analyzable options object (no shared var).
// The loader const is shared with prefetchTab below; importing the same
// specifier twice resolves to one webpack chunk, so prefetch and render never
// double-fetch.
// ---------------------------------------------------------------------------

const overviewLoader: TabLoader = () => import("../BusinessOverview");
export const BusinessOverview = dynamic(overviewLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const analyticsLoader: TabLoader = () => import("../../dashboard/Dashboard");
export const Dashboard = dynamic(analyticsLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const accountingLoader: TabLoader = () => import("../AccountingDashboard");
export const AccountingDashboard = dynamic(accountingLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const menuLoader: TabLoader = () => import("../MenuBuilder");
export const MenuBuilder = dynamic(menuLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const inventoryLoader: TabLoader = () => import("../InventoryManager");
export const InventoryManager = dynamic(inventoryLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const tablesLoader: TabLoader = () => import("../TableManager");
export const TableManager = dynamic(tablesLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const counterLoader: TabLoader = () => import("../CounterManager");
export const CounterManager = dynamic(counterLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const billsLoader: TabLoader = () =>
  import("../BillManager").then((m) => ({ default: m.BillManager }));
export const BillManager = dynamic(billsLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const cashRegisterLoader: TabLoader = () => import("../CashRegisterDashboard");
export const CashRegisterDashboard = dynamic(cashRegisterLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const scheduleLoader: TabLoader = () => import("../schedule/ScheduleBuilder");
export const ScheduleBuilder = dynamic(scheduleLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const kitchenLoader: TabLoader = () => import("../Kitchen");
export const Kitchen = dynamic(kitchenLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const staffLoader: TabLoader = () => import("../StaffManagement");
export const StaffManagement = dynamic(staffLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const crmLoader: TabLoader = () => import("../CRMManager");
export const CRMManager = dynamic(crmLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const reservationsLoader: TabLoader = () => import("../ReservationManager");
export const ReservationManager = dynamic(reservationsLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const pluginsLoader: TabLoader = () => import("../PluginManager");
export const PluginManager = dynamic(pluginsLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const settingsLoader: TabLoader = () => import("../BusinessSettings");
export const BusinessSettings = dynamic(settingsLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const printersLoader: TabLoader = () => import("../printers/PrintersSettings");
export const PrintersSettings = dynamic(printersLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const businessPageLoader: TabLoader = () => import("../BusinessPageEditor");
export const BusinessPageEditor = dynamic(businessPageLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const deliveryLoader: TabLoader = () => import("../delivery/DeliveryAdmin");
export const DeliveryAdmin = dynamic(deliveryLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const aiWaiterLoader: TabLoader = () => import("../AiWaiter/AiWaiterDashboard");
export const AiWaiterDashboard = dynamic(aiWaiterLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const directorConsoleLoader: TabLoader = () =>
  import("../DirectorConsole/DirectorConsoleDashboard");
export const DirectorConsoleDashboard = dynamic(directorConsoleLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

const marketingLoader: TabLoader = () => import("../Marketing");
export const MarketingDashboard = dynamic(marketingLoader, {
  ssr: false,
  loading: () => <TabSkeleton />,
});

// ---------------------------------------------------------------------------
// Registry table. `pageGated` marks tabs whose panel does not render its own
// admin-lock notice, so renderTabContent shows the shared one on a deep link
// while the business is suspended or closed.
// ---------------------------------------------------------------------------

export const TAB_REGISTRY = {
  overview: { icon: Home, loader: overviewLoader },
  bills: { icon: Receipt, loader: billsLoader },
  "cash-register": {
    icon: Banknote,
    loader: cashRegisterLoader,
    pageGated: true,
  },
  printers: { icon: PrinterIcon, loader: printersLoader },
  kitchen: { icon: ChefHat, loader: kitchenLoader },
  reservations: { icon: Calendar, loader: reservationsLoader },
  menu: { icon: Utensils, loader: menuLoader, pageGated: true },
  tables: { icon: QrCode, loader: tablesLoader, pageGated: true },
  delivery: { icon: Truck, loader: deliveryLoader },
  counter: { icon: Coffee, loader: counterLoader },
  "ai-waiter": { icon: Bot, loader: aiWaiterLoader },
  "director-console": { icon: BarChart3, loader: directorConsoleLoader },
  marketing: { icon: Megaphone, loader: marketingLoader },
  analytics: { icon: TrendingUp, loader: analyticsLoader },
  accounting: { icon: FileText, loader: accountingLoader },
  fiscal: { icon: Stamp, loader: accountingLoader },
  crm: { icon: UserCircle, loader: crmLoader },
  inventory: { icon: Boxes, loader: inventoryLoader },
  staff: { icon: Users, loader: staffLoader },
  schedule: {
    icon: CalendarClock,
    loader: scheduleLoader,
    pageGated: true,
  },
  "business-page": { icon: Globe, loader: businessPageLoader },
  plugins: { icon: CreditCard, loader: pluginsLoader },
  settings: { icon: Settings, loader: settingsLoader },
} as const satisfies Record<
  string,
  { icon: LucideIcon; loader: TabLoader; pageGated?: boolean }
>;

export type TabKey = keyof typeof TAB_REGISTRY;

/** Every rail key, including the standalone `printers` route. */
export const TAB_KEYS = Object.keys(TAB_REGISTRY) as TabKey[];

/**
 * Whether renderTabContent must show the shared admin-lock notice for a deep
 * link to this tab. Tabs that render their own notice return false.
 */
export function isDeepLinkPageGated(tab: string): boolean {
  const entry = (TAB_REGISTRY as Record<string, { pageGated?: boolean }>)[tab];
  return entry?.pageGated === true;
}

/**
 * Build a chunk prefetcher: one memoized, rejection-swallowed promise per key.
 * Exported separately from prefetchTab so the dedupe semantics are unit-tested
 * with fake loaders (Jest never imports real rail chunks).
 */
export function createTabPrefetcher(
  loaders: Record<string, () => Promise<unknown>>,
) {
  const cache = new Map<string, Promise<unknown>>();
  return (key: string): Promise<unknown> => {
    let p = cache.get(key);
    if (!p) {
      p = Promise.resolve()
        .then(() => loaders[key]())
        // A failed prefetch must never surface — the real dynamic() render
        // will retry the chunk on navigation.
        .catch(() => undefined);
      cache.set(key, p);
    }
    return p;
  };
}

const TAB_LOADERS: Record<TabKey, () => Promise<unknown>> = Object.fromEntries(
  TAB_KEYS.map((key) => [key, TAB_REGISTRY[key].loader]),
) as Record<TabKey, () => Promise<unknown>>;

/**
 * Warm a rail's webpack chunk before the operator clicks (sidebar hover/focus,
 * idle prefetch of TODAY tabs). Memoized + fail-silent; dynamic() reuses the
 * warmed chunk on navigation.
 */
export const prefetchTab = createTabPrefetcher(TAB_LOADERS) as (
  key: TabKey,
) => Promise<unknown>;
