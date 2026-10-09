/** @jest-environment jsdom */
/**
 * Task 2.3 — TableManager rebuilt as list + drawer.
 *
 * The grid toggle and the absolutely-positioned QR cards are gone. The
 * tab now renders a single tabular list, and clicking a row opens the
 * TableDetailDrawer (Overview / QR / Settings).
 */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

let mockLocale = "en";

// i18n: echo keys verbatim so assertions stay deterministic.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale, setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

// Keep new print-all / setup-status imports inert in list layout tests.
jest.mock("../tables/useQrSheetPrint", () => ({
  useQrSheetPrint: () => ({
    printAll: jest.fn(),
    printing: false,
    activeTableCount: 1,
  }),
}));

jest.mock("@/api/onboarding", () => ({
  getSetupStatus: jest.fn(() =>
    Promise.resolve({
      has_first_paid_bill: true,
      steps: {},
      completed_count: 5,
      total_count: 5,
      required_done: true,
      all_done: true,
    }),
  ),
}));

jest.mock("@/api/spaces", () => ({
  spacesApi: {
    list: jest.fn(() => Promise.resolve([])),
  },
  parseLayoutDocument: jest.fn(() => null),
}));

// businessApi: return one table that maps cleanly into TableRowData.
jest.mock("@/api/business", () => {
  const table = {
    id: 37,
    business_id: 1,
    name: "Main Room 1",
    table_code: "AI-T01",
    capacity: 4,
    qr_code: "",
    qr_url: "/t/AI-T01",
    is_active: true,
    created_at: "2026-04-15T10:00:00.000Z",
    updated_at: "2026-04-15T10:00:00.000Z",
  };
  // The list now renders from the status-aware endpoint, which returns each
  // table wrapped in a TableWithStatus envelope.
  const tableWithStatus = {
    table,
    status: "available",
    active_bills: [],
    active_bills_count: 0,
    reservations: [],
    reservations_count: 0,
  };
  return {
    businessApi: {
      getBusinessTables: jest.fn(() => Promise.resolve({ tables: [table] })),
      getTablesWithStatus: jest.fn(() =>
        Promise.resolve({ tables: [tableWithStatus] }),
      ),
      createTableWithQR: jest.fn(),
      updateTableDetails: jest.fn(),
      deleteTable: jest.fn(),
      updateBusinessTable: jest.fn(),
      updateBusiness: jest.fn(),
    },
    // TableManager now resolves the business's currency (for the
    // active-bill column) in parallel with the tables query.
    getBusiness: jest.fn(() =>
      Promise.resolve({ id: 1, default_currency: "USD" }),
    ),
  };
});

// Stub the detail modal so we don't need to render the full QR stack
// (which pulls in the qrcode library + a canvas) just to assert that
// clicking a row opens it.
jest.mock("../tables/TableDetailModal", () => ({
  __esModule: true,
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="table-detail-modal" /> : null,
}));

import TableManager from "../TableManager";
import { businessApi } from "@/api/business";

describe("TableManager (list layout)", () => {
  beforeEach(() => {
    mockLocale = "en";
    jest.clearAllMocks();
  });
  it("renders a list of tables and opens the detail modal on row click", async () => {
    render(<TableManager businessId={1} />);
    const row = await screen.findByText("Main Room 1");
    fireEvent.click(row.closest("tr")!);
    expect(
      await screen.findByTestId("table-detail-modal"),
    ).toBeInTheDocument();
  });

  it("keeps NAME and CURRENT BILL on one line instead of table-fixed crush", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");
    const table = screen.getByTestId("tables-live-table");
    expect(table.className).not.toMatch(/table-fixed/);
    expect(table.className).toMatch(/min-w-\[72rem\]/);
    expect(table.className).not.toMatch(/writing-mode|vertical/);
    const headers = Array.from(table.querySelectorAll("th"));
    expect(headers.length).toBeGreaterThanOrEqual(2);
    headers.forEach((th) => {
      expect(th.className).toMatch(/whitespace-nowrap/);
    });
    const name = screen.getByTestId("table-name");
    const bill = screen.getByTestId("table-current-bill");
    expect(name.className).toMatch(/whitespace-nowrap/);
    expect(bill.className).toMatch(/whitespace-nowrap/);
  });

  it("no longer has a Grid toggle", async () => {
    render(<TableManager businessId={1} />);
    // wait for the load so the page has settled past the loading state
    await screen.findByText("Main Room 1");
    expect(screen.queryByRole("button", { name: /^Grid$/i })).not.toBeInTheDocument();
  });

  it("refreshes active table status when the operator locale changes", async () => {
    const { rerender } = render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");
    const callsBeforeSwitch = (businessApi.getTablesWithStatus as jest.Mock)
      .mock.calls.length;

    mockLocale = "es-AR";
    rerender(<TableManager businessId={1} />);

    await waitFor(() =>
      expect(
        (businessApi.getTablesWithStatus as jest.Mock).mock.calls.length,
      ).toBeGreaterThan(callsBeforeSwitch),
    );
  });
});
