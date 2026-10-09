import { useQuery } from "@tanstack/react-query";
import { inventoryApi } from "@/api/inventory";
import {
  dishRecipeCoverageFromStatuses,
  type DishRecipeCoverage,
} from "./dishRecipeCoverage";

export function useDishRecipeCoverage(
  businessId: string | number,
): { data: DishRecipeCoverage | undefined; isLoading: boolean } {
  const numericId = Number(businessId);
  const query = useQuery({
    queryKey: ["inventory-summary", "dish-recipe-coverage", String(businessId)],
    queryFn: () => inventoryApi.getSummary(numericId),
    enabled: Number.isFinite(numericId) && numericId > 0,
    select: (summary) =>
      dishRecipeCoverageFromStatuses(summary.menu_item_statuses),
  });
  return { data: query.data, isLoading: query.isLoading };
}
