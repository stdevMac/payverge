/** Default visible cards per kanban column (including completed). */
export const RESERVATION_BOARD_COLUMN_CAP = 25;

export type CappedColumnSlice<T> = {
  visible: T[];
  hiddenCount: number;
  total: number;
};

/**
 * Cap a kanban column to `cap` rows unless expanded. Pure helper for unit tests
 * and board rendering.
 */
export function capBoardColumnItems<T>(
  items: T[],
  options: { expanded: boolean; cap?: number } = { expanded: false },
): CappedColumnSlice<T> {
  const cap = options.cap ?? RESERVATION_BOARD_COLUMN_CAP;
  const total = items.length;
  if (options.expanded || total <= cap) {
    return { visible: items, hiddenCount: 0, total };
  }
  return {
    visible: items.slice(0, cap),
    hiddenCount: total - cap,
    total,
  };
}
