/**
 * Money-honesty rules for the operator Bills tab (#647).
 *
 * A failed or unknown list must never render as a genuine empty till
 * ("0 Resultados · 0,00 US$"). Empty/zero is only legal after a successful
 * hydrate that is not contradicted by a failed live active-bills feed.
 */

export interface BillsListHonestyInput {
  /** True after a list response with a real `bills` array has been applied. */
  listHydrated: boolean;
  /** True when the paged list fetch threw (network / 5xx / invalid payload). */
  listLoadFailed: boolean;
  /**
   * True when the dashboard's `getAllActiveBills` reconcile threw. That is
   * the request whose axios interceptor toasts "Couldn't reach the server".
   */
  globalBillsFailed: boolean;
  resultCount: number;
  visibleValue: number;
  /**
   * True when the current list query is scoped (search, dates, customer, or
   * History). A later empty scoped result must not inherit a sticky live-feed
   * fail flag as if the till failed to load.
   */
  queryIsScoped: boolean;
}

export interface BillsListHonesty {
  /** Show retry chrome (full empty-state or a stale banner). */
  showFailureState: boolean;
  /** Paint header result count + listed total. */
  showMoneyStats: boolean;
  /** Keep the last successful rows instead of replacing them with EmptyState. */
  keepLastKnownList: boolean;
}

export function resolveBillsListHonesty(
  input: BillsListHonestyInput,
): BillsListHonesty {
  const hasKnownMoney =
    input.listHydrated &&
    (input.resultCount > 0 || input.visibleValue > 0);

  if (input.listLoadFailed && hasKnownMoney) {
    return {
      showFailureState: true,
      showMoneyStats: true,
      keepLastKnownList: true,
    };
  }

  // A failed local fetch with no last-known money, OR an unfiltered Active
  // empty while the live feed failed. A later search/history empty must not
  // inherit the sticky live-feed flag as a fake outage.
  if (
    input.listLoadFailed ||
    (input.globalBillsFailed && !hasKnownMoney && !input.queryIsScoped)
  ) {
    return {
      showFailureState: true,
      showMoneyStats: false,
      keepLastKnownList: false,
    };
  }

  if (!input.listHydrated) {
    return {
      showFailureState: false,
      showMoneyStats: false,
      keepLastKnownList: false,
    };
  }

  return {
    showFailureState: false,
    showMoneyStats: true,
    keepLastKnownList: false,
  };
}
