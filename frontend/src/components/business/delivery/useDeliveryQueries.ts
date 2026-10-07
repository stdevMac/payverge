"use client";

import { useQuery } from "@tanstack/react-query";
import { getBusiness } from "@/api/business";

/** Shared query keys for the Delivery tab — keep sub-tabs from thrashing. */
const deliveryQueryKeys = {
  business: (businessId: number) =>
    ["delivery", "business", businessId] as const,
  settings: (businessId: number) =>
    ["delivery", "settings", businessId] as const,
  drivers: (businessId: number) =>
    ["delivery", "drivers", businessId] as const,
  availableDrivers: (businessId: number) =>
    ["delivery", "available-drivers", businessId] as const,
  performance: (businessId: number) =>
    ["delivery", "performance", businessId] as const,
};

const BUSINESS_STALE_MS = 5 * 60_000;

/** Business currency/timezone for Delivery sub-tabs (shared, stable). */
export function useDeliveryBusiness(businessId: number) {
  return useQuery({
    queryKey: deliveryQueryKeys.business(businessId),
    queryFn: () => getBusiness(businessId),
    enabled: businessId > 0,
    staleTime: BUSINESS_STALE_MS,
  });
}
