/** @jest-environment jsdom */
import React from "react";
import {
  act,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import ServiceCallsQueue from "./ServiceCallsQueue";
import type { OperationalAlert } from "@/api/operationalAlerts";

const mockClaimAlert = jest.fn().mockResolvedValue(undefined);
const mockResolveAlert = jest.fn().mockResolvedValue(undefined);

const mockState: { alerts: OperationalAlert[] | null } = { alerts: null };

jest.mock("@/api/staff", () => ({
  getBusinessStaff: jest.fn(async () => ({
    staff: [
      {
        id: 19,
        name: "Dana",
        email: "dana@example.com",
        role: "server",
        business_id: 1,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ],
    pending_invitations: [],
  })),
}));

jest.mock("../operational-alerts/useOperationalAlerts", () => ({
  useOptionalOperationalAlerts: () => {
    if (!mockState.alerts) return null;
    return {
      alerts: mockState.alerts,
      claimAlert: mockClaimAlert,
      resolveAlert: mockResolveAlert,
    };
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "businessDashboard.dashboard.tableManager.serviceCalls.title":
        "Open service calls",
      "businessDashboard.dashboard.tableManager.serviceCalls.acknowledge":
        "Acknowledge",
      "businessDashboard.dashboard.tableManager.serviceCalls.assign": "Assign",
      "businessDashboard.dashboard.tableManager.serviceCalls.assignTo":
        "Assign to server",
      "businessDashboard.dashboard.tableManager.serviceCalls.assignMe":
        "I'll take it",
      "businessDashboard.dashboard.tableManager.serviceCalls.markHandled":
        "Mark handled",
      "businessDashboard.dashboard.tableManager.serviceCalls.updating":
        "Updating…",
      "businessDashboard.dashboard.tableManager.serviceCalls.updateFailed":
        "Could not update this call. Try again.",
      "businessDashboard.dashboard.tableManager.serviceCalls.justNow":
        "Just now",
      "businessDashboard.dashboard.tableManager.serviceCalls.status.open":
        "Open",
      "businessDashboard.dashboard.tableManager.serviceCalls.status.claimed":
        "Claimed",
    };
    return map[key] || key;
  },
}));

function makeAlert(
  overrides: Partial<OperationalAlert> & { id: number },
): OperationalAlert {
  const { id, ...rest } = overrides;
  return {
    id,
    business_id: 1,
    alert_type: "service_call",
    title: "Service call",
    body: "help",
    status: "open",
    priority: "urgent",
    resource_type: "table",
    resource_id: 2,
    claimed_by_name: null,
    last_event_at: new Date().toISOString(),
    metadata: { table_name: "Table 2", reason: "water" },
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...rest,
  };
}

describe("ServiceCallsQueue (PV-LIVE-20260720-009)", () => {
  beforeEach(() => {
    mockState.alerts = null;
    mockClaimAlert.mockClear();
    mockResolveAlert.mockClear();
  });

  it("renders nothing when there are no active service calls", async () => {
    mockState.alerts = null;
    const { container } = render(<ServiceCallsQueue businessId={1} />);
    await act(async () => {
      await Promise.resolve();
    });
    expect(container).toBeEmptyDOMElement();
  });

  it("lists open service calls with table and reason", async () => {
    mockState.alerts = [
      makeAlert({ id: 11 }),
      makeAlert({
        id: 12,
        status: "claimed",
        metadata: { table_name: "Table 5", reason: "check" },
      }),
    ];
    render(<ServiceCallsQueue businessId={1} />);
    await screen.findByTestId("service-call-assign-11");
    expect(screen.getByTestId("service-calls-queue")).toBeInTheDocument();
    expect(screen.getByText(/Table 2/)).toBeInTheDocument();
    expect(screen.getByText(/Water/)).toBeInTheDocument();
    expect(screen.getByText(/Table 5/)).toBeInTheDocument();
    expect(screen.getByText(/Check, please/)).toBeInTheDocument();
    expect(screen.getByText("Acknowledge")).toBeInTheDocument();
    expect(screen.getByText("Mark handled")).toBeInTheDocument();
  });

  it("claims an open call and resolves a claimed call", async () => {
    mockState.alerts = [
      makeAlert({ id: 21, status: "open" }),
      makeAlert({
        id: 22,
        status: "claimed",
        metadata: { table_name: "Table 3", reason: "order" },
      }),
    ];
    render(<ServiceCallsQueue businessId={1} />);
    await screen.findByTestId("service-call-assign-21");

    fireEvent.click(screen.getByText("Acknowledge"));
    await waitFor(() =>
      expect(mockClaimAlert).toHaveBeenCalledWith(21, "tables_queue"),
    );

    fireEvent.click(screen.getByText("Mark handled"));
    await waitFor(() =>
      expect(mockResolveAlert).toHaveBeenCalledWith(22, "handled"),
    );
  });

  it("hides service calls older than the seating SLA", async () => {
    const dayAgo = new Date(Date.now() - 26 * 60 * 60_000).toISOString();
    mockState.alerts = [
      makeAlert({
        id: 41,
        last_event_at: dayAgo,
        created_at: dayAgo,
        metadata: { table_name: "Table 1", reason: "water" },
      }),
      makeAlert({
        id: 42,
        metadata: { table_name: "Table 4", reason: "order" },
      }),
    ];
    render(<ServiceCallsQueue businessId={1} />);
    await screen.findByTestId("service-call-row-42");
    expect(screen.queryByTestId("service-call-row-41")).toBeNull();
    expect(screen.queryByText(/Table 1/)).toBeNull();
    expect(screen.getByText(/Table 4/)).toBeInTheDocument();
  });

  it("keeps a day-old unpaid check-please the API still returned", async () => {
    const dayAgo = new Date(Date.now() - 26 * 60 * 60_000).toISOString();
    mockState.alerts = [
      makeAlert({
        id: 51,
        last_event_at: dayAgo,
        created_at: dayAgo,
        metadata: { table_name: "Table 7", reason: "check" },
      }),
    ];
    render(<ServiceCallsQueue businessId={1} />);
    await screen.findByTestId("service-call-row-51");
    expect(screen.getByText(/Table 7/)).toBeInTheDocument();
    expect(screen.getByText(/Check, please/)).toBeInTheDocument();
  });

  it("routes an open call to a named server", async () => {
    mockState.alerts = [makeAlert({ id: 31, status: "open" })];
    render(<ServiceCallsQueue businessId={1} />);

    const assign = await screen.findByTestId("service-call-assign-31");
    fireEvent.change(assign, { target: { value: "19" } });
    fireEvent.click(screen.getByText("Assign"));
    await waitFor(() =>
      expect(mockClaimAlert).toHaveBeenCalledWith(31, "tables_queue", 19),
    );
  });
});
