/**
 * Surface catalog of the platform inventory (public surfaces and business-owner
 * dashboard tabs). Used by the local QA crawl pipeline.
 *
 * Status legend: Live | Live (config-dependent) | Partially wired.
 */

type SurfaceKind =
  | "public"
  | "owner-tab"
  | "owner-subtab"
  | "guest"
  | "staff"
  | "admin";

type SurfaceStatus = "live" | "config-dependent" | "partial";

export interface Surface {
  id: string;
  kind: SurfaceKind;
  /** Human label for the report */
  label: string;
  /** Path template. Tokens: {businessId}, {tableCode}, {customUrl} */
  path: string;
  status: SurfaceStatus;
  /** When true, page may show a lock (no LLM configured, AI budget reached) rather than full UI */
  allowLocked?: boolean;
  /** Safe sub-tab query values (?sub=) for multi-panel tabs */
  subTabs?: string[];
  /** Platform inventory section this maps to */
  source: string;
}

/** Owner dashboard tabs from the platform inventory + sidebarConfig (printers added in product). */
export const OWNER_TABS: Surface[] = [
  {
    id: "tab.overview",
    kind: "owner-tab",
    label: "Overview",
    path: "/business/{businessId}/dashboard?tab=overview",
    status: "live",
    source: "inventory § Business-owner dashboard tabs",
  },
  {
    id: "tab.bills",
    kind: "owner-tab",
    label: "Bills",
    path: "/business/{businessId}/dashboard?tab=bills",
    status: "live",
    source: "inventory § Floor ops / bills",
  },
  {
    id: "tab.cash-register",
    kind: "owner-tab",
    label: "Cash register",
    path: "/business/{businessId}/dashboard?tab=cash-register",
    status: "live",
    source: "inventory § Floor ops",
  },
  {
    id: "tab.printers",
    kind: "owner-tab",
    label: "Printers",
    path: "/business/{businessId}/dashboard?tab=printers",
    status: "live",
    source: "sidebarConfig PRIMARY_TABS",
  },
  {
    id: "tab.kitchen",
    kind: "owner-tab",
    label: "Kitchen",
    path: "/business/{businessId}/dashboard?tab=kitchen",
    status: "live",
    source: "inventory § Floor ops / kitchen",
  },
  {
    id: "tab.reservations",
    kind: "owner-tab",
    label: "Reservations",
    path: "/business/{businessId}/dashboard?tab=reservations",
    status: "live",
    source: "inventory § Reservations",
  },
  {
    id: "tab.menu",
    kind: "owner-tab",
    label: "Menu",
    path: "/business/{businessId}/dashboard?tab=menu",
    status: "live",
    source: "inventory § Menu",
  },
  {
    id: "tab.tables",
    kind: "owner-tab",
    label: "Tables",
    path: "/business/{businessId}/dashboard?tab=tables",
    status: "live",
    source: "inventory § Tables",
  },
  {
    id: "tab.ai-waiter",
    kind: "owner-tab",
    label: "AI Waiter",
    path: "/business/{businessId}/dashboard?tab=ai-waiter",
    status: "config-dependent",
    allowLocked: true,
    source: "inventory § AI Waiter",
  },
  {
    id: "tab.director-console",
    kind: "owner-tab",
    label: "Director Console",
    path: "/business/{businessId}/dashboard?tab=director-console",
    status: "config-dependent",
    allowLocked: true,
    source: "inventory § Director Console",
  },
  {
    id: "tab.marketing",
    kind: "owner-tab",
    label: "Marketing",
    path: "/business/{businessId}/dashboard?tab=marketing",
    status: "config-dependent",
    allowLocked: true,
    source: "inventory § Marketing",
  },
  {
    id: "tab.analytics",
    kind: "owner-tab",
    label: "Analytics",
    path: "/business/{businessId}/dashboard?tab=analytics",
    status: "live",
    source: "inventory § Analytics",
  },
  {
    id: "tab.crm",
    kind: "owner-tab",
    label: "CRM",
    path: "/business/{businessId}/dashboard?tab=crm",
    status: "live",
    subTabs: ["customers", "segments", "loyalty"],
    source: "inventory § CRM",
  },
  {
    id: "tab.delivery",
    kind: "owner-tab",
    label: "Delivery",
    path: "/business/{businessId}/dashboard?tab=delivery",
    status: "live",
    source: "inventory § Delivery",
  },
  {
    id: "tab.counter",
    kind: "owner-tab",
    label: "Counter",
    path: "/business/{businessId}/dashboard?tab=counter",
    status: "live",
    source: "inventory § Counter",
  },
  {
    id: "tab.inventory",
    kind: "owner-tab",
    label: "Inventory",
    path: "/business/{businessId}/dashboard?tab=inventory",
    status: "live",
    source: "inventory § Inventory",
  },
  {
    id: "tab.staff",
    kind: "owner-tab",
    label: "Team",
    path: "/business/{businessId}/dashboard?tab=staff",
    status: "live",
    subTabs: ["people", "positions", "communication"],
    source: "inventory § Team / staff",
  },
  {
    id: "tab.schedule",
    kind: "owner-tab",
    label: "Schedule",
    path: "/business/{businessId}/dashboard?tab=schedule",
    status: "live",
    source: "inventory § Schedule",
  },
  {
    id: "tab.business-page",
    kind: "owner-tab",
    label: "Business page",
    path: "/business/{businessId}/dashboard?tab=business-page",
    status: "live",
    source: "inventory § Business page",
  },
  {
    id: "tab.accounting",
    kind: "owner-tab",
    label: "Accounting",
    path: "/business/{businessId}/dashboard?tab=accounting",
    status: "live",
    subTabs: [
      "overview",
      "entries",
      "payroll",
      "invoices",
      "reports",
      "outstanding",
    ],
    source: "inventory § Accounting",
  },
  {
    id: "tab.plugins",
    kind: "owner-tab",
    label: "Plugins",
    path: "/business/{businessId}/dashboard?tab=plugins",
    status: "live",
    source: "inventory § Plugins",
  },

  {
    id: "tab.settings",
    kind: "owner-tab",
    label: "Settings",
    path: "/business/{businessId}/dashboard?tab=settings",
    status: "live",
    source: "inventory § Settings",
  },
  // Setup → Invoices: its own tab key, rendering accounting's invoices view.
  {
    id: "tab.fiscal",
    kind: "owner-tab",
    label: "Invoices (Setup → fiscal)",
    path: "/business/{businessId}/dashboard?tab=fiscal",
    status: "live",
    source: "inventory § Setup → Invoices",
  },
];

/** Public / marketing pages. */
export const PUBLIC_SURFACES: Surface[] = [
  {
    id: "public.home",
    kind: "public",
    label: "Landing",
    path: "/",
    status: "live",
    source: "inventory § Public/marketing",
  },
  {
    id: "public.privacy",
    kind: "public",
    label: "Privacy policy",
    path: "/privacy-policy",
    status: "live",
    source: "inventory § Legal",
  },
  {
    id: "public.terms",
    kind: "public",
    label: "Terms",
    path: "/terms-and-conditions",
    status: "live",
    source: "inventory § Legal",
  },
  {
    id: "public.login",
    kind: "public",
    label: "Login (business)",
    path: "/business/login",
    status: "live",
    source: "auth surfaces",
  },
  {
    id: "public.staff-login",
    kind: "staff",
    label: "Staff login",
    path: "/staff/login",
    status: "live",
    source: "inventory § Staff pages",
  },
];

/** Guest / public business surfaces (paths need resolved tokens). */
export const GUEST_SURFACES: Surface[] = [
  {
    id: "guest.business-page",
    kind: "guest",
    label: "Public business page",
    path: "/b/{customUrl}",
    status: "live",
    source: "inventory § Public business page",
  },
  {
    id: "guest.table-home",
    kind: "guest",
    label: "Guest table home",
    path: "/t/{tableCode}",
    status: "live",
    source: "inventory § QR table journey",
  },
  {
    id: "guest.table-menu",
    kind: "guest",
    label: "Guest table menu",
    path: "/t/{tableCode}/menu",
    status: "live",
    source: "inventory § QR table journey",
  },
  {
    id: "guest.table-bill",
    kind: "guest",
    label: "Guest table bill",
    path: "/t/{tableCode}/bill",
    status: "live",
    source: "inventory § QR table journey",
  },
  {
    id: "guest.table-profile",
    kind: "guest",
    label: "Guest table profile",
    path: "/t/{tableCode}/profile",
    status: "partial",
    source: "inventory § Customer profile",
  },
];

export function expandPath(
  template: string,
  vars: Record<string, string | number>,
): string {
  return template.replace(/\{(\w+)\}/g, (_, key: string) => {
    const v = vars[key];
    if (v === undefined || v === null || v === "") {
      throw new Error(`Missing path token {${key}} for ${template}`);
    }
    return String(v);
  });
}
