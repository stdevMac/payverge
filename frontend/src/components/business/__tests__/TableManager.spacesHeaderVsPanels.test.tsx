/** @jest-environment jsdom */
/**
 * #658 — chips vs floor-panel disagreement.
 *
 * Live dinner-rush failure: search “Table 9” on Vista en vivo, switch to
 * Espacios y mesas. Header showed `1 Mesas · 0 Disponibles · 1 Ocupadas`
 * while the Spaces strip still showed PENDIENTES DE UBICAR 10 / ASIENTOS MÁX. 40.
 *
 * TableManager.spacesHeaderCounts.test.tsx mocks SpacesOverview away, so it
 * cannot see that disagreement. This file mounts the real overview strip.
 *
 * Product fix already lives on main (`unfilteredTableCounts` when
 * tablesView === "spaces"). This test is the missing proof.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);
import { resetTestUrl } from "@/test/nextNavigationMock";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("../tables/useQrSheetPrint", () => ({
  useQrSheetPrint: () => ({
    printAll: jest.fn(),
    printing: false,
    activeTableCount: 10,
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

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("../spaces/editor/SpaceEditorPage", () => ({
  __esModule: true,
  default: () => <div data-testid="space-editor-mock" />,
}));

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      ...actual.spacesApi,
      list: jest.fn(() => Promise.resolve([])),
      summary: jest.fn(() =>
        Promise.resolve({
          summary: {
            total_spaces: 0,
            draft_spaces: 0,
            published_spaces: 0,
            archived_spaces: 0,
            unassigned_tables: 10,
            assigned_tables: 0,
          },
          unassigned_tables: [],
        }),
      ),
    },
  };
});

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
  getBusinessTables: jest.fn(() => Promise.resolve({ tables: [] })),
}));

jest.mock("../tables/TableDetailModal", () => ({
  __esModule: true,
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="table-detail-modal" /> : null,
}));

jest.mock("../tables/ServiceCallsQueue", () => ({
  __esModule: true,
  default: () => null,
}));

import TableManager from "../TableManager";
import { businessApi, getBusinessTables } from "@/api/business";

const getTablesWithStatus = businessApi.getTablesWithStatus as jest.Mock;
const getBusinessTablesMock = getBusinessTables as jest.Mock;

const NS = "businessDashboard.dashboard.tableManager";

const mk = (
  id: number,
  name: string,
  code: string,
  status: string,
) => ({
  table: {
    id,
    business_id: 1,
    name,
    table_code: code,
    capacity: 4,
    qr_code: "",
    qr_url: `/t/${code}`,
    is_active: true,
    created_at: "2026-04-15T10:00:00.000Z",
    updated_at: "2026-04-15T10:00:00.000Z",
  },
  status,
  active_bills: [],
  active_bills_count: 0,
  active_bill_physical_item_quantity: 0,
  reservations: [],
  reservations_count: 0,
});

const tenFloorTables = () =>
  Array.from({ length: 10 }, (_, i) => {
    const id = i + 1;
    const isNine = id === 9;
    return mk(
      id,
      isNine ? "Table 9" : `Table ${id}`,
      isNine ? "T-09" : `T-${String(id).padStart(2, "0")}`,
      isNine ? "occupied" : "available",
    );
  });

const statValue = (label: string) => {
  const labelEl = screen.getByText(label);
  const wrapper = labelEl.parentElement as HTMLElement;
  return wrapper.querySelector("span")?.textContent;
};

beforeEach(() => {
  resetTestUrl("/business/1/dashboard?tab=tables");
  const tables = tenFloorTables();
  getTablesWithStatus.mockReset();
  getTablesWithStatus.mockResolvedValue({ tables });
  getBusinessTablesMock.mockReset();
  getBusinessTablesMock.mockResolvedValue({
    tables: tables.map((row) => row.table),
  });
});

describe("Spaces header chips vs floor panels (#658)", () => {
  it("keeps full-floor chips after searching Table 9 then opening Spaces, matching the waiting/seats strip", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Table 9");

    fireEvent.change(screen.getByPlaceholderText(`${NS}.search.placeholder`), {
      target: { value: "Table 9" },
    });

    await waitFor(() => expect(statValue(`${NS}.hero.total`)).toBe("1"));
    expect(statValue(`${NS}.hero.available`)).toBe("0");
    expect(statValue(`${NS}.hero.occupied`)).toBe("1");

    fireEvent.click(
      screen.getByRole("tab", { name: /spacesTables\.nav\.spacesTables/ }),
    );

    const waiting = await screen.findByTestId("spaces-aggregate-waiting");
    const seats = await screen.findByTestId("spaces-aggregate-seats");
    await waitFor(() => {
      expect(waiting).toHaveTextContent("10");
      expect(seats).toHaveTextContent("40");
    });

    await waitFor(() => expect(statValue(`${NS}.hero.total`)).toBe("10"));
    expect(statValue(`${NS}.hero.available`)).toBe("9");
    expect(statValue(`${NS}.hero.occupied`)).toBe("1");

    // Live bug signature: chips stay on the Table 9 search (1) while the
    // floor strip still reads PENDIENTES DE UBICAR 10 / ASIENTOS MÁX. 40.
    expect(statValue(`${NS}.hero.total`)).not.toBe("1");
    expect(waiting.textContent?.trim()).toBe("10");
    expect(seats.textContent?.trim()).toBe("40");
  });
});
