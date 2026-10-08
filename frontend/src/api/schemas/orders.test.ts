import { validateOrdersResponse } from "@/api/schemas/orders";
import { logError } from "@/utils/errorLogger";

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

describe("orders schema validation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("accepts nullable order metadata returned by the backend", () => {
    const response = {
      orders: [
        {
          id: 1,
          bill_id: 10,
          business_id: 42,
          order_number: "O-nullable",
          status: "pending",
          created_by: "guest",
          approved_by: "",
          cancelled_by: null,
          notes: "",
          items: "[]",
          currency: "USD",
          cancel_reason: null,
          created_at: "2026-05-04T12:00:00Z",
          updated_at: "2026-05-04T12:00:00Z",
          approved_at: null,
          cancelled_at: null,
          table_id: null,
        },
      ],
      total: 1,
      page: 1,
      page_size: 100,
      total_pages: 1,
    };

    expect(validateOrdersResponse(response)).toBe(response);
    expect(logError).not.toHaveBeenCalled();
  });

  it("accepts numeric table ids returned by table-attached backend orders", () => {
    const response = {
      orders: [
        {
          id: 2,
          bill_id: 11,
          business_id: 42,
          order_number: "O-table",
          status: "approved",
          created_by: "guest",
          approved_by: "owner",
          cancelled_by: null,
          notes: "",
          items: "[]",
          currency: "AED",
          cancel_reason: null,
          created_at: "2026-05-04T12:00:00Z",
          updated_at: "2026-05-04T12:00:00Z",
          approved_at: "2026-05-04T12:05:00Z",
          cancelled_at: null,
          table_id: 7,
        },
      ],
      total: 1,
    };

    expect(validateOrdersResponse(response)).toBe(response);
    expect(logError).not.toHaveBeenCalled();
  });

  it("accepts order items as a line array (#771)", () => {
    const response = {
      orders: [
        {
          id: 1128,
          bill_id: 761,
          business_id: 86,
          order_number: "K-1128",
          status: "in_kitchen",
          created_by: "guest",
          approved_by: "owner",
          cancelled_by: null,
          notes: "",
          items: [
            { menu_item_name: "Steak", quantity: 1 },
            { menu_item_name: "Wine", quantity: 1 },
          ],
          currency: "USD",
          cancel_reason: null,
          created_at: "2026-05-04T12:00:00Z",
          updated_at: "2026-05-04T12:00:00Z",
          approved_at: "2026-05-04T12:05:00Z",
          cancelled_at: null,
        },
      ],
      total: 1,
    };

    expect(validateOrdersResponse(response)).toBe(response);
    expect(logError).not.toHaveBeenCalled();
  });
});
