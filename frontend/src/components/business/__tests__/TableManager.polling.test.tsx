/** @jest-environment jsdom */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { businessApi } from "@/api/business";

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

jest.mock("@/api/spaces", () => ({
  spacesApi: {
    list: jest.fn(() => Promise.resolve([])),
  },
  parseLayoutDocument: jest.fn(() => null),
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
      Promise.resolve({ id: 1, default_currency: "USD" }),
    ),
  };
});

jest.mock("../tables/TableDetailModal", () => ({
  __esModule: true,
  default: () => null,
}));

import TableManager from "../TableManager";

function setDocumentHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", {
    configurable: true,
    get: () => hidden,
  });
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (hidden ? "hidden" : "visible"),
  });
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

describe("TableManager status poll visibility", () => {
  beforeEach(() => {
    jest.useFakeTimers({ advanceTimers: true });
    setDocumentHidden(false);
    jest.clearAllMocks();
  });

  afterEach(() => {
    setDocumentHidden(false);
    jest.useRealTimers();
  });

  it("does not tick the 25s status poll while the tab is hidden", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");
    const initial = (businessApi.getTablesWithStatus as jest.Mock).mock.calls.length;

    setDocumentHidden(true);

    await act(async () => {
      jest.advanceTimersByTime(50_000);
      await Promise.resolve();
    });

    expect((businessApi.getTablesWithStatus as jest.Mock).mock.calls.length).toBe(
      initial,
    );
  });

  it("ticks the 25s status poll while the tab is visible", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");
    const initial = (businessApi.getTablesWithStatus as jest.Mock).mock.calls.length;

    await act(async () => {
      jest.advanceTimersByTime(25_000);
      await Promise.resolve();
    });

    await waitFor(() =>
      expect(
        (businessApi.getTablesWithStatus as jest.Mock).mock.calls.length,
      ).toBeGreaterThan(initial),
    );
  });
});
