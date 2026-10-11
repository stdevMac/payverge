/**
 * Storefront hero / sticky-bar CTA derivation (plan 3.1 / 3.2).
 *
 * Pure logic, extracted so the hero, the sticky tab bar, and tests all agree
 * on the hierarchy without re-deriving it inline:
 *   1. "Book a table"  — when reservations are enabled (highest intent).
 *   2. "Order delivery" — when delivery is enabled and reservations are not.
 *   3. "View menu"     — always-available fallback.
 * The hero renders the primary CTA + "View menu" as secondary (unless the
 * menu IS the primary) + the existing Call button — never more than three.
 */

export type StorefrontPrimaryCtaKind = "reservations" | "delivery" | "menu";

export interface StorefrontCtaFeatures {
  hasReservations?: boolean;
  hasDelivery?: boolean;
}

export function resolvePrimaryStorefrontCta({
  hasReservations,
  hasDelivery,
}: StorefrontCtaFeatures): StorefrontPrimaryCtaKind {
  if (hasReservations) return "reservations";
  if (hasDelivery) return "delivery";
  return "menu";
}

/** Translation keys for each primary CTA kind (guest bundle: businessPage.hero.*). */
export const STOREFRONT_CTA_LABEL_KEYS: Record<StorefrontPrimaryCtaKind, string> = {
  reservations: "businessPage.hero.bookTableCta",
  delivery: "businessPage.hero.orderDeliveryCta",
  menu: "businessPage.hero.viewMenuCta",
};

/** English fallbacks for tests / provider-less renders (hero t() returns "" outside the guest provider). */
export const STOREFRONT_CTA_FALLBACKS: Record<StorefrontPrimaryCtaKind, string> = {
  reservations: "Book a table",
  delivery: "Order delivery",
  menu: "View Menu",
};
