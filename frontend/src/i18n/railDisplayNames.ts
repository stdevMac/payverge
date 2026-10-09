/**
 * Root E / F-cand-5 — canonical rail display-name keys.
 *
 * Rail *keys* live in `config/dashboard-tabs.json`. Their *display names* used
 * to be scattered across `businessDashboard` namespaces, which is how the same
 * rail ended up with three Spanish names. Sidebar and every cross-reference
 * must read these keys (under the `businessDashboard` prefix the operator
 * provider already applies).
 *
 * Coordinator naming decision (reversible one-string change):
 *   bills  → Spanish "Cuentas"  (restaurant check)
 *   fiscal → Spanish "Facturas" (ARCA/AFIP official document)
 */
export const RAIL_LABEL_KEYS = {
  bills: "tabs.bills",
  fiscal: "tabs.fiscal",
} as const;

export type RailLabelKey = keyof typeof RAIL_LABEL_KEYS;

/** Spanish canonical display names — locked by the rail-names guard test. */
export const RAIL_CANONICAL_ES: Record<RailLabelKey, string> = {
  bills: "Cuentas",
  fiscal: "Facturas",
};
