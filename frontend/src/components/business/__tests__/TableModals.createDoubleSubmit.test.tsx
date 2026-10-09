/** @jest-environment jsdom */
/**
 * F32 — the Create Table button had no submitting/disabled state, so a
 * double-tap fired two create requests and produced duplicate tables.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const mockCreateTableWithQR = jest.fn();

jest.mock("@/api/business", () => ({
  businessApi: {
    getTablesWithStatus: jest.fn(() => Promise.resolve({ tables: [] })),
    getBusinessTables: jest.fn(() => Promise.resolve({ tables: [] })),
    createTableWithQR: (...a: unknown[]) => mockCreateTableWithQR(...a),
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
  default: () => null,
}));

import TableManager from "../TableManager";

const NS = "businessDashboard.dashboard.tableManager";

it("fires only one create request on a rapid double-tap (F32)", async () => {
  // Keep the create request in flight so the busy guard is exercised.
  let resolveCreate!: (v: unknown) => void;
  mockCreateTableWithQR.mockImplementation(
    () => new Promise((res) => (resolveCreate = res)),
  );

  render(<TableManager businessId={1} />);

  // Open the create modal.
  fireEvent.click(await screen.findByText(`${NS}.createTable`));

  // Name the table so the form is valid.
  const nameInput = await screen.findByPlaceholderText(
    `${NS}.modals.create.placeholder`,
  );
  fireEvent.change(nameInput, { target: { value: "Patio 1" } });

  // Double-tap the modal's Create button.
  const createBtn = screen
    .getAllByText(`${NS}.modals.create.create`)
    .map((el) => el.closest("button"))
    .find(Boolean)!;
  fireEvent.click(createBtn);
  fireEvent.click(createBtn);

  await waitFor(() => expect(mockCreateTableWithQR).toHaveBeenCalledTimes(1));

  // Button is in its loading/disabled state during the in-flight request.
  expect(createBtn).toBeDisabled();

  resolveCreate({
    id: 9,
    business_id: 1,
    name: "Patio 1",
    table_code: "T09",
    capacity: 4,
    qr_code: "",
    qr_url: "/t/T09",
    is_active: true,
    created_at: "2026-04-15T10:00:00.000Z",
    updated_at: "2026-04-15T10:00:00.000Z",
  });
});

it("disables Create until a name is entered (F32 guard)", async () => {
  mockCreateTableWithQR.mockResolvedValue({});
  render(<TableManager businessId={1} />);
  fireEvent.click(await screen.findByText(`${NS}.createTable`));

  const createBtn = screen
    .getAllByText(`${NS}.modals.create.create`)
    .map((el) => el.closest("button"))
    .find(Boolean)!;
  expect(createBtn).toBeDisabled();
});
