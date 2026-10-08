"use client";

import React from "react";
import {
  Button,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
} from "@nextui-org/react";
import Image from "next/image";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import {
  X,
  Calendar,
  Bot,
  ChevronsLeft,
  ChevronsRight,
  Lock,
  ArrowLeft,
  HelpCircle,
  MoreHorizontal,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { TAB_REGISTRY, prefetchTab } from "./tabs/tabRegistry";
// GitHub / Telegram / WhatsApp brand marks have no faithful lucide equivalent,
// so they ship as dependency-free inline SVG glyphs (simple-icons paths).
import {
  GithubIcon,
  TelegramIcon,
  WhatsappIcon,
} from "@/components/icons/brands";
import { brandLinks } from "@/config/brand";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { RAIL_LABEL_KEYS } from "@/i18n/railDisplayNames";
import { Business } from "@/api/business";
import { StaffData } from "@/utils/staffAuth";

import { Reservation } from "@/api/reservations";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import {
  PRIMARY_TABS,
  PRIMARY_TAB_GROUPS,
  SECONDARY_TABS,
  type TabKey,
} from "./sidebar/sidebarConfig";
import { useLocalStorageState } from "@/hooks/useLocalStorageState";
import { countKey } from "@/i18n/countForm";
import SimpleLanguageSwitcher from "@/components/SimpleLanguageSwitcher";
// Operational/role lock logic is shared with the ⌘K command palette via this module,
// so the sidebar and the palette can never offer divergent navigation.
import { getTabLockMeta as getSharedTabLockMeta } from "./commandPalette/tabAccess";
import { capBadge, sumBadges } from "./sidebar/railBadge";
import CommandPaletteTrigger from "./commandPalette/CommandPaletteTrigger";
import RecentAlertsPopover from "./operational-alerts/RecentAlertsPopover";
import { useOptionalOperationalAlerts } from "./operational-alerts/useOperationalAlerts";
import { useRailBadges } from "./sidebar/useRailBadges";
import { AnimatedBadge } from "./premium";
import { SidebarNavRow } from "./sidebar/SidebarNavRow";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { SidebarTooltip } from "./sidebar/SidebarTooltip";
import { useInstance } from "@/hooks/useInstance";
import { instanceOffFeatureForTab } from "@/lib/instance/featureGates";
import { navigateToVenuesOverview } from "@/utils/businessUrl";

/** Tabs that need an LLM; the sidebar's "AI" header is only shown with one. */
const AI_MODEL_TABS: ReadonlySet<string> = new Set(["ai-waiter", "director-console"]);

interface DashboardSidebarProps {
  business: Business;
  activeTab: string;
  setActiveTab: (tab: string) => void;
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
  allowedTabs?: string[];
  isStaffUser?: boolean;
  staffData?: StaffData | null;
  globalOrders?: Record<number, any[]>;
  /** True once the dashboard's first orders reconcile has landed (K-1). */
  globalOrdersLoaded?: boolean;
  /** Occupied tables on the floor — Mesas badge source of truth (#774). */
  occupiedTablesCount?: number;
  upcomingReservations?: Reservation[];
  tutorialOpen?: boolean;
  tutorialTabKey?: string | null;
  tutorialTarget?: "tab" | "support" | null;
  onStartTutorial?: () => void;
  /** Opens the in-dashboard ops assistant from the labeled Help menu (#61). */
  onOpenOpsAssistant?: () => void;
}

type MenuItem = {
  key: TabKey;
  label: string;
  icon: LucideIcon;
  description: string;
  badge: number | null;
};

/**
 * Derive avatar initials from a business name.
 *
 * One word: first letter ("Payverge" -> "P").
 * Two+ words: first letter of first + first letter of last
 * ("Mara AI Lounge" -> "ML"). This avoids generic middle-word initials
 * (e.g. "MA") and matches the common avatar convention used elsewhere on
 * the dashboard.
 *
 * Exported so the avatar test can pin behavior without reaching into JSX.
 */
function getBusinessInitials(name?: string | null): string {
  if (!name) return "";
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "";
  const first = words[0][0] || "";
  const last = words.length > 1 ? words[words.length - 1][0] || "" : "";
  return (first + last).toUpperCase();
}

export default function DashboardSidebar({
  business,
  activeTab,
  setActiveTab,
  sidebarOpen,
  setSidebarOpen,
  allowedTabs = [],
  isStaffUser = false,
  staffData = null,
  globalOrders = {},
  globalOrdersLoaded,
  occupiedTablesCount,
  upcomingReservations = [],
  tutorialOpen = false,
  tutorialTabKey = null,
  tutorialTarget = null,
  onStartTutorial,
  onOpenOpsAssistant,
}: DashboardSidebarProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const operationalAlerts = useOptionalOperationalAlerts();
  const alertCounts = operationalAlerts?.counts ?? null;
  // Urgent chat channel for the Help menu: WhatsApp, else Telegram, else none.
  const urgentSupportUrl = brandLinks.whatsappUrl ?? brandLinks.telegramUrl;

  // Desktop-only icon-rail collapse. Same per-business, null-key-guarded
  // persistence. The boolean is a no-op on mobile because every
  // collapse-effect class below is `lg:`-prefixed.
  const [collapsed, setCollapsed] = useLocalStorageState<boolean>(
    business?.id ? `payverge_sidebar_collapsed:${business.id}` : null,
    false,
  );
  const isMobileNav = useMediaQuery("(max-width: 1023px)");
  const sidebarRootRef = React.useRef<HTMLDivElement>(null);
  const mobileDrawer = isMobileNav && sidebarOpen;

  // Mirror `collapsed` into a ref so the global key listener can read the
  // latest value without re-subscribing on every toggle.
  const collapsedRef = React.useRef(collapsed);
  collapsedRef.current = collapsed;

  // Mirror tutorialOpen so the global shortcut can bail during the guided tour
  // (the visible toggle button is already disabled during the tutorial).
  const tutorialOpenRef = React.useRef(tutorialOpen);
  tutorialOpenRef.current = tutorialOpen;

  // Global ⌘\ / Ctrl+\ toggle, mirroring the ⌘K command-palette handler.
  // Ignored while typing so it never hijacks text editing.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.key !== "\\") return;
      if (tutorialOpenRef.current) return;
      const el = document.activeElement as HTMLElement | null;
      const tag = el?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || el?.isContentEditable)
        return;
      e.preventDefault();
      setCollapsed(!collapsedRef.current);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [setCollapsed]);

  // Clean up any legacy stale keys written before the null-key guard was added.
  React.useEffect(() => {
    try {
      ["payverge_sidebar_more:none", "payverge_sidebar_more:pending"].forEach(
        (k) => {
          window.localStorage.removeItem(k);
        },
      );
    } catch {
      /* ignore */
    }
  }, []);

  // Admin lifecycle lock (suspended / closed) — the only tab gate.
  const {
    isSuspended,
    loading: accessLoading,
    isError: accessError,
  } = useBusinessAccess(business?.id);

  // M3: every rail badge (Bills pending approvals #793, Kitchen cook queue
  // #792, Reservations seated window, Tables occupancy #774 / service-call
  // fallback, Delivery alerts-only, Accounting/Fiscal failed invoices, Staff
  // unread chat) derives in one tested hook.
  const tablesBadgeIsOccupancy =
    typeof occupiedTablesCount === "number" && occupiedTablesCount > 0;
  const railBadges = useRailBadges({
    businessId: business?.id ?? 0,
    hasAccess: !accessLoading && !accessError && !isSuspended,
    alertCounts,
    occupiedTablesCount,
    globalOrders,
    globalOrdersLoaded,
    upcomingReservations,
  });

  const tString = React.useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  React.useEffect(() => {
    if (!mobileDrawer) return;
    const root = sidebarRootRef.current;
    if (!root) return;
    const focusables = () =>
      Array.from(
        root.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      );
    const focusFirst = () => {
      if (!root.isConnected) return;
      if (root.contains(document.activeElement)) return;
      const closeLabel = tString("sidebar.close");
      const headerClose = Array.from(
        root.querySelectorAll<HTMLElement>("button"),
      ).find((el) => el.getAttribute("aria-label") === closeLabel);
      (headerClose ?? focusables()[0] ?? root).focus();
    };
    focusFirst();
    const raf = requestAnimationFrame(() => {
      focusFirst();
      requestAnimationFrame(focusFirst);
    });
    const t0 = window.setTimeout(focusFirst, 0);
    const t1 = window.setTimeout(focusFirst, 50);
    const t2 = window.setTimeout(focusFirst, 160);

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setSidebarOpen(false);
        const opener = document.querySelector<HTMLElement>(
          `button[aria-label="${tString("openMenuAria")}"]`,
        );
        opener?.focus();
        return;
      }
      if (event.key !== "Tab") return;
      const nodes = focusables();
      if (nodes.length === 0) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      } else if (!root.contains(active)) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      cancelAnimationFrame(raf);
      window.clearTimeout(t0);
      window.clearTimeout(t1);
      window.clearTimeout(t2);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [mobileDrawer, setSidebarOpen, tString]);

  // Setup group label. Staff get a neutral "Other" so the section doesn't
  // feel like owner-only admin chrome.
  const moreLabel = isStaffUser
    ? tString("sidebarGroups.other")
    : tString("sidebarGroups.setup");

  // Full map of all tab definitions keyed by TabKey. Includes fiscal so the
  // Setup group can render it from SECONDARY_TABS.
  const allTabsMap: Record<TabKey, MenuItem> = React.useMemo(
    () =>
      ({
        overview: {
          key: "overview",
          label: tString("tabs.overview"),
          icon: TAB_REGISTRY.overview.icon,
          description: tString("tabs.overviewDesc"),
          badge: null,
        },
        bills: {
          key: "bills",
          label: tString(RAIL_LABEL_KEYS.bills),
          icon: TAB_REGISTRY.bills.icon,
          description: tString("tabs.billsDesc"),
          badge: railBadges.bills ?? null,
        },
        "cash-register": {
          key: "cash-register",
          label: tString("tabs.cashRegister"),
          icon: TAB_REGISTRY["cash-register"].icon,
          description: tString("tabs.cashRegisterDesc"),
          badge: null,
        },
        kitchen: {
          key: "kitchen",
          label: tString("tabs.kitchen"),
          icon: TAB_REGISTRY.kitchen.icon,
          description: tString("tabs.kitchenDesc"),
          badge: railBadges.kitchen ?? null,
        },
        menu: {
          key: "menu",
          label: tString("tabs.menu"),
          icon: TAB_REGISTRY.menu.icon,
          description: tString("tabs.menuDesc"),
          badge: null,
        },
        reservations: {
          key: "reservations",
          label: tString("tabs.reservations"),
          icon: TAB_REGISTRY.reservations.icon,
          description: tString("tabs.reservationsDesc"),
          badge: railBadges.reservations ?? null,
        },
        analytics: {
          key: "analytics",
          label: tString("tabs.analytics"),
          icon: TAB_REGISTRY.analytics.icon,
          description: tString("tabs.analyticsDesc"),
          badge: null,
        },
        "ai-waiter": {
          key: "ai-waiter",
          label: tString("tabs.aiWaiter"),
          icon: TAB_REGISTRY["ai-waiter"].icon,
          description: tString("tabs.aiWaiterDesc"),
          badge: null,
        },
        "director-console": {
          key: "director-console",
          label: tString("tabs.directorConsole"),
          icon: TAB_REGISTRY["director-console"].icon,
          description: tString("tabs.directorConsoleDesc"),
          badge: null,
        },
        marketing: {
          key: "marketing",
          label: tString("tabs.marketing"),
          icon: TAB_REGISTRY.marketing.icon,
          description: tString("tabs.marketingDesc"),
          badge: null,
        },
        tables: {
          key: "tables",
          label: tString("tabs.tables"),
          icon: TAB_REGISTRY.tables.icon,
          description: tString("tabs.tablesDesc"),
          badge: railBadges.tables ?? null,
        },
        crm: {
          key: "crm",
          label: tString("tabs.crm"),
          icon: TAB_REGISTRY.crm.icon,
          description: tString("tabs.crmDesc"),
          badge: null,
        },
        delivery: {
          key: "delivery",
          label: tString("tabs.delivery"),
          icon: TAB_REGISTRY.delivery.icon,
          description: tString("tabs.deliveryDesc"),
          badge: railBadges.delivery ?? null,
        },
        counter: {
          key: "counter",
          label: tString("tabs.counter"),
          icon: TAB_REGISTRY.counter.icon,
          description: tString("tabs.counterDesc"),
          badge: null,
        },
        inventory: {
          key: "inventory",
          label: tString("tabs.inventory"),
          icon: TAB_REGISTRY.inventory.icon,
          description: tString("tabs.inventoryDesc"),
          badge: null,
        },
        staff: {
          key: "staff",
          label: tString("tabs.staff"),
          icon: TAB_REGISTRY.staff.icon,
          description: tString("tabs.staffDesc"),
          badge: railBadges.staff ?? null,
        },
        schedule: {
          key: "schedule",
          label: tString("tabs.schedule"),
          icon: TAB_REGISTRY.schedule.icon,
          description: tString("tabs.scheduleDesc"),
          badge: null,
        },
        "business-page": {
          key: "business-page",
          label: tString("tabs.businessPage"),
          icon: TAB_REGISTRY["business-page"].icon,
          description: tString("tabs.businessPageDesc"),
          badge: null,
        },
        accounting: {
          key: "accounting",
          label: tString("tabs.accounting"),
          icon: TAB_REGISTRY.accounting.icon,
          description: tString("tabs.accountingDesc"),
          badge: railBadges.accounting ?? null,
        },
        fiscal: {
          key: "fiscal",
          label: tString(RAIL_LABEL_KEYS.fiscal),
          // Distinct from bills' Receipt — collapsed rail must not share glyphs.
          icon: TAB_REGISTRY.fiscal.icon,
          description: tString("tabs.fiscalDesc"),
          badge: railBadges.fiscal ?? null,
        },
        printers: {
          key: "printers",
          label: tString("tabs.printers"),
          icon: TAB_REGISTRY.printers.icon,
          description: tString("tabs.printersDesc"),
          badge: null,
        },
        plugins: {
          key: "plugins",
          label: tString("tabs.plugins"),
          icon: TAB_REGISTRY.plugins.icon,
          description: tString("tabs.pluginsDesc"),
          badge: null,
        },
        settings: {
          key: "settings",
          label: tString("tabs.settings"),
          icon: TAB_REGISTRY.settings.icon,
          description: tString("tabs.settingsDesc"),
          badge: null,
        },
      }) as Record<TabKey, MenuItem>,
    [railBadges, tString],
  );

  // Filter helper — if no allowedTabs restriction, pass everything through
  // Rails whose integration this install has not configured (AI, fiscal) are
  // hidden; a deep link still renders FeatureUnavailable.
  const { instance } = useInstance();
  const filterByAllowed = React.useCallback(
    (items: MenuItem[]) =>
      (allowedTabs.length === 0
        ? items
        : items.filter((i) => allowedTabs.includes(i.key))
      ).filter((i) => !instanceOffFeatureForTab(i.key, instance)),
    [allowedTabs, instance],
  );

  // TODAY group: always show tabs the role is allowed to access. Pending /
  // upcoming counts drive the badge, not the gate — kitchen workers must
  // be able to reach their KDS on a quiet queue, hosts to reservations
  // before guests arrive.
  const todayItems = React.useMemo(() => {
    const items: MenuItem[] = [];
    if (allowedTabs.length === 0 || allowedTabs.includes("overview")) {
      items.push(allTabsMap["overview"]);
    }
    if (allowedTabs.length === 0 || allowedTabs.includes("kitchen")) {
      items.push(allTabsMap["kitchen"]);
    }
    if (allowedTabs.length === 0 || allowedTabs.includes("reservations")) {
      items.push(allTabsMap["reservations"]);
    }
    return items;
  }, [allTabsMap, allowedTabs]);

  // M1: idle-prefetch the TODAY rail — the tabs an operator is most likely to
  // open next — so the first click after mount is already warm. prefetchTab is
  // memoized + fail-silent, so badge-driven todayItems churn is a no-op.
  React.useEffect(() => {
    const keys = todayItems.map((i) => i.key);
    if (keys.length === 0) return;
    const warm = () => keys.forEach((k) => void prefetchTab(k));
    const ric = window.requestIdleCallback;
    if (typeof ric === "function") {
      const id = ric.call(window, warm, { timeout: 4000 });
      return () => window.cancelIdleCallback?.(id);
    }
    const id = window.setTimeout(warm, 1500);
    return () => window.clearTimeout(id);
  }, [todayItems]);

  // Primary tabs (max 8) — exclude overview/kitchen/reservations since they live in TODAY
  const primaryItems = React.useMemo(
    () =>
      filterByAllowed(
        PRIMARY_TABS.filter(
          (k) => k !== "overview" && k !== "kitchen" && k !== "reservations",
        ).map((k) => allTabsMap[k]),
      ),
    [allTabsMap, filterByAllowed],
  );

  // Group the flat PRIMARY block under the labeled section headers the i18n
  // bundle already defines. Order + membership come from PRIMARY_TAB_GROUPS;
  // any allowed primary tab not claimed by a group is appended under "Other"
  // so nothing is ever silently dropped.
  const primaryGroups = React.useMemo(() => {
    const byKey = new Map(primaryItems.map((i) => [i.key, i] as const));
    const groups = PRIMARY_TAB_GROUPS.map((g) => ({
      id: g.id as string,
      label: tString(`sidebarGroups.${g.id}`),
      items: g.tabs
        .map((k) => byKey.get(k))
        .filter((x): x is MenuItem => Boolean(x)),
    })).filter((g) => g.items.length > 0);

    // With AI off on the instance, the AI-model tabs are filtered out above
    // and the "AI" group would hold only Marketing under an "AI" header.
    // Fold what is left into Management instead.
    const aiIndex = groups.findIndex((g) => g.id === "ai");
    if (
      aiIndex >= 0 &&
      !groups[aiIndex].items.some((i) => AI_MODEL_TABS.has(i.key))
    ) {
      const leftoverAi = groups[aiIndex];
      const management = groups.find((g) => g.id === "management");
      if (management) {
        management.items.push(...leftoverAi.items);
        groups.splice(aiIndex, 1);
      } else {
        leftoverAi.id = "management";
        leftoverAi.label = tString("sidebarGroups.management");
      }
    }

    const claimed = new Set<TabKey>(PRIMARY_TAB_GROUPS.flatMap((g) => g.tabs));
    const leftover = primaryItems.filter((i) => !claimed.has(i.key as TabKey));
    if (leftover.length > 0) {
      groups.push({
        id: "other",
        label: tString("sidebarGroups.other"),
        items: leftover,
      });
    }
    return groups;
  }, [primaryItems, tString]);

  // Secondary tabs shown under the labeled Setup group
  const secondaryItems = React.useMemo(
    () => filterByAllowed(SECONDARY_TABS.map((k) => allTabsMap[k])),
    [allTabsMap, filterByAllowed],
  );

  // Roll-up guard: if any "More" tab ever carries a count, surface the sum on
  // the collapsed ⋯ icon so a collapsed operator never has a count out of view.
  // Today this is always 0 (no SECONDARY tab is badged) — see the spec's
  // Notification sync section.
  const secondaryBadgeSum = React.useMemo(
    () => sumBadges(secondaryItems),
    [secondaryItems],
  );

  const isTutorialTabStep =
    tutorialOpen && tutorialTarget === "tab" && !!tutorialTabKey;
  const isTutorialSupportStep = tutorialOpen && tutorialTarget === "support";
  const tutorialItemRef = React.useRef<HTMLButtonElement | null>(null);
  // Keep the active nav row in view on deep-links / tab changes (AI group is
  // often below the fold on short viewports). Tutorial uses its own center scroll.
  const activeItemRef = React.useRef<HTMLButtonElement | null>(null);

  React.useEffect(() => {
    if (isTutorialTabStep) return;
    const el = activeItemRef.current;
    // jsdom and some embedded webviews lack Element.scrollIntoView.
    if (el && typeof el.scrollIntoView === "function") {
      el.scrollIntoView({
        block: "nearest",
        behavior: "smooth",
      });
    }
  }, [activeTab, isTutorialTabStep]);

  const handleTabClick = (tabKey: string) => {
    if (tutorialOpen) return;

    // Additional check for staff users
    if (
      isStaffUser &&
      allowedTabs.length > 0 &&
      !allowedTabs.includes(tabKey)
    ) {
      console.warn(
        `Staff user does not have permission to access tab: ${tabKey}`,
      );
      return;
    }

    setActiveTab(tabKey);
    setSidebarOpen(false); // Close sidebar on mobile after selection
  };

  const getTabLockMeta = React.useCallback(
    (tabKey: string) =>
      getSharedTabLockMeta(
        tabKey,
        // A FAILED access fetch is treated like the loading state (never
        // gate) so a transient/network failure reads as neutral, not locked.
        {
          loading: accessLoading || accessError,
          isSuspended,
        },
        isStaffUser,
      ),
    [isSuspended, accessLoading, accessError, isStaffUser],
  );

  React.useEffect(() => {
    if (!isTutorialTabStep || !tutorialItemRef.current) return;
    tutorialItemRef.current.scrollIntoView({
      block: "center",
      behavior: "smooth",
    });
  }, [isTutorialTabStep, tutorialTabKey]);

  // Render a single menu item button (shared across TODAY, primary, secondary).
  //
  // Row density target: icon + label on the left, ONE trailing indicator on the
  // right (lock > badge > nothing). The legacy two-line description moves to
  // the button's `title` attribute so it surfaces as a hover tooltip + is
  // announced by assistive tech without taking up a second visible row.
  const renderMenuItem = (item: MenuItem) => {
    const Icon = item.icon;
    const isActive = activeTab === item.key;
    const tabLockMeta = getTabLockMeta(item.key);
    // Collapsed rail shows the count in place of the icon (unless the tab is
    // locked, where the lock indicator already communicates state).
    const showRailBadge =
      collapsed && (item.badge ?? 0) > 0 && !tabLockMeta.locked;

    // Staff users never see locked tabs.
    if (tabLockMeta.hidden) return null;

    const isTutorialFocusedItem =
      isTutorialTabStep && tutorialTabKey === item.key;
    const blurItemForTutorial = tutorialOpen && !isTutorialFocusedItem;

    // Collapsed rail hides the trailing lock (children are `lg:hidden` in the
    // row), so a locked tab was pixel-identical to an unlocked one. Surface the
    // lock in the leading badge slot instead — same slot as the rail count.
    const showRailLock = collapsed && tabLockMeta.locked;
    const lockedTitleSuffix = showRailLock
      ? ` — ${tString("locked.short")}`
      : "";

    const tutorialClasses = [
      blurItemForTutorial ? "opacity-35 blur-[1px] pointer-events-none" : "",
      isTutorialFocusedItem
        ? "ring-2 ring-brand/30 shadow-md shadow-brand/20"
        : "",
    ]
      .filter(Boolean)
      .join(" ");

    const badgeAriaLabel =
      item.badge !== null && item.badge > 0
        ? item.key === "tables"
          ? tablesBadgeIsOccupancy
            ? tString(
                countKey("sidebar.occupiedTablesBadge", item.badge),
              ).replace("{count}", String(item.badge))
            : tString("sidebar.activeServiceCalls").replace(
                "{count}",
                String(item.badge),
              )
          : item.key === "bills"
            ? // Bills badge = tickets waiting for approval (#793) — open
              // checks are tab copy, never a rail pip.
              tString(countKey("sidebar.billsAlertBadge", item.badge)).replace(
                "{count}",
                String(item.badge),
              )
            : item.key === "kitchen"
              ? // Kitchen badge = tickets waiting on the cook (#792):
                // needs-approval + approved/Start Cooking.
                tString(countKey("sidebar.kitchenReadyBadge", item.badge)).replace(
                  "{count}",
                  String(item.badge),
                )
              : tString("sidebar.badgeCount")
                  .replace("{count}", String(item.badge))
                  .replace("{label}", item.label)
        : undefined;

    return (
      <SidebarNavRow
        key={item.key}
        buttonRef={
          isTutorialFocusedItem
            ? tutorialItemRef
            : isActive
              ? activeItemRef
              : undefined
        }
        label={item.label}
        description={item.description}
        icon={Icon}
        active={isActive}
        collapsed={collapsed}
        disabled={tutorialOpen}
        title={`${item.label}${item.badge ? ` (${item.badge})` : ""}${lockedTitleSuffix}`}
        tutorialClasses={tutorialClasses}
        onClick={() => handleTabClick(item.key)}
        onPrefetch={() => prefetchTab(item.key)}
        leadingBadge={
          showRailBadge ? (
            <AnimatedBadge
              count={item.badge}
              label={item.label}
              testId={`rail-badge-${item.key}`}
              className="hidden lg:inline-flex"
            />
          ) : showRailLock ? (
            <span
              data-testid={`rail-lock-${item.key}`}
              aria-label={tString("locked.short")}
              className="hidden h-4 w-4 shrink-0 items-center justify-center lg:inline-flex"
            >
              <Lock
                className="h-3.5 w-3.5 text-ink-400/80"
                aria-hidden="true"
              />
            </span>
          ) : undefined
        }
      >
        <TrailingIndicator
          locked={tabLockMeta.locked}
          badge={item.badge}
          badgeLabel={badgeAriaLabel}
          lockedLabel={tString("locked.short")}
        />
      </SidebarNavRow>
    );
  };

  return (
    <>
      {/* Mobile overlay - High z-index to cover everything including TopMenu */}
      {sidebarOpen && !tutorialOpen && (
        <div
          className="fixed inset-0 bg-black/40 backdrop-blur-sm z-[140] lg:hidden animate-fade-in"
          role="button"
          tabIndex={-1}
          aria-label={tString("sidebar.close")}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              setSidebarOpen(false);
              document
                .querySelector<HTMLElement>(
                  `button[aria-label="${tString("openMenuAria")}"]`,
                )
                ?.focus();
            }
          }}
          onClick={() => {
            setSidebarOpen(false);
            document
              .querySelector<HTMLElement>(
                `button[aria-label="${tString("openMenuAria")}"]`,
              )
              ?.focus();
          }}
        />
      )}

      {/* Sidebar - z-[150] to be above overlay and TopMenu */}
      <div
        ref={sidebarRootRef}
        data-testid="dashboard-sidebar-root"
        role={mobileDrawer ? "dialog" : undefined}
        aria-modal={mobileDrawer ? "true" : undefined}
        aria-label={mobileDrawer ? tString("sidebar.navigation") : undefined}
        aria-hidden={isMobileNav && !sidebarOpen ? true : undefined}
        inert={(isMobileNav && !sidebarOpen) || undefined}
        tabIndex={mobileDrawer ? -1 : undefined}
        className={`
        fixed lg:relative inset-y-0 left-0 ${tutorialOpen ? "z-[180] lg:z-[180]" : "z-[150] lg:z-30"}
        w-[85vw] max-w-[286px] ${collapsed ? "lg:w-16" : "lg:w-72"} lg:max-w-none border-r border-warm-200/80 bg-white/90 backdrop-blur-xl
        transform transition-[transform,width] duration-300 ease-in-out lg:transform-none
        shadow-2xl lg:shadow-none
        ${sidebarOpen ? "translate-x-0" : "-translate-x-full max-lg:invisible max-lg:pointer-events-none lg:translate-x-0"}
        top-0 lg:top-0 h-full
      `}
      >
        {/* Collapse handle — a chevron that straddles the sidebar's right
            border line (the "(|)" the operator pictured). « collapses,
            » expands. Desktop-only; absolutely anchored to the lg:relative
            root so it floats on the divider without shifting layout. */}
        <button
          type="button"
          onClick={() => {
            if (tutorialOpen) return;
            setCollapsed(!collapsed);
          }}
          disabled={tutorialOpen}
          aria-expanded={!collapsed}
          aria-keyshortcuts="Meta+\ Control+\"
          aria-label={
            collapsed ? tString("sidebar.expand") : tString("sidebar.collapse")
          }
          title={
            collapsed ? tString("sidebar.expand") : tString("sidebar.collapse")
          }
          className={`hidden lg:flex absolute top-7 right-0 z-20 translate-x-1/2 items-center justify-center w-6 h-6 rounded-full bg-white border border-warm-200/90 shadow-sm shadow-ink-900/5 text-ink-500 hover:text-ink-900 hover:border-brand/30 hover:shadow-md hover:shadow-brand/10 transition-all focus:outline-none focus:ring-2 focus:ring-brand/30 ${tutorialOpen ? "opacity-45 pointer-events-none" : ""}`}
        >
          {collapsed ? (
            <ChevronsRight className="w-3.5 h-3.5" aria-hidden="true" />
          ) : (
            <ChevronsLeft className="w-3.5 h-3.5" aria-hidden="true" />
          )}
        </button>
        <div className="flex flex-col h-full bg-transparent">
          {/* Header — one calm row: back, logo, name. No shadowed tile, no
              filler sublabel; the operator knows this is their dashboard. */}
          <div
            className={`flex items-center justify-between gap-3 p-4 border-b border-warm-200/70 transition-all ${
              collapsed ? "lg:flex-col lg:gap-2 lg:p-3" : ""
            } ${tutorialOpen ? "opacity-45 blur-[1px] pointer-events-none" : ""}`}
          >
            <div
              className={`flex items-center gap-2.5 overflow-hidden ${collapsed ? "lg:gap-0 lg:justify-center" : ""}`}
            >
              <button
                type="button"
                onClick={() => {
                  if (tutorialOpen) return;
                  navigateToVenuesOverview();
                }}
                className={`group flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg text-ink-400 transition-colors hover:bg-warm-100 hover:text-ink-900 focus:outline-none focus:ring-2 focus:ring-brand/30 ${collapsed ? "lg:hidden" : ""}`}
                aria-label={tString("goToBusinesses")}
                title={tString("goToBusinesses")}
                disabled={tutorialOpen}
              >
                <ArrowLeft className="w-4 h-4" strokeWidth={2} aria-hidden="true" />
              </button>

              <div className="w-8 h-8 bg-brand rounded-lg flex-shrink-0 flex items-center justify-center overflow-hidden relative">
                {business?.logo ? (
                  <Image
                    src={business.logo}
                    unoptimized={!canOptimizeImageSrc(business.logo)}
                    alt={business.name}
                    width={32}
                    height={32}
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <span className="text-white font-semibold text-sm">
                    {getBusinessInitials(business?.name) || "B"}
                  </span>
                )}
              </div>
              <div
                data-collapsible="business-name"
                className={`flex min-w-0 flex-col ${collapsed ? "lg:hidden" : ""}`}
              >
                <h2 className="line-clamp-2 break-words text-sm font-semibold leading-tight text-ink-950">
                  {business?.name}
                </h2>
              </div>
            </div>
            <Button
              isIconOnly
              variant="light"
              size="sm"
              isDisabled={tutorialOpen}
              aria-label={tString("sidebar.close")}
              className="lg:hidden text-ink-600 hover:bg-warm-100 rounded-full"
              onPress={() => {
                setSidebarOpen(false);
                document
                  .querySelector<HTMLElement>(
                    `button[aria-label="${tString("openMenuAria")}"]`,
                  )
                  ?.focus();
              }}
            >
              <X className="w-5 h-5" aria-hidden="true" />
            </Button>
          </div>

          {/* Navigation */}
          <div className="flex-1 overflow-hidden relative">
            {/* M34: the 23-destination nav overflows below the fold on ~768px
                laptops. `scrollbar-hide` hid the only affordance that the list
                kept going, so operators never discovered the lower tabs. Swap
                to a thin warm-toned scrollbar (tailwind-scrollbar) AND lay a
                bottom fade-out gradient over the scroll container as a second
                overflow cue. The fade is pointer-events-none so it never eats a
                nav click, and matches the sidebar's white surface. */}
            <nav className="h-full overflow-y-auto p-3 lg:p-4 scrollbar-thin scrollbar-thumb-warm-400/90 scrollbar-track-warm-100/80">
              <div className="space-y-1 pb-16">
                {/* ⌘K command palette launcher — the fast way to jump anywhere.
                    Hidden during the guided tour so it doesn't compete with the
                    spotlight. */}
                {!tutorialOpen && (
                  <div className="mb-3">
                    <div className={collapsed ? "lg:hidden" : ""}>
                      <CommandPaletteTrigger fullWidth />
                    </div>
                    <div
                      className={`hidden ${collapsed ? "lg:flex lg:justify-center" : ""}`}
                    >
                      <CommandPaletteTrigger iconOnly />
                    </div>
                  </div>
                )}

                {/* TODAY group — render items flush without an eyebrow. The
                    "TODAY" label was paired with no contrasting "YESTERDAY" /
                    "THIS WEEK" group, so it added no information. */}
                {todayItems.length > 0 && (
                  <div className="mb-4 space-y-1">
                    {todayItems.map(renderMenuItem)}
                  </div>
                )}

                {/* PRIMARY list — grouped under labeled section headers
                    (Operations / AI / Management / Finance). Labels use the
                    same uppercase muted eyebrow styling as the Setup
                    group and hide in the collapsed desktop rail. */}
                {primaryGroups.map((group) => (
                  <div key={group.id} className="mb-4">
                    <div
                      className={`px-3 pb-1 text-[11px] font-bold text-ink-500 uppercase tracking-wider ${
                        collapsed ? "block lg:hidden" : "block"
                      }`}
                    >
                      {group.label}
                    </div>
                    <div className="space-y-1">
                      {group.items.map(renderMenuItem)}
                    </div>
                  </div>
                ))}

                {/* Setup — labeled group, same chrome as Operations (#60).
                    Collapsed desktop rail still uses a single ⋯ so six setup
                    icons don't stack under the icon rail. */}
                {secondaryItems.length > 0 && (
                  <div className="mb-4" data-testid="sidebar-setup-group">
                    <div
                      className={`px-3 pb-1 text-[11px] font-bold text-ink-500 uppercase tracking-wider ${
                        collapsed ? "block lg:hidden" : "block"
                      }`}
                    >
                      {moreLabel}
                    </div>
                    {collapsed && (
                      <button
                        onClick={() => setCollapsed(false)}
                        data-testid="rail-more"
                        aria-label={moreLabel}
                        title={moreLabel}
                        className="relative w-full items-center justify-center rounded-xl px-0 py-2 mt-4 text-ink-400 hover:bg-warm-100 transition-colors hidden lg:flex"
                      >
                        <MoreHorizontal
                          className="w-4 h-4"
                          aria-hidden="true"
                        />
                        {secondaryBadgeSum > 0 && (
                          <span
                            key={secondaryBadgeSum}
                            data-testid="rail-badge-more"
                            className="absolute top-1 right-2 flex items-center justify-center min-w-[16px] h-[16px] px-1 bg-rose-500 text-white text-[10px] font-bold rounded-full shadow-sm motion-safe:animate-[badge-pop-in_220ms_ease-out]"
                          >
                            {capBadge(secondaryBadgeSum)}
                          </span>
                        )}
                      </button>
                    )}
                    <div
                      className={`space-y-1 ${collapsed ? "lg:hidden" : ""}`}
                    >
                      {secondaryItems.map(renderMenuItem)}
                    </div>
                  </div>
                )}

              </div>
            </nav>
            {/* M34: bottom fade-out gradient — a second overflow cue that the
                nav continues past the fold. Anchored to the relative scroll
                wrapper (not the scrolling nav), pointer-events-none so it never
                intercepts a tab click, and fades to the sidebar's white surface. */}
            <div
              aria-hidden="true"
              className="pointer-events-none absolute inset-x-0 bottom-0 h-12 bg-gradient-to-t from-white via-white/85 to-transparent"
            />
          </div>

          {/* Footer */}
          {(!tutorialOpen || isTutorialSupportStep) && (
            <div
              className={`p-3 border-t border-warm-200/70 z-10 relative ${
                isTutorialSupportStep
                  ? "bg-white ring-2 ring-brand/40 rounded-t-2xl shadow-lg"
                  : ""
              }`}
            >
              {/* One quiet utility row: ghost icons left, locale right. The
                  earlier stack of bordered lifting tiles + a separate pill
                  row read as five competing controls. */}
              <div
                className={`flex items-center justify-between gap-2 ${collapsed ? "lg:flex-col lg:justify-center lg:gap-2" : ""}`}
              >
                <div
                  className={`flex items-center gap-1 ${collapsed ? "lg:flex-col lg:gap-2" : ""}`}
                >
                  {/* Alerts — one control merging the 7-day "what did I miss"
                      history with the alert-sound toggle (was two look-alike
                      icons). Wrapped so it shares the sidebar's tooltip style. */}
                  {!tutorialOpen && (
                    <SidebarIconTooltip label={tString("recentAlerts.title")}>
                      <RecentAlertsPopover
                        businessId={business.id}
                        placement="top-start"
                        showSoundControl
                        hideNativeTitle
                        onNavigate={setActiveTab}
                      />
                    </SidebarIconTooltip>
                  )}

                  {/* Help — one labeled control for Tour / support / urgent
                      chat / Book a call so the footer isn't unlabeled icon
                      soup. Links come from @/config/brand: support always
                      exists (GitHub by default); urgent chat and booking are
                      hidden until an operator configures them. */}
                  <Dropdown placement="top-start">
                    <DropdownTrigger>
                      <button
                        type="button"
                        data-testid="sidebar-help-menu"
                        aria-label={tString("footer.helpMenu")}
                        className={`group relative flex h-11 items-center justify-center gap-1.5 rounded-lg px-2.5 text-ink-400 transition-colors hover:bg-warm-100 hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand [@media(hover:hover)_and_(pointer:fine)]:h-8 ${
                          collapsed
                            ? "lg:h-8 lg:w-auto lg:px-1.5"
                            : ""
                        }`}
                      >
                        <HelpCircle className="h-4 w-4 flex-shrink-0" />
                        <span className="text-xs font-medium text-ink-600">
                          {tString("footer.helpMenu")}
                        </span>
                      </button>
                    </DropdownTrigger>
                    <DropdownMenu
                      aria-label={tString("footer.helpMenu")}
                      onAction={(key) => {
                        if (key === "tour" && onStartTutorial) {
                          onStartTutorial();
                        }
                        if (key === "ops-assistant" && onOpenOpsAssistant) {
                          onOpenOpsAssistant();
                        }
                      }}
                    >
                      {onOpenOpsAssistant ? (
                        <DropdownItem
                          key="ops-assistant"
                          startContent={<Bot className="h-4 w-4" />}
                        >
                          {tString("footer.askAssistant")}
                        </DropdownItem>
                      ) : null}
                      {!tutorialOpen && onStartTutorial ? (
                        <DropdownItem
                          key="tour"
                          startContent={<HelpCircle className="h-4 w-4" />}
                        >
                          {tString("tutorial.takeATour")}
                        </DropdownItem>
                      ) : null}
                      <DropdownItem
                        key="support"
                        href={brandLinks.supportUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        startContent={<GithubIcon className="h-4 w-4" />}
                      >
                        {tString("footer.requestHelp")}
                      </DropdownItem>
                      {urgentSupportUrl ? (
                        <DropdownItem
                          key="urgent-support"
                          href={urgentSupportUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          startContent={
                            brandLinks.whatsappUrl ? (
                              <WhatsappIcon className="h-4 w-4" />
                            ) : (
                              <TelegramIcon className="h-4 w-4" />
                            )
                          }
                        >
                          {tString("footer.urgentSupport")}
                        </DropdownItem>
                      ) : null}
                      {brandLinks.bookCallUrl ? (
                        <DropdownItem
                          key="book-call"
                          href={brandLinks.bookCallUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          startContent={<Calendar className="h-4 w-4" />}
                        >
                          {tString("footer.bookCall")}
                        </DropdownItem>
                      ) : null}
                    </DropdownMenu>
                  </Dropdown>

                  {/* Locale switcher — Globe + ISO code, same labeled-control
                      pattern as Help. Compact so it fits the row and the
                      collapsed rail (the old wide name pill overflowed). */}
                  <SidebarIconTooltip label={tString("footer.language")}>
                    <SimpleLanguageSwitcher compact />
                  </SidebarIconTooltip>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}

// Wraps a control that renders its own trigger (a NextUI Popover/Dropdown, e.g.
// the Alerts and Language switchers) with the shared styled tooltip (M5) — so
// the whole footer row speaks one tooltip style.
function SidebarIconTooltip({
  label,
  children,
}: {
  label: string;
  children: React.ReactElement;
}) {
  return (
    <SidebarTooltip label={label} className="inline-flex">
      {children}
    </SidebarTooltip>
  );
}

/**
 * Single trailing indicator for a sidebar row.
 *
 * Precedence (only ONE is rendered, ever):
 *   1. Lock icon — shown while an administrator has suspended the business.
 *   2. Numeric badge — shown when `badge > 0`.
 *   3. Nothing.
 *
 * One trailing element per row keeps the rail calm.
 */
function TrailingIndicator({
  locked,
  badge,
  badgeLabel,
  lockedLabel,
}: {
  locked: boolean;
  badge: number | null;
  badgeLabel?: string;
  lockedLabel: string;
}) {
  if (locked) {
    const label = lockedLabel;
    // Visible text hint — a bare lock icon with only title/aria was unread mid-shift (#59).
    return (
      <span
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-warm-200 bg-warm-50 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-ink-500"
        aria-label={label}
        title={label}
      >
        <Lock className="h-3 w-3 text-ink-400" aria-hidden="true" />
        <span className="max-w-[4.5rem] truncate">
          {lockedLabel}
        </span>
      </span>
    );
  }

  if (badge !== null && badge > 0) {
    return (
      <span
        className="shrink-0 flex items-center justify-center min-w-[18px] h-[18px] px-1 bg-rose-500 text-white text-[10px] font-bold rounded-full shadow-sm"
        aria-label={badgeLabel ?? String(badge)}
      >
        {badge}
      </span>
    );
  }

  return null;
}
