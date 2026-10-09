import React from "react";
import type { LoyaltyTier } from "@/api/loyalty";
import { normalizeLoyaltyTierName } from "./tierNameNormalization";

interface TierPreviewProps {
  distribution: Record<string, number>;
  total: number;
  // Tier list in ladder order. Drives sort + color of preview segments.
  // Optional so legacy callers still render with brand-only segments.
  tiers?: LoyaltyTier[];
  // Optional translator for caption strings. If absent, falls back to the
  // English defaults so test-only callers don't need to wire i18n.
  t?: (key: string, params?: Record<string, string | number>) => string;
}

// Default fallback palette used when a tier in `distribution` doesn't have
// a configured color. Picked to read on both light + dark backgrounds.
// Hex literals are intentional — these are *data* (user-pickable tier
// colors), not theme tokens, and need to round-trip through the DB.
/* eslint-disable no-restricted-syntax */
const FALLBACK_COLORS = [
  "#B7791F", // bronze-ish
  "#94A3B8", // silver-ish
  "#D69E2E", // gold-ish
  "#6B46C1", // platinum-ish
];
/* eslint-enable no-restricted-syntax */

export default function TierPreview({
  distribution,
  total,
  tiers,
  t,
}: TierPreviewProps) {
  const tx = (key: string, params?: Record<string, string | number>): string => {
    if (!t) {
      if (key === "noCustomers") return "No customers yet.";
      if (key === "ofCustomers") return `(of ${params?.total ?? 0} customers)`;
      return key;
    }
    // The upstream `t()` may not interpolate params (e.g. SimpleTranslationProvider
    // returns the raw string with `{key}` placeholders). Do a manual replacement pass
    // so callers always see resolved values.
    let result = t(key, params);
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        result = result.replace(`{${k}}`, String(v));
      }
    }
    return result;
  };

  if (total === 0) {
    return <p className="text-ink-500 text-sm">{tx("noCustomers")}</p>;
  }

  // Order the segments by tier ladder when we have it (Bronze → Silver →
  // Gold), falling back to whatever order the distribution arrived in.
  // Customers landing outside any tier (e.g. "(none)" from the backend)
  // get sorted to the end so the visual flow reads "tier ascending."
  const ordered: Array<{ name: string; count: number; color: string }> = (() => {
    if (tiers && tiers.length) {
      const tierOrder = tiers
        .slice()
        .sort((a, b) =>
          a.sort_order !== b.sort_order
            ? a.sort_order - b.sort_order
            : a.min_lifetime_spent - b.min_lifetime_spent,
        );
      const seen = new Set<string>();
      const result: Array<{ name: string; count: number; color: string }> = [];
      tierOrder.forEach((t, i) => {
        const normalizedName = normalizeLoyaltyTierName(t.name);
        if (seen.has(normalizedName)) return;
        seen.add(normalizedName);
        result.push({
          name: t.name,
          count: distribution[t.name] ?? 0,
          color: t.color || FALLBACK_COLORS[i % FALLBACK_COLORS.length],
        });
      });
      // Append any extras (e.g. "(none)" bucket) in their backend order.
      for (const [name, count] of Object.entries(distribution)) {
        if (!seen.has(normalizeLoyaltyTierName(name))) {
          result.push({
            name,
            count,
            // eslint-disable-next-line no-restricted-syntax -- data color, not a theme token
            color: "#CBD5E1", // ink-200 for un-tiered
          });
        }
      }
      return result.filter((s) => s.count > 0);
    }
    return Object.entries(distribution)
      .filter(([, c]) => c > 0)
      .map(([name, count], i) => ({
        name,
        count,
        color: FALLBACK_COLORS[i % FALLBACK_COLORS.length],
      }));
  })();

  return (
    <div>
      <div className="flex h-6 rounded-full overflow-hidden bg-warm-100">
        {ordered.map((seg) => (
          <div
            key={seg.name}
            title={`${seg.count} ${seg.name}`}
            className="h-full"
            style={{
              width: `${(seg.count / total) * 100}%`,
              backgroundColor: seg.color,
            }}
          />
        ))}
      </div>
      <p className="text-sm text-ink-600 mt-2">
        {ordered.map((s) => `${s.count} ${s.name}`).join(" · ")}{" "}
        {tx("ofCustomers", { total })}
      </p>
    </div>
  );
}
