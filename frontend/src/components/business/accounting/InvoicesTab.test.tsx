/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReceiptRow } from "@/api/fiscal";
import { useReceipts } from "@/hooks/accounting/useAccountingQueries";
import { listReceiptDelivery } from "@/api/fiscal";
import InvoicesTab from "./InvoicesTab";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useReceipts: jest.fn(),
  useResendReceipt: jest.fn(() => ({ mutate: jest.fn(), isPending: false })),
  useCreditNote: jest.fn(() => ({ mutate: jest.fn(), isPending: false })),
}));

jest.mock("@/components/admin/primitives", () => ({
  useDebouncedValue: (value: string) => value,
}));

const mockListReceiptsPage = jest.fn();
const mockGetSettings = jest.fn();
jest.mock("@/api/fiscal", () => ({
  fiscalApi: {
    receiptsExportUrl: jest.fn(
      () =>
        "https://api.test/fiscal/receipts/export.csv?start=2026-01-01&end=2026-01-31",
    ),
    listReceiptsPage: (...args: unknown[]) => mockListReceiptsPage(...args),
  },
  listReceiptsPage: (...args: unknown[]) => mockListReceiptsPage(...args),
  listReceiptDelivery: jest.fn(() => Promise.resolve([])),
  getSettings: (...args: unknown[]) => mockGetSettings(...args),
  receiptsExportUrl: jest.fn(
    () =>
      "https://api.test/fiscal/receipts/export.csv?start=2026-01-01&end=2026-01-31",
  ),
}));

jest.mock("../fiscal/FiscalDashboard", () => ({
  __esModule: true,
  default: ({ variant }: { variant?: string }) => (
    <div data-testid="fiscal-dashboard" data-variant={variant ?? "full"} />
  ),
}));

jest.mock("./ReceiptDetailDrawer", () => ({
  __esModule: true,
  default: ({
    open,
    receipt,
  }: {
    open: boolean;
    receipt: { id: number } | null;
  }) =>
    open && receipt ? (
      <div data-testid="receipt-detail-drawer" data-receipt-id={receipt.id} />
    ) : null,
}));

jest.mock("./IssueInvoiceDrawer", () => ({
  __esModule: true,
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="issue-invoice-drawer" /> : null,
}));

const mockExportLocalizedCsv = jest.fn();
jest.mock("@/utils/exportLocalizedCsv", () => ({
  exportLocalizedCsv: (...args: unknown[]) => mockExportLocalizedCsv(...args),
}));

const useReceiptsMock = useReceipts as jest.Mock;
const listReceiptDeliveryMock = listReceiptDelivery as jest.Mock;

const t = (key: string, params?: Record<string, string | number>) => {
  if (!params) return key;
  return Object.entries(params).reduce(
    (acc, [name, value]) =>
      acc.replace(new RegExp(`\\{${name}\\}`, "g"), String(value)),
    key,
  );
};

function receipt(
  overrides: Partial<ReceiptRow> & { id: number },
): ReceiptRow {
  const id = overrides.id;
  return {
    business_id: 42,
    settings_id: 1,
    bill_id: 100 + id,
    payment_id: null,
    alternative_payment_id: null,
    country: "AR",
    provider: "arca",
    action: "issue",
    receipt_type: "invoice_b",
    receipt_number: `0001-0000000${id}`,
    provider_receipt_id: null,
    auth_code: "12345678901234",
    auth_expires_at: null,
    qr_payload: null,
    qr_image_path: null,
    pdf_path: null,
    customer_doc_type: null,
    customer_doc_number: null,
    total_amount_cents: 12500,
    tip_amount_cents: 0,
    currency: "ARS",
    status: "authorized",
    error_code: null,
    error_message: null,
    issued_at: "2026-01-15T18:00:00Z",
    created_at: "2026-01-15T18:00:00Z",
    updated_at: "2026-01-15T18:00:00Z",
    delivery: [
      { task_id: 1, channel: "artifact", status: "succeeded" },
      { task_id: 2, channel: "email", status: "dead" },
      { task_id: 3, channel: "print", status: "succeeded" },
    ],
    needs_attention: false,
    ...overrides,
    id,
  };
}

function mockPage(
  receipts: ReceiptRow[],
  overrides: {
    total?: number;
    page?: number;
    page_size?: number;
    forParams?: (params: unknown) => boolean;
  } = {},
) {
  const page = {
    receipts,
    total: overrides.total ?? receipts.length,
    page: overrides.page ?? 1,
    page_size: overrides.page_size ?? 20,
    total_pages: Math.max(
      1,
      Math.ceil(
        (overrides.total ?? receipts.length) / (overrides.page_size ?? 20),
      ),
    ),
  };

  useReceiptsMock.mockImplementation(
    (_biz: unknown, params: Record<string, unknown> = {}) => {
      if (overrides.forParams && !overrides.forParams(params)) {
        return {
          data: {
            receipts: [],
            total: 0,
            page: 1,
            page_size: 20,
            total_pages: 1,
          },
          isLoading: false,
          isFetching: false,
          isSuccess: true,
        };
      }
      // Attention count probe (page_size 1 + needs_attention).
      if (params.needs_attention === true && params.page_size === 1) {
        return {
          data: {
            receipts: receipts.filter((r) => r.needs_attention).slice(0, 1),
            total: overrides.total ?? receipts.filter((r) => r.needs_attention).length,
            page: 1,
            page_size: 1,
            total_pages: 1,
          },
          isLoading: false,
          isFetching: false,
          isSuccess: true,
        };
      }
      return {
        data: page,
        isLoading: false,
        isFetching: false,
        isSuccess: true,
      };
    },
  );
}

async function renderTab(
  props: Partial<React.ComponentProps<typeof InvoicesTab>> = {},
) {
  const view = render(
    <InvoicesTab
      businessId="42"
      start="2026-01-01"
      end="2026-01-31"
      locale="en"
      currency="ARS"
      canWrite
      businessTimezone="America/Argentina/Buenos_Aires"
      t={t}
      {...props}
    />,
  );
  // Flush getSettings → setFiscalCountry so act() warnings stay quiet.
  await act(async () => {
    await Promise.resolve();
  });
  return view;
}

describe("InvoicesTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetSettings.mockResolvedValue({ country: "AR", provider: "arca" });
    mockPage([
      receipt({
        id: 1,
        bill_id: 501,
        receipt_number: "0001-00000001",
        needs_attention: true,
        delivery: [
          { task_id: 1, channel: "artifact", status: "succeeded" },
          { task_id: 2, channel: "email", status: "dead" },
          // print missing → warm-300
        ],
      }),
      receipt({
        id: 2,
        bill_id: 502,
        status: "pending",
        receipt_number: null,
        needs_attention: false,
        delivery: [],
      }),
    ]);
  });

  it("renders FiscalDashboard in setup-only variant", async () => {
    await renderTab();
    const dash = screen.getByTestId("fiscal-dashboard");
    expect(dash).toHaveAttribute("data-variant", "setup-only");
  });

  it("renders one-line receipt rows from useReceipts", async () => {
    await renderTab();

    expect(screen.getByText(/#501/)).toBeInTheDocument();
    expect(screen.getByText(/#502/)).toBeInTheDocument();
    expect(screen.getByText("0001-00000001")).toBeInTheDocument();
    expect(useReceiptsMock).toHaveBeenCalled();
  });

  it("renders embedded delivery dots without calling listReceiptDelivery", async () => {
    await renderTab();

    const cluster = screen.getByTestId("delivery-dots-1");
    expect(cluster).toBeInTheDocument();

    const dots = cluster.querySelectorAll("[data-delivery-channel]");
    expect(dots).toHaveLength(3);

    const pdf = cluster.querySelector('[data-delivery-channel="pdf"]');
    const email = cluster.querySelector('[data-delivery-channel="email"]');
    const print = cluster.querySelector('[data-delivery-channel="print"]');

    expect(pdf?.className).toMatch(/emerald/);
    expect(email?.className).toMatch(/rose/);
    expect(print?.className).toMatch(/warm-300/);

    expect(cluster).toHaveAttribute("aria-label");

    expect(listReceiptDeliveryMock).not.toHaveBeenCalled();
  });

  it("seeds filter from initialFilter deep-link prop", async () => {
    await renderTab({ initialFilter: "needs_attention" });

    expect(useReceiptsMock).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({ needs_attention: true }),
    );
    const attentionChip = screen.getByRole("tab", {
      name: /invoices\.filters\.needsAttention/i,
    });
    expect(attentionChip).toHaveAttribute("aria-selected", "true");
  });

  it("refires useReceipts with needs_attention when Needs attention chip is selected", async () => {
    mockPage(
      [
        receipt({
          id: 1,
          needs_attention: true,
          delivery: [
            { task_id: 2, channel: "email", status: "dead" },
          ],
        }),
      ],
      { total: 7 },
    );
    await renderTab();

    // Badge count comes from the attention probe envelope total (page_size=1).
    const attentionTab = screen.getByRole("tab", {
      name: /invoices\.filters\.needsAttention/i,
    });
    expect(attentionTab.textContent).toMatch(/7/);

    fireEvent.click(attentionTab);

    await waitFor(() => {
      const attentionCalls = useReceiptsMock.mock.calls.filter(
        (call) =>
          call[1] &&
          typeof call[1] === "object" &&
          (call[1] as { needs_attention?: boolean }).needs_attention === true &&
          (call[1] as { page_size?: number }).page_size !== 1,
      );
      expect(attentionCalls.length).toBeGreaterThan(0);
    });
  });

  it("opens detail drawer when a receipt row is clicked", async () => {
    await renderTab();

    const billCell = screen.getByText(/#501/);
    const row = billCell.closest("tr");
    expect(row).toBeTruthy();
    fireEvent.click(row!);

    await waitFor(() => {
      expect(screen.getByTestId("receipt-detail-drawer")).toBeInTheDocument();
    });
  });

  it("renders guest/table column and search, and wires q into useReceipts", async () => {
    mockPage([
      receipt({
        id: 1,
        bill_id: 501,
        customer_name: "Ada Guest",
        table_label: "Table 6",
      }),
    ]);
    renderTab();

    expect(screen.getByTestId("invoice-guest-table-1")).toHaveTextContent(
      "Table 6",
    );
    expect(screen.getByTestId("invoice-guest-table-1")).toHaveTextContent(
      "Ada Guest",
    );
    expect(screen.getByTestId("invoices-search")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("invoices-search"), {
      target: { value: "Table 6" },
    });

    await waitFor(() => {
      const listCalls = useReceiptsMock.mock.calls.filter(
        (call) =>
          call[1] &&
          typeof call[1] === "object" &&
          (call[1] as { page_size?: number }).page_size !== 1,
      );
      expect(listCalls.some((call) => (call[1] as { q?: string }).q === "Table 6")).toBe(
        true,
      );
    });
  });

  it("exposes invoice-actions test ids for the row menu trigger", async () => {
    await renderTab();
    expect(screen.getByTestId("invoice-actions-1")).toBeInTheDocument();
    expect(screen.getByTestId("invoice-actions-2")).toBeInTheDocument();
  });

  // issue 258: ⋮ used to focus and mount nothing — View/Print never appeared.
  it("opens the row ⋮ menu and View opens the receipt drawer", async () => {
    await renderTab();

    fireEvent.click(screen.getByTestId("invoice-actions-1"));

    const view = await screen.findByRole("menuitem", {
      name: "invoices.menu.view",
    });
    fireEvent.click(view);

    await waitFor(() => {
      expect(screen.getByTestId("receipt-detail-drawer")).toHaveAttribute(
        "data-receipt-id",
        "1",
      );
    });
  });

  it("shows Export CSV link via receiptsExportUrl and opens IssueInvoiceDrawer", async () => {
    await renderTab();

    const exportBtn = screen.getByTestId("invoices-export-csv");
    expect(exportBtn).toBeInTheDocument();

    expect(
      screen.queryByTestId("issue-invoice-drawer"),
    ).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: /invoices\.issueInvoice/i }),
    );

    await waitFor(() => {
      expect(screen.getByTestId("issue-invoice-drawer")).toBeInTheDocument();
    });
  });

  it("wires pagination into the next useReceipts page", async () => {
    mockPage([receipt({ id: 1 })], { total: 45, page: 1, page_size: 20 });
    await renderTab();

    mockPage([receipt({ id: 21 })], { total: 45, page: 2, page_size: 20 });

    fireEvent.click(screen.getByRole("button", { name: "Next page" }));

    await waitFor(() => {
      const lastListCall = [...useReceiptsMock.mock.calls]
        .reverse()
        .find(
          (call) =>
            call[1] &&
            typeof call[1] === "object" &&
            (call[1] as { page_size?: number }).page_size !== 1,
        );
      expect(lastListCall?.[1]).toEqual(
        expect.objectContaining({ page: 2 }),
      );
    });
  });

  // L6-20: client CSV must ship the ENTIRE filtered date range — every page
  // from the API, not just the 20 rows currently on screen — and keep the
  // tip column the removed server export carried.
  it("exports every page of the filtered range with the tip column (L6-20)", async () => {
    mockExportLocalizedCsv.mockClear();
    mockListReceiptsPage.mockReset();

    // On-screen list shows only ONE row (current page truncated view).
    mockPage([
      receipt({
        id: 10,
        bill_id: 610,
        receipt_type: "factura_a",
        needs_attention: true,
        status: "authorized",
      }),
    ]);

    // API has 4 factura_a receipts across 2 pages for the active filters.
    const pageOne = [
      receipt({
        id: 10,
        bill_id: 610,
        receipt_type: "factura_a",
        needs_attention: true,
        tip_amount_cents: 150,
      }),
      receipt({
        id: 11,
        bill_id: 611,
        receipt_type: "factura_a",
        needs_attention: false,
        tip_amount_cents: 0,
      }),
    ];
    const pageTwo = [
      receipt({
        id: 12,
        bill_id: 612,
        receipt_type: "factura_a",
        needs_attention: false,
        tip_amount_cents: 275,
      }),
      receipt({
        id: 13,
        bill_id: 613,
        receipt_type: "factura_a",
        needs_attention: true,
        tip_amount_cents: 0,
      }),
    ];
    mockListReceiptsPage.mockImplementation(
      (_biz: unknown, params: Record<string, unknown> = {}) => {
        const page = (params.page as number) ?? 1;
        const rows = page === 1 ? pageOne : page === 2 ? pageTwo : [];
        return Promise.resolve({
          receipts: rows,
          total: 4,
          page,
          page_size: params.page_size ?? 200,
          total_pages: 2,
        });
      },
    );

    await renderTab();

    // Wait for AR settings so Factura A/B/C options are mounted.
    await waitFor(() => {
      expect(
        within(screen.getByTestId("receipt-type-filter")).getByText(
          "invoices.receiptType.factura_a",
        ),
      ).toBeInTheDocument();
    });

    // Select type factura_a
    fireEvent.change(screen.getByTestId("receipt-type-filter"), {
      target: { value: "factura_a" },
    });

    await waitFor(() => {
      // list re-queried with receipt_type
      expect(useReceiptsMock).toHaveBeenCalledWith(
        "42",
        expect.objectContaining({ receipt_type: "factura_a" }),
      );
    });

    fireEvent.click(screen.getByTestId("invoices-export-csv"));

    await waitFor(() => {
      expect(mockExportLocalizedCsv).toHaveBeenCalledTimes(1);
    });

    // Export fetched from the API with the ACTIVE filters, a big page size,
    // and walked every page (1 then 2) — not the on-screen page.
    expect(mockListReceiptsPage).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({
        receipt_type: "factura_a",
        start: "2026-01-01",
        end: "2026-01-31",
        page: 1,
        page_size: 200,
      }),
    );
    expect(mockListReceiptsPage).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({ receipt_type: "factura_a", page: 2 }),
    );

    const arg = mockExportLocalizedCsv.mock.calls[0][0] as {
      rows: Array<Record<string, unknown>>;
      columns: Array<{ key: string; headerKey: string }>;
    };
    // ALL 4 rows across both pages — page truncation would give 1 or 2.
    expect(arg.rows).toHaveLength(4);
    expect(arg.rows.map((r) => r.bill_id).sort()).toEqual([610, 611, 612, 613]);
    expect(arg.rows.every((r) => r.receipt_type === "factura_a")).toBe(true);
    // needs_attention column preserved with yes/no
    expect(arg.columns.some((c) => c.key === "needs_attention")).toBe(true);
    expect(arg.rows.map((r) => r.needs_attention)).toEqual([
      "yes",
      "no",
      "no",
      "yes",
    ]);
    // Tip column restored (server export shipped total AND tip).
    expect(arg.columns.some((c) => c.key === "tip")).toBe(true);
    expect(arg.rows.map((r) => r.tip)).toEqual(["1.50", "0.00", "2.75", "0.00"]);
  });

  it("offers Invoice/Receipt type filters for a US venue (not Factura A/B/C)", async () => {
    mockGetSettings.mockResolvedValue({ country: "US", provider: "demo" });
    mockPage([
      receipt({
        id: 21,
        country: "US",
        provider: "demo",
        receipt_type: "invoice",
        receipt_number: "DEMO-21",
      }),
      receipt({
        id: 22,
        country: "US",
        provider: "demo",
        receipt_type: "receipt",
        receipt_number: "DEMO-22",
      }),
    ]);

    await renderTab();

    await waitFor(() => {
      const select = screen.getByTestId("receipt-type-filter");
      expect(
        within(select).getByText("invoices.receiptType.invoice"),
      ).toBeInTheDocument();
      expect(
        within(select).getByText("invoices.receiptType.receipt"),
      ).toBeInTheDocument();
      expect(
        within(select).queryByText("invoices.receiptType.factura_a"),
      ).not.toBeInTheDocument();
      expect(
        within(select).queryByText("invoices.receiptType.factura_b"),
      ).not.toBeInTheDocument();
      expect(
        within(select).queryByText("invoices.receiptType.factura_c"),
      ).not.toBeInTheDocument();
    });

    // Single type label — no chip + raw slug duplicate (#209).
    expect(screen.getAllByTestId("receipt-type-label")).toHaveLength(2);
    expect(screen.queryByTestId("receipt-letter-chip")).not.toBeInTheDocument();
  });

  it("does not show Factura A/B/C chips for leftover US factura rows", async () => {
    mockGetSettings.mockResolvedValue({ country: "US", provider: "demo" });
    mockPage([
      receipt({
        id: 31,
        country: "US",
        provider: "demo",
        receipt_type: "factura_a",
        receipt_number: "DEMO-31",
      }),
      receipt({
        id: 32,
        country: "US",
        provider: "demo",
        receipt_type: "factura_b",
        receipt_number: "DEMO-32",
      }),
    ]);

    await renderTab();

    await waitFor(() => {
      const select = screen.getByTestId("receipt-type-filter");
      expect(
        within(select).getByText("invoices.receiptType.invoice"),
      ).toBeInTheDocument();
    });
    expect(screen.queryByTestId("receipt-letter-chip")).not.toBeInTheDocument();
    expect(
      screen.getAllByTestId("receipt-type-label").map((el) => el.textContent),
    ).toEqual(["invoice", "receipt"]);
  });

  it("offers Factura A/B/C type filters for an AR venue", async () => {
    mockGetSettings.mockResolvedValue({ country: "AR", provider: "arca" });
    await renderTab();

    await waitFor(() => {
      const select = screen.getByTestId("receipt-type-filter");
      expect(
        within(select).getByText("invoices.receiptType.factura_a"),
      ).toBeInTheDocument();
      expect(
        within(select).getByText("invoices.receiptType.factura_b"),
      ).toBeInTheDocument();
      expect(
        within(select).getByText("invoices.receiptType.factura_c"),
      ).toBeInTheDocument();
      expect(
        within(select).queryByText("invoices.receiptType.invoice"),
      ).not.toBeInTheDocument();
      expect(
        within(select).queryByText("invoices.receiptType.receipt"),
      ).not.toBeInTheDocument();
    });
  });

  it("does not truncate the export to the on-screen page when unfiltered (L6-20)", async () => {
    mockExportLocalizedCsv.mockClear();
    mockListReceiptsPage.mockReset();

    // Screen shows page 1 of 45 (20 rows max); export must pull all 3 API pages.
    mockPage([receipt({ id: 1 })], { total: 45, page: 1, page_size: 20 });

    const mk = (ids: number[]) => ids.map((id) => receipt({ id, bill_id: 600 + id }));
    mockListReceiptsPage.mockImplementation(
      (_biz: unknown, params: Record<string, unknown> = {}) => {
        const page = (params.page as number) ?? 1;
        const rows =
          page === 1
            ? mk([1, 2])
            : page === 2
              ? mk([3, 4])
              : page === 3
                ? mk([5])
                : [];
        return Promise.resolve({
          receipts: rows,
          total: 5,
          page,
          page_size: params.page_size ?? 200,
          total_pages: 3,
        });
      },
    );

    await renderTab();
    fireEvent.click(screen.getByTestId("invoices-export-csv"));

    await waitFor(() => {
      expect(mockExportLocalizedCsv).toHaveBeenCalledTimes(1);
    });

    const arg = mockExportLocalizedCsv.mock.calls[0][0] as {
      rows: Array<Record<string, unknown>>;
    };
    expect(arg.rows).toHaveLength(5);
    expect(arg.rows.map((r) => r.bill_id)).toEqual([601, 602, 603, 604, 605]);
    expect(mockListReceiptsPage).toHaveBeenCalledTimes(3);
  });

  it("does not treat a failed receipts fetch as an empty period (#616)", async () => {
    useReceiptsMock.mockImplementation(() => ({
      data: undefined,
      isLoading: false,
      isFetching: false,
      isSuccess: false,
      isError: true,
      refetch: jest.fn(),
    }));

    await renderTab();

    expect(screen.getByText("invoices.loadError.title")).toBeInTheDocument();
    expect(screen.queryByText("invoices.empty.title")).not.toBeInTheDocument();
    expect(screen.getByTestId("invoices-retry")).toBeInTheDocument();
  });

});
