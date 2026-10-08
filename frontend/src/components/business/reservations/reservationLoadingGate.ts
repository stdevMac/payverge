/**
 * When to replace the list surface with the full ReservationsSkeleton.
 * Only before the ACTIVE view's data source has completed its first load —
 * background refreshes keep prior rows visible. Keying on the active source
 * (paged rows for the plain list, the complete hydrate for the board/derived
 * filters) instead of "any data loaded" prevents a dishonest "no reservations"
 * empty-state flash when switching into a view whose source is still loading.
 * `!activeSourceHasLoaded` alone (not `loading && …`) also covers the frame
 * between a view switch and its load effect flipping `loading` on.
 */
export function shouldShowReservationsSkeleton(
  activeSourceHasLoaded: boolean,
): boolean {
  return !activeSourceHasLoaded;
}

/**
 * Lightweight "refreshing" affordance after the active source's first
 * successful load.
 */
export function shouldShowReservationsRefreshing(
  loading: boolean,
  activeSourceHasLoaded: boolean,
): boolean {
  return loading && activeSourceHasLoaded;
}
