import { getAllActiveBills, type Bill } from "@/api/bills";

/**
 * Dashboard live active-bills reconcile (#647).
 *
 * `getAllActiveBills` is the request whose axios interceptor toasts
 * "Couldn't reach the server". A throw must not be collapsed into a
 * loaded-empty till (`items: []` + ok).
 */
export type ActiveBillsFeedResult =
  | {
      ok: true;
      items: Bill[];
      capped: boolean;
      warning?: string;
    }
  | {
      ok: false;
      items: [];
      capped: false;
      error: unknown;
    };

export async function fetchActiveBillsFeed(
  businessId: number,
): Promise<ActiveBillsFeedResult> {
  try {
    const data = await getAllActiveBills(businessId);
    return {
      ok: true,
      items: data.items,
      capped: data.capped,
      warning: data.warning,
    };
  } catch (error) {
    return { ok: false, items: [], capped: false, error };
  }
}
