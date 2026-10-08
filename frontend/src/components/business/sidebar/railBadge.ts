/**
 * Display string for the collapsed-rail notification badge. Single/double-digit
 * counts read as-is up to 9; anything larger is capped at "9+" so the number
 * always fits inside the ~64px icon rail.
 */
export function capBadge(count: number): string {
  return count > 9 ? "9+" : String(count);
}

/**
 * Sum of badge counts across a set of menu items (null treated as 0). Used to
 * roll a hidden tab's pending count up onto the collapsed "More" (⋯) icon, so
 * an operator never has a count trapped out of view while collapsed.
 */
export function sumBadges(items: { badge: number | null }[]): number {
  return items.reduce((total, item) => total + (item.badge ?? 0), 0);
}
