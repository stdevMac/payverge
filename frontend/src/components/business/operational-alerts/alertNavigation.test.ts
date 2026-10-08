import { alertTabSpec } from "./alertNavigation";
import type { OperationalAlert } from "@/api/operationalAlerts";

const base = {
  id: 1,
  business_id: 1,
  status: "open",
  priority: "normal",
  title: "t",
  body: "b",
  claimed_by_name: null,
  last_event_at: "",
  created_at: "",
  updated_at: "",
} as const;

function alert(over: Partial<OperationalAlert>): OperationalAlert {
  return {
    ...base,
    alert_type: "order_new",
    resource_type: "order",
    resource_id: 5,
    ...over,
  } as OperationalAlert;
}

describe("alertTabSpec", () => {
  it("focuses the exact bill for bill-resource alerts", () => {
    expect(
      alertTabSpec(
        alert({
          alert_type: "bill_new",
          resource_type: "bill",
          resource_id: 42,
        }),
      ),
    ).toBe("bills?billId=42");
  });

  it("never treats a webhook_event resource id as a bill id", () => {
    expect(
      alertTabSpec(
        alert({
          alert_type: "payment_refund_review",
          resource_type: "webhook_event",
          resource_id: 901,
        }),
      ),
    ).toBe("bills");
  });

  // F-cand-11: record-specific deep links when resource_id is present
  it("deep-links reservation alerts to reservationId", () => {
    expect(
      alertTabSpec(
        alert({
          alert_type: "reservation_new",
          resource_type: "reservation",
          resource_id: 77,
        }),
      ),
    ).toBe("reservations?reservationId=77");
  });

  it("deep-links service_call to tables?tableId=", () => {
    expect(
      alertTabSpec(
        alert({
          alert_type: "service_call",
          resource_type: "table",
          resource_id: 12,
        }),
      ),
    ).toBe("tables?tableId=12");
  });

  it("deep-links delivery_new to delivery dispatch sub-tab", () => {
    expect(
      alertTabSpec(alert({ alert_type: "delivery_new", resource_id: 9 })),
    ).toBe("delivery?sub=dispatch");
  });

  it("deep-links order/payment alerts with resource_id to billId when typed as bill", () => {
    expect(
      alertTabSpec(
        alert({
          alert_type: "payment_received",
          resource_type: "bill",
          resource_id: 42,
        }),
      ),
    ).toBe("bills?billId=42");
  });

  it.each([
    ["order_new", "bills?billId=5"],
    ["kitchen_order_ready", "kitchen"],
    ["reservation_new", "reservations?reservationId=5"],
    ["reservation_approval", "reservations?reservationId=5"],
    ["delivery_new", "delivery?sub=dispatch"],
    ["payment_received", "bills?billId=5"],
    ["payment_refund_review", "bills?billId=5"],
    ["service_call", "tables?tableId=5"],
    ["ai_takeover", "ai-waiter"],
  ] as const)("%s → %s (with default resource_id)", (type, tab) => {
    expect(alertTabSpec(alert({ alert_type: type }))).toBe(tab);
  });

  it("falls back to bare rail when resource_id is missing", () => {
    expect(
      alertTabSpec(
        alert({ alert_type: "reservation_new", resource_id: null as never }),
      ),
    ).toBe("reservations");
    expect(
      alertTabSpec(
        alert({ alert_type: "service_call", resource_id: null as never }),
      ),
    ).toBe("tables");
  });

  it("returns null for unknown types", () => {
    expect(
      alertTabSpec(alert({ alert_type: "future_type" as never })),
    ).toBeNull();
  });
});
