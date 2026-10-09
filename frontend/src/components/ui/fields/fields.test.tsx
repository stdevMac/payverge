/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { TimeField } from "./TimeField";
import { PositionSelect } from "./PositionSelect";
import { StaffSelect } from "./StaffSelect";
import type { Position } from "@/api/positions";
import type { StaffMember } from "@/api/staff";

const positions: Position[] = [
  { id: 9, business_id: 1, name: "Server", color_hex: "#1a6b6a", department: "FOH", is_active: true, sort_order: 0 },
  { id: 10, business_id: 1, name: "Cook", color_hex: "#b45309", department: "BOH", is_active: true, sort_order: 1 },
];

const staff: StaffMember[] = [
  {
    id: 7,
    name: "Dana Ruiz",
    email: "dana@x.com",
    role: "server",
    business_id: 1,
    is_active: true,
    created_at: "",
    updated_at: "",
  },
  {
    id: 8,
    name: "Sam Lee",
    email: "sam@x.com",
    role: "host",
    business_id: 1,
    is_active: true,
    created_at: "",
    updated_at: "",
  },
];

describe("TimeField", () => {
  it("renders the value and reports changes", () => {
    const onChange = jest.fn();
    render(<TimeField label="Start" value="09:00" onChange={onChange} />);
    const input = screen.getByDisplayValue("09:00") as HTMLInputElement;
    expect(input.type).toBe("time");
    fireEvent.change(input, { target: { value: "10:30" } });
    expect(onChange).toHaveBeenCalledWith("10:30");
  });
});

describe("PositionSelect", () => {
  it("shows the placeholder when nothing is selected", () => {
    render(<PositionSelect label="Position" positions={positions} value={null} onChange={jest.fn()} placeholder="Pick one" />);
    expect(screen.getByText("Pick one")).toBeInTheDocument();
  });

  it("renders the selected position name in the trigger", () => {
    render(<PositionSelect label="Position" positions={positions} value={10} onChange={jest.fn()} />);
    // renderValue surfaces the chosen role's name (NextUI also keeps it in the
    // hidden option list, so assert presence rather than uniqueness).
    expect(screen.getAllByText("Cook").length).toBeGreaterThan(0);
  });
});

describe("StaffSelect", () => {
  it("shows the selected staff member's name", () => {
    render(<StaffSelect label="Assignee" staff={staff} value={7} onChange={jest.fn()} unassignedLabel="Open shift" />);
    expect(screen.getByDisplayValue("Dana Ruiz")).toBeInTheDocument();
  });

  it("shows the unassigned label when value is null", () => {
    render(<StaffSelect label="Assignee" staff={staff} value={null} onChange={jest.fn()} unassignedLabel="Open shift" />);
    expect(screen.getByDisplayValue("Open shift")).toBeInTheDocument();
  });
});
