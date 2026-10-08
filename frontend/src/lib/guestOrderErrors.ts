import {
  getApiErrorStatus,
  getApiErrorCode,
  getApiErrorData,
  getApiErrorMessage,
  isApiNetworkError,
} from "@/utils/apiError";

/**
 * Maps guest order-create failures to translated message keys + behavior
 * flags (Stage 2 backend contract: guest order-create error bodies carry a
 * stable `code` field alongside the `error` string).
 *
 * Truthfulness rule (G-1): the "Order placed, but…" wording is reserved for
 * network/timeout failures of the ORDER request itself, where the request may
 * genuinely have reached the server. Any HTTP response means the order was
 * NOT placed and the guest must be told so.
 */

// FE mirrors of the backend caps (Stage 2, shared validation constants).
export const GUEST_ORDER_QUANTITY_CAP = 50;
export const GUEST_ORDER_LINE_CAP = 60;
export const GUEST_ORDER_TEXT_CAP = 500;
/** Soft max for fat-finger bundle adds on phones (Guest QA #80). */
export const GUEST_BUNDLE_QUANTITY_SOFT_MAX = 4;

export interface GuestOrderErrorInfo {
  status?: number;
  code?: string;
  message?: string;
  /** The request never produced an HTTP response (offline, timeout, DNS…). */
  isNetwork: boolean;
  /** Authoritative item IDs/reasons returned by atomic checkout validation. */
  blockedItems?: GuestBlockedItem[];
}

type GuestBlockedItemReason =
  | "manual_disabled"
  | "inventory_out"
  | "inventory_warning"
  | "business_closed"
  | "ordering_disabled";

interface GuestBlockedItem {
  menuItemId: string;
  reason: GuestBlockedItemReason;
}

const BLOCKED_ITEM_REASONS = new Set<GuestBlockedItemReason>([
  "manual_disabled",
  "inventory_out",
  "inventory_warning",
  "business_closed",
  "ordering_disabled",
]);

function getBlockedItems(err: unknown): GuestBlockedItem[] | undefined {
  const data = getApiErrorData(err);
  if (typeof data !== "object" || data === null) return undefined;
  const details = (data as { details?: unknown }).details;
  if (typeof details !== "object" || details === null) return undefined;
  const items = (details as { items?: unknown }).items;
  if (!Array.isArray(items)) return undefined;

  const blocked = items.flatMap((item): GuestBlockedItem[] => {
    if (typeof item !== "object" || item === null) return [];
    const { menu_item_id: menuItemId, reason } = item as {
      menu_item_id?: unknown;
      reason?: unknown;
    };
    if (
      typeof menuItemId !== "string" ||
      menuItemId.trim() === "" ||
      typeof reason !== "string" ||
      !BLOCKED_ITEM_REASONS.has(reason as GuestBlockedItemReason)
    ) {
      return [];
    }
    return [
      {
        menuItemId: menuItemId.trim(),
        reason: reason as GuestBlockedItemReason,
      },
    ];
  });
  return blocked.length > 0 ? blocked : undefined;
}

export function guestOrderErrorInfo(err: unknown): GuestOrderErrorInfo {
  const blockedItems = getBlockedItems(err);
  return {
    status: getApiErrorStatus(err),
    code: getApiErrorCode(err),
    message: getApiErrorMessage(err),
    isNetwork: isApiNetworkError(err),
    ...(blockedItems ? { blockedItems } : {}),
  };
}

export interface GuestOrderErrorPresentation {
  /** Guest i18n key for the toast. */
  messageKey: string;
  params?: Record<string, string | number>;
  /** The cart is never cleared on failure; kept explicit so tests assert it. */
  preserveCart: true;
  /** Offer a tap-to-remove affordance for the offending cart line(s). */
  offerRemoveItem: boolean;
  /** Network/timeout on the order request only: the order may have landed. */
  maybePlaced: boolean;
}

export function presentGuestOrderError(
  info: GuestOrderErrorInfo,
  context: { path: "create-bill" | "add-items"; orderRequestSent: boolean },
): GuestOrderErrorPresentation {
  const base = {
    preserveCart: true as const,
    offerRemoveItem: false,
    maybePlaced: false,
  };

  if (info.isNetwork) {
    if (!context.orderRequestSent) {
      return { ...base, messageKey: "menu.orderErrorGeneric" };
    }
    return {
      ...base,
      maybePlaced: true,
      messageKey:
        context.path === "create-bill"
          ? "menu.orderApprovalWarning"
          : "menu.additionalApprovalWarning",
    };
  }

  switch (info.code) {
    // A server administrator suspended or closed the venue.
    case "business_unavailable":
    case "ordering_disabled":
      return { ...base, messageKey: "menu.orderingDisabled" };
    // Top-level Closed Mode code (FIND-035 / guest checkout after hours).
    case "business_closed":
      return { ...base, messageKey: "menu.businessClosed" };
    case "bill_not_open":
      return { ...base, messageKey: "menu.orderErrorBillClosed" };
    case "item_unavailable":
    case "item_not_found":
    case "bundle_not_found":
      return {
        ...base,
        offerRemoveItem: true,
        messageKey: "menu.orderErrorItemUnavailable",
      };
    case "item_not_orderable": {
      const reasons = new Set(info.blockedItems?.map((item) => item.reason));
      if (reasons.has("business_closed")) {
        return { ...base, messageKey: "menu.businessClosed" };
      }
      if (reasons.has("ordering_disabled")) {
        return { ...base, messageKey: "menu.orderingDisabled" };
      }
      return {
        ...base,
        offerRemoveItem: true,
        messageKey: "menu.orderErrorItemUnavailable",
      };
    }
    case "option_not_found":
      return { ...base, messageKey: "menu.orderErrorOptionNotFound" };
    case "quantity_exceeded":
      return {
        ...base,
        messageKey: "menu.orderErrorQuantityExceeded",
        params: { max: GUEST_ORDER_QUANTITY_CAP },
      };
    case "too_many_items":
      return {
        ...base,
        messageKey: "menu.orderErrorTooManyItems",
        params: { max: GUEST_ORDER_LINE_CAP },
      };
    case "text_too_long":
      return {
        ...base,
        messageKey: "menu.orderErrorTextTooLong",
        params: { max: GUEST_ORDER_TEXT_CAP },
      };
    default:
      return { ...base, messageKey: "menu.orderErrorGeneric" };
  }
}

/**
 * G-2: create-bill 409 when the table already holds an open/partial bill.
 * Deviation note: the backend body today is `{"error":"Table already has an
 * open bill"}` with NO code (backend/internal/server/guest_handlers.go,
 * both the pre-check and the unique-index race path emit the same string),
 * so detection is status + stable substring, with a forward-compat code
 * check should a code be added later.
 */
export function isOpenBillConflict(err: unknown): boolean {
  const info = guestOrderErrorInfo(err);
  return (
    info.status === 409 &&
    (info.code === "bill_already_exists" ||
      (info.message ?? "").toLowerCase().includes("already has an open bill"))
  );
}
