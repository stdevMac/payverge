import {
  getApiErrorCode,
  getApiErrorMessage,
  getApiErrorStatus,
} from "@/utils/apiError";

export type GuestCancelConflictKey =
  | "orders.alreadyAccepted"
  | "orders.alreadyCancelled"
  | "orders.cancelFailed";

/** Map a guest-cancel API failure to a guest catalog key (#528). */
export function guestCancelConflictKey(
  status?: number,
  code?: string,
  message?: string,
): GuestCancelConflictKey {
  if (status !== 409) {
    return "orders.cancelFailed";
  }
  const normalizedCode = (code || "").toLowerCase();
  const normalizedMessage = (message || "").toLowerCase();
  if (
    normalizedCode === "order_already_accepted" ||
    normalizedMessage.includes("accepted by the kitchen")
  ) {
    return "orders.alreadyAccepted";
  }
  return "orders.alreadyCancelled";
}

const AXIOS_TRANSPORT_CODE = /^(ERR_|ECONNABORTED$)/;

function backendCancelCode(error: unknown): string | undefined {
  const fromBody = getApiErrorCode(error);
  if (fromBody) return fromBody;
  const top =
    typeof error === "object" && error !== null
      ? (error as { code?: unknown }).code
      : undefined;
  if (typeof top !== "string" || top.length === 0) return undefined;
  if (AXIOS_TRANSPORT_CODE.test(top)) return undefined;
  return top;
}

function backendCancelMessage(error: unknown): string | undefined {
  const fromBody = getApiErrorMessage(error);
  if (fromBody) return fromBody;
  return error instanceof Error ? error.message : undefined;
}

/** Resolve a caught cancel error, including sanitized axios / top-level codes. */
export function guestCancelConflictFromError(
  error: unknown,
): GuestCancelConflictKey {
  return guestCancelConflictKey(
    getApiErrorStatus(error),
    backendCancelCode(error),
    backendCancelMessage(error),
  );
}
