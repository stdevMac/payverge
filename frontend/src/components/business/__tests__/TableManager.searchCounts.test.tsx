/** @jest-environment jsdom */
/**
 * L3-27 — the hero KPI chips must describe the rows the operator is looking at.
 *
 * The chips were counted from the full row set while the list was
 * search-filtered, so a search that matched one table still advertised
 * "12 tables · 7 available". Faceted counting: the search narrows both, the
 * status facet narrows only the list (otherwise every chip but the selected
 * one would read 0).
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

/** Reads the numeric value rendered next to a hero stat label. */
const statValue = (label: string) => {
  const labelEl = screen.getByText(label);
  const wrapper = labelEl.parentElement as HTMLElement;
  return wrapper.querySelector("span")?.textContent;
};

beforeEach(() => {
  getTablesWithStatus.mockReset();
  getTablesWithStatus.mockResolvedValue({
    tables: [
      mk(1, "Patio One", "PAT-01", true, "available"),
      mk(2, "Bar Two", "BAR-02", true, "occupied"),
      mk(3, "Patio Three", "PAT-03", true, "reserved"),
      mk(4, "Retired Patio", "RET-99", false, "available"),
    ],
  });
});

const search = async (query: string) => {
  render(<TableManager businessId={1} />);
  await screen.findByText("Patio One");
  fireEvent.change(screen.getByPlaceholderText(`${NS}.search.placeholder`), {
    target: { value: query },
  });
};

describe("L3-27 hero counts follow the search", () => {
  it("counts only the tables the search matched", async () => {
    await search("Bar");

    await waitFor(() =>
      expect(screen.queryByText("Patio One")).not.toBeInTheDocument(),
    );
    expect(screen.getByText("Bar Two")).toBeInTheDocument();

    expect(statValue(`${NS}.hero.total`)).toBe("1");
    expect(statValue(`${NS}.hero.available`)).toBe("0");
    expect(statValue(`${NS}.hero.occupied`)).toBe("1");
    expect(statValue(`${NS}.hero.reserved`)).toBe("0");
  });

  it("splits the matched set across the status chips", async () => {
    await search("Patio");

    // L3-27: inactive matches count in total; available/occupied/reserved
    // chips remain active-only. "Retired Patio" + 2 active = total 3.
    await waitFor(() => expect(statValue(`${NS}.hero.total`)).toBe("3"));
    expect(statValue(`${NS}.hero.available`)).toBe("1");
    expect(statValue(`${NS}.hero.occupied`)).toBe("0");
    expect(statValue(`${NS}.hero.reserved`)).toBe("1");
  });

  it("keeps the Inactive filter option reachable while searching", async () => {
    // The option is gated on inactive tables EXISTING, not on the current
    // search matching one — otherwise it can vanish while it is the selected
    // key and leave the Select showing nothing.
    await search("Bar");

    fireEvent.click(screen.getByRole("button", { name: new RegExp("statusFilter.all") }));
    expect(
      await screen.findByRole("option", { name: /tableStatus.inactive/ }),
    ).toBeInTheDocument();
  });
});
