/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import TimesheetReview from "./TimesheetReview";
import { timeclockApi, type TimeEntry } from "@/api/timeclock";
import { laborCostApi } from "@/api/laborCost";
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
jest.mock("@/api/timeclock", () => ({
  timeclockApi: {
    listForReview: jest.fn(),
    listForReviewPaged: jest.fn(),
    approve: jest.fn(),
    reject: jest.fn(),
    edit: jest.fn(),
    createManual: jest.fn(),
  },
}));
jest.mock("@/api/laborCost", () => ({ laborCostApi: { getActual: jest.fn() } }));
jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));
// The panel subscribes to timeclock.entry via useStaffRealtime; stub it so the
// test doesn't open a real SSE connection.
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

const mockedTime = timeclockApi as unknown as {
  listForReview: jest.Mock;
  listForReviewPaged: jest.Mock;
  approve: jest.Mock;
  reject: jest.Mock;
  edit: jest.Mock;
  createManual: jest.Mock;
};

// Wrap a bare array in the paged envelope the component now consumes.
const paged = (rows: unknown[]) => ({
  data: rows,
  total: rows.length,
  offset: 0,
  limit: 20,
});
const mockedLabor = laborCostApi as unknown as { getActual: jest.Mock };
const mockedStaff = getBusinessStaff as unknown as jest.Mock;

const pendingEntry: TimeEntry = {
  id: 5,
  business_id: 42,
  staff_id: 7,
  shift_id: 9,
  clock_in_at: "2026-06-30T17:00:00.000Z",
  clock_out_at: "2026-06-30T22:30:00.000Z",
  break_minutes: 30,
  source: "staff_punch",
  status: "pending_review",
  approved_by_staff_id: null,
  note: "",
  created_at: "",
  updated_at: "",
  worked_minutes: 300,
  worked_hours: 5,
};

function renderReview(
  canViewFinancials: boolean,
  businessTimezone: string | null = null,
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <TimesheetReview
        businessId="42"
        businessTimezone={businessTimezone}
        canViewFinancials={canViewFinancials}
        currency="USD"
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedStaff.mockResolvedValue({
    staff: [{ id: 7, name: "Dana", email: "dana@example.com", role: "server", business_id: 42, created_at: "", updated_at: "" }],
    pending_invitations: [],
  });
});

describe("TimesheetReview", () => {
  it("shows punch walls in the venue zone, not 4:00 AM UTC (#247)", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(
      paged([
        {
          ...pendingEntry,
          clock_in_at: "2026-08-11T20:00:00.000Z",
          clock_out_at: "2026-08-12T04:00:00.000Z",
        },
      ]),
    );
    renderReview(false, "America/New_York");
    await waitFor(() =>
      expect(screen.getAllByText("Dana").length).toBeGreaterThan(0),
    );
    const body = document.body.textContent || "";
    expect(body).toMatch(/4:00\s*PM/);
    expect(body).not.toMatch(/4:00\s*AM/);
  });

  it("non-financial view: shows worked hours and NO dollars, approve calls timeclock.approve", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(paged([pendingEntry]));
    mockedTime.approve.mockResolvedValue({ ...pendingEntry, status: "approved" });

    renderReview(false);

    // Pending row renders (approve button only appears when there's an entry),
    // resolves the name, and shows worked hours (net of break = 5h).
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /dashboardTimesheets\.approve/ })).toBeInTheDocument(),
    );
    expect(screen.getAllByText("Dana").length).toBeGreaterThan(0);
    expect(document.body.textContent).toContain("5dashboardTimesheets.hoursUnit");
    // Server-scoped, paginated pending query.
    expect(mockedTime.listForReviewPaged).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({ status: "pending_review" }),
    );
    // Non-financial managers never trigger the labor-$ fetch.
    expect(mockedLabor.getActual).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: /dashboardTimesheets\.approve/ }));
    await waitFor(() => expect(mockedTime.approve).toHaveBeenCalledWith("42", 5));
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());

    // The financial gate: hours only, never a dollar amount.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("flags zero-minute and unlinked entries; clean entries carry no flag", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(
      paged([
        pendingEntry, // clean: linked shift, 5h worked
        {
          ...pendingEntry,
          id: 6,
          worked_minutes: 0,
          worked_hours: 0,
          clock_out_at: pendingEntry.clock_in_at,
        },
        { ...pendingEntry, id: 7, shift_id: null },
      ]),
    );

    renderReview(false);
    await waitFor(() =>
      expect(
        screen.getAllByRole("button", { name: /dashboardTimesheets\.approve/ })
          .length,
      ).toBe(3),
    );
    // One flag per anomaly, each naming its specific reason.
    expect(
      screen.getByText("dashboardTimesheets.flagZeroWorked"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("dashboardTimesheets.flagNoShift"),
    ).toBeInTheDocument();
    // The clean entry contributes no flag (exactly one of each above).
    expect(
      screen.getAllByText(/dashboardTimesheets\.flag/).length,
    ).toBe(2);
  });

  it("manual entry rejects an out<=in range and submits a valid one", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(paged([]));
    mockedTime.createManual.mockResolvedValue({ ...pendingEntry, source: "manager_manual" });

    renderReview(false);
    await waitFor(() =>
      expect(screen.getByText("dashboardTimesheets.emptyTitle")).toBeInTheDocument(),
    );

    // Staff is a NextUI Select: open the dropdown and pick Dana (id 7).
    await userEvent.click(
      screen.getByRole("button", { name: /dashboardTimesheets\.manualStaff/ }),
    );
    await userEvent.click(await screen.findByRole("option", { name: "Dana" }));

    const inField = screen.getByLabelText("dashboardTimesheets.manualClockIn");
    const outField = screen.getByLabelText("dashboardTimesheets.manualClockOut");

    fireEvent.change(inField, { target: { value: "2026-06-30T17:00" } });
    // out before in -> validation error, no submit.
    fireEvent.change(outField, { target: { value: "2026-06-30T16:00" } });
    await userEvent.click(screen.getByRole("button", { name: /dashboardTimesheets\.manualSubmit/ }));
    await waitFor(() =>
      expect(screen.getByText("dashboardTimesheets.manualInvalidRange")).toBeInTheDocument(),
    );
    expect(mockedTime.createManual).not.toHaveBeenCalled();

    // Fix the clock-out to be after clock-in -> submits with the business id + body.
    fireEvent.change(outField, { target: { value: "2026-06-30T22:00" } });
    await userEvent.click(screen.getByRole("button", { name: /dashboardTimesheets\.manualSubmit/ }));
    await waitFor(() => expect(mockedTime.createManual).toHaveBeenCalledTimes(1));
    expect(mockedTime.createManual.mock.calls[0][0]).toBe("42");
    // A venue without a timezone reads and writes UTC wall times, the same
    // zone the review rows render in.
    expect(mockedTime.createManual.mock.calls[0][1]).toMatchObject({
      staff_id: 7,
      clock_in_at: "2026-06-30T17:00:00.000Z",
      clock_out_at: "2026-06-30T22:00:00.000Z",
    });
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("financial view: surfaces the gated labor-$ summary", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(paged([pendingEntry]));
    mockedLabor.getActual.mockResolvedValue({
      period: "week",
      basis: "actual",
      labor_cost_pct: 0.282,
      worked_hours: 120,
      has_data: true,
      staff_contributions: [{ staff_id: 7, worked_minutes: 300, worked_hours: 5, labor_cost: 90 }],
      labor_cost: 3400,
    });

    renderReview(true);

    await waitFor(() => expect(mockedLabor.getActual).toHaveBeenCalledWith("42", "week"));
    // A dollar amount renders for financial callers (USD formatting).
    await waitFor(() => expect(document.body.textContent).toMatch(/\$3,400\.00/));
    // The percentage explains what it's a share OF.
    expect(screen.getByText("dashboardTimesheets.laborHint")).toBeInTheDocument();
    // The strip is explicitly labelled "this week", not the whole queue.
    expect(
      screen.getByText("dashboardTimesheets.laborLabelThisWeek"),
    ).toBeInTheDocument();
  });

  it("rejects a pending entry behind a confirm with a reason", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(paged([pendingEntry]));
    mockedTime.reject.mockResolvedValue({ ...pendingEntry, status: "rejected" });

    renderReview(false);
    await userEvent.click(
      await screen.findByRole("button", { name: /dashboardTimesheets\.reject/ }),
    );
    // The confirm modal is open; reject only fires on confirm.
    expect(mockedTime.reject).not.toHaveBeenCalled();
    const reasonField = await screen.findByLabelText(
      "dashboardTimesheets.rejectReasonLabel",
    );
    fireEvent.change(reasonField, { target: { value: "wrong out time" } });
    // The confirm button is the danger reject inside the modal (second match).
    const rejectButtons = screen.getAllByRole("button", {
      name: /dashboardTimesheets\.reject/,
    });
    await userEvent.click(rejectButtons[rejectButtons.length - 1]);
    await waitFor(() =>
      expect(mockedTime.reject).toHaveBeenCalledWith("42", 5, "wrong out time"),
    );
  });

  it("edits a pending entry via the edit modal", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(paged([pendingEntry]));
    mockedTime.edit.mockResolvedValue({ ...pendingEntry, status: "pending_review" });

    renderReview(false);
    await userEvent.click(
      await screen.findByRole("button", { name: /dashboardTimesheets\.edit/ }),
    );
    // The modal prefills from the entry; adjust the clock-out (scoped to the
    // dialog, since the standalone manual form shares the same field label).
    const dialog = await screen.findByRole("dialog");
    const outField = within(dialog).getByLabelText(
      "dashboardTimesheets.manualClockOut",
    );
    fireEvent.change(outField, { target: { value: "2026-06-30T23:00" } });
    await userEvent.click(
      within(dialog).getByRole("button", {
        name: /dashboardTimesheets\.editSubmit/,
      }),
    );
    await waitFor(() => expect(mockedTime.edit).toHaveBeenCalledTimes(1));
    expect(mockedTime.edit.mock.calls[0][0]).toBe("42");
    expect(mockedTime.edit.mock.calls[0][1]).toBe(5);
    expect(mockedTime.edit.mock.calls[0][2]).toMatchObject({
      clock_in_at: "2026-06-30T17:00:00.000Z",
      clock_out_at: "2026-06-30T23:00:00.000Z",
    });
  });

  it("edits punches as venue wall times, not device-local ones", async () => {
    mockedTime.listForReviewPaged.mockResolvedValue(paged([pendingEntry]));
    mockedTime.edit.mockResolvedValue({ ...pendingEntry, status: "pending_review" });

    renderReview(false, "America/New_York");
    await userEvent.click(
      await screen.findByRole("button", { name: /dashboardTimesheets\.edit/ }),
    );
    const dialog = await screen.findByRole("dialog");
    // 17:00Z / 22:30Z are 13:00 / 18:30 in New York (EDT) on any device.
    expect(
      within(dialog).getByLabelText("dashboardTimesheets.manualClockIn"),
    ).toHaveValue("2026-06-30T13:00");
    const outField = within(dialog).getByLabelText(
      "dashboardTimesheets.manualClockOut",
    );
    expect(outField).toHaveValue("2026-06-30T18:30");
    fireEvent.change(outField, { target: { value: "2026-06-30T20:00" } });
    await userEvent.click(
      within(dialog).getByRole("button", {
        name: /dashboardTimesheets\.editSubmit/,
      }),
    );
    await waitFor(() => expect(mockedTime.edit).toHaveBeenCalledTimes(1));
    expect(mockedTime.edit.mock.calls[0][2]).toMatchObject({
      clock_in_at: "2026-06-30T17:00:00.000Z",
      clock_out_at: "2026-07-01T00:00:00.000Z",
    });
  });
});
