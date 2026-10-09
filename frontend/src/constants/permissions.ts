/**
 * FE permission catalog — single source of truth for permission string literals
 * used by staff nav, grant/revoke UI, and capability checks.
 *
 * Synced to backend/internal/server/rbac.go via permissions.contract.test.ts.
 * Do not invent or omit strings; update this file when rbac.go gains/drops Permission constants.
 */

export const ALL_PERMISSIONS = [
  // Business Management
  "business:read",
  "business:write",
  "business:delete",
  "business:settings",

  // Staff Management
  "staff:read",
  "staff:write",
  "staff:invite",
  "staff:remove",
  "staff:roles",
  "staff:delete",
  "staff:role",
  "staff:permissions",
  "staff:deactivate",
  "staff:reactivate",
  "staff:audit",
  "staff:admin",

  // Menu Management
  "menu:read",
  "menu:write",
  "menu:translate",
  "menu:categories",
  "menu:items",

  // Table Management
  "tables:read",
  "tables:write",
  "tables:create",
  "tables:delete",
  "tables:qr",

  // Bill Management
  "bills:read",
  "bills:write",
  "bills:create",
  "bills:close",
  "bills:items",
  "bills:payment",
  "bills:refund",
  // Wave 4 noncustodial crypto refunds (owner-only by default)
  "refunds:crypto:request",
  "refunds:crypto:approve",

  // Order Management
  "orders:read",
  "orders:write",
  "orders:create",
  "orders:status",
  "orders:kitchen",

  // Analytics & Reporting
  "analytics:read",
  "analytics:sales",
  "analytics:tips",
  "analytics:items",
  "reports:read",
  "reports:export",

  // Financial Operations
  "financial:read",
  "financial:write",
  "financial:withdraw",
  "financial:blockchain",
  "payroll:write",

  // Plugin Management
  "plugins:read",
  "plugins:write",
  "plugins:config",
  "plugins:payments",

  // Delivery Operations
  "delivery:settings:read",
  "delivery:settings:write",
  "delivery:dispatch:read",
  "delivery:dispatch:write",
  "delivery:drivers:read",
  "delivery:drivers:write",

  // Settings & Configuration
  "settings:read",
  "settings:write",
  "settings:design",
  "settings:google",
  "settings:currency",
  "settings:language",
  "currencies:read",
  "currencies:write",
  "languages:read",
  "languages:write",

  // Tenant-scoped File Management
  "files:upload",
  "files:delete",

  // Counter Operations
  "counter:read",
  "counter:write",
  "counter:settings",

  // Overview Dashboard
  "overview:read",
  "overview:kpi",

  // CRM
  "crm:read",
  "crm:write",
  "crm:export",

  // Inventory Management
  "inventory:read",
  "inventory:write",
  "inventory:adjust",
  "inventory:recipes",

  // Reservations Management
  "reservations:read",
  "reservations:write",
  "reservations:create",
  "reservations:delete",
  "reservations:settings",

  // Director Console
  "director:read",
  "director:write",

  // Ops Assistant
  "assistant:read",
  "assistant:write",

  // AI Waiter
  "ai_waiter:read",
  "ai_waiter:reply",
  "ai_waiter:insights",
  "ai_waiter:write",

  // Marketing
  "marketing:read",
  "marketing:write",

  // Thermal Printer Support
  "printers:read",
  "printers:write",
  "print:bill",
  "print:receipt",

  // Cash Register / Caja
  "cash_register:read",
  "cash_register:operate",

  // Operational Alerts
  "alerts:read",
  "alerts:claim",
  "alerts:resolve",
  "alerts:settings",

  // Fiscal Compliance
  "fiscal:read",
  "fiscal:write",
  "fiscal:issue",
  "fiscal:retry",
  "fiscal:export",
  "fiscal:credit",
  "fiscal:credentials",

  // Staff Scheduling
  "schedule:read",
  "schedule:write",
  "schedule:publish",
  "schedule:approve",
  "schedule:self",

  // Time clock
  "timeclock:punch",
  "timeclock:manage",

  // Staff chat & announcements
  "chat:read",
  "chat:send",
  "chat:announce",
  "chat:moderate",

  // Staff Engagement
  "checklist:complete",
  "checklist:manage",
  "doc:read",
  "doc:manage",
  "recognition:send",
  "poll:vote",
  "poll:manage",
] as const;

export function hasPerm(
  perms: readonly string[] | undefined | null,
  p: string,
): boolean {
  return !!perms?.includes(p);
}

export function hasAllPerms(
  perms: readonly string[] | undefined | null,
  required: readonly string[],
): boolean {
  if (!perms || required.length === 0) return false;
  return required.every((p) => perms.includes(p));
}

export function hasAnyPerm(
  perms: readonly string[] | undefined | null,
  options: readonly string[],
): boolean {
  if (!perms || options.length === 0) return false;
  return options.some((p) => perms.includes(p));
}

/**
 * Operator-facing permission label (L5-23). Looks up
 * `rbacPermissions.<slug>.label` via the provided translator; falls back to
 * the raw slug so new backend permissions degrade gracefully.
 */
export function permissionLabel(
  perm: string,
  t: (key: string) => string,
): string {
  const key = `rbacPermissions.${perm}.label`;
  const label = t(key);
  if (!label || label === key || label.includes("rbacPermissions.") || label === "Label") {
    return perm;
  }
  return label;
}

/** One-line description helper; null when missing. */
export function permissionDescription(
  perm: string,
  t: (key: string) => string,
): string | null {
  const key = `rbacPermissions.${perm}.description`;
  const desc = t(key);
  if (
    !desc ||
    desc === key ||
    desc.includes("rbacPermissions.") ||
    desc === "Description"
  ) {
    return null;
  }
  return desc;
}

/**
 * Grouped for the grant/revoke UI. Every ALL_PERMISSIONS entry must appear in
 * exactly one category (enforced by permissions.contract.test.ts).
 */
export const PERMISSION_CATEGORIES: Record<string, readonly string[]> = {
  business: [
    "business:read",
    "business:write",
    "business:delete",
    "business:settings",
  ],
  staff: [
    "staff:read",
    "staff:write",
    "staff:invite",
    "staff:remove",
    "staff:roles",
    "staff:delete",
    "staff:role",
    "staff:permissions",
    "staff:deactivate",
    "staff:reactivate",
    "staff:audit",
    "staff:admin",
  ],
  menu: [
    "menu:read",
    "menu:write",
    "menu:translate",
    "menu:categories",
    "menu:items",
  ],
  tables: [
    "tables:read",
    "tables:write",
    "tables:create",
    "tables:delete",
    "tables:qr",
  ],
  bills: [
    "bills:read",
    "bills:write",
    "bills:create",
    "bills:close",
    "bills:items",
    "bills:payment",
    "bills:refund",
    "refunds:crypto:request",
    "refunds:crypto:approve",
  ],
  orders: [
    "orders:read",
    "orders:write",
    "orders:create",
    "orders:status",
    "orders:kitchen",
  ],
  analytics: [
    "analytics:read",
    "analytics:sales",
    "analytics:tips",
    "analytics:items",
    "reports:read",
    "reports:export",
  ],
  financial: [
    "financial:read",
    "financial:write",
    "financial:withdraw",
    "financial:blockchain",
    "payroll:write",
  ],
  plugins: [
    "plugins:read",
    "plugins:write",
    "plugins:config",
    "plugins:payments",
  ],
  delivery: [
    "delivery:settings:read",
    "delivery:settings:write",
    "delivery:dispatch:read",
    "delivery:dispatch:write",
    "delivery:drivers:read",
    "delivery:drivers:write",
  ],
  settings: [
    "settings:read",
    "settings:write",
    "settings:design",
    "settings:google",
    "settings:currency",
    "settings:language",
    "currencies:read",
    "currencies:write",
    "languages:read",
    "languages:write",
    "files:upload",
    "files:delete",
  ],
  counter: ["counter:read", "counter:write", "counter:settings"],
  overview: ["overview:read", "overview:kpi"],
  crm: ["crm:read", "crm:write", "crm:export"],
  inventory: [
    "inventory:read",
    "inventory:write",
    "inventory:adjust",
    "inventory:recipes",
  ],
  reservations: [
    "reservations:read",
    "reservations:write",
    "reservations:create",
    "reservations:delete",
    "reservations:settings",
  ],
  director: ["director:read", "director:write"],
  assistant: ["assistant:read", "assistant:write"],
  ai_waiter: [
    "ai_waiter:read",
    "ai_waiter:reply",
    "ai_waiter:insights",
    "ai_waiter:write",
  ],
  marketing: ["marketing:read", "marketing:write"],
  printers: [
    "printers:read",
    "printers:write",
    "print:bill",
    "print:receipt",
  ],
  cash_register: ["cash_register:read", "cash_register:operate"],
  alerts: [
    "alerts:read",
    "alerts:claim",
    "alerts:resolve",
    "alerts:settings",
  ],
  fiscal: [
    "fiscal:read",
    "fiscal:write",
    "fiscal:issue",
    "fiscal:retry",
    "fiscal:export",
    "fiscal:credit",
    "fiscal:credentials",
  ],
  schedule: [
    "schedule:read",
    "schedule:write",
    "schedule:publish",
    "schedule:approve",
    "schedule:self",
  ],
  timeclock: ["timeclock:punch", "timeclock:manage"],
  chat: ["chat:read", "chat:send", "chat:announce", "chat:moderate"],
  engagement: [
    "checklist:complete",
    "checklist:manage",
    "doc:read",
    "doc:manage",
    "recognition:send",
    "poll:vote",
    "poll:manage",
  ],
};
