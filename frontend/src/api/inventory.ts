import { axiosInstance } from "./tools/instance";

export interface InventorySettings {
  id?: number;
  business_id?: number;
  inventory_enabled: boolean;
  auto_deduct_on_order_approval: boolean;
  low_stock_warnings_enabled: boolean;
  availability_sync_mode: "warn" | "manual" | "hard_block";
  created_at?: string;
  updated_at?: string;
}

export interface InventoryItem {
  id: number;
  business_id: number;
  name: string;
  sku?: string;
  category?: string;
  unit: string;
  current_quantity: number;
  reorder_threshold: number;
  cost_per_unit: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface InventoryRecipe {
  id: number;
  business_id: number;
  menu_item_id: string;
  menu_item_name?: string;
  inventory_item_id: number;
  quantity_required: number;
  inventory_item?: InventoryItem;
  created_at: string;
  updated_at: string;
}

export interface InventoryMovement {
  id: number;
  business_id: number;
  inventory_item_id: number;
  movement_type: string;
  quantity_delta: number;
  quantity_before: number;
  quantity_after: number;
  reason?: string;
  actor?: string;
  menu_item_id?: string;
  menu_item_name?: string;
  reference_order_id?: number;
  reference_bill_id?: number;
  inventory_item?: InventoryItem;
  created_at: string;
  updated_at: string;
}

/** One page of the server-side movement ledger. */
export interface InventoryMovementsPage {
  movements: InventoryMovement[];
  /** Total rows matching the filter across the whole ledger (not just this page). */
  total: number;
  offset: number;
  limit: number;
}

export interface InventoryItemHealth {
  id: number;
  name: string;
  sku?: string;
  category?: string;
  unit: string;
  current_quantity: number;
  reorder_threshold: number;
  status: string;
}

export interface InventoryMenuItemStatus {
  menu_item_id: string;
  menu_item_name: string;
  category_name: string;
  manual_available: boolean;
  has_recipe: boolean;
  status: "ok" | "low_stock" | "out_of_stock" | "manual_unavailable" | "untracked";
  max_possible_servings: number;
  recommended_available: boolean;
  shows_warning: boolean;
  blocks_sale: boolean;
  affected_inventory: string[];
  warning_inventory: string[];
}

export interface InventorySummary {
  settings: InventorySettings;
  total_items: number;
  low_stock_items: number;
  out_of_stock_items: number;
  /** Total on-hand value in business currency (single source for the stat card). */
  total_stock_value: number;
  total_recipes: number;
  menu_items_tracked: number;
  menu_items_low_stock: number;
  menu_items_out_of_stock: number;
  low_stock_details: InventoryItemHealth[];
  out_of_stock_details: InventoryItemHealth[];
  menu_item_statuses: InventoryMenuItemStatus[];
}

export interface InventoryItemPayload {
  name: string;
  sku?: string;
  category?: string;
  unit?: string;
  current_quantity?: number;
  reorder_threshold?: number;
  cost_per_unit?: number;
  is_active?: boolean;
}

export interface InventoryRecipeEntryPayload {
  inventory_item_id: number;
  quantity_required: number;
}

export interface InventoryAdjustmentPayload {
  inventory_item_id: number;
  movement_type?: string;
  /** Relative signed delta (receive/waste). Omit when sending target_quantity. */
  quantity_change?: number;
  /**
   * Absolute on-hand target for a physical count. The backend computes the
   * delta against the live locked row, so the count is not corrupted by stock
   * that moved since page load (INV-M1). Omit when sending quantity_change.
   */
  target_quantity?: number;
  reason?: string;
}

export const inventoryApi = {
  getSettings: async (businessId: number): Promise<InventorySettings> => {
    const response = await axiosInstance.get<InventorySettings>(`/inside/businesses/${businessId}/inventory/settings`);
    return response.data;
  },
  updateSettings: async (businessId: number, settings: Partial<InventorySettings>): Promise<InventorySettings> => {
    const response = await axiosInstance.put<InventorySettings>(`/inside/businesses/${businessId}/inventory/settings`, settings);
    return response.data;
  },
  listItems: async (businessId: number, includeInactive = false): Promise<InventoryItem[]> => {
    const response = await axiosInstance.get<{ items: InventoryItem[] }>(`/inside/businesses/${businessId}/inventory/items`, {
      params: includeInactive ? { include_inactive: true } : undefined,
    });
    return response.data.items || [];
  },
  createItem: async (businessId: number, payload: InventoryItemPayload): Promise<InventoryItem> => {
    const response = await axiosInstance.post<InventoryItem>(`/inside/businesses/${businessId}/inventory/items`, payload);
    return response.data;
  },
  updateItem: async (businessId: number, itemId: number, payload: Partial<InventoryItemPayload>): Promise<InventoryItem> => {
    const response = await axiosInstance.put<InventoryItem>(`/inside/businesses/${businessId}/inventory/items/${itemId}`, payload);
    return response.data;
  },
  deleteItem: async (businessId: number, itemId: number): Promise<void> => {
    await axiosInstance.delete(`/inside/businesses/${businessId}/inventory/items/${itemId}`);
  },
  listRecipes: async (businessId: number): Promise<InventoryRecipe[]> => {
    const response = await axiosInstance.get<{ recipes: InventoryRecipe[] }>(`/inside/businesses/${businessId}/inventory/recipes`);
    return response.data.recipes || [];
  },
  replaceRecipe: async (
    businessId: number,
    menuItemId: string,
    menuItemName: string,
    entries: InventoryRecipeEntryPayload[],
  ): Promise<void> => {
    await axiosInstance.put(`/inside/businesses/${businessId}/inventory/recipes/${menuItemId}`, {
      menu_item_name: menuItemName,
      entries,
    });
  },
  listMovements: async (businessId: number, limit = 50): Promise<InventoryMovement[]> => {
    const response = await axiosInstance.get<{ movements: InventoryMovement[] }>(`/inside/businesses/${businessId}/inventory/movements`, {
      params: { limit },
    });
    return response.data.movements || [];
  },
  /**
   * Server-side movement ledger: optional item/type filters, offset paging, and
   * the total matching count so the UI can page honestly instead of filtering
   * 25 global rows in the browser. `total` is the count across the whole
   * filtered ledger, not the returned page.
   */
  listMovementsPage: async (
    businessId: number,
    opts: {
      itemId?: number;
      movementType?: string;
      limit?: number;
      offset?: number;
    } = {},
  ): Promise<InventoryMovementsPage> => {
    const params: Record<string, string | number> = {
      limit: opts.limit ?? 25,
      offset: opts.offset ?? 0,
    };
    if (opts.itemId) params.item_id = opts.itemId;
    if (opts.movementType) params.movement_type = opts.movementType;
    const response = await axiosInstance.get<InventoryMovementsPage>(
      `/inside/businesses/${businessId}/inventory/movements`,
      { params },
    );
    return {
      movements: response.data.movements || [],
      total: response.data.total ?? 0,
      offset: response.data.offset ?? params.offset as number,
      limit: response.data.limit ?? params.limit as number,
    };
  },
  createAdjustment: async (
    businessId: number,
    payload: InventoryAdjustmentPayload,
  ): Promise<{ item: InventoryItem; movement: InventoryMovement }> => {
    const response = await axiosInstance.post<{ item: InventoryItem; movement: InventoryMovement }>(
      `/inside/businesses/${businessId}/inventory/adjustments`,
      payload,
    );
    return response.data;
  },
  getSummary: async (businessId: number): Promise<InventorySummary> => {
    const response = await axiosInstance.get<InventorySummary>(`/inside/businesses/${businessId}/inventory/summary`);
    return response.data;
  },
};
