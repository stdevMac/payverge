/**
 * #773 — a live-refresh / business-fetch flake must not unmount the operator
 * shell. Auth and forbidden remain fatal. A generic/rate-limit/not-found
 * refetch that still has last-good business data keeps the dashboard.
 */
export function shouldReplaceDashboardWithFetchError(opts: {
  error: string | null;
  hasLastGoodBusiness: boolean;
  isAuthError: boolean;
  isForbiddenError: boolean;
}): boolean {
  if (!opts.error) return false;
  if (opts.isAuthError || opts.isForbiddenError) return true;
  return !opts.hasLastGoodBusiness;
}
