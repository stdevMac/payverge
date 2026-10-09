import type { MenuCategory, MenuItem } from "@/api/business";
import type { InventoryItem, InventoryMenuItemStatus, InventoryRecipe } from "@/api/inventory";

export type InventoryMenuRef = {
  categoryId: string;
  categoryIndex: number;
  itemIndex: number;
  itemId: string;
  item: MenuItem;
};

export function buildMenuItemIndex(
  categories: MenuCategory[],
): Record<string, InventoryMenuRef> {
  const index: Record<string, InventoryMenuRef> = {};
  categories.forEach((cat, categoryIndex) => {
    (cat.items || []).forEach((item, itemIndex) => {
      if (!item.id) return;
      index[item.id] = {
        categoryId: cat.id ?? "",
        categoryIndex,
        itemIndex,
        itemId: item.id,
        item,
      };
    });
  });
  return index;
}

function inventoryNamesMatch(a?: string, b?: string): boolean {
  const left = (a ?? "").trim().toLowerCase();
  const right = (b ?? "").trim().toLowerCase();
  return left.length > 0 && left === right;
}

type RecipeMenuRef = Pick<InventoryRecipe, "menu_item_id" | "menu_item_name">;
type LiveMenuRef = { id?: string; name?: string };

/**
 * Map a recipe onto the live dish it belongs to. The stored menu_item_id is
 * the strong key and wins whenever it matches a live dish — including when
 * menu_item_name names a DIFFERENT live dish (#727 B2: recipes are one row
 * per (dish, ingredient), so a multi-ingredient dish legitimately owns
 * several rows sharing its id, and only names drift). This mirrors the
 * backend resolveInventoryRecipeMenuItem and the guest orderability path,
 * which key on the stored id. The name only decides when the id no longer
 * matches any live dish (rewritten menus, dead demo ids).
 */
export function resolveRecipeMenuItemId(
  recipe: RecipeMenuRef,
  liveItems: LiveMenuRef[],
): string | null {
  const id = (recipe.menu_item_id ?? "").trim();
  const name = (recipe.menu_item_name ?? "").trim();
  const byId = id ? liveItems.find((item) => item.id === id) : undefined;
  if (byId?.id) return byId.id;
  const byName = name
    ? liveItems.find((item) => inventoryNamesMatch(item.name, name))
    : undefined;
  if (byName?.id) return byName.id;
  return id || null;
}

function bindStatusToLiveItem(
  item: LiveMenuRef,
  status?: InventoryMenuItemStatus,
): InventoryMenuItemStatus | undefined {
  if (!status) return undefined;
  const liveId = (item.id ?? "").trim();
  if (liveId && status.menu_item_id !== liveId) {
    return { ...status, menu_item_id: liveId };
  }
  return status;
}

export function inventoryStatusForMenuItem(
  item: LiveMenuRef,
  statuses: Record<string, InventoryMenuItemStatus>,
  liveItems: LiveMenuRef[] = [],
): InventoryMenuItemStatus | undefined {
  const liveId = (item.id ?? "").trim();
  if (liveId && statuses[liveId]) {
    const byId = statuses[liveId];
    const statusName = (byId.menu_item_name ?? "").trim();
    // Translated Menu Builder names ("Bowl de la cosecha") do not match
    // English summary names ("Harvest Bowl"). That is not drift — trust
    // the id unless the status name is a *different* live dish.
    const namesOtherLiveDish =
      statusName.length > 0 &&
      liveItems.some(
        (live) =>
          inventoryNamesMatch(live.name, statusName) &&
          (live.id ?? "").trim() !== liveId,
      );
    const nameConflictsWithThisItem =
      statusName.length > 0 &&
      Boolean(item.name) &&
      !inventoryNamesMatch(statusName, item.name);
    if (namesOtherLiveDish || (liveItems.length === 0 && nameConflictsWithThisItem)) {
      return bindStatusToLiveItem(
        item,
        Object.values(statuses).find((status) =>
          inventoryNamesMatch(status.menu_item_name, item.name),
        ),
      );
    }
    return bindStatusToLiveItem(item, byId);
  }
  if (item.name) {
    return bindStatusToLiveItem(
      item,
      Object.values(statuses).find((status) =>
        inventoryNamesMatch(status.menu_item_name, item.name),
      ),
    );
  }
  return undefined;
}

export function liveMenuRefsFromIndex(
  index: Record<string, InventoryMenuRef>,
): LiveMenuRef[] {
  return Object.values(index).map((ref) => ({
    id: ref.itemId,
    name: ref.item.name,
  }));
}

function dishesForInventoryItem(
  item: InventoryItem,
  recipes: InventoryRecipe[],
  menuStatuses: InventoryMenuItemStatus[],
  liveItems: LiveMenuRef[],
  sellableOnly: boolean,
): InventoryMenuItemStatus[] {
  const live =
    liveItems.length > 0
      ? liveItems
      : menuStatuses.map((status) => ({
          id: status.menu_item_id,
          name: status.menu_item_name,
        }));
  const seen = new Set<string>();
  const dishes: InventoryMenuItemStatus[] = [];
  for (const recipe of recipes) {
    if (recipe.inventory_item_id !== item.id) continue;
    const liveId = resolveRecipeMenuItemId(recipe, live);
    if (!liveId || seen.has(liveId)) continue;
    const liveName = live.find((entry) => entry.id === liveId)?.name;
    const status =
      menuStatuses.find((entry) => entry.menu_item_id === liveId) ??
      menuStatuses.find((entry) =>
        inventoryNamesMatch(entry.menu_item_name, liveName),
      );
    if (!status) continue;
    if (sellableOnly) {
      if (!status.has_recipe) continue;
      if (status.manual_available === false) continue;
      if (status.status === "manual_unavailable") continue;
    }
    seen.add(liveId);
    dishes.push(
      status.menu_item_id === liveId
        ? status
        : { ...status, menu_item_id: liveId },
    );
  }
  return dishes;
}

/** Dishes that consume this SKU and are still marked available on the menu. */
export function mappedSellableDishes(
  item: InventoryItem,
  recipes: InventoryRecipe[],
  menuStatuses: InventoryMenuItemStatus[],
  liveItems: LiveMenuRef[] = [],
): InventoryMenuItemStatus[] {
  return dishesForInventoryItem(item, recipes, menuStatuses, liveItems, true);
}

/** Dishes whose recipe consumes this SKU, including already-86'd rows. */
export function dishesUsingInventoryItem(
  item: InventoryItem,
  recipes: InventoryRecipe[],
  menuStatuses: InventoryMenuItemStatus[],
  liveItems: LiveMenuRef[] = [],
): InventoryMenuItemStatus[] {
  return dishesForInventoryItem(item, recipes, menuStatuses, liveItems, false);
}

export function canOfferEightySix(
  item: InventoryItem,
  dishes: InventoryMenuItemStatus[],
): boolean {
  return item.current_quantity <= 0 && dishes.length > 0;
}
