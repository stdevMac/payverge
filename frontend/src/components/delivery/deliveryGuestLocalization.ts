/**
 * Shared guest-facing localization helpers for the delivery flow.
 *
 * The backend delivery quote/checkout endpoints return hardcoded English
 * strings (see backend/internal/services/delivery_v1.go). Guest surfaces run
 * on the 21-locale tier, so we never render those messages verbatim — we map
 * the machine-readable reason_code (or, for thrown errors, the message text)
 * to guest translation keys instead.
 */

type TranslateFn = (key: string) => unknown;

/** Backend error-message patterns → guest translation keys. */
const BACKEND_ERROR_KEYS: Array<[RegExp, string]> = [
  [/email is required/i, "businessPage.guestDelivery.emailRequired"],
  [/capacity/i, "businessPage.deliveryQuote.capacityFull"],
  [/outside.*delivery area|zone/i, "businessPage.deliveryQuote.outsideArea"],
  [/minimum/i, "businessPage.deliveryQuote.belowMinimum"],
  [/outside active hours|unavailable/i, "businessPage.deliveryQuote.outsideHours"],
];

/**
 * Map a raw backend error message to a localized guest string.
 * Unknown messages fall back to `fallbackKey` — never to the raw English text.
 */
export function localizeBackendError(
  message: string,
  t: TranslateFn,
  fallbackKey = "businessPage.checkout.errorFallback",
): string {
  for (const [pattern, key] of BACKEND_ERROR_KEYS) {
    if (pattern.test(message)) return t(key) as string;
  }
  return t(fallbackKey) as string;
}

/**
 * Quote reason_code → guest translation key.
 * Codes come from backend/internal/services/delivery_v1.go QuoteDelivery.
 */
const QUOTE_REASON_KEYS: Record<string, string> = {
  quote_available: "businessPage.deliveryQuote.addressDeliverable",
  below_minimum: "businessPage.deliveryQuote.addressDeliverable",
  delivery_disabled: "businessPage.deliveryQuote.deliveryDisabled",
  outside_delivery_hours: "businessPage.deliveryQuote.outsideHours",
  // Legacy reason retained for older backends; public quote now emits
  // delivery_unavailable so ops capacity is not advertised (#292).
  capacity_reached: "businessPage.deliveryQuote.capacityFull",
  delivery_unavailable: "businessPage.deliveryQuote.notAvailable",
  zone_unavailable: "businessPage.deliveryQuote.outsideArea",
};

/** Localized headline for a delivery quote result. Never returns backend English. */
export function localizeQuoteReason(reasonCode: string, t: TranslateFn): string {
  const key = QUOTE_REASON_KEYS[reasonCode] || "businessPage.deliveryQuote.notAvailable";
  return t(key) as string;
}
