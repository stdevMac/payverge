import { axiosInstance } from "./tools/instance";
import type { FetchAllPagesResult, PaginatedResponse, PaginationOptions } from "./pagination";
import { fetchAllPages } from "./pagination";

interface ExternalPartnerLink {
  name: string;
  url: string;
  provider_key?: string;
  icon_url?: string;
}

export interface ReservationSettingsDto {
  id: number;
  business_id: number;
  enabled: boolean;
  max_advance_days: number;
  min_advance_minutes: number;
  min_party_size: number;
  max_party_size: number;
  default_duration: number;
  slot_interval_minutes: number;
  service_buffer_minutes: number;
  max_covers_per_slot: number;
  auto_assign_tables: boolean;
  approval_mode: "auto" | "manual";
  allow_waitlist: boolean;
  hold_duration_minutes: number;
  allow_cancellation: boolean;
  cancellation_deadline: number;
  no_show_grace_minutes: number;
  send_confirmation_email: boolean;
  send_reminder_email: boolean;
  reminder_hours_before: number;
  external_partner_links: ExternalPartnerLink[];
  next_available_slot?: string;
  available_slot_count?: number;
  created_at: string;
  updated_at: string;
}

export type ReservationStatus =
  | "pending"
  | "confirmed"
  | "waitlist"
  | "seated"
  | "completed"
  | "cancelled"
  | "no_show";

interface ReservationStatusHistory {
  id: number;
  reservation_id: number;
  status: ReservationStatus;
  table_id?: number | null;
  notes?: string;
  changed_by?: string;
  created_at: string;
  table?: {
    id: number;
    name: string;
    table_code: string;
    capacity: number;
  };
}

export interface Reservation {
  id: number;
  business_id: number;
  table_id?: number | null;
  customer_name: string;
  customer_phone?: string;
  customer_email?: string;
  party_size: number;
  reservation_time: string;
  duration: number;
  status: ReservationStatus;
  source?: string;
  confirmation_code?: string;
  /** Guest locale captured at booking (i18n Batch B). */
  language?: string;
  confirmed_at?: string | null;
  assigned_at?: string | null;
  seated_at?: string | null;
  completed_at?: string | null;
  cancelled_at?: string | null;
  cancelled_by?: string;
  cancellation_reason?: string;
  waitlist_position?: number | null;
  special_requests?: string;
  notes?: string;
  created_by?: string;
  /** L4-8 multi-operator claim lock (null/absent = unclaimed). */
  claimed_by_staff_id?: number | null;
  claimed_by_name?: string;
  claimed_by_role?: string;
  claimed_at?: string | null;
  created_at: string;
  updated_at: string;
  table?: {
    id: number;
    name: string;
    table_code: string;
    capacity: number;
  };
  status_history?: ReservationStatusHistory[];
}

/** L4-8 claim response / conflict envelope. */
export interface ReservationClaimState {
  claimed_by_staff_id?: number | null;
  claimed_by_name?: string;
  claimed_by_role?: string;
  claimed_at?: string | null;
}

/** 409 body when claim gate blocks a reservation claim or mutate. */
export interface ReservationClaimConflict {
  code?: string;
  claimed_by_staff_id?: number | null;
  claimed_by_name?: string;
  claimed_by_role?: string;
  error?: string;
}

export interface CreateReservationRequest {
  table_id?: number | null;
  customer_name: string;
  customer_phone?: string;
  customer_email?: string;
  party_size: number;
  reservation_time: string;
  duration?: number;
  special_requests?: string;
  notes?: string;
  /** Guest locale at booking time (e.g. "fr", "es-AR"). Persisted on the row. */
  language?: string;
  allow_occupied_table_override?: boolean;
}

export interface UpdateReservationRequest {
  table_id?: number | null;
  clear_table?: boolean;
  customer_name?: string;
  customer_phone?: string;
  customer_email?: string;
  party_size?: number;
  reservation_time?: string;
  duration?: number;
  status?: ReservationStatus;
  special_requests?: string;
  notes?: string;
  cancellation_reason?: string;
  allow_occupied_table_override?: boolean;
}

export interface ReservationTransitionRequest {
  table_id?: number;
  notes?: string;
  reason?: string;
  allow_occupied_table_override?: boolean;
}

export interface ReservationStats {
  total: number;
  pending: number;
  confirmed: number;
  waitlist: number;
  seated: number;
  completed: number;
  cancelled: number;
  no_show: number;
  /**
   * SUM(party_size) over non-cancelled/non-no-show rows — true covers
   * (guests), not a booking count. Optional: absent on older backends
   * (BE-first), consumers fall back to the booking count.
   */
  covers?: number;
  /** Active bookings (pending/confirmed/waitlist) without a table. Optional. */
  needs_table?: number;
}

interface ReservationGuestHistory {
  prior_no_shows: number;
  prior_visits: number;
}

export interface ReservationsResponse extends PaginatedResponse<Reservation> {
  reservations: Reservation[];
  total: number;
  /** Wave 4: only present on status=pending list responses. */
  guest_history?: Record<string, ReservationGuestHistory>;
}

export interface ReservationListOptions extends PaginationOptions {
  disableCache?: boolean;
  /** Optional free-text search across customer name/phone/email (server `q`). */
  q?: string;
  /** Cancel in-flight list fetches on unmount (L9-2). */
  signal?: AbortSignal;
}

export interface ReservationAvailabilitySlotDto {
  time: string;
  available_tables: number;
  recommended: boolean;
  reason_code: string;
}

export interface ReservationAvailabilityDto {
  date: string;
  party_size: number;
  available_slots: ReservationAvailabilitySlotDto[];
  total_slots: number;
  next_available_slot?: string;
  waitlist_available: boolean;
}

type ReservationOccupancyState =
  | "available"
  | "occupied"
  | "stale_occupied";

export interface ReservationTableOption {
  id: number;
  name: string;
  capacity: number;
  /** Present when the table is linked to a space (soft picker grouping). */
  space_id?: number | null;
  space_name?: string;
  occupancy_state: ReservationOccupancyState;
  active_bill_id?: number;
  active_bill_opened_at?: string;
  active_bill_age_minutes?: number;
  reservation_available: boolean;
  recommended: boolean;
  requires_occupancy_override: boolean;
  conflict_reason?:
    | "capacity"
    | "reservation_conflict"
    | "occupied"
    | "stale_occupied";
}

export interface ReservationTableOptionsResponse {
  business_timezone: string;
  near_term: boolean;
  tables: ReservationTableOption[];
}

export interface ReservationActionResponse {
  message: string;
  reservation: Reservation;
  business_name?: string;
  businessName?: string;
  business_custom_url?: string;
  business_timezone?: string;
}

/**
 * Response shape of the public guest create endpoint. For manual-approval
 * businesses, `approval_deadline` (RFC3339 UTC) is included when the
 * reservation lands in "pending" — the guest must hear back by then.
 */
export interface GuestReservationCreatedResponse extends Reservation {
  approval_deadline?: string;
}

interface PublicReservation {
  id: number;
  customer_name: string;
  customer_email?: string;
  customer_phone?: string;
  party_size: number;
  reservation_time: string;
  duration: number;
  status: ReservationStatus;
  source?: string;
  confirmation_code: string;
  /** Guest locale captured at booking (i18n Batch B). */
  language?: string;
  special_requests?: string;
  confirmed_at?: string | null;
  assigned_at?: string | null;
  seated_at?: string | null;
  completed_at?: string | null;
  cancelled_at?: string | null;
  waitlist_position?: number | null;
  table_name?: string;
}

export interface PublicReservationDetailsResponse {
  reservation: PublicReservation;
  business_name: string;
  business_phone?: string;
  business_address?: string;
  business_custom_url?: string;
  /** Venue IANA timezone for reservation_time wall-clock display. */
  business_timezone?: string;
  can_cancel: boolean;
  can_cancel_reason?: "status" | "not_allowed" | "never_open" | "window_closed" | string;
  cancellation_deadline_hours?: number;
}

export const reservationAPI = {
  getSettings: async (businessId: number): Promise<ReservationSettingsDto> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reservations/settings`
    );
    return response.data;
  },

  updateSettings: async (
    businessId: number,
    settings: Partial<ReservationSettingsDto>
  ): Promise<ReservationSettingsDto> => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/reservations/settings`,
      settings
    );
    return response.data;
  },

  getReservations: async (
    businessId: number,
    startDate?: string,
    endDate?: string,
    status?: string,
    options: ReservationListOptions = {},
  ): Promise<ReservationsResponse> => {
    const params = new URLSearchParams();
    if (startDate) params.append("start_date", startDate);
    if (endDate) params.append("end_date", endDate);
    if (status && status !== "all") params.append("status", status);
    const searchQ = options.q?.trim();
    if (searchQ) params.append("q", searchQ);
    if (options.page) params.append("page", String(options.page));
    if (options.pageSize) params.append("page_size", String(options.pageSize));

    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reservations?${params.toString()}`,
      {
        _useCache: options.disableCache === false ? undefined : false,
        ...(options.signal ? { signal: options.signal } : {}),
      } as any,
    );
    return response.data;
  },

  getReservation: async (
    businessId: number,
    reservationId: number
  ): Promise<Reservation> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reservations/${reservationId}`
    );
    return response.data;
  },

  getTableOptions: async (
    businessId: number,
    input: {
      reservationTime: string;
      duration: number;
      partySize: number;
      excludeReservationId?: number;
    },
  ): Promise<ReservationTableOptionsResponse> => {
    const params = new URLSearchParams({
      reservation_time: input.reservationTime,
      duration: String(input.duration),
      party_size: String(input.partySize),
    });
    if (input.excludeReservationId) {
      params.set("exclude_reservation_id", String(input.excludeReservationId));
    }
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reservations/table-options?${params.toString()}`,
      { _useCache: false } as never,
    );
    return response.data;
  },

  createReservation: async (
    businessId: number,
    reservation: CreateReservationRequest
  ): Promise<Reservation> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations`,
      reservation
    );
    return response.data;
  },

  updateReservation: async (
    businessId: number,
    reservationId: number,
    updates: UpdateReservationRequest
  ): Promise<Reservation> => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/reservations/${reservationId}`,
      updates
    );
    return response.data;
  },

  cancelReservation: async (
    businessId: number,
    reservationId: number
  ): Promise<void> => {
    await axiosInstance.delete(
      `/inside/businesses/${businessId}/reservations/${reservationId}`
    );
  },

  assignTable: async (
    businessId: number,
    reservationId: number,
    payload: ReservationTransitionRequest
  ): Promise<Reservation> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations/${reservationId}/assign-table`,
      payload
    );
    return response.data;
  },

  checkIn: async (
    businessId: number,
    reservationId: number,
    payload: ReservationTransitionRequest = {}
  ): Promise<Reservation> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations/${reservationId}/check-in`,
      payload
    );
    return response.data;
  },

  markNoShow: async (
    businessId: number,
    reservationId: number,
    payload: ReservationTransitionRequest
  ): Promise<Reservation> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations/${reservationId}/no-show`,
      payload
    );
    return response.data;
  },

  promoteWaitlist: async (
    businessId: number,
    reservationId: number,
    payload: ReservationTransitionRequest = {}
  ): Promise<Reservation> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations/${reservationId}/promote-waitlist`,
      payload
    );
    return response.data;
  },

  /** L4-8: exclusive operator claim before mutate. Pass steal:true to take over. */
  claimReservation: async (
    businessId: number,
    reservationId: number,
    options?: { steal?: boolean },
  ): Promise<ReservationClaimState> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations/${reservationId}/claim`,
      options?.steal ? { steal: true } : {},
    );
    return response.data;
  },

  /**
   * L4-8: release claim. Pass selfOnly:true for automatic post-mutate release
   * so a manager/owner never force-clears a claim stolen mid-flight.
   * Omit (or selfOnly:false) for explicit force-release by CanSteal actors.
   */
  releaseReservation: async (
    businessId: number,
    reservationId: number,
    options?: { selfOnly?: boolean },
  ): Promise<void> => {
    await axiosInstance.post(
      `/inside/businesses/${businessId}/reservations/${reservationId}/release`,
      options?.selfOnly ? { self_only: true } : {},
    );
  },

  getUpcomingReservations: async (
    businessId: number,
    limit: number = 10,
    signal?: AbortSignal,
  ): Promise<{ reservations: Reservation[]; total: number }> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reservations/upcoming?limit=${limit}`,
      signal ? { signal } : undefined,
    );
    return response.data;
  },

  getStats: async (
    businessId: number,
    startDate?: string,
    endDate?: string,
    signal?: AbortSignal,
  ): Promise<ReservationStats> => {
    const params = new URLSearchParams();
    if (startDate) params.append("start_date", startDate);
    if (endDate) params.append("end_date", endDate);

    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reservations/stats?${params.toString()}`,
      signal ? { signal } : undefined,
    );
    return response.data;
  },
};

export const getAllUpcomingReservations = async (
  businessId: number,
  args: {
    startDate?: string;
    endDate?: string;
    status?: string;
    q?: string;
    pageSize?: number;
    maxPages?: number;
    maxRows?: number;
  } = {},
): Promise<FetchAllPagesResult<Reservation>> =>
  fetchAllPages<Reservation>(
    (page, pageSize) =>
      reservationAPI.getReservations(businessId, args.startDate, args.endDate, args.status, {
        page,
        pageSize,
        q: args.q,
      }) as Promise<ReservationsResponse>,
    (response) => (response as ReservationsResponse).reservations,
    args,
  );

export const guestReservationAPI = {
  getSettings: async (customUrl: string): Promise<ReservationSettingsDto> => {
    const response = await axiosInstance.get(
      `/business/${customUrl}/reservations/settings`
    );
    return response.data;
  },

  createReservation: async (
    customUrl: string,
    reservation: CreateReservationRequest
  ): Promise<GuestReservationCreatedResponse> => {
    const response = await axiosInstance.post(
      `/business/${customUrl}/reservations`,
      reservation
    );
    return response.data;
  },

  getReservation: async (
    confirmationCode: string
  ): Promise<PublicReservationDetailsResponse> => {
    const response = await axiosInstance.get(
      `/reservations/${confirmationCode}`
    );
    return response.data;
  },

  getAvailability: async (
    customUrl: string,
    date: string,
    partySize: number = 2
  ): Promise<ReservationAvailabilityDto> => {
    const response = await axiosInstance.get(
      `/business/${customUrl}/reservations/availability?date=${date}&party_size=${partySize}`
    );
    return response.data;
  },

  cancelReservation: async (
    confirmationCode: string
  ): Promise<ReservationActionResponse> => {
    const response = await axiosInstance.post(
      `/reservations/${confirmationCode}/cancel`
    );
    return response.data;
  },
};

/** Extract a claim-conflict payload from an axios-shaped error (L4-8). */
export function getReservationClaimConflict(
  error: unknown,
): ReservationClaimConflict | null {
  if (typeof error !== "object" || error === null) return null;
  const resp = (error as { response?: { status?: number; data?: unknown } })
    .response;
  if (resp?.status !== 409 || typeof resp.data !== "object" || resp.data === null) {
    return null;
  }
  const data = resp.data as ReservationClaimConflict;
  if (
    data.code === "claim_held" ||
    data.code === "claim_steal_required" ||
    data.code === "claim_forbidden" ||
    typeof data.claimed_by_name === "string"
  ) {
    return data;
  }
  return null;
}

export type ReservationSettings = ReservationSettingsDto;
