/** @jest-environment jsdom */
// P2-16 + P3: the Tables tab owns the "Print all QR codes" button, the
// celebration hand-off flag, and the "no orders yet" activation banner.

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const mockPrintAll = jest.fn();
jest.mock("../tables/useQrSheetPrint", () => ({
  useQrSheetPrint: () => ({
    printAll: mockPrintAll,
    printing: false,
    activeTableCount: 1,
  }),
}));

jest.mock("@/api/onboarding", () => ({
  getSetupStatus: jest.fn(() =>
    Promise.resolve({
      steps: {
        business_profile: { done: true, has_name: true, has_address: true, has_currency: true },
        tables: { done: true, count: 1 },
        menu: { done: true, categories: 1, items: 3 },
        staff: { done: false, count: 0 },
        payment: { done: false, count: 0, has_settlement_address: false },
      },
      completed_count: 3,
      total_count: 5,
      required_done: true,
      all_done: false,
      has_first_paid_bill: false,
    }),
  ),
}));

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
    getBusiness: jest.fn(() =>
      Promise.resolve({ id: 1, default_currency: "USD", name: "Biz" }),
    ),
  };
});

jest.mock("../tables/TableDetailModal", () => ({
  __esModule: true,
  default: () => null,
}));

import TableManager from "../TableManager";

describe("TableManager — print-all + no-orders banner", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.sessionStorage.clear();
  });

  it("renders the Print-all button and triggers the sheet", async () => {
    render(<TableManager businessId={1} />);
    const btn = await screen.findByRole("button", {
      name: /tableManager\.printAll\.button/,
    });
    fireEvent.click(btn);
    await waitFor(() => expect(mockPrintAll).toHaveBeenCalledTimes(1));
  });

  it("shows the no-orders banner (with print + preview CTAs) until the first paid bill", async () => {
    render(<TableManager businessId={1} />);
    expect(
      await screen.findByText(/tableManager\.noOrdersBanner\.title/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/tableManager\.noOrdersBanner\.body/),
    ).toBeInTheDocument();
    const preview = screen.getByRole("link", {
      name: /tableManager\.noOrdersBanner\.previewCta/,
    });
    expect(preview).toHaveAttribute("href", "/t/AI-T01");
    fireEvent.click(
      screen.getByRole("button", { name: /tableManager\.noOrdersBanner\.printCta/ }),
    );
    await waitFor(() => expect(mockPrintAll).toHaveBeenCalled());
  });

  it("consumes the celebration one-shot flag and auto-opens the print sheet once", async () => {
    window.sessionStorage.setItem("payverge_print_qr_on_open_1", "1");
    render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");
    await waitFor(() => expect(mockPrintAll).toHaveBeenCalledTimes(1));
    expect(window.sessionStorage.getItem("payverge_print_qr_on_open_1")).toBeNull();
  });
});
