import type { LucideIcon } from "lucide-react";
import {
  LayoutDashboard,
  Users,
  Store,
  Puzzle,
  FlaskConical,
  BarChart3,
  Bug,
  Mail,
  LifeBuoy,
  FileText,
  Languages,
} from "lucide-react";

export type AdminNavItem = {
  key: string;
  label: string;
  href: string;
  icon: LucideIcon;
  description?: string;
};

export type AdminNavGroup = {
  key: string;
  label: string;
  items: AdminNavItem[];
};

export const ADMIN_NAV_GROUPS: AdminNavGroup[] = [
  {
    key: "overview",
    label: "Overview",
    items: [
      {
        key: "dashboard",
        label: "Dashboard",
        href: "/admin",
        icon: LayoutDashboard,
        description: "Platform health and KPIs",
      },
    ],
  },
  {
    key: "customers",
    label: "Customers",
    items: [
      {
        key: "users",
        label: "Users",
        href: "/admin/users",
        icon: Users,
        description: "Accounts and admin actions",
      },
      {
        key: "businesses",
        label: "Businesses",
        href: "/admin/businesses",
        icon: Store,
        description: "Venues, suspend/reactivate",
      },
      {
        key: "escalations",
        label: "Escalations",
        href: "/admin/escalations",
        icon: LifeBuoy,
        description: "Ops assistant support handoffs",
      },
    ],
  },
  {
    key: "integrations",
    label: "Integrations",
    items: [
      {
        key: "plugins",
        label: "Plugins",
        href: "/admin/plugins",
        icon: Puzzle,
        description: "Payment integrations registry",
      },
    ],
  },
  {
    key: "platform",
    label: "Platform",
    items: [
      {
        key: "analytics",
        label: "Analytics",
        href: "/admin/analytics",
        icon: BarChart3,
        description: "Page views, sessions, funnel",
      },
      {
        key: "translations",
        label: "Translations",
        href: "/admin/translations",
        icon: Languages,
        description: "Missing i18n keys from telemetry",
      },
      {
        key: "fiscal",
        label: "Fiscal",
        href: "/admin/fiscal",
        icon: FileText,
        description: "AFIP jobs, receipts, requeue",
      },
      {
        key: "errors",
        label: "Error Logs",
        href: "/admin/errors",
        icon: Bug,
        description: "Backend and frontend failures",
      },
      {
        key: "emails",
        label: "Emails",
        href: "/admin/emails",
        icon: Mail,
        description: "Operational broadcasts",
      },
      {
        key: "demo",
        label: "Demo Center",
        href: "/admin/demo",
        icon: FlaskConical,
        description: "Isolated demo data engine",
      },
    ],
  },
];

export const ADMIN_NAV_ITEMS: AdminNavItem[] = ADMIN_NAV_GROUPS.flatMap(
  (group) => group.items,
);

export function findAdminNavItem(pathname: string): AdminNavItem | undefined {
  if (pathname === "/admin") {
    return ADMIN_NAV_ITEMS.find((item) => item.href === "/admin");
  }
  return ADMIN_NAV_ITEMS.find(
    (item) => item.href !== "/admin" && pathname.startsWith(item.href),
  );
}

