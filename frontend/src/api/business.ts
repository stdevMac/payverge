import { axiosInstance } from "./tools/instance";
import type { Orderability } from "./orders";
import {
  validateBusiness,
  validateBusinessList,
} from "./schemas/business";
import type { Cents, Dollars } from "@/types/money";
import type {
  HeroLayout,
  SectionDensity,
} from "@/components/business-page/designClasses";

// Business API utilities
export interface BusinessAddress {
  street: string;
  city: string;
  state: string;
  postal_code: string;
  country: string;
}

export interface Business {
  id: number;
  business_id?: string; // New string-based business identifier
  /**
   * Owning account id. Emitted on owner/admin reads of a business. The hub uses
   * it (with demo_owner_user_id) to tell venues the viewer owns apart from demo
   * venues a platform admin merely gets to see (#832).
   */
  user_id?: number;
  /** True for generated demo venues (backend `is_demo`). */
  is_demo?: boolean;
  /** Admin account a generated demo venue was scoped to. */
  demo_owner_user_id?: number;
  owner_address: string;
  owner_name?: string;
  name: string;
  logo: string;
  address: BusinessAddress;
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
  // P1-12: present only on staff UpdateBusiness responses when owner-only
  // fields were submitted and dropped (backend stripOwnerOnlyFieldsForStaff).
  skipped_fields?: string[];
  is_active: boolean;
  /** Set when a server administrator closed the business (read-only). */
  closed_at?: string | null;
  // New fields for enhanced business features
  description?: string;
  custom_url?: string;
  phone?: string;
  website?: string;
  social_media?: string;
  banner_images?: string;
  business_page_enabled?: boolean;
  show_reviews?: boolean;
  google_reviews_enabled?: boolean;
  // Google Business Integration fields
  google_place_id?: string;
  google_business_name?: string;
  google_review_link?: string;
  google_business_url?: string;
  // Trustpilot Integration fields
  trustpilot_enabled?: boolean;
  trustpilot_review_url?: string;
  // Currency settings
  default_currency?: string;
  display_currency?: string;
  // Guest language: which language the menu is authored in. Used by the
  // /t/<code>/menu page to skip the translated-menu fetch when the saved
  // guest language matches the default.
  default_language?: string;
  // Hospitality features
  welcome_message?: string;
  about_story?: string;
  show_welcome_message?: boolean;
  show_about_story?: boolean;
  show_gallery?: boolean;
  show_operating_hours?: boolean;
  show_special_features?: boolean;
  // AI Waiter settings
  ai_settings?: {
    ai_enabled: boolean;
    ai_name: string;
    ai_priority: string;
    special_instructions?: string;
    business_page_ai_enabled?: boolean;
  };
  // Authoritative guest AI gate (operational business + AI toggle),
  // computed server-side and surfaced on the public guest response.
  ai_available?: boolean;
  // "llm" when a model is configured; "basic" for set replies from the menu.
  ai_waiter_mode?: "llm" | "basic";
  // Kitchen and Orders feature toggle
  kitchen_enabled?: boolean;
  orders_enabled?: boolean;
  // CRM feature toggle
  crm_enabled?: boolean;
  // Delivery settings
  delivery_enabled?: boolean;
  // L3-39: delivery_radius removed — no operator UI, no enforcement, and it is
  // no longer on the v1 delivery settings contract.
  delivery_fee_type?: "flat" | "distance_based" | "free_above_minimum";
  flat_delivery_fee?: Dollars;
  distance_based_rate?: Dollars;
  free_delivery_minimum?: Dollars;
  minimum_order_amount?: Dollars;
  estimated_prep_time?: number;
  max_concurrent_deliveries?: number;
  delivery_hours_same_as_business?: boolean;
  delivery_start_time?: string;
  delivery_end_time?: string;
  delivery_zones?: string;
  delivery_instructions?: string;
  design_settings?: BusinessDesignSettings;
  timezone?: string; // IANA timezone identifier (e.g., "Asia/Dubai")
  /** Minutes after local midnight when the venue service day begins (0–1439). */
  service_day_start_minute?: number;
  business_type?: string; // restaurant, cafe, bar, quick_service, food_truck, bakery, fine_dining, other
  // Default QR Code Customization — used by the Tables drawer to render
  // the per-table preview when the row's own qr_* override isn't set.
  default_qr_logo_url?: string;
  default_qr_foreground_color?: string;
  default_qr_background_color?: string;
  default_qr_logo_size?: number;
  default_qr_show_business_name?: boolean;
  default_qr_show_table_name?: boolean;
  default_qr_text_font?: string;
  // Public landing surface (guest /t/{code}) can include condensed weekly
  // hours so the open/closed pill can render without a separate fetch.
  hours?: {
    day_of_week: number;
    open_time: string;
    close_time: string;
    is_closed?: boolean;
  }[];
  created_at: string;
  updated_at: string;
  /** ISO timestamp set when the operator finishes onboarding. Null while incomplete. */
  onboarding_completed_at?: string | null;
}

export interface CreateBusinessRequest {
  owner_name?: string;
  name: string;
  logo?: string;
  address: BusinessAddress;
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
  business_type?: string;
  description?: string;
  custom_url?: string;
  phone?: string;
  email?: string;
  website?: string;
  social_media?: string;
  banner_images?: string;
  business_page_enabled?: boolean;
  show_reviews?: boolean;
  google_reviews_enabled?: boolean;
  counter_enabled?: boolean;
  counter_count?: number;
  counter_prefix?: string;
  default_currency?: string;
  display_currency?: string;
  default_language?: string;
  source_language?: string;
  timezone?: string;
}

export interface CreateBusinessOptions {
  idempotencyKey: string;
}

export interface UpdateBusinessRequest {
  name?: string;
  logo?: string;
  address?: BusinessAddress;
  settlement_address?: string;
  tipping_address?: string;
  tax_rate?: number;
  service_fee_rate?: number;
  tax_inclusive?: boolean;
  service_inclusive?: boolean;
  // New fields for enhanced business features
  description?: string;
  custom_url?: string;
  phone?: string;
  website?: string;
  social_media?: string;
  banner_images?: string;
  business_page_enabled?: boolean;
  show_reviews?: boolean;
  google_reviews_enabled?: boolean;
  design_settings?: BusinessDesignSettings;
  timezone?: string; // IANA timezone identifier
  /** Minutes after local midnight when the venue service day begins (0–1439). */
  service_day_start_minute?: number;
  business_type?: string;
  // Currency settings
  default_currency?: string;
  display_currency?: string;
  // Guest language: which language the menu is authored in. Used by the
  // /t/<code>/menu page to skip the translated-menu fetch when the saved
  // guest language matches the default.
  default_language?: string;
  // Hospitality features
  welcome_message?: string;
  about_story?: string;
  show_welcome_message?: boolean;
  show_about_story?: boolean;
  show_gallery?: boolean;
  show_operating_hours?: boolean;
  show_special_features?: boolean;
  // Default QR Code Customization
  default_qr_logo_url?: string;
  default_qr_foreground_color?: string;
  default_qr_background_color?: string;
  default_qr_logo_size?: number;
  default_qr_show_business_name?: boolean;
  default_qr_show_table_name?: boolean;
  default_qr_text_font?: string;
}

const deliverySettingsKeys = new Set([
  "delivery_enabled",
  "delivery_fee_type",
  "flat_delivery_fee",
  "distance_based_rate",
  "free_delivery_minimum",
  "minimum_order_amount",
  "estimated_prep_time",
  "max_concurrent_deliveries",
  "delivery_hours_same_as_business",
  "delivery_start_time",
  "delivery_end_time",
  "delivery_zones",
  "delivery_instructions",
]);

function toBusinessUpdatePayload(
  businessData: UpdateBusinessRequest,
): UpdateBusinessRequest {
  return Object.fromEntries(
    Object.entries(businessData as Record<string, unknown>).filter(
      ([key]) => !deliverySettingsKeys.has(key),
    ),
  ) as UpdateBusinessRequest;
}

// Generate business ID response type
interface GenerateBusinessIdResponse {
  business_id: string;
}

// Generate a unique business ID for registration
const generateBusinessId = async (name: string): Promise<string> => {
  const response = await axiosInstance.post<GenerateBusinessIdResponse>(
    "/inside/businesses/generate-id",
    {
      name,
    },
  );
  return response.data.business_id;
};

// Create a new business
export const createBusiness = async (
  businessData: CreateBusinessRequest,
  options?: CreateBusinessOptions,
): Promise<Business> => {
  const response = options
    ? await axiosInstance.post<Business>("/inside/businesses", businessData, {
        headers: { "Idempotency-Key": options.idempotencyKey },
      })
    : await axiosInstance.post<Business>("/inside/businesses", businessData);
  return validateBusiness(response.data);
};

// Get all businesses owned by the authenticated user
export const getMyBusinesses = async (): Promise<Business[]> => {
  const response = await axiosInstance.get<Business[]>("/inside/businesses");
  return validateBusinessList(response.data);
};

// Get a specific business by ID or businessId
export const getBusiness = async (
  businessId: string | number,
): Promise<Business> => {
  const response = await axiosInstance.get<Business>(
    `/inside/businesses/${businessId}`,
  );
  return validateBusiness(response.data);
};

// Update a business
export const updateBusiness = async (
  businessId: number,
  businessData: UpdateBusinessRequest,
): Promise<Business> => {
  const response = await axiosInstance.put<Business>(
    `/inside/businesses/${businessId}`,
    toBusinessUpdatePayload(businessData),
  );
  return validateBusiness(response.data);
};

// Delete a business
const deleteBusiness = async (businessId: number): Promise<void> => {
  await axiosInstance.delete(`/inside/businesses/${businessId}`);
};

// Check custom URL availability
export const checkCustomURLAvailability = async (
  url: string,
  excludeBusinessId?: number,
): Promise<{ available: boolean; error?: string }> => {
  const params = new URLSearchParams({ url });
  if (excludeBusinessId) {
    params.append("exclude_business_id", excludeBusinessId.toString());
  }

  const response = await axiosInstance.get<{
    available: boolean;
    error?: string;
  }>(`/inside/businesses/check-url?${params.toString()}`);
  return response.data;
};

// Menu Management Types. Menu prices are stored as float64 dollars in the
// DB and emitted directly (see internal/database/models.go).
export interface MenuItemOption {
  id: string;
  name: string;
  price_change: Dollars;
  is_required: boolean;
}

export interface MenuItem {
  id?: string;
  name: string;
  description: string;
  price: Dollars;
  /**
   * Optional plate-level food cost in dollars (same wire unit as price).
   * Food-cost / menu engineering use this when the item has no inventory
   * recipe — operators can enter COGS without building a full recipe first.
   */
  cogs?: Dollars;
  currency?: string;
  image?: string; // Cover photo
  images?: string[]; // Gallery photos
  options?: MenuItemOption[];
  allergens?: string[];
  dietary_tags?: string[];
  /**
   * Effective sellability on operator reads: false when the operator switched
   * the dish off OR inventory blocks it. Same answer the guest menu gives, so
   * the two surfaces cannot disagree about a dish (#727).
   */
  is_available: boolean;
  /**
   * The STORED manual 86 flag, present on operator menu reads only. Anything
   * that edits availability (the edit form, one-tap 86, Kitchen's 86 control)
   * must read this, not is_available — otherwise saving an inventory-blocked
   * dish pins a manual 86 that outlives the restock (#727).
   */
  manual_available?: boolean;
  sort_order?: number;
  orderability_state?: import("./orders").OrderabilityState;
  /** Serve-time 86 stamp. Survives guest hours overwriting inventory_out. */
  inventory_status?: string;
}

export interface MenuCategory {
  id?: string;
  name: string;
  description: string;
  items: MenuItem[];
  sort_order?: number;
}

export interface MenuSanitizeDecision {
  category_index: number;
  category_name: string;
  item_index: number;
  item_name: string;
  field: "allergens" | "dietary_tags" | "price" | string;
  value: string;
  canonical_value?: string;
  reason:
    | "canonical_value"
    | "alias_normalized"
    | "unknown_enum_value"
    | "price_out_of_range"
    | string;
}

export interface MenuSanitizeReport {
  dropped_allergens: number;
  dropped_dietary_tags: number;
  dropped_items: number;
  retained: MenuSanitizeDecision[];
  dropped: MenuSanitizeDecision[];
}

export interface MenuImportResponse {
  message: string;
  categories?: number;
  version?: number;
  requires_confirmation: boolean;
  sanitized_categories?: MenuCategory[];
  sanitization: MenuSanitizeReport;
  dropped_allergens?: number;
  dropped_dietary_tags?: number;
  dropped_items?: number;
}

export interface Offer {
  id?: number;
  business_id?: number;
  name: string;
  description: string;
  image?: string;
  code?: string;
  discount_type: "percentage" | "fixed";
  discount_value: number;
  start_date?: string;
  end_date?: string;
  weekday_mask?: number;
  start_minute?: number;
  end_minute?: number;
  is_active: boolean;
  applicable_to?: "all" | "category" | "item" | "bundle";
  target_id?: string;
  created_at?: string;
  updated_at?: string;
  /** Server-computed (#835): item-targeted offer whose target dish is
   *  currently not sellable (inventory 86 or manual 86). Guest surfaces
   *  suppress the offer; the operator list shows a warning chip instead. */
  inventory_blocked?: boolean;
  /**
   * The STORED is_active column, present on operator offer reads only.
   * `is_active` above is the EFFECTIVE answer — false while inventory blocks
   * the target dish — so anything that EDITS the switch must read this, or
   * saving a blocked offer would turn it off for good and it would stay off
   * after the restock (#835).
   */
  manual_active?: boolean;
}

export interface BundleItemRef {
  menu_item_id: string;
  name?: string;
  quantity: number;
}

export interface Bundle {
  id?: number;
  business_id?: number;
  name: string;
  description: string;
  price: Dollars;
  currency?: string;
  image?: string;
  items: BundleItemRef[] | string;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface Menu {
  id: number;
  business_id: number;
  // Raw categories: present only on the guest/empty-menu shapes and legacy
  // callers. The authenticated operator GET /menu response (§3.7 wire-contract
  // slim) no longer ships this redundant copy — it emits parsed_categories
  // only. parseMenuCategories() prefers parsed_categories and tolerates absence.
  categories?: MenuCategory[] | string; // Can be array or JSON string
  parsed_categories?: MenuCategory[]; // Translated categories when language is specified
  is_active: boolean;
  version: number;
  created_at: string;
  updated_at: string;
  item_orderability?: Record<string, Orderability>;
}

// Phase 2: Enhanced Menu Management API Functions

// Get menu for a business with optional language
export const getMenu = async (
  businessId: number,
  language?: string,
): Promise<Menu & { language?: string }> => {
  const params = language ? { language } : {};
  const response = await axiosInstance.get<Menu & { language?: string }>(
    `/inside/businesses/${businessId}/menu`,
    { params },
  );
  return response.data;
};

// Granular reorder (§3.7 fix 4): move a single category (scope "category") or a
// single item within a category (scope "item", category_id required) under
// optimistic CAS. The server applies the one move to the stored tree, so a drag
// sends only {scope, from, to, version} instead of re-uploading the whole menu.
export const reorderMenu = async (
  businessId: number,
  params: {
    scope: "category" | "item";
    category_id?: string;
    from: number;
    to: number;
    version: number;
  },
): Promise<{ version?: number }> => {
  const response = await axiosInstance.post<{ version?: number }>(
    `/inside/businesses/${businessId}/menu/reorder`,
    params,
  );
  return response.data ?? {};
};

// Response type for menu mutation endpoints. Since §3.7 fix 3 the add/update
// endpoints also echo the mutated entity (with its server-assigned ID) so the
// client can patch state in place instead of re-downloading the whole menu.
export interface MenuMutationResponse {
  message: string;
  version?: number;
  category?: MenuCategory;
  item?: MenuItem;
  requires_confirmation?: boolean;
  sanitization?: MenuSanitizeReport;
}

// Add a new category to menu
export const addMenuCategory = async (
  businessId: number,
  category: MenuCategory,
  version?: number,
  confirmSanitization = false,
): Promise<MenuMutationResponse> => {
  const response = await axiosInstance.post<MenuMutationResponse>(
    `/inside/businesses/${businessId}/menu/categories`,
    {
      ...category,
      version,
      confirm_sanitization: confirmSanitization,
    },
  );
  return response.data;
};

// Update a menu category (supports both index-based and ID-based)
export const updateMenuCategory = async (
  businessId: number,
  categoryIndex: number,
  category: MenuCategory,
  version?: number,
  categoryId?: string,
  confirmSanitization = false,
): Promise<MenuMutationResponse> => {
  // Use ID-based route if categoryId and version are provided
  if (categoryId && version !== undefined) {
    const response = await axiosInstance.put<MenuMutationResponse>(
      `/inside/businesses/${businessId}/menu/category/${categoryId}`,
      { ...category, version, confirm_sanitization: confirmSanitization },
    );
    return response.data;
  }
  // Legacy index-based route
  const response = await axiosInstance.put<MenuMutationResponse>(
    `/inside/businesses/${businessId}/menu/categories/${categoryIndex}`,
    { ...category, version, confirm_sanitization: confirmSanitization },
  );
  return response.data;
};

// Delete a menu category (supports both index-based and ID-based)
export const deleteMenuCategory = async (
  businessId: number,
  categoryIndex: number,
  version?: number,
  categoryId?: string,
): Promise<MenuMutationResponse> => {
  // Use ID-based route if categoryId and version are provided
  if (categoryId && version !== undefined) {
    const response = await axiosInstance.delete<MenuMutationResponse>(
      `/inside/businesses/${businessId}/menu/category/${categoryId}?version=${version}`,
    );
    return response.data;
  }
  // Legacy index-based route
  const response = await axiosInstance.delete<MenuMutationResponse>(
    `/inside/businesses/${businessId}/menu/categories/${categoryIndex}`,
  );
  return response.data;
};

// Add a menu item to a category (supports both index-based and ID-based)
export const addMenuItem = async (
  businessId: number,
  categoryIndex: number,
  item: MenuItem,
  version?: number,
  categoryId?: string,
  confirmSanitization = false,
): Promise<MenuMutationResponse> => {
  // Transform the item to match backend expectations
  const backendItem = {
    id: item.id || "", // Backend will generate ID if empty
    name: item.name,
    description: item.description,
    price: item.price,
    // L3-4: plate-level food cost must reach the backend (both add + update).
    cogs: item.cogs,
    currency: item.currency || "USD",
    image: item.image || "",
    images: item.images || [],
    options: item.options || [],
    allergens: item.allergens || [],
    dietary_tags: item.dietary_tags || [],
    is_available: item.is_available,
    sort_order: item.sort_order || 0,
  };

  const response = await axiosInstance.post<MenuMutationResponse>(
    `/inside/businesses/${businessId}/menu/items`,
    {
      category_index: categoryIndex,
      category_id: categoryId || "",
      item: backendItem,
      version,
      confirm_sanitization: confirmSanitization,
    },
  );
  return response.data;
};

// Update a menu item (supports both index-based and ID-based)
export const updateMenuItem = async (
  businessId: number,
  categoryIndex: number,
  itemIndex: number,
  item: MenuItem,
  version?: number,
  categoryId?: string,
  itemId?: string,
  confirmSanitization = false,
): Promise<MenuMutationResponse> => {
  // Transform the item to match backend expectations
  const backendItem = {
    id: item.id || "", // Backend will handle ID
    name: item.name,
    description: item.description,
    price: item.price,
    // L3-4: plate-level food cost must reach the backend (both add + update).
    cogs: item.cogs,
    currency: item.currency || "USD",
    image: item.image || "",
    images: item.images || [],
    options: item.options || [],
    allergens: item.allergens || [],
    dietary_tags: item.dietary_tags || [],
    is_available: item.is_available,
    sort_order: item.sort_order || 0,
  };

  const response = await axiosInstance.put<MenuMutationResponse>(
    `/inside/businesses/${businessId}/menu/items`,
    {
      category_index: categoryIndex,
      item_index: itemIndex,
      category_id: categoryId || "",
      item_id: itemId || "",
      item: backendItem,
      version,
      confirm_sanitization: confirmSanitization,
    },
  );
  return response.data;
};

// Delete a menu item (supports both index-based and ID-based)
export const deleteMenuItem = async (
  businessId: number,
  categoryIndex: number,
  itemIndex: number,
  version?: number,
  categoryId?: string,
  itemId?: string,
): Promise<MenuMutationResponse> => {
  // Use ID-based route if IDs and version are provided
  if (categoryId && itemId && version !== undefined) {
    const response = await axiosInstance.delete<MenuMutationResponse>(
      `/inside/businesses/${businessId}/menu/category/${categoryId}/item/${itemId}?version=${version}`,
    );
    return response.data;
  }
  // Legacy index-based route
  const response = await axiosInstance.delete<MenuMutationResponse>(
    `/inside/businesses/${businessId}/menu/categories/${categoryIndex}/items/${itemIndex}`,
  );
  return response.data;
};

// Table Management Types
export interface Table {
  id: number;
  business_id: number;
  name: string;
  table_code: string;
  capacity: number;
  qr_code: string;
  qr_url?: string;
  is_active: boolean;
  /** Present when the tables payload includes layout placement. */
  space_id?: number | null;
  qr_logo_url?: string;
  qr_foreground_color?: string;
  qr_background_color?: string;
  qr_logo_size?: number;
  qr_show_business_name?: boolean;
  qr_show_table_name?: boolean;
  qr_text_font?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTableRequest {
  name: string;
  capacity?: number;
}

export interface UpdateTableRequest {
  name?: string;
  capacity?: number;
  is_active?: boolean;
  qr_logo_url?: string;
  qr_foreground_color?: string;
  qr_background_color?: string;
  qr_logo_size?: number;
  qr_show_business_name?: boolean;
  qr_show_table_name?: boolean;
  qr_text_font?: string;
}

type TableBillStatus = "open" | "partial" | "paid" | "closed" | "voided";

interface BillSummary {
  id: number;
  business_id?: number;
  table_id?: number;
  bill_number: string;
  total_amount: Dollars;
  paid_amount: Dollars;
  tip_amount?: Dollars;
  currency?: string;
  status: TableBillStatus;
  created_by_staff_id?: number | null;
  created_at: string;
  updated_at: string;
}

type ReservationSummaryStatus =
  | "pending"
  | "confirmed"
  | "waitlist"
  | "seated"
  | "completed"
  | "cancelled"
  | "no_show";

interface ReservationSummary {
  id: number;
  business_id?: number;
  table_id?: number | null;
  customer_name: string;
  customer_phone?: string;
  customer_email?: string;
  party_size: number;
  reservation_time: string;
  duration?: number;
  status: ReservationSummaryStatus;
  confirmation_code?: string;
  created_at?: string;
  updated_at?: string;
}

// Table with status information
export interface TableWithStatus {
  table: Table;
  status: "available" | "occupied" | "reserved";
  active_bills: BillSummary[];
  active_bills_count: number;
  /** Physical units to prepare across active bills, excluding grouping and discount rows. */
  active_bill_physical_item_quantity: number;
  /** Staff name who opened the active bill (host Live View "server" column). */
  active_bill_server_name?: string;
  /** Earliest usable seated/open instant (open check or occupying unfinished bill). */
  seated_at?: string | null;
  /** Last activity on the occupying check, when known. */
  last_seen?: string | null;
  reservations: ReservationSummary[];
  reservations_count: number;
}

// Phase 2: Enhanced Table Management API Functions

// Get all tables for a business
export const getBusinessTables = async (
  businessId: number,
  signal?: AbortSignal,
): Promise<{ tables: Table[] }> => {
  const response = await axiosInstance.get<{ tables: Table[] }>(
    `/inside/businesses/${businessId}/tables`,
    signal ? { signal } : undefined,
  );
  return response.data;
};

// Get tables with status information (occupied, available, reserved).
// includeInactive surfaces soft-deleted tables so the operator can reactivate
// them (BE-first: omitted → legacy active-only board).
export const getTablesWithStatus = async (
  businessId: number,
  includeInactive = false,
): Promise<{ tables: TableWithStatus[] }> => {
  const query = includeInactive ? "?include_inactive=true" : "";
  const response = await axiosInstance.get<{ tables: TableWithStatus[] }>(
    `/inside/businesses/${businessId}/tables/status${query}`,
  );
  const tables = (response.data?.tables ?? []).map((row) => ({
    ...row,
    active_bills: row.active_bills ?? [],
    reservations: row.reservations ?? [],
  }));
  return { ...response.data, tables };
};

// Create a new table with QR code
export const createTableWithQR = async (
  businessId: number,
  tableData: CreateTableRequest,
): Promise<Table> => {
  const response = await axiosInstance.post<Table>(
    `/inside/businesses/${businessId}/tables`,
    tableData,
  );
  return response.data;
};

// Update table details
export const updateTableDetails = async (
  tableId: number,
  tableData: UpdateTableRequest,
): Promise<Table> => {
  const response = await axiosInstance.put<Table>(
    `/inside/tables/${tableId}`,
    tableData,
  );
  return response.data;
};

// Update business table (with business context)
export const updateBusinessTable = async (
  businessId: number,
  tableId: number,
  tableData: UpdateTableRequest,
): Promise<Table> => {
  const response = await axiosInstance.put<Table>(
    `/inside/businesses/${businessId}/tables/${tableId}`,
    tableData,
  );
  return response.data;
};

// Delete a table (soft delete)
export const deleteTable = async (tableId: number): Promise<void> => {
  await axiosInstance.delete(`/inside/tables/${tableId}`);
};

export interface QRBrandingPayload {
  qr_logo_url: string;
  qr_foreground_color: string;
  qr_background_color: string;
  qr_logo_size: number;
  qr_show_business_name: boolean;
  qr_show_table_name: boolean;
  qr_text_font: string;
}

// Apply one QR branding set to every table + the business defaults in a single
// transactional request (replaces the per-table PUT fan-out). Returns the
// number of tables updated.
export const applyQrBrandingToAllTables = async (
  businessId: number,
  branding: QRBrandingPayload,
): Promise<number> => {
  const response = await axiosInstance.post<{ updated: number }>(
    `/inside/businesses/${businessId}/tables/qr-branding`,
    branding,
  );
  return response.data.updated;
};

// Design Settings interface
export interface BusinessDesignSettings {
  primary_color: string;
  secondary_color: string;
  font_family: string;
  theme: string;
  menu_layout: string;
  show_images: boolean;
  show_descriptions: boolean;
  header_style?: string;
  corner_radius?: string;
  shadow_intensity?: string;
  background_pattern?: string;
  pattern_opacity?: number;
  hero_layout?: HeroLayout;
  section_density?: SectionDensity;
}

// Operating Hours interface
export interface BusinessOperatingHours {
  id: number;
  business_id: number;
  day_of_week: number; // 0=Sunday, 1=Monday, etc.
  open_time: string;
  close_time: string;
  /** Optional kitchen / last-seating close ("22:00"); dining ends here. */
  kitchen_close_time?: string | null;
  is_closed: boolean;
  created_at: string;
  updated_at: string;
}

export interface BusinessOperatingException {
  id: number;
  business_id: number;
  exception_date: string; // YYYY-MM-DD
  open_time?: string | null;
  close_time?: string | null;
  kitchen_close_time?: string | null;
  is_closed: boolean;
  label: string;
  created_at: string;
  updated_at: string;
}

// Special Features interface
export interface BusinessSpecialFeature {
  id: number;
  business_id: number;
  title: string;
  description: string;
  icon: string;
  display_order: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// Update business design settings
export const updateBusinessDesignSettings = async (
  businessId: number,
  designSettings: BusinessDesignSettings,
): Promise<void> => {
  await axiosInstance.put(
    `/inside/businesses/${businessId}/design-settings`,
    designSettings,
  );
};

// Get business operating hours
export const getBusinessOperatingHours = async (
  businessId: number,
): Promise<BusinessOperatingHours[]> => {
  const response = await axiosInstance.get<BusinessOperatingHours[]>(
    `/inside/businesses/${businessId}/operating-hours`,
  );
  return response.data;
};

// Update business operating hours
export const updateBusinessOperatingHours = async (
  businessId: number,
  hours: Omit<
    BusinessOperatingHours,
    "id" | "business_id" | "created_at" | "updated_at"
  >[],
): Promise<{ message: string }> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/operating-hours`,
    hours,
  );
  return response.data;
};

export const getBusinessOperatingExceptions = async (
  businessId: number,
): Promise<BusinessOperatingException[]> => {
  const response = await axiosInstance.get<BusinessOperatingException[]>(
    `/inside/businesses/${businessId}/operating-exceptions`,
  );
  return response.data;
};

export const updateBusinessOperatingExceptions = async (
  businessId: number,
  exceptions: Omit<
    BusinessOperatingException,
    "id" | "business_id" | "created_at" | "updated_at"
  >[],
): Promise<{ message: string }> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/operating-exceptions`,
    exceptions,
  );
  return response.data;
};

// Get business special features
export const getBusinessSpecialFeatures = async (
  businessId: number,
): Promise<BusinessSpecialFeature[]> => {
  const response = await axiosInstance.get<BusinessSpecialFeature[]>(
    `/inside/businesses/${businessId}/special-features`,
  );
  return response.data;
};

// Update business special features
export const updateBusinessSpecialFeatures = async (
  businessId: number,
  features: Omit<
    BusinessSpecialFeature,
    "id" | "business_id" | "created_at" | "updated_at"
  >[],
): Promise<{ message: string }> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/special-features`,
    features,
  );
  return response.data;
};

// Gallery Images interface
export interface BusinessGalleryImage {
  id: number;
  business_id: number;
  image_url: string;
  caption: string;
  display_order: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// Get business gallery images.
// Optional `limit` is a client-side bound (backend returns the full active
// set today) so pickers never materialize unbounded original-res previews.
export const getBusinessGalleryImages = async (
  businessId: number,
  options?: { limit?: number },
): Promise<BusinessGalleryImage[]> => {
  const response = await axiosInstance.get<BusinessGalleryImage[]>(
    `/inside/businesses/${businessId}/gallery-images`,
  );
  const images = Array.isArray(response.data) ? response.data : [];
  const limit = options?.limit;
  if (limit != null && Number.isFinite(limit) && limit >= 0) {
    return images.slice(0, Math.floor(limit));
  }
  return images;
};

// Update business gallery images. Optional `id` lets the backend upsert in
// place (preserving row IDs + caption translations) instead of delete-all.
export const updateBusinessGalleryImages = async (
  businessId: number,
  images: Array<
    Omit<
      BusinessGalleryImage,
      "id" | "business_id" | "created_at" | "updated_at"
    > & {
      id?: number;
    }
  >,
): Promise<{ message: string }> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/gallery-images`,
    images,
  );
  return response.data;
};

// Upload a logo for an existing, canonically owned business.
const uploadBusinessLogo = async (
  file: File,
  businessId: number,
): Promise<{ location: string; filename: string; folder: string }> => {
  const formData = new FormData();
  formData.append("file", file);

  const response = await axiosInstance.post<{
    location: string;
    filename: string;
    folder: string;
  }>(`/inside/businesses/${businessId}/uploads/logo`, formData, {
    headers: {
      "Content-Type": "multipart/form-data",
    },
  });
  return response.data;
};

interface GenerateMenuImageBundleItem {
  name: string;
  quantity: number;
}

export interface GenerateMenuImageRequest {
  name: string;
  description: string;
  ingredients?: string;
  // Item dietary tag ids (e.g. "vegetarian", "vegan"). Used server-side to add
  // hard no-meat/no-animal-product constraints to the exploded-view breakdown
  // prompt so it can never hallucinate animal protein onto a vegetarian/vegan
  // item (#588).
  dietary_tags?: string[];
  entity_type?: "menu_item" | "offer" | "bundle";
  offer_stamp_text?: string;
  offer_scope?: "all" | "category" | "item" | "bundle";
  related_item_names?: string[];
  bundle_items?: GenerateMenuImageBundleItem[];
  bundle_price?: Dollars;
  currency?: string;
}

// Generate AI composition/promotional image
export const generateMenuImage = async (
  businessId: number,
  data: GenerateMenuImageRequest,
): Promise<{ url: string; credit?: string }> => {
  const response = await axiosInstance.post<{ url: string; credit?: string }>(
    `/inside/businesses/${businessId}/generate-menu-image`,
    data,
  );
  return response.data;
};

// ========== AI Menu Features API ==========

// Types for AI Menu Extraction
interface ExtractedMenuItem {
  name: string;
  price: string;
  description: string;
  category: string;
  allergens: string[];
  dietary_tags?: string[];
  add_ons: { name: string; price: string }[];
  image_box?: number[]; // [pageIndex, ymin, xmin, ymax, xmax]
  image_url?: string;
}

interface ExtractedMenuCategory {
  name: string;
  items: ExtractedMenuItem[];
}

export interface ExtractedMenu {
  restaurant_name: string;
  currency: string;
  categories: ExtractedMenuCategory[];
}

// Types for AI Wizard
export interface WizardResponse {
  message: string;
  is_complete: boolean;
  extracted_config?: Record<string, string>;
  suggested_options?: string[];
}

export interface GeneratedMenu {
  categories: MenuCategory[];
  currency: string;
}

// AI Menu Extraction API (Session-based)
export interface MenuExtractionJob {
  id: number;
  business_id: number;
  status: "pending" | "uploading" | "processing" | "completed" | "failed";
  error_message?: string;
  extracted_menu?: string; // JSON string
  image_count: number;
  created_at: string;
}

export const startMenuExtraction = async (
  businessId: number,
): Promise<{ job_id: number }> => {
  const response = await axiosInstance.post<{ job_id: number }>(
    `/inside/businesses/${businessId}/ai/extract-menu/start`,
  );
  return response.data;
};

export const uploadMenuPage = async (
  businessId: number,
  jobId: number,
  file: Blob,
  pageOrder: number,
): Promise<{ image_id: number }> => {
  const formData = new FormData();
  const filename =
    file instanceof File && file.name
      ? file.name
      : `menu-page-${pageOrder}.jpg`;
  formData.append("file", file, filename);
  formData.append("page_order", pageOrder.toString());

  const response = await axiosInstance.post<{ image_id: number }>(
    `/inside/businesses/${businessId}/ai/extract-menu/upload/${jobId}`,
    formData,
    {
      headers: {
        "Content-Type": "multipart/form-data",
      },
    },
  );
  return response.data;
};

export const processMenuExtraction = async (
  businessId: number,
  jobId: number,
): Promise<{ status: string }> => {
  const response = await axiosInstance.post<{ status: string }>(
    `/inside/businesses/${businessId}/ai/extract-menu/process/${jobId}`,
  );
  return response.data;
};

export const getMenuExtractionJob = async (
  businessId: number,
  jobId: number,
): Promise<MenuExtractionJob> => {
  // Add timestamp to bust any caching (browser, proxy, CDN)
  const response = await axiosInstance.get<MenuExtractionJob>(
    `/inside/businesses/${businessId}/ai/extract-menu/${jobId}`,
    { params: { _t: Date.now() } },
  );
  return response.data;
};

export const importExtractedMenu = async (
  businessId: number,
  categories: MenuCategory[],
  currency?: string,
  confirmSanitization = false,
): Promise<MenuImportResponse> => {
  const response = await axiosInstance.post<MenuImportResponse>(
    `/inside/businesses/${businessId}/ai/import-extracted-menu`,
    {
      categories,
      currency,
      confirm_sanitization: confirmSanitization,
    },
  );
  return response.data;
};

// AI Wizard API
//
// #598: wizard replies are AI-generated and routinely take ~150s — far past
// the shared axiosInstance 30s default. Give the AI-backed wizard calls their
// own 300s budget (matching the Caddy wizard-route transport timeout) and
// skip the interceptor's generic "Couldn't reach the server" toast: AIWizard
// renders its own inline retry affordance (`retry: true` recovers the reply
// the backend already persisted), so the connection-blaming toast is both
// wrong and redundant here.
const WIZARD_REQUEST_CONFIG = {
  timeout: 300_000,
  _skipErrorToast: true,
} as const;

export const startWizardSession = async (
  businessId: number,
  language?: string,
): Promise<{ session_id: number; response: WizardResponse }> => {
  const response = await axiosInstance.post<{
    session_id: number;
    response: WizardResponse;
  }>(
    `/inside/businesses/${businessId}/ai/wizard/start`,
    language ? { language } : {},
    WIZARD_REQUEST_CONFIG,
  );
  return response.data;
};

export const sendWizardMessage = async (
  businessId: number,
  sessionId: number,
  message: string,
  language?: string,
  options: { retry?: boolean } = {},
): Promise<{ session_id: number; response: WizardResponse }> => {
  const response = await axiosInstance.post<{
    session_id: number;
    response: WizardResponse;
  }>(
    `/inside/businesses/${businessId}/ai/wizard/${sessionId}/message`,
    {
      message,
      retry: options.retry === true,
      ...(language ? { language } : {}),
    },
    WIZARD_REQUEST_CONFIG,
  );
  return response.data;
};

export const generateMenuFromWizard = async (
  businessId: number,
  sessionId: number,
): Promise<{ session_id: number; menu: GeneratedMenu }> => {
  const response = await axiosInstance.post<{
    session_id: number;
    menu: GeneratedMenu;
  }>(
    `/inside/businesses/${businessId}/ai/wizard/${sessionId}/generate`,
    undefined,
    WIZARD_REQUEST_CONFIG,
  );
  return response.data;
};

const importWizardMenu = async (
  businessId: number,
  sessionId: number,
  confirmSanitization = false,
): Promise<MenuImportResponse> => {
  const response = await axiosInstance.post<MenuImportResponse>(
    `/inside/businesses/${businessId}/ai/wizard/${sessionId}/import`,
    { confirm_sanitization: confirmSanitization },
  );
  return response.data;
};

export const regenerateMenuItemImage = async (
  businessId: number,
  itemName: string,
  itemDescription?: string,
  customPrompt?: string,
  // Item dietary tag ids (e.g. "vegetarian", "vegan"). Used server-side to add
  // hard dietary-safety constraints to the photo prompt so a regenerated image
  // can never hallucinate non-compliant ingredients (#601).
  dietaryTags?: string[],
): Promise<{ url: string; credit: string }> => {
  const response = await axiosInstance.post<{ url: string; credit: string }>(
    `/inside/businesses/${businessId}/ai/regenerate-image`,
    {
      item_name: itemName,
      item_description: itemDescription,
      custom_prompt: customPrompt,
      ...(dietaryTags && dietaryTags.length > 0
        ? { dietary_tags: dietaryTags }
        : {}),
    },
  );
  return response.data;
};

export const enhanceMenuItemImage = async (
  businessId: number,
  imageUrl: string,
  itemName: string,
  itemDescription?: string,
  // Item dietary tag ids — same server-side dietary-safety behavior as
  // regenerateMenuItemImage (#601).
  dietaryTags?: string[],
): Promise<{ url: string; credit: string }> => {
  const response = await axiosInstance.post<{ url: string; credit: string }>(
    `/inside/businesses/${businessId}/ai/enhance-image`,
    {
      image_url: imageUrl,
      item_name: itemName,
      item_description: itemDescription,
      ...(dietaryTags && dietaryTags.length > 0
        ? { dietary_tags: dietaryTags }
        : {}),
    },
  );
  return response.data;
};

// Export all functions as businessApi object
export const businessApi = {
  generateBusinessId,
  createBusiness,
  getMyBusinesses,
  getBusiness,
  updateBusiness,
  updateBusinessDesignSettings,
  deleteBusiness,
  getMenu,
  reorderMenu,
  addMenuCategory,
  updateMenuCategory,
  deleteMenuCategory,
  addMenuItem,
  updateMenuItem,
  deleteMenuItem,
  getBusinessTables,
  getTablesWithStatus,
  createTableWithQR,
  updateTableDetails,
  updateBusinessTable,
  deleteTable,
  applyQrBrandingToAllTables,
  getBusinessOperatingHours,
  updateBusinessOperatingHours,
  getBusinessOperatingExceptions,
  updateBusinessOperatingExceptions,
  getBusinessSpecialFeatures,
  updateBusinessSpecialFeatures,
  getBusinessGalleryImages,
  updateBusinessGalleryImages,
  uploadBusinessLogo,
  generateMenuImage,

  // Offers
  createOffer: async (businessId: number, offer: Offer) => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/offers`,
      offer,
    );
    return response.data;
  },
  getOffers: async (businessId: number): Promise<Offer[]> => {
    const response = await axiosInstance.get<Offer[]>(
      `/inside/businesses/${businessId}/offers`,
    );
    return response.data;
  },
  updateOffer: async (businessId: number, offerId: number, offer: Offer) => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/offers/${offerId}`,
      offer,
    );
    return response.data;
  },
  deleteOffer: async (businessId: number, offerId: number) => {
    const response = await axiosInstance.delete(
      `/inside/businesses/${businessId}/offers/${offerId}`,
    );
    return response.data;
  },

  // Bundles
  createBundle: async (businessId: number, bundle: Bundle) => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/bundles`,
      bundle,
    );
    return response.data;
  },
  getBundles: async (businessId: number): Promise<Bundle[]> => {
    const response = await axiosInstance.get<Bundle[]>(
      `/inside/businesses/${businessId}/bundles`,
    );
    return response.data;
  },
  updateBundle: async (
    businessId: number,
    bundleId: number,
    bundle: Bundle,
  ) => {
    const response = await axiosInstance.put(
      `/inside/businesses/${businessId}/bundles/${bundleId}`,
      bundle,
    );
    return response.data;
  },
  deleteBundle: async (businessId: number, bundleId: number) => {
    const response = await axiosInstance.delete(
      `/inside/businesses/${businessId}/bundles/${bundleId}`,
    );
    return response.data;
  },

  // AI Menu Features
  startMenuExtraction,
  uploadMenuPage,
  processMenuExtraction,
  getMenuExtractionJob,
  importExtractedMenu,
  startWizardSession,
  sendWizardMessage,
  generateMenuFromWizard,
  importWizardMenu,
  regenerateMenuItemImage,
  enhanceMenuItemImage,
};
