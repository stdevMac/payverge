/**
 * RV-4 — guest menu sticky chrome measurement gate.
 *
 * HEADER_OFFSET must match the sticky header height used by IntersectionObserver
 * and layout. After scroll-collapse of the brand strip, the occupied height is
 * lower than the audit's 254–288px full stack.
 *
 * GUEST_NAV_HEIGHT_PX is owned by PersistentGuestNav; re-exported here so the
 * chrome budget test has one import surface.
 */
import { GUEST_NAV_HEIGHT_PX } from "@/components/navigation/PersistentGuestNav";

/** Sticky menu header offset (px) after compact brand strip — keep in sync with page.tsx. */
export const GUEST_MENU_HEADER_OFFSET_PX = 152;

/** Total permanent chrome budget: sticky menu header + bottom guest nav. */
export const GUEST_MENU_PERMANENT_CHROME_PX =
  GUEST_MENU_HEADER_OFFSET_PX + GUEST_NAV_HEIGHT_PX;

export { GUEST_NAV_HEIGHT_PX };

/** Upper bound gate: must stay under ~half of a short phone viewport (844). */
export const GUEST_MENU_CHROME_BUDGET_MAX_PX = 240;

/**
 * RV-4 presentation helpers — page.tsx must use these so collapse reclaims
 * vertical space (table label hidden, tighter padding, smaller title).
 */
export function guestMenuChromeBarPaddingClass(compact: boolean): string {
  return compact ? "py-1.5" : "py-3";
}

/** NEW-10: always heading-sm — heading-md ellipsized venue names at 390px. */
export function guestMenuChromeTitleClass(compact: boolean): string {
  void compact;
  return "text-heading-sm sm:text-heading-md";
}

/** Stable id for the guest-menu Filters disclosure panel (#423). */
export const GUEST_MENU_FILTERS_PANEL_ID = "guest-menu-filters-panel";

/** Table label is dropped when compact to reclaim ~20–24px. */
export function guestMenuChromeShowsTableLabel(compact: boolean): boolean {
  return !compact;
}

