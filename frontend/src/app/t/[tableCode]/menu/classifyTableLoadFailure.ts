/**
 * Classifies a failed getTableByCode (or equivalent) rejection into a guest-facing
 * error mode. Network / 5xx / offline must NOT be presented as "table not found"
 * — that makes diners believe the QR is wrong when the API is simply down.
 */
export type TableLoadErrorKind = "not_found" | "network";

export function classifyTableLoadFailure(reason: unknown): TableLoadErrorKind {
  if (reason == null) return "network";

  const r = reason as {
    response?: { status?: number };
    status?: number;
    message?: string;
    code?: string;
  };

  const status = r.response?.status ?? r.status;
  if (status === 404) return "not_found";

  // Axios-style offline / no response
  if (r.code === "ERR_NETWORK" || r.code === "ECONNABORTED") return "network";

  if (typeof r.message === "string") {
    if (/\b404\b/.test(r.message)) return "not_found";
    if (/network|timeout|failed to fetch|ECONNREFUSED/i.test(r.message)) {
      return "network";
    }
  }

  // Default to network so we never claim the table is missing without a 404.
  return "network";
}

export type SettledTableLookup =
  | { status: "fulfilled"; value: unknown }
  | { status: "rejected"; reason: unknown };

/**
 * A guest table lookup that must be retried once before any surface is allowed
 * to tell a diner the code does not exist. Two shapes qualify:
 *  - a rejection classified as 404 (a live code can 404 on one edge hop), and
 *  - a FULFILLED response carrying no table (a 200 whose body was lost in
 *    transit resolves as a falsy/empty `data`, so it never rejects and a
 *    404-only retry skips it entirely).
 *
 * Shared by /t/[code]/bill and /t/[code]/menu so the QR landing surface and the
 * check surface agree on what "missing" means (issue 682).
 */
export function isTransientTableMiss(result: SettledTableLookup): boolean {
  if (result.status === "rejected") {
    return classifyTableLoadFailure(result.reason) === "not_found";
  }
  return !(result.value as { table?: unknown } | undefined)?.table;
}
