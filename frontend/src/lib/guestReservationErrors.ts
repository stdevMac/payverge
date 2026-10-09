import {
  getApiErrorCode,
  getApiErrorMessage,
  getApiErrorStatus,
  isApiNetworkError,
} from "@/utils/apiError";

/**
 * Present guest-facing reservation cancel/success/load feedback without
 * leaking raw backend English prose (i18n Batch B Task 4 / #383).
 */

export type GuestReservationMessageKey =
  | "reservationConfirmation.action.cancelledBody"
  | "reservationConfirmation.action.alreadyCancelledBody"
  | "reservationConfirmation.action.cancelErrorBody"
  | "reservationConfirmation.action.notFoundBody"
  | "reservationConfirmation.action.windowClosedBody"
  | "reservationConfirmation.action.windowNeverOpenBody"
  | "reservationConfirmation.action.notAllowedBody"
  | "reservationConfirmation.action.noLongerCancellableBody";

const CODE_TO_KEY: Record<string, GuestReservationMessageKey> = {
  reservation_cancelled: "reservationConfirmation.action.cancelledBody",
  reservation_already_cancelled:
    "reservationConfirmation.action.alreadyCancelledBody",
  reservation_not_found: "reservationConfirmation.action.notFoundBody",
  cancellation_window_closed:
    "reservationConfirmation.action.windowClosedBody",
  reservation_cancel_never_open:
    "reservationConfirmation.action.windowNeverOpenBody",
  reservation_cancel_window:
    "reservationConfirmation.action.windowClosedBody",
  cancellations_not_allowed: "reservationConfirmation.action.notAllowedBody",
  reservation_not_cancellable:
    "reservationConfirmation.action.noLongerCancellableBody",
};

function messageToKey(message: string): GuestReservationMessageKey | null {
  const m = message.toLowerCase();
  if (!m) return null;
  if (m.includes("already cancelled")) {
    return "reservationConfirmation.action.alreadyCancelledBody";
  }
  if (m.includes("not found")) {
    return "reservationConfirmation.action.notFoundBody";
  }
  if (m.includes("cannot be cancelled online") || m.includes("never had a cancellation")) {
    return "reservationConfirmation.action.windowNeverOpenBody";
  }
  if (m.includes("window has closed") || m.includes("cancellation window")) {
    return "reservationConfirmation.action.windowClosedBody";
  }
  if (m.includes("not allowed")) {
    return "reservationConfirmation.action.notAllowedBody";
  }
  if (m.includes("no longer be cancelled") || m.includes("can no longer")) {
    return "reservationConfirmation.action.noLongerCancellableBody";
  }
  if (m.includes("cancelled successfully")) {
    return "reservationConfirmation.action.cancelledBody";
  }
  return null;
}

export function presentGuestReservationCancelSuccess(response: {
  code?: string;
  message?: string;
}): GuestReservationMessageKey {
  if (response.code && CODE_TO_KEY[response.code]) {
    return CODE_TO_KEY[response.code];
  }
  const fromMessage = messageToKey(response.message || "");
  if (fromMessage) return fromMessage;
  return "reservationConfirmation.action.cancelledBody";
}

export function presentGuestReservationCancelError(
  err: unknown,
): GuestReservationMessageKey {
  if (isApiNetworkError(err)) {
    return "reservationConfirmation.action.cancelErrorBody";
  }
  const code = getApiErrorCode(err);
  if (code && CODE_TO_KEY[code]) {
    return CODE_TO_KEY[code];
  }
  const fromMessage = messageToKey(getApiErrorMessage(err) || "");
  if (fromMessage) return fromMessage;
  return "reservationConfirmation.action.cancelErrorBody";
}

export type GuestReservationLoadMessageKey =
  | "reservationConfirmation.notFoundFallback"
  | "errors.networkErrorDescription"
  | "errors.serverErrorDescription";

function isNotFoundLoadError(err: unknown): boolean {
  if (getApiErrorStatus(err) === 404) {
    return true;
  }
  const code = getApiErrorCode(err);
  if (code === "reservation_not_found" || code === "NOT_FOUND") {
    return true;
  }
  const message = (
    getApiErrorMessage(err) || (err instanceof Error ? err.message : "")
  ).toLowerCase();
  return message.includes("not found");
}

/** Map a public reservation lookup failure to a guest catalog key. */
export function presentGuestReservationLoadError(
  err: unknown,
): GuestReservationLoadMessageKey {
  if (isNotFoundLoadError(err)) {
    return "reservationConfirmation.notFoundFallback";
  }
  if (isApiNetworkError(err)) {
    return "errors.networkErrorDescription";
  }
  return "errors.serverErrorDescription";
}
