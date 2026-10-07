import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

// Menu-engineering money fields arrive as DOLLARS (float) over JSON, consistent
// with the food-cost endpoint (see backend internal/services/menuengineering).
export type Quadrant = "star" | "plowhorse" | "puzzle" | "dog";

interface MenuEngineeringDish {
  menu_item_id: string;
  menu_item_name: string;
  food_cost_pct: number; // 0..1
  qty_sold: number;
  avg_price: Dollars;
  unit_cost: Dollars;
  margin_per_unit: Dollars;
  quadrant: Quadrant;
  action: "protect" | "reprice_up" | "promote" | "cut";
  suggested_price: Dollars; // 0 when N/A
}

export interface MenuEngineeringRollup {
  quadrant: Quadrant;
  count: number;
  revenue_share: number; // 0..1
}

export interface MenuEngineeringReport {
  period: string;
  median_food_cost_pct: number; // 0..1
  median_qty_sold: number;
  dishes: MenuEngineeringDish[];
  rollups: MenuEngineeringRollup[];
  items_needing_cost: number;
  sparse: boolean;
  /**
   * True only when the window recorded at least one sold unit. Do NOT infer
   * sales from items_needing_cost — uncosted items exist with zero sales too;
   * copy claiming "sales are recorded" must key on this flag (#834).
   */
  has_sales: boolean;
}

export const menuEngineeringApi = {
  getMenuEngineering: async (
    businessId: string,
    period: "day" | "week" | "month" = "week",
  ): Promise<MenuEngineeringReport> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/menu-engineering",
      { params: { period } },
    );
    return response.data.data;
  },
};
