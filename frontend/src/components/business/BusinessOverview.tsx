"use client";

import React, { useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  MapPin,
  DollarSign,
  Users,
  ArrowRight,
  BarChart3,
  QrCode,
  Receipt,
  RefreshCw,
  ChevronDown,
  ChefHat,
  Coffee,
  Globe,
  Coins,
  CreditCard,
  Plus,
  UtensilsCrossed,
  Boxes,
  type LucideIcon,
} from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { PLUGIN } from "@/constants/plugins";
import { Business, businessApi, getMenu, Table } from "@/api/business";
import { analyticsApi, DashboardSummary } from "@/api/analytics";
import { InventorySummary, inventoryApi } from "@/api/inventory";
import { useProactiveInsights } from "@/hooks/useProactiveInsights";
import { pluginAPI } from "@/api/plugins";
import { hasStaffPermission } from "@/utils/staffAuth";
import { resolveStaffTabs } from "@/constants/staffTabAccess";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { parseMenuCategories } from "@/utils/businessDataParsers";
import type { MenuCategory } from "@/api/business";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import Metric from "./overview/Metric";
import ProactiveInsights from "./overview/ProactiveInsights";
import { filterQuickActionsDuplicatingInsights } from "./overview/dedupeQuickActions";
import { welcomeSubtitleKey } from "./overview/welcomeSubtitleKey";
import { loadOverviewCoreWidgets } from "./overview/loadOverviewCoreWidgets";
import { isCatalogPluginOffered } from "./overview/catalogPluginOffered";
import StaleWidgetBanner from "./shared/StaleWidgetBanner";

export function inventoryAlertDescriptionKey(outOfStockCount: number): string {
  if (outOfStockCount === 0) return "descriptionOutZero";
  if (outOfStockCount === 1) return "descriptionOutOne";
  return "descriptionOutMany";
}
import OnboardingHub from "./onboarding/OnboardingHub";
import { BusinessLockNotice } from "./BusinessLockNotice";
import { CurrencyPrice } from "@/components/common/CurrencyConverter";
import { intlLocaleFor } from "@/utils/intlLocale";
import { formatCurrency } from "@/api/currency";
import { countKey } from "@/i18n/countForm";
import LiveTableGrid, { ActiveBillInfo } from "./staff/LiveTableGrid";
import ReservationsTodayCard from "./ReservationsTodayCard";
import PageHeader from "./shared/PageHeader";
import { PremiumPanel } from "./premium";
import DashboardTabLoadingSkeleton from "./shared/DashboardTabLoadingSkeleton";
import { useSiteUrl } from "@/hooks/useSiteUrl";
import { useInstance } from "@/hooks/useInstance";
import { isPublicDemo } from "@/lib/instance/instanceInfo";

interface BusinessOverviewProps {
  business: Business;
  onNavigateToTab?: (tab: string) => void;
  isStaffUser?: boolean;
  staffRole?: string;
  setupRefreshKey?: number;
}

type QuickAction = {
  key: string;
  icon: LucideIcon;
  title: string;
  description: string;
  tab: string;
  variant?: "urgent";
};

/** Hide extras behind a disclosure only when there are enough tiles to justify it. */
export const OVERVIEW_EXTRAS_DISCLOSURE_MIN = 3;

export default function BusinessOverview({
  business,
  onNavigateToTab,
  isStaffUser = false,
  staffRole,
  setupRefreshKey = 0,
}: BusinessOverviewProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  // Resolved BCP-47 tag so operator money grouping follows the operator locale.
  const intlLocale = intlLocaleFor(locale);
  const [currentLocale, setCurrentLocale] = useState(locale);
  const { permissions } = useStaffPermissionsContext();

  // Admin lifecycle lock (suspended / closed) and instance AI availability.
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
    aiConfigured,
  } = useBusinessAccess(business?.id);

  // Dashboard data state
  const [dashboardData, setDashboardData] = useState<DashboardSummary | null>(
    null,
  );
  const [tableCount, setTableCount] = useState<number>(0);
  const [tables, setTables] = useState<Table[]>([]);
  const [inventorySummary, setInventorySummary] =
    useState<InventorySummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [initialLoading, setInitialLoading] = useState(true);
  const [_error, setError] = useState<string | null>(null);
  // Tracks an analytics-summary fetch failure separately from "no data yet".
  // Without this a failed fetch (dashboardData === null) is indistinguishable
  // from a genuinely-empty day and shows the misleading "first sale of the day"
  // empty state. When true we show a "couldn't load, retry" hint instead.
  const [analyticsError, setAnalyticsError] = useState(false);
  // L9-2: tables fetch failure must not render as "0 tables".
  const [tablesError, setTablesError] = useState(false);
  // Bumped by the manual refresh button so ReservationsTodayCard refetches too.
  const [reservationsRefreshKey, setReservationsRefreshKey] = useState(0);
  /** F-cand-14: button+aria-expanded disclosure (not native details/summary). */
  const [moreMetricsOpen, setMoreMetricsOpen] = useState(false);

  // Fix 8: menu-emptiness for the quick actions. This is used ONLY as a boolean
  // ("does the menu have any items?" — drives the Add-menu-items nudge and the
  // basics-complete gate); it was never displayed as a number. It previously
  // downloaded the ENTIRE menu JSON inside the imperative fetch on every mount.
  // A React Query with a long staleTime caches it so an Overview revisit reuses
  // the last result instead of re-downloading the whole menu, and it stays out
  // of the critical-path effect. (Backend menu-summary endpoint is owned by the
  // Menu stream — deferred; caching removes the per-revisit cost meanwhile.)
  const menuCountQuery = useQuery({
    queryKey: ["overview", "menuItemCount", business?.id],
    enabled: !!business?.id && !isStaffUser,
    staleTime: 5 * 60 * 1000, // 5 min — menu size changes rarely relative to revisits
    queryFn: async () => {
      const menuResponse = await getMenu(business.id);
      const categories = parseMenuCategories(menuResponse);
      return categories.reduce(
        (acc: number, cat: MenuCategory) => acc + (cat.items?.length || 0),
        0,
      );
    },
  });
  // 0 until resolved (matches the prior initial value); staff never fetch it.
  const menuItemCount = menuCountQuery.data ?? 0;

  // Proactive briefings (AI-Growth-owner-only). The hook fetches on mount and
  // refetches in realtime on the stuck-bill watchdog's `bill.stuck` SSE event
  // (and on SSE reconnect), so the "N open bills over 2h old" alert stays fresh
  // instead of going stale until a tab remount.
  const { insights, loading: insightsLoading } = useProactiveInsights(
    business?.id,
    !isStaffUser && hasAccess && aiConfigured,
  );

  // Plugin states - default to false, only show quick actions after we confirm plugins are not enabled
  const [isUsdcEnabled, setIsUsdcEnabled] = useState(false);
  const [isCrossChainEnabled, setIsCrossChainEnabled] = useState(false);
  // False while the catalog marks cross-chain coming-soon (guests cannot
  // settle through it), so the overview never suggests enabling it.
  const [isCrossChainOffered, setIsCrossChainOffered] = useState(false);
  const [isCardRailEnabled, setIsCardRailEnabled] = useState(false);
  const [pluginsLoaded, setPluginsLoaded] = useState(false);
  const siteUrl = useSiteUrl();
  // The public demo cannot take real card payments, so it never nudges
  // visitors toward connecting a processor.
  const { instance } = useInstance();
  const isDemoInstance = isPublicDemo(instance);

  // Operational writes are refused while an administrator holds the lock.
  const isRestricted = !hasAccess;

  // Check if user can see specific stats (owners: full access; staff: effective perms)
  const canSeeAnalytics =
    !isStaffUser || hasStaffPermission(permissions, "analytics");
  const canSeeTables =
    !isStaffUser || hasStaffPermission(permissions, "tables");
  const canSeeBills =
    !isStaffUser || hasStaffPermission(permissions, "bills");
  const canSeeInventory =
    !isStaffUser || hasStaffPermission(permissions, "inventory");
  // Mirror the sidebar's gate: owners always see Reservations; staff only when
  // their effective permissions resolve the reservations tab.
  const canSeeReservations =
    !isStaffUser || resolveStaffTabs(permissions).includes("reservations");

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Fetch dashboard data based on role permissions
  useEffect(() => {
    const controller = new AbortController();
    const fetchDashboardData = async () => {
      if (!business?.id) return;

      try {
        setLoading(true);
        setError(null);

        // L9-2: real cancel-on-unmount — signal is passed into axios calls.
        const core = await loadOverviewCoreWidgets({
          businessId: business.id,
          canSeeAnalytics,
          canSeeTables,
          isRestricted,
          signal: controller.signal,
          deps: {
            getDashboardSummary: analyticsApi.getDashboardSummary,
            getBusinessTables: businessApi.getBusinessTables,
          },
        });
        if (controller.signal.aborted) return;

        setAnalyticsError(core.analyticsError);
        setTablesError(core.tablesError);
        setDashboardData(core.dashboard as DashboardSummary | null);
        setTableCount(core.tables.length);
        setTables(core.tables as Table[]);

        // Core vitals are ready — reveal the overview before slower owner-only
        // fetches (menu counts, inventory, plugin status) finish populating.
        setInitialLoading(false);

        // Independent owner-only fetches — dispatch concurrently. Each settles
        // through its own handler so one failure can't reject the batch. The
        // menu-item count moved to a cached React Query (see menuCountQuery
        // above) so it's no longer re-downloaded on every Overview revisit.
        const ownerFetches: Promise<void>[] = [];

        if (canSeeInventory) {
          ownerFetches.push(
            inventoryApi
              .getSummary(business.id)
              .then((nextInventorySummary) =>
                setInventorySummary(nextInventorySummary),
              )
              .catch((inventoryErr) => {
                console.error(
                  "Error fetching inventory summary:",
                  inventoryErr,
                );
                setInventorySummary(null);
              }),
          );
        }

        // Check plugin status for owners only
        // Use same approach as PluginManager - fetch platform plugins and merge with business plugins
        if (!isStaffUser && !isRestricted) {
          ownerFetches.push(
            Promise.all([
              pluginAPI.protected.getAllPlugins(),
              pluginAPI.business.getBusinessPlugins(business.id.toString()),
            ])
              .then(([platformResponse, businessResponse]) => {
                const platformPlugins = platformResponse.plugins || [];
                const businessPlugins = businessResponse.plugins || [];

                // Find USDC and cross-chain plugins from platform plugins
                const usdcPlatformPlugin = platformPlugins.find(
                  (p) => p.name === PLUGIN.usdcPayment,
                );
                const crossChainPlatformPlugin = platformPlugins.find(
                  (p) => p.name === PLUGIN.crossChainPayment,
                );

                // Check if they're enabled for this business
                const usdcBusinessPlugin = usdcPlatformPlugin
                  ? businessPlugins.find(
                      (bp) => bp.plugin_id === usdcPlatformPlugin.id,
                    )
                  : null;
                const crossChainBusinessPlugin = crossChainPlatformPlugin
                  ? businessPlugins.find(
                      (bp) => bp.plugin_id === crossChainPlatformPlugin.id,
                    )
                  : null;

                setIsUsdcEnabled(usdcBusinessPlugin?.is_enabled || false);
                const crossChainOffered = isCatalogPluginOffered(
                  crossChainPlatformPlugin,
                );
                setIsCrossChainOffered(crossChainOffered);
                setIsCrossChainEnabled(
                  crossChainOffered &&
                    Boolean(crossChainBusinessPlugin?.is_enabled),
                );
                const cardNames = new Set<string>([
                  PLUGIN.mercadopago,
                  PLUGIN.paypal,
                  PLUGIN.stripe,
                ]);
                setIsCardRailEnabled(
                  platformPlugins.some((plugin) => {
                    if (!cardNames.has(plugin.name)) return false;
                    return Boolean(
                      businessPlugins.find((bp) => bp.plugin_id === plugin.id)
                        ?.is_enabled,
                    );
                  }),
                );
                setPluginsLoaded(true);
              })
              .catch((pluginErr) => {
                console.error("Error fetching plugin status:", pluginErr);
                // Set pluginsLoaded to true even on error so we don't show stale state
                setPluginsLoaded(true);
              }),
          );
        }

        await Promise.all(ownerFetches);
      } catch (err) {
        console.error("Error fetching dashboard data:", err);
        // Don't set error state for null data - just use fallback values
        setDashboardData(null);
        setTableCount(0);
        setInventorySummary(null);
        setInitialLoading(false);
      } finally {
        setLoading(false);
      }
    };

    if (!accessLoading) {
      void fetchDashboardData();
    }
    // L9-2 UX: cancel in-flight overview widget requests on unmount / re-run.
    return () => {
      controller.abort();
    };
  }, [
    business?.id,
    canSeeAnalytics,
    canSeeTables,
    canSeeInventory,
    isStaffUser,
    accessLoading,
    isRestricted,
  ]);

  // Refresh function - role-aware
  const handleRefresh = async () => {
    if (!business?.id || loading) return;
    const controller = new AbortController();

    try {
      setLoading(true);
      setError(null);
      setReservationsRefreshKey((key) => key + 1);

      // L9-2: same cancel-capable path as the mount effect (no silent empty tables).
      const core = await loadOverviewCoreWidgets({
        businessId: business.id,
        canSeeAnalytics,
        canSeeTables,
        isRestricted,
        signal: controller.signal,
        deps: {
          getDashboardSummary: analyticsApi.getDashboardSummary,
          getBusinessTables: businessApi.getBusinessTables,
        },
      });

      setAnalyticsError(core.analyticsError);
      setTablesError(core.tablesError);
      setDashboardData(core.dashboard as DashboardSummary | null);
      setTableCount(core.tables.length);
      setTables(core.tables as Table[]);
      // Refresh the cached menu-item count too (owners only) so a manual refresh
      // re-checks whether the menu is still empty.
      if (!isStaffUser) void menuCountQuery.refetch();
      if (canSeeInventory) {
        try {
          const nextInventorySummary = await inventoryApi.getSummary(
            business.id,
          );
          setInventorySummary(nextInventorySummary);
        } catch (inventoryErr) {
          console.error("Error refreshing inventory summary:", inventoryErr);
          setInventorySummary(null);
        }
      }
    } catch (err) {
      console.error("Error refreshing dashboard data:", err);
      // Don't set error state for null data - just use fallback values
      setDashboardData(null);
      setTableCount(0);
      setInventorySummary(null);
    } finally {
      setLoading(false);
    }
  };

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.overview.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const tWith = (
    key: string,
    replacements: Record<string, string | number>,
  ) => {
    let value = tString(key);
    Object.entries(replacements).forEach(([name, replacement]) => {
      value = value.replace(
        new RegExp(`\\{${name}\\}`, "g"),
        String(replacement),
      );
    });
    return value;
  };

  const activeBillsByTableId = (dashboardData?.live?.active_bills_by_table ??
    {}) as Record<number, ActiveBillInfo>;

  const visibleLowStockCount =
    inventorySummary?.settings?.low_stock_warnings_enabled === false
      ? 0
      : (inventorySummary?.low_stock_items ?? 0);
  // Role-specific content helpers
  const getWelcomeMessage = () => {
    if (!isStaffUser) return tString("welcome.title");

    switch (staffRole) {
      case "kitchen":
        return tString("roleSpecific.kitchen.welcome.title");
      case "host":
        return tString("roleSpecific.host.welcome.title");
      case "server":
        return tString("roleSpecific.server.welcome.title");
      case "manager":
        return tString("roleSpecific.manager.welcome.title");
      default:
        return tString("welcome.title");
    }
  };

  // #795: the owner subtitle only claims "ready to accept payments" when the
  // backend plugin flags confirm at least one enabled payment rail; otherwise
  // (rails off, flags still loading, restricted tier) it stays neutral.
  const getWelcomeSubtitle = () =>
    tString(
      welcomeSubtitleKey({
        isStaffUser: Boolean(isStaffUser),
        staffRole,
        paymentRailsKnown: pluginsLoaded,
        anyPaymentRailEnabled:
          isCardRailEnabled || isUsdcEnabled || isCrossChainEnabled,
      }),
    );

  // Get role-specific quick actions
  const getRoleSpecificActions = () => {
    if (!isStaffUser) {
      // Owner/Manager gets all actions
      const actions: QuickAction[] = [];

      // While locked by an administrator, show only setup actions: tables, menu
      if (!accessLoading && !hasAccess) {
        // 1. Add tables if none exist
        if (!loading && tableCount === 0) {
          actions.push({
            key: "add-tables",
            icon: Plus,
            title: tString("quickActions.addTables.title"),
            description: tString("quickActions.addTables.description"),
            tab: "tables",
          });
        }

        // 2. Add menu items if none exist
        if (!loading && menuItemCount === 0) {
          actions.push({
            key: "add-menu-items",
            icon: UtensilsCrossed,
            title: tString("quickActions.addMenuItems.title"),
            description: tString("quickActions.addMenuItems.description"),
            tab: "menu",
          });
        }

        // Return early while locked - setup actions only
        return actions.slice(0, 3); // Max 3 actions
      }

      // Task 33 priority: finish setup / add a table / print a QR first.
      // Cross-chain and USDC are discoverable advanced add-ons — never primary.
      // Gate on initialLoading (vitals ready), not `loading` — the latter stays
      // true until plugins finish and would hide setup actions forever when the
      // plugin fetch hangs.

      // 1. Add tables if none exist
      if (!initialLoading && tableCount === 0) {
        actions.push({
          key: "add-tables",
          icon: Plus,
          title: tString("quickActions.addTables.title"),
          description: tString("quickActions.addTables.description"),
          tab: "tables",
        });
      }

      // 2. Add menu items if none exist
      if (!initialLoading && menuItemCount === 0) {
        actions.push({
          key: "add-menu-items",
          icon: UtensilsCrossed,
          title: tString("quickActions.addMenuItems.title"),
          description: tString("quickActions.addMenuItems.description"),
          tab: "menu",
        });
      }

      // 3. Print table QR codes once tables exist (the next setup step after
      // "add a table" — not a crypto payment plugin).
      if (!initialLoading && tableCount > 0) {
        actions.push({
          key: "print-qr",
          icon: QrCode,
          title: tString("quickActions.printQr.title"),
          description: tString("quickActions.printQr.description"),
          tab: "tables",
        });
      }

      // Card/local checkout before crypto. Do not auto-enable — the Plugins
      // tab still requires Mercado Pago (or Stripe/PayPal) credentials (#202).
      if (pluginsLoaded && !isCardRailEnabled && !isDemoInstance) {
        actions.push({
          key: "enable-card-payments",
          icon: CreditCard,
          title: tString("quickActions.enableCardPayments.title"),
          description: tString("quickActions.enableCardPayments.description"),
          tab: "plugins",
        });
      }

      if (
        canSeeInventory &&
        inventorySummary &&
        (visibleLowStockCount > 0 || inventorySummary.out_of_stock_items > 0)
      ) {
        actions.push({
          key: "inventory-alerts",
          icon: Boxes,
          title: tString("quickActions.inventoryAlerts.title"),
          description: tWith(
            `quickActions.inventoryAlerts.${inventoryAlertDescriptionKey(
              inventorySummary.out_of_stock_items,
            )}`,
            {
              out: inventorySummary.out_of_stock_items,
              low: visibleLowStockCount,
            },
          ),
          tab: "inventory",
          variant: "urgent" as const,
        });
      }

      // Dynamic: surface active bills when there are any AND at least one
      // is recent (within the last 24h). We don't have per-bill timestamps
      // here, but `active_bills_by_table` exposes `oldest_minutes` per
      // table. If ANY table's oldest open bill is younger than 24h, every
      // bill on that table is necessarily within 24h too — so showing the
      // action is justified. If every table's oldest open bill is stale,
      // the demo (or restaurant) likely just hasn't closed week-old test
      // bills, and nagging the operator is misleading. When the by-table
      // breakdown is absent we fall back to the legacy behavior of showing
      // the action whenever `active_bills > 0`.
      const activeBillsCount = dashboardData?.live?.active_bills ?? 0;
      const activeBillsByTable = dashboardData?.live?.active_bills_by_table;
      const RECENT_BILL_WINDOW_MINUTES = 24 * 60;
      const hasRecentActiveBill =
        activeBillsByTable && Object.keys(activeBillsByTable).length > 0
          ? Object.values(activeBillsByTable).some(
              (info) =>
                info != null &&
                typeof info.oldest_minutes === "number" &&
                info.oldest_minutes < RECENT_BILL_WINDOW_MINUTES,
            )
          : true; // unknown breakdown → preserve legacy behavior
      if (activeBillsCount > 0 && hasRecentActiveBill) {
        actions.push({
          key: "active-bills",
          icon: Receipt,
          title: tString("quickActions.activeBills.title"),
          description: tWith(
            countKey("quickActions.activeBills.description", activeBillsCount),
            {
              count: activeBillsCount,
            },
          ),
          tab: "bills",
          variant: activeBillsCount >= 3 ? ("urgent" as const) : undefined,
        });
      }

      // 4. Evergreen setup/ops suggestions before advanced crypto.
      if (actions.length < 3) {
        const evergreen: QuickAction[] = [
          {
            key: "business-page",
            icon: Globe,
            title: tString("quickActions.businessPage.title"),
            description: tWith("quickActions.businessPage.description", {
              host: siteUrl.replace(/^https?:\/\//, ""),
            }),
            tab: "business-page",
          },
          {
            key: "menu",
            icon: UtensilsCrossed,
            title: tString("quickActions.manageMenu.title"),
            description: tString("quickActions.manageMenu.description"),
            tab: "menu",
          },
          {
            key: "analytics",
            icon: BarChart3,
            title: tString("quickActions.viewAnalytics.title"),
            description: tString("quickActions.viewAnalytics.description"),
            tab: "analytics",
          },
        ];
        actions.push(...evergreen.slice(0, 3 - actions.length));
      }

      // 5. Crypto only fills remaining slots after a card rail exists — never leads.
      const basicsComplete =
        !initialLoading && tableCount > 0 && menuItemCount > 0;
      if (
        actions.length < 3 &&
        basicsComplete &&
        pluginsLoaded &&
        isCardRailEnabled &&
        !isUsdcEnabled
      ) {
        actions.push({
          key: "enable-usdc",
          icon: Coins,
          title: tString("quickActions.enableUsdc.title"),
          description: tString("quickActions.enableUsdc.description"),
          tab: "plugins",
        });
      }
      if (
        actions.length < 3 &&
        basicsComplete &&
        pluginsLoaded &&
        isCardRailEnabled &&
        isCrossChainOffered &&
        !isCrossChainEnabled
      ) {
        actions.push({
          key: "enable-crosschain",
          icon: Globe,
          title: tString("quickActions.enableCrossChain.title"),
          description: tString("quickActions.enableCrossChain.description"),
          tab: "plugins",
        });
      }

      return filterQuickActionsDuplicatingInsights(actions, insights).slice(
        0,
        3,
      );
    }

    // Role-specific actions for staff
    switch (staffRole) {
      case "kitchen":
        return [
          {
            key: "kitchen",
            icon: ChefHat,
            title: tString("roleSpecificActions.kitchen.kitchenOrders.title"),
            description: tString(
              "roleSpecificActions.kitchen.kitchenOrders.description",
            ),
            tab: "kitchen",
          },
          {
            key: "menu",
            icon: Coffee,
            title: tString("roleSpecificActions.kitchen.viewMenu.title"),
            description: tString(
              "roleSpecificActions.kitchen.viewMenu.description",
            ),
            tab: "menu",
          },
          {
            key: "bills",
            icon: Receipt,
            title: tString("roleSpecificActions.kitchen.orderStatus.title"),
            description: tString(
              "roleSpecificActions.kitchen.orderStatus.description",
            ),
            tab: "bills",
          },
        ];

      case "host":
        return [
          {
            key: "tables",
            icon: MapPin,
            title: tString("roleSpecificActions.host.manageTables.title"),
            description: tString(
              "roleSpecificActions.host.manageTables.description",
            ),
            tab: "tables",
          },
          {
            key: "menu",
            icon: QrCode,
            title: tString("roleSpecificActions.host.viewMenu.title"),
            description: tString(
              "roleSpecificActions.host.viewMenu.description",
            ),
            tab: "menu",
          },
          {
            key: "bills",
            icon: Receipt,
            title: tString("roleSpecificActions.host.guestBills.title"),
            description: tString(
              "roleSpecificActions.host.guestBills.description",
            ),
            tab: "bills",
          },
        ];

      case "server":
        return [
          {
            key: "tables",
            icon: MapPin,
            title: tString("roleSpecificActions.server.myTables.title"),
            description: tString(
              "roleSpecificActions.server.myTables.description",
            ),
            tab: "tables",
          },
          {
            key: "bills",
            icon: Receipt,
            title: tString("roleSpecificActions.server.createBills.title"),
            description: tString(
              "roleSpecificActions.server.createBills.description",
            ),
            tab: "bills",
          },
          {
            key: "counter",
            icon: Coffee,
            title: tString("roleSpecificActions.server.counterService.title"),
            description: tString(
              "roleSpecificActions.server.counterService.description",
            ),
            tab: "counter",
          },
        ];

      case "manager":
        return [
          ...(canSeeInventory && inventorySummary
            ? [
                {
                  key: "inventory",
                  icon: Boxes,
                  title: tString("roleSpecificActions.manager.inventory.title"),
                  description:
                    inventorySummary.out_of_stock_items > 0 ||
                    visibleLowStockCount > 0
                      ? tWith(
                          "roleSpecificActions.manager.inventory.descriptionAlerts",
                          {
                            out: inventorySummary.out_of_stock_items,
                            low: visibleLowStockCount,
                          },
                        )
                      : tString(
                          "roleSpecificActions.manager.inventory.descriptionDefault",
                        ),
                  tab: "inventory",
                },
              ]
            : []),
          {
            key: "analytics",
            icon: BarChart3,
            title: tString("quickActions.viewAnalytics.title"),
            description: tString("quickActions.viewAnalytics.description"),
            tab: "analytics",
          },
          {
            key: "staff",
            icon: Users,
            title: tString("roleSpecificActions.manager.manageStaff.title"),
            description: tString(
              "roleSpecificActions.manager.manageStaff.description",
            ),
            tab: "staff",
          },
          {
            key: "menu",
            icon: QrCode,
            title: tString("quickActions.manageMenu.title"),
            description: tString("quickActions.manageMenu.description"),
            tab: "menu",
          },
        ];

      default:
        return [
          {
            key: "menu",
            icon: QrCode,
            title: tString("roleSpecificActions.default.viewMenu.title"),
            description: tString(
              "roleSpecificActions.default.viewMenu.description",
            ),
            tab: "menu",
          },
        ];
    }
  };

  if (initialLoading || accessLoading) {
    return (
      <div className="min-h-full bg-transparent">
        <DashboardTabLoadingSkeleton variant="overview" />
      </div>
    );
  }

  return (
    <div className="min-h-full bg-transparent">
      {!isStaffUser && isSuspended ? (
        <div className="container mx-auto px-4 pt-4 sm:px-6">
          <BusinessLockNotice businessId={business.id} />
        </div>
      ) : null}

      <section
        data-testid="business-overview-content"
        className="mx-auto max-w-screen-2xl space-y-5 p-4 sm:p-6"
      >
        <PageHeader
          title={`${getWelcomeMessage()} ${business.name}`}
          subtitle={getWelcomeSubtitle()}
          actions={
            <>
              <button
                onClick={handleRefresh}
                disabled={loading}
                className="inline-flex h-10 flex-shrink-0 items-center justify-center gap-2 rounded-full border border-warm-300 px-3.5 text-sm font-medium text-ink-600 transition-colors hover:bg-warm-50 disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                title={tString("welcome.refreshData")}
                aria-label={tString("welcome.refreshData")}
              >
                <RefreshCw
                  className={`h-4 w-4 ${loading ? "animate-spin" : ""}`}
                />
                <span>{tString("welcome.refreshData")}</span>
              </button>
            </>
          }
        />

        {!isStaffUser && business?.id && (
          <OnboardingHub
            businessId={business.id}
            businessName={business.name || ""}
            onboardingCompletedAt={business.onboarding_completed_at ?? null}
            refreshKey={setupRefreshKey}
            onNavigateToTab={(tab) => onNavigateToTab?.(tab)}
            firstTableCode={tables[0]?.table_code ?? null}
          />
        )}

        {/* Proactive Insights briefings — owners, when AI is configured. Sits below
            the welcome (warmer hierarchy) but above stats so the operator
            sees what Sage flagged before scanning numbers. */}
        {!isStaffUser && hasAccess && aiConfigured ? (
          <ProactiveInsights
            insights={insights}
            loading={insightsLoading}
            t={(key, params) => (params ? tWith(key, params) : tString(key))}
            onOpenConsole={() => onNavigateToTab?.("director-console")}
          />
        ) : null}

        {staffRole === "server" && canSeeTables ? (
          <div>
            <PremiumPanel
                as="section"
                className="p-4 sm:p-5"
                aria-labelledby="overview-server-floor-title"
              >
                <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
                  <div>
                    <p className="text-label uppercase text-ink-500">
                      {tString("roleSpecific.server.tablesLabel")}
                    </p>
                    <h2
                      id="overview-server-floor-title"
                      className="text-base font-semibold text-ink-950 mt-1"
                    >
                      {tString("roleSpecific.server.tablesTitle")}
                    </h2>
                  </div>
                  <button
                    onClick={() => onNavigateToTab?.("counter")}
                    className="inline-flex min-h-11 items-center justify-center rounded-xl bg-brand px-5 py-2.5 text-sm font-semibold text-white shadow-sm shadow-brand/15 transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
                  >
                    {tString("roleSpecific.server.counterShortcut")}
                  </button>
                </div>
                <LiveTableGrid
                  tables={tables}
                  activeBillsByTableId={activeBillsByTableId}
                  onStartNewOrder={(tableId) =>
                    onNavigateToTab?.(`bills?tableId=${tableId}`)
                  }
                  t={tString}
                />
              </PremiumPanel>
            </div>
          ) : (
            <section aria-label={tString("vitals.badge")} className="space-y-3">
              {/* Uniform 4-column stat grid — every card the same size + variant
                  so the row feels balanced. We label each card's time scope
                  explicitly via `hint` so "Today" and "7 days" never get mixed
                  in a row without a label. When the analytics view has zero
                  activity today we surface a friendly empty-state hint instead
                  of bare "AED 0.00 / 0" so a brand-new business doesn't read as
                  dead. We intentionally skip `delta` here because
                  DashboardSummary only exposes today + last-7-days totals (no
                  previous-7-days series), so a week-over-week percent change
                  would be fabricated. */}
                {(() => {
                  const todayHint = tString("hintToday") || "Today";
                  const weekHint = tString("thisWeek") || "This week";
                  const emptyHint =
                    tString("emptyTodayMessage") ||
                    "First sale of the day will appear here";
                  const errorHint =
                    tString("analyticsLoadErrorHint") ||
                    "Couldn't load — tap refresh to retry";
                  const todayRevenue =
                    dashboardData?.today?.collected_revenue ??
                    dashboardData?.today?.revenue ??
                    0;
                  const todayBills = dashboardData?.today?.bills ?? 0;
                  const floorRemaining =
                    dashboardData?.today?.floor_remaining ?? 0;
                  const openTables = dashboardData?.live?.open_tables ?? 0;
                  const untabledBills =
                    dashboardData?.live?.untabled_bills ?? 0;
                  const activeBillsHint =
                    untabledBills > 0
                      ? tWith("hintLiveSplit", {
                          tables: openTables,
                          delivery: untabledBills,
                        })
                      : tString("hintLive") || "Live";
                  // A failed fetch must not read as an empty sales day. When the
                  // analytics summary errored, show a retry hint instead of the
                  // "first sale of the day" empty-state copy.
                  const todaySalesHint = analyticsError
                    ? errorHint
                    : canSeeAnalytics && todayRevenue === 0 && todayBills === 0
                      ? emptyHint
                      : floorRemaining > 0
                        ? tWith("hintRemainingOnFloor", {
                            amount: formatCurrency(
                              floorRemaining,
                              business.default_currency || "USD",
                              undefined,
                              intlLocale,
                            ),
                          })
                        : todayHint;
                  return (
                    <div className="space-y-3">
                      <div
                        data-testid="overview-secondary-strip"
                        role="list"
                        className={`grid grid-cols-1 sm:grid-cols-2 gap-3 ${
                          canSeeAnalytics
                            ? "xl:grid-cols-3"
                            : "xl:grid-cols-4"
                        }`}
                      >
                        {canSeeAnalytics ? (
                          <>
                            <div role="listitem" className="min-w-0">
                              <Metric
                                size="card"
                                variant="accent"
                                label={tString("todaySales")}
                                value={
                                  <CurrencyPrice
                                    amount={todayRevenue}
                                    fromCurrency={
                                      business.default_currency || "USD"
                                    }
                                    displayCurrency={
                                      business.display_currency || "USD"
                                    }
                                    locale={intlLocale}
                                  />
                                }
                                hint={todaySalesHint}
                                loading={loading}
                                onClick={() => onNavigateToTab?.("analytics")}
                              />
                            </div>
                            {canSeeBills && (
                              <div role="listitem" className="min-w-0">
                                <Metric
                                  size="card"
                                  label={tString("activeBills")}
                                  value={String(
                                    dashboardData?.live?.active_bills ?? 0,
                                  )}
                                  hint={activeBillsHint}
                                  loading={loading}
                                  onClick={() => onNavigateToTab?.("bills")}
                                />
                              </div>
                            )}
                            <div role="listitem" className="min-w-0">
                              <Metric
                                size="card"
                                label={tString("todayOrders")}
                                value={String(
                                  dashboardData?.today?.order_count ?? todayBills,
                                )}
                                hint={todayHint}
                                loading={loading}
                                onClick={() => onNavigateToTab?.("analytics")}
                              />
                            </div>
                          </>
                        ) : (
                          <>
                            {/* Both tiles read the analytics summary, which
                                this branch never fetches (no overview:kpi).
                                Rendering them would show a confident 0 while
                                orders are pending, so they appear only when
                                that summary is actually loaded. */}
                            {canSeeBills && dashboardData && (
                              <div role="listitem" className="min-w-0">
                                <Metric
                                  size="card"
                                  variant="accent"
                                  label={tString("activeBills")}
                                  value={String(
                                    dashboardData?.live?.active_bills ?? 0,
                                  )}
                                  hint={activeBillsHint}
                                  loading={loading}
                                  onClick={() => onNavigateToTab?.("bills")}
                                />
                              </div>
                            )}
                            {canSeeBills && dashboardData && (
                              <div role="listitem" className="min-w-0">
                                <Metric
                                  size="card"
                                  label={tString("todayOrders")}
                                  value={String(
                                    dashboardData?.today?.order_count ??
                                      todayBills,
                                  )}
                                  hint={todayHint}
                                  loading={loading}
                                />
                              </div>
                            )}
                            {canSeeTables && (
                              <div role="listitem" className="min-w-0">
                                {tablesError ? (
                                  <StaleWidgetBanner
                                    onRetry={() => void handleRefresh()}
                                  />
                                ) : (
                                  <Metric
                                    size="card"
                                    label={tString("tables")}
                                    value={String(tableCount)}
                                    hint={tString("hintLive") || "Live"}
                                    loading={loading}
                                    onClick={() => onNavigateToTab?.("tables")}
                                  />
                                )}
                              </div>
                            )}
                            {dashboardData?.today?.by_current_staff ? (
                              <div role="listitem" className="min-w-0">
                                <Metric
                                  size="card"
                                  label={tString("myShift.billsCreated")}
                                  value={String(
                                    dashboardData.today.by_current_staff
                                      .bills_created,
                                  )}
                                  hint={tString("hintMyShift") || "My shift"}
                                  loading={loading}
                                />
                              </div>
                            ) : canSeeInventory ? (
                              <div role="listitem" className="min-w-0">
                                <Metric
                                  size="card"
                                  label={tString("inventory.title")}
                                  value={String(
                                    (inventorySummary?.out_of_stock_items ?? 0) +
                                      visibleLowStockCount,
                                  )}
                                  hint={tString("hintLive") || "Live"}
                                  loading={loading}
                                  onClick={() => onNavigateToTab?.("inventory")}
                                />
                              </div>
                            ) : staffRole === "kitchen" ? (
                              <div role="listitem" className="min-w-0">
                                <Metric
                                  size="card"
                                  label={tString(
                                    "roleSpecific.kitchen.stats.kitchenStatus",
                                  )}
                                  value={tString(
                                    "roleSpecific.kitchen.stats.statusReady",
                                  )}
                                  hint={tString("hintLive") || "Live"}
                                  loading={loading}
                                />
                              </div>
                            ) : null}
                          </>
                        )}
                      </div>
                      {canSeeAnalytics ? (
                        <div
                          data-testid="overview-week-metrics"
                          className="rounded-2xl border border-warm-200/80 bg-warm-50/60 p-3 sm:p-4"
                        >
                          <p className="mb-3 text-label uppercase text-ink-500">
                            {tString("thisWeek") || "This week"}
                          </p>
                          <div role="list" className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
                            <div role="listitem" className="min-w-0 sm:col-span-1">
                              <Metric
                                size="card"
                                label={tString("avgTicket") || "Avg Ticket"}
                                value={
                                  <CurrencyPrice
                                    amount={
                                      dashboardData?.week?.average_ticket ?? 0
                                    }
                                    fromCurrency={
                                      business.default_currency || "USD"
                                    }
                                    displayCurrency={
                                      business.display_currency || "USD"
                                    }
                                    locale={intlLocale}
                                  />
                                }
                                hint={weekHint}
                                loading={loading}
                                onClick={() => onNavigateToTab?.("analytics")}
                              />
                            </div>
                          </div>
                        </div>
                      ) : null}
                    </div>
                  );
                })()}

                {/* Additional metrics. Two-or-fewer tiles stay visible (#70 /
                  #229) — a nested "More metrics" disclosure is too weak for
                  that. Three-or-more use a chevron disclosure with an obvious
                  collapse control. */}
                {(() => {
                  const extras: React.ReactNode[] = [];
                  if (canSeeAnalytics && canSeeTables) {
                    extras.push(
                      <Metric
                        key="tables"
                        size="inline"
                        label={tString("tables")}
                        value={String(tableCount)}
                        icon={<Users className="w-4 h-4" />}
                        loading={loading}
                        onClick={() => onNavigateToTab?.("tables")}
                      />,
                    );
                  }
                  if (canSeeAnalytics && canSeeInventory) {
                    extras.push(
                      <Metric
                        key="inventory"
                        size="inline"
                        label={tString("inventory.title")}
                        value={String(
                          (inventorySummary?.out_of_stock_items ?? 0) +
                            visibleLowStockCount,
                        )}
                        icon={<Boxes className="w-4 h-4" />}
                        loading={loading}
                        onClick={() => onNavigateToTab?.("inventory")}
                      />,
                    );
                  }
                  if (dashboardData?.today?.by_current_staff) {
                    extras.push(
                      <Metric
                        key="my-tips"
                        size="inline"
                        label={tString("myShift.myTips")}
                        value={
                          <CurrencyPrice
                            amount={
                              dashboardData.today.by_current_staff
                                .tips_from_paid
                            }
                            fromCurrency={business.default_currency || "USD"}
                            displayCurrency={business.display_currency || "USD"}
                            locale={intlLocale}
                          />
                        }
                        icon={<DollarSign className="w-4 h-4" />}
                        loading={loading}
                      />,
                    );
                    // Only surface "bills created" in extras when the primary
                    // strip already consumed its slot (i.e., analytics view).
                    if (canSeeAnalytics) {
                      extras.push(
                        <Metric
                          key="my-bills"
                          size="inline"
                          label={tString("myShift.billsCreated")}
                          value={String(
                            dashboardData.today.by_current_staff.bills_created,
                          )}
                          icon={<Receipt className="w-4 h-4" />}
                          loading={loading}
                        />,
                      );
                    }
                  }
                  if (extras.length === 0) return null;
                  if (extras.length < OVERVIEW_EXTRAS_DISCLOSURE_MIN) {
                    return (
                      <div
                        data-testid="overview-more-metrics-inline"
                        className="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-3"
                      >
                        {extras}
                      </div>
                    );
                  }
                  return (
                    <div className="mt-3 rounded-2xl border border-warm-200 bg-warm-50/80">
                      <button
                        type="button"
                        data-testid="overview-more-metrics"
                        aria-expanded={moreMetricsOpen}
                        aria-controls="overview-more-metrics-panel"
                        aria-label={
                          moreMetricsOpen
                            ? tString("additionalMetricsHide") || "Show less"
                            : tString("additionalMetrics") || "More metrics"
                        }
                        onClick={() => setMoreMetricsOpen((open) => !open)}
                        className="flex w-full cursor-pointer select-none items-center justify-between gap-3 rounded-2xl px-4 py-3 text-left text-sm font-medium text-ink-700 hover:bg-warm-100/80 hover:text-ink-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                      >
                        <span>
                          {moreMetricsOpen
                            ? tString("additionalMetricsHide") || "Show less"
                            : tString("additionalMetrics") || "More metrics"}
                        </span>
                        <ChevronDown
                          aria-hidden
                          data-testid="overview-more-metrics-chevron"
                          className={`h-5 w-5 flex-shrink-0 text-ink-500 transition-transform duration-200 ${
                            moreMetricsOpen ? "rotate-180" : ""
                          }`}
                        />
                      </button>
                      {moreMetricsOpen ? (
                        <div
                          id="overview-more-metrics-panel"
                          className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3 border-t border-warm-200 px-4 py-4"
                        >
                          {extras}
                        </div>
                      ) : null}
                    </div>
                  );
                })()}
            </section>
          )}

          {/* Today's reservations — compact summary composed client-side from
              the reservations list endpoint; links into the Reservations tab. */}
          {canSeeReservations && business?.id ? (
            <ReservationsTodayCard
              businessId={business.id}
              businessTimezone={business.timezone}
              refreshKey={reservationsRefreshKey}
              onNavigate={() => onNavigateToTab?.("reservations")}
            />
          ) : null}

          {/* Quick actions — one calm list, no marketing headline. */}
          <section aria-labelledby="overview-quick-actions-title">
            <h2
              id="overview-quick-actions-title"
              className="text-sm font-semibold text-ink-900"
            >
              {tString("quickActions.badge")}
            </h2>
            <PremiumPanel withTexture={false} className="mt-3 overflow-hidden">
              <div className="divide-y divide-warm-200/70">
                {getRoleSpecificActions().map((action) => {
                  const IconComponent = action.icon;
                  const urgent = action.variant === "urgent";
                  return (
                    <button
                      key={action.key}
                      type="button"
                      onClick={() => onNavigateToTab?.(action.tab)}
                      className="group flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-warm-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand"
                    >
                      <IconComponent
                        className={`h-5 w-5 flex-shrink-0 ${
                          urgent ? "text-amber-600" : "text-brand"
                        }`}
                        aria-hidden="true"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block text-sm font-medium text-ink-900">
                          {action.title}
                        </span>
                        <span className="block truncate text-sm text-ink-500">
                          {action.description}
                        </span>
                      </span>
                      <ArrowRight
                        className="h-4 w-4 flex-shrink-0 text-ink-400 transition-transform group-hover:translate-x-0.5"
                        aria-hidden="true"
                      />
                    </button>
                  );
                })}
              </div>
            </PremiumPanel>
          </section>
      </section>
    </div>
  );
}
