import type { OperationalAlert } from "@/api/operationalAlerts";
import { localizeAlert } from "./operationalAlertCopy";

function alert(
  alertType: OperationalAlert["alert_type"],
  overrides: Partial<OperationalAlert> = {},
): OperationalAlert {
  return {
    id: 1,
    business_id: 42,
    alert_type: alertType,
    resource_type: "bill",
    resource_id: 9,
    status: "open",
    priority: "high",
    title: "backend fallback",
    body: "backend fallback body",
    claimed_by_name: null,
    last_event_at: "2026-08-01T10:00:00Z",
    created_at: "2026-08-01T10:00:00Z",
    updated_at: "2026-08-01T10:00:00Z",
    metadata: { bill_number: "B-9" },
    ...overrides,
  };
}

describe("C10 operational alert wording contracts", () => {
  it("never calls a pending payment request received", () => {
    expect(localizeAlert(alert("payment_requested"), "en")).toEqual({
      title: "Payment requested",
      body: "Payment requested for bill #B-9",
    });
    expect(localizeAlert(alert("payment_received"), "en")).toEqual({
      title: "Payment received",
      body: "Payment received for bill #B-9",
    });
    expect(
      localizeAlert(
        alert("payment_received", {
          title: "Demo payment received",
          body: "B75-aff0f207-af2 was paid.",
          metadata: { demo: true },
        }),
        "en",
      ),
    ).toEqual({
      title: "Payment received",
      body: "B75-aff0f207-af2 was paid.",
    });
  });

  it("surfaces an exhausted scheduled-report delivery as a failure needing review", () => {
    const copy = localizeAlert(
      alert("report_delivery_failed", {
        resource_type: "report_delivery",
        metadata: { schedule_id: 7 },
      }),
      "en",
    );
    expect(copy.title).toBe("Scheduled report delivery failed");
    expect(copy.body).toMatch(/needs review/i);
  });
});
