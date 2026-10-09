/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import CashRegisterDashboard from "../CashRegisterDashboard";
import { cashRegisterApi } from "@/api/cashRegister";
import { queryKeys } from "@/api/queryKeys";
import { asDollars } from "@/types/money";

const mockGetBusiness = jest.fn(() =>
  Promise.resolve({ default_currency: "USD", timezone: "UTC" }),
);
jest.mock("@/api/business", () => ({
  getBusiness: () => mockGetBusiness(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    const labels: Record<string, string> = {
      "businessDashboard.cashRegisterDashboard.title": "Caja",
      "businessDashboard.cashRegisterDashboard.subtitle":
        "Cash register control",
      "businessDashboard.cashRegisterDashboard.openSession": "Open session",
      "businessDashboard.cashRegisterDashboard.openingFloat": "Opening float",
      "businessDashboard.cashRegisterDashboard.openingFloatRequired":
        "Enter the opening float to open the drawer.",
      "businessDashboard.cashRegisterDashboard.suggestedFloat":
        "Last declared opening float: {amount}",
      "businessDashboard.cashRegisterDashboard.useSuggestedFloat":
        "Use suggested",
      "businessDashboard.cashRegisterDashboard.shell.inDrawer": "In drawer",
      "businessDashboard.cashRegisterDashboard.shell.unassigned":
        "Unassigned cash",
      "businessDashboard.cashRegisterDashboard.shell.history": "Closed shifts",
      "businessDashboard.cashRegisterDashboard.openingNote": "Opening note",
      "businessDashboard.cashRegisterDashboard.currentSession":
        "Current session",
      "businessDashboard.cashRegisterDashboard.noOpenSession":
        "No open session",
      "businessDashboard.cashRegisterDashboard.cashSales": "Cash sales",
      "businessDashboard.cashRegisterDashboard.cashRefunds": "Cash refunds",
      "businessDashboard.cashRegisterDashboard.cashIn": "Cash in",
      "businessDashboard.cashRegisterDashboard.cashOut": "Cash out",
      "businessDashboard.cashRegisterDashboard.manualMovement":
        "Manual movement",
      "businessDashboard.cashRegisterDashboard.amount": "Amount",
      "businessDashboard.cashRegisterDashboard.reason": "Reason",
      "businessDashboard.cashRegisterDashboard.note": "Note",
      "businessDashboard.cashRegisterDashboard.closeSession": "Close session",
      "businessDashboard.cashRegisterDashboard.countedCash": "Counted cash",
      "businessDashboard.cashRegisterDashboard.cashCountInput": "Drawer count",
      "businessDashboard.cashRegisterDashboard.expectedCash": "Expected cash",
      "businessDashboard.cashRegisterDashboard.variance": "Over / Short",
      "businessDashboard.cashRegisterDashboard.overAmount": "Over {amount}",
      "businessDashboard.cashRegisterDashboard.shortAmount": "Short {amount}",
      "businessDashboard.cashRegisterDashboard.evenAmount": "Even {amount}",
      "businessDashboard.cashRegisterDashboard.history": "History",
      "businessDashboard.cashRegisterDashboard.unassignedCash":
        "Unassigned cash",
      "businessDashboard.cashRegisterDashboard.over": "Over",
      "businessDashboard.cashRegisterDashboard.short": "Short",
      "businessDashboard.cashRegisterDashboard.loading": "Loading caja",
      "businessDashboard.cashRegisterDashboard.error": "Could not load caja",
      "businessDashboard.cashRegisterDashboard.emptyHistory":
        "No closed sessions yet",
      "businessDashboard.cashRegisterDashboard.emptyMovements":
        "No movements yet",
      "businessDashboard.cashRegisterDashboard.opened": "Opened",
      "businessDashboard.cashRegisterDashboard.closed": "Closed",
      "businessDashboard.cashRegisterDashboard.buttons.open": "Open",
      "businessDashboard.cashRegisterDashboard.buttons.addCashIn":
        "Add cash in",
      "businessDashboard.cashRegisterDashboard.buttons.addCashOut":
        "Add cash out",
      "businessDashboard.cashRegisterDashboard.buttons.submitClose":
        "Submit close",
      "businessDashboard.cashRegisterDashboard.buttons.cancel": "Cancel",
      "businessDashboard.cashRegisterDashboard.buttons.previous": "Previous",
      "businessDashboard.cashRegisterDashboard.buttons.next": "Next",
      "businessDashboard.cashRegisterDashboard.inspect": "Inspect",
      "businessDashboard.cashRegisterDashboard.inspectUnassigned":
        "Inspect unassigned cash",
      "businessDashboard.cashRegisterDashboard.emptyUnassigned":
        "No unassigned cash tenders",
      "businessDashboard.cashRegisterDashboard.session": "Session",
      "businessDashboard.cashRegisterDashboard.sessionNumber": "Session #{id}",
      "businessDashboard.cashRegisterDashboard.status": "Status",
      "businessDashboard.cashRegisterDashboard.closedBy": "Closed by",
      "businessDashboard.cashRegisterDashboard.statuses.open": "Open",
      "businessDashboard.cashRegisterDashboard.statuses.closed": "Closed",
      "businessDashboard.cashRegisterDashboard.viewSessionDetail":
        "View session {id} detail",
    };
    let out = labels[key] ?? key;
    if (params) {
      Object.entries(params).forEach(([k, v]) => {
        out = out.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
      });
    }
    return out;
  },
}));

jest.mock("@/api/cashRegister", () => ({
  cashRegisterApi: {
    getCurrent: jest.fn(),
    listSessions: jest.fn(),
    getUnassigned: jest.fn(),
    listUnassigned: jest.fn(),
    getSession: jest.fn(),
    openSession: jest.fn(),
    createMovement: jest.fn(),
    closeSession: jest.fn(),
  },
}));

const mockedApi = cashRegisterApi as jest.Mocked<typeof cashRegisterApi>;

function openSession(overrides: Record<string, unknown> = {}) {
  return {
    id: 10,
    business_id: 1,
    status: "open",
    opening_float: asDollars(100),
    opening_note: "Morning",
    opened_by_user_id: 7,
    opened_by_staff_id: null,
    opened_by_label: "Owner",
    opened_at: "2026-06-27T10:00:00Z",
    cash_sales: asDollars(250),
    cash_refunds: asDollars(15),
    cash_in: asDollars(40),
    cash_out: asDollars(20),
    closing_note: "",
    closed_by_user_id: null,
    closed_by_staff_id: null,
    closed_by_label: "",
    closed_at: null,
    created_at: "2026-06-27T10:00:00Z",
    updated_at: "2026-06-27T10:00:00Z",
    movements: [],
    ...overrides,
  } as any;
}

function movement(overrides: Record<string, unknown> = {}) {
  return {
    id: 88,
    business_id: 1,
    session_id: 10,
    movement_type: "cash_in",
    amount: asDollars(18.75),
    reason: "Safe drop",
    note: "",
    alternative_payment_id: null,
    bill_id: null,
    actor_user_id: 7,
    actor_staff_id: null,
    actor_label: "Owner",
    occurred_at: "2026-06-27T12:00:00Z",
    created_at: "2026-06-27T12:00:00Z",
    ...overrides,
  } as any;
}

const closedSession = openSession({
  id: 9,
  status: "closed",
  expected_cash: asDollars(355),
  counted_cash: asDollars(360),
  variance: asDollars(5),
  closed_at: "2026-06-27T20:00:00Z",
  closing_note: "Closed",
  closed_by_label: "Night Manager",
  opened_by_label: "Morning Opener",
});

function setupApi({
  currentSession = null,
  history = [],
  unassigned = { total: asDollars(0), count: 0 },
  suggestedOpeningFloat = null,
}: {
  currentSession?: any;
  history?: any[];
  unassigned?: { total: ReturnType<typeof asDollars>; count: number };
  suggestedOpeningFloat?: ReturnType<typeof asDollars> | null;
} = {}) {
  mockedApi.getCurrent.mockResolvedValue({
    session: currentSession,
    unassigned_cash_total: unassigned.total,
    unassigned_cash_count: unassigned.count,
    suggested_opening_float: suggestedOpeningFloat,
  });
  mockedApi.listSessions.mockResolvedValue({
    sessions: history,
    total: history.length,
  });
  mockedApi.getUnassigned.mockResolvedValue(unassigned);
  mockedApi.listUnassigned.mockResolvedValue({ items: [], total: 0 });
  mockedApi.getSession.mockResolvedValue(currentSession ?? openSession());
}

function renderDashboard(businessId = "1") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidateSpy = jest.spyOn(client, "invalidateQueries");
  const result = render(
    <QueryClientProvider client={client}>
      <CashRegisterDashboard businessId={businessId} />
    </QueryClientProvider>,
  );
  const rerenderWithBusinessId = (nextBusinessId: string) =>
    result.rerender(
      <QueryClientProvider client={client}>
        <CashRegisterDashboard businessId={nextBusinessId} />
      </QueryClientProvider>,
    );
  return { client, invalidateSpy, rerenderWithBusinessId };
}

describe("CashRegisterDashboard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetBusiness.mockResolvedValue({
      default_currency: "USD",
      timezone: "UTC",
    });
    setupApi();
  });

  it("shows the open-session form when no caja session is open", async () => {
    renderDashboard();

    expect(
      await screen.findByRole("heading", { name: "No open session" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Opening float")).toBeInTheDocument();
    expect(screen.getByLabelText("Opening note")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open" })).toBeInTheDocument();
  });

  it("opens a session and invalidates Caja queries", async () => {
    mockedApi.openSession.mockResolvedValue(openSession());
    const { invalidateSpy } = renderDashboard();

    await screen.findByRole("heading", { name: "No open session" });
    await userEvent.type(screen.getByLabelText("Opening float"), "125.50");
    await userEvent.type(screen.getByLabelText("Opening note"), "Start drawer");
    await userEvent.click(screen.getByRole("button", { name: "Open" }));

    await waitFor(() =>
      expect(mockedApi.openSession).toHaveBeenCalledWith("1", {
        opening_float: asDollars(125.5),
        opening_note: "Start drawer",
      }),
    );
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: queryKeys.cashRegister.all("1"),
      }),
    );
  });

  it("shows current open session totals without expected cash or variance", async () => {
    setupApi({ currentSession: openSession() });
    renderDashboard();

    expect(
      await screen.findByRole("heading", { name: "Current session" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Opening float")).toBeInTheDocument();
    expect(screen.getByText("$100.00")).toBeInTheDocument();
    expect(screen.getByText("Cash sales")).toBeInTheDocument();
    expect(screen.getByText("$250.00")).toBeInTheDocument();
    expect(screen.queryByText("Expected cash")).not.toBeInTheDocument();
    expect(screen.queryByText("Variance")).not.toBeInTheDocument();
    expect(screen.queryByText("Counted cash")).not.toBeInTheDocument();
  });

  // Fix 13: past sessions are read-only and independent, so history is shown
  // even while a session is open (the old canShowHistory gate hid ALL history
  // while any session was open — the audit's dead-end). The CURRENT open-session
  // panel still omits Expected/Counted/Variance (covered by the test above);
  // here we assert the closed session's history row IS reachable while open.
  it("shows read-only history (with closed-session columns) while a session is open", async () => {
    setupApi({ currentSession: openSession(), history: [closedSession] });
    renderDashboard();

    expect(
      await screen.findByRole("heading", { name: "Current session" }),
    ).toBeInTheDocument();
    // The history table renders alongside the open session, with its own
    // Counted cash / Variance columns for the closed rows.
    const historyTable = await screen.findByRole("table", { name: "History" });
    expect(within(historyTable).getByText("Counted cash")).toBeInTheDocument();
    expect(within(historyTable).getByText("Over / Short")).toBeInTheDocument();
    // The closed session's counted-cash value is visible in the history row.
    expect(within(historyTable).getByText("$360.00")).toBeInTheDocument();
  });

  it("shows closed-by only for closed history rows (not opener fallback)", async () => {
    const openHistoryRow = openSession({
      id: 11,
      status: "open",
      opened_by_label: "user:8",
      closed_by_label: "",
      closed_at: null,
    });
    setupApi({
      currentSession: null,
      history: [openHistoryRow, closedSession],
    });
    renderDashboard();

    const historyTable = await screen.findByRole("table", { name: "History" });
    // Closed row keeps the real closer name.
    expect(within(historyTable).getByText("Night Manager")).toBeInTheDocument();
    // Open row must NOT put opened_by into the Closed by column.
    expect(within(historyTable).queryByText("user:8")).not.toBeInTheDocument();
    expect(
      within(historyTable).queryByText("Morning Opener"),
    ).not.toBeInTheDocument();
  });

  it("creates manual cash-in and cash-out movements", async () => {
    setupApi({ currentSession: openSession() });
    mockedApi.createMovement.mockResolvedValue({
      movement: {} as any,
      session: openSession(),
    });
    renderDashboard();

    await screen.findByRole("heading", { name: "Manual movement" });
    await userEvent.type(screen.getByLabelText("Reason"), "Bank top-up");
    await userEvent.type(screen.getByLabelText("Note"), "Manager approved");
    await userEvent.type(screen.getByLabelText("Amount"), "30");
    await userEvent.click(screen.getByRole("button", { name: "Add cash in" }));

    await waitFor(() =>
      expect(mockedApi.createMovement).toHaveBeenCalledWith("1", 10, {
        movement_type: "cash_in",
        amount: asDollars(30),
        reason: "Bank top-up",
        note: "Manager approved",
      }),
    );

    mockedApi.createMovement.mockClear();
    await userEvent.clear(screen.getByLabelText("Reason"));
    await userEvent.type(screen.getByLabelText("Reason"), "Petty cash");
    await userEvent.clear(screen.getByLabelText("Note"));
    await userEvent.clear(screen.getByLabelText("Amount"));
    await userEvent.type(screen.getByLabelText("Amount"), "12.25");
    await userEvent.click(screen.getByRole("button", { name: "Add cash out" }));

    await waitFor(() =>
      expect(mockedApi.createMovement).toHaveBeenCalledWith("1", 10, {
        movement_type: "cash_out",
        amount: asDollars(12.25),
        reason: "Petty cash",
        note: "",
      }),
    );
  });

  it("renders Amount/Reason/Note as distinct block labels, never one inline run (issue 828)", async () => {
    // Inline <label> boxes inside a space-y column collapse into a single
    // line — the operator saw "AmountReasonNote" over three naked inputs.
    // Every form label must be block-level so each field reads as its own
    // labeled row.
    setupApi({ currentSession: openSession() });
    renderDashboard();

    await screen.findByRole("heading", { name: "Manual movement" });
    for (const name of ["Amount", "Reason", "Note"]) {
      const field = screen.getByLabelText(name);
      const label = field.closest("label");
      expect(label).not.toBeNull();
      expect(label!.className).toMatch(/\bblock\b/);
    }
  });

  it("does not allow zero manual movement amounts", async () => {
    setupApi({ currentSession: openSession() });
    renderDashboard();

    await screen.findByRole("heading", { name: "Manual movement" });
    await userEvent.type(screen.getByLabelText("Reason"), "Zero adjustment");
    await userEvent.type(screen.getByLabelText("Amount"), "0");

    const cashIn = screen.getByRole("button", { name: "Add cash in" });
    const cashOut = screen.getByRole("button", { name: "Add cash out" });
    expect(cashIn).toBeDisabled();
    expect(cashOut).toBeDisabled();

    await userEvent.click(cashIn);
    await userEvent.click(cashOut);
    expect(mockedApi.createMovement).not.toHaveBeenCalled();
  });

  it("renders movement rows from session detail when current omits movements", async () => {
    setupApi({
      currentSession: openSession({ movements: undefined }),
    });
    mockedApi.getSession.mockResolvedValue(
      openSession({ movements: [movement({ reason: "Register top-up" })] }),
    );
    renderDashboard();

    expect(await screen.findByText("Register top-up")).toBeInTheDocument();
    expect(screen.getByText("$18.75")).toBeInTheDocument();
    expect(mockedApi.getSession).toHaveBeenCalledWith("1", 10);
  });

  it("renders the drawer opening instant in the business timezone", async () => {
    mockGetBusiness.mockResolvedValue({
      default_currency: "USD",
      timezone: "Asia/Tokyo",
    });
    setupApi({
      currentSession: openSession({ opened_at: "2026-07-12T01:30:00Z" }),
    });
    renderDashboard();

    expect(await screen.findByText(/Jul 12, 2026, 10:30/)).toBeInTheDocument();
  });

  it("does not render the open-session form when the current query errors", async () => {
    mockedApi.getCurrent.mockRejectedValue(new Error("current failed"));
    renderDashboard();

    expect(await screen.findByText("Could not load caja")).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "No open session" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Opening float")).not.toBeInTheDocument();
  });

  it("previews expected vs counted only AFTER a count is entered (L2-30 blind-close)", async () => {
    setupApi({ currentSession: openSession() });
    renderDashboard();

    await screen.findByRole("heading", { name: "Current session" });
    await userEvent.click(
      screen.getByRole("button", { name: "Close session" }),
    );

    expect(
      screen.getByRole("dialog", { name: "Close session" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Drawer count")).toBeInTheDocument();
    // L2-30: blind-close — expected cash must NOT appear before a count.
    expect(
      screen.queryByTestId("cash-register-close-preview"),
    ).not.toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Drawer count"), "360");
    const preview = await screen.findByTestId("cash-register-close-preview");
    expect(within(preview).getByText("Expected cash")).toBeInTheDocument();
    // 100 + 250 - 15 + 40 - 20 = 355
    expect(within(preview).getByText("$355.00")).toBeInTheDocument();
    expect(within(preview).getByText("Counted cash")).toBeInTheDocument();
    expect(within(preview).getByText("Over / Short")).toBeInTheDocument();
  });

  it("shows expected cash and variance after close succeeds", async () => {
    setupApi({ currentSession: openSession() });
    mockedApi.closeSession.mockResolvedValue(closedSession);
    renderDashboard();

    await screen.findByRole("heading", { name: "Current session" });
    await userEvent.click(
      screen.getByRole("button", { name: "Close session" }),
    );
    await userEvent.type(screen.getByLabelText("Drawer count"), "360");
    await userEvent.click(screen.getByRole("button", { name: "Submit close" }));

    await waitFor(() =>
      expect(mockedApi.closeSession).toHaveBeenCalledWith("1", 10, {
        counted_cash: asDollars(360),
        closing_note: "",
      }),
    );
    const result = await screen.findByTestId("cash-register-close-result");
    expect(within(result).getByText("Expected cash")).toBeInTheDocument();
    expect(within(result).getByText("$355.00")).toBeInTheDocument();
    expect(within(result).getByText("Counted cash")).toBeInTheDocument();
    expect(within(result).getByText("$360.00")).toBeInTheDocument();
    expect(within(result).getByText("Over / Short")).toBeInTheDocument();
    // Drawer was over by $5 — variance carries an explicit + sign.
    expect(within(result).getByText("Over $5.00")).toBeInTheDocument();
  });

  it("clears local close state when business changes", async () => {
    setupApi({ currentSession: openSession() });
    mockedApi.closeSession.mockResolvedValue(closedSession);
    const { rerenderWithBusinessId } = renderDashboard();

    await screen.findByRole("heading", { name: "Current session" });
    await userEvent.click(
      screen.getByRole("button", { name: "Close session" }),
    );
    await userEvent.type(screen.getByLabelText("Drawer count"), "360");
    await userEvent.click(screen.getByRole("button", { name: "Submit close" }));
    expect(
      await screen.findByTestId("cash-register-close-result"),
    ).toBeInTheDocument();

    mockedApi.getCurrent.mockResolvedValue({
      session: null,
      unassigned_cash_total: asDollars(0),
      unassigned_cash_count: 0,
    });
    mockedApi.listSessions.mockResolvedValue({ sessions: [], total: 0 });
    mockedApi.getUnassigned.mockResolvedValue({
      total: asDollars(0),
      count: 0,
    });
    rerenderWithBusinessId("2");

    await screen.findByRole("heading", { name: "No open session" });
    expect(
      screen.queryByTestId("cash-register-close-result"),
    ).not.toBeInTheDocument();
    // Money inputs are now type="text" (accept comma decimals), so an empty
    // field reads as "" rather than the numeric input's null.
    expect(screen.getByLabelText("Opening float")).toHaveValue("");
  });

  it("renders historical sessions with variance", async () => {
    setupApi({ history: [closedSession] });
    renderDashboard();

    const history = await screen.findByRole("table", { name: "History" });
    expect(
      within(history).getByText("Jun 27, 2026, 10:00"),
    ).toBeInTheDocument();
    expect(
      within(history).getByText("Jun 27, 2026, 20:00"),
    ).toBeInTheDocument();
    expect(within(history).getByText("Over $5.00")).toBeInTheDocument();
  });

  it("suggests last opening float, not counted cash that includes a shortage", async () => {
    // Omit /current.suggested_opening_float so this exercises the history
    // fallback. If that path still reads counted_cash, the label becomes
    // $1,473.07 and this test fails.
    setupApi({
      history: [
        openSession({
          id: 254,
          status: "closed",
          opening_float: asDollars(200),
          expected_cash: asDollars(1474.9),
          counted_cash: asDollars(1473.07),
          variance: asDollars(-1.83),
          closed_at: "2026-06-27T20:00:00Z",
        }),
        openSession({
          id: 8,
          status: "closed",
          opening_float: asDollars(900),
          counted_cash: asDollars(950),
          closed_at: "2026-06-26T20:00:00Z",
        }),
      ],
    });
    renderDashboard();

    expect(
      await screen.findByText("Last declared opening float: $200.00"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Last declared opening float: $360.00"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("Last declared opening float: $1,473.07"),
    ).not.toBeInTheDocument();

    const openingInput = screen.getByRole("textbox", {
      name: /opening float/i,
    });
    expect((openingInput as HTMLInputElement).value).toBe("");

    await userEvent.click(screen.getByRole("button", { name: "Use suggested" }));
    expect((openingInput as HTMLInputElement).value).toBe("200");
    expect((openingInput as HTMLInputElement).value).not.toBe("1473.07");
  });

  it("prefers /current suggested_opening_float over a history counted total", async () => {
    setupApi({
      suggestedOpeningFloat: asDollars(200),
      history: [
        openSession({
          id: 254,
          status: "closed",
          opening_float: asDollars(200),
          counted_cash: asDollars(1473.07),
          variance: asDollars(-1.83),
          closed_at: "2026-06-27T20:00:00Z",
        }),
      ],
    });
    renderDashboard();

    expect(
      await screen.findByText("Last declared opening float: $200.00"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Last declared opening float: $1,473.07"),
    ).not.toBeInTheDocument();
  });

  // #652 follow-up: the label is a claim about the LAST close, so the suggestion
  // has to stay pinned to the newest closed shift. It used to read row 0 of the
  // *currently displayed* history page, so paging the closed-shift table below
  // swapped an older shift's float into the open-drawer form under the same
  // "last close" label.
  it("keeps the suggested float on the newest close while paging session history", async () => {
    setupApi();
    const newestClose = openSession({
      id: 260,
      status: "closed",
      opening_float: asDollars(100),
      counted_cash: asDollars(1473.07),
      closed_at: "2026-06-27T20:00:00Z",
    });
    const olderClose = openSession({
      id: 240,
      status: "closed",
      opening_float: asDollars(900),
      counted_cash: asDollars(950),
      closed_at: "2026-06-01T20:00:00Z",
    });
    mockedApi.listSessions.mockImplementation((async (
      _businessId: string,
      params?: { limit?: number; offset?: number },
    ) =>
      (params?.offset ?? 0) === 0
        ? { sessions: [newestClose], total: 25 }
        : { sessions: [olderClose], total: 25 }) as any);

    renderDashboard();

    expect(
      await screen.findByText("Last declared opening float: $100.00"),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    // Page 2 is on screen...
    await screen.findByTestId("cash-register-history-card-240");

    // ...but "last close" still means session #260's declared float.
    expect(
      screen.getByText("Last declared opening float: $100.00"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Last declared opening float: $900.00"),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Use suggested" }));
    const openingInput = screen.getByRole("textbox", {
      name: /opening float/i,
    }) as HTMLInputElement;
    expect(openingInput.value).toBe("100");
    expect(openingInput.value).not.toBe("900");
  });

  // Fix 14: the unassigned-cash panel opens a drawer listing the individual
  // tenders via listUnassigned.
  it("opens a drawer listing the individual unassigned cash tenders", async () => {
    setupApi({ unassigned: { total: asDollars(45), count: 2 } });
    mockedApi.listUnassigned.mockResolvedValue({
      items: [
        {
          id: 1,
          bill_id: 10,
          bill_number: "B-10",
          table_name: "",
          participant_name: "Cash",
          amount: asDollars(25),
          status: "confirmed",
          created_at: "2026-06-27T12:00:00Z",
        },
      ],
      total: 2,
    });

    renderDashboard();

    const inspect = await screen.findByRole("button", {
      name: "Inspect unassigned cash",
    });
    await userEvent.click(inspect);

    await waitFor(() => expect(mockedApi.listUnassigned).toHaveBeenCalled());
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("#B-10")).toBeInTheDocument();
    expect(within(dialog).getByText("$25.00")).toBeInTheDocument();
  });

  // R2-10: a tender with a bill_id but no bill_number must still read as the
  // person who paid — not as a bare internal number.
  it("labels an unassigned tender without a bill number by participant", async () => {
    setupApi({ unassigned: { total: asDollars(45), count: 2 } });
    mockedApi.listUnassigned.mockResolvedValue({
      items: [
        {
          id: 1,
          bill_id: 10,
          bill_number: "",
          table_name: "",
          participant_name: "Ana",
          amount: asDollars(25),
          status: "confirmed",
          created_at: "2026-06-27T12:00:00Z",
        },
        {
          id: 2,
          bill_id: 11,
          bill_number: "",
          table_name: "",
          participant_name: "",
          amount: asDollars(20),
          status: "confirmed",
          created_at: "2026-06-27T12:05:00Z",
        },
      ],
      total: 2,
    });

    renderDashboard();

    await userEvent.click(
      await screen.findByRole("button", { name: "Inspect unassigned cash" }),
    );

    await waitFor(() => expect(mockedApi.listUnassigned).toHaveBeenCalled());
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Ana")).toBeInTheDocument();
    expect(within(dialog).queryByText("10")).not.toBeInTheDocument();
    // No name either — the internal id stays marked as one.
    expect(within(dialog).getByText("#11")).toBeInTheDocument();
  });

  // Fix 12: clicking a closed-session history row opens a read-only detail
  // drawer that fetches the full session (movements included) via getSession.
  it("opens a read-only detail drawer for a closed history row", async () => {
    setupApi({ history: [closedSession] });
    mockedApi.getSession.mockResolvedValue({
      ...closedSession,
      movements: [
        {
          id: 501,
          business_id: 1,
          session_id: closedSession.id as number,
          movement_type: "cash_in",
          amount: asDollars(20),
          reason: "Top-up",
          note: "",
          alternative_payment_id: null,
          bill_id: null,
          actor_user_id: null,
          actor_staff_id: null,
          actor_label: "Manager",
          occurred_at: "2026-06-27T15:00:00Z",
          created_at: "2026-06-27T15:00:00Z",
        },
      ],
    });

    renderDashboard();

    const history = await screen.findByRole("table", { name: "History" });
    // Match the closed row by its unique variance cell (the mocked translation
    // provider returns raw keys, so sessionNumber isn't a stable text anchor).
    const row = within(history)
      .getByText("Over $5.00")
      .closest("tr") as HTMLElement;
    await userEvent.click(row);

    // The drawer fetches the clicked session and shows its movement amount.
    await waitFor(() =>
      expect(mockedApi.getSession).toHaveBeenCalledWith("1", closedSession.id),
    );
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("$20.00")).toBeInTheDocument();
  });

  it("labels a positive variance as Over", async () => {
    setupApi({ history: [closedSession] });
    renderDashboard();

    const history = await screen.findByRole("table", { name: "History" });
    expect(within(history).getByText("Over $5.00")).toBeInTheDocument();
  });

  it("shows a positive close-result variance with an explicit +", async () => {
    setupApi({ currentSession: openSession() });
    mockedApi.closeSession.mockResolvedValue(closedSession);
    renderDashboard();

    await screen.findByRole("heading", { name: "Current session" });
    await userEvent.click(
      screen.getByRole("button", { name: "Close session" }),
    );
    await userEvent.type(screen.getByLabelText("Drawer count"), "360");
    await userEvent.click(screen.getByRole("button", { name: "Submit close" }));

    const result = await screen.findByTestId("cash-register-close-result");
    expect(within(result).getByText("Over $5.00")).toBeInTheDocument();
  });

  it("leaves a negative (short) variance with its minus sign and no +", async () => {
    const shortSession = openSession({
      id: 8,
      status: "closed",
      expected_cash: asDollars(355),
      counted_cash: asDollars(350),
      variance: asDollars(-5),
      closed_at: "2026-06-27T20:00:00Z",
      closing_note: "Closed short",
    });
    setupApi({ history: [shortSession] });
    renderDashboard();

    const history = await screen.findByRole("table", { name: "History" });
    expect(within(history).getByText("Short $5.00")).toBeInTheDocument();
  });

  it("does not prefix a zero (reconciled) variance with +", async () => {
    const evenSession = openSession({
      id: 7,
      status: "closed",
      expected_cash: asDollars(355),
      counted_cash: asDollars(355),
      variance: asDollars(0),
      closed_at: "2026-06-27T20:00:00Z",
      closing_note: "Balanced",
    });
    setupApi({ history: [evenSession] });
    renderDashboard();

    const history = await screen.findByRole("table", { name: "History" });
    expect(within(history).getByText("Even $0.00")).toBeInTheDocument();
  });

  it("renders a mobile history card with session, status, and money", async () => {
    setupApi({ history: [closedSession] });
    renderDashboard();

    const card = await screen.findByTestId("cash-register-history-card-9");
    expect(within(card).getByText("Session #9")).toBeInTheDocument();
    expect(card).toHaveTextContent("Closed");
    expect(within(card).getByText("$100.00")).toBeInTheDocument();
    expect(within(card).getByText("$360.00")).toBeInTheDocument();
    expect(within(card).getByText("Over $5.00")).toBeInTheDocument();
    expect(
      screen.getByTestId("cash-register-history-cards"),
    ).toBeInTheDocument();
  });

  it("opens the read-only detail drawer from a history card", async () => {
    setupApi({ history: [closedSession] });
    mockedApi.getSession.mockResolvedValue({
      ...closedSession,
      movements: [movement({ amount: asDollars(20), reason: "Card top-up" })],
    });
    renderDashboard();

    await userEvent.click(
      await screen.findByTestId("cash-register-history-card-9"),
    );

    await waitFor(() =>
      expect(mockedApi.getSession).toHaveBeenCalledWith("1", closedSession.id),
    );
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("$20.00")).toBeInTheDocument();
  });

  it("renders unassigned cash summary when present", async () => {
    setupApi({ unassigned: { total: asDollars(42.75), count: 3 } });
    renderDashboard();

    // Header stat + panel title both say "Unassigned cash" (money-aware header).
    expect(
      (await screen.findAllByText("Unassigned cash")).length,
    ).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("$42.75").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("3")).toBeInTheDocument();
  });
});
