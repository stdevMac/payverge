import type { OperationalAlert } from "@/api/operationalAlerts";

/**
 * Wave 4: maps an operational alert to the dashboard tab spec that resolves it
 * (consumed by handleSetActiveTab, which enforces staff tab permissions).
 * Bill-resource alerts focus the exact bill via the bills?billId deep-link.
 * Unknown types return null so the row stays non-interactive rather than
 * navigating somewhere wrong.
 */
export function alertTabSpec(alert: OperationalAlert): string | null {
  // Prefer resource-typed deep links when we have a concrete id (F-cand-11).
  // Bill is special-cased first because several alert_types share resource_type
  // "bill" and must open the exact bill, not the bare rail.
  if (alert.resource_type === "bill" && alert.resource_id != null) {
    return `bills?billId=${alert.resource_id}`;
  }
  // A webhook_event alert names a provider event with no bill: its resource_id
  // is a webhook_events row, never a bill id, so open the bills rail bare.
  if (alert.resource_type === "webhook_event") {
    return "bills";
  }
  if (alert.resource_type === "reservation" && alert.resource_id != null) {
    return `reservations?reservationId=${alert.resource_id}`;
  }
  if (alert.resource_type === "table" && alert.resource_id != null) {
    return `tables?tableId=${alert.resource_id}`;
  }
  if (alert.resource_type === "delivery" && alert.resource_id != null) {
    // Delivery deep-link: land on dispatch with the order context when possible.
    return `delivery?sub=dispatch`;
  }

  switch (alert.alert_type) {
    case "order_new":
    case "bill_new":
    case "payment_requested":
    case "payment_received":
    case "payment_refund_review":
      return alert.resource_id != null
        ? `bills?billId=${alert.resource_id}`
        : "bills";
    case "kitchen_order_ready":
      return "kitchen";
    case "reservation_new":
    case "reservation_approval":
      return alert.resource_id != null
        ? `reservations?reservationId=${alert.resource_id}`
        : "reservations";
    case "delivery_new":
      return "delivery?sub=dispatch";
    case "service_call":
      return alert.resource_id != null
        ? `tables?tableId=${alert.resource_id}`
        : "tables";
    case "ai_takeover":
      return "ai-waiter";
    default:
      return null;
  }
}
