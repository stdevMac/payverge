/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import LiveBills from "./LiveBills";
import { analyticsApi } from "@/api/analytics";
import { getBusiness } from "@/api/business";
import { getBill } from "@/api/bills";

// Force the operator locale to Argentine Spanish; keep the real getTranslation so
// labels resolve. es-AR money must render "US$ 37,82" (comma decimal), not the
// en-US "$37.82" — the bug audit finding H4 caught on the live view.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "es-AR", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: { getLiveBills: jest.fn() },
}));
jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(),
}));
jest.mock("@/api/bills", () => ({
  __esModule: true,
  getBill: jest.fn(),
}));
jest.mock("@/components/business/BillDetailsModal", () => ({
  BillDetailsModal: ({
    isOpen,
    bill,
  }: {
    isOpen: boolean;
    bill: { bill?: { id?: number } } | null;
  }) =>
    isOpen && bill?.bill?.id != null ? (
      <div data-testid="bill-details-modal" data-bill-id={String(bill.bill.id)}>
        BillDetailsModal
      </div>
    ) : null,
}));
// Keep polling inert so the test doesn't leak timers.
jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => ({ isPolling: false, startPolling: jest.fn(), stopPolling: jest.fn() }),
}));

const mockGetLiveBills = analyticsApi.getLiveBills as jest.Mock;
const mockGetBusiness = getBusiness as jest.Mock;

const bill = (over: Partial<Record<string, unknown>> = {}) => ({
  id: 1,
  bill_number: "1001",
  table_name: "Table 1",
  table_code: "ABC123",
  total_amount: 100,
  paid_amount: 0,
  remaining_amount: 100,
  tip_amount: 0,
  status: "unpaid",
  created_at: "2026-07-01T12:00:00Z",
  updated_at: "2026-07-01T12:00:00Z",
  ...over,
});

describe("LiveBills View button", () => {
  // Jest has no clearMocks; restore the module-level getBusiness resolution each
  // test since clearAllMocks below wipes mockResolvedValue too.
  beforeEach(() => {
    mockGetBusiness.mockResolvedValue({
      default_currency: "USD",
      timezone: "America/Argentina/Buenos_Aires",
    });
  });
  afterEach(() => jest.clearAllMocks());

  it("shows the View button for table bills with a table_code", async () => {
    mockGetLiveBills.mockResolvedValue([bill({ id: 1, table_code: "ABC123" })]);
    render(<LiveBills businessId="42" />);
    await waitFor(() =>
      expect(screen.getByText(/#1001/)).toBeInTheDocument(),
    );
    // Task 8: View opens operator BillDetailsModal (not the guest /t page).
    expect(screen.getByRole("button", { name: /ver/i })).toBeInTheDocument();
  });

  // Task 8 / finding 43: delivery/counter bills previously hid View because
  // the guest /t page needs a table_code. Operator drill-in uses bill id.
  it("shows View for delivery bills and opens BillDetailsModal", async () => {
    mockGetLiveBills.mockResolvedValue([
      bill({
        id: 2,
        bill_number: "1001",
        table_name: "",
        counter_id: undefined,
        table_code: "",
      }),
    ]);
    (getBill as jest.Mock).mockResolvedValue({
      bill: {
        id: 2,
        bill_number: "1001",
        status: "open",
        business_id: 42,
        paid_amount: 0,
        payments: [],
        alternative_payments: [],
      },
      items: [],
    });
    render(<LiveBills businessId="42" />);
    await waitFor(() =>
      expect(screen.getByText(/#1001/)).toBeInTheDocument(),
    );
    const viewBtn = screen.getByRole("button", { name: /ver/i });
    expect(viewBtn).toBeInTheDocument();
    await userEvent.click(viewBtn);
    await waitFor(() => {
      expect(getBill).toHaveBeenCalledWith(2, undefined, expect.any(Number));
    });
    expect(await screen.findByTestId("bill-details-modal")).toHaveAttribute(
      "data-bill-id",
      "2",
    );
  });
});

describe("LiveBills money formatting (operator locale)", () => {
  beforeEach(() => {
    mockGetBusiness.mockResolvedValue({
      default_currency: "USD",
      timezone: "America/Argentina/Buenos_Aires",
    });
  });
  afterEach(() => jest.clearAllMocks());

  it("formats live bill amounts in the operator locale (es-AR)", async () => {
    mockGetLiveBills.mockResolvedValue([
      bill({ id: 1, total_amount: 37.82, table_code: "ABC123" }),
    ]);
    render(<LiveBills businessId="9" />);
    // Intl es-AR emits USD as "US$ 37,82" with an NBSP — match whitespace-loosely.
    // A single bill's amount shows on both the bill card and the summary card,
    // so assert at least one match (findAllByText) rather than exactly one.
    expect((await screen.findAllByText(/US\$\s?37,82/)).length).toBeGreaterThan(0);
    expect(screen.queryByText("$37.82")).toBeNull();
  });

  it("formats the Estadísticas Rápidas summary in the operator locale (es-AR)", async () => {
    mockGetLiveBills.mockResolvedValue([
      bill({ id: 1, total_amount: 37.82, table_code: "ABC123" }),
      bill({ id: 2, total_amount: 73.9, table_code: "DEF456" }),
    ]);
    render(<LiveBills businessId="9" />);
    // Total value summary card sums to 111.72 → es-AR "US$ 111,72".
    expect(await screen.findByText(/US\$\s?111,72/)).toBeInTheDocument();
    expect(screen.queryByText("$111.72")).toBeNull();
  });
});

describe("LiveBills business-timezone correctness (R17)", () => {
  beforeEach(() => {
    // Buenos Aires is UTC-3, so a UTC-noon instant is 09:00 wall-clock there.
    mockGetBusiness.mockResolvedValue({
      default_currency: "USD",
      timezone: "America/Argentina/Buenos_Aires",
    });
  });
  afterEach(() => jest.clearAllMocks());

  it("renders the bill created_at in the business timezone, not device time", async () => {
    // created_at is 2026-07-01T12:00:00Z (UTC noon). America/Argentina/Buenos_Aires
    // is UTC-3, so the wall clock is 09:00 — never the raw-UTC 12:00.
    mockGetLiveBills.mockResolvedValue([
      bill({ id: 1, created_at: "2026-07-01T12:00:00Z", table_code: "ABC123" }),
    ]);
    render(<LiveBills businessId="42" />);
    // Scope the assertion to the "created" span (label + time) so the
    // non-deterministic "lastUpdated" header clock can't cause a false match.
    // Under es-AR the label is Spanish, so match on the trailing 09:00 wall clock.
    const createdSpan = await screen.findByText(
      (_content, el) =>
        el?.tagName === "SPAN" && /\b09:00\b/.test(el.textContent ?? ""),
    );
    // Business-time 09:00 present; the UTC/device 12:00 absent from that span.
    expect(createdSpan).toBeInTheDocument();
    expect(createdSpan.textContent).not.toMatch(/\b12:00\b/);
  });
});
