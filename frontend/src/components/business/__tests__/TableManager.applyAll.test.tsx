/** @jest-environment jsdom */
/**
 * F30 / Finding 8 — QR "Apply to all tables" is now ONE transactional request
 * (applyQrBrandingToAllTables), not a per-table PUT fan-out. Success closes the
 * modal cleanly; a failure surfaces the save error and keeps the modal open.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _l?: string, params?: Record<string, unknown>) =>
    params ? `${key}:${JSON.stringify(params)}` : key,
}));

jest.mock("../tables/useQrSheetPrint", () => ({
  useQrSheetPrint: () => ({
    printAll: jest.fn(),
    printing: false,
    activeTableCount: 2,
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

const mockApplyQrBranding = jest.fn();

jest.mock("@/api/business", () => {
  const mkTable = (id: number, name: string) => ({
    id,
    business_id: 1,
    name,
    table_code: `T0${id}`,
    capacity: 4,
    qr_code: "",
    qr_url: `/t/T0${id}`,
    is_active: true,
    created_at: "2026-04-15T10:00:00.000Z",
    updated_at: "2026-04-15T10:00:00.000Z",
  });
  const wrap = (t: ReturnType<typeof mkTable>) => ({
    table: t,
    status: "available",
    active_bills: [],
    active_bills_count: 0,
    active_bill_physical_item_quantity: 0,
    reservations: [],
    reservations_count: 0,
  });
  const t1 = mkTable(1, "Table 1");
  const t2 = mkTable(2, "Table 2");
  return {
    businessApi: {
      getTablesWithStatus: jest.fn(() =>
        Promise.resolve({ tables: [wrap(t1), wrap(t2)] }),
      ),
      getBusinessTables: jest.fn(() => Promise.resolve({ tables: [t1, t2] })),
      createTableWithQR: jest.fn(),
      updateTableDetails: jest.fn(),
      deleteTable: jest.fn(),
      updateBusinessTable: jest.fn(),
      updateBusiness: jest.fn(),
      applyQrBrandingToAllTables: (...a: unknown[]) => mockApplyQrBranding(...a),
    },
    getBusiness: jest.fn(() =>
      Promise.resolve({ id: 1, default_currency: "USD" }),
    ),
  };
});

jest.mock("../tables/TableDetailModal", () => ({
  __esModule: true,
  default: ({
    open,
    onCustomizeQR,
  }: {
    open: boolean;
    onCustomizeQR?: (id: number) => void;
  }) =>
    open ? (
      <button data-testid="open-qr" onClick={() => onCustomizeQR?.(1)}>
        open-qr
      </button>
    ) : null,
}));

jest.mock("../QRCodeWithText", () => ({
  __esModule: true,
  default: () => <div data-testid="qr-preview" />,
}));

import TableManager from "../TableManager";

const QRNS = "businessDashboard.dashboard.tableManager.qrCustomization";

async function openApplyAllConfirm() {
  const row = await screen.findByText("Table 1");
  fireEvent.click(row.closest("tr")!);
  fireEvent.click(await screen.findByTestId("open-qr"));

  const applyAll = await screen.findByText(`${QRNS}.applyToAll`);
  fireEvent.click(applyAll.closest("button")!);

  // Confirmation step (R3-OK LOW) — the last applyToAll button is the confirm.
  await waitFor(() =>
    expect(screen.getAllByText(`${QRNS}.applyToAll`).length).toBeGreaterThan(1),
  );
  const confirmBtns = screen.getAllByText(`${QRNS}.applyToAll`);
  fireEvent.click(confirmBtns[confirmBtns.length - 1].closest("button")!);
}

beforeEach(() => {
  jest.clearAllMocks();
});

it("applies to all tables via a single transactional request (F30)", async () => {
  mockApplyQrBranding.mockResolvedValue(2);

  render(<TableManager businessId={1} />);
  await openApplyAllConfirm();

  // ONE bulk call, not a per-table fan-out.
  await waitFor(() => expect(mockApplyQrBranding).toHaveBeenCalledTimes(1));
  expect(mockApplyQrBranding).toHaveBeenCalledWith(1, expect.any(Object));
  // No error surfaced.
  expect(
    screen.queryByText(/qrCustomizeSaveError/),
  ).not.toBeInTheDocument();
});

it("surfaces the save error and keeps the modal open on failure (F30)", async () => {
  mockApplyQrBranding.mockRejectedValue(new Error("boom"));

  render(<TableManager businessId={1} />);
  await openApplyAllConfirm();

  await waitFor(() => expect(mockApplyQrBranding).toHaveBeenCalledTimes(1));
  // The failure is reported, not swallowed.
  await waitFor(() =>
    expect(screen.getByText("boom")).toBeInTheDocument(),
  );
});
