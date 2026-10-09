import { useQuery } from "@tanstack/react-query";
import { businessApi, MenuCategory } from "@/api/business";
import type { Orderability } from "@/api/orders";
import { parseMenuCategories } from "@/utils/businessDataParsers";

/**
 * Shared React Query key for a business's menu categories (§3.7 fix 7).
 * MenuBuilder, Offers and Bundles all read/prime this one key so switching
 * between the Menu / Offers / Bundles sub-tabs no longer re-fetches the full
 * menu on every mount — the first fetch is cached and reused.
 */
const menuQueryKey = (businessId: number) =>
  ["business-menu-categories", businessId] as const;

/**
 * L3-16: project item_orderability onto each item so shared pickers
 * (Bundles, Offers) see inventory-driven 86 states, not just the manual flag.
 */
export function projectMenuOrderability(
  categories: MenuCategory[],
  orderability: Record<string, Orderability> = {},
): MenuCategory[] {
  if (!orderability || Object.keys(orderability).length === 0) {
    return categories;
  }
  return categories.map((category) => ({
    ...category,
    items: (category.items ?? []).map((item) => {
      const decision = item.id ? orderability[item.id] : undefined;
      if (!decision) return item;
      return {
        ...item,
        // Match MenuBuilder orderableMenu projection so shared pickers see
        // the same 86 surface (manual + inventory).
        is_available: decision.orderable,
        orderability_state: decision.state,
      };
    }),
  }));
}

/**
 * useSharedMenu returns the business's menu categories from a shared cache.
 * Offers/Bundles use this instead of an independent per-mount getMenu call.
 * staleTime keeps the categories cached across sub-tab switches within a
 * session; a mutation elsewhere can invalidate the key to force a refresh.
 */
export function useSharedMenu(businessId: number) {
  return useQuery<MenuCategory[]>({
    queryKey: menuQueryKey(businessId),
    queryFn: async () => {
      const menuData = await businessApi.getMenu(businessId);
      const categories = parseMenuCategories(menuData);
      return projectMenuOrderability(
        categories,
        menuData.item_orderability ?? {},
      );
    },
    enabled: Number.isFinite(businessId) && businessId > 0,
    // Categories change rarely relative to sub-tab bounces; 5 min keeps the
    // Offers/Bundles pickers instant without going stale for a working session.
    staleTime: 5 * 60 * 1000,
  });
}
