/**
 * Guest-menu load helpers.
 *
 * The page used to wait on Promise.allSettled(table, bill, business) before
 * leaving the "Loading menu…" skeleton. A hanging bill/business request (axios
 * can sit for 30s, plus retries) left the essential table/menu payload unused.
 * These helpers apply the table result independently and bound every wait.
 */

import {
  classifyTableLoadFailure,
  isTransientTableMiss,
  type TableLoadErrorKind,
} from "./classifyTableLoadFailure";
export const MENU_DEPENDENCY_TIMEOUT_MS = 8000;

export type Settled<T> =
  | { status: "fulfilled"; value: T }
  | { status: "rejected"; reason: unknown };

export function shouldShowMenuLoadingGate(
  loading: boolean,
  tableData: { table?: { table_code?: string } } | null,
  tableCode?: string,
): boolean {
  if (!loading) return false;
  if (!tableData) return true;
  if (tableCode && tableData.table?.table_code) {
    const have = tableData.table.table_code.trim().toUpperCase();
    const want = tableCode.trim().toUpperCase();
    if (have && want && have !== want) return true;
  }
  return false;
}

export async function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  label = "timeout",
): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      const err = new Error(label);
      (err as Error & { code?: string }).code = "ECONNABORTED";
      reject(err);
    }, ms);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

async function settleWithTimeout<T>(
  promise: Promise<T>,
  ms: number,
  label = "timeout",
): Promise<Settled<T>> {
  try {
    return { status: "fulfilled", value: await withTimeout(promise, ms, label) };
  } catch (reason) {
    return { status: "rejected", reason };
  }
}

export async function loadGuestMenuDependencies<TTable, TBill, TBusiness>(opts: {
  table: () => Promise<TTable>;
  bill: () => Promise<TBill>;
  business: () => Promise<TBusiness>;
  timeoutMs?: number;
}): Promise<{
  table: Settled<TTable>;
  rest: Promise<{ bill: Settled<TBill>; business: Settled<TBusiness> }>;
}> {
  const timeoutMs = opts.timeoutMs ?? MENU_DEPENDENCY_TIMEOUT_MS;
  const tableP = settleWithTimeout(opts.table(), timeoutMs, "table");
  const billP = settleWithTimeout(opts.bill(), timeoutMs, "bill");
  const businessP = settleWithTimeout(opts.business(), timeoutMs, "business");

  // A live table can miss on the first hop and resolve on reload. The miss is
  // not always a rejection: an edge/proxy hiccup answers 200 with an empty
  // body, which axios resolves with a falsy `data`. Spend exactly one more hop
  // for BOTH shapes before this route is allowed to render "table not found"
  // (issue 682 — /bill already does this; the QR landing surface did not).
  let table = await tableP;
  if (isTransientTableMiss(table)) {
    table = await settleWithTimeout(opts.table(), timeoutMs, "table");
  }
  return {
    table,
    rest: Promise.all([billP, businessP]).then(([bill, business]) => ({
      bill,
      business,
    })),
  };
}

export type GuestTableSettlement<T> =
  | { kind: "ready"; value: T }
  | { kind: "error"; error: TableLoadErrorKind };

/**
 * Single sink for "can this route paint the menu?".
 *
 * A fulfilled settlement is NOT proof the table exists: a 200 whose body was
 * lost in transit resolves with no `table`. Treating that as success left the
 * page with falsy table data and no error kind, which fell through to the hard
 * 404 / "your QR is wrong" screen. An unproven miss is always `network` —
 * retryable copy — and only a real 404 rejection earns `not_found`.
 */
export function resolveGuestTableSettlement<T>(
  settled: Settled<T>,
): GuestTableSettlement<T> {
  if (settled.status === "rejected") {
    return { kind: "error", error: classifyTableLoadFailure(settled.reason) };
  }
  if (!(settled.value as { table?: unknown } | undefined)?.table) {
    return { kind: "error", error: "network" };
  }
  return { kind: "ready", value: settled.value };
}
