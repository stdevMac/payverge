import {
  getApiErrorCode,
  getApiErrorMessage,
  isApiNetworkError,
} from "@/utils/apiError";

export interface GuestPaymentErrorInfo {
  code?: string;
  message?: string;
  isNetwork: boolean;
}

export interface GuestPaymentErrorPresentation {
  messageKey: string;
  params?: Record<string, string | number>;
  preserveCart: true;
}

export function guestPaymentErrorInfo(err: unknown): GuestPaymentErrorInfo {
  return {
    code: getApiErrorCode(err),
    message: getApiErrorMessage(err),
    isNetwork: isApiNetworkError(err),
  };
}

const CODE_TO_KEY: Record<string, string> = {
  amount_not_representable: "payment.errors.generic",
  awaiting_confirmations: "payment.errors.awaitingConfirmations",
  verification_unavailable: "payment.errors.verificationUnavailable",
  payment_failed: "payment.errors.paymentFailed",
  payment_declined: "payment.errors.paymentDeclined",
  insufficient_funds: "payment.errors.insufficientFunds",
  split_share_conflict: "payment.errors.splitShareConflict",
  split_not_open: "payment.errors.splitNotOpen",
  plugin_unavailable: "payment.errors.pluginUnavailable",
  // A server administrator suspended or closed the venue.
  business_unavailable: "menu.orderingDisabled",
  crypto_quote_expired: "payment.errors.cryptoQuoteExpired",
  // Guest USDC payer binding: a transfer only settles the quote whose exact
  // amount it carries, mined after that quote was issued.
  amount_mismatch: "payment.errors.cryptoTransferNotForQuote",
  transfer_predates_quote: "payment.errors.cryptoTransferNotForQuote",
  // The transfer is already recorded as a payment (it paid another bill, or
  // this bill under another quote); staff get a tx_hash_conflict alert.
  crypto_tx_already_recorded: "payment.errors.cryptoTransferNotForQuote",
  crypto_quote_used: "payment.errors.cryptoQuoteUsed",
  crypto_quote_limit: "payment.errors.generic",
  quote_invalid: "payment.errors.generic",
  auth_failed: "payment.errors.authFailed",
  AUTH_FAILED: "payment.errors.authFailed",
  // Cashier / alternative-payment request path (GUEST-001)
  payment_request_failed: "payment.errors.cashierRequestFailed",
  bill_not_payable: "payment.errors.billNotPayable",
  // The venue rotated its payout wallet mid-payment; refresh the bill.
  settlement_wallet_changed: "payment.errors.billNotPayable",
  // A repeat cashier request while staff already hold one for the balance.
  payment_request_pending: "bill.cashierRequestSentNote",
  idempotency_conflict: "payment.errors.idempotencyConflict",
  // Legacy generic conflict still used by some handlers
  CONFLICT: "payment.errors.billNotPayable",
};

export function presentGuestPaymentError(
  info: GuestPaymentErrorInfo,
): GuestPaymentErrorPresentation {
  const base = { preserveCart: true as const };
  if (info.isNetwork) {
    return { ...base, messageKey: "payment.errors.generic" };
  }
  if (info.code && CODE_TO_KEY[info.code]) {
    return { ...base, messageKey: CODE_TO_KEY[info.code] };
  }
  return { ...base, messageKey: "payment.errors.generic" };
}

// Hold-creation failures carry ONE backend code (split_share_conflict) for
// several distinct races; the sentinel error strings from
// backend/internal/database/bill_split.go are the stable discriminator.
export function presentSplitHoldError(
  info: GuestPaymentErrorInfo,
): GuestPaymentErrorPresentation {
  const base = { preserveCart: true as const };
  if (info.code === "split_share_conflict") {
    const message = (info.message ?? "").toLowerCase();
    if (message.includes("hold has expired")) {
      return { ...base, messageKey: "payment.errors.splitHoldExpired" };
    }
    if (message.includes("exceeds available balance")) {
      return { ...base, messageKey: "payment.errors.splitHoldOverBalance" };
    }
    if (message.includes("fraction is unavailable")) {
      return { ...base, messageKey: "payment.errors.splitHoldItemTaken" };
    }
    return { ...base, messageKey: "payment.errors.splitShareConflict" };
  }
  if (info.code === "split_not_open") {
    return { ...base, messageKey: "payment.errors.splitNotOpen" };
  }
  return { ...base, messageKey: "bill.splitHoldFailed" };
}
