import { useMemo } from "react";
import { InventoryItem } from "@/api/inventory";
import { itemStatus, selectItemValue } from "./inventorySelectors";

export type InventoryStatusFilter = "all" | "healthy" | "low" | "out";
export type InventorySort = "attention" | "name" | "value" | "quantity" | "category";

export interface InventoryFilterState {
  search: string;
  category: string; // "" = all
  status: InventoryStatusFilter;
  sort: InventorySort;
}

export interface InventoryFilterResult {
  visibleItems: InventoryItem[];
  counts: { all: number; healthy: number; low: number; out: number };
  categories: string[];
}

const ATTENTION_RANK: Record<string, number> = {
  out_of_stock: 0,
  low_stock: 1,
  ok: 2,
};

export function useInventoryFilter(
  items: InventoryItem[],
  state: InventoryFilterState,
): InventoryFilterResult {
  return useMemo(() => {
    const counts = { all: items.length, healthy: 0, low: 0, out: 0 };
    for (const it of items) {
      const s = itemStatus(it);
      if (s === "out_of_stock") counts.out += 1;
      else if (s === "low_stock") counts.low += 1;
      else counts.healthy += 1;
    }

    const categories = Array.from(
      new Set(items.map((i) => (i.category || "").trim()).filter(Boolean)),
    ).sort((a, b) => a.localeCompare(b));

    const query = state.search.trim().toLowerCase();
    let visible = items.filter((it) => {
      if (query) {
        const hay = `${it.name} ${it.sku || ""}`.toLowerCase();
        if (!hay.includes(query)) return false;
      }
      if (state.category && (it.category || "").trim() !== state.category) return false;

      const s = itemStatus(it);
      if (state.status === "healthy" && s !== "ok") return false;
      if (state.status === "low" && s !== "low_stock") return false;
      if (state.status === "out" && s !== "out_of_stock") return false;

      return true;
    });

    visible = [...visible].sort((a, b) => {
      switch (state.sort) {
        case "name":
          return a.name.localeCompare(b.name);
        case "value":
          return selectItemValue(b) - selectItemValue(a);
        case "quantity":
          return a.current_quantity - b.current_quantity;
        case "category":
          return (
            (a.category || "").localeCompare(b.category || "") || a.name.localeCompare(b.name)
          );
        case "attention":
        default: {
          const rank = ATTENTION_RANK[itemStatus(a)] - ATTENTION_RANK[itemStatus(b)];
          return rank !== 0 ? rank : a.name.localeCompare(b.name);
        }
      }
    });

    return { visibleItems: visible, counts, categories };
  }, [items, state.search, state.category, state.status, state.sort]);
}
