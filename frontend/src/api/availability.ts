import { axiosInstance } from "@/api/tools/instance";

// Availability & time-off (Slice 3). NO money anywhere on these types — staff
// set when they can work and request time off; pay never crosses this wire.
// Mirrors backend/internal/database/availability_service.go.

export type AvailabilityKind = "preferred" | "unavailable";
export type TimeOffStatus = "pending" | "approved" | "denied" | "cancelled";

export interface StaffAvailability {
  id: number;
  business_id: number;
  staff_id: number;
  weekday: number; // 0 (Sun) – 6 (Sat)
  start_min: number; // minute-of-day, 0..1440
  end_min: number; // minute-of-day, 0..1440, strictly > start_min
  kind: AvailabilityKind;
  created_at: string;
  updated_at: string;
}

/** One window in a full-replace PUT body. The server stamps the staff_id. */
export interface AvailabilityWindowInput {
  weekday: number;
  start_min: number;
  end_min: number;
  kind: AvailabilityKind;
}

export interface TimeOffRequest {
  id: number;
  business_id: number;
  staff_id: number;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
  reason: string;
  status: TimeOffStatus;
  decided_by_staff_id: number | null;
  decided_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateTimeOffInput {
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
  reason: string;
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const availabilityApi = {
  // The caller's OWN weekly availability windows (server scopes to ctx staff_id).
  getMine: async (businessId: string): Promise<StaffAvailability[]> => {
    const res = await axiosInstance.get(base(businessId) + "/me/availability");
    return res.data.data;
  },

  // Full replace: empty array clears all windows. The server stamps staff_id and
  // rejects any body staff_id, so the caller can only write its own availability.
  putMine: async (
    businessId: string,
    windows: AvailabilityWindowInput[],
  ): Promise<StaffAvailability[]> => {
    const res = await axiosInstance.put(base(businessId) + "/me/availability", { windows });
    return res.data.data;
  },

  // Manager overlay read: every staff member's recurring availability windows
  // grouped by staff_id, in one bounded call (schedule:read). Feeds the schedule
  // builder's availability overlay + assign-time conflict warnings. The server
  // keys the map by staff_id as a string; we normalize to a numeric-keyed record.
  getTeam: async (
    businessId: string,
  ): Promise<Record<number, StaffAvailability[]>> => {
    const res = await axiosInstance.get(base(businessId) + "/team-availability");
    const raw = (res.data?.data?.staff_availabilities ?? {}) as Record<
      string,
      StaffAvailability[]
    >;
    const out: Record<number, StaffAvailability[]> = {};
    for (const [k, v] of Object.entries(raw)) out[Number(k)] = v ?? [];
    return out;
  },

  // Files a pending time-off request for the caller (status starts "pending").
  createTimeOff: async (
    businessId: string,
    input: CreateTimeOffInput,
  ): Promise<TimeOffRequest> => {
    const res = await axiosInstance.post(base(businessId) + "/me/time-off", input);
    return res.data.data;
  },

  // Row-scoped on the server: approvers (schedule:approve) see all; plain staff
  // see only their own. Optional status filter ("pending"/"approved"/…).
  listTimeOff: async (
    businessId: string,
    status?: TimeOffStatus,
  ): Promise<TimeOffRequest[]> => {
    const res = await axiosInstance.get(base(businessId) + "/time-off", {
      params: status ? { status } : undefined,
    });
    return res.data.data;
  },

  // Approve/deny a pending request (schedule:approve). 409 if no longer pending.
  decide: async (
    businessId: string,
    reqId: number,
    approve: boolean,
    reason: string,
  ): Promise<TimeOffRequest> => {
    const res = await axiosInstance.post(
      base(businessId) + "/time-off/" + reqId + "/decision",
      { approve, reason },
    );
    return res.data.data;
  },
};
