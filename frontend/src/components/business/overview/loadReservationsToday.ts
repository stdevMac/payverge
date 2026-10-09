/**
 * L9-2: ReservationsTodayCard data load with real AbortSignal cancel-on-unmount.
 */

export type ReservationsTodayDeps = {
  getStats: (
    businessId: number,
    startDate?: string,
    endDate?: string,
    signal?: AbortSignal,
  ) => Promise<{
    total?: number;
    cancelled?: number;
    no_show?: number;
    pending?: number;
  }>;
  getUpcomingReservations: (
    businessId: number,
    limit?: number,
    signal?: AbortSignal,
  ) => Promise<{ reservations?: unknown[]; total?: number }>;
  getReservations: (
    businessId: number,
    startDate?: string,
    endDate?: string,
    status?: string,
    options?: { pageSize?: number; signal?: AbortSignal },
  ) => Promise<{ total?: number }>;
};

export type ReservationsTodayResult =
  | {
      ok: true;
      todayCount: number;
      pendingCount: number;
      upcoming: unknown[];
    }
  | { ok: false; aborted: boolean };

export async function loadReservationsToday(args: {
  businessId: number;
  today: string;
  signal: AbortSignal;
  deps: ReservationsTodayDeps;
}): Promise<ReservationsTodayResult> {
  const { businessId, today, signal, deps } = args;

  try {
    const [stats, upcoming, pendingList] = await Promise.all([
      deps.getStats(businessId, today, today, signal),
      deps.getUpcomingReservations(businessId, 5, signal),
      deps.getReservations(businessId, undefined, undefined, "pending", {
        pageSize: 1,
        signal,
      }),
    ]);

    if (signal.aborted) return { ok: false, aborted: true };

    const total = Number(stats?.total ?? 0);
    const cancelledCount = Number(stats?.cancelled ?? 0);
    const noShow = Number(stats?.no_show ?? 0);

    return {
      ok: true,
      todayCount: Math.max(0, total - cancelledCount - noShow),
      pendingCount: Number(pendingList?.total ?? stats?.pending ?? 0),
      upcoming: upcoming?.reservations ?? [],
    };
  } catch (err) {
    if (signal.aborted || isAbortError(err)) {
      return { ok: false, aborted: true };
    }
    return { ok: false, aborted: false };
  }
}

function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const name = (err as { name?: string }).name;
  const code = (err as { code?: string }).code;
  return name === "AbortError" || name === "CanceledError" || code === "ERR_CANCELED";
}
