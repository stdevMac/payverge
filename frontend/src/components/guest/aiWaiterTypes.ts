export type AiWaiterCartRequest =
  | {
      itemType: "menu_item";
      menuItemId: string;
      bundleId?: never;
      itemName: string;
      price: number;
      currency?: string;
      quantity: number;
      notes?: string;
    }
  | {
      itemType: "bundle";
      menuItemId?: never;
      bundleId: number;
      itemName: string;
      price: number;
      currency?: string;
      quantity: number;
      notes?: string;
    };

type AiWaiterCartActionError =
  | "cart_action_missing"
  | "cart_action_ambiguous"
  | "cart_action_not_ready"
  | "invalid_target"
  | "menu_item_not_found"
  | "menu_item_unavailable"
  | "bundle_not_found"
  | "bundle_unavailable"
  | "legacy_args_invalid"
  | "legacy_item_not_found"
  | "legacy_item_ambiguous";

export type AiWaiterCartActionResult =
  | {
      ok: true;
      source: "v2_stable_id" | "v1_name_compatibility";
      request: AiWaiterCartRequest;
    }
  | { ok: false; error: AiWaiterCartActionError };

export interface LegacyAiWaiterCartCall {
  item_name: string;
  item_type?: string;
  quantity?: number;
  notes?: string;
}
