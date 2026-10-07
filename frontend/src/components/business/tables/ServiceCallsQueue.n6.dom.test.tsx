/**
 * N-6 age labels still render for live calls. Day-old waiter calls leave
 * the live queue (#729 seating SLA) instead of showing "26d ago".
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import ServiceCallsQueue from "./ServiceCallsQueue";

jest.mock("@/api/staff", () => ({
  getBusinessStaff: jest.fn(async () => ({
    staff: [],
    pending_invitations: [],
  })),
}));

const NOW_MS = Date.parse("2026-08-06T12:00:00Z");

jest.mock("../operational-alerts/useOperationalAlerts", () => ({
  useOptionalOperationalAlerts: () => ({
    alerts: [
      {
        id: 99,
        business_id: 1,
        alert_type: "service_call",
        status: "open",
        priority: "normal",
        title: "Service call",
        body: "",
        last_event_at: "2026-07-11T12:00:00Z",
        created_at: "2026-07-11T12:00:00Z",
        updated_at: "2026-07-11T12:00:00Z",
        metadata: { table_name: "Mesa 3", reason: "water" },
      },
      {
        id: 100,
        business_id: 1,
        alert_type: "service_call",
        status: "open",
        priority: "normal",
        title: "Service call",
        body: "",
        last_event_at: "2026-08-06T11:20:00Z",
        created_at: "2026-08-06T11:20:00Z",
        updated_at: "2026-08-06T11:20:00Z",
        metadata: { table_name: "Mesa 4", reason: "order" },
      },
    ],
    claimAlert: jest.fn(),
    resolveAlert: jest.fn(),
  }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: (
      key: string,
      _locale?: string,
      params?: Record<string, string | number>,
    ) => {
      if (key.endsWith("serviceCalls.justNow")) return "Just now";
      if (key.endsWith("serviceCalls.elapsedAgo")) {
        return `${params?.duration ?? ""} ago`;
      }
      if (key.endsWith("serviceCalls.title")) return "Service calls";
      if (key.endsWith("serviceCalls.status.open")) return "Open";
      if (key.endsWith("serviceCalls.acknowledge")) return "Acknowledge";
      if (key.endsWith("serviceCalls.markHandled")) return "Handled";
      if (key.endsWith("serviceCalls.updating")) return "Updating";
      return key;
    },
  };
});

describe("N-6 ServiceCallsQueue DOM age labels", () => {
  beforeEach(() => {
    jest.spyOn(Date, "now").mockReturnValue(NOW_MS);
  });
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("drops a 26-day-old call and still humanizes a live 40m age", async () => {
    render(<ServiceCallsQueue businessId={1} />);
    await waitFor(() =>
      expect(screen.getByTestId("service-call-row-100")).toBeInTheDocument(),
    );

    expect(screen.queryByTestId("service-call-row-99")).toBeNull();
    const row = screen.getByTestId("service-call-row-100");
    expect(row.textContent).toMatch(/40m ago/);
    expect(row.textContent).not.toMatch(/26d/);
    expect(row.textContent).not.toMatch(/648/);
  });
});
