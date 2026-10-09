/** @jest-environment jsdom */
import { render, fireEvent, screen } from "@testing-library/react";
import TableRow from "../TableRow";

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

const baseTable = {
  id: 37,
  name: "Main Room 1",
  table_code: "AI-T01",
  is_active: true,
  status: "occupied" as const,
  capacity: 4,
  active_bill: {
    id: 99,
    total: 380,
    physical_item_quantity: 2,
    created_at: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
  },
  server_name: "Alex Server",
  next_reservation: null,
  last_seen: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
};

function renderRow(
  table: typeof baseTable,
  props: { onSelect?: jest.Mock; currency?: string } = {},
) {
  return render(
    <table>
      <tbody>
        <TableRow
          table={table}
          onSelect={props.onSelect ?? jest.fn()}
          currency={props.currency ?? "USD"}
        />
      </tbody>
    </table>,
  );
}

describe("TableRow", () => {
  it("renders table_code, name, status, current bill, and last-seen", () => {
    renderRow(baseTable);
    // The "#" column was leaking the DB primary key (e.g. "37") — now we
    // show the operator-facing `table_code` instead so staff recognize
    // their own QR codes.
    expect(screen.getByText("AI-T01")).toBeInTheDocument();
    expect(screen.getByText("Main Room 1")).toBeInTheDocument();
    expect(screen.getByText(/occupied/i)).toBeInTheDocument();
    // Currency formatting goes through Intl.NumberFormat now instead of
    // a hardcoded "AED " prefix, so a USD business reads "$380.00".
    expect(screen.getByText(/\$380\.00/)).toBeInTheDocument();
    // Seated + last-seen both use relative age for an open bill.
    expect(screen.getAllByText(/2 min ago/i).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Alex Server")).toBeInTheDocument();
  });

  it("formats bill item counts without prepared-units jargon", () => {
    render(<TableRow table={baseTable} onSelect={jest.fn()} currency="USD" />);
    expect(screen.getByText(/2 items/i)).toBeInTheDocument();
    expect(screen.queryByText(/prepared unit/i)).not.toBeInTheDocument();
  });

  it("links to the open bill for host checkout", () => {
    render(<TableRow table={baseTable} onSelect={jest.fn()} currency="USD" />);
    // Two entry points since #794: the Receipt icon action and the Current
    // Bill amount itself. Both must land on the same bill.
    const billLinks = screen.getAllByRole("link", { name: /open bill/i });
    expect(billLinks.length).toBeGreaterThanOrEqual(2);
    for (const link of billLinks) {
      expect(link).toHaveAttribute("href", "?tab=bills&billId=99");
    }
  });

  it("keeps the table name and current bill on one line", () => {
    renderRow(baseTable);
    const name = screen.getByTestId("table-name");
    const bill = screen.getByTestId("table-current-bill");
    expect(name).toHaveTextContent("Main Room 1");
    expect(bill).toHaveTextContent(/\$380\.00/);
    expect(bill).toHaveTextContent(/2 items/);
    expect(name.className).toMatch(/whitespace-nowrap/);
    expect(bill.className).toMatch(/whitespace-nowrap/);
    expect(`${name.className} ${bill.className}`).not.toMatch(
      /writing-mode|vertical|[\\s:]rotate-/,
    );
  });

  it("truncates long table codes so names stay readable", () => {
    const longCode = {
      ...baseTable,
      table_code: "DEMO-75-AI-PRO-TABLE-01",
      name: "Patio Window",
    };
    const { container } = render(
      <TableRow table={longCode} onSelect={jest.fn()} currency="USD" />,
    );
    const codeCell = container.querySelector("td span.truncate");
    expect(codeCell).not.toBeNull();
    expect(codeCell).toHaveAttribute("title", "DEMO-75-AI-PRO-TABLE-01");
    expect(screen.getByText("Patio Window")).toBeInTheDocument();
  });

  it("surfaces upcoming reservation time on reserved tables", () => {
    const reserved = {
      ...baseTable,
      status: "reserved" as const,
      active_bill: null,
      server_name: null,
      last_seen: null,
      next_reservation: {
        id: 7,
        customer_name: "Demo Reservation",
        party_size: 4,
        reservation_time: "2026-08-11T19:00:00.000Z",
        status: "confirmed",
      },
    };
    render(
      <TableRow
        table={reserved}
        onSelect={jest.fn()}
        currency="USD"
        businessTimezone="UTC"
      />,
    );
    // Capacity and covers both render "4" — assert covers column via party size
    // secondary reservation line plus status chip.
    expect(screen.getAllByText("4").length).toBeGreaterThanOrEqual(2);
    // Status chip + "Reserved {time}" secondary line both match /reserved/i.
    expect(screen.getAllByText(/reserved/i).length).toBeGreaterThanOrEqual(2);
  });

  it("formats the active-bill total in the provided currency", () => {
    renderRow(baseTable, { currency: "EUR" });
    // EUR Intl format renders as "€380.00" — just assert the symbol so
    // the test is locale-tolerant.
    expect(screen.getByText(/€/)).toBeInTheDocument();
  });

  it("calls onSelect when row is clicked", () => {
    const onSelect = jest.fn();
    renderRow(baseTable, { onSelect });
    fireEvent.click(screen.getByRole("row"));
    expect(onSelect).toHaveBeenCalledWith(baseTable);
  });

  it("exposes a preview-as-guest link that opens /t/{tableCode} in a new tab", () => {
    render(<TableRow table={baseTable} onSelect={jest.fn()} currency="USD" />);
    const link = screen.getByRole("link", { name: /preview|guest|live/i });
    expect(link).toHaveAttribute("href", "/t/AI-T01");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("uses last_seen when the open check has no created_at, never 'Open check warning'", () => {
    render(
      <table>
        <tbody>
          <TableRow
            table={{
              ...baseTable,
              last_seen: new Date(Date.now() - 12 * 60 * 1000).toISOString(),
              active_bill: {
                id: 9,
                total: 408.55,
                physical_item_quantity: 0,
                created_at: null,
              },
            }}
            onSelect={jest.fn()}
            currency="USD"
          />
        </tbody>
      </table>,
    );
    const seated = screen.getByTestId("table-seated-age");
    expect(seated.textContent).toMatch(/12 min ago/i);
    expect(seated.textContent).not.toMatch(/open check warning/i);
    expect(seated.getAttribute("title") || "").not.toBe("Open check warning");
  });

  it("shows seated-ago time, not the missing-key leaf 'Open check warning'", () => {
    render(
      <table>
        <tbody>
          <TableRow
            table={{
              ...baseTable,
              active_bill: {
                id: 9,
                total: 408.55,
                physical_item_quantity: 0,
                created_at: new Date(Date.now() - 130 * 60 * 1000).toISOString(),
              },
            }}
            onSelect={jest.fn()}
            currency="USD"
          />
        </tbody>
      </table>,
    );
    const seated = screen.getByTestId("table-seated-age");
    expect(seated.textContent).toMatch(/h ago|min ago|d ago|just now/i);
    expect(seated.textContent).not.toMatch(/open check warning/i);
    expect(seated.textContent).not.toBe("Open check warning");
    // Real aging.openCheckWarning copy — never the sentence-cased leaf.
    expect(seated.getAttribute("title")).toBe(
      "This check has been open a long time",
    );
  });

  it("uses last_seen when created_at is a Go-zero sentinel", () => {
    render(
      <table>
        <tbody>
          <TableRow
            table={{
              ...baseTable,
              last_seen: new Date(Date.now() - 18 * 60 * 1000).toISOString(),
              active_bill: {
                id: 9,
                total: 408.55,
                physical_item_quantity: 0,
                created_at: "0001-01-01T00:00:00Z",
              },
            }}
            onSelect={jest.fn()}
            currency="USD"
          />
        </tbody>
      </table>,
    );
    const seated = screen.getByTestId("table-seated-age");
    expect(seated.textContent).toMatch(/18 min ago/i);
    expect(seated.textContent).not.toMatch(/open check warning/i);
  });

  it("shows venue-native unknown when an occupied table has no usable timestamp", () => {
    render(
      <table>
        <tbody>
          <TableRow
            table={{
              ...baseTable,
              last_seen: null,
              active_bill: {
                id: 9,
                total: 408.55,
                physical_item_quantity: 0,
                created_at: null,
              },
            }}
            onSelect={jest.fn()}
            currency="USD"
          />
        </tbody>
      </table>,
    );
    const seated = screen.getByTestId("table-seated-age");
    expect(seated.textContent).toBe("Unknown");
    expect(seated.getAttribute("title")).toBeNull();
    expect(seated.textContent).not.toMatch(/open check warning/i);
    expect(seated.textContent).not.toBe("Seated unknown");
  });

  it("ages long-open seated checks with a warning tone", () => {
    render(
      <table>
        <tbody>
          <TableRow
            table={{
              ...baseTable,
              active_bill: {
                id: 9,
                total: 408.55,
                physical_item_quantity: 0,
                created_at: new Date(Date.now() - 130 * 60 * 1000).toISOString(),
              },
            }}
            onSelect={jest.fn()}
            currency="USD"
          />
        </tbody>
      </table>,
    );
    const seated = screen.getByTestId("table-seated-age");
    expect(seated.className).toMatch(/rose/);
  });

  it("renders table status through the shared StatusChip", () => {
    render(
      <TableRow
        table={{ ...baseTable, status: "available", active_bill: null, server_name: null }}
        onSelect={jest.fn()}
        currency="USD"
      />,
    );
    const chip = document.querySelector('[data-status-kind="table"]');
    expect(chip).not.toBeNull();
    expect(chip).toHaveAttribute("data-status", "available");
  });

  it("surfaces host floor actions when Live View wires businessId", () => {
    render(
      <TableRow
        table={baseTable}
        onSelect={jest.fn()}
        currency="USD"
        businessId={42}
        hostTargets={[]}
        onHostActionComplete={jest.fn()}
      />,
    );
    expect(screen.getByTestId("table-host-actions")).toBeInTheDocument();
  });
});
