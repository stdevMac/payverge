import type { OperationalAlert } from "@/api/operationalAlerts";

/** One typical dining seating. Must stay aligned with backend ServiceCallTTL. */
export const SERVICE_CALL_TTL_MS = 90 * 60 * 1000;

type ServiceCallClock = Pick<
  OperationalAlert,
  "alert_type" | "status" | "last_event_at" | "created_at" | "metadata"
>;

/**
 * A raised hand older than the seating SLA is not a live floor alert.
 * Claimed/assigned calls stay live. reason=check stays live here because
 * occupancy is server-side: the API omits empty-table / newer-bill leftovers
 * and only returns unpaid same-seating check-please. A missing last_event_at
 * is not stale — same fail-closed clock as GET.
 */
export function isStaleServiceCall(
  alert: ServiceCallClock,
  nowMs: number = Date.now(),
): boolean {
  if (alert.alert_type !== "service_call") return false;
  if (alert.status === "claimed") return false;
  if (alert.status !== "open") return false;
  if (alert.metadata?.reason === "check") return false;
  const raw = alert.last_event_at;
  if (!raw) return false;
  const ts = Date.parse(raw);
  if (!Number.isFinite(ts)) return false;
  return nowMs - ts >= SERVICE_CALL_TTL_MS;
}
