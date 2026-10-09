import { API_BASE_URL } from "@/api/tools/baseUrl";
import { authenticatedFetch } from "@/api/authenticatedFetch";

export type OperationalAlertType =
  | "order_new"
  | "kitchen_order_ready"
  | "reservation_new"
  | "reservation_approval"
  | "delivery_new"
  | "bill_new"
  | "payment_requested"
  | "payment_received"
  | "payment_refund_review"
  | "report_delivery_failed"
  | "service_call"
  | "ai_takeover";

export type OperationalAlertPreferenceType = Exclude<
  OperationalAlertType,
  "report_delivery_failed"
>;

export type OperationalAlertResourceType =
  | "order"
  | "bill"
  | "reservation"
  | "delivery"
  | "payment"
  | "alternative_payment"
  | "table"
  | "ai_conversation"
  | "report_delivery"
  | "webhook_event";

export type OperationalAlertStatus =
  | "open"
  | "claimed"
  | "resolved"
  | "dismissed";

export type OperationalAlertPriority = "low" | "normal" | "high" | "urgent";

export interface OperationalAlert {
  id: number;
  business_id: number;
  alert_type: OperationalAlertType;
  resource_type: OperationalAlertResourceType;
  resource_id: number;
  status: OperationalAlertStatus;
  priority: OperationalAlertPriority;
  title: string;
  body: string;
  claimed_by_staff_id?: number | null;
  claimed_by_user_id?: number | null;
  claimed_by_name: string | null;
  claimed_at?: string | null;
  resolved_at?: string | null;
  snoozed_until?: string | null;
  last_event_at: string;
  metadata?: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
}

export type OperationalAlertEventType =
  | "created"
  | "claimed"
  | "resolved"
  | "dismissed"
  | "reopened"
  | "status_changed"
  | "snoozed";

export interface OperationalAlertEvent {
  id: number;
  business_id: number;
  alert_id: number;
  event_type: OperationalAlertEventType;
  actor_staff_id?: number | null;
  actor_user_id?: number | null;
  actor_name?: string | null;
  source?: string | null;
  reason?: string | null;
  from_status?: OperationalAlertStatus | null;
  to_status?: OperationalAlertStatus | null;
  metadata?: Record<string, unknown> | null;
  created_at: string;
}

export interface AlertTypeSetting {
  enabled: boolean;
  repeating?: boolean;
  browser_notifications_enabled?: boolean;
  browser_notification?: boolean;
  sound_enabled?: boolean;
  sound?: boolean;
  priority?: OperationalAlertPriority;
  repeat_interval_seconds?: number | null;
}

export interface OperationalAlertSettings {
  enabled: boolean;
  browser_notifications_enabled: boolean;
  sound_enabled: boolean;
  volume: number;
  repeat_interval_seconds: number;
  // Backend-emitted alert types can precede their inclusion in the operator
  // preference schema. report_delivery_failed is informational today and is
  // deliberately absent from the settings payload.
  event_settings: Record<OperationalAlertPreferenceType, AlertTypeSetting>;
}

export interface FetchOperationalAlertsFilters {
  status?: OperationalAlertStatus[];
  types?: OperationalAlertType[];
}

/**
 * Claim/resolve raced another operator: the backend answers HTTP 409 with a
 * body carrying `claimed_by_name` and the alert's current `status`. Surfaced
 * as a typed error so the UI can show "Already claimed by {name}" + refetch
 * instead of a generic failure.
 */
export class OperationalAlertConflictError extends Error {
  readonly status = 409;
  readonly claimedByName: string | null;
  readonly alertStatus: OperationalAlertStatus | null;

  constructor(claimedByName: string | null, alertStatus: string | null) {
    super(
      claimedByName
        ? `Alert already claimed by ${claimedByName}`
        : "Alert already claimed",
    );
    this.name = "OperationalAlertConflictError";
    this.claimedByName = claimedByName;
    this.alertStatus = (alertStatus as OperationalAlertStatus | null) ?? null;
  }
}

const parseConflictBody = (
  text: string,
): { claimedByName: string | null; alertStatus: string | null } => {
  try {
    const parsed = JSON.parse(text) as Record<string, unknown>;
    // Tolerate both a flat body and one nesting the alert row.
    const nested = (parsed.alert ?? {}) as Record<string, unknown>;
    const name = parsed.claimed_by_name ?? nested.claimed_by_name;
    const status = parsed.status ?? nested.status;
    return {
      claimedByName: typeof name === "string" && name ? name : null,
      alertStatus: typeof status === "string" && status ? status : null,
    };
  } catch {
    return { claimedByName: null, alertStatus: null };
  }
};

async function request<T>(path: string, init: RequestInit): Promise<T> {
  const res = await authenticatedFetch(`${API_BASE_URL}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text();
    if (res.status === 409) {
      const { claimedByName, alertStatus } = parseConflictBody(text);
      throw new OperationalAlertConflictError(claimedByName, alertStatus);
    }
    throw new Error(`HTTP ${res.status}: ${text}`);
  }
  return res.json();
}

const buildAlertListPath = (
  businessId: number,
  filters: FetchOperationalAlertsFilters = {},
): string => {
  const params = new URLSearchParams();
  if (filters.status?.length) {
    params.set("status", filters.status.join(","));
  }
  if (filters.types?.length) {
    params.set("types", filters.types.join(","));
  }

  const query = params.toString();
  return `/inside/businesses/${businessId}/alerts${query ? `?${query}` : ""}`;
};

export async function fetchOperationalAlerts(
  businessId: number,
  filters?: FetchOperationalAlertsFilters,
): Promise<{ alerts: OperationalAlert[] }> {
  return request(buildAlertListPath(businessId, filters), { method: "GET" });
}

/**
 * "What did I miss" feed: the backend's `recent=1` mode returns the last 7
 * days of alerts (limit 100) including terminal statuses, so the popover can
 * show resolved/dismissed history alongside anything still open.
 */
export async function fetchRecentOperationalAlerts(
  businessId: number,
): Promise<{ alerts: OperationalAlert[] }> {
  const params = new URLSearchParams();
  params.set("status", "open,claimed,resolved,dismissed");
  params.set("recent", "1");
  return request(
    `/inside/businesses/${businessId}/alerts?${params.toString()}`,
    {
      method: "GET",
    },
  );
}

export async function claimOperationalAlert(
  businessId: number,
  alertId: number,
  source: string,
  staffId?: number,
): Promise<OperationalAlert> {
  return request(`/inside/businesses/${businessId}/alerts/${alertId}/claim`, {
    method: "POST",
    body: JSON.stringify({
      source,
      ...(staffId && staffId > 0 ? { staff_id: staffId } : {}),
    }),
  });
}

export async function resolveOperationalAlert(
  businessId: number,
  alertId: number,
  reason: string,
): Promise<OperationalAlert> {
  return request(`/inside/businesses/${businessId}/alerts/${alertId}/resolve`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

export async function snoozeOperationalAlert(
  businessId: number,
  alertId: number,
  until: string,
): Promise<OperationalAlert> {
  return request(`/inside/businesses/${businessId}/alerts/${alertId}/snooze`, {
    method: "POST",
    body: JSON.stringify({ until }),
  });
}

export async function fetchOperationalAlertEvents(
  businessId: number,
  alertId: number,
): Promise<{ events: OperationalAlertEvent[] }> {
  return request(`/inside/businesses/${businessId}/alerts/${alertId}/events`, {
    method: "GET",
  });
}

export async function fetchOperationalAlertSettings(
  businessId: number,
): Promise<OperationalAlertSettings> {
  return request(`/inside/businesses/${businessId}/alerts/settings`, {
    method: "GET",
  });
}

export async function updateOperationalAlertSettings(
  businessId: number,
  settings: OperationalAlertSettings,
): Promise<OperationalAlertSettings> {
  return request(`/inside/businesses/${businessId}/alerts/settings`, {
    method: "PUT",
    body: JSON.stringify(settings),
  });
}

export type { OperationalAlertType as default };
