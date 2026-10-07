/**
 * L9-2: core overview widget loads with real AbortSignal cancel-on-unmount.
 *
 * Extracted so tests can drive the shipped path without mounting the full
 * BusinessOverview shell. Callers must pass `signal` from an AbortController
 * they abort in effect cleanup / refresh supersession.
 */

type OverviewTablesResult = { tables: unknown[] };

export type OverviewCoreWidgetsResult = {
  dashboard: unknown | null;
  analyticsError: boolean;
  tables: unknown[];
  tablesError: boolean;
};

export type OverviewCoreWidgetDeps = {
  getDashboardSummary: (
    businessId: string,
    signal?: AbortSignal,
  ) => Promise<unknown>;
  getBusinessTables: (
    businessId: number,
    signal?: AbortSignal,
  ) => Promise<OverviewTablesResult>;
};

export async function loadOverviewCoreWidgets(args: {
  businessId: number;
  canSeeAnalytics: boolean;
  canSeeTables: boolean;
  isRestricted: boolean;
  signal: AbortSignal;
  deps: OverviewCoreWidgetDeps;
}): Promise<OverviewCoreWidgetsResult> {
  const {
    businessId,
    canSeeAnalytics,
    canSeeTables,
    isRestricted,
    signal,
    deps,
  } = args;

  let analyticsError = false;
  let tablesError = false;
  let dashboard: unknown | null = null;
  let tables: unknown[] = [];

  const analyticsPromise =
    canSeeAnalytics && !isRestricted
      ? deps.getDashboardSummary(String(businessId), signal).catch((err) => {
          if (signal.aborted || isAbortError(err)) {
            return null;
          }
          analyticsError = true;
          return null;
        })
      : Promise.resolve(null);

  const tablesPromise = canSeeTables
    ? deps.getBusinessTables(businessId, signal).catch((err) => {
        if (signal.aborted || isAbortError(err)) {
          return { tables: [] as unknown[], _aborted: true as const };
        }
        tablesError = true;
        // Do not fabricate a successful empty board — mark error and return
        // no tables so the UI can show a stale banner instead of "0 tables".
        return { tables: [] as unknown[], _failed: true as const };
      })
    : Promise.resolve({ tables: [] as unknown[] });

  const [dashboardResponse, tablesResponse] = await Promise.all([
    analyticsPromise,
    tablesPromise,
  ]);

  if (signal.aborted) {
    return {
      dashboard: null,
      analyticsError: false,
      tables: [],
      tablesError: false,
    };
  }

  dashboard = dashboardResponse;
  if (dashboardResponse) analyticsError = false;

  const rawTables = (tablesResponse as OverviewTablesResult | undefined)?.tables;
  tables = Array.isArray(rawTables) ? rawTables : [];
  if (
    tablesResponse &&
    typeof tablesResponse === "object" &&
    "_failed" in (tablesResponse as object)
  ) {
    tablesError = true;
  }

  return { dashboard, analyticsError, tables, tablesError };
}

function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const name = (err as { name?: string }).name;
  const code = (err as { code?: string }).code;
  return name === "AbortError" || name === "CanceledError" || code === "ERR_CANCELED";
}
