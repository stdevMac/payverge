/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import TimeOffRequests, { type TimeOffRequestsLabels } from "./TimeOffRequests";
import { availabilityApi } from "@/api/availability";
import type { StaffData } from "@/utils/staffAuth";

jest.mock("@/api/availability", () => ({
  availabilityApi: { listTimeOff: jest.fn(), createTimeOff: jest.fn() },
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
// Avoid opening a real EventSource in jsdom.
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

const mocked = availabilityApi as unknown as {
  listTimeOff: jest.Mock;
  createTimeOff: jest.Mock;
};

const staff: StaffData = {
  id: 7,
  name: "Dana",
  email: "dana@example.com",
  role: "server",
  business_id: 42,
  business_name: "Cafe 42",
  is_active: true,
};

const labels: TimeOffRequestsLabels = {
  title: "Time off",
  subtitle: "Request days off and track your requests",
  loading: "Loading your requests",
  error: "Could not load requests",
  emptyTitle: "No time-off requests",
  emptySubtitle: "When you request time off, it'll show up here",
  formTitle: "Request time off",
  startLabel: "From",
  endLabel: "To",
  reasonLabel: "Reason",
  reasonPlaceholder: "Add a short note",
  submit: "Submit request",
  submitting: "Submitting",
  submitError: "Couldn't submit",
  invalidRange: "End must be after start",
  statusPending: "Pending",
  statusApproved: "Approved",
  statusDenied: "Denied",
  statusCancelled: "Cancelled",
};

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <TimeOffRequests staff={staff} labels={labels} locale="en" />
    </QueryClientProvider>,
  );
}

beforeEach(() => jest.clearAllMocks());

describe("TimeOffRequests", () => {
  it("renders the staff member's own requests with status chips (no dollars)", async () => {
    mocked.listTimeOff.mockResolvedValue([
      {
        id: 9,
        business_id: 42,
        staff_id: 7,
        starts_at: "2026-07-01T13:00:00Z",
        ends_at: "2026-07-02T13:00:00Z",
        reason: "Family trip",
        status: "pending",
        decided_by_staff_id: null,
        decided_at: null,
        created_at: "",
        updated_at: "",
      },
      {
        id: 10,
        business_id: 42,
        staff_id: 7,
        starts_at: "2026-06-10T13:00:00Z",
        ends_at: "2026-06-11T13:00:00Z",
        reason: "Doctor",
        status: "approved",
        decided_by_staff_id: 1,
        decided_at: "2026-06-09T00:00:00Z",
        created_at: "",
        updated_at: "",
      },
    ]);

    renderView();

    await waitFor(() => expect(screen.getByText("Family trip")).toBeInTheDocument());
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.getByText("Approved")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("shows the empty state when there are no requests", async () => {
    mocked.listTimeOff.mockResolvedValue([]);
    renderView();

    await waitFor(() =>
      expect(screen.getByText("No time-off requests")).toBeInTheDocument(),
    );
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("submits a new request via createTimeOff", async () => {
    mocked.listTimeOff.mockResolvedValue([]);
    mocked.createTimeOff.mockResolvedValue({ id: 11, status: "pending" });
    renderView();

    await waitFor(() =>
      expect(screen.getByText("No time-off requests")).toBeInTheDocument(),
    );

    const startVal = "2026-07-01T09:00";
    const endVal = "2026-07-02T09:00";
    fireEvent.change(screen.getByLabelText("From"), { target: { value: startVal } });
    fireEvent.change(screen.getByLabelText("To"), { target: { value: endVal } });
    fireEvent.change(screen.getByLabelText("Reason"), { target: { value: "Vacation" } });

    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));

    await waitFor(() =>
      expect(mocked.createTimeOff).toHaveBeenCalledWith("42", {
        starts_at: new Date(startVal).toISOString(),
        ends_at: new Date(endVal).toISOString(),
        reason: "Vacation",
      }),
    );
  });
});
