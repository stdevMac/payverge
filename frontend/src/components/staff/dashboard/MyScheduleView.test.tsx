/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import MyScheduleView, { type MyScheduleViewLabels } from "./MyScheduleView";
import { scheduleApi } from "@/api/schedule";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import { positionsApi } from "@/api/positions";
import { coverageApi } from "@/api/coverage";
import type { StaffData } from "@/utils/staffAuth";

jest.mock("@/api/schedule", () => ({ scheduleApi: { get: jest.fn() } }));
jest.mock("@/api/scheduleSettings", () => ({ scheduleSettingsApi: { get: jest.fn() } }));
jest.mock("@/api/positions", () => ({ positionsApi: { list: jest.fn() } }));
jest.mock("@/api/coverage", () => ({
  coverageApi: { listMine: jest.fn(), requestSwap: jest.fn() },
}));
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
// Mock the realtime hook so the test never opens an EventSource (jsdom has none).
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

const mockedSchedule = scheduleApi as unknown as { get: jest.Mock };
const mockedSettings = scheduleSettingsApi as unknown as { get: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };
const mockedCoverage = coverageApi as unknown as {
  listMine: jest.Mock;
  requestSwap: jest.Mock;
};

// A shift comfortably in the future so `isFuture` is true whenever the suite runs.
const FUTURE_START = "2030-01-15T17:00:00Z";
const FUTURE_END = "2030-01-15T23:00:00Z";

function futureShift(overrides: Record<string, unknown> = {}) {
  return {
    id: 21,
    business_id: 42,
    schedule_id: 1,
    staff_id: 7,
    position_id: 3,
    starts_at: FUTURE_START,
    ends_at: FUTURE_END,
    break_minutes: 0,
    status: "filled",
    published: true,
    notes: "",
    created_by_staff_id: 1,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

const staff: StaffData = {
  id: 7,
  name: "Dana",
  email: "dana@example.com",
  role: "server",
  business_id: 42,
  business_name: "Cafe 42",
  business_slug: "cafe-42",
  is_active: true,
};

const labels: MyScheduleViewLabels = {
  title: "Your week",
  subtitle: "Your published shifts",
  loading: "Loading your schedule",
  error: "Could not load your schedule",
  emptyTitle: "No shifts this week",
  emptySubtitle: "Nothing published yet",
  hoursUnit: "h",
  minutesUnit: "m",
  breakTemplate: "{minutes}m break",
  positionFallback: "Shift",
  offerAction: "Offer shift",
  requested: "Requested",
  thisWeek: "This week",
  nextWeek: "Next week",
  offerSuccess: "Offer sent",
  giveUpSuccess: "Shift given up",
  actionError: "Something went wrong",
  offer: {
    title: "Offer this shift",
    subtitle: "Two ways to pass this shift.",
    offerCover: "Offer for a teammate to cover",
    offerCoverHint: "A teammate accepts, then your manager approves.",
    giveUp: "Give up the shift",
    giveUpHint: "Your manager finds cover.",
    cancel: "Cancel",
    close: "Close",
  },
};

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MyScheduleView staff={staff} labels={labels} locale="en" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedSettings.get.mockResolvedValue({ week_start_day: 1 });
  mockedPositions.list.mockResolvedValue([
    {
      id: 3,
      business_id: 42,
      name: "Server",
      color_hex: "#1a6b6a",
      department: "FOH",
      is_active: true,
      sort_order: 0,
    },
  ]);
  mockedCoverage.listMine.mockResolvedValue({ claims: [], swaps: [] });
});

describe("MyScheduleView", () => {
  it("renders the staff member's own shifts (hours only, no dollars)", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 1, status: "published" },
      shifts: [
        {
          id: 11,
          business_id: 42,
          schedule_id: 1,
          staff_id: 7,
          position_id: 3,
          starts_at: "2026-06-29T17:00:00Z",
          ends_at: "2026-06-29T23:00:00Z",
          break_minutes: 30,
          status: "filled",
          published: true,
          notes: "",
          created_by_staff_id: 1,
          created_at: "",
          updated_at: "",
        },
        // Open shift (unassigned) — must NOT show in "your week".
        {
          id: 12,
          business_id: 42,
          schedule_id: 1,
          staff_id: null,
          position_id: 3,
          starts_at: "2026-06-30T10:00:00Z",
          ends_at: "2026-06-30T14:00:00Z",
          break_minutes: 0,
          status: "open",
          published: true,
          notes: "",
          created_by_staff_id: 1,
          created_at: "",
          updated_at: "",
        },
      ],
    });

    renderView();

    // Position chip resolved from the positions list.
    await waitFor(() => expect(screen.getByText("Server")).toBeInTheDocument());
    // Net duration = 6h span − 30m break = 5h 30m.
    expect(screen.getByText("5h 30m")).toBeInTheDocument();
    expect(screen.getByText("30m break")).toBeInTheDocument();
    // Staff-facing guard: never a dollar amount.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("offers a hand-off on a future shift: tap → sheet → give up calls requestSwap(giveup)", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 1, status: "published" },
      shifts: [futureShift()],
    });
    mockedCoverage.requestSwap.mockResolvedValue({ id: 1 });
    renderView();

    // The action appears on the future shift.
    const offerBtn = await screen.findByRole("button", { name: "Offer shift" });
    fireEvent.click(offerBtn);

    // The sheet reveals the two hand-off choices.
    expect(await screen.findByText("Offer for a teammate to cover")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Give up the shift"));

    await waitFor(() =>
      expect(mockedCoverage.requestSwap).toHaveBeenCalledWith("42", 21, {
        kind: "giveup",
        target: "all_in_role",
      }),
    );
    await waitFor(() => expect(mockToast.showSuccess).toHaveBeenCalledWith("Shift given up"));
  });

  it("offer-for-cover choice calls requestSwap(swap)", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 1, status: "published" },
      shifts: [futureShift()],
    });
    mockedCoverage.requestSwap.mockResolvedValue({ id: 2 });
    renderView();

    fireEvent.click(await screen.findByRole("button", { name: "Offer shift" }));
    fireEvent.click(await screen.findByText("Offer for a teammate to cover"));

    await waitFor(() =>
      expect(mockedCoverage.requestSwap).toHaveBeenCalledWith("42", 21, {
        kind: "swap",
        target: "all_in_role",
      }),
    );
  });

  it("shows 'Requested' (not the action) when a live swap already exists for the shift", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 1, status: "published" },
      shifts: [futureShift()],
    });
    mockedCoverage.listMine.mockResolvedValue({
      claims: [],
      swaps: [{ id: 5, shift_id: 21, kind: "giveup", status: "pending_approval" }],
    });
    renderView();

    await waitFor(() => expect(screen.getByText("Requested")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Offer shift" })).toBeNull();
  });

  it("does not offer a hand-off on a past shift", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 1, status: "published" },
      shifts: [
        futureShift({
          id: 22,
          starts_at: "2020-01-15T17:00:00Z",
          ends_at: "2020-01-15T23:00:00Z",
        }),
      ],
    });
    renderView();

    await waitFor(() => expect(screen.getByText("Server")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Offer shift" })).toBeNull();
  });

  it("shows the empty state when no shift belongs to the staff member", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: null,
      shifts: [],
    });

    renderView();

    await waitFor(() =>
      expect(screen.getByText("No shifts this week")).toBeInTheDocument(),
    );
    expect(screen.getByText("Nothing published yet")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });
});
