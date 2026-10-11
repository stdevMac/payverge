import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

/**
 * Canonical list of vehicle types accepted by the backend.
 * Source of truth: backend/internal/database/delivery_models.go — VehicleType enum.
 * Update both if the enum changes.
 */
export const VEHICLE_TYPES = ["bicycle", "scooter", "motorcycle", "car", "van"] as const;
export type VehicleType = (typeof VEHICLE_TYPES)[number];

const DELIVERY_STATUSES = [
  "pending",
  "confirmed",
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
  "delivered",
  "cancelled",
  "failed",
] as const;
export type DeliveryStatus = (typeof DELIVERY_STATUSES)[number];

const DRIVER_STATUSES = ["offline", "online", "busy", "on_break"] as const;
export type DriverStatus = (typeof DRIVER_STATUSES)[number];

const isDeliveryStatus = (value: string): value is DeliveryStatus =>
  (DELIVERY_STATUSES as readonly string[]).includes(value);

/**
 * Error thrown by guest delivery endpoints. Carries the HTTP status so guest
 * pages can map failures to localized messages (21-locale tier) instead of
 * rendering the backend's English `message` verbatim.
 */
interface DeliveryApiError extends Error {
  status?: number;
}

const getDeliveryErrorStatus = (error: unknown): number | undefined => {
  if (typeof error === "object" && error !== null) {
    const status = (error as { response?: { status?: unknown } }).response?.status;
    if (typeof status === "number") {
      return status;
    }
  }
  return undefined;
};

const getDeliveryErrorMessage = (error: unknown, fallback: string): string => {
  if (typeof error === "object" && error !== null) {
    const maybe = (error as { response?: { data?: { error?: unknown } } }).response
      ?.data?.error;
    if (typeof maybe === "string" && maybe.trim().length > 0) {
      return maybe;
    }
  }
  return fallback;
};

export interface ExternalPartnerLink {
  name: string;
  url: string;
  provider_key?: string;
  icon_url?: string;
}

// Wire money fields are DOLLARS (backend converts from stored cents).
export interface DeliveryZoneDto {
  id: number;
  name: string;
  description?: string;
  delivery_fee: Dollars;
  minimum_order_amount: Dollars;
  estimated_time: number;
  priority: number;
  cutoff_buffer_minutes: number;
  operating_hours?: string;
  boundaries?: Record<string, string[]>;
  is_active: boolean;
  /**
   * L3-38 — client-only stable identity, never sent to the backend.
   *
   * Server ids cannot carry UI identity across a save: a brand-new zone has no
   * id until the PUT returns, and the backend re-sorts the whole list by
   * `priority DESC, estimated_time ASC, id ASC`. Keying the editor off this
   * keeps the right card expanded through the round-trip.
   * `buildDeliverySettingsPayload` strips it from the wire payload.
   */
  _client_key?: string;
}

export interface DeliveryDriver {
  id: number;
  business_id: number;
  staff_id?: number | null;
  name: string;
  phone: string;
  email?: string;
  license_number?: string;
  vehicle_type?: VehicleType;
  vehicle_plate?: string;
  status: DriverStatus;
  is_available: boolean;
  current_delivery_id?: number | null;
  is_active: boolean;
  total_deliveries?: number;
  completed_deliveries?: number;
  average_rating?: number;
}

export interface DriverQueueItem {
  delivery_id: number;
  delivery_number: string;
  status: DeliveryStatus;
  customer_name: string;
  address_short: string;
  assigned_at?: string | null;
  estimated_delivery_time?: string | null;
  delivery_fee: Dollars;
  driver_tip: Dollars;
}

export interface DriverPerformanceDto {
  driver_id: number;
  driver_name: string;
  status: DriverStatus;
  is_active: boolean;
  completed_today: number;
  completed_week: number;
  completed_all_time: number;
  cancelled_count: number;
  failed_count: number;
  in_progress_count: number;
  avg_pickup_minutes: number | null;
  avg_delivery_minutes: number | null;
  on_time_rate: number | null;
  average_rating: number | null;
  gross_fees_collected: Dollars;
  gross_tips_collected: Dollars;
  active_queue: DriverQueueItem[];
}

interface UpdateDeliveryZoneInput {
  id?: number;
  name?: string;
  description?: string;
  delivery_fee?: Dollars;
  minimum_order_amount?: Dollars;
  estimated_time?: number;
  priority?: number;
  cutoff_buffer_minutes?: number;
  operating_hours?: string;
  boundaries?: Record<string, string[]>;
  is_active?: boolean;
}

export interface DeliverySettingsDto {
  business_id: number;
  delivery_enabled: boolean;
  in_house_delivery_enabled: boolean;
  third_party_enabled: boolean;
  /** Payment mode for guest delivery orders. */
  payment_mode: "online" | "cash_on_delivery";
  /** True when the business has a working online-payment setup (Stripe etc.). */
  online_payment_available: boolean;
  flat_delivery_fee: Dollars;
  free_delivery_minimum: Dollars;
  minimum_order_amount: Dollars;
  estimated_prep_time: number;
  max_concurrent_deliveries: number;
  delivery_hours_same_as_business: boolean;
  delivery_start_time?: string;
  delivery_end_time?: string;
  delivery_instructions?: string;
  external_partner_links: ExternalPartnerLink[];
  zones: DeliveryZoneDto[];
  live_order_count?: number;
  partner_fallback_available: boolean;
  estimated_delivery_minutes?: number;
}

export interface UpdateDeliverySettingsInput {
  delivery_enabled?: boolean;
  in_house_delivery_enabled?: boolean;
  third_party_enabled?: boolean;
  payment_mode?: "online" | "cash_on_delivery";
  flat_delivery_fee?: Dollars;
  free_delivery_minimum?: Dollars;
  minimum_order_amount?: Dollars;
  estimated_prep_time?: number;
  max_concurrent_deliveries?: number;
  delivery_hours_same_as_business?: boolean;
  delivery_start_time?: string;
  delivery_end_time?: string;
  delivery_instructions?: string;
  external_partner_links?: ExternalPartnerLink[];
  zones?: UpdateDeliveryZoneInput[];
}

export interface DeliveryQuoteRequest {
  order_subtotal: Dollars;
  delivery_address: {
    street: string;
    apartment?: string;
    city: string;
    state?: string;
    postal_code?: string;
    country: string;
    formatted_address?: string;
  };
}

export interface DeliveryQuoteDto {
  eligible: boolean;
  reason_code: string;
  message: string;
  zone?: DeliveryZoneDto;
  delivery_fee: Dollars;
  minimum_order_amount: Dollars;
  free_delivery_minimum: Dollars;
  order_subtotal: Dollars;
  meets_minimum: boolean;
  estimated_prep_time: number;
  estimated_delivery_minutes: number;
  estimated_total_minutes: number;
  estimated_delivery_time?: string;
  cutoff_at?: string;
  fulfillment_mode: string;
  partner_fallback_available: boolean;
  external_partner_links: ExternalPartnerLink[];
}

export interface DeliveryCheckoutItemInput {
  menu_item_name: string;
  menu_item_id?: string;
  quantity: number;
  price: Dollars;
  options?: Array<{
    id?: string;
    name: string;
    price_change: Dollars;
    is_required?: boolean;
  }>;
  special_requests?: string;
  item_type?: string;
  bundle_id?: number;
  parent_bundle_id?: number;
  source_offer_id?: number;
}

export interface DeliveryCheckoutDto {
  bill: {
    id: number;
    business_id: number;
    bill_number: string;
    subtotal: Dollars;
    tax_amount: Dollars;
    service_fee_amount: Dollars;
    total_amount: Dollars;
    status: string;
  };
  order: {
    id: number;
    bill_id: number;
    order_number: string;
    status: string;
    notes?: string;
  };
  delivery_order: DeliveryOrder;
  tracking_url: string;
  // Wave 4 idempotency: true when the backend replayed an existing order for a
  // retried X-Request-Id (HTTP 200) instead of creating a new one (HTTP 201).
  // The bill/order/delivery_order/tracking_url all reference the ORIGINAL order,
  // so the client treats this exactly like a fresh success — no error path.
  duplicate?: boolean;
}

export interface DeliveryOrder {
  id: number;
  business_id: number;
  bill_id?: number;
  order_id?: number;
  customer_id?: number;
  zone_id?: number | null;
  delivery_number: string;
  delivery_type: string;
  status: DeliveryStatus;
  fulfillment_mode?: string;
  driver_id?: number;
  customer_name: string;
  customer_phone: string;
  customer_email?: string;
  delivery_address: {
    street: string;
    apartment?: string;
    city: string;
    state?: string;
    postal_code?: string;
    country: string;
    formatted_address?: string;
  };
  delivery_fee: Dollars;
  driver_tip?: Dollars;
  /** Bill total in dollars (list/detail projection). */
  total?: Dollars;
  /** Business default currency on list/detail rows. */
  currency?: string;
  pickup_location?: { latitude: number; longitude: number; timestamp?: string };
  dropoff_location?: { latitude: number; longitude: number; timestamp?: string };
  current_location?: { latitude: number; longitude: number; timestamp?: string } | null;
  /**
   * Effective payment mode snapshotted at order creation
   * (backend DeliveryOrder.payment_mode_stored). Empty string = unknown:
   * rows created before the column existed carry "" and must not be
   * treated as a mismatch against the current settings mode.
   */
  payment_mode_stored?: "" | "online" | "cash_on_delivery";
  estimated_delivery_time?: string;
  actual_delivery_time?: string;
  delivery_instructions?: string;
  contactless_delivery: boolean;
  leave_at_door: boolean;
  cutoff_at?: string;
  /** Quote snapshot — may include order_subtotal / item_count for dispatch cards. */
  quote_metadata?: {
    order_subtotal?: number;
    item_count?: number;
    [key: string]: unknown;
  } | null;
  payment_expires_at?: string | null;
  created_at: string;
  updated_at: string;
  zone?: DeliveryZoneDto;
  driver?: DeliveryDriver | null;
  /** Dispatcher claim (one operator at a time). Absent/empty = unclaimed. */
  claimed_by_staff_id?: number | null;
  claimed_by_name?: string;
  claimed_by_role?: string;
  claimed_at?: string | null;
  /** Status audit trail from the projected detail read. */
  status_history?: Array<{
    id: number;
    delivery_order_id: number;
    status: DeliveryStatus;
    notes?: string;
    changed_by?: string;
    created_at: string;
  }>;
}

/** 409 claim conflict payload from dispatch mutators / claim endpoint. */
export interface DeliveryClaimConflict {
  code?: string;
  error?: string;
  claimed_by_name?: string;
  claimed_by_role?: string;
  claimed_by_staff_id?: number | null;
}

export interface PublicDeliveryTrackingDto {
  delivery_number: string;
  status: DeliveryStatus;
  estimated_delivery_time?: string;
  current_location?: {
    latitude: number;
    longitude: number;
    timestamp?: string;
  } | null;
  /**
   * Wire truth (backend TrackDelivery, delivery_handlers.go): the driver
   * summary carries ONLY name + phone (the Driver preload selects id/name/
   * phone and the handler emits `{name, phone}`). The handler currently
   * always sends an object (with empty strings when unassigned), but treat it
   * as absent-able and gate rendering on `driver?.name`.
   */
  driver?: {
    name: string;
    phone: string;
  } | null;
  business_name?: string;
  business_custom_url?: string;
  external_tracking_url?: string;
  updated_at?: string;
  business_id?: number;
  payment_mode?: "online" | "cash_on_delivery";
  awaiting_payment?: boolean;
  payment_expires_at?: string | null;
  cancellation_reason?: string;
  /** Currency fields for PaymentSection on the guest pay page. */
  default_currency?: string;
  display_currency?: string;
  /** Venue IANA timezone for ETA / timestamp wall-clock (not device TZ). */
  timezone?: string;
  /** Configured courier partners (PedidosYa, Rappi, …) for guest copy. */
  external_partner_links?: ExternalPartnerLink[];
  bill?: {
    id: number;
    bill_number: string;
    /** Unguessable guest capability for /guest/bill routes when present. */
    public_token?: string;
    status: string;
    total_amount: Dollars;
    paid_amount?: Dollars;
    paid: boolean;
    /** Crypto settlement address (snapshot stored on bill at creation). */
    settlement_address?: string;
    /** Crypto tipping address (snapshot stored on bill at creation). */
    tipping_address?: string;
  } | null;
}

export interface DeliveryListResult {
  deliveries: DeliveryOrder[];
  total: number;
  has_more: boolean;
}

export const deliveryApi = {
  getDeliverySettings: async (businessId: number): Promise<DeliverySettingsDto> => {
    const response = await axiosInstance.get<DeliverySettingsDto>(
      `/inside/businesses/${businessId}/delivery-settings`
    );
    return response.data;
  },

  updateDeliverySettings: async (
    businessId: number,
    settings: UpdateDeliverySettingsInput
  ): Promise<DeliverySettingsDto> => {
    const response = await axiosInstance.put<DeliverySettingsDto>(
      `/inside/businesses/${businessId}/delivery-settings`,
      settings
    );
    return response.data;
  },

  getBusinessDeliveries: async (
    businessId: number,
    options?: {
      /** Single status or multi-status window (comma-joined on the wire). */
      status?: DeliveryStatus | DeliveryStatus[];
      limit?: number;
      offset?: number;
      since?: string;
      signal?: AbortSignal;
    }
  ): Promise<DeliveryListResult> => {
    const params: Record<string, string | number> = {};
    if (options?.status) {
      params.status = Array.isArray(options.status)
        ? options.status.join(",")
        : options.status;
    }
    if (options?.limit !== undefined) params.limit = options.limit;
    if (options?.offset !== undefined) params.offset = options.offset;
    if (options?.since) params.since = options.since;
    const response = await axiosInstance.get<DeliveryListResult>(
      `/inside/businesses/${businessId}/deliveries`,
      { params, signal: options?.signal }
    );
    return response.data;
  },

  getDeliveryOrder: async (
    businessId: number,
    deliveryId: number
  ): Promise<DeliveryOrder> => {
    const response = await axiosInstance.get<DeliveryOrder>(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}`
    );
    return response.data;
  },

  updateDeliveryOrderStatus: async (
    businessId: number,
    deliveryId: number,
    status: DeliveryStatus
  ): Promise<void> => {
    if (!isDeliveryStatus(status)) {
      throw new Error(`Invalid delivery status: ${status}`);
    }
    await axiosInstance.put(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}/status`,
      { status }
    );
  },

  /**
   * Scoped operator edit of contact/address/instructions/ETA on a non-terminal
   * delivery. Backend rejects terminal orders with 409.
   */
  patchDeliveryOrder: async (
    businessId: number,
    deliveryId: number,
    patch: {
      customer_name?: string;
      customer_phone?: string;
      customer_email?: string;
      street?: string;
      apartment?: string;
      city?: string;
      state?: string;
      postal_code?: string;
      country?: string;
      formatted_address?: string;
      delivery_instructions?: string;
      contactless_delivery?: boolean;
      leave_at_door?: boolean;
      estimated_delivery_time?: string;
    }
  ): Promise<DeliveryOrder> => {
    const response = await axiosInstance.patch<DeliveryOrder>(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}`,
      patch
    );
    return response.data;
  },

  assignDriver: async (
    businessId: number,
    deliveryId: number,
    driverId: number
  ): Promise<void> => {
    await axiosInstance.post(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}/assign`,
      { driver_id: driverId }
    );
  },

  getBusinessDrivers: async (businessId: number): Promise<DeliveryDriver[]> => {
    const response = await axiosInstance.get<DeliveryDriver[]>(
      `/inside/businesses/${businessId}/drivers`
    );
    return response.data;
  },

  getAvailableDrivers: async (
    businessId: number,
    options?: { signal?: AbortSignal },
  ): Promise<DeliveryDriver[]> => {
    const response = await axiosInstance.get<DeliveryDriver[]>(
      `/inside/businesses/${businessId}/drivers/available`,
      options?.signal ? { signal: options.signal } : undefined,
    );
    return response.data;
  },

  createDriver: async (
    businessId: number,
    payload: {
      name: string;
      phone: string;
      email?: string;
      vehicle_type?: VehicleType;
      vehicle_plate?: string;
      license_number?: string;
    }
  ): Promise<DeliveryDriver> => {
    const response = await axiosInstance.post<DeliveryDriver>(
      `/inside/businesses/${businessId}/drivers`,
      payload
    );
    return response.data;
  },

  updateDriver: async (
    businessId: number,
    driverId: number,
    payload: Partial<{
      name: string;
      phone: string;
      email: string;
      vehicle_type: VehicleType;
      vehicle_plate: string;
      license_number: string;
      is_available: boolean;
      is_active: boolean;
    }>
  ): Promise<DeliveryDriver> => {
    const response = await axiosInstance.put<DeliveryDriver>(
      `/inside/businesses/${businessId}/drivers/${driverId}`,
      payload
    );
    return response.data;
  },

  deleteDriver: async (businessId: number, driverId: number): Promise<void> => {
    await axiosInstance.delete(
      `/inside/businesses/${businessId}/drivers/${driverId}`
    );
  },

  listDriverPerformance: async (
    businessId: number
  ): Promise<{ drivers: DriverPerformanceDto[] }> => {
    const response = await axiosInstance.get<{ drivers: DriverPerformanceDto[] }>(
      `/inside/businesses/${businessId}/drivers/performance`
    );
    return response.data;
  },

  getDriverPerformance: async (
    businessId: number,
    driverId: number
  ): Promise<DriverPerformanceDto> => {
    const response = await axiosInstance.get<DriverPerformanceDto>(
      `/inside/businesses/${businessId}/drivers/${driverId}/performance`
    );
    return response.data;
  },

  cancelDeliveryOrder: async (
    businessId: number,
    deliveryId: number,
    reason: string
  ): Promise<void> => {
    await axiosInstance.post(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}/cancel`,
      { reason }
    );
  },

  /**
   * Claim a delivery for exclusive dispatch. Pass steal:true to take over a
   * fresh claim held by someone else (managers/owners only; audited + notifies).
   */
  claimDeliveryOrder: async (
    businessId: number,
    deliveryId: number,
    options?: { steal?: boolean }
  ): Promise<{
    claimed_by_staff_id?: number | null;
    claimed_by_name?: string;
    claimed_by_role?: string;
    claimed_at?: string;
  }> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}/claim`,
      options?.steal ? { steal: true } : {}
    );
    return response.data;
  },

  releaseDeliveryOrder: async (
    businessId: number,
    deliveryId: number
  ): Promise<void> => {
    await axiosInstance.post(
      `/inside/businesses/${businessId}/deliveries/${deliveryId}/release`
    );
  },
};

/** Extract a claim-conflict payload from an axios-shaped error. */
export function getDeliveryClaimConflict(
  error: unknown
): DeliveryClaimConflict | null {
  if (typeof error !== "object" || error === null) return null;
  const resp = (error as { response?: { status?: number; data?: unknown } })
    .response;
  if (resp?.status !== 409 || typeof resp.data !== "object" || resp.data === null) {
    return null;
  }
  const data = resp.data as DeliveryClaimConflict;
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

export const guestDeliveryApi = {
  getSettings: async (businessId: number): Promise<DeliverySettingsDto> => {
    const response = await axiosInstance.get<DeliverySettingsDto>(
      `/businesses/${businessId}/delivery-settings`
    );
    return response.data;
  },

  quote: async (
    businessId: number,
    payload: DeliveryQuoteRequest
  ): Promise<DeliveryQuoteDto> => {
    try {
      const response = await axiosInstance.post<DeliveryQuoteDto>(
        `/businesses/${businessId}/delivery/quote`,
        payload
      );
      return response.data;
    } catch (error) {
      throw new Error(getDeliveryErrorMessage(error,"Failed to check delivery availability"));
    }
  },

  checkout: async (
    businessId: number,
    payload: {
      customer_name: string;
      customer_phone: string;
      customer_email: string;
      customer_locale?: string;
      delivery_address: DeliveryQuoteRequest["delivery_address"];
      delivery_instructions?: string;
      contactless_delivery?: boolean;
      leave_at_door?: boolean;
      notes?: string;
      driver_tip?: number;
      items: DeliveryCheckoutItemInput[];
    },
    options?: { idempotencyKey?: string }
  ): Promise<DeliveryCheckoutDto> => {
    try {
      const response = await axiosInstance.post<DeliveryCheckoutDto>(
        `/businesses/${businessId}/delivery/orders`,
        payload,
        options?.idempotencyKey
          ? { headers: { "X-Request-Id": options.idempotencyKey } }
          : undefined
      );
      return response.data;
    } catch (error) {
      throw new Error(getDeliveryErrorMessage(error,"Failed to complete delivery checkout"));
    }
  },

  track: async (deliveryNumber: string): Promise<PublicDeliveryTrackingDto> => {
    try {
      const response = await axiosInstance.get<PublicDeliveryTrackingDto>(
        `/delivery/${deliveryNumber}/track`
      );
      return response.data;
    } catch (error) {
      const wrapped: DeliveryApiError = new Error(
        getDeliveryErrorMessage(error, "Failed to load delivery tracking"),
      );
      wrapped.status = getDeliveryErrorStatus(error);
      throw wrapped;
    }
  },
};

// Standalone re-exports removed — all consumers use deliveryApi.* directly.
