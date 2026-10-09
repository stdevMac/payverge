/**
 * L6-9: true when both ends of a custom YYYY-MM-DD range are set and start > end.
 * Lexicographic compare is safe for ISO date keys.
 */
export function isReversedCustomRange(start: string, end: string): boolean {
  return Boolean(start && end && start > end);
}
