import type { BillItem } from "@/api/bills";

export type BillItemTranslationMap = Record<
  string,
  Record<string, string> | undefined
>;

/**
 * Resolve a guest-facing display name for a bill line item.
 *
 * Translation maps from `/guest/table/:code/menu-translations` are keyed by
 * menu entity id (and dual-keyed by menu item string id). Bill line items
 * expose a distinct UUID as `id` and the menu catalog id as `menu_item_id`.
 * Prefer `menu_item_id` so Spanish (etc.) names match the translated table
 * menu; fall back to bill-item `id` for legacy payloads that reused one id.
 *
 * Non-menu rows (discounts) and missing translations keep the stored name.
 */
export function resolveBillItemTranslatedName(
  item: Pick<BillItem, "id" | "menu_item_id" | "name" | "item_type" | "itemType">,
  translations: BillItemTranslationMap,
): string {
  const type = (item.item_type || item.itemType || "menu_item").toLowerCase();
  if (type === "discount") {
    return item.name;
  }

  const rawIds = [item.menu_item_id, item.id]
    .map((value) => (typeof value === "string" ? value.trim() : ""))
    .filter((value) => value.length > 0);
  const candidates = [...rawIds];
  for (const key of rawIds) {
    if (key.startsWith("bundle:")) {
      candidates.push(key.slice("bundle:".length));
    }
  }

  for (const key of candidates) {
    const translated = translations[key]?.name;
    if (typeof translated === "string" && translated.trim().length > 0) {
      return translated;
    }
  }

  return item.name;
}
