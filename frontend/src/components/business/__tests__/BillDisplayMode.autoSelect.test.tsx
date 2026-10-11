/** @jest-environment jsdom */
/**
 * D1 / L2-27: Pantalla de Facturas detail rail must load the default bill
 * instead of spinning forever.
 *
 * Blocker #3: the auto-select effect self-aborted when `detailLoading` (or a
 * state `hasAutoSelected` flip) sat in the effect deps — handleViewBill set
 * loading true, re-ran the effect, cleanup aborted the fetch, and
 * hasAutoSelected blocked retry. Production uses a ref latch + deps that omit
 * detailLoading, plus a request-id gate on handleViewBill.
 *
 * Revert-proof (DOM): re-adding `detailLoading` to the effect deps makes
 * getBill abort and this suite fail — closeDetailPanel never appears.
 * Pure `shouldAutoSelectBillDetail` unit tests alone are not sufficient.
 *
 * The mocked getBill honors the AbortSignal exactly like the axios path does,
 * so an aborted request rejects with a CanceledError instead of resolving.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

const mockGetBill = jest.fn();

jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  getBill: (...args: unknown[]) => mockGetBill(...args),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock("@/components/business/BillItemEditor", () => ({
  BillItemEditor: () => null,
}));

// Needs a QueryClientProvider; irrelevant to the auto-select contract.
jest.mock("@/components/business/BillRecordPayment", () => ({
  BillRecordPayment: () => null,
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (value: number) => `$${value}`,
}));

import BillDisplayMode from "@/components/business/BillDisplayMode";
import type { Bill } from "@/api/bills";

const openBill: Bill = {
  id: 77,
  business_id: 42,
  table_id: 7,
  bill_number: "B-AUTOSELECT",
  notes: "",
  items: "[]",
  subtotal: 10,
  tax_amount: 0,
  service_fee_amount: 0,
  total_amount: 10,
  paid_amount: 0,
  tip_amount: 0,
  currency: "USD",
  status: "open",
  settlement_address: "0x",
  tipping_address: "0x",
  created_at: new Date(Date.now() - 5 * 60_000).toISOString(),
  updated_at: new Date().toISOString(),
  physical_item_quantity: 1,
} as Bill;

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBill.mockImplementation(
    (_billId: number, signal?: AbortSignal) =>
      new Promise((resolve, reject) => {
        const timer = setTimeout(
          () => resolve({ bill: openBill, items: [], history: [] }),
          10,
        );
        signal?.addEventListener("abort", () => {
          clearTimeout(timer);
          const err = new Error("canceled") as Error & { code?: string };
          err.name = "CanceledError";
          err.code = "ERR_CANCELED";
          reject(err);
        });
      }),
  );
});

it("loads the default selection instead of aborting its own fetch", async () => {
  render(
    <BillDisplayMode
      bills={[openBill]}
      orders={{}}
      onExit={jest.fn()}
      onCloseBill={jest.fn()}
      onCreateBill={jest.fn()}
      onBillUpdated={jest.fn()}
      businessId={42}
      tString={(key: string) => key}
      currency="USD"
    />,
  );

  await waitFor(() => expect(mockGetBill).toHaveBeenCalledWith(77, expect.anything()));

  // The detail rail only renders once selectedBill is populated — proof the
  // auto-select fetch actually resolved rather than being cancelled.
  await waitFor(() =>
    expect(
      screen.getByLabelText("display.closeDetailPanelAria"),
    ).toBeInTheDocument(),
  );

  // Spinner must clear — stuck detailLoading was the L2-27 symptom ("spins forever").
  expect(document.querySelector('[data-testid="bill-detail-spinner"]')).toBeNull();
  expect(screen.queryByRole("status")).toBeNull();

  // And it must not have been torn down by the effect's own cleanup.
  const signal = mockGetBill.mock.calls[0][1] as AbortSignal;
  expect(signal.aborted).toBe(false);
});
