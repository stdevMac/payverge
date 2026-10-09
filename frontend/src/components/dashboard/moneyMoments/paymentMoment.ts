/**
 * Pure helpers for "money moments" — the payment-received celebration.
 *
 * Deliberately free of React so the trust-critical parsing (never celebrate a
 * payment that isn't real) is exhaustively unit-tested. The SSE `payment.received`
 * payload is `{ bill_id, amount, method }` where `amount` is already dollars
 * (the backend converts cents → dollars before publishing — see the money wire
 * contract).
 */

export interface ParsedPayment {
  /** Amount in dollars. Guaranteed finite and > 0. */
  amount: number;
  /** Raw method string from the backend ("crypto", "card", …); "" if absent. */
  method: string;
  /** Bill id when present, else null. */
  billId: number | null;
}

/**
 * Validate + extract a payment payload. Returns null when the amount isn't a
 * real positive number — we would rather show nothing than celebrate a phantom
 * payment, which would erode the operator's trust in the money numbers.
 */
export function parsePaymentEvent(data: Record<string, unknown>): ParsedPayment | null {
  const rawAmount = data?.amount;
  if (typeof rawAmount !== "number" || !Number.isFinite(rawAmount) || rawAmount <= 0) {
    return null;
  }

  const method = typeof data?.method === "string" ? data.method : "";
  const billId = typeof data?.bill_id === "number" ? data.bill_id : null;

  return { amount: rawAmount, method, billId };
}

const KNOWN_METHOD_KEYS: Record<string, string> = {
  crypto: "methodCrypto",
  "cross-chain": "methodCrossChain",
  card: "methodCard",
  stripe: "methodCard",
  cash: "methodCash",
  paypal: "methodPaypal",
  mercadopago: "methodMercadoPago",
};

function titleCase(value: string): string {
  return value
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

/**
 * Friendly label for a payment method. Known methods resolve through the
 * localized `businessDashboard.moneyMoments.*` keys; anything unknown falls
 * back to a title-cased version of the raw string so we never render an ugly
 * machine token.
 */
export function formatMethodLabel(method: string, t: (key: string) => string): string {
  if (!method.trim()) return "";
  const known = KNOWN_METHOD_KEYS[method.toLowerCase()];
  if (known) return t(`businessDashboard.moneyMoments.${known}`);
  return titleCase(method);
}
