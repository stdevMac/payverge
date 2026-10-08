/** @jest-environment jsdom */
/**
 * Finding 2 — deactivating a table must no longer be a permanent trapdoor.
 * The board fetches inactive tables (include_inactive=true), hides them from
 * the default view, and reveals them under an "Inactive" filter so the drawer's
 * reactivate action becomes reachable.
 */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

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

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusinessTables: jest.fn(),
    getTablesWithStatus: jest.fn(),
    createTableWithQR: jest.fn(),
    updateTableDetails: jest.fn(),
    deleteTable: jest.fn(),
    updateBusinessTable: jest.fn(),
    updateBusiness: jest.fn(),
  },
  getBusiness: jest.fn(() =>
    Promise.resolve({ id: 1, default_currency: "USD" }),
  ),
}));

jest.mock("../tables/TableDetailModal", () => ({
  __esModule: true,
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="table-detail-modal" /> : null,
}));

import TableManager from "../TableManager";
import { businessApi } from "@/api/business";

const getTablesWithStatus = businessApi.getTablesWithStatus as jest.Mock;

const mk = (id: number, name: string, code: string, active: boolean) => ({
  table: {
    id,
    business_id: 1,
    name,
    table_code: code,
    capacity: 4,
    qr_code: "",
    qr_url: `/t/${code}`,
    is_active: active,
    created_at: "2026-04-15T10:00:00.000Z",
    updated_at: "2026-04-15T10:00:00.000Z",
  },
  status: "available",
  active_bills: [],
  active_bills_count: 0,
  active_bill_physical_item_quantity: 0,
  reservations: [],
  reservations_count: 0,
});

beforeEach(() => {
  getTablesWithStatus.mockReset();
  getTablesWithStatus.mockResolvedValue({
    tables: [
      mk(1, "Active Table", "ACT-01", true),
      mk(2, "Retired Table", "RET-99", false),
    ],
  });
});

it("requests inactive tables from the backend", async () => {
  render(<TableManager businessId={1} />);
  await screen.findByText("Active Table");
  expect(getTablesWithStatus).toHaveBeenCalledWith(1, true);
});

it('includes inactive tables under "all" and isolates them under Inactive (L3-27)', async () => {
  render(<TableManager businessId={1} />);
  await screen.findByText("Active Table");

  // L3-27 audit: "Todos los estados" / all must include inactive so Lista
  // matches Mapa. Prior default hid them entirely.
  expect(screen.getByText("Retired Table")).toBeInTheDocument();

  const trigger = screen.getByRole("button", { name: /statusFilter.all/ });
  fireEvent.click(trigger);
  const option = await screen.findByRole("option", {
    name: /tableStatus.inactive/,
  });
  fireEvent.click(option);

  await waitFor(() =>
    expect(screen.getByText("Retired Table")).toBeInTheDocument(),
  );
  expect(screen.queryByText("Active Table")).not.toBeInTheDocument();
});
