"use client";

import React, { useCallback, useEffect, useRef } from "react";

import type { Bundle, BundleItemRef, MenuCategory } from "@/api/business";
import type { Orderability } from "@/api/orders";
import { GUEST_ORDER_QUANTITY_CAP } from "@/lib/guestOrderErrors";
import { isMenuItemOrderable } from "@/components/guest/menuItemAvailability";
import {
  canonicalPendingCartTableCode,
  consumePendingCartIntent,
  getOrCreatePendingCartSessionBinding,
  type PendingCartIntent,
  type PendingCartStorage,
} from "@/components/guest/pendingCartIntent";
import type { CartItem } from "./_types";

export const menuRouteScopeKey = (tableCode: string): string =>
  canonicalPendingCartTableCode(tableCode) ?? tableCode;

export function GuestMenuTableScope({
  tableCode,
  children,
}: {
  tableCode: string;
  children: React.ReactNode;
}) {
  return (
    <React.Fragment key={menuRouteScopeKey(tableCode)}>
      {children}
    </React.Fragment>
  );
}

export type PendingCartTarget =
  | {
      itemType: "menu_item";
      menuItemId: string;
      itemName: string;
      price: number;
    }
  | {
      itemType: "bundle";
      bundleId: number;
      itemName: string;
      price: number;
    };

type PendingCartTargetResult =
  | { ok: true; target: PendingCartTarget }
  | { ok: false; error: "item_unavailable" };

const safePendingPresentation = (value: unknown): value is string =>
  typeof value === "string" &&
  value.trim().length > 0 &&
  Array.from(value).length <= 300 &&
  !/[\p{Cc}\p{Cf}]/u.test(value);

const bundleItemRefs = (bundle: Bundle): BundleItemRef[] | null => {
  let raw: unknown;
  if (Array.isArray(bundle.items)) {
    raw = bundle.items;
  } else if (typeof bundle.items === "string") {
    try {
      raw = JSON.parse(bundle.items);
    } catch {
      return null;
    }
  } else {
    return null;
  }
  if (!Array.isArray(raw)) return null;
  const refs = raw.map((entry): BundleItemRef | null => {
    if (typeof entry === "string") {
      return safePendingPresentation(entry)
        ? { menu_item_id: entry, quantity: 1 }
        : null;
    }
    if (typeof entry !== "object" || entry === null) {
      return null;
    }
    const value = entry as Record<string, unknown>;
    const menuItemID =
      typeof value.menu_item_id === "string"
        ? value.menu_item_id
        : typeof value.id === "string"
          ? value.id
          : "";
    const quantity = value.quantity === undefined ? 1 : value.quantity;
    if (
      !safePendingPresentation(menuItemID) ||
      typeof quantity !== "number" ||
      !Number.isSafeInteger(quantity) ||
      quantity <= 0
    ) {
      return null;
    }
    return { menu_item_id: menuItemID, quantity };
  });
  return refs.some((ref) => ref === null) ? null : (refs as BundleItemRef[]);
};

const blockedPendingOrderabilityStates = new Set([
  "manual_disabled",
  "inventory_out",
  "business_closed",
  "ordering_disabled",
]);

const pendingOrderabilityAllows = (
  decision: Orderability | undefined,
): boolean =>
  decision === undefined ||
  (decision.orderable !== false &&
    !blockedPendingOrderabilityStates.has(decision.state));

export function resolvePendingCartTarget(
  intent: PendingCartIntent,
  categories: MenuCategory[],
  bundles: Bundle[],
  orderability: Record<string, Orderability>,
): PendingCartTargetResult {
  if (intent.menuItemId !== undefined) {
    const matches = categories.flatMap((category) =>
      (category.items || []).filter((item) => item.id === intent.menuItemId),
    );
    if (matches.length !== 1) return { ok: false, error: "item_unavailable" };
    const item = matches[0];
    if (
      !isMenuItemOrderable(item) ||
      !pendingOrderabilityAllows(orderability[intent.menuItemId]) ||
      !safePendingPresentation(item.name) ||
      typeof item.price !== "number" ||
      !Number.isFinite(item.price) ||
      item.price < 0
    ) {
      return { ok: false, error: "item_unavailable" };
    }
    return {
      ok: true,
      target: {
        itemType: "menu_item",
        menuItemId: intent.menuItemId,
        itemName: item.name.trim(),
        price: item.price,
      },
    };
  }

  const matches = bundles.filter((bundle) => bundle.id === intent.bundleId);
  if (matches.length !== 1) return { ok: false, error: "item_unavailable" };
  const bundle = matches[0];
  const refs = bundleItemRefs(bundle);
  const liveItems = categories.flatMap((category) => category.items || []);
  if (
    !Number.isSafeInteger(bundle.id) ||
    (bundle.id as number) <= 0 ||
    bundle.is_active !== true ||
    refs === null ||
    refs.length === 0 ||
    refs.some((ref) => {
      if (!ref.menu_item_id) return true;
      const itemMatches = liveItems.filter(
        (item) => item.id === ref.menu_item_id,
      );
      return (
        itemMatches.length !== 1 ||
        !isMenuItemOrderable(itemMatches[0]) ||
        !pendingOrderabilityAllows(orderability[ref.menu_item_id])
      );
    }) ||
    !safePendingPresentation(bundle.name) ||
    typeof bundle.price !== "number" ||
    !Number.isFinite(bundle.price) ||
    bundle.price < 0
  ) {
    return { ok: false, error: "item_unavailable" };
  }
  return {
    ok: true,
    target: {
      itemType: "bundle",
      bundleId: bundle.id as number,
      itemName: bundle.name.trim(),
      price: bundle.price,
    },
  };
}

const matchesPendingCartTarget = (
  item: CartItem,
  target: PendingCartTarget,
  notes?: string,
): boolean => {
  if (item.specialRequests !== notes) return false;
  if (item.name !== target.itemName) return false;
  if (item.price !== target.price) return false;
  if (target.itemType === "menu_item") {
    if (item.itemType !== "menu_item") return false;
    return (
      item.menuItemId === target.menuItemId &&
      (!item.addOns || item.addOns.length === 0)
    );
  }
  if (item.itemType !== "bundle") return false;
  return (
    item.bundleId === target.bundleId &&
    item.sourceOfferId === undefined &&
    item.menuItemId === undefined
  );
};

export function undoPendingCartDelta(
  cart: CartItem[],
  target: PendingCartTarget,
  intent: PendingCartIntent,
): CartItem[] {
  let remaining = intent.quantity;
  return cart.flatMap((item) => {
    if (
      remaining === 0 ||
      !matchesPendingCartTarget(item, target, intent.notes)
    ) {
      return [item];
    }
    const removed = Math.min(remaining, item.quantity);
    remaining -= removed;
    return item.quantity === removed
      ? []
      : [{ ...item, quantity: item.quantity - removed }];
  });
}

export function canApplyPendingCartDelta(
  cart: CartItem[],
  target: PendingCartTarget,
  intent: PendingCartIntent,
  quantityCap: number,
): boolean {
  const existingQuantity = cart
    .filter((item) => matchesPendingCartTarget(item, target, intent.notes))
    .reduce((sum, item) => sum + item.quantity, 0);
  return existingQuantity + intent.quantity <= quantityCap;
}

type PendingCartRouteConsumerProps = {
  tableCode: string;
  ready: boolean;
  categories: MenuCategory[];
  bundles: Bundle[];
  orderability: Record<string, Orderability>;
  addToCart: (target: PendingCartTarget, intent: PendingCartIntent) => boolean;
  cart: CartItem[];
  undo: (target: PendingCartTarget, intent: PendingCartIntent) => void;
  onApplied: (
    target: PendingCartTarget,
    intent: PendingCartIntent,
    undo: () => void,
  ) => void;
  onRejected: (
    reason:
      | "storage"
      | "scope"
      | "expired"
      | "unavailable"
      | "quantity"
      | "mutation",
  ) => void;
  quantityCap?: number;
  storage?: PendingCartStorage;
  now?: number;
};

export function PendingCartRouteConsumer({
  tableCode,
  ready,
  categories,
  bundles,
  orderability,
  addToCart,
  cart,
  undo,
  onApplied,
  onRejected,
  quantityCap = GUEST_ORDER_QUANTITY_CAP,
  storage,
  now,
}: PendingCartRouteConsumerProps) {
  const attemptedRef = useRef<string | null>(null);
  const rejectedRef = useRef(false);
  const acknowledgedRef = useRef(false);
  const scopeRef = useRef(tableCode);
  const pendingAckRef = useRef<{
    target: PendingCartTarget;
    intent: PendingCartIntent;
    beforeQuantity: number;
  } | null>(null);
  if (scopeRef.current !== tableCode) {
    scopeRef.current = tableCode;
    attemptedRef.current = null;
    rejectedRef.current = false;
    acknowledgedRef.current = false;
    pendingAckRef.current = null;
  }
  const rejectOnce = useCallback(
    (
      reason:
        | "storage"
        | "scope"
        | "expired"
        | "unavailable"
        | "quantity"
        | "mutation",
    ) => {
      if (rejectedRef.current) return;
      rejectedRef.current = true;
      onRejected(reason);
    },
    [onRejected],
  );

  useEffect(() => {
    if (!ready) return;
    const binding = getOrCreatePendingCartSessionBinding(tableCode, {
      storage,
    });
    if (!binding.ok) {
      rejectOnce(binding.error === "invalid_scope" ? "scope" : "storage");
      return;
    }
    const attemptKey = `${tableCode}\u0000${binding.sessionId}`;
    if (attemptedRef.current === attemptKey) return;
    attemptedRef.current = attemptKey;

    const consumed = consumePendingCartIntent(tableCode, binding.sessionId, {
      storage,
      ...(now === undefined ? {} : { now }),
    });
    if (!consumed.ok) {
      if (consumed.error !== "not_found" && consumed.error !== "replayed") {
        rejectOnce(
          consumed.error === "expired"
            ? "expired"
            : consumed.error === "table_mismatch" ||
                consumed.error === "session_mismatch" ||
                consumed.error === "invalid_scope"
              ? "scope"
              : "storage",
        );
      }
      return;
    }
    const resolved = resolvePendingCartTarget(
      consumed.intent,
      categories,
      bundles,
      orderability,
    );
    if (!resolved.ok) {
      rejectOnce("unavailable");
      return;
    }
    if (
      !canApplyPendingCartDelta(
        cart,
        resolved.target,
        consumed.intent,
        quantityCap,
      )
    ) {
      rejectOnce("quantity");
      return;
    }
    const beforeQuantity = cart
      .filter((item) =>
        matchesPendingCartTarget(item, resolved.target, consumed.intent.notes),
      )
      .reduce((sum, item) => sum + item.quantity, 0);
    let accepted = false;
    try {
      accepted = addToCart(resolved.target, consumed.intent);
    } catch {
      rejectOnce("mutation");
      return;
    }
    if (!accepted) {
      rejectOnce("mutation");
      return;
    }
    pendingAckRef.current = {
      target: resolved.target,
      intent: consumed.intent,
      beforeQuantity,
    };
  }, [
    addToCart,
    bundles,
    categories,
    cart,
    now,
    orderability,
    quantityCap,
    ready,
    rejectOnce,
    storage,
    tableCode,
    undo,
  ]);

  useEffect(() => {
    const pending = pendingAckRef.current;
    if (pending === null || acknowledgedRef.current) return;
    const currentQuantity = cart
      .filter((item) =>
        matchesPendingCartTarget(item, pending.target, pending.intent.notes),
      )
      .reduce((sum, item) => sum + item.quantity, 0);
    if (currentQuantity < pending.beforeQuantity + pending.intent.quantity) {
      return;
    }
    acknowledgedRef.current = true;
    onApplied(pending.target, pending.intent, () =>
      undo(pending.target, pending.intent),
    );
  }, [cart, onApplied, undo]);

  return null;
}

export function PendingCartAppliedNotice({
  message,
  undoLabel,
  onUndo,
}: {
  message: string;
  undoLabel: string;
  onUndo: () => void;
}) {
  return (
    <span className="flex items-center gap-3">
      <span>{message}</span>
      <button
        type="button"
        onClick={onUndo}
        className="font-semibold text-brand-dark hover:underline"
      >
        {undoLabel}
      </button>
    </span>
  );
}
