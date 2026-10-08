/** @jest-environment jsdom */
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import OutstandingTab from "./OutstandingTab";
import { useUnpaidBills } from "@/hooks/accounting/useAccountingQueries";
import { getBill } from "@/api/bills";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useUnpaidBills: jest.fn(),
}));

jest.mock("@/api/bills", () => ({
  getBill: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  getTranslation: (key: string) => key,
  // DataTable (N-1) calls useSimpleLocale for pagination chrome.
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
}));

jest.mock("@/components/business/BillDetailsModal", () => ({
  BillDetailsModal: ({
    isOpen,
    mode,
    bill,
  }: {
    isOpen: boolean;
    mode?: string;
    bill: { bill?: { id: number } } | null;
  }) =>
    isOpen ? (
      <div data-testid="bill-details-modal" data-mode={mode ?? "default"}>
        bill:{bill?.bill?.id ?? "none"}
      </div>
    ) : null,
}));

const mockUseUnpaid = useUnpaidBills as jest.Mock;
const mockGetBill = getBill as jest.Mock;

function interpolate(
  key: string,
  params?: Record<string, string | number>,
): string {
  if (!params) return key;
  let value = key;
  Object.entries(params).forEach(([name, replacement]) => {
    const token = `{${name}}`;
    value = value.includes(token)
      ? value.replace(new RegExp(`\\{${name}\\}`, "g"), String(replacement))
      : `${value} ${replacement}`;
  });
  return value;
}

function renderTab(
  props: Partial<React.ComponentProps<typeof OutstandingTab>> = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <OutstandingTab
        businessId="42"
        start="2026-05-01"
        end="2026-05-31"
        locale="en"
        currency="USD"
        t={interpolate}
        {...props}
      />
    </QueryClientProvider>,
  );
}

function unpaidQuery(bills: Array<Record<string, unknown>>, total = bills.length) {
  return {
    data: { bills, total },
    isPending: false,
    isFetching: false,
    isError: false,
    refetch: jest.fn(),
  };
}

describe("OutstandingTab L6-23 drilldown", () => {
  beforeEach(() => {
    mockUseUnpaid.mockReset();
    mockGetBill.mockReset();
  });

  it("states as-of the end date and older debt for a bill created before the range start", () => {
    mockUseUnpaid.mockReturnValue(
      unpaidQuery([
        {
          id: 755,
          created_at: "2026-07-08T12:00:00Z",
          table_label: "T1",
          total: 36.68,
          paid: 0,
          outstanding: 36.68,
          status: "closed",
          age_days: 36,
        },
      ]),
    );

    renderTab({ start: "2026-07-16", end: "2026-08-14" });

    const scope = screen.getByTestId("outstanding-scope");
    expect(scope).toHaveTextContent("outstanding.asOf");
    expect(scope).toHaveTextContent("Aug 14, 2026");
    expect(screen.getByText("#755")).toBeInTheDocument();
  });

  it("hides the as-of older-debt note when there are no outstanding bills", () => {
    mockUseUnpaid.mockReturnValue(unpaidQuery([], 0));
    renderTab({ start: "2026-07-16", end: "2026-08-14" });
    expect(screen.queryByTestId("outstanding-scope")).toBeNull();
    expect(screen.getByText("outstanding.empty")).toBeInTheDocument();
  });

  it("opens operator BillDetailsModal when a row is activated", async () => {
    mockUseUnpaid.mockReturnValue(
      unpaidQuery([
        {
          id: 99,
          created_at: "2026-03-01T12:00:00Z",
          table_label: "T1",
          total: 100,
          paid: 0,
          outstanding: 100,
          status: "open",
          age_days: 70,
        },
      ]),
    );
    mockGetBill.mockResolvedValue({
      bill: { id: 99, bill_number: "B-99", status: "open" },
      items: [],
      history: [],
    });

    renderTab();

    // DataTable rows are keyboard-activatable when onRowClick is set.
    const row = screen.getByText("#99").closest("tr");
    expect(row).toBeTruthy();
    fireEvent.click(row!);

    await waitFor(() => expect(mockGetBill).toHaveBeenCalledWith(99, undefined, 50));
    const modal = await screen.findByTestId("bill-details-modal");
    expect(modal).toHaveAttribute("data-mode", "operator");
    expect(modal).toHaveTextContent("bill:99");
  });

  // #900: the row now names its own currency. The tab used to format every
  // amount with summary.currency, which falls back to "USD" whenever the
  // summary has not resolved — printing an ARS outstanding book as dollars.
  it("formats an outstanding amount in the currency the row names", () => {
    mockUseUnpaid.mockReturnValue(
      unpaidQuery([
        {
          id: 1702,
          created_at: "2026-05-10T12:00:00Z",
          table_id: 4,
          table_label: "Mesa 4",
          total: 1702,
          paid: 0,
          outstanding: 1702,
          currency: "ARS",
          status: "open",
          age_days: 3,
        },
      ]),
    );

    // The stale prop still says USD — the row must win.
    renderTab({ currency: "USD" });

    const cell = screen.getByText("#1702").closest("tr");
    expect(cell).toBeTruthy();
    expect(cell!.textContent).not.toContain("$1,702.00");
    expect(cell!.textContent).toMatch(/ARS|\$\s?1\.702,00/);
  });
});
