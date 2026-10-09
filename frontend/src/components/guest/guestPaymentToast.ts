import {
  guestPaymentErrorInfo,
  presentGuestPaymentError,
  type GuestPaymentErrorInfo,
} from "@/lib/guestPaymentErrors";

/**
 * Resolve a guest payment/auth/split failure to a localized toast string.
 *
 * Accepts:
 * - thrown API errors (sanitized axios shape with `response.data.code`)
 * - failure results `{ code?, message?, success?: false, isNetwork? }`
 *
 * Never returns raw backend English — unmapped codes fall through to
 * `payment.errors.generic` via `presentGuestPaymentError`.
 */
function infoFromUnknown(err: unknown): GuestPaymentErrorInfo {
  if (err && typeof err === "object" && !("response" in (err as object))) {
    const o = err as {
      code?: string;
      message?: string;
      isNetwork?: boolean;
      success?: boolean;
    };
    const hasCode = typeof o.code === "string";
    const hasMessage = typeof o.message === "string";
    const hasNetwork = typeof o.isNetwork === "boolean";
    if (hasCode || hasNetwork || (o.success === false && hasMessage)) {
      return {
        code: hasCode ? o.code : undefined,
        message: hasMessage ? o.message : undefined,
        isNetwork: Boolean(o.isNetwork),
      };
    }
  }
  return guestPaymentErrorInfo(err);
}

export function guestPaymentToastMessage(
  err: unknown,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  const presentation = presentGuestPaymentError(infoFromUnknown(err));
  return t(presentation.messageKey, presentation.params);
}
