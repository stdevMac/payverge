import type { Offer } from "../../../../api/business";

interface CartItemBase {
  name: string;
  price: number;
  quantity: number;
  specialRequests?: string;
}

export interface CartMenuItem extends CartItemBase {
  itemType: "menu_item";
  menuItemId: string;
  addOns?: Array<{ id?: string; name: string; price: number }>;
}

interface CartBundle extends CartItemBase {
  itemType: "bundle";
  bundleId: number;
  menuItemId?: string;
  sourceOfferId?: number;
}

interface CartBundleItem extends CartItemBase {
  itemType: "bundle_item";
  menuItemId: string;
  parentBundleId: number;
}

interface CartDiscount extends CartItemBase {
  itemType: "discount";
  sourceOfferId: number;
}

export type CartItem =
  | CartMenuItem
  | CartBundle
  | CartBundleItem
  | CartDiscount;

export interface StoredCart {
  items: CartItem[];
  timestamp: number;
  tableCode: string;
}

interface PromotionAppliedEntry {
  offer: Offer;
  amount: number;
}

export interface PromotionPreview {
  baseSubtotal: number;
  discountTotal: number;
  autoDiscountTotal: number;
  promoDiscount: number;
  discountedSubtotal: number;
  netSubtotal: number;
  tax: number;
  serviceFee: number;
  tip: number;
  finalTotal: number;
  applied: PromotionAppliedEntry[];
}

export interface BusinessCurrencies {
  default_currency: string;
  display_currency: string;
}
