import type { Bundle, MenuCategory, Offer } from "@/api/business";
import type { Orderability } from "@/api/orders";

/**
 * Shared storefront menu payload normalization.
 *
 * The guest menu endpoint (`GET /business/:customUrl/menu`) returns categories
 * either as an already-parsed array (`parsed_categories`, present when a
 * translation was applied) or as a raw JSON string (`categories`). Both the
 * client React Query hook (`useBusinessPageData`) and the server SSR seed in
 * `app/b/[customUrl]/page.tsx` MUST produce the exact same query-data shape,
 * or the dehydrated cache seed would not match what the client queryFn would
 * have returned.
 */
export interface StorefrontMenuQueryResult {
  categories: MenuCategory[];
  offers: Offer[];
  bundles: Bundle[];
  itemOrderability: Record<string, Orderability>;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Normalize the raw guest-menu payload into the query-data shape.
 * Defensive: categories may be a JSON string, offers/bundles may be missing,
 * item_orderability may live at the top level or under `menu`.
 *
 * NOTE: a malformed categories JSON string still throws (JSON.parse), mirroring
 * the long-standing client behavior. Callers that must never fail (the SSR
 * page) wrap this in try/catch.
 */
export function normalizeStorefrontMenuPayload(
  data: unknown,
): StorefrontMenuQueryResult {
  const payload = (isRecord(data) ? data : {}) as {
    parsed_categories?: unknown;
    categories?: unknown;
    offers?: unknown;
    bundles?: unknown;
    item_orderability?: unknown;
    menu?: unknown;
  };

  const sourceCategories = Array.isArray(payload.parsed_categories)
    ? payload.parsed_categories
    : payload.categories;
  let parsedCategories: unknown = sourceCategories;
  if (typeof sourceCategories === "string") {
    parsedCategories = JSON.parse(sourceCategories);
  }
  const categories: MenuCategory[] = Array.isArray(parsedCategories)
    ? (parsedCategories as MenuCategory[])
    : [];

  const directOrderability = isRecord(payload.item_orderability)
    ? payload.item_orderability
    : undefined;
  const nestedOrderability =
    isRecord(payload.menu) && isRecord(payload.menu.item_orderability)
      ? payload.menu.item_orderability
      : undefined;

  return {
    categories,
    offers: Array.isArray(payload.offers) ? (payload.offers as Offer[]) : [],
    bundles: Array.isArray(payload.bundles)
      ? (payload.bundles as Bundle[])
      : [],
    itemOrderability: (directOrderability ??
      nestedOrderability ??
      {}) as Record<string, Orderability>,
  };
}

/**
 * Format a menu price for the crawlable (visually hidden) SEO menu block.
 * Returns null for non-numeric garbage; falls back to the bare number when the
 * currency code is missing/invalid (Intl throws RangeError on bad codes).
 */
export function formatStorefrontSeoPrice(
  price: unknown,
  currency?: string,
): string | null {
  if (typeof price !== "number" || !Number.isFinite(price)) return null;
  if (currency && /^[A-Za-z]{3}$/.test(currency)) {
    try {
      return new Intl.NumberFormat("en", {
        style: "currency",
        currency: currency.toUpperCase(),
      }).format(price);
    } catch {
      // fall through to the bare number
    }
  }
  return String(price);
}
