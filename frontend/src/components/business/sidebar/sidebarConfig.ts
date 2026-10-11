/**
 * Operator sidebar information architecture.
 *
 * Membership comes from `config/dashboard-tabs.json` (backend source of truth)
 * so the two taxonomies cannot diverge. Presentation groups and ordering are
 * FE-only overrides on top of that membership.
 *
 * Presentation:
 *   - operations / ai / management  — labeled primary sections
 *   - finance                        — Analytics + Accounting (not buried under Setup)
 *   - setup                          — labeled group, same chrome as Operations
 *
 * Every `route_kind: "dashboard_tab"` key appears in exactly one group.
 * `printers` is `route_kind: "standalone"` but still surfaces in operations.
 */

// Tracked mirror of repo-root config/dashboard-tabs.json (backend source of
// truth). The root file sits outside the ./frontend Docker build context, so
// the bundle imports this copy; byte-parity is pinned by sidebarConfig.test.ts
// — after editing the root file, run:
//   cp config/dashboard-tabs.json frontend/src/config/dashboard-tabs.json
import dashboardTabsJson from "../../../config/dashboard-tabs.json";
import type { TabKey } from "../tabs/tabRegistry";

// TabKey derives from the tab registry (single definition point: loader,
// icon, lock gate). Type-only import — no runtime cycle with the registry.
export type { TabKey };

type ConfigGroup = "operations" | "ai" | "management" | "settings";

/** Sidebar presentation groups (FE-facing ids / i18n keys under sidebarGroups.*). */
export type SidebarGroupId =
  | "operations"
  | "ai"
  | "management"
  | "finance"
  | "setup";

type DashboardTabRow = {
  key: string;
  group: ConfigGroup;
  route_kind: string;
};

const DASHBOARD_TABS = dashboardTabsJson as DashboardTabRow[];

/** Keys that participate in the operator sidebar (dashboard_tab + printers). */
function sidebarEligibleKeys(): TabKey[] {
  return DASHBOARD_TABS.filter(
    (row) =>
      row.route_kind === "dashboard_tab" ||
      // printers is standalone (settings route lives outside the tab switch)
      // but is daily operational infrastructure — keep it in the sidebar.
      (row.route_kind === "standalone" && row.key === "printers"),
  ).map((row) => row.key as TabKey);
}

/**
 * FE presentation override: remaps a config group (or a specific key) onto a
 * sidebar presentation group. Everything not listed inherits:
 *   config settings → setup, other config groups → same name.
 *
 * Analytics + Accounting form a real Finance group (the old financeAndSettings
 * held Analytics + Inventory — neither is finance). Inventory stays in
 * management with CRM / Staff / Schedule.
 */
const KEY_GROUP_OVERRIDE: Partial<Record<TabKey, SidebarGroupId>> = {
  analytics: "finance",
  accounting: "finance",
  // Counters is naming/count setup only — not a live pickup board. Keep it out
  // of Operations next to Kitchen/Tables until a real dinner-service queue ships.
  counter: "setup",
};

function presentationGroupFor(key: TabKey, configGroup: ConfigGroup): SidebarGroupId {
  const overridden = KEY_GROUP_OVERRIDE[key];
  if (overridden) return overridden;
  if (configGroup === "settings") return "setup";
  return configGroup;
}

/**
 * FE-only order inside each presentation group. Keys not listed fall at the
 * end in config order. This is the only place order is owned — membership is
 * owned by the JSON.
 */
const GROUP_TAB_ORDER: Record<SidebarGroupId, readonly TabKey[]> = {
  operations: [
    "bills",
    "cash-register",
    "printers",
    "menu",
    "tables",
    "delivery",
  ],
  ai: ["ai-waiter", "director-console", "marketing"],
  management: ["crm", "staff", "schedule", "inventory"],
  finance: ["analytics", "accounting"],
  setup: [
    "counter",
    "business-page",
    "fiscal",
    "plugins",
    "settings",
  ],
};

/** Tabs rendered in the separate TODAY block (not under grouped PRIMARY headers). */
export const TODAY_TABS: readonly TabKey[] = [
  "overview",
  "kitchen",
  "reservations",
] as const;

function buildGroups(): {
  primaryGroups: { id: SidebarGroupId; tabs: TabKey[] }[];
  secondaryTabs: TabKey[];
  primaryTabs: TabKey[];
} {
  const byPresentation = new Map<SidebarGroupId, TabKey[]>();
  for (const id of Object.keys(GROUP_TAB_ORDER) as SidebarGroupId[]) {
    byPresentation.set(id, []);
  }

  for (const row of DASHBOARD_TABS) {
    if (
      row.route_kind !== "dashboard_tab" &&
      !(row.route_kind === "standalone" && row.key === "printers")
    ) {
      continue;
    }
    const key = row.key as TabKey;
    const group = presentationGroupFor(key, row.group);
    byPresentation.get(group)!.push(key);
  }

  // Apply FE ordering within each group.
  for (const [id, tabs] of byPresentation) {
    const order = GROUP_TAB_ORDER[id];
    const rank = new Map(order.map((k, i) => [k, i]));
    tabs.sort((a, b) => {
      const ra = rank.has(a) ? rank.get(a)! : 1000;
      const rb = rank.has(b) ? rank.get(b)! : 1000;
      if (ra !== rb) return ra - rb;
      return a.localeCompare(b);
    });
    byPresentation.set(id, tabs);
  }

  const secondaryTabs = byPresentation.get("setup") ?? [];
  const primaryGroupIds: SidebarGroupId[] = [
    "operations",
    "ai",
    "management",
    "finance",
  ];
  const primaryGroups = primaryGroupIds.map((id) => ({
    id,
    // TODAY tabs stay out of the labeled PRIMARY sections.
    tabs: (byPresentation.get(id) ?? []).filter(
      (t) => !(TODAY_TABS as readonly string[]).includes(t),
    ),
  }));

  // Flat PRIMARY list. Membership = today ∪ primary groups; order is FE-only
  // (matches the pre-Task-32 rail: overview → ops → today-rest → ai → …).
  const primaryMembers = new Set<TabKey>([
    ...TODAY_TABS,
    ...primaryGroups.flatMap((g) => g.tabs),
  ]);
  const PRIMARY_ORDER: readonly TabKey[] = [
    "overview",
    "bills",
    "cash-register",
    "printers",
    "kitchen",
    "reservations",
    "menu",
    "tables",
    "ai-waiter",
    "director-console",
    "marketing",
    "analytics",
    "accounting",
    "crm",
    "delivery",
    "inventory",
    "staff",
    "schedule",
  ];
  const primaryTabs: TabKey[] = [
    ...PRIMARY_ORDER.filter((k) => primaryMembers.has(k)),
    // Any newly added config key not yet listed above lands at the end.
    ...[...primaryMembers].filter((k) => !PRIMARY_ORDER.includes(k)),
  ];

  return { primaryGroups, secondaryTabs, primaryTabs };
}

const BUILT = buildGroups();

/**
 * Tabs that are always visible at the top level (Today + primary groups).
 * Daily-use operational surfaces — not hunted through a disclosure.
 */
export const PRIMARY_TABS: TabKey[] = BUILT.primaryTabs;

/**
 * Labeled sections for the always-visible PRIMARY block.
 * Group ids map to `businessDashboard.sidebarGroups.<id>` i18n keys.
 */
export const PRIMARY_TAB_GROUPS: {
  id: SidebarGroupId;
  tabs: TabKey[];
}[] = BUILT.primaryGroups;

/**
 * Setup / admin-leaning tabs in a labeled Setup group (same chrome as
 * Operations — not a collapsed disclosure). Includes fiscal.
 */
export const SECONDARY_TABS: TabKey[] = BUILT.secondaryTabs;

/** Every dashboard_tab key from config (for parity tests). */
export function allDashboardTabKeysFromConfig(): string[] {
  return DASHBOARD_TABS.filter((r) => r.route_kind === "dashboard_tab").map(
    (r) => r.key,
  );
}

/** Presentation group for a tab key (tests + consumers). */
export function sidebarGroupForTab(key: TabKey): SidebarGroupId | "today" {
  if ((TODAY_TABS as readonly string[]).includes(key)) return "today";
  for (const g of PRIMARY_TAB_GROUPS) {
    if (g.tabs.includes(key)) return g.id;
  }
  if (SECONDARY_TABS.includes(key)) return "setup";
  // Fallback: look up config
  const row = DASHBOARD_TABS.find((r) => r.key === key);
  if (!row) return "setup";
  return presentationGroupFor(key, row.group);
}

// Touch eligible keys so unused-export lint doesn't drop the helper and so
// the build re-runs when the JSON gains a key the TabKey union doesn't cover.
void sidebarEligibleKeys;
