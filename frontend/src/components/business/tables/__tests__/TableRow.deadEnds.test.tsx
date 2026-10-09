/** @jest-environment jsdom */
/**
 * Related to issue 794: occupied rows must never be dead ends. A table with
 * an open check gets an obvious "open this bill" tap on the Current Bill
 * cell itself; a leftover-kitchen table (occupied, no bill) points at its
 * kitchen tickets instead of rendering a mute em dash.
 */
import { render, screen, within } from "@testing-library/react";
import TableRow, { type TableRowData } from "../TableRow";

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

const baseTable: TableRowData = {
  id: 37,
  name: "T1",
  table_code: "CORE-T01",
  is_active: true,
  status: "occupied" as const,
  capacity: 4,
  active_bill: {
    id: 99,
    total: 38.5,
    physical_item_quantity: 2,
    created_at: new Date(Date.now() - 20 * 60 * 1000).toISOString(),
  },
  server_name: null,
  next_reservation: null,
  last_seen: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
};

function renderRow(table: typeof baseTable) {
  return render(
    <table>
      <tbody>
        <TableRow table={table} onSelect={jest.fn()} currency="USD" />
      </tbody>
    </table>,
  );
}

describe("TableRow dead-end fixes (issue 794)", () => {
  it("makes the Current Bill amount an open-the-bill link", () => {
    renderRow(baseTable);
    const cell = screen.getByTestId("table-current-bill");
    const link = within(cell).getByRole("link");
    expect(link).toHaveAttribute("href", "?tab=bills&billId=99");
    expect(link).toHaveTextContent(/\$38\.50/);
  });

  it("links leftover-kitchen occupied tables to their kitchen tickets", () => {
    renderRow({ ...baseTable, active_bill: null });
    const cell = screen.getByTestId("table-current-bill");
    const link = within(cell).getByRole("link");
    // O5: the row does not know the leftover ticket's status (it can be
    // approved / in_kitchen / ready), so the deep link must land on the
    // unfiltered queue — a status filter could show an empty list.
    expect(link).toHaveAttribute("href", "?tab=kitchen&kitchenStatus=all");
    expect(link).toHaveTextContent(/in kitchen/i);
    expect(cell.textContent).not.toContain("—");
  });

  it("keeps the em dash on available tables with no bill", () => {
    renderRow({
      ...baseTable,
      status: "available" as const,
      active_bill: null,
    });
    const cell = screen.getByTestId("table-current-bill");
    expect(within(cell).queryByRole("link")).toBeNull();
    expect(cell.textContent).toContain("—");
  });
});
