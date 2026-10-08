/**
 * Tabs on the operator /account page. Keyed by the `?tab=` query param so they
 * are shareable/bookmarkable and so the email footer's "manage preferences"
 * link, which points straight at /account?tab=notifications, lands directly
 * on Notifications. "privacy" keeps the security/data-export
 * surface separate from notifications.
 */
export const ACCOUNT_TABS = ["account", "notifications", "privacy"] as const;

export type AccountTab = (typeof ACCOUNT_TABS)[number];

export function resolveAccountTab(tab: string | null | undefined): AccountTab {
  return (ACCOUNT_TABS as readonly string[]).includes(tab ?? "")
    ? (tab as AccountTab)
    : "account";
}
