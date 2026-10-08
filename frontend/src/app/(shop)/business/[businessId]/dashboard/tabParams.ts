// Audit M2: switching sidebar tabs used to leak query params across tabs
// (a deny-list that only dropped `tableId`), so e.g. Analytics' `?sub=service`
// survived onto Inventario and selected a wrong subview / made URLs unshareable.
//
// This whitelist is the single source of truth for which deep-link params each
// tab owns. On a tab switch we keep only `tab` plus the incoming tab's declared
// params; every other param is dropped. Each entry is backed by an actual
// searchParams read in the tab's component (Session P / L9-12 extends the
// per-rail contract so deep links are addressable, not "intentionally empty"):
//
//   analytics      → "sub"     components/dashboard/Dashboard.tsx via useSubTab
//                    "period"  shared analytics period (today|week|…)
//   bills          → "tableId"  components/business/BillManager.tsx reads "tableId"
//                    "billId"   BillManager consumes it once to open BillDetailsModal
//                    "billCustomer" BillManager filters the list to one CRM customer
//                    "billTab"  active|history sub-view (L5-8 / L21 write-back)
//                    "billPage"/"billSearch"/"billFrom"/"billTo"/"billStatus"
//                               history list filters (L21) — must survive tab
//                               switches that stay on bills
//   accounting/staff/crm → "sub"  this dir's page.tsx seeds their sub-tab from
//                                 getSearchParam(searchParams, "sub")
//   accounting     → "status" PayrollTab seeds status filter (draft|paid|void)
//                    "filter" InvoicesTab seeds filter (needs_attention|…)
//                    "period" shared finance period (L6-2) — survives rail switch
//   settings       → "section" components/business/BusinessSettings.tsx reads "section"
//   business-page  → "section" BusinessPageEditor Contact deep-link (#225)
//   crm            → "sub"   CRM shell sub-tab
//                    "focus"  segment drilldown (lapsed|vip|new|at-risk) (L5-11)
//   tables         → "tableSearch" TableManager seeds its list filter (matches name or table_code)
//                    "tablesView"  live | spaces sub-view (Spaces & Tables)
//                    "spaceId"     deep-link into a space editor from overview
//                    "tableId"     alert deep-link / service_call (F-cand-11)
//   menu           → "menuSearch" MenuBuilder seeds its item search
//   staff          → "staffSearch" StaffManagement seeds its roster search
//   delivery       → "sub" configuration|dispatch|history|drivers|performance (L3-43)
//   reservations   → "reservationId" alert deep-link (F-cand-11)
//                    "reservationsView" list | board (host Board persistence)
//                    "sub" reservations|settings nested destination (#455)
//   inventory      → "sub" items|recipes|activity|settings (#455)
//
// Tabs with no deep-link params (overview, counter, cash-register,
// schedule, kitchen, plugins) intentionally have no entry — they are not
// addressable beyond `?tab=`. Business Page owns `section` so Settings →
// Contact can land on `?tab=business-page&section=contact` (#225).
export const TAB_PARAM_WHITELIST: Record<string, readonly string[]> = {
  analytics: ["sub", "period"],
  bills: [
    "tableId",
    "billId",
    "billCustomer",
    "billTab",
    "billPage",
    "billSearch",
    "billFrom",
    "billTo",
    "billStatus",
  ],
  accounting: ["sub", "status", "filter", "period"],
  staff: ["sub", "staffSearch"],
  crm: ["sub", "focus"],
  settings: ["section"],
  tables: ["tableSearch", "tablesView", "spaceId", "tableId"],
  menu: ["menuSearch"],
  delivery: ["sub"],
  reservations: ["reservationId", "reservationsView", "sub"],
  inventory: ["sub"],
  "business-page": ["section"],
};

/**
 * Safe aliases for common singular/legacy `?tab=` values (L4-1 / F-9).
 * When a present-but-unknown tab param maps here, the dashboard normalizes it
 * (and rewrites the URL via replace) instead of dead-ending on the not-found
 * panel. Canonical TabKeys use hyphens (`ai-waiter`, `director-console`).
 */
export const TAB_ALIASES: Record<string, string> = {
  plugin: "plugins",
  table: "tables",
  bill: "bills",
  reservation: "reservations",
  // Marketing/docs/audit often say "director"; the TabKey is director-console.
  director: "director-console",
  director_console: "director-console",
  // Gate F-9: snake_case / short forms for AI Waiter.
  ai_waiter: "ai-waiter",
  waiter: "ai-waiter",
  // #193: operators say "Counters"; canonical TabKey remains `counter`.
  counters: "counter",
};

/** Resolve a raw `?tab=` value through TAB_ALIASES; returns null if unknown. */
export function resolveTabAlias(raw: string | null | undefined): string | null {
  if (!raw) return null;
  return TAB_ALIASES[raw] ?? null;
}

/**
 * Params that survive a *cross-rail* switch when the destination also declares
 * them (L6-2). `period` is shared across finance rails (analytics / accounting);
 * rail-specific keys like `sub` still do not leak.
 */
export const CROSS_RAIL_SHARED_PARAMS = new Set<string>(["period"]);

// Build the URLSearchParams for a tab switch: always sets `tab`, carries over
// only the incoming tab's whitelisted params from the current URL, then applies
// any explicitly passed deep-link params (which win over carried-over values).
// All params here are single-valued; a repeated key collapses to its last value.
export function nextTabSearchParams(
  current: URLSearchParams,
  nextTab: string,
  extraQuery?: string,
): URLSearchParams {
  const next = new URLSearchParams();
  next.set("tab", nextTab);

  const allowed = new Set(TAB_PARAM_WHITELIST[nextTab] ?? []);
  // A query key such as `sub` can be owned by several rails but still carry a
  // rail-specific value (`analytics?sub=revenue` is not a valid accounting
  // section). Carry deep-link state only while staying on the same rail;
  // cross-rail clicks start at the destination's default section — except for
  // CROSS_RAIL_SHARED_PARAMS that the destination also whitelists (L6-2 period).
  if (current.get("tab") === nextTab) {
    current.forEach((value, key) => {
      if (allowed.has(key)) next.set(key, value);
    });
  } else {
    current.forEach((value, key) => {
      if (CROSS_RAIL_SHARED_PARAMS.has(key) && allowed.has(key)) {
        next.set(key, value);
      }
    });
  }

  if (extraQuery) {
    new URLSearchParams(extraQuery).forEach((value, key) => {
      if (allowed.has(key)) next.set(key, value);
    });
  }

  return next;
}
