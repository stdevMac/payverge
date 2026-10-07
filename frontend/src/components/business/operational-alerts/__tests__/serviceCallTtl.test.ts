import { isStaleServiceCall, SERVICE_CALL_TTL_MS } from "../serviceCallTtl";

describe("isStaleServiceCall", () => {
  const now = Date.parse("2026-08-21T12:00:00Z");

  it("keeps a call inside the seating SLA", () => {
    expect(
      isStaleServiceCall(
        {
          alert_type: "service_call",
          status: "open",
          last_event_at: new Date(now - 30 * 60_000).toISOString(),
          created_at: new Date(now - 30 * 60_000).toISOString(),
        },
        now,
      ),
    ).toBe(false);
  });

  it("marks an open water call at the 90-minute SLA as stale", () => {
    expect(SERVICE_CALL_TTL_MS).toBe(90 * 60 * 1000);
    expect(
      isStaleServiceCall(
        {
          alert_type: "service_call",
          status: "open",
          last_event_at: new Date(now - SERVICE_CALL_TTL_MS).toISOString(),
          created_at: new Date(now - SERVICE_CALL_TTL_MS).toISOString(),
          metadata: { reason: "water" },
        },
        now,
      ),
    ).toBe(true);
  });

  it("keeps claimed/assigned calls past the SLA", () => {
    expect(
      isStaleServiceCall(
        {
          alert_type: "service_call",
          status: "claimed",
          last_event_at: new Date(now - SERVICE_CALL_TTL_MS).toISOString(),
          created_at: new Date(now - SERVICE_CALL_TTL_MS).toISOString(),
          metadata: { reason: "water" },
        },
        now,
      ),
    ).toBe(false);
  });

  it("keeps API-returned check-please past the SLA (occupancy is server-side)", () => {
    expect(
      isStaleServiceCall(
        {
          alert_type: "service_call",
          status: "open",
          last_event_at: new Date(now - 26 * 60 * 60_000).toISOString(),
          created_at: new Date(now - 26 * 60 * 60_000).toISOString(),
          metadata: { reason: "check" },
        },
        now,
      ),
    ).toBe(false);
  });

  it("does not treat a missing last_event_at as stale", () => {
    expect(
      isStaleServiceCall(
        {
          alert_type: "service_call",
          status: "open",
          last_event_at: "",
          created_at: new Date(now - 26 * 60 * 60_000).toISOString(),
          metadata: { reason: "water" },
        },
        now,
      ),
    ).toBe(false);
  });

  it("ignores non-service-call alerts even when old", () => {
    expect(
      isStaleServiceCall(
        {
          alert_type: "order_new",
          status: "open",
          last_event_at: new Date(now - 26 * 60 * 60_000).toISOString(),
          created_at: new Date(now - 26 * 60 * 60_000).toISOString(),
        },
        now,
      ),
    ).toBe(false);
  });
});
