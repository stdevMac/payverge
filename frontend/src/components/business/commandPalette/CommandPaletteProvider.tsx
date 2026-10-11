"use client";

import React from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { Business, getMenu, getTablesWithStatus } from "@/api/business";
import { getBusinessBills } from "@/api/bills";
import { getBusinessStaff } from "@/api/staff";
import { parseMenuCategories } from "@/utils/businessDataParsers";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useInstance } from "@/hooks/useInstance";
import { instanceOffFeatureForTab } from "@/lib/instance/featureGates";
import { useLocalStorageState } from "@/hooks/useLocalStorageState";
import { runWithFeedback } from "@/lib/runWithFeedback";
import CommandPalette from "./CommandPalette";
import {
  buildNavCommands,
  buildRecordCommands,
  type Command,
  type NavTabDef,
  type RecordDef,
} from "./commandRegistry";

/**
 * Canonical, ordered tab registry for the palette. Mirrors the sidebar's
 * reading order (Today → primary → setup). `labelKey` maps a `TabKey` to its
 * camelCase `businessDashboard.tabs.*` translation key; `keywords` are English
 * aliases so search works regardless of the operator's locale.
 */
const TAB_REGISTRY: Array<{
  key: string;
  labelKey: string;
  iconKey: string;
  keywords: string[];
}> = [
  { key: "overview", labelKey: "overview", iconKey: "Home", keywords: ["home", "dashboard", "stats"] },
  { key: "bills", labelKey: "bills", iconKey: "Receipt", keywords: ["checks", "tabs", "orders", "payments"] },
  { key: "cash-register", labelKey: "cashRegister", iconKey: "Banknote", keywords: ["cash", "caja", "register", "drawer", "reconciliation"] },
  { key: "printers", labelKey: "printers", iconKey: "Printer", keywords: ["print", "receipt", "thermal", "station"] },
  { key: "kitchen", labelKey: "kitchen", iconKey: "ChefHat", keywords: ["kds", "cook", "tickets"] },
  { key: "reservations", labelKey: "reservations", iconKey: "Calendar", keywords: ["bookings", "guests"] },
  { key: "menu", labelKey: "menu", iconKey: "Utensils", keywords: ["items", "dishes", "food", "products"] },
  { key: "tables", labelKey: "tables", iconKey: "QrCode", keywords: ["qr", "seating", "floor"] },
  { key: "ai-waiter", labelKey: "aiWaiter", iconKey: "Bot", keywords: ["assistant", "chatbot", "ai"] },
  { key: "director-console", labelKey: "directorConsole", iconKey: "BarChart3", keywords: ["ai", "director", "advisor", "insights"] },
  { key: "marketing", labelKey: "marketing", iconKey: "Megaphone", keywords: ["marketing", "posts", "campaigns", "social", "promotions"] },
  { key: "analytics", labelKey: "analytics", iconKey: "TrendingUp", keywords: ["reports", "revenue", "sales", "insights"] },
  { key: "crm", labelKey: "crm", iconKey: "UserCircle", keywords: ["customers", "contacts", "loyalty"] },
  { key: "delivery", labelKey: "delivery", iconKey: "Truck", keywords: ["shipping", "drivers"] },
  { key: "counter", labelKey: "counter", iconKey: "Coffee", keywords: ["setup", "takeaway", "naming", "prefix"] },
  { key: "inventory", labelKey: "inventory", iconKey: "Boxes", keywords: ["stock", "supplies"] },
  // TabKey is `staff` (matches RBAC staff:* and ?tab=staff). Label is localized
  // as "Staff" so URL / TabKey / i18n key agree (Task 35). "team" stays a keyword.
  { key: "staff", labelKey: "staff", iconKey: "Users", keywords: ["team", "employees", "roles", "permissions", "staff"] },
  { key: "schedule", labelKey: "schedule", iconKey: "CalendarClock", keywords: ["shifts", "roster", "rota", "scheduling", "team"] },
  { key: "business-page", labelKey: "businessPage", iconKey: "Globe", keywords: ["storefront", "website", "public", "page"] },
  { key: "accounting", labelKey: "accounting", iconKey: "FileText", keywords: ["finance", "ledger", "payroll", "books"] },
  // Task 32: fiscal restored as its own Setup row (was orphaned from sidebar).
  { key: "fiscal", labelKey: "fiscal", iconKey: "Receipt", keywords: ["fiscal", "tax", "afip", "invoice", "receipts", "invoices"] },
  { key: "plugins", labelKey: "plugins", iconKey: "CreditCard", keywords: ["integrations", "payments", "stripe", "apps"] },
  { key: "settings", labelKey: "settings", iconKey: "Settings", keywords: ["config", "preferences", "options"] },
];

/**
 * Exported so the parity test can assert TAB_REGISTRY stays in lock-step with
 * the sidebar's PRIMARY_TABS ∪ SECONDARY_TABS invariant (tabAccess.ts:4-8).
 */
export const TAB_REGISTRY_KEYS: string[] = TAB_REGISTRY.map((tab) => tab.key);

const RECENT_KEY_CAP = 8;

interface CommandPaletteContextValue {
  open: boolean;
  openPalette: () => void;
  closePalette: () => void;
  toggle: () => void;
  /** True once mounted on the client — lets the trigger render the right hotkey hint. */
  ready: boolean;
}

const CommandPaletteContext = React.createContext<CommandPaletteContextValue | null>(null);

/**
 * Read the palette controls from anywhere inside the provider (e.g. the
 * header trigger pill). Returns a no-op stub outside a provider so consumers
 * never crash.
 */
export function useCommandPalette(): CommandPaletteContextValue {
  return (
    React.useContext(CommandPaletteContext) ?? {
      open: false,
      openPalette: () => {},
      closePalette: () => {},
      toggle: () => {},
      ready: false,
    }
  );
}

export interface CommandPaletteProviderProps {
  business: Business | null;
  activeTab: string;
  setActiveTab: (tab: string) => void;
  setSidebarOpen: (open: boolean) => void;
  allowedTabs?: string[];
  isStaffUser?: boolean;
  /** True while the guided tour is running — suppresses the global ⌘K shortcut. */
  tutorialOpen?: boolean;
  onStartTutorial?: () => void;
  children: React.ReactNode;
}

export function CommandPaletteProvider({
  business,
  setActiveTab,
  setSidebarOpen,
  allowedTabs = [],
  isStaffUser = false,
  tutorialOpen = false,
  onStartTutorial,
  children,
}: CommandPaletteProviderProps) {
  const { locale } = useSimpleLocale();
  const [open, setOpen] = React.useState(false);
  const [ready, setReady] = React.useState(false);

  const businessId = business?.id ? String(business.id) : null;
  const [recentKeys, setRecentKeys] = useLocalStorageState<string[]>(
    businessId ? `payverge_cmdk_recent:${businessId}` : null,
    [],
  );

  const {
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(business?.id);

  React.useEffect(() => setReady(true), []);

  const t = React.useCallback(
    (key: string): string => {
      const result = getTranslation(`businessDashboard.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const openPalette = React.useCallback(() => setOpen(true), []);
  const closePalette = React.useCallback(() => setOpen(false), []);
  const toggle = React.useCallback(() => setOpen((o) => !o), []);

  // Mirror tutorialOpen into a ref so the global shortcut can bail during the
  // guided tour without re-subscribing the listener on every state flip
  // (mirrors DashboardSidebar's ⌘\ guard at DashboardSidebar.tsx:165-173).
  const tutorialOpenRef = React.useRef(tutorialOpen);
  tutorialOpenRef.current = tutorialOpen;

  // Global ⌘K / Ctrl+K to summon the palette from anywhere in the dashboard.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === "k") {
        if (tutorialOpenRef.current) return;
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Same instance gate as the sidebar: a tab whose integration this install
  // has not configured (e.g. AI off) is never offered by the palette.
  const { instance } = useInstance();
  const navCommands = React.useMemo<Command[]>(() => {
    const tabs: NavTabDef[] = TAB_REGISTRY.filter(
      (tab) => instanceOffFeatureForTab(tab.key, instance) === null,
    ).map((tab) => ({
      key: tab.key,
      label: t(`tabs.${tab.labelKey}`),
      description: t(`tabs.${tab.labelKey}Desc`),
      iconKey: tab.iconKey,
      keywords: tab.keywords,
    }));
    return buildNavCommands(tabs, {
      allowedTabs,
      isStaffUser,
      access: { loading: accessLoading, isSuspended },
    });
  }, [t, allowedTabs, isStaffUser, accessLoading, isSuspended, instance]);

  const actionCommands = React.useMemo<Command[]>(() => {
    const actions: Command[] = [];
    const canStorefront = Boolean(business?.custom_url && business?.business_page_enabled);

    if (canStorefront) {
      actions.push({
        id: "action:storefront",
        group: "actions",
        kind: "action",
        label: t("commandPalette.viewStorefront"),
        description: t("commandPalette.viewStorefrontDesc"),
        keywords: ["storefront", "public", "website", "preview", "menu"],
        iconKey: "ExternalLink",
        locked: false,
        actionKey: "storefront",
      });
      actions.push({
        id: "action:copy-link",
        group: "actions",
        kind: "action",
        label: t("commandPalette.copyStorefrontLink"),
        description: t("commandPalette.copyStorefrontLinkDesc"),
        keywords: ["copy", "share", "link", "url"],
        iconKey: "Copy",
        locked: false,
        actionKey: "copy-link",
      });
    }

    if (onStartTutorial) {
      actions.push({
        id: "action:tour",
        group: "actions",
        kind: "action",
        label: t("commandPalette.startTour"),
        description: t("commandPalette.startTourDesc"),
        keywords: ["tutorial", "tour", "guide", "help", "onboarding"],
        iconKey: "GraduationCap",
        locked: false,
        actionKey: "tour",
      });
    }

    return actions;
  }, [business?.custom_url, business?.business_page_enabled, onStartTutorial, t]);

  // Task 35: index live records (bills, staff, menu items, tables) so ⌘K is
  // not just a slower sidebar. Load once per open — failures degrade to empty
  // records without blocking section navigation.
  const [recordDefs, setRecordDefs] = React.useState<RecordDef[]>([]);
  const [recordsLoading, setRecordsLoading] = React.useState(false);
  const recordsLoadedForRef = React.useRef<number | null>(null);

  React.useEffect(() => {
    if (!open || !business?.id) return;
    // Refresh when the palette opens for a different business, or always when
    // opening so edits made in-session appear without a full page reload.
    const bizId = business.id;
    let cancelled = false;
    setRecordsLoading(true);
    void (async () => {
      const defs: RecordDef[] = [];
      try {
        const [billsRes, staffRes, menuRes, tablesRes] = await Promise.allSettled([
          getBusinessBills(bizId, { page: 1, pageSize: 40 }),
          getBusinessStaff(String(bizId)),
          getMenu(bizId),
          getTablesWithStatus(bizId),
        ]);

        if (billsRes.status === "fulfilled") {
          const bills = billsRes.value?.bills ?? [];
          for (const bill of bills) {
            if (!bill?.id) continue;
            const tableBit =
              bill.table_name || bill.table?.name || bill.table?.table_code || "";
            const label = tableBit
              ? `${t("commandPalette.bill")} #${bill.id} · ${tableBit}`
              : `${t("commandPalette.bill")} #${bill.id}`;
            defs.push({
              id: `bill:${bill.id}`,
              tabKey: "bills",
              navSpec: `bills?billId=${bill.id}`,
              label,
              description: t("commandPalette.openBill"),
              keywords: [
                String(bill.id),
                bill.bill_number || "",
                bill.status || "",
                bill.table_name || "",
                bill.table?.name || "",
                bill.table?.table_code || "",
                "bill",
                "check",
              ].filter(Boolean),
              iconKey: "Receipt",
            });
          }
        }

        if (staffRes.status === "fulfilled") {
          const staff = staffRes.value?.staff ?? [];
          for (const member of staff) {
            if (!member?.id) continue;
            defs.push({
              id: `staff:${member.id}`,
              tabKey: "staff",
              navSpec: `staff?sub=people&staffSearch=${encodeURIComponent(member.name || member.email || "")}`,
              label: member.name || member.email || `Staff #${member.id}`,
              description: t("commandPalette.goToStaff"),
              keywords: [
                member.name || "",
                member.email || "",
                member.role || "",
                "staff",
                "team",
                "employee",
              ].filter(Boolean),
              iconKey: "Users",
            });
          }
        }

        if (menuRes.status === "fulfilled") {
          const categories = parseMenuCategories(menuRes.value);
          for (const cat of categories) {
            for (const item of cat.items ?? []) {
              const name = item?.name;
              if (!name) continue;
              const id = item.id || name;
              defs.push({
                id: `menu:${id}`,
                tabKey: "menu",
                navSpec: `menu?menuSearch=${encodeURIComponent(name)}`,
                label: name,
                description:
                  cat.name
                    ? `${t("commandPalette.menuItem")} · ${cat.name}`
                    : t("commandPalette.goToMenuItem"),
                keywords: [
                  name,
                  cat.name || "",
                  item.description || "",
                  "menu",
                  "dish",
                  "item",
                ].filter(Boolean),
                iconKey: "Utensils",
              });
            }
          }
        }

        if (tablesRes.status === "fulfilled") {
          const rows = tablesRes.value?.tables ?? [];
          for (const row of rows) {
            const table = row.table;
            if (!table) continue;
            const name = table.name || table.table_code;
            if (!name) continue;
            defs.push({
              id: `table:${table.id ?? table.table_code ?? name}`,
              tabKey: "tables",
              navSpec: `tables?tableSearch=${encodeURIComponent(table.table_code || name)}`,
              label: table.name
                ? `${table.name}${table.table_code ? ` (${table.table_code})` : ""}`
                : String(table.table_code),
              description: t("commandPalette.goToTable"),
              keywords: [
                table.name || "",
                table.table_code || "",
                "table",
                "qr",
                "seat",
              ].filter(Boolean),
              iconKey: "QrCode",
            });
          }
        }
      } finally {
        if (!cancelled) {
          setRecordDefs(defs);
          setRecordsLoading(false);
          recordsLoadedForRef.current = bizId;
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, business?.id, t]);

  const recordCommands = React.useMemo<Command[]>(
    () =>
      buildRecordCommands(recordDefs, {
        allowedTabs,
        isStaffUser,
        access: { loading: accessLoading, isSuspended },
      }),
    [recordDefs, allowedTabs, isStaffUser, accessLoading, isSuspended],
  );

  // `useLocalStorageState`'s setter takes a concrete value (not a functional
  // updater — it JSON-stringifies the argument), so derive the next list from
  // the current `recentKeys` snapshot.
  const recordRecent = React.useCallback(
    (tabKey: string) => {
      const next = [tabKey, ...recentKeys.filter((k) => k !== tabKey)].slice(
        0,
        RECENT_KEY_CAP,
      );
      setRecentKeys(next);
    },
    [recentKeys, setRecentKeys],
  );

  const runCommand = React.useCallback(
    (command: Command) => {
      if ((command.kind === "nav" || command.kind === "record") && (command.navSpec || command.tabKey)) {
        const spec = command.navSpec || command.tabKey!;
        setActiveTab(spec);
        setSidebarOpen(false);
        if (command.tabKey) recordRecent(command.tabKey);
        setOpen(false);
        return;
      }

      switch (command.actionKey) {
        case "storefront":
          if (business?.custom_url) {
            window.open(`/b/${business.custom_url}`, "_blank", "noopener,noreferrer");
          }
          break;
        case "copy-link":
          if (business?.custom_url && typeof navigator !== "undefined" && navigator.clipboard) {
            const url = `${window.location.origin}/b/${business.custom_url}`;
            void runWithFeedback(
              () => navigator.clipboard.writeText(url),
              {
                success: t("commandPalette.linkCopied"),
                error: t("commandPalette.linkCopyFailed"),
              },
            );
          }
          break;
        case "tour":
          onStartTutorial?.();
          break;
      }
      setOpen(false);
    },
    [business?.custom_url, setActiveTab, setSidebarOpen, recordRecent, onStartTutorial, t],
  );

  const ctx = React.useMemo<CommandPaletteContextValue>(
    () => ({ open, openPalette, closePalette, toggle, ready }),
    [open, openPalette, closePalette, toggle, ready],
  );

  return (
    <CommandPaletteContext.Provider value={ctx}>
      {children}
      <CommandPalette
        open={open}
        onClose={closePalette}
        navCommands={navCommands}
        actionCommands={actionCommands}
        recordCommands={recordCommands}
        recordsLoading={recordsLoading}
        recentTabKeys={recentKeys}
        onRun={runCommand}
        t={t}
      />
    </CommandPaletteContext.Provider>
  );
}
