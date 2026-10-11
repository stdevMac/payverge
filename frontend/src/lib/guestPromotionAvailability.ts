import type { Orderability } from "@/api/orders";

type GuestPromoItem = {
  id?: string;
  name?: string;
  is_available?: boolean;
  isAvailable?: boolean;
};

type GuestPromoCategory = {
  items?: GuestPromoItem[] | null;
};

export type GuestPromoOffer = {
  name?: string;
  description?: string;
  is_active?: boolean;
  applicable_to?: string | null;
  target_id?: string | null;
};

export type GuestPromoBundle = {
  id?: number;
  name?: string;
  description?: string;
  is_active?: boolean;
  items?: unknown;
};

export type GuestPromoBundleItem = {
  menu_item_id: string;
  name?: string;
  quantity: number;
};

export type GuestPromotionCatalog = {
  categories?: GuestPromoCategory[] | null;
  orderability?: Record<string, Orderability | undefined> | null;
  bundles?: GuestPromoBundle[] | null;
};

/** Venue-wide states are not an 86 — the closed strip already owns that story. */
const VENUE_WIDE_ORDERABILITY = new Set([
  "business_closed",
  "ordering_disabled",
]);

export function parseGuestBundleItems(
  bundle: Pick<GuestPromoBundle, "items"> | null | undefined,
): GuestPromoBundleItem[] {
  if (!bundle?.items) return [];
  if (Array.isArray(bundle.items)) {
    if (bundle.items.length === 0) return [];
    if (typeof bundle.items[0] === "string") {
      return (bundle.items as string[]).map((id) => ({
        menu_item_id: id,
        quantity: 1,
      }));
    }
    return (bundle.items as Array<Record<string, unknown>>).map((item) => ({
      menu_item_id: String(item.menu_item_id || item.id || ""),
      name: typeof item.name === "string" ? item.name : undefined,
      quantity: Number(item.quantity) || 1,
    }));
  }
  if (typeof bundle.items !== "string") return [];
  try {
    const parsed = JSON.parse(bundle.items);
    return parseGuestBundleItems({ items: parsed });
  } catch {
    return [];
  }
}

function isGuestMenuItemSellable(
  item: GuestPromoItem | undefined,
  orderability?: Record<string, Orderability | undefined> | null,
): boolean {
  if (!item) return false;
  if (item.is_available === false || item.isAvailable === false) return false;
  const key = item.id || item.name;
  const decision = key ? orderability?.[key] : undefined;
  if (!decision) return true;
  if (decision.state && VENUE_WIDE_ORDERABILITY.has(decision.state)) {
    return true;
  }
  return decision.orderable !== false;
}

function indexCatalogItems(
  categories: GuestPromoCategory[] | null | undefined,
): {
  byId: Map<string, GuestPromoItem>;
  byName: Map<string, GuestPromoItem>;
  loaded: boolean;
} {
  const byId = new Map<string, GuestPromoItem>();
  const byName = new Map<string, GuestPromoItem>();
  let count = 0;
  for (const category of categories || []) {
    for (const item of category.items || []) {
      count += 1;
      if (item.id) byId.set(String(item.id), item);
      if (item.name) byName.set(item.name.toLowerCase(), item);
    }
  }
  return { byId, byName, loaded: count > 0 };
}

function resolveCatalogItem(
  target: string,
  index: ReturnType<typeof indexCatalogItems>,
): GuestPromoItem | undefined {
  return index.byId.get(target) || index.byName.get(target.toLowerCase());
}

export function isGuestBundleSellable(
  bundle: GuestPromoBundle,
  catalog: GuestPromotionCatalog,
): boolean {
  if (bundle.is_active === false) return false;
  const refs = parseGuestBundleItems(bundle);
  if (refs.length === 0) {
    // Unparseable / empty payload: keep rather than hide on a data quirk.
    return true;
  }
  const index = indexCatalogItems(catalog.categories);
  if (!index.loaded) return true;
  return refs.every((ref) => {
    const item =
      resolveCatalogItem(ref.menu_item_id, index) ||
      (ref.name ? resolveCatalogItem(ref.name, index) : undefined);
    if (!item) return false;
    return isGuestMenuItemSellable(item, catalog.orderability);
  });
}

export function isGuestOfferSellable(
  offer: GuestPromoOffer,
  catalog: GuestPromotionCatalog,
): boolean {
  if (offer.is_active === false) return false;
  const scope = offer.applicable_to || "all";
  const target = (offer.target_id || "").trim();

  if (scope === "item") {
    if (!target) return true;
    const index = indexCatalogItems(catalog.categories);
    if (!index.loaded) return true;
    const item = resolveCatalogItem(target, index);
    if (!item) return false;
    return isGuestMenuItemSellable(item, catalog.orderability);
  }

  if (scope === "bundle") {
    if (!target) return true;
    const bundles = catalog.bundles || [];
    if (bundles.length === 0) return true;
    const bundle = bundles.find(
      (candidate) =>
        (candidate.id != null && String(candidate.id) === target) ||
        candidate.name === target,
    );
    if (!bundle) return false;
    return isGuestBundleSellable(bundle, catalog);
  }

  return true;
}

export function filterGuestSellableOffers<T extends GuestPromoOffer>(
  offers: T[] | null | undefined,
  catalog: GuestPromotionCatalog,
): T[] {
  return (offers || []).filter((offer) => isGuestOfferSellable(offer, catalog));
}

export function filterGuestSellableBundles<T extends GuestPromoBundle>(
  bundles: T[] | null | undefined,
  catalog: GuestPromotionCatalog,
): T[] {
  return (bundles || []).filter((bundle) =>
    isGuestBundleSellable(bundle, catalog),
  );
}

type GuestBundleNameLookup =
  | GuestPromotionCatalog
  | Map<string, { name?: string } | undefined>
  | Record<string, { name?: string } | undefined>;

function localizedCatalogName(
  id: string,
  lookup: GuestBundleNameLookup,
): string | undefined {
  if (!id) return undefined;
  if (lookup instanceof Map) {
    const name = lookup.get(id)?.name?.trim();
    return name || undefined;
  }
  if (lookup && typeof lookup === "object") {
    const catalog = lookup as GuestPromotionCatalog;
    if (Array.isArray(catalog.categories) || catalog.categories === null) {
      const index = indexCatalogItems(catalog.categories);
      return resolveCatalogItem(id, index)?.name?.trim() || undefined;
    }
    const record = lookup as Record<string, { name?: string } | undefined>;
    const name = record[id]?.name?.trim();
    return name || undefined;
  }
  return undefined;
}

/** Prefer the live localized menu row; fall back to the persisted snapshot name. */
export function resolveGuestBundleChildName(
  ref: Pick<GuestPromoBundleItem, "menu_item_id" | "name">,
  lookup: GuestBundleNameLookup,
): string {
  const id = String(ref.menu_item_id || "").trim();
  return localizedCatalogName(id, lookup) || (ref.name || "").trim() || id;
}

function guestTextMatchesSearch(
  value: string | undefined | null,
  query: string,
): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (value || "").toLowerCase().includes(q);
}

export function filterGuestMerchOffers<T extends GuestPromoOffer>(
  offers: T[] | null | undefined,
  query: string,
): T[] {
  const list = offers || [];
  if (!query.trim()) return list;
  return list.filter(
    (offer) =>
      guestTextMatchesSearch(offer.name, query) ||
      guestTextMatchesSearch(offer.description, query),
  );
}

export function filterGuestMerchBundles<T extends GuestPromoBundle>(
  bundles: T[] | null | undefined,
  query: string,
  lookup: GuestBundleNameLookup,
): T[] {
  const list = bundles || [];
  if (!query.trim()) return list;
  return list.filter((bundle) => {
    if (
      guestTextMatchesSearch(bundle.name, query) ||
      guestTextMatchesSearch(bundle.description, query)
    ) {
      return true;
    }
    return parseGuestBundleItems(bundle).some((ref) =>
      guestTextMatchesSearch(resolveGuestBundleChildName(ref, lookup), query),
    );
  });
}

/** Hide leftover merch beside an empty item search (issue 417). */
export function shouldHideGuestPromoMerchForSearch(
  searchActive: boolean,
  filteredItemCount: number,
): boolean {
  return Boolean(searchActive && filteredItemCount === 0);
}

export function computeGuestFulfillmentMinimumDelta(
  cartSubtotalValue: number,
  minimumOrderAmount: number,
): number {
  return Math.max(minimumOrderAmount - cartSubtotalValue, 0);
}
