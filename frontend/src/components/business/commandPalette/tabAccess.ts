/**
 * Shared access logic for the operator dashboard.
 *
 * This is the SINGLE SOURCE OF TRUTH for "can this tab be opened, is it locked,
 * and should it be hidden entirely?" Both the sidebar (`DashboardSidebar`) and
 * the ⌘K command palette consume it, so the two navigation surfaces can never
 * drift — a tab the sidebar hides can never be offered by the palette, and vice
 * versa.
 *
 * The only lock is the server administrator's lifecycle: a suspended or closed
 * business is read-only everywhere except its settings.
 */

/** Tabs that stay reachable while a server administrator has locked the business. */
export const LOCK_EXEMPT_TABS = ["settings"] as const;

export interface AccessState {
  /** True while the lock state is still being fetched — never gate during this. */
  loading: boolean;
  /** True when a server administrator suspended or closed the business. */
  isSuspended: boolean;
}

export interface TabLockMeta {
  /** The tab is reachable but shows the lock notice instead of its panel. */
  locked: boolean;
  /** The tab should not be shown at all (staff cannot act on the lock). */
  hidden: boolean;
}

const UNLOCKED: TabLockMeta = { locked: false, hidden: false };

/**
 * Decide a tab's lock/hidden state for a given access state + role.
 *
 *   1. Loading  → never gate (avoids a flash of locks on first paint).
 *   2. Locked   → everything locks except LOCK_EXEMPT_TABS.
 * Staff never see a locked teaser, so locked tabs are `hidden` for them.
 */
export function getTabLockMeta(
  tabKey: string,
  access: AccessState,
  isStaffUser: boolean,
): TabLockMeta {
  if (access.loading) return UNLOCKED;
  if (access.isSuspended && !LOCK_EXEMPT_TABS.includes(tabKey as never)) {
    return { locked: true, hidden: isStaffUser };
  }
  return UNLOCKED;
}
