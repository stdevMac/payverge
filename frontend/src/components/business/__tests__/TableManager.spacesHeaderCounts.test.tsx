/** @jest-environment jsdom */
/**
 * Spaces header counters must stay unfiltered totals. Live View search is not
 * visible on Espacios y mesas, so leaking it makes the floor look empty.
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
    activeTableCount: 3,
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
    summary: jest.fn(() =>
      Promise.resolve({ summary: {}, unassigned_tables: [] }),
    ),
  },
  parseLayoutDocument: jest.fn(() => null),
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

jest.mock("../spaces/SpacesOverview", () => ({
  __esModule: true,
  default: () => <div data-testid="spaces-overview" />,
}));

jest.mock("../tables/ServiceCallsQueue", () => ({
  __esModule: true,
  default: () => null,
}));

import TableManager from "../TableManager";
import { businessApi } from "@/api/business";

const getTablesWithStatus = businessApi.getTablesWithStatus as jest.Mock;

const NS = "businessDashboard.dashboard.tableManager";

const mk = (
  id: number,
  name: string,
  code: string,
  active: boolean,
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
    is_active: active,
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

const statValue = (label: string) => {
  const labelEl = screen.getByText(label);
  const wrapper = labelEl.parentElement as HTMLElement;
  return wrapper.querySelector("span")?.textContent;
};

beforeEach(() => {
  resetTestUrl("/business/1/dashboard?tab=tables");
  getTablesWithStatus.mockReset();
  getTablesWithStatus.mockResolvedValue({
    tables: [
      mk(1, "Patio One", "PAT-01", true, "available"),
      mk(2, "Table 9", "T-09", true, "occupied"),
      mk(3, "Patio Three", "PAT-03", true, "reserved"),
      mk(4, "Retired Patio", "RET-99", false, "available"),
    ],
  });
});

describe("Spaces header counters ignore Live View search", () => {
  it("restores unfiltered totals after searching Table 9 then opening Spaces", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Table 9");

    fireEvent.change(screen.getByPlaceholderText(`${NS}.search.placeholder`), {
      target: { value: "Table 9" },
    });

    await waitFor(() => expect(statValue(`${NS}.hero.total`)).toBe("1"));
    expect(statValue(`${NS}.hero.occupied`)).toBe("1");
    expect(statValue(`${NS}.hero.available`)).toBe("0");

    fireEvent.click(
      screen.getByRole("tab", { name: /spacesTables\.nav\.spacesTables/ }),
    );

    await screen.findByTestId("spaces-overview");
    await waitFor(() => expect(statValue(`${NS}.hero.total`)).toBe("4"));
    expect(statValue(`${NS}.hero.available`)).toBe("1");
    expect(statValue(`${NS}.hero.occupied`)).toBe("1");
    expect(statValue(`${NS}.hero.reserved`)).toBe("1");
  });

  it("ignores a leftover tableSearch deep-link when opening Spaces from the URL (#658)", async () => {
    resetTestUrl(
      "/business/1/dashboard?tab=tables&tablesView=spaces&tableSearch=Table%209",
    );
    render(<TableManager businessId={1} />);
    await screen.findByTestId("spaces-overview");
    await waitFor(() => expect(statValue(`${NS}.hero.total`)).toBe("4"));
    expect(statValue(`${NS}.hero.available`)).toBe("1");
    expect(statValue(`${NS}.hero.occupied`)).toBe("1");
    expect(statValue(`${NS}.hero.reserved`)).toBe("1");
  });
});
