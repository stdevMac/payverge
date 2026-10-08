import type { MenuCategory } from "../../../../api/business";
import type { Orderability } from "@/api/orders";

export type CachedOperatorMenu = {
  menu: MenuCategory[];
  version: number;
  orderability: Record<string, Orderability>;
};

const lastGoodByBusinessId = new Map<number, CachedOperatorMenu>();

export function rememberLastGoodOperatorMenu(
  businessId: number,
  cached: CachedOperatorMenu,
): void {
  lastGoodByBusinessId.set(businessId, cached);
}

export function readLastGoodOperatorMenu(
  businessId: number,
): CachedOperatorMenu | undefined {
  return lastGoodByBusinessId.get(businessId);
}

export function clearLastGoodOperatorMenu(businessId?: number): void {
  if (businessId == null) {
    lastGoodByBusinessId.clear();
    return;
  }
  lastGoodByBusinessId.delete(businessId);
}

/**
 * #773 — a failed catalog fetch must never look like an empty menu.
 * Keep last-good categories when we have them; otherwise the UI shows retry.
 */
export type EmptyMenuSurface = "catalog" | "retry" | "empty";

export function emptyMenuSurface(opts: {
  categoryCount: number;
  loadFailed: boolean;
}): EmptyMenuSurface {
  if (opts.categoryCount > 0) return "catalog";
  if (opts.loadFailed) return "retry";
  return "empty";
}
