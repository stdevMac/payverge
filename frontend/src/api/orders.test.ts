import {
  createGuestOrder,
  createOrder,
  quoteBusinessOrder,
  quoteGuestOrder,
  getAllActiveOrders,
  getGuestOrdersByBillNumber,
  getOrders,
} from "@/api/orders";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

// logError is fire-and-forget (void) and POSTs to the API; unmocked, its
// console fallback lands after this file's teardown and fails the run.
jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
}));

describe("orders api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads guest orders by guest bill token", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { orders: [], total: 0 },
    });

    await getGuestOrdersByBillNumber("B42-opaque", { disableCache: true });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/orders",
      expect.objectContaining({
        _useCache: false,
        params: expect.any(Object),
      }),
    );
  });

  it("sends a stable idempotency key for guest order creation when provided", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        order: { id: 1 },
        quote: {
          subtotal: 10,
          discount: 0,
          net_subtotal: 10,
          tax: 0,
          service_fee: 0,
          tip: 0,
          total: 10,
          lines: [
            {
              key: "burger",
              line_type: "menu_item",
              unit_price: 10,
              quantity: 1,
              subtotal: 10,
            },
          ],
        },
      },
    });

    await createGuestOrder(
      "T1",
      {
        bill_id: 7,
        items: [
          {
            menu_item_name: "Burger",
            menu_item_id: "burger",
            quantity: 1,
            price: asDollars(10),
          },
        ],
      },
      {
        idempotencyKey: "guest-order-retry-1",
      },
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/table/T1/order",
      expect.any(Object),
      {
        headers: {
          "X-Request-Id": "guest-order-retry-1",
        },
      },
    );
  });

  it("quotes guest and operator orders through the authoritative endpoints", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        subtotal: 9.99,
        discount: 1,
        net_subtotal: 8.99,
        tax: 0.9,
        service_fee: 0.45,
        tip: 0,
        total: 10.34,
        lines: [
          {
            key: "burger",
            line_type: "menu_item",
            unit_price: 9.99,
            quantity: 1,
            subtotal: 9.99,
          },
          {
            key: "offer:1",
            line_type: "discount",
            unit_price: -1,
            quantity: 1,
            subtotal: -1,
          },
        ],
      },
    });
    const items = [
      {
        menu_item_name: "Burger",
        menu_item_id: "burger",
        quantity: 1,
        price: asDollars(9.99),
      },
    ];

    const guestQuote = await quoteGuestOrder("T1", { items });
    await quoteBusinessOrder(42, { items });

    expect(guestQuote).toEqual(
      expect.objectContaining({
        net_subtotal: 8.99,
        tax: 0.9,
        service_fee: 0.45,
        tip: 0,
        total: 10.34,
      }),
    );

    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      1,
      "/guest/table/T1/order/quote",
      { items },
      { _skipErrorToast: true },
    );
    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/42/orders/quote",
      { items },
    );
  });

  it("rejects a quote that omits the authoritative settlement breakdown", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        subtotal: 10,
        discount: 0,
        total: 10,
        lines: [
          {
            key: "burger",
            line_type: "menu_item",
            unit_price: 10,
            quantity: 1,
            subtotal: 10,
          },
        ],
      },
    });

    await expect(
      quoteGuestOrder("T1", {
        items: [
          {
            menu_item_name: "Burger",
            menu_item_id: "burger",
            quantity: 1,
            price: asDollars(10),
          },
        ],
      }),
    ).rejects.toThrow("Malformed order quote");
  });

  it.each([
    ["a mismatched net subtotal", { net_subtotal: 9, total: 9 }],
    ["a mismatched final total", { total: 10.01 }],
    ["a negative settlement component", { service_fee: -0.01 }],
  ])("rejects a quote with %s", async (_name, override) => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        subtotal: 10,
        discount: 0,
        net_subtotal: 10,
        tax: 0,
        service_fee: 0,
        tip: 0,
        total: 10,
        lines: [
          {
            key: "burger",
            line_type: "menu_item",
            unit_price: 10,
            quantity: 1,
            subtotal: 10,
          },
        ],
        ...override,
      },
    });

    await expect(
      quoteGuestOrder("T1", {
        items: [
          {
            menu_item_name: "Burger",
            menu_item_id: "burger",
            quantity: 1,
            price: asDollars(10),
          },
        ],
      }),
    ).rejects.toThrow("Malformed order quote");
  });

  it.each([
    ["missing totals", { lines: [] }],
    [
      "non-finite totals",
      { subtotal: Number.NaN, discount: 0, total: 0, lines: [] },
    ],
    ["negative totals", { subtotal: -1, discount: 0, total: 0, lines: [] }],
    ["missing lines", { subtotal: 1, discount: 0, total: 1 }],
    [
      "fractional cents",
      {
        subtotal: 1.001,
        discount: 0,
        total: 1.001,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 1.001,
            quantity: 1,
            subtotal: 1.001,
          },
        ],
      },
    ],
    [
      "invalid line quantities",
      {
        subtotal: 1,
        discount: 0,
        total: 1,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 1,
            quantity: 0,
            subtotal: 1,
          },
        ],
      },
    ],
    [
      "unsafe line quantities",
      {
        subtotal: 1,
        discount: 0,
        total: 1,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 0,
            quantity: Number.MAX_SAFE_INTEGER + 1,
            subtotal: 0,
          },
        ],
      },
    ],
    [
      "invalid orderability",
      {
        subtotal: 1,
        discount: 0,
        total: 1,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 1,
            quantity: 1,
            subtotal: 1,
            orderability: { state: "maybe", orderable: "yes" },
          },
        ],
      },
    ],
    [
      "line arithmetic mismatch",
      {
        subtotal: 2,
        discount: 0,
        total: 2,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 1,
            quantity: 2,
            subtotal: 1,
          },
        ],
      },
    ],
    [
      "quote arithmetic mismatch",
      {
        subtotal: 2,
        discount: 0.5,
        total: 2,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 1,
            quantity: 2,
            subtotal: 2,
          },
        ],
      },
    ],
  ])("rejects a malformed authoritative quote with %s", async (_name, data) => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data });
    const items = [
      {
        menu_item_name: "Tea",
        menu_item_id: "tea",
        quantity: 1,
        price: asDollars(1),
      },
    ];

    await expect(quoteGuestOrder("T1", { items })).rejects.toThrow(
      "Malformed order quote",
    );
  });

  it("accepts ordinary floating point wire noise when values still resolve to cents", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        subtotal: 0.1 + 0.2,
        discount: 0,
        net_subtotal: 0.3,
        tax: 0,
        service_fee: 0,
        tip: 0,
        total: 0.3,
        lines: [
          {
            key: "tea",
            line_type: "menu_item",
            unit_price: 0.1,
            quantity: 3,
            subtotal: 0.3,
          },
        ],
      },
    });

    await expect(
      quoteGuestOrder("T1", {
        items: [{ menu_item_name: "Tea", quantity: 3, price: asDollars(0.1) }],
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        subtotal: expect.any(Number),
        total: expect.any(Number),
      }),
    );
  });

  it("accepts the real bundle quote shape with priced informational child lines", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        subtotal: 18,
        discount: 0,
        net_subtotal: 18,
        tax: 0,
        service_fee: 0,
        tip: 0,
        total: 18,
        lines: [
          {
            key: "bundle:7",
            line_type: "bundle",
            unit_price: 18,
            quantity: 1,
            subtotal: 18,
          },
          {
            key: "burger",
            line_type: "bundle_item",
            unit_price: 12,
            quantity: 1,
            subtotal: 0,
            orderability: { state: "available", orderable: true },
          },
          {
            key: "fries",
            line_type: "bundle_item",
            unit_price: 5,
            quantity: 2,
            subtotal: 0,
            orderability: { state: "inventory_warning", orderable: true },
          },
        ],
      },
    });

    await expect(
      quoteGuestOrder("T1", {
        items: [
          {
            menu_item_name: "Burger combo",
            item_type: "bundle",
            bundle_id: 7,
            quantity: 1,
            price: asDollars(18),
          },
        ],
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        subtotal: 18,
        total: 18,
        lines: expect.arrayContaining([
          expect.objectContaining({
            key: "burger",
            line_type: "bundle_item",
            subtotal: 0,
          }),
        ]),
      }),
    );
  });

  it("accepts the real discount quote shape with explicit negative adjustment lines", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        subtotal: 20,
        discount: 4,
        net_subtotal: 16,
        tax: 0,
        service_fee: 0,
        tip: 0,
        total: 16,
        lines: [
          {
            key: "burger",
            line_type: "menu_item",
            unit_price: 10,
            quantity: 2,
            subtotal: 20,
          },
          {
            key: "offer:12",
            line_type: "discount",
            unit_price: -4,
            quantity: 1,
            subtotal: -4,
          },
        ],
      },
    });

    await expect(
      quoteBusinessOrder(42, {
        items: [
          {
            menu_item_name: "Burger",
            menu_item_id: "burger",
            quantity: 2,
            price: asDollars(10),
          },
        ],
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        subtotal: 20,
        discount: 4,
        total: 16,
        lines: expect.arrayContaining([
          expect.objectContaining({
            line_type: "discount",
            unit_price: -4,
            subtotal: -4,
          }),
        ]),
      }),
    );
  });

  it.each([
    [
      "a normal line with a negative amount",
      {
        subtotal: 10,
        discount: 0,
        total: 10,
        lines: [
          {
            key: "burger",
            line_type: "menu_item",
            unit_price: -10,
            quantity: 1,
            subtotal: -10,
          },
        ],
      },
    ],
    [
      "a discount line with a positive amount",
      {
        subtotal: 10,
        discount: 2,
        total: 8,
        lines: [
          {
            key: "burger",
            line_type: "menu_item",
            unit_price: 10,
            quantity: 1,
            subtotal: 10,
          },
          {
            key: "offer:1",
            line_type: "discount",
            unit_price: 2,
            quantity: 1,
            subtotal: 2,
          },
        ],
      },
    ],
    [
      "a bundle child with a billable subtotal",
      {
        subtotal: 10,
        discount: 0,
        total: 10,
        lines: [
          {
            key: "bundle:1",
            line_type: "bundle",
            unit_price: 10,
            quantity: 1,
            subtotal: 10,
          },
          {
            key: "burger",
            line_type: "bundle_item",
            unit_price: 8,
            quantity: 1,
            subtotal: 8,
          },
        ],
      },
    ],
    [
      "an unknown line type",
      {
        subtotal: 10,
        discount: 0,
        total: 10,
        lines: [
          {
            key: "burger",
            line_type: "mystery",
            unit_price: 10,
            quantity: 1,
            subtotal: 10,
          },
        ],
      },
    ],
  ])("rejects line-type semantic violations: %s", async (_name, data) => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data });

    await expect(
      quoteGuestOrder("T1", {
        items: [
          {
            menu_item_name: "Burger",
            menu_item_id: "burger",
            quantity: 1,
            price: asDollars(10),
          },
        ],
      }),
    ).rejects.toThrow("Malformed order quote");
  });

  it("passes pagination params to the staff orders endpoint", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { orders: [], total: 0, page: 2, page_size: 25, total_pages: 1 },
    });

    await getOrders(42, "ready", { page: 2, pageSize: 25 });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/orders?status=ready&page=2&page_size=25",
    );
  });

  it("omits the query separator when staff orders have no query params", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { orders: [], total: 0, page: 1, page_size: 100, total_pages: 1 },
    });

    await getOrders(42);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/orders",
    );
  });

  it("forwards sort=asc so the kitchen FIFO cap keeps the oldest orders", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { orders: [], total: 0, page: 1, page_size: 25, total_pages: 1 },
    });

    await getOrders(42, "pending", { page: 1, pageSize: 25, sort: "asc" });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/orders?status=pending&page=1&page_size=25&sort=asc",
    );
  });

  it("can request active orders only for active bills", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { orders: [], total: 0, page: 1, page_size: 2, total_pages: 1 },
    });

    await getAllActiveOrders(42, {
      statuses: ["pending"],
      pageSize: 2,
      activeBillsOnly: true,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/orders?status=pending&page=1&page_size=2&active_bills_only=true",
    );
  });

  it("loads multiple active statuses with one paginated request", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        orders: [
          {
            id: 1,
            status: "pending",
            bill_id: 1,
            business_id: 42,
            order_number: "O1",
            created_by: "",
            approved_by: "",
            notes: "",
            items: "[]",
            created_at: "",
            updated_at: "",
          },
          {
            id: 2,
            status: "approved",
            bill_id: 1,
            business_id: 42,
            order_number: "O2",
            created_by: "",
            approved_by: "",
            notes: "",
            items: "[]",
            created_at: "",
            updated_at: "",
          },
        ],
        total: 2,
        page: 1,
        page_size: 100,
        total_pages: 1,
      },
    });

    const result = await getAllActiveOrders(42, {
      statuses: ["pending", "approved"],
      activeBillsOnly: true,
    });

    expect(result.items.map((order) => order.id)).toEqual([1, 2]);
    expect(axiosInstance.get).toHaveBeenCalledTimes(1);
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/orders?status=pending%2Capproved&page=1&page_size=100&active_bills_only=true",
    );
  });

  it("loads every active order page until exhausted", async () => {
    (axiosInstance.get as jest.Mock)
      .mockResolvedValueOnce({
        data: {
          orders: [
            {
              id: 1,
              status: "ready",
              bill_id: 1,
              business_id: 42,
              order_number: "O1",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
          ],
          total: 2,
          page: 1,
          page_size: 1,
          total_pages: 2,
        },
      })
      .mockResolvedValueOnce({
        data: {
          orders: [
            {
              id: 2,
              status: "ready",
              bill_id: 2,
              business_id: 42,
              order_number: "O2",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
          ],
          total: 2,
          page: 2,
          page_size: 1,
          total_pages: 2,
        },
      });

    const result = await getAllActiveOrders(42, {
      statuses: ["ready"],
      pageSize: 1,
    });

    expect(result.items.map((order) => order.id)).toEqual([1, 2]);
    expect(result.capped).toBe(false);
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/42/orders?status=ready&page=1&page_size=1",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/42/orders?status=ready&page=2&page_size=1",
    );
    expect(axiosInstance.get).toHaveBeenCalledTimes(2);
  });

  it("returns successful active order statuses when another status fails", async () => {
    (axiosInstance.get as jest.Mock)
      .mockRejectedValueOnce(new Error("combined status unavailable"))
      .mockResolvedValueOnce({
        data: {
          orders: [
            {
              id: 1,
              status: "ready",
              bill_id: 1,
              business_id: 42,
              order_number: "O1",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
          ],
          total: 1,
          page: 1,
          page_size: 1,
          total_pages: 1,
        },
      })
      .mockRejectedValueOnce(new Error("status unavailable"));

    const result = await getAllActiveOrders(42, {
      statuses: ["ready", "pending"],
      pageSize: 1,
      maxRows: 2,
    });

    expect(result.items.map((order) => order.id)).toEqual([1]);
    expect(result.metadata).toEqual({
      total: 1,
      page: 1,
      page_size: 1,
      total_pages: 1,
    });
    expect(result.capped).toBe(true);
    expect(result.warning).toBe("live_data_cap_reached");
    expect(axiosInstance.get).toHaveBeenCalledTimes(3);
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/42/orders?status=ready%2Cpending&page=1&page_size=1",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/42/orders?status=ready&page=1&page_size=1",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      3,
      "/inside/businesses/42/orders?status=pending&page=1&page_size=1",
    );
  });

  it("uses the normalized page size for active order metadata", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        orders: [
          {
            id: 1,
            status: "ready",
            bill_id: 1,
            business_id: 42,
            order_number: "O1",
            created_by: "",
            approved_by: "",
            notes: "",
            items: "[]",
            created_at: "",
            updated_at: "",
          },
        ],
        total: 1,
        page: 1,
        page_size: 100,
        total_pages: 1,
      },
    });

    const result = await getAllActiveOrders(42, {
      statuses: ["ready"],
      pageSize: 0,
    });

    expect(result.metadata).toEqual({
      total: 1,
      page: 1,
      page_size: 100,
      total_pages: 1,
    });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/orders?status=ready&page=1&page_size=100",
    );
  });

  it("applies maxRows as a global cap across active order statuses", async () => {
    (axiosInstance.get as jest.Mock)
      .mockResolvedValueOnce({
        data: {
          orders: [
            {
              id: 1,
              status: "ready",
              bill_id: 1,
              business_id: 42,
              order_number: "O1",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
            {
              id: 2,
              status: "ready",
              bill_id: 2,
              business_id: 42,
              order_number: "O2",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
          ],
          total: 4,
          page: 1,
          page_size: 2,
          total_pages: 2,
        },
      })
      .mockResolvedValueOnce({
        data: {
          orders: [
            {
              id: 3,
              status: "pending",
              bill_id: 3,
              business_id: 42,
              order_number: "O3",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
            {
              id: 4,
              status: "pending",
              bill_id: 4,
              business_id: 42,
              order_number: "O4",
              created_by: "",
              approved_by: "",
              notes: "",
              items: "[]",
              created_at: "",
              updated_at: "",
            },
          ],
          total: 4,
          page: 2,
          page_size: 2,
          total_pages: 2,
        },
      });

    const result = await getAllActiveOrders(42, {
      statuses: ["ready", "pending"],
      pageSize: 2,
      maxRows: 3,
    });

    expect(result.items.map((order) => order.id)).toEqual([1, 2, 3]);
    expect(result.capped).toBe(true);
    expect(result.warning).toBe("live_data_cap_reached");
    expect(result.metadata).toEqual({
      total: 4,
      page: 2,
      page_size: 2,
      total_pages: 2,
    });
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/42/orders?status=ready%2Cpending&page=1&page_size=2",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/42/orders?status=ready%2Cpending&page=2&page_size=2",
    );
  });

  it("caps the combined active order request by maxRows", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValueOnce({
      data: {
        orders: [],
        total: 0,
        page: 1,
        page_size: 4,
        total_pages: 1,
      },
    });

    await getAllActiveOrders(42, {
      statuses: ["ready", "pending"],
      pageSize: 10,
      maxRows: 4,
    });

    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/42/orders?status=ready%2Cpending&page=1&page_size=4",
    );
    expect(axiosInstance.get).toHaveBeenCalledTimes(1);
  });

  it("defaults the global active order cap to 1000 rows", async () => {
    const makeOrder = (id: number) => ({
      id,
      status: "ready" as const,
      bill_id: id,
      business_id: 42,
      order_number: `O${id}`,
      created_by: "",
      approved_by: "",
      notes: "",
      items: "[]",
      created_at: "",
      updated_at: "",
    });

    (axiosInstance.get as jest.Mock)
      .mockResolvedValueOnce({
        data: {
          orders: Array.from({ length: 600 }, (_, index) =>
            makeOrder(index + 1),
          ),
          total: 1001,
          page: 1,
          page_size: 100,
          total_pages: 2,
        },
      })
      .mockResolvedValueOnce({
        data: {
          orders: Array.from({ length: 401 }, (_, index) =>
            makeOrder(index + 601),
          ),
          total: 1001,
          page: 2,
          page_size: 100,
          total_pages: 2,
        },
      });

    const result = await getAllActiveOrders(42, {
      statuses: ["ready", "pending"],
    });

    expect(result.items).toHaveLength(1000);
    expect(result.items.at(-1)?.id).toBe(1000);
    expect(result.capped).toBe(true);
    expect(result.warning).toBe("live_data_cap_reached");
    expect(result.metadata).toEqual({
      total: 1001,
      page: 2,
      page_size: 100,
      total_pages: 2,
    });
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/42/orders?status=ready%2Cpending&page=1&page_size=100",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/42/orders?status=ready%2Cpending&page=2&page_size=100",
    );
  });

  it("sends X-Request-Id on operator order creation when provided (B-6)", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { order: { id: 1 } },
    });

    await createOrder(
      5,
      {
        bill_id: 7,
        items: [
          {
            menu_item_name: "Burger",
            menu_item_id: "burger",
            quantity: 1,
            price: asDollars(10),
          },
        ],
      },
      { idempotencyKey: "staff-order-retry-1" },
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/5/orders",
      expect.objectContaining({ bill_id: 7 }),
      expect.objectContaining({
        headers: { "X-Request-Id": "staff-order-retry-1" },
      }),
    );
  });

  it("omits the header when no idempotency key is provided", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { order: { id: 1 } },
    });

    await createOrder(5, { bill_id: 7, items: [] });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/5/orders",
      expect.objectContaining({ bill_id: 7 }),
      expect.objectContaining({ headers: undefined }),
    );
  });
});
