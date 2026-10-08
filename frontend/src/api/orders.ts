import { axiosInstance } from "./tools/instance";
import { validateOrder, validateOrdersResponse } from "./schemas/orders";
import { fetchAllPages } from "./pagination";
import type {
  FetchAllPagesResult,
  PaginatedResponse,
  PaginationOptions,
} from "./pagination";
import { asDollars, type Dollars } from "@/types/money";
import type { Bill } from "./bills";
import type { Business } from "./business";

// Order money fields are in DOLLARS (MenuItem/Bundle/Offer store float64
// dollars in the DB; see internal/database/models.go).
interface MenuItemOption {
  id: string;
  name: string;
  price_change: Dollars;
  is_required: boolean;
}

export interface OrderItem {
  id: string;
  item_type?: "menu_item" | "bundle" | "bundle_item" | "discount";
  menu_item_id?: string;
  bundle_id?: number;
  parent_bundle_id?: number;
  source_offer_id?: number;
  menu_item_name: string;
  quantity: number;
  price: Dollars;
  options: MenuItemOption[]; // Add-ons/modifiers
  special_requests: string;
  subtotal: Dollars;
}

/** Mirrors the backend DeliveryStatus constants (delivery_models.go). */
export type OrderDeliveryStatus =
  | "pending"
  | "confirmed"
  | "preparing"
  | "ready"
  | "assigned"
  | "picked_up"
  | "in_transit"
  | "nearby"
  | "delivered"
  | "cancelled"
  | "failed";

export interface Order {
  id: number;
  bill_id: number;
  business_id: number;
  order_number: string;
  status:
    | "pending"
    | "approved"
    | "in_kitchen"
    | "ready"
    | "delivered"
    | "cancelled";
  created_by?: string | null;
  approved_by?: string | null;
  cancelled_by?: string | null;
  notes?: string | null;
  /** Line array on the wire (#771). Legacy quoted JSON strings still parse. */
  items: string | OrderItem[];
  currency: string;
  cancel_reason?: string | null;
  created_at: string;
  updated_at: string;
  approved_at?: string | null;
  cancelled_at?: string | null;
  delivery?: {
    delivery_id: number;
    delivery_number: string;
    delivery_status: OrderDeliveryStatus;
    customer_name: string;
    customer_phone: string;
    street: string;
    city: string;
    payment_expires_at?: string | null;
  };
  // Optional preloaded relations. The backend includes the full Bill/Business
  // rows when it preloads them (json:"bill,omitempty" / "business,omitempty");
  // the kitchen display only reads bill.table_id today.
  bill?: Bill;
  business?: Business;
}

export interface CreateOrderRequest {
  bill_id: number;
  notes?: string;
  promo_code?: string;
  items: {
    menu_item_name: string;
    menu_item_id?: string;
    quantity: number;
    price: Dollars;
    item_type?: "menu_item" | "bundle" | "bundle_item" | "discount";
    bundle_id?: number;
    parent_bundle_id?: number;
    source_offer_id?: number;
    options?: MenuItemOption[]; // Add-ons/modifiers
    special_requests?: string;
  }[];
}

export type OrderabilityState =
  | "available"
  | "manual_disabled"
  | "inventory_out"
  | "inventory_warning"
  | "business_closed"
  | "ordering_disabled";

export interface Orderability {
  state: OrderabilityState;
  orderable: boolean;
}

type OrderQuoteLineType = "menu_item" | "bundle" | "bundle_item" | "discount";

interface OrderQuoteLine {
  key: string;
  line_type: OrderQuoteLineType;
  unit_price: Dollars;
  quantity: number;
  subtotal: Dollars;
  orderability?: Orderability;
}

export interface OrderQuote {
  subtotal: Dollars;
  discount: Dollars;
  net_subtotal: Dollars;
  tax: Dollars;
  service_fee: Dollars;
  tip: Dollars;
  /** Final payable amount: net_subtotal + tax + service_fee + tip. */
  total: Dollars;
  lines: OrderQuoteLine[];
}

export interface OrderDraft {
  bill_id?: number;
  items: CreateOrderRequest["items"];
  notes?: string;
  promo_code?: string;
}

export interface GuestCheckoutResult {
  bill: Bill;
  order: Order;
  quote: OrderQuote;
  replay: boolean;
  /** Compatibility alias retained while older guest screens migrate. */
  duplicate?: boolean;
}

const QUOTE_CENT_TOLERANCE = 1e-6;

function quoteContractError(field: string): Error {
  return new Error(`Malformed order quote: ${field}`);
}

function readCentAmount(value: unknown, field: string): number {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw quoteContractError(`${field} must be a finite number`);
  }

  // The wire contract uses major currency units, while the quote engine is
  // cent-exact. Accept normal IEEE-754 noise, but reject values that contain a
  // real fractional cent instead of silently rounding them in the browser.
  const scaled = value * 100;
  if (Math.abs(scaled - Math.round(scaled)) > QUOTE_CENT_TOLERANCE) {
    throw quoteContractError(`${field} must resolve to whole cents`);
  }
  if (!Number.isSafeInteger(Math.round(scaled))) {
    throw quoteContractError(`${field} exceeds the safe cent range`);
  }
  return value;
}

function readQuoteAmount(value: unknown, field: string): number {
  const amount = readCentAmount(value, field);
  if (amount < 0) {
    throw quoteContractError(`${field} must be nonnegative`);
  }
  return amount;
}

function amountToCents(value: number): number {
  return Math.round(value * 100);
}

const ORDERABILITY_STATES = new Set<OrderabilityState>([
  "available",
  "manual_disabled",
  "inventory_out",
  "inventory_warning",
  "business_closed",
  "ordering_disabled",
]);

const ORDER_QUOTE_LINE_TYPES = new Set<OrderQuoteLineType>([
  "menu_item",
  "bundle",
  "bundle_item",
  "discount",
]);

function readOrderability(
  value: unknown,
  field: string,
): Orderability | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw quoteContractError(`${field} must be an object`);
  }
  const raw = value as Record<string, unknown>;
  if (
    typeof raw.state !== "string" ||
    !ORDERABILITY_STATES.has(raw.state as OrderabilityState) ||
    typeof raw.orderable !== "boolean"
  ) {
    throw quoteContractError(`${field} is invalid`);
  }
  return { state: raw.state as OrderabilityState, orderable: raw.orderable };
}

const deserializeOrderQuote = (value: unknown): OrderQuote => {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw quoteContractError("response must be an object");
  }
  const raw = value as Record<string, unknown>;
  if (!Array.isArray(raw.lines)) {
    throw quoteContractError("lines must be an array");
  }

  const subtotal = readQuoteAmount(raw.subtotal, "subtotal");
  const discount = readQuoteAmount(raw.discount, "discount");
  const lines = raw.lines.map((value, index): OrderQuoteLine => {
    if (typeof value !== "object" || value === null || Array.isArray(value)) {
      throw quoteContractError(`lines[${index}] must be an object`);
    }
    const line = value as Record<string, unknown>;
    if (typeof line.key !== "string" || line.key.trim() === "") {
      throw quoteContractError(`lines[${index}].key is required`);
    }
    if (
      typeof line.line_type !== "string" ||
      !ORDER_QUOTE_LINE_TYPES.has(line.line_type as OrderQuoteLineType)
    ) {
      throw quoteContractError(`lines[${index}].line_type is invalid`);
    }
    const lineType = line.line_type as OrderQuoteLineType;
    if (
      !Number.isSafeInteger(line.quantity) ||
      (line.quantity as number) <= 0
    ) {
      throw quoteContractError(
        `lines[${index}].quantity must be a positive integer`,
      );
    }
    const unitPrice = readCentAmount(
      line.unit_price,
      `lines[${index}].unit_price`,
    );
    const lineSubtotal = readCentAmount(
      line.subtotal,
      `lines[${index}].subtotal`,
    );
    const unitPriceCents = amountToCents(unitPrice);
    const lineSubtotalCents = amountToCents(lineSubtotal);
    const expectedSubtotalCents = unitPriceCents * (line.quantity as number);
    if (!Number.isSafeInteger(expectedSubtotalCents)) {
      throw quoteContractError(
        `lines[${index}].subtotal exceeds the safe cent range`,
      );
    }

    if (lineType === "bundle_item") {
      if (unitPriceCents < 0 || lineSubtotalCents !== 0) {
        throw quoteContractError(
          `lines[${index}] bundle items must be nonnegative informational lines with a zero subtotal`,
        );
      }
    } else if (lineType === "discount") {
      if (
        unitPriceCents >= 0 ||
        lineSubtotalCents >= 0 ||
        lineSubtotalCents !== expectedSubtotalCents
      ) {
        throw quoteContractError(
          `lines[${index}] discounts must be negative and match unit price times quantity`,
        );
      }
    } else if (
      unitPriceCents < 0 ||
      lineSubtotalCents < 0 ||
      lineSubtotalCents !== expectedSubtotalCents
    ) {
      throw quoteContractError(
        `lines[${index}].subtotal does not match unit price and quantity`,
      );
    }
    return {
      key: line.key,
      line_type: lineType,
      unit_price: asDollars(unitPrice),
      quantity: line.quantity as number,
      subtotal: asDollars(lineSubtotal),
      orderability: readOrderability(
        line.orderability,
        `lines[${index}].orderability`,
      ),
    };
  });

  const subtotalCents = amountToCents(subtotal);
  const discountCents = amountToCents(discount);
  const billableLineCents = lines.reduce(
    (sum, line) =>
      line.line_type === "menu_item" || line.line_type === "bundle"
        ? sum + amountToCents(Number(line.subtotal))
        : sum,
    0,
  );
  const adjustmentCents = lines.reduce(
    (sum, line) =>
      line.line_type === "discount"
        ? sum + amountToCents(Number(line.subtotal))
        : sum,
    0,
  );
  if (billableLineCents !== subtotalCents) {
    throw quoteContractError("subtotal does not match line subtotals");
  }
  if (-adjustmentCents !== discountCents) {
    throw quoteContractError("discount does not match discount lines");
  }
  if (discountCents > subtotalCents) {
    throw quoteContractError("discount cannot exceed subtotal");
  }

  const netSubtotal = readQuoteAmount(raw.net_subtotal, "net_subtotal");
  const tax = readQuoteAmount(raw.tax, "tax");
  const serviceFee = readQuoteAmount(raw.service_fee, "service_fee");
  const tip = readQuoteAmount(raw.tip, "tip");
  const total = readQuoteAmount(raw.total, "total");
  if (subtotalCents - discountCents !== amountToCents(netSubtotal)) {
    throw quoteContractError(
      "net_subtotal does not match subtotal minus discount",
    );
  }
  const expectedFinalTotalCents =
    amountToCents(netSubtotal) +
    amountToCents(tax) +
    amountToCents(serviceFee) +
    amountToCents(tip);
  if (
    !Number.isSafeInteger(expectedFinalTotalCents) ||
    expectedFinalTotalCents !== amountToCents(total)
  ) {
    throw quoteContractError(
      "total does not match net_subtotal plus tax, service_fee, and tip",
    );
  }

  return {
    subtotal: asDollars(subtotal),
    discount: asDollars(discount),
    net_subtotal: asDollars(netSubtotal),
    tax: asDollars(tax),
    service_fee: asDollars(serviceFee),
    tip: asDollars(tip),
    total: asDollars(total),
    lines,
  };
};

export interface UpdateOrderStatusRequest {
  status:
    | "pending"
    | "approved"
    | "in_kitchen"
    | "ready"
    | "delivered"
    | "cancelled";
  approved_by?: string;
  reason?: string;
}

export interface OrdersResponse extends PaginatedResponse<Order> {
  orders: Order[];
  total: number;
}

export interface OrderListOptions extends PaginationOptions {
  activeBillsOnly?: boolean;
  /**
   * "asc" orders oldest-first so a row cap drops the newest orders rather than
   * the oldest FIFO ones (the kitchen board uses this). Omit for the legacy
   * newest-first ordering.
   */
  sort?: "asc" | "desc";
}

// Order API functions

// Create a new order (for staff)
export const createOrder = async (
  businessId: number,
  orderData: CreateOrderRequest,
  options?: {
    /**
     * B-6: forwarded as X-Request-Id. The backend stores it as
     * ClientRequestID with the same dedupe-replay semantics as guest orders,
     * so a double-click or transport retry can't create duplicate tickets.
     */
    idempotencyKey?: string;
  },
): Promise<Order> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/orders`,
    orderData,
    {
      headers: options?.idempotencyKey
        ? {
            "X-Request-Id": options.idempotencyKey,
          }
        : undefined,
    },
  );
  return validateOrder(response.data.order);
};

// Get orders for a business
export const getOrders = async (
  businessId: number,
  status?: string,
  options?: OrderListOptions,
  signal?: AbortSignal,
): Promise<OrdersResponse> => {
  const params = new URLSearchParams();
  if (status) params.append("status", status);
  if (options?.page) params.append("page", String(options.page));
  if (options?.pageSize) params.append("page_size", String(options.pageSize));
  if (options?.activeBillsOnly) params.append("active_bills_only", "true");
  if (options?.sort) params.append("sort", options.sort);

  const query = params.toString();
  const path = `/inside/businesses/${businessId}/orders`;
  // Pass a config object only when a signal is present so the call shape stays
  // identical for the (vast majority of) callers that don't cancel.
  const response = await axiosInstance.get(
    query ? `${path}?${query}` : path,
    ...(signal ? [{ signal }] : []),
  );
  return validateOrdersResponse(response.data);
};

const ACTIVE_ORDER_STATUSES: Order["status"][] = [
  "pending",
  "approved",
  "in_kitchen",
  "ready",
];
const DEFAULT_ACTIVE_ORDER_PAGE_SIZE = 100;
const DEFAULT_ACTIVE_ORDER_MAX_ROWS = 1000;
const LIVE_DATA_CAP_WARNING = "live_data_cap_reached";

const normalizePositiveInt = (value: unknown, fallback: number): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < 1) {
    return fallback;
  }
  return Math.floor(parsed);
};

const fetchActiveOrdersByStatus = async (
  businessId: number,
  status: string,
  options: {
    pageSize: number;
    maxPages?: number;
    maxRows: number;
    activeBillsOnly?: boolean;
    sort?: "asc" | "desc";
  },
): Promise<FetchAllPagesResult<Order>> =>
  fetchAllPages<Order>(
    (page, pageSize) =>
      getOrders(businessId, status, {
        page,
        pageSize,
        activeBillsOnly: options.activeBillsOnly,
        sort: options.sort,
      }) as Promise<OrdersResponse>,
    (response) => (response as OrdersResponse).orders,
    {
      pageSize: options.pageSize,
      maxPages: options.maxPages,
      maxRows: options.maxRows,
    },
  );

export const getAllActiveOrders = async (
  businessId: number,
  options: {
    statuses?: Order["status"][];
    pageSize?: number;
    maxPages?: number;
    maxRows?: number;
    activeBillsOnly?: boolean;
    /** "asc" keeps the OLDEST orders when the row cap is hit (kitchen FIFO). */
    sort?: "asc" | "desc";
  } = {},
): Promise<FetchAllPagesResult<Order>> => {
  const statuses = options.statuses ?? ACTIVE_ORDER_STATUSES;
  const maxRows = normalizePositiveInt(
    options.maxRows,
    DEFAULT_ACTIVE_ORDER_MAX_ROWS,
  );
  const requestedPageSize = normalizePositiveInt(
    options.pageSize,
    DEFAULT_ACTIVE_ORDER_PAGE_SIZE,
  );
  const combinedStatus = statuses.join(",");

  if (combinedStatus) {
    try {
      const page = await fetchActiveOrdersByStatus(businessId, combinedStatus, {
        pageSize: Math.min(requestedPageSize, maxRows),
        maxPages: options.maxPages,
        maxRows,
        activeBillsOnly: options.activeBillsOnly,
        sort: options.sort,
      });

      return {
        ...page,
        warning: page.capped ? LIVE_DATA_CAP_WARNING : undefined,
      };
    } catch {
      // Older deployments or transient failures can still be recovered by the
      // per-status fallback below.
    }
  }

  const pages: FetchAllPagesResult<Order>[] = [];
  let remainingRows = maxRows;
  let statusFailed = false;
  let stoppedForBudget = false;

  for (let index = 0; index < statuses.length; index += 1) {
    if (remainingRows <= 0) {
      stoppedForBudget = true;
      break;
    }

    const status = statuses[index];
    const statusPageSize = Math.min(requestedPageSize, remainingRows);

    try {
      const page = await fetchActiveOrdersByStatus(businessId, status, {
        pageSize: statusPageSize,
        maxPages: options.maxPages,
        maxRows: remainingRows,
        activeBillsOnly: options.activeBillsOnly,
        sort: options.sort,
      });

      pages.push(page);
      remainingRows -= page.items.length;

      if (remainingRows <= 0 && index < statuses.length - 1) {
        stoppedForBudget = true;
        break;
      }
    } catch {
      statusFailed = true;
    }
  }

  const allItems = pages.flatMap((page) => page.items);
  const items = allItems.slice(0, maxRows);
  const trimmed = items.length < allItems.length;
  const capped =
    pages.some((page) => page.capped) ||
    trimmed ||
    statusFailed ||
    stoppedForBudget;
  const combinedTotal = pages.reduce(
    (sum, page) => sum + page.metadata.total,
    0,
  );
  const pageSize =
    pages[0]?.metadata.page_size ?? Math.min(requestedPageSize, maxRows);
  const combinedTotalPages = Math.max(1, Math.ceil(combinedTotal / pageSize));

  return {
    items,
    metadata: {
      total: combinedTotal,
      page: 1,
      page_size: pageSize,
      total_pages: combinedTotalPages,
    },
    capped,
    warning: capped ? LIVE_DATA_CAP_WARNING : undefined,
  };
};

// Get orders for a specific bill (public - for guests). billToken is the
// unguessable public_token (legacy bill_number fallback via guestBillRef).
export const getGuestOrdersByBillNumber = async (
  billToken: string,
  options?: { disableCache?: boolean },
  signal?: AbortSignal,
): Promise<OrdersResponse> => {
  const config: any = {};

  // Disable caching if requested (for real-time polling)
  if (options?.disableCache) {
    config._useCache = false;
    // Also add timestamp to URL to bust any browser cache
    config.params = { _t: Date.now() };
  }

  // Thread the poll cycle's AbortSignal so a superseded/unmounted guest poll
  // cancels its in-flight request instead of resolving into a stale write.
  if (signal) {
    config.signal = signal;
  }

  const response = await axiosInstance.get(
    `/guest/bill/${billToken}/orders`,
    config,
  );
  return response.data;
};

// Cancel an order (for staff — approved or ready orders)
export async function cancelOrder(
  businessId: number | string,
  orderId: number,
  reason: string,
): Promise<void> {
  await axiosInstance.patch(
    `/inside/businesses/${businessId}/orders/${orderId}/cancel`,
    { reason },
  );
}

// Update order status (for staff approval/kitchen updates)
export const updateOrderStatus = async (
  businessId: number,
  orderId: number,
  statusData: UpdateOrderStatusRequest,
): Promise<Order> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/orders/${orderId}/status`,
    statusData,
  );
  return response.data.order;
};

// Guest order creation (public endpoint)
export const createGuestOrder = async (
  tableCode: string,
  orderData: OrderDraft,
  options?: {
    idempotencyKey?: string;
  },
): Promise<GuestCheckoutResult> => {
  const response = await axiosInstance.post(
    `/guest/table/${tableCode}/order`,
    orderData,
    {
      headers: options?.idempotencyKey
        ? {
            "X-Request-Id": options.idempotencyKey,
          }
        : undefined,
    },
  );
  return {
    ...response.data,
    quote: deserializeOrderQuote(response.data.quote),
  } as GuestCheckoutResult;
};

export const quoteGuestOrder = async (
  tableCode: string,
  orderData: OrderDraft,
): Promise<OrderQuote> => {
  const response = await axiosInstance.post(
    `/guest/table/${tableCode}/order/quote`,
    orderData,
    { _skipErrorToast: true },
  );
  return deserializeOrderQuote(response.data);
};

export const quoteBusinessOrder = async (
  businessId: number,
  orderData: OrderDraft,
): Promise<OrderQuote> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/orders/quote`,
    orderData,
  );
  return deserializeOrderQuote(response.data);
};

// Guest order cancellation (public endpoint — pending orders only)
export async function guestCancelOrder(
  tableCode: string,
  orderId: number,
  reason?: string,
): Promise<void> {
  await axiosInstance.post(
    `/guest/table/${tableCode}/orders/${orderId}/cancel`,
    { reason: reason || "" },
  );
}

/** Normalizes order.items from an array or a legacy JSON string (#771). */
export const parseOrderItems = (itemsJson: unknown): OrderItem[] => {
  if (Array.isArray(itemsJson)) {
    return itemsJson as OrderItem[];
  }
  if (typeof itemsJson !== "string" || !itemsJson.trim()) {
    return [];
  }
  try {
    const parsed = JSON.parse(itemsJson) as unknown;
    return Array.isArray(parsed) ? (parsed as OrderItem[]) : [];
  } catch {
    return [];
  }
};
