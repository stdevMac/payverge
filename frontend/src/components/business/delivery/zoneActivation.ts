import type { DeliveryZoneDto } from "@/api/delivery";

/**
 * L3-40: a zone may only be activated when it has a name and some geography
 * (postal codes and/or cities). Fee/minimum may be zero (free zone is valid).
 */
export function canActivateZone(zone: Pick<DeliveryZoneDto, "name" | "boundaries">): boolean {
  const nameOk = (zone.name ?? "").trim().length > 0;
  const postal = zone.boundaries?.postal_codes ?? [];
  const cities = zone.boundaries?.cities ?? [];
  const geographyOk = postal.length > 0 || cities.length > 0;
  return nameOk && geographyOk;
}

export function zoneActivationBlockReason(
  zone: Pick<DeliveryZoneDto, "name" | "boundaries">,
): "name" | "geography" | null {
  if ((zone.name ?? "").trim().length === 0) return "name";
  const postal = zone.boundaries?.postal_codes ?? [];
  const cities = zone.boundaries?.cities ?? [];
  if (postal.length === 0 && cities.length === 0) return "geography";
  return null;
}
