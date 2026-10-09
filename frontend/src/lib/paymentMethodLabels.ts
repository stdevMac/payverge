import { PLUGIN } from "@/constants/plugins";

export type TranslateFn = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/**
 * Single source of truth for guest-facing payment-method display names.
 * Replaces the drifting maps in PaymentStatusChecker and the bill page.
 * Brand names (PayPal, MercadoPago, USDT) are identical in every language;
 * generic instruments localize through paymentMethods.* guest keys.
 */
export function paymentMethodLabel(
  method: string | undefined | null,
  t: TranslateFn,
): string {
  const key = (method ?? "").trim().toLowerCase();
  if (!key) return "";
  switch (key) {
    case "crypto":
    case PLUGIN.usdcPayment:
      return t("paymentMethods.cryptoUsdc");
    case "cross-chain":
    case PLUGIN.crossChainPayment:
      return t("paymentMethods.crossChain");
    case PLUGIN.stripe:
      return t("paymentMethods.card");
    case PLUGIN.paypal:
      return "PayPal";
    case PLUGIN.mercadopago:
      return "MercadoPago";
    default:
      // Unknown id → readable words ("cross_chain" → "Cross Chain") so a new
      // backend plugin degrades to something human, never a machine token.
      return key.replace(/[_-]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
  }
}

/** Split-share tender codes (backend BillSplitShare.Tender) → display names. */
export function splitTenderLabel(
  tender: string | undefined | null,
  t: TranslateFn,
): string {
  const key = (tender ?? "").trim().toLowerCase();
  if (!key) return "";
  switch (key) {
    case "cash":
      return t("paymentMethods.cash");
    case "plugin":
    case "card":
      return t("paymentMethods.card");
    default:
      return paymentMethodLabel(key, t);
  }
}
