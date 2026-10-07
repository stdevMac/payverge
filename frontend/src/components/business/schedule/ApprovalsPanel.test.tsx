/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ApprovalsPanel from "./ApprovalsPanel";
import { availabilityApi } from "@/api/availability";
import { coverageApi } from "@/api/coverage";
import { positionsApi } from "@/api/positions";
import { getBusinessStaff } from "@/api/staff";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/availability", () => ({
  availabilityApi: { listTimeOff: jest.fn(), decide: jest.fn() },
}));
jest.mock("@/api/coverage", () => ({
  coverageApi: { listOpen: jest.fn(), decide: jest.fn(), history: jest.fn() },
}));
jest.mock("@/api/positions", () => ({
  positionsApi: { list: jest.fn() },
}));
jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

const mocked = availabilityApi as unknown as { listTimeOff: jest.Mock; decide: jest.Mock };
const mockedCoverage = coverageApi as unknown as {
  listOpen: jest.Mock;
  decide: jest.Mock;
  history: jest.Mock;
};
const mockedPositions = positionsApi as unknown as { list: jest.Mock };
const mockedStaff = getBusinessStaff as unknown as jest.Mock;

const emptyCoverage = {
  open_shifts: [],
  swap_inbox: [],
  pending_approvals: { swaps: [], claims: [] },
};

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ApprovalsPanel businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedStaff.mockResolvedValue({
    staff: [{ id: 7, name: "Dana", email: "dana@example.com", role: "server", business_id: 42, created_at: "", updated_at: "" }],
    pending_invitations: [],
  });
  mockedCoverage.listOpen.mockResolvedValue(emptyCoverage);
  mockedCoverage.history.mockResolvedValue([]);
  mockedPositions.list.mockResolvedValue([
    { id: 9, business_id: 42, name: "Server", color_hex: "#1a6b6a", department: "FOH", is_active: true, sort_order: 0 },
  ]);
});

const futureIso = (hoursFromNow: number) =>
  new Date(Date.now() + hoursFromNow * 3600_000).toISOString();

describe("ApprovalsPanel L5-35 stale + count cap", () => {
  it("marks past-dated time-off rows with a stale badge and hides Approve/Deny", async () => {
    mocked.listTimeOff.mockResolvedValue([
      {
        id: 1,
        staff_id: 7,
        business_id: 42,
        starts_at: "2020-01-01T00:00:00Z",
        ends_at: "2020-01-02T00:00:00Z",
        reason: "old vacation",
        status: "pending",
      },
    ]);
    renderPanel();
    expect(await screen.findByText("Dana")).toBeInTheDocument();
    const staleRow = document.querySelector("[data-stale]");
    expect(staleRow).toBeTruthy();
    expect(
      screen.getByText("dashboardApprovals.staleBadge"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /dashboardApprovals\.approve/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /dashboardApprovals\.deny/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("dashboardApprovals.pendingBadge"),
    ).not.toBeInTheDocument();
  });

  it("counts only live requests in the pending badge when stale rows exist", async () => {
    mocked.listTimeOff.mockResolvedValue([
      {
        id: 1,
        staff_id: 7,
        business_id: 42,
        starts_at: "2020-01-01T00:00:00Z",
        ends_at: "2020-01-02T00:00:00Z",
        reason: "old vacation",
        status: "pending",
      },
      {
        id: 2,
        staff_id: 7,
        business_id: 42,
        starts_at: futureIso(24),
        ends_at: futureIso(48),
        reason: "upcoming",
        status: "pending",
      },
    ]);
    renderPanel();
    expect(await screen.findByText("upcoming")).toBeInTheDocument();
    expect(screen.getByText("dashboardApprovals.pendingBadge")).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: /dashboardApprovals\.approve/ }),
    ).toHaveLength(1);
  });
});

describe("ApprovalsPanel", () => {
  it("renders pending requests and approving one calls availability.decide", async () => {
    mocked.listTimeOff.mockResolvedValue([
      {
        id: 9,
        business_id: 42,
        staff_id: 7,
        starts_at: futureIso(24),
        ends_at: futureIso(48),
        reason: "Family trip",
        status: "pending",
        decided_by_staff_id: null,
        decided_at: null,
        created_at: "",
        updated_at: "",
      },
    ]);
    mocked.decide.mockResolvedValue({ id: 9, status: "approved" });

    renderPanel();

    // Pending row renders the requester name resolved from the staff list.
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    expect(screen.getByText("Family trip")).toBeInTheDocument();
    // Server-scoped pending query.
    expect(mocked.listTimeOff).toHaveBeenCalledWith("42", "pending");

    // With no coverage items, only the single time-off approve button exists.
    await userEvent.click(screen.getByRole("button", { name: /dashboardApprovals\.approve/ }));
    await waitFor(() =>
      expect(mocked.decide).toHaveBeenCalledWith("42", 9, true, ""),
    );
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
    // No money on the approvals surface.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("shows one consolidated empty state when nothing needs approval", async () => {
    mocked.listTimeOff.mockResolvedValue([]);
    renderPanel();

    await waitFor(() =>
      expect(screen.getByText("dashboardApprovals.emptyTitle")).toBeInTheDocument(),
    );
    // The old bare "no coverage requests" line + its subsection header are gone;
    // an empty inbox now shows a single styled empty state, not two stacked.
    expect(screen.queryByText("dashboardApprovals.emptyCoverage")).not.toBeInTheDocument();
    expect(screen.queryByText("dashboardApprovals.coverageTitle")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("renders coverage rows and decides them with the correct kind discriminator", async () => {
    mocked.listTimeOff.mockResolvedValue([]);
    mockedCoverage.listOpen.mockResolvedValue({
      open_shifts: [],
      swap_inbox: [],
      pending_approvals: {
        swaps: [
          {
            id: 5,
            shift_id: 12,
            position_id: 9,
            kind: "swap",
            status: "accepted",
            requesting_staff_id: 3,
            accepting_staff_id: 4,
            claiming_staff_id: null,
            starts_at: futureIso(24),
            ends_at: futureIso(30),
          },
        ],
        claims: [
          {
            id: 7,
            shift_id: 13,
            position_id: 9,
            kind: "open_claim",
            status: "pending",
            requesting_staff_id: 0,
            accepting_staff_id: null,
            claiming_staff_id: 8,
            starts_at: futureIso(48),
            ends_at: futureIso(54),
          },
        ],
      },
    });
    mockedCoverage.decide.mockResolvedValue({ id: 5, status: "approved" });

    renderPanel();

    // Both coverage rows render under the Coverage subsection.
    const swapRow = (await screen.findByText("dashboardApprovals.swapRow")).closest("li");
    const claimRow = screen.getByText("dashboardApprovals.claimRow").closest("li");
    expect(swapRow).not.toBeNull();
    expect(claimRow).not.toBeNull();

    // Badge folds coverage into the count even with zero time-off pending.
    expect(screen.getByText("dashboardApprovals.pendingBadge")).toBeInTheDocument();

    // The section explains itself in plain language.
    expect(
      screen.getByText("dashboardApprovals.coverageHint"),
    ).toBeInTheDocument();

    // Approving the swap row passes kind="swap" + the swap id.
    await userEvent.click(
      within(swapRow as HTMLElement).getByRole("button", { name: /dashboardApprovals\.approve/ }),
    );
    await waitFor(() =>
      expect(mockedCoverage.decide).toHaveBeenCalledWith("42", 5, { kind: "swap", decision: "approve" }),
    );

    // Denying the claim row passes kind="open_claim" against the same route.
    await userEvent.click(
      within(claimRow as HTMLElement).getByRole("button", { name: /dashboardApprovals\.deny/ }),
    );
    await waitFor(() =>
      expect(mockedCoverage.decide).toHaveBeenCalledWith("42", 7, { kind: "open_claim", decision: "deny" }),
    );

    // Still no money on the approvals surface.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("shows resolved coverage history collapsed, expanding to authored rows", async () => {
    mocked.listTimeOff.mockResolvedValue([]);
    mockedCoverage.history.mockResolvedValue([
      {
        kind: "swap",
        request_id: 5,
        shift_id: 12,
        position_id: 9,
        starts_at: "2026-07-03T16:00:00Z",
        ends_at: "2026-07-03T22:00:00Z",
        status: "approved",
        requester_staff_id: 7,
        decider_staff_id: 7,
        resolved_at: "2026-07-03T12:00:00Z",
      },
    ]);

    renderPanel();

    // The history toggle appears (count 1); rows are collapsed until opened.
    const toggle = await screen.findByRole("button", { name: /dashboardApprovals\.historyTitle/ });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("dashboardApprovals.historyStatus.approved")).not.toBeInTheDocument();

    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    // Resolved row: requester name + status chip + "decided by" line.
    expect(screen.getByText("dashboardApprovals.historyStatus.approved")).toBeInTheDocument();
    expect(screen.getByText("dashboardApprovals.historyDecidedBy")).toBeInTheDocument();
    expect(mockedCoverage.history).toHaveBeenCalledWith("42");
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("hides the history subsection when there is no resolved history", async () => {
    mocked.listTimeOff.mockResolvedValue([]);
    mockedCoverage.history.mockResolvedValue([]);
    renderPanel();
    await waitFor(() =>
      expect(screen.getByText("dashboardApprovals.emptyTitle")).toBeInTheDocument(),
    );
    expect(
      screen.queryByRole("button", { name: /dashboardApprovals\.historyTitle/ }),
    ).not.toBeInTheDocument();
  });
});
