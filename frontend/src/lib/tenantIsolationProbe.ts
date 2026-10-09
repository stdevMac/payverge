/**
 * GAP-2 — pure helpers for cross-account tenant isolation matrix.
 * Used by tenant-isolation.spec.ts against a live stack; unit-tested offline.
 */

export type IsolationOutcome = {
  owner: "A" | "B";
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  path: string;
  status: number;
  /** True when status is 403/404 (or 401) — no cross-tenant data. */
  isolated: boolean;
  /** True when the request got a 2xx: the other tenant's resource answered. */
  leaked: boolean;
  /**
   * True for any status that is neither a denial nor a 2xx (5xx, 3xx, 400,
   * ...). The probe proved nothing for that row, so the run fails.
   */
  failed: boolean;
  note?: string;
};

/** Paths to probe for owner A's token against owner B's resource IDs. */
export function buildCrossTenantMatrix(params: {
  otherBusinessId: number;
  /** Optional known foreign IDs when available; 0/empty skips that row. */
  otherBillId?: number;
  otherStaffId?: number;
  otherMenuItemId?: number;
  otherReservationId?: number;
}): Array<{ method: IsolationOutcome["method"]; path: string; label: string }> {
  const b = params.otherBusinessId;
  const rows: Array<{
    method: IsolationOutcome["method"];
    path: string;
    label: string;
  }> = [
    {
      method: "GET",
      path: `/inside/businesses/${b}`,
      label: "business detail",
    },
    {
      method: "GET",
      path: `/inside/businesses/${b}/bills`,
      label: "bills list",
    },
    {
      method: "GET",
      path: `/inside/businesses/${b}/menu`,
      label: "menu",
    },
    {
      method: "GET",
      path: `/inside/businesses/${b}/staff`,
      label: "staff",
    },
    {
      method: "GET",
      path: `/inside/businesses/${b}/reservations`,
      label: "reservations",
    },
    {
      method: "GET",
      path: `/inside/businesses/${b}/analytics/overview`,
      label: "analytics",
    },
    {
      method: "GET",
      path: `/inside/businesses/${b}/settings`,
      label: "settings",
    },
    {
      method: "PUT",
      path: `/inside/businesses/${b}`,
      label: "business mutate",
    },
  ];
  if (params.otherBillId) {
    rows.push({
      method: "GET",
      path: `/inside/businesses/${b}/bills/${params.otherBillId}`,
      label: "bill by id",
    });
  }
  if (params.otherStaffId) {
    rows.push({
      method: "GET",
      path: `/inside/businesses/${b}/staff/${params.otherStaffId}`,
      label: "staff by id",
    });
  }
  if (params.otherMenuItemId) {
    rows.push({
      method: "GET",
      path: `/inside/businesses/${b}/menu/items/${params.otherMenuItemId}`,
      label: "menu item by id",
    });
  }
  if (params.otherReservationId) {
    rows.push({
      method: "GET",
      path: `/inside/businesses/${b}/reservations/${params.otherReservationId}`,
      label: "reservation by id",
    });
  }
  return rows;
}

/**
 * Classify an HTTP status for tenant isolation.
 * 401/403/404 = denied (isolated); 2xx = leak; anything else (5xx, 3xx, 400,
 * ...) = failed: the request errored before the tenant check could be
 * observed, so it proves nothing and must fail the matrix.
 */
export function classifyIsolationStatus(status: number): {
  isolated: boolean;
  leaked: boolean;
  failed: boolean;
} {
  if (status === 401 || status === 403 || status === 404) {
    return { isolated: true, leaked: false, failed: false };
  }
  if (status >= 200 && status < 300) {
    return { isolated: false, leaked: true, failed: false };
  }
  return { isolated: false, leaked: false, failed: true };
}
