/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { Receipt } from "lucide-react";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/localeRegistry";
import DataTable, {
  type DataTableColumn,
  type DataTablePagination,
} from "./DataTable";

function renderWithLocale(
  ui: React.ReactElement,
  locale: Locale = "en",
) {
  return render(
    <SimpleTranslationProvider initialLocale={locale}>{ui}</SimpleTranslationProvider>,
  );
}

type Row = { id: number; name: string; amount: string };

const rows: Row[] = [
  { id: 1, name: "Rent", amount: "$1,200.00" },
  { id: 2, name: "Utilities", amount: "$84.50" },
];

const columns: DataTableColumn<Row>[] = [
  {
    key: "name",
    header: "Description",
    render: (row) => row.name,
  },
  {
    key: "amount",
    header: "Amount",
    align: "right",
    render: (row) => row.amount,
  },
];

const emptyState = {
  icon: Receipt,
  title: "No entries yet",
  subtitle: "Create your first ledger entry.",
};

const baseProps = {
  columns,
  rows,
  rowKey: (row: Row) => row.id,
  emptyState,
  "aria-label": "Ledger entries",
};

function pagination(
  overrides: Partial<DataTablePagination> = {},
): DataTablePagination {
  return {
    page: 1,
    pageSize: 20,
    total: 143,
    onPageChange: jest.fn(),
    ...overrides,
  };
}

describe("DataTable", () => {
  it("renders a real table with sr-only caption and column headers", () => {
    renderWithLocale(<DataTable {...baseProps} />);

    const table = screen.getByRole("table", { name: "Ledger entries" });
    expect(table.tagName).toBe("TABLE");

    const caption = table.querySelector("caption");
    expect(caption).not.toBeNull();
    expect(caption).toHaveClass("sr-only");
    expect(caption).toHaveTextContent("Ledger entries");

    expect(
      screen.getByRole("columnheader", { name: "Description" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Amount" }),
    ).toBeInTheDocument();

    const amountHeader = screen.getByRole("columnheader", { name: "Amount" });
    expect(amountHeader.className).toMatch(/text-\[11px\]/);
    expect(amountHeader.className).toMatch(/uppercase/);
    expect(amountHeader.className).toMatch(/tracking-wide/);
    expect(amountHeader.className).toMatch(/text-warm-500/);
  });

  it("renders rows from column defs", () => {
    renderWithLocale(<DataTable {...baseProps} />);

    expect(screen.getByText("Rent")).toBeInTheDocument();
    expect(screen.getByText("Utilities")).toBeInTheDocument();
    expect(screen.getByText("$1,200.00")).toBeInTheDocument();
    expect(screen.getByText("$84.50")).toBeInTheDocument();
  });

  it("applies text-right tabular-nums to right-aligned cells", () => {
    renderWithLocale(<DataTable {...baseProps} />);

    const amountCell = screen.getByText("$1,200.00").closest("td");
    expect(amountCell).not.toBeNull();
    expect(amountCell!.className).toMatch(/text-right/);
    expect(amountCell!.className).toMatch(/tabular-nums/);
  });

  it("renders pageSize skeleton rows while loading (no data rows)", () => {
    const { container } = renderWithLocale(
      <DataTable
        {...baseProps}
        rows={[]}
        loading
        pagination={pagination({ pageSize: 5, total: 100 })}
      />,
    );

    expect(screen.queryByText("Rent")).not.toBeInTheDocument();
    expect(screen.queryByText("No entries yet")).not.toBeInTheDocument();

    // Skeleton lines live inside body cells — one row per pageSize entry.
    const skeletonRows = container.querySelectorAll("tbody tr");
    expect(skeletonRows).toHaveLength(5);
    expect(container.querySelectorAll(".bg-ink-100").length).toBeGreaterThan(0);
  });

  it("renders EmptyState with provided copy when empty and not loading", () => {
    renderWithLocale(<DataTable {...baseProps} rows={[]} />);

    expect(
      screen.getByRole("heading", { name: "No entries yet" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Create your first ledger entry."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("renders pagination footer range and wires onPageChange", () => {
    const onPageChange = jest.fn();
    renderWithLocale(
      <DataTable
        {...baseProps}
        pagination={pagination({ page: 1, pageSize: 20, total: 143, onPageChange })}
      />,
    );

    expect(screen.getByText("1–20 of 143")).toBeInTheDocument();

    const prev = screen.getByRole("button", { name: /previous/i });
    const next = screen.getByRole("button", { name: /next/i });

    expect(prev).toBeDisabled();
    expect(next).not.toBeDisabled();

    fireEvent.click(next);
    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  // N-1: Spanish operator locale must translate shared pagination chrome.
  // Revert-proof: asserts real rendered strings, not a source grep.
  it("translates pagination range and controls for es locale", () => {
    renderWithLocale(
      <DataTable
        {...baseProps}
        pagination={pagination({ page: 1, pageSize: 20, total: 230 })}
      />,
      "es",
    );

    expect(screen.getByText("1–20 de 230")).toBeInTheDocument();
    expect(screen.queryByText(/of 230/)).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Página anterior" }),
    ).toHaveTextContent("Anterior");
    expect(
      screen.getByRole("button", { name: "Página siguiente" }),
    ).toHaveTextContent("Siguiente");
  });

  it("hides pagination footer when total fits on one page", () => {
    renderWithLocale(
      <DataTable
        {...baseProps}
        pagination={pagination({ page: 1, pageSize: 20, total: 2 })}
      />,
    );

    expect(screen.queryByText(/of 2/)).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /previous/i }),
    ).not.toBeInTheDocument();
  });

  it("fires onRowClick on click and keyboard Enter/Space", () => {
    const onRowClick = jest.fn();
    renderWithLocale(<DataTable {...baseProps} onRowClick={onRowClick} />);

    const rentCell = screen.getByText("Rent");
    const row = rentCell.closest("tr");
    expect(row).not.toBeNull();
    expect(row).toHaveAttribute("tabIndex", "0");
    expect(row!.className).toMatch(/cursor-pointer/);

    fireEvent.click(row!);
    expect(onRowClick).toHaveBeenCalledWith(rows[0]);

    onRowClick.mockClear();
    fireEvent.keyDown(row!, { key: "Enter" });
    expect(onRowClick).toHaveBeenCalledWith(rows[0]);

    onRowClick.mockClear();
    fireEvent.keyDown(row!, { key: " " });
    expect(onRowClick).toHaveBeenCalledWith(rows[0]);
  });

  it("does not make rows interactive without onRowClick", () => {
    renderWithLocale(<DataTable {...baseProps} />);

    const row = screen.getByText("Rent").closest("tr");
    expect(row).not.toBeNull();
    expect(row).not.toHaveAttribute("tabIndex");
    expect(row!.className).not.toMatch(/cursor-pointer/);
  });

  it("renders rowActions in a trailing cell", () => {
    renderWithLocale(
      <DataTable
        {...baseProps}
        rowActionsLabel="Row actions"
        rowActions={(row) => (
          <button type="button" aria-label={`Actions for ${row.name}`}>
            ···
          </button>
        )}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Actions for Rent" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Actions for Utilities" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Row actions" })).toHaveClass(
      "w-12",
    );
  });

  it("applies hideBelow responsive classes on header and body cells", () => {
    const responsiveColumns: DataTableColumn<Row>[] = [
      {
        key: "name",
        header: "Description",
        hideBelow: "md",
        render: (row) => row.name,
      },
      {
        key: "amount",
        header: "Amount",
        align: "right",
        hideBelow: "sm",
        render: (row) => row.amount,
      },
    ];

    renderWithLocale(<DataTable {...baseProps} columns={responsiveColumns} />);

    const nameHeader = screen.getByRole("columnheader", { name: "Description" });
    const amountHeader = screen.getByRole("columnheader", { name: "Amount" });
    expect(nameHeader.className).toMatch(/hidden/);
    expect(nameHeader.className).toMatch(/md:table-cell/);
    expect(amountHeader.className).toMatch(/hidden/);
    expect(amountHeader.className).toMatch(/sm:table-cell/);

    const nameCell = screen.getByText("Rent").closest("td");
    expect(nameCell!.className).toMatch(/md:table-cell/);
  });

  it("wraps the table in overflow-x-auto", () => {
    const { container } = renderWithLocale(<DataTable {...baseProps} />);
    const scroller = container.querySelector(".overflow-x-auto");
    expect(scroller).not.toBeNull();
    expect(within(scroller as HTMLElement).getByRole("table")).toBeInTheDocument();
  });
});
