/** Surface a predefined provider belongs to (#829). */
type ProviderKind = "delivery" | "reservation";

export interface ProviderCatalogItem {
  key: string;
  name: string;
  iconPath: string;
  kind: ProviderKind;
}

export const PROVIDER_CATALOG: ProviderCatalogItem[] = [
  {
    key: "talabat",
    name: "Talabat",
    iconPath: "/images/provider-icons/talabat.svg",
    kind: "delivery",
  },
  {
    key: "careem",
    name: "Careem",
    iconPath: "/images/provider-icons/careem.svg",
    kind: "delivery",
  },
  {
    key: "deliveroo",
    name: "Deliveroo",
    iconPath: "/images/provider-icons/deliveroo.svg",
    kind: "delivery",
  },
  {
    key: "ubereats",
    name: "Uber Eats",
    iconPath: "/images/provider-icons/ubereats.svg",
    kind: "delivery",
  },
  {
    key: "doordash",
    name: "DoorDash",
    iconPath: "/images/provider-icons/doordash.svg",
    kind: "delivery",
  },
  {
    key: "zomato",
    name: "Zomato",
    iconPath: "/images/provider-icons/zomato.svg",
    kind: "delivery",
  },
  {
    key: "opentable",
    name: "OpenTable",
    iconPath: "/images/provider-icons/opentable.svg",
    kind: "reservation",
  },
  {
    key: "resy",
    name: "Resy",
    iconPath: "/images/provider-icons/resy.svg",
    kind: "reservation",
  },
];

/**
 * Couriers only — the delivery partner picker must not offer reservation
 * platforms like OpenTable/Resy (#829).
 */
export const DELIVERY_PROVIDER_CATALOG: ProviderCatalogItem[] =
  PROVIDER_CATALOG.filter((provider) => provider.kind === "delivery");

/** Reservation booking platforms, used by reservation settings. */
export const RESERVATION_PROVIDER_CATALOG: ProviderCatalogItem[] =
  PROVIDER_CATALOG.filter((provider) => provider.kind === "reservation");

export const PROVIDER_CATALOG_BY_KEY = PROVIDER_CATALOG.reduce<Record<string, ProviderCatalogItem>>(
  (acc, provider) => {
    acc[provider.key] = provider;
    return acc;
  },
  {},
);
