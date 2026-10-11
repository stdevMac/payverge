/** @jest-environment jsdom */
/**
 * L3-25 — the name cap must come from tableNameLimits, not a literal.
 *
 * Both name inputs hardcoded `maxLength={64}` and a `x/64` counter while the
 * shared constant lived in tableNameLimits.ts (and drives the backend binding
 * tag). Two copies of a limit drift: raise the BE cap and the inputs silently
 * keep truncating at the old one.
 *
 * The constant is mocked to a value that is NOT 64 so the assertion fails
 * against a hardcoded literal — a test written against 64 would pass either
 * way and prove nothing.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("../tableNameLimits", () => ({ TABLE_NAME_MAX_LENGTH: 17 }));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

import TableModals from "../TableModals";
import TableDetailModal from "../tables/TableDetailModal";

const table = {
  id: 1,
  business_id: 1,
  name: "Patio",
  table_code: "PAT-01",
  capacity: 4,
  qr_code: "",
  qr_url: "/t/PAT-01",
  is_active: true,
  created_at: "2026-04-15T10:00:00.000Z",
  updated_at: "2026-04-15T10:00:00.000Z",
};

describe("L3-25 name cap is sourced from tableNameLimits", () => {
  it("create-table input uses the shared constant", () => {
    render(
      <TableModals
        isCreateOpen
        onCreateOpenChange={jest.fn()}
        tableName="ab"
        setTableName={jest.fn()}
        tableCapacity={4}
        setTableCapacity={jest.fn()}
        handleCreateTable={jest.fn()}
      />,
    );

    const input = screen.getByTestId("create-table-name") as HTMLInputElement;
    expect(input.maxLength).toBe(17);
    expect(screen.getByText("2/17")).toBeInTheDocument();
  });

  it("rename input uses the shared constant", () => {
    render(
      <TableDetailModal
        open
        onClose={jest.fn()}
        businessId={1}
        table={table as never}
        onRename={jest.fn()}
      />,
    );

    const input = screen.getByTestId("rename-table-name") as HTMLInputElement;
    expect(input.maxLength).toBe(17);
    expect(screen.getByText(/5\/17/)).toBeInTheDocument();
  });
});
