export enum PaymentMethod {
  CRYPTO = 0,
  CASH = 1,
  CARD = 2,
  VENMO = 3,
  OTHER = 4,
}

declare const moneyStringBrand: unique symbol;

export type MoneyString = string & { readonly [moneyStringBrand]: true };

export function asMoneyString(amount: number | string): MoneyString {
  if (typeof amount === "number") {
    if (!Number.isFinite(amount)) {
      throw new Error("Dollar amount must be finite");
    }

    const cents = Math.round(amount * 100);
    if (!Number.isSafeInteger(cents)) {
      throw new Error("Dollar amount is too large");
    }
    if (Math.abs(amount * 100 - cents) > Number.EPSILON * 100) {
      throw new Error("Dollar amounts must use cents precision");
    }
    if (cents <= 0) {
      throw new Error("Dollar amount must be greater than zero");
    }

    return (cents / 100).toFixed(2) as MoneyString;
  }

  const trimmed = amount.trim();
  if (!/^\d+(?:\.\d{1,2})?$/.test(trimmed)) {
    throw new Error("Dollar amounts must use cents precision");
  }

  const [wholePart, centsPart = ""] = trimmed.split(".");
  const whole = wholePart.replace(/^0+(?=\d)/, "") || "0";
  const cents = centsPart.padEnd(2, "0");
  const normalized = `${whole}.${cents}`;

  if (normalized === "0.00") {
    throw new Error("Dollar amount must be greater than zero");
  }

  return normalized as MoneyString;
}

/**
 * Normalize a free-text money field into a clean 2-decimal string, or null if
 * it can't be represented in cents. Accepts a comma OR dot as the decimal
 * separator (es-AR operators type "10,50"), strips thousands separators, and
 * rejects >2 decimal places (".999") and non-positive amounts. This is the
 * single validation gate that must agree with `asMoneyString` below — before,
 * `parseFloat` validation let "10,50"/"10.999"/".50" pass then `asMoneyString`
 * threw a generic failure at submit time (R3-BP-6).
 */
export function normalizeMoneyInput(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;

  // Treat the LAST separator as the decimal point; earlier separators are
  // thousands grouping and get removed. Handles "1.234,56", "1,234.56",
  // "10,50", "10.50", "1234".
  const lastComma = trimmed.lastIndexOf(",");
  const lastDot = trimmed.lastIndexOf(".");
  const decimalIdx = Math.max(lastComma, lastDot);

  let whole: string;
  let cents: string;
  if (decimalIdx === -1) {
    whole = trimmed;
    cents = "";
  } else {
    whole = trimmed.slice(0, decimalIdx);
    cents = trimmed.slice(decimalIdx + 1);
  }

  whole = whole.replace(/[.,]/g, "");
  // A leading-decimal shorthand (".50") leaves an empty whole part — treat it
  // as zero dollars rather than rejecting it.
  if (whole === "") whole = "0";
  if (!/^\d+$/.test(whole)) return null;
  if (cents !== "" && !/^\d{1,2}$/.test(cents)) return null;

  const normalizedCents = cents.padEnd(2, "0");
  const wholeTrimmed = whole.replace(/^0+(?=\d)/, "") || "0";
  const normalized = `${wholeTrimmed}.${normalizedCents}`;
  if (normalized === "0.00") return null;
  return normalized;
}

export interface AlternativePayment {
  id: string;
  billId: string;
  participantName?: string;
  participantAddress: string;
  amount: string; // Amount in micro USDC
  paymentMethod: PaymentMethod;
  timestamp: number;
  verified: boolean;
}

export interface BillPaymentBreakdown {
  totalAmount: string; // Total bill amount
  cryptoPaid: string; // Amount paid via crypto
  alternativePaid: string; // Amount paid via alternative methods
  remaining: string; // Remaining amount to be paid
  isComplete: boolean; // Whether bill is fully paid
}

export interface AlternativePaymentRequest {
  billId: string;
  participantAddress?: string;
  participantName?: string;
  amount: MoneyString;
  tipAmount?: MoneyString;
  paymentMethod: PaymentMethod;
  businessConfirmation: boolean;
  requestId?: string;
  idempotencyKey?: string;
}

export interface AlternativePaymentResponse {
  success: boolean;
  transactionHash?: string;
  message: string;
  paymentBreakdown: BillPaymentBreakdown;
}

export interface PendingAlternativePayment {
  id: string;
  billId: string;
  participantName?: string;
  participantAddress: string;
  amount: string;
  paymentMethod: PaymentMethod;
  timestamp: number;
  expiresAt?: string;
  status:
    | "pending"
    | "confirmed"
    | "failed"
    | "refunded"
    | "expired"
    | "cancelled"
    | "rejected";
}

// UI-specific types
export interface PaymentMethodOption {
  value: PaymentMethod;
  label: string;
  icon: string;
  description: string;
}

export const PAYMENT_METHOD_OPTIONS: PaymentMethodOption[] = [
  {
    value: PaymentMethod.CRYPTO,
    label: "Crypto (USDC)",
    icon: "₿",
    description: "Pay with USDC on blockchain",
  },
  {
    value: PaymentMethod.CASH,
    label: "Cash",
    icon: "💵",
    description: "Pay with physical cash",
  },
  {
    value: PaymentMethod.CARD,
    label: "Credit/Debit Card",
    icon: "💳",
    description: "Pay with credit or debit card",
  },
  {
    value: PaymentMethod.VENMO,
    label: "Venmo/PayPal",
    icon: "📱",
    description: "Pay with Venmo or PayPal",
  },
  {
    value: PaymentMethod.OTHER,
    label: "Other",
    icon: "🔄",
    description: "Other payment method",
  },
];

function getPaymentMethodLabel(method: PaymentMethod): string {
  const option = PAYMENT_METHOD_OPTIONS.find((opt) => opt.value === method);
  return option?.label || "Unknown";
}

// i18n key suffix for each method, so render sites can translate the label
// instead of showing the hardcoded English `label` above. Keys live under
// `alternativePaymentManager.paymentMethods.*` (see the message bundles).
const PAYMENT_METHOD_I18N_KEY: Record<PaymentMethod, string> = {
  [PaymentMethod.CRYPTO]: "crypto",
  [PaymentMethod.CASH]: "cash",
  [PaymentMethod.CARD]: "card",
  [PaymentMethod.VENMO]: "venmo",
  [PaymentMethod.OTHER]: "other",
};

/**
 * Translate a PaymentMethod label through the provided translation function.
 * `tMethods` receives the bare key suffix (e.g. "cash") and should resolve it
 * under whatever namespace the caller owns. Falls back to the hardcoded English
 * label when the translation is missing/echoed.
 */
export function translatePaymentMethodLabel(
  method: PaymentMethod,
  tMethods: (keySuffix: string) => string,
): string {
  const keySuffix = PAYMENT_METHOD_I18N_KEY[method] ?? "unknown";
  const translated = tMethods(keySuffix);
  if (translated && translated !== keySuffix) {
    return translated;
  }
  return getPaymentMethodLabel(method);
}

export function getPaymentMethodIcon(method: PaymentMethod): string {
  const option = PAYMENT_METHOD_OPTIONS.find((opt) => opt.value === method);
  return option?.icon || "❓";
}

// Internal storage uses micro-dollars (dollars × 1_000_000) for legacy
// compatibility with formatUSDCAmount. Backend JSON always emits dollars.

export function dollarsToAmountMicro(
  dollars: number | string | undefined,
): string {
  const parsed =
    typeof dollars === "number" ? dollars : parseFloat(String(dollars ?? 0));
  if (!Number.isFinite(parsed)) {
    return "0";
  }
  return Math.round(parsed * 1_000_000).toString();
}

export function amountMicroToDollars(
  micro: string | number | undefined,
): number {
  const parsed =
    typeof micro === "number" ? micro : parseFloat(String(micro ?? 0));
  if (!Number.isFinite(parsed)) {
    return 0;
  }
  return parsed / 1_000_000;
}

export function formatUSDCAmount(amount: string): string {
  return amountMicroToDollars(amount).toFixed(2);
}
