export type InsightLike = { type: string };

/**
 * Quick Action keys that would repeat a briefing already shown by
 * ProactiveInsights (issue 63).
 */
export function quickActionKeysSuppressedByInsights(
  insights: readonly InsightLike[],
): Set<string> {
  const keys = new Set<string>();
  for (const insight of insights) {
    if (
      insight.type === "inventory_out_of_stock" ||
      insight.type === "inventory_low_stock"
    ) {
      keys.add("inventory-alerts");
    }
    if (insight.type === "stale_open_bills") {
      keys.add("active-bills");
    }
  }
  return keys;
}

export function filterQuickActionsDuplicatingInsights<
  T extends { key: string },
>(actions: readonly T[], insights: readonly InsightLike[]): T[] {
  const suppressed = quickActionKeysSuppressedByInsights(insights);
  if (suppressed.size === 0) return [...actions];
  return actions.filter((action) => !suppressed.has(action.key));
}
