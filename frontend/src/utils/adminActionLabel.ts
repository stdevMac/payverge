/**
 * Map snake_case admin action types (e.g. `suspend_business`) to
 * human-readable labels for display in admin tables and recent-activity
 * cards. Falls back to a Title-Cased version of the input when an action
 * is not in the canonical map, so a new action type added on the backend
 * still renders reasonably without a frontend change.
 */
const ADMIN_ACTION_LABELS: Record<string, string> = {
  suspend_business: "Suspended business",
  reactivate_business: "Reactivated business",
  close_account: "Closed account",
  reset_password: "Reset password",
};

export function formatAdminActionType(actionType: string | null | undefined): string {
  if (!actionType) return "Action";
  if (ADMIN_ACTION_LABELS[actionType]) return ADMIN_ACTION_LABELS[actionType];
  // Fall back: snake_case → Sentence case. "do_foo_bar" -> "Do foo bar".
  const cleaned = actionType.replace(/_+/g, " ").trim();
  if (!cleaned) return "Action";
  return cleaned.charAt(0).toUpperCase() + cleaned.slice(1);
}
