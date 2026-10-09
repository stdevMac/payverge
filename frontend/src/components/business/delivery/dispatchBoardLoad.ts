import type {
  DeliveryListResult,
  DeliveryOrder,
  DeliveryStatus,
} from "@/api/delivery";
import { isAbortError } from "@/api/tools/abort";

/** In-app tab switches abort the dispatch fetch; that is not a board failure (#715). */
export function isDispatchLoadCanceled(
  error: unknown,
  signal?: AbortSignal,
): boolean {
  return isAbortError(error) || Boolean(signal?.aborted);
}

/** Statuses still in flight — never date-filtered. */
export const DISPATCH_ACTIVE_STATUSES: DeliveryStatus[] = [
  "pending",
  "confirmed",
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
];

/** Terminal statuses shown for "today" (or history) only. */
export const DISPATCH_TERMINAL_STATUSES: DeliveryStatus[] = [
  "delivered",
  "cancelled",
  "failed",
];

/** Default window per list request for the dispatch board. */
export const DISPATCH_PAGE_SIZE = 100;

/**
 * Calendar-day start (00:00:00.000) for `timeZone` as an RFC3339 UTC timestamp.
 * Used as the `since` filter for terminal-today loads so the server drops
 * yesterday's completed deliveries before they hit the wire.
 */
export function startOfBusinessDayISO(
  timeZone?: string,
  now: Date = new Date(),
): string {
  const dayKey = businessDayKey(now, timeZone);
  // Interpret dayKey as midnight in the business zone by probing offsets.
  // Strategy: find a UTC instant whose formatted day in `timeZone` is dayKey
  // and whose local hour is 0. Walk backwards from `now` to local midnight.
  if (timeZone) {
    try {
      const parts = new Intl.DateTimeFormat("en-US", {
        timeZone,
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hourCycle: "h23",
      }).formatToParts(now);
      const get = (type: string) =>
        Number(parts.find((p) => p.type === type)?.value ?? "0");
      const localHour = get("hour");
      const localMin = get("minute");
      const localSec = get("second");
      const msIntoDay =
        ((localHour * 60 + localMin) * 60 + localSec) * 1000 +
        now.getMilliseconds();
      const midnight = new Date(now.getTime() - msIntoDay);
      // Guard: confirm the computed midnight still formats as dayKey.
      if (businessDayKey(midnight, timeZone) === dayKey) {
        return midnight.toISOString();
      }
    } catch {
      // fall through
    }
  }
  const d = new Date(now);
  d.setHours(0, 0, 0, 0);
  return d.toISOString();
}

function businessDayKey(d: Date, timeZone?: string): string {
  if (timeZone) {
    try {
      return new Intl.DateTimeFormat("en-CA", {
        timeZone,
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(d);
    } catch {
      // invalid zone
    }
  }
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export interface DispatchBoardLoadOptions {
  /** Max rows for the active-status query. */
  activeLimit?: number;
  /** Max rows for the terminal-today query. */
  terminalLimit?: number;
  /** Business IANA timezone for the terminal `since` day boundary. */
  timeZone?: string;
  /** Injectable clock (tests). */
  now?: Date;
}

export interface DispatchBoardLoadPlan {
  active: {
    status: DeliveryStatus[];
    limit: number;
  };
  terminal: {
    status: DeliveryStatus[];
    limit: number;
    since: string;
  };
}

/** Pure query plan for the dispatch board — no I/O. */
export function buildDispatchBoardLoadPlan(
  options: DispatchBoardLoadOptions = {},
): DispatchBoardLoadPlan {
  const activeLimit = options.activeLimit ?? DISPATCH_PAGE_SIZE;
  const terminalLimit = options.terminalLimit ?? DISPATCH_PAGE_SIZE;
  return {
    active: {
      status: [...DISPATCH_ACTIVE_STATUSES],
      limit: activeLimit,
    },
    terminal: {
      status: [...DISPATCH_TERMINAL_STATUSES],
      limit: terminalLimit,
      since: startOfBusinessDayISO(options.timeZone, options.now),
    },
  };
}

export interface MergedDispatchBoard {
  orders: DeliveryOrder[];
  /** Server total for active statuses (drives poll cadence). */
  activeTotal: number;
  /** Server total for terminal-today query. */
  terminalTotal: number;
  activeHasMore: boolean;
  terminalHasMore: boolean;
  hasMore: boolean;
}

/** Merge the two board queries into one ordered list + truncation flags. */
export function mergeDispatchBoardResults(
  active: DeliveryListResult,
  terminal: DeliveryListResult,
): MergedDispatchBoard {
  const byId = new Map<number, DeliveryOrder>();
  for (const o of active.deliveries ?? []) {
    byId.set(o.id, o);
  }
  for (const o of terminal.deliveries ?? []) {
    if (!byId.has(o.id)) byId.set(o.id, o);
  }
  return {
    orders: Array.from(byId.values()),
    activeTotal: active.total ?? 0,
    terminalTotal: terminal.total ?? 0,
    activeHasMore: Boolean(active.has_more),
    terminalHasMore: Boolean(terminal.has_more),
    hasMore: Boolean(active.has_more || terminal.has_more),
  };
}
