// Single source of truth for backend plugin `name` values. The frontend
// switches on these in several places (config factory, payment-method icons,
// completion reports, image map); duplicating the raw string literals meant a
// backend rename could silently route to a `default:` fallback. Importing the
// PLUGIN constants instead means a rename is a one-line change and the
// PluginName union gives the switches compile-time exhaustiveness.
//
// Note: callers intentionally keep their tolerant `default:` branches. We do
// NOT validate plugin.name with a throwing parser at the API boundary — a
// newly added backend plugin should degrade to the generic UI, not crash the
// page. PluginName is for our own literals, not a contract that the backend can
// only ever return these.

export const PLUGIN = {
  usdcPayment: "usdc_payment",
  crossChainPayment: "cross_chain_payment",
  stripe: "stripe",
  paypal: "paypal",
  mercadopago: "mercadopago",
  telegram: "telegram",
  trustpilot: "trustpilot",
  dailyEmailReport: "daily_email_report",
  weeklyEmailReport: "weekly_email_report",
} as const;

export type PluginName = (typeof PLUGIN)[keyof typeof PLUGIN];

// The venue's own counter/cash rail. Deliberately NOT part of PLUGIN: no
// plugin implements it, it carries no processor credentials, and it is settled
// by staff at the register. The guest payment-rail endpoint lists it in
// `plugins` (with `settlement: "counter"`) so a venue with no processor
// configured is not reported as having nothing to tap, so guest UI must filter
// it out of the plugin rails and render it with its own localized cashier
// affordance instead of creating a plugin checkout for it.
export const HOUSE_COUNTER_RAIL = "counter_cash";

export const KNOWN_PLUGIN_NAMES = Object.values(PLUGIN) as PluginName[];

export function isKnownPluginName(name: string | undefined | null): name is PluginName {
  return !!name && (KNOWN_PLUGIN_NAMES as string[]).includes(name);
}
