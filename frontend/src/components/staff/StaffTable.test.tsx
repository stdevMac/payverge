/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import StaffTable from "./StaffTable";
import type { StaffMember } from "../../api/staff";

const staff: StaffMember[] = [
  {
    id: 7,
    name: "Dana",
    email: "dana@x.com",
    role: "server",
    business_id: 1,
    is_active: true,
    created_at: "2026-07-01",
    updated_at: "",
  } as StaffMember,
  {
    id: 8,
    name: "Sam",
    email: "sam@x.com",
    role: "server",
    business_id: 1,
    is_active: false,
    created_at: "2026-07-01",
    updated_at: "",
  } as StaffMember,
];

describe("StaffTable", () => {
  it("keeps role descriptions out of the rows (tooltip only) — no repeated wallpaper", () => {
    render(
      <StaffTable
        staff={staff}
        getRoleLabel={(r) => `label:${r}`}
        getRoleDescription={(r) => `desc:${r}`}
        tString={(k) => k}
        formatDate={(d) => d}
        handleUpdateRole={jest.fn()}
        handleRemoveStaff={jest.fn()}
      />,
    );
    // The chip renders per row…
    expect(screen.getAllByText("label:server")).toHaveLength(2);
    // …but the long description is NOT repeated as row text.
    expect(screen.queryByText("desc:server")).not.toBeInTheDocument();
    // It stays reachable as a hover tooltip on the role cell.
    expect(screen.getAllByTitle("desc:server")).toHaveLength(2);
  });

  it("translates the staff table aria-label via tString", () => {
    render(
      <StaffTable
        staff={staff}
        getRoleLabel={(r) => `label:${r}`}
        getRoleDescription={(r) => `desc:${r}`}
        tString={(k) => k}
        formatDate={(d) => d}
        handleUpdateRole={jest.fn()}
        handleRemoveStaff={jest.fn()}
      />,
    );
    expect(screen.getByLabelText("table.tableAria")).toBeInTheDocument();
  });

  it("keeps email secondary under the name — never its own widest column (#189)", () => {
    render(
      <StaffTable
        staff={[
          {
            ...staff[0],
            email: "demo+admin8-business75-staff1@payverge.local",
          },
        ]}
        getRoleLabel={(r) => `label:${r}`}
        getRoleDescription={(r) => `desc:${r}`}
        tString={(k) => k}
        formatDate={(d) => d}
        handleUpdateRole={jest.fn()}
        handleRemoveStaff={jest.fn()}
      />,
    );
    expect(screen.queryByText("table.columns.email")).not.toBeInTheDocument();
    expect(screen.getByText("table.columns.name")).toBeInTheDocument();
    expect(screen.getByText("table.columns.role")).toBeInTheDocument();
    const email = screen.getByTitle(
      "demo+admin8-business75-staff1@payverge.local",
    );
    expect(email).toHaveClass("truncate");
    expect(email.tagName).toBe("P");
  });

  it("keeps inactive staff visible and lets Manage open their lifecycle controls", () => {
    const handleUpdateRole = jest.fn();
    render(
      <StaffTable
        staff={staff}
        getRoleLabel={(r) => `label:${r}`}
        getRoleDescription={(r) => `desc:${r}`}
        tString={(k) => k}
        formatDate={(d) => d}
        handleUpdateRole={handleUpdateRole}
        handleRemoveStaff={jest.fn()}
      />,
    );

    expect(screen.getByText("table.inactive")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "table.manageStaffNamed Sam" }));
    expect(handleUpdateRole).toHaveBeenCalledWith(staff[1]);
  });

  it("names Manage and Remove after the target staff member (#450)", () => {
    render(
      <StaffTable
        staff={staff}
        getRoleLabel={(r) => `label:${r}`}
        getRoleDescription={(r) => `desc:${r}`}
        tString={(k) => k}
        formatDate={(d) => d}
        handleUpdateRole={jest.fn()}
        handleRemoveStaff={jest.fn()}
      />,
    );
    expect(
      screen.getByRole("button", { name: "table.manageStaffNamed Dana" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "table.removeStaffNamed Dana" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "table.manageStaffNamed Sam" }),
    ).toBeInTheDocument();
    expect(screen.queryAllByRole("button", { name: /^table.manageStaffNamed$/ })).toHaveLength(0);
  });
});
