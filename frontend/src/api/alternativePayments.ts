import { axiosInstance } from "@/api/tools/instance";
import { logError } from "@/utils/errorLogger";
import { sanitizeError } from "@/utils/errorMessages";
import {
  AlternativePaymentRequest,
  AlternativePaymentResponse,
  BillPaymentBreakdown,
  MoneyString,
  PendingAlternativePayment,
  PaymentMethod,
} from "@/types/alternativePayments";

export { asMoneyString } from "@/types/alternativePayments";

interface AlternativePaymentsAPI {
  // Business owner functions
  markAlternativePayment: (
    request: AlternativePaymentRequest,
  ) => Promise<AlternativePaymentResponse>;
  getPendingAlternativePayments: (
    billId: string,
  ) => Promise<PendingAlternativePayment[]>;
  rejectPendingAlternativePayment: (
    billId: string,
    requestId: string,
    reason?: string,
  ) => Promise<AlternativePaymentResolutionResponse>;
  getBillPaymentBreakdownInside: (
    billId: string,
  ) => Promise<BillPaymentBreakdown>;
}

export interface AlternativePaymentResolutionResponse {
  success: boolean;
  status: "rejected";
}

interface BackendAlternativePayment {
  id: number | string;
  bill_id: number | string;
  participant_name?: string;
  participant_address: string;
  amount: number | string;
  payment_method: string;
  status:
    | "pending"
    | "confirmed"
    | "failed"
    | "refunded"
    | "expired"
    | "cancelled"
    | "rejected";
  created_at?: string;
  confirmed_at?: string;
  expires_at?: string;
}

interface BackendPaymentBreakdown {
  total_amount?: number | string;
  crypto_paid?: number | string;
  alternative_paid?: number | string;
  remaining?: number | string;
  is_complete?: boolean;
  totalAmount?: number | string;
  cryptoPaid?: number | string;
  alternativePaid?: number | string;
  isComplete?: boolean;
}

import { dollarsToAmountMicro } from "@/types/alternativePayments";

const guestAlternativePaymentAttempts = new Map<string, string>();

function randomAlternativePaymentKey(prefix: string): string {
  const suffix =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}-${suffix}`;
}

function randomGuestAlternativePaymentKey(): string {
  return randomAlternativePaymentKey("guest-alt");
}

// L2-1: operator marks need the same attempt-scoped key reuse as guest
// requests. A retried submit replays the SAME key (backend dedupes it); a
// received success retires the key so a second, genuinely separate tender of
// the same amount mints a fresh key and is credited.
const operatorAlternativePaymentAttempts = new Map<string, string>();

function operatorAlternativePaymentFingerprint(
  request: AlternativePaymentRequest,
): string {
  return JSON.stringify([
    request.billId,
    request.participantAddress ?? "",
    request.participantName ?? "",
    request.amount,
    request.tipAmount ?? "",
    request.paymentMethod,
    request.requestId ?? "",
    request.businessConfirmation,
  ]);
}

function guestAlternativePaymentFingerprint(
  billToken: string,
  amount: MoneyString,
  paymentMethod: PaymentMethod,
  participantName?: string,
  splitShareId?: number,
  tipAmount?: string,
): string {
  return JSON.stringify([
    billToken,
    amount,
    paymentMethod,
    participantName ?? "",
    splitShareId ?? 0,
    tipAmount ?? "",
  ]);
}

function toAlternativePaymentAmountPayload(amount: MoneyString): string {
  return amount;
}

function toAlternativePaymentTipPayload(
  amount: string | undefined,
): string | undefined {
  if (amount === undefined) {
    return undefined;
  }
  const trimmed = amount.trim();
  if (!/^\d+(?:\.\d{1,2})?$/.test(trimmed)) {
    throw new Error("Tip amount must use cents precision");
  }
  const [wholePart, centsPart = ""] = trimmed.split(".");
  const whole = wholePart.replace(/^0+(?=\d)/, "") || "0";
  const cents = centsPart.padEnd(2, "0");
  return `${whole}.${cents}`;
}

function toTimestamp(value?: string): number {
  const timestamp = value ? Date.parse(value) : NaN;
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function fromBackendPaymentMethod(method: string): PaymentMethod {
  switch (method) {
    case "cash":
      return PaymentMethod.CASH;
    case "card":
      return PaymentMethod.CARD;
    case "venmo":
      return PaymentMethod.VENMO;
    case "other":
      return PaymentMethod.OTHER;
    default:
      return PaymentMethod.OTHER;
  }
}

function mapPendingPayment(
  payment: BackendAlternativePayment,
): PendingAlternativePayment {
  return {
    id: String(payment.id),
    billId: String(payment.bill_id),
    participantName: payment.participant_name || undefined,
    participantAddress: payment.participant_address,
    amount: dollarsToAmountMicro(payment.amount),
    paymentMethod: fromBackendPaymentMethod(payment.payment_method),
    timestamp: toTimestamp(payment.created_at),
    ...(payment.expires_at ? { expiresAt: payment.expires_at } : {}),
    status: payment.status,
  };
}

function mapBreakdown(
  breakdown?: BackendPaymentBreakdown,
): BillPaymentBreakdown {
  return {
    totalAmount: dollarsToAmountMicro(
      breakdown?.total_amount ?? breakdown?.totalAmount,
    ),
    cryptoPaid: dollarsToAmountMicro(
      breakdown?.crypto_paid ?? breakdown?.cryptoPaid,
    ),
    alternativePaid: dollarsToAmountMicro(
      breakdown?.alternative_paid ?? breakdown?.alternativePaid,
    ),
    remaining: dollarsToAmountMicro(breakdown?.remaining),
    isComplete: Boolean(breakdown?.is_complete ?? breakdown?.isComplete),
  };
}

/**
 * Mark an alternative payment (owner or staff with bills:payment)
 */
export async function markAlternativePayment(
  request: AlternativePaymentRequest,
): Promise<AlternativePaymentResponse> {
  const headers: Record<string, string> = {};
  let attemptFingerprint: string | null = null;
  if (request.idempotencyKey) {
    headers["Idempotency-Key"] = request.idempotencyKey;
  } else {
    attemptFingerprint = operatorAlternativePaymentFingerprint(request);
    const attemptKey =
      operatorAlternativePaymentAttempts.get(attemptFingerprint) ??
      randomAlternativePaymentKey("mark-alt");
    operatorAlternativePaymentAttempts.set(attemptFingerprint, attemptKey);
    headers["Idempotency-Key"] = attemptKey;
  }
  try {
    const response = await axiosInstance.post(
      `/inside/bills/${request.billId}/alternative-payment`,
      {
        ...(request.participantAddress
          ? { participant_address: request.participantAddress }
          : {}),
        ...(request.participantName
          ? { participant_name: request.participantName }
          : {}),
        amount: toAlternativePaymentAmountPayload(request.amount),
        ...(request.tipAmount
          ? { tip_amount: toAlternativePaymentTipPayload(request.tipAmount) }
          : {}),
        payment_method: PaymentMethod[request.paymentMethod].toLowerCase(),
        business_confirmation: request.businessConfirmation,
        ...(request.requestId
          ? { request_id: Number.parseInt(request.requestId, 10) }
          : {}),
      },
      { headers },
    );

    // A received success closes this attempt; a lost/failed response keeps
    // the key so the next submit replays safely.
    if (attemptFingerprint) {
      operatorAlternativePaymentAttempts.delete(attemptFingerprint);
    }

    return {
      success: true,
      transactionHash: response.data.transaction_hash,
      message:
        response.data.message || "Alternative payment marked successfully",
      paymentBreakdown: mapBreakdown(response.data.payment_breakdown),
    };
  } catch (error) {
    void logError(
      error instanceof Error ? error : String(error),
      "alternativePayments",
      "markAlternativePayment",
    );
    console.error("Error marking alternative payment:", error);
    return {
      success: false,
      message:
        sanitizeError(error).message || "Failed to mark alternative payment",
      paymentBreakdown: {
        totalAmount: "0",
        cryptoPaid: "0",
        alternativePaid: "0",
        remaining: "0",
        isComplete: false,
      },
    };
  }
}

/**
 * Get pending alternative payments for a bill
 */
export async function getPendingAlternativePayments(
  billId: string,
): Promise<PendingAlternativePayment[]> {
  try {
    const response = await axiosInstance.get(
      `/inside/bills/${billId}/pending-alternative-payments`,
    );
    return (response.data.pending_payments || []).map(mapPendingPayment);
  } catch (error) {
    void logError(
      error instanceof Error ? error : String(error),
      "alternativePayments",
      "getPendingAlternativePayments",
    );
    throw error;
  }
}

async function resolvePendingAlternativePayment(
  billId: string,
  requestId: string,
  action: "reject",
  reason = "",
): Promise<AlternativePaymentResolutionResponse> {
  try {
    const response = await axiosInstance.post(
      `/inside/bills/${billId}/pending-alternative-payments/${requestId}/${action}`,
      { reason },
    );
    const expectedStatus = "rejected";
    if (
      response.data.success !== true ||
      response.data.status !== expectedStatus
    ) {
      throw new Error("Payment request resolution returned an invalid result");
    }
    return {
      success: true,
      status: expectedStatus,
    };
  } catch (error) {
    void logError(
      error instanceof Error ? error : String(error),
      "alternativePayments",
      `${action}PendingAlternativePayment`,
    );
    throw error;
  }
}

export function rejectPendingAlternativePayment(
  billId: string,
  requestId: string,
  reason?: string,
): Promise<AlternativePaymentResolutionResponse> {
  return resolvePendingAlternativePayment(billId, requestId, "reject", reason);
}

/**
 * Get bill payment breakdown (operator)
 */
export async function getBillPaymentBreakdownInside(
  billId: string,
): Promise<BillPaymentBreakdown> {
  try {
    const response = await axiosInstance.get(
      `/inside/bills/${billId}/payment-breakdown`,
    );
    return mapBreakdown(response.data.breakdown);
  } catch (error) {
    void logError(
      error instanceof Error ? error : String(error),
      "alternativePayments",
      "getBillPaymentBreakdownInside",
    );
    throw error;
  }
}

/**
 * Request alternative payment (guest function).
 * billToken: unguessable public_token (legacy bill_number fallback).
 * This creates a pending payment request that business owner can confirm
 */
export async function requestAlternativePayment(
  billToken: string,
  amount: MoneyString,
  paymentMethod: PaymentMethod,
  participantName?: string,
  splitShareId?: number,
  tipAmount?: string,
): Promise<{ success: boolean; message: string; requestId?: string }> {
  const fingerprint = guestAlternativePaymentFingerprint(
    billToken,
    amount,
    paymentMethod,
    participantName,
    splitShareId,
    tipAmount,
  );
  const idempotencyKey =
    guestAlternativePaymentAttempts.get(fingerprint) ??
    randomGuestAlternativePaymentKey();
  guestAlternativePaymentAttempts.set(fingerprint, idempotencyKey);
  try {
    const normalizedTipAmount = toAlternativePaymentTipPayload(tipAmount);
    const response = await axiosInstance.post(
      `/guest/bill/${billToken}/request-alternative-payment`,
      {
        amount: toAlternativePaymentAmountPayload(amount),
        ...(normalizedTipAmount !== undefined
          ? { tip_amount: normalizedTipAmount }
          : {}),
        payment_method: PaymentMethod[paymentMethod].toLowerCase(),
        participant_name: participantName,
        ...(splitShareId ? { split_share_id: splitShareId } : {}),
      },
      { headers: { "Idempotency-Key": idempotencyKey } },
    );

    // A received success response closes this attempt. A lost/failed response
    // intentionally keeps the key so the next submit replays safely.
    guestAlternativePaymentAttempts.delete(fingerprint);

    return {
      success: true,
      message: "Alternative payment request sent to business owner",
      requestId:
        response.data.request_id != null
          ? String(response.data.request_id)
          : undefined,
    };
  } catch (error) {
    void logError(
      error instanceof Error ? error : String(error),
      "alternativePayments",
      "requestAlternativePayment",
    );
    // Rethrow so guest UI can map response.data.code via presentGuestPaymentError
    // (returning {success:false, message:english} strips the structured code).
    throw error;
  }
}

// Export all functions as default API object
const alternativePaymentsAPI: AlternativePaymentsAPI = {
  markAlternativePayment,
  getPendingAlternativePayments,
  rejectPendingAlternativePayment,
  getBillPaymentBreakdownInside,
};

export default alternativePaymentsAPI;
