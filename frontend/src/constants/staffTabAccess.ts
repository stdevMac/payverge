import type { TabKey } from "@/components/business/sidebar/sidebarConfig";
import { hasAllPerms, hasAnyPerm, hasPerm } from "./permissions";

export type StaffTabResolveOpts = {
  kitchenOrdersEnabled?: boolean;
};

type TabAccessDef = {
  key: TabKey;
  require?: readonly string[];
  requireAny?: readonly string[];
  staffEligible: boolean;
};

/**
 * Product allowlist ∩ permission requirements.
 * Task 32: fiscal is a first-class Setup tab again (fiscal:read).
 */
const TAB_DEFS: readonly TabAccessDef[] = [
  { key: "overview", require: ["overview:read"], staffEligible: true },
  { key: "bills", require: ["bills:read"], staffEligible: true },
  { key: "cash-register", require: ["cash_register:read"], staffEligible: true },
  {
    key: "kitchen",
    requireAny: ["orders:read", "orders:kitchen"],
    staffEligible: true,
  },
  { key: "menu", require: ["menu:read"], staffEligible: true },
  { key: "reservations", require: ["reservations:read"], staffEligible: true },
  {
    key: "analytics",
    requireAny: ["analytics:read", "analytics:sales"],
    staffEligible: true,
  },
  { key: "ai-waiter", require: ["ai_waiter:read"], staffEligible: true },
  { key: "director-console", require: ["director:read"], staffEligible: true },
  { key: "marketing", require: ["marketing:read"], staffEligible: true },
  { key: "tables", require: ["tables:read"], staffEligible: true },
  { key: "crm", require: ["crm:read"], staffEligible: true },
  {
    key: "delivery",
    requireAny: ["delivery:dispatch:read", "delivery:dispatch:write"],
    staffEligible: true,
  },
  { key: "counter", require: ["counter:read"], staffEligible: true },
  { key: "inventory", require: ["inventory:read"], staffEligible: true },
  { key: "staff", require: ["staff:read"], staffEligible: true },
  {
    key: "schedule",
    require: ["schedule:read"],
    staffEligible: true,
  },
  {
    key: "business-page",
    requireAny: ["settings:design", "settings:write"],
    staffEligible: true,
  },
  {
    key: "accounting",
    requireAny: ["financial:read", "accounting:read"],
    staffEligible: true,
  },
  {
    key: "fiscal",
    require: ["fiscal:read"],
    staffEligible: true,
  },
  { key: "printers", require: ["printers:read"], staffEligible: true },
  { key: "plugins", require: ["plugins:read"], staffEligible: true },
  { key: "settings", require: ["settings:read"], staffEligible: true },
];

function tabSatisfied(def: TabAccessDef, perms: readonly string[]): boolean {
  if (!def.staffEligible) return false;
  if (
    def.key === "delivery" &&
    hasPerm(perms, "orders:kitchen") &&
    !hasPerm(perms, "bills:create")
  ) {
    return false;
  }
  if (def.key === "schedule") {
    return (
      hasPerm(perms, "schedule:read") &&
      hasAnyPerm(perms, [
        "schedule:write",
        "schedule:publish",
        "schedule:approve",
      ])
    );
  }
  if (def.require?.length) {
    if (!hasAllPerms(perms, def.require)) return false;
  }
  if (def.requireAny?.length) {
    if (!hasAnyPerm(perms, def.requireAny)) return false;
  }
  // schedule is handled above; remaining defs must declare require/requireAny
  if (!def.require?.length && !def.requireAny?.length) {
    return false;
  }
  return true;
}

export function resolveStaffTabs(
  permissions: readonly string[],
  opts?: StaffTabResolveOpts,
): TabKey[] {
  let tabs = TAB_DEFS.filter((d) => tabSatisfied(d, permissions)).map(
    (d) => d.key,
  );

  if (opts?.kitchenOrdersEnabled === false) {
    const canActivate = hasPerm(permissions, "settings:write");
    const isKitchenOnlySurface =
      hasPerm(permissions, "orders:kitchen") &&
      !hasPerm(permissions, "bills:create");
    tabs = tabs.filter((t) => {
      if (t === "bills" && !canActivate) return false;
      if (t === "kitchen" && !canActivate && !isKitchenOnlySurface) return false;
      return true;
    });
  }

  return tabs;
}
