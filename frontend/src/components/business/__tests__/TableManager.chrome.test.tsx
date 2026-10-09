/** @jest-environment jsdom */
/**
 * Related to #188 — Spaces is a sub-view of Tables, not a different page.
 * H1 stays "Tables", Print all / Create Table stay, and the summary strip stays.
 */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { resetTestUrl } from "@/test/nextNavigationMock";

jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

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
    summary: jest.fn(() =>
      Promise.resolve({ summary: {}, unassigned_tables: [] }),
    ),
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

jest.mock("../spaces/SpacesOverview", () => ({
  __esModule: true,
  default: () => <div data-testid="spaces-overview" />,
}));

jest.mock("../tables/ServiceCallsQueue", () => ({
  __esModule: true,
  default: () => null,
}));

import TableManager from "../TableManager";

const TITLE_KEY = "businessDashboard.dashboard.tableManager.title";
const SPACES_TITLE_KEY = "spacesTables.overview.title";
const SPACES_NAV_KEY = "spacesTables.nav.spacesTables";
const LIVE_NAV_KEY = "spacesTables.nav.liveView";

function expectTablesChrome() {
  expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
    TITLE_KEY,
  );
  expect(screen.queryByRole("heading", { name: SPACES_TITLE_KEY })).toBeNull();
  expect(
    screen.getByRole("button", {
      name: /tableManager\.printAll\.button/,
    }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: /tableManager\.createTable/ }),
  ).toBeInTheDocument();
  expect(
    screen.getByText("businessDashboard.dashboard.tableManager.hero.total"),
  ).toBeInTheDocument();
}

describe("TableManager chrome across Live / Spaces (#188)", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=tables");
  });

  it("keeps the Tables H1, actions, and summary strip on Live", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");
    expectTablesChrome();
    const liveTab = screen.getByRole("tab", { name: new RegExp(LIVE_NAV_KEY) });
    expect(liveTab).toHaveAttribute("aria-selected", "true");
    expect(liveTab.textContent).toContain(LIVE_NAV_KEY);
  });

  it("keeps the same page chrome when switching to Spaces & Tables", async () => {
    render(<TableManager businessId={1} />);
    await screen.findByText("Main Room 1");

    fireEvent.click(
      screen.getByRole("tab", { name: new RegExp(SPACES_NAV_KEY) }),
    );

    await waitFor(() =>
      expect(
        screen.getByRole("tab", { name: new RegExp(SPACES_NAV_KEY) }),
      ).toHaveAttribute("aria-selected", "true"),
    );
    expect(await screen.findByTestId("spaces-overview")).toBeInTheDocument();
    expectTablesChrome();
    // Inactive Live tab label stays fully present (white-pill must not clip it).
    expect(
      screen.getByRole("tab", { name: new RegExp(LIVE_NAV_KEY) }),
    ).toHaveTextContent(LIVE_NAV_KEY);
  });
});
