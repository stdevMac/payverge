import { PLUGIN } from "@/constants/plugins";

/**
 * Operator marketplace payment order for the BA dinner wedge:
 * real card/checkout paths first (Mercado Pago lead), crypto selective / last.
 * Unknown payment plugins sort after known rails and before crypto.
 */
export const PAYMENT_PLUGIN_ORDER: readonly string[] = [
  PLUGIN.mercadopago,
  PLUGIN.paypal,
  PLUGIN.stripe,
  PLUGIN.usdcPayment,
  PLUGIN.crossChainPayment,
];

const CRYPTO_PAYMENT_NAMES = new Set<string>([
  PLUGIN.usdcPayment,
  PLUGIN.crossChainPayment,
]);

const CARD_PAYMENT_NAMES = new Set<string>([
  PLUGIN.mercadopago,
  PLUGIN.paypal,
  PLUGIN.stripe,
]);

export function isCryptoPaymentPlugin(name: string): boolean {
  return CRYPTO_PAYMENT_NAMES.has(name);
}

export function isCardPaymentPlugin(name: string): boolean {
  return CARD_PAYMENT_NAMES.has(name);
}

export function paymentPluginRank(name: string): number {
  const index = PAYMENT_PLUGIN_ORDER.indexOf(name);
  return index === -1 ? PAYMENT_PLUGIN_ORDER.length : index;
}

type SortablePlugin = {
  name: string;
  category: string;
  display_name: string;
  coming_soon?: boolean;
};

/**
 * Within a category: live plugins first, Coming Soon last; payment rails follow
 * PAYMENT_PLUGIN_ORDER so Mercado Pago leads and crypto never door-leads.
 */
export function sortPluginsForOperatorMarketplace<T extends SortablePlugin>(
  plugins: T[],
): T[] {
  return [...plugins].sort((a, b) => {
    const aSoon = Boolean(a.coming_soon);
    const bSoon = Boolean(b.coming_soon);
    if (aSoon !== bSoon) return aSoon ? 1 : -1;

    if (a.category === "payment" && b.category === "payment") {
      const rankDiff = paymentPluginRank(a.name) - paymentPluginRank(b.name);
      if (rankDiff !== 0) return rankDiff;
    }

    return a.display_name.localeCompare(b.display_name);
  });
}
