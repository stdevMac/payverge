/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import LiveTableGrid from "./LiveTableGrid";
import type { Table } from "@/api/business";

const tStub = (key: string, params?: Record<string, string | number>) => {
  // minutesOpen / multipleBills receive already-humanized duration in `n` / `oldest`
  if (params && key === "tableGrid.minutesOpen") return String(params.n);
  if (params && key === "tableGrid.multipleBills") {
    return `${params.count} bills · oldest ${params.oldest}`;
  }
  return key;
};

const sampleTables = [
  { id: 1, name: "T1", capacity: 4 } as unknown as Table,
  { id: 2, name: "T2", capacity: 2 } as unknown as Table,
];

describe("LiveTableGrid", () => {
  it("renders empty state when no tables", () => {
    render(
      <LiveTableGrid
        tables={[]}
        activeBillsByTableId={{}}
        onStartNewOrder={jest.fn()}
        t={tStub}
      />,
    );
    expect(screen.getByText("tableGrid.emptyTitle")).toBeInTheDocument();
  });

  it("renders all tables", () => {
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{}}
        onStartNewOrder={jest.fn()}
        t={tStub}
      />,
    );
    expect(screen.getByText("T1")).toBeInTheDocument();
    expect(screen.getByText("T2")).toBeInTheDocument();
  });

  it("shows available status for tables without active bills", () => {
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{}}
        onStartNewOrder={jest.fn()}
        t={tStub}
      />,
    );
    expect(screen.getAllByText("tableGrid.available").length).toBe(2);
  });

  it("shows occupied state when active bill exists", () => {
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{ 1: { count: 1, oldest_bill_id: 100, oldest_minutes: 14 } }}
        onStartNewOrder={jest.fn()}
        t={tStub}
      />,
    );
    expect(screen.getByText("14m")).toBeInTheDocument();
    expect(screen.getAllByText("tableGrid.occupied").length).toBeGreaterThan(0);
  });

  it("shows multi-bill label when count > 1", () => {
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{
          1: { count: 2, oldest_bill_id: 100, oldest_minutes: 32 },
        }}
        onStartNewOrder={jest.fn()}
        t={tStub}
      />,
    );
    expect(
      screen.getByText("2 bills · oldest 32m"),
    ).toBeInTheDocument();
  });

  it("humanizes multi-day occupancy (L1-9: never raw 38858 min)", () => {
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{
          1: { count: 1, oldest_bill_id: 100, oldest_minutes: 38858 },
        }}
        onStartNewOrder={jest.fn()}
        t={tStub}
      />,
    );
    expect(screen.getByText("26d")).toBeInTheDocument();
    expect(screen.queryByText(/38858/)).not.toBeInTheDocument();
  });

  it("calls onStartNewOrder with table id when clicked", () => {
    const handler = jest.fn();
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{}}
        onStartNewOrder={handler}
        t={tStub}
      />,
    );
    fireEvent.click(screen.getByText("T1").closest("button") as HTMLElement);
    expect(handler).toHaveBeenCalledWith(1);
  });

  it("calls onStartNewOrder for occupied table as well", () => {
    const handler = jest.fn();
    render(
      <LiveTableGrid
        tables={sampleTables}
        activeBillsByTableId={{ 2: { count: 1, oldest_bill_id: 200, oldest_minutes: 5 } }}
        onStartNewOrder={handler}
        t={tStub}
      />,
    );
    fireEvent.click(screen.getByText("T2").closest("button") as HTMLElement);
    expect(handler).toHaveBeenCalledWith(2);
  });
});
