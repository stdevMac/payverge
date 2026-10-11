import React from "react";
import {
  Award,
  Car,
  Clock,
  Coffee,
  CreditCard,
  Dog,
  Flame,
  Gift,
  Heart,
  Leaf,
  MapPin,
  Music,
  ParkingCircle,
  Phone,
  Salad,
  Shield,
  Sparkles,
  Star,
  Sun,
  TreePalm,
  Truck,
  Users,
  Utensils,
  Wifi,
  Wine,
  Zap,
} from "lucide-react";

/** Keys that match SpecialFeaturesEditor ICON_OPTIONS (and stored feature.icon). */
export const FEATURE_ICON_KEYS = [
  "wifi",
  "car",
  "credit-card",
  "coffee",
  "music",
  "utensils",
  "shield",
  "heart",
  "star",
  "gift",
  "zap",
  "users",
  "clock",
  "map-pin",
  "phone",
  "award",
] as const;

type FeatureIconKey = (typeof FEATURE_ICON_KEYS)[number];

const ICON_BY_KEY: Record<
  FeatureIconKey,
  React.ComponentType<{ className?: string }>
> = {
  wifi: Wifi,
  car: Car,
  "credit-card": CreditCard,
  coffee: Coffee,
  music: Music,
  utensils: Utensils,
  shield: Shield,
  heart: Heart,
  star: Star,
  gift: Gift,
  zap: Zap,
  users: Users,
  clock: Clock,
  "map-pin": MapPin,
  phone: Phone,
  award: Award,
};

// Title-heuristic map (public storefront fallback when icon is unset).
// Order matters: longer keys sorted first at match time.
const TITLE_HEURISTICS: Record<string, FeatureIconKey | string> = {
  delivery: "delivery",
  takeout: "delivery",
  loyalty: "award",
  rewards: "award",
  points: "award",
  family: "users",
  kids: "users",
  group: "users",
  party: "users",
  local: "salad",
  ingredient: "salad",
  ingredients: "salad",
  seasonal: "leaf",
  organic: "leaf",
  vegan: "leaf",
  vegetarian: "leaf",
  fast: "zap",
  quick: "zap",
  scan: "zap",
  qr: "zap",
  ordering: "utensils",
  menu: "utensils",
  outdoor: "sun",
  patio: "terrace",
  terrace: "terrace",
  sun: "sun",
  dog: "dog",
  pet: "dog",
  wifi: "wifi",
  bar: "wine",
  wine: "wine",
  music: "music",
  live: "music",
  coffee: "coffee",
  parking: "car",
  grill: "flame",
};

const EXTRA_ICONS: Record<string, React.ComponentType<{ className?: string }>> =
  {
    delivery: Truck,
    salad: Salad,
    leaf: Leaf,
    sun: Sun,
    terrace: TreePalm,
    dog: Dog,
    wine: Wine,
    flame: Flame,
    parking: ParkingCircle,
  };

const FEATURE_ICON_KEY_SET = new Set<string>(FEATURE_ICON_KEYS);

function tokenizeFeatureTitle(title: string): string[] {
  const tokens = title.toLowerCase().match(/[\p{L}]+/gu) || [];
  return tokens.filter((tok) => tok.length >= 3);
}

/**
 * Resolve a stable icon key for a special feature.
 * Prefers the stored `icon` field (editor key); falls back to title heuristics.
 */
export function resolveFeatureIconKey(
  icon: string | undefined | null,
  title?: string | undefined | null,
): string {
  const normalized = (icon || "").trim().toLowerCase();
  if (normalized && FEATURE_ICON_KEY_SET.has(normalized)) {
    return normalized;
  }
  // Aliases operators may have typed historically.
  if (normalized === "parking" || normalized === "parking-circle") {
    return "car";
  }
  if (normalized === "creditcard" || normalized === "card") {
    return "credit-card";
  }
  if (normalized === "mappin" || normalized === "location") {
    return "map-pin";
  }

  if (title) {
    const words = new Set(tokenizeFeatureTitle(title));
    const keys = Object.keys(TITLE_HEURISTICS).sort(
      (a, b) => b.length - a.length,
    );
    for (const key of keys) {
      if (words.has(key)) {
        const mapped = TITLE_HEURISTICS[key];
        if (FEATURE_ICON_KEY_SET.has(mapped)) return mapped;
        return mapped; // extra heuristic key (delivery, salad, …)
      }
    }
  }

  return "award";
}

export function getFeatureIconComponent(
  iconKey: string,
): React.ComponentType<{ className?: string }> {
  if (FEATURE_ICON_KEY_SET.has(iconKey)) {
    return ICON_BY_KEY[iconKey as FeatureIconKey];
  }
  if (EXTRA_ICONS[iconKey]) {
    return EXTRA_ICONS[iconKey];
  }
  return Sparkles;
}

/** Resolve icon component from feature.icon + feature.title. */
export function pickFeatureIcon(
  icon?: string | null,
  title?: string | null,
): React.ComponentType<{ className?: string }> {
  return getFeatureIconComponent(resolveFeatureIconKey(icon, title));
}
