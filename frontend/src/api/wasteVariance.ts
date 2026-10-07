import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

export type WasteReason = "spoilage" | "count_shrink" | "manual";

interface WasteVarianceIngredient {
  inventory_item_id: number;
  name: string;
  unit: string;
  cost_per_unit: Dollars;
  theoretical_usage: number;
  actual_usage: number;
  variance: number;
  variance_cost: Dollars;
  tracked_loss_cost: Dollars;
  has_recipe: boolean;
  // is_active is optional until the backend variance dim exposes it (see
  // HANDOFF, R3-AC-9). When present and false, the ingredient was
  // deliberately included despite being deactivated/discontinued; the card
  // tags it so the row isn't mistaken for a live item.
  is_active?: boolean;
}

interface WasteLossByReason {
  reason: WasteReason;
  cost: Dollars;
}

export interface WasteVarianceReport {
  period: string;
  tracked_loss_cost: Dollars;
  total_variance_cost: Dollars;
  theoretical_usage_cost: Dollars;
  loss_by_reason: WasteLossByReason[];
  ingredients: WasteVarianceIngredient[];
  items_without_recipe: number;
  sparse: boolean;
}

export const wasteVarianceApi = {
  getWasteVariance: async (
    businessId: string,
    period: "day" | "week" | "month" = "week",
  ): Promise<WasteVarianceReport> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/waste-variance",
      { params: { period } },
    );
    return response.data.data;
  },
};
