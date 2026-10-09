"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { MenuCategory, MenuItem, Offer, Bundle } from "@/api/business";
import type { Orderability } from "@/api/orders";
import { getBusinessMenuByCustomUrl } from "@/api/publicBusiness";
import { guestReservationAPI, ReservationSettingsDto } from "@/api/reservations";
import { guestDeliveryApi, DeliverySettingsDto } from "@/api/delivery";
import { logError } from '@/utils/errorLogger';
import { queryKeys } from "@/api/queryKeys";
import {
  normalizeStorefrontMenuPayload,
  type StorefrontMenuQueryResult,
} from "@/lib/storefront/menuPayload";
import { isRelativeOwnMediaUrl } from "@/lib/media/ownMedia";

interface ExternalPartnerLink {
  name: string;
  url: string;
  provider_key?: string;
  icon_url?: string;
}

const EMPTY_MENU: MenuCategory[] = [];
const EMPTY_OFFERS: Offer[] = [];
const EMPTY_BUNDLES: Bundle[] = [];
const EMPTY_ITEM_ORDERABILITY: Record<string, Orderability> = {};

const isValidHttpUrl = (value: string): boolean => {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
};

/**
 * Returns true when a URL looks like a backend seed/demo placeholder.
 * Matches paths that end in `-demo` (possibly followed by /, ?, #) to catch
 * patterns like `/r/acme-demo` or `/cities/foo/acme-demo` — the trailing form
 * used by backend seed data.  Does NOT block paths that merely start with
 * "demo" (e.g. `/demo-kitchen` would not be blocked because the slug does not
 * end in `-demo`).
 */
const isDemoSlugUrl = (u: string): boolean => {
  try {
    const { pathname } = new URL(u);
    return /-demo($|[/?#])/i.test(pathname);
  } catch {
    return false;
  }
};

/** Bare provider homepages (e.g. https://www.opentable.com) are seed placeholders. */
const isBareProviderHomepage = (u: string): boolean => {
  try {
    const { pathname } = new URL(u);
    return pathname === "/" || pathname === "";
  } catch {
    return false;
  }
};

export const normalizeExternalPartnerLinks = (raw: unknown): ExternalPartnerLink[] => {
  let parsed: unknown = raw;
  if (typeof raw === "string") {
    try { parsed = JSON.parse(raw); } catch { parsed = []; }
  }
  if (!Array.isArray(parsed)) return [];

  return parsed
    .map((item) => {
      if (!item || typeof item !== "object") return null;
      const candidate = item as Record<string, unknown>;
      const name = typeof candidate.name === "string" ? candidate.name.trim() : "";
      const url = typeof candidate.url === "string" ? candidate.url.trim() : "";
      const providerKey = typeof candidate.provider_key === "string" ? candidate.provider_key.trim().toLowerCase() : "";
      const iconUrl = typeof candidate.icon_url === "string" ? candidate.icon_url.trim() : "";
      if (!name || !url || !isValidHttpUrl(url)) return null;
      // Reject obvious demo/seed placeholder URLs: paths ending in -demo or
      // containing a /<slug>-demo/ segment (e.g. acme-demo, mara-core-kitchen-demo).
      // This is conservative — it only matches the trailing "-demo" placeholder
      // pattern used by backend seed data, not slugs that start with "demo"
      // (e.g. a real business named "Demo Kitchen" → slug "demo-kitchen" is
      // NOT blocked).
      if (isDemoSlugUrl(url)) return null;
      // Also drop bare provider homepages (OpenTable/Resy root with no venue path).
      if (isBareProviderHomepage(url)) return null;
      const hasProvider = providerKey.length > 0;
      // Uploaded icons on the default storage driver are relative
      // /media/<key> URLs; absolute ones pass isValidHttpUrl.
      const hasIconUrl =
        iconUrl.length > 0 &&
        (isValidHttpUrl(iconUrl) || isRelativeOwnMediaUrl(iconUrl));
      if (!hasProvider && !hasIconUrl) return null;
      return {
        name,
        url,
        provider_key: hasProvider ? providerKey : undefined,
        icon_url: hasIconUrl ? iconUrl : undefined,
      } as ExternalPartnerLink;
    })
    .filter((item): item is ExternalPartnerLink => item !== null)
    .slice(0, 5);
};

interface BusinessPageData {
  menu: MenuCategory[];
  offers: Offer[];
  bundles: Bundle[];
  itemOrderability: Record<string, Orderability>;
  menuSnapshotAuthoritative: boolean;
  menuLoading: boolean;
  popularItems: any[];
  googleRating: number | null;
  deliveryEnabled: boolean;
  deliveryPartnerLinks: ExternalPartnerLink[];
  deliverySettings: DeliverySettingsDto | null;
  deliverySettingsLoading: boolean;
  reservationsEnabled: boolean;
  reservationPartnerLinks: ExternalPartnerLink[];
  reservationSettings: ReservationSettingsDto | null;
  reservationSettingsLoading: boolean;
  featureTabsReady: boolean;
}

// The query-data shape is owned by lib/storefront/menuPayload.ts so the SSR
// dehydrated seed (app/b/[customUrl]/page.tsx) matches this queryFn exactly.
type MenuQueryResult = StorefrontMenuQueryResult;

export function shouldPollStorefrontMenu(
  visibleTab: string | null | undefined,
  deliveryCartNeedsMenu = false,
): boolean {
  if (visibleTab === "menu") return true;
  return visibleTab === "delivery" && deliveryCartNeedsMenu;
}

const STOREFRONT_MENU_POLL_MS = 60_000;

export function storefrontMenuRefetchInterval(
  pollMenu: boolean,
): number | false {
  return pollMenu ? STOREFRONT_MENU_POLL_MS : false;
}

export function storefrontMenuRefreshOptions(pollMenu: boolean): {
  refetchInterval: number | false;
  refetchOnWindowFocus: boolean;
  refetchIntervalInBackground: false;
} {
  return {
    refetchInterval: storefrontMenuRefetchInterval(pollMenu),
    refetchOnWindowFocus: pollMenu,
    refetchIntervalInBackground: false,
  };
}

export type BusinessPageDataOptions = {
  pollMenu?: boolean;
};

export function useBusinessPageData(
  businessId: number,
  customUrl: string,
  currentLanguage: string,
  googleReviewsEnabled?: boolean,
  googlePlaceId?: string,
  options?: BusinessPageDataOptions,
): BusinessPageData {
  // Menu query
  const {
    data: menuData,
    isLoading: menuLoading,
  } = useQuery({
    queryKey: [...queryKeys.business.menu(String(businessId)), customUrl, currentLanguage],
    queryFn: async (): Promise<MenuQueryResult> => {
      try {
        const data = await getBusinessMenuByCustomUrl(customUrl, currentLanguage);
        return normalizeStorefrontMenuPayload(data);
      } catch (error) {
        void logError(error instanceof Error ? error : String(error), 'useBusinessPageData', 'loadMenu');
        throw error;
      }
    },
    enabled: !!customUrl,
    // Background cadence is 60s, and a returning guest refetches on focus so
    // inventory is fresh immediately. The endpoint's ETag/304 keeps those
    // polls cheap, and background tabs stay paused. Both switches are off
    // unless the menu (or a delivery cart that still needs the menu) is on
    // screen.
    staleTime: 5_000,
    ...storefrontMenuRefreshOptions(options?.pollMenu !== false),
  });

  const menu = useMemo<MenuCategory[]>(
    () => menuData?.categories ?? EMPTY_MENU,
    [menuData?.categories],
  );
  const offers = useMemo<Offer[]>(
    () => menuData?.offers ?? EMPTY_OFFERS,
    [menuData?.offers],
  );
  const bundles = useMemo<Bundle[]>(
    () => menuData?.bundles ?? EMPTY_BUNDLES,
    [menuData?.bundles],
  );
  const itemOrderability = useMemo<Record<string, Orderability>>(
    () => menuData?.itemOrderability ?? EMPTY_ITEM_ORDERABILITY,
    [menuData?.itemOrderability],
  );

  // Popular items derived from menu data
  const popularItems = useMemo(() => {
    const items = menu.flatMap((category) => category.items || []);
    return items
      .filter((item: MenuItem) => {
        const decision = item.id ? itemOrderability[item.id] : undefined;
        return decision ? decision.orderable : item.is_available;
      })
      .slice(0, 6);
  }, [itemOrderability, menu]);

  // Google rating query
  const {
    data: googleRating = null,
  } = useQuery({
    queryKey: [...queryKeys.business.googleRating(String(businessId)), customUrl],
    queryFn: async (): Promise<number | null> => {
      try {
        const { getBusinessGoogleDetails } = await import("@/api/googleReviews");
        const details = await getBusinessGoogleDetails(customUrl);
        return details.place_details?.rating ?? null;
      } catch {
        // Google rating is decorative; missing-or-failed is the common case.
        return null;
      }
    },
    enabled: !!(googleReviewsEnabled && googlePlaceId && customUrl),
    staleTime: 60 * 60 * 1000, // 1 hour
  });

  // Delivery settings query
  const {
    data: deliveryData,
    isLoading: deliverySettingsLoading,
  } = useQuery({
    queryKey: queryKeys.business.deliverySettings(String(businessId)),
    queryFn: async (): Promise<{
      settings: DeliverySettingsDto | null;
      deliveryEnabled: boolean;
      deliveryPartnerLinks: ExternalPartnerLink[];
    }> => {
      try {
        const settings = await guestDeliveryApi.getSettings(businessId);
        const directDeliveryEnabled =
          Boolean(settings.delivery_enabled) && Boolean(settings.in_house_delivery_enabled);
        const deliveryPartnerLinks =
          settings.delivery_enabled && settings.third_party_enabled
            ? normalizeExternalPartnerLinks(settings.external_partner_links)
            : [];
        return {
          settings,
          deliveryEnabled: directDeliveryEnabled,
          deliveryPartnerLinks,
        };
      } catch (error) {
        void logError(error instanceof Error ? error : String(error), 'useBusinessPageData', 'loadDeliverySettings');
        return { settings: null, deliveryEnabled: false, deliveryPartnerLinks: [] };
      }
    },
    enabled: !!businessId,
    staleTime: 10 * 60 * 1000, // 10 minutes
  });

  const deliverySettings: DeliverySettingsDto | null = deliveryData?.settings ?? null;
  const deliveryEnabled: boolean = deliveryData?.deliveryEnabled ?? false;
  const deliveryPartnerLinks: ExternalPartnerLink[] = deliveryData?.deliveryPartnerLinks ?? [];

  // Reservation settings query
  const {
    data: reservationData,
    isLoading: reservationSettingsLoading,
  } = useQuery({
    queryKey: [...queryKeys.business.reservationSettings(String(businessId)), customUrl],
    queryFn: async (): Promise<{
      settings: ReservationSettingsDto | null;
      reservationsEnabled: boolean;
      reservationPartnerLinks: ExternalPartnerLink[];
    }> => {
      try {
        const settings = await guestReservationAPI.getSettings(customUrl);
        return {
          settings,
          reservationsEnabled: settings.enabled,
          reservationPartnerLinks: normalizeExternalPartnerLinks(settings.external_partner_links),
        };
      } catch (error) {
        void logError(error instanceof Error ? error : String(error), 'useBusinessPageData', 'loadReservationSettings');
        return {
          settings: null,
          reservationsEnabled: false,
          reservationPartnerLinks: [],
        };
      }
    },
    enabled: !!customUrl,
    staleTime: 10 * 60 * 1000, // 10 minutes
  });

  const reservationSettings: ReservationSettingsDto | null = reservationData?.settings ?? null;
  const reservationsEnabled: boolean = reservationData?.reservationsEnabled ?? false;
  const reservationPartnerLinks: ExternalPartnerLink[] = reservationData?.reservationPartnerLinks ?? [];

  return {
    menu,
    offers,
    bundles,
    itemOrderability,
    menuSnapshotAuthoritative: menuData !== undefined,
    menuLoading,
    popularItems,
    googleRating,
    deliveryEnabled,
    deliveryPartnerLinks,
    deliverySettings,
    deliverySettingsLoading,
    reservationsEnabled,
    reservationPartnerLinks,
    reservationSettings,
    reservationSettingsLoading,
    featureTabsReady: !deliverySettingsLoading && !reservationSettingsLoading,
  };
}

export type { ExternalPartnerLink };
