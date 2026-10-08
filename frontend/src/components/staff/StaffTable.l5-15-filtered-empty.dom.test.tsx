/** @jest-environment jsdom */
/**
 * D1 / L5-15: filtered-to-zero staff must show "no matches" framing, never the
 * cold-start "no team yet" invite copy. Source greps of isFiltered= are pass-on-revert.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import StaffTable from "./StaffTable";

const tString = (key: string) => {
  const map: Record<string, string> = {
    "table.noStaffYet": "No team yet",
    "table.inviteFirstStaff": "Invite your first staff member",
    "table.noStaffFound": "No staff match this search",
    "table.adjustSearch": "Try a different name or clear the filter",
    "buttons.inviteStaff": "Invite staff",
    "search.clear": "Clear search",
  };
  return map[key] ?? key;
};

describe("StaffTable L5-15 filtered empty DOM (D1)", () => {
  it("shows filtered empty title when isFiltered and list is empty", () => {
    const onClear = jest.fn();
    render(
      <StaffTable
        staff={[]}
        getRoleLabel={(r) => r}
        getRoleDescription={() => ""}
        tString={tString}
        formatDate={() => ""}
        handleUpdateRole={jest.fn()}
        handleRemoveStaff={jest.fn()}
        onInviteClick={jest.fn()}
        isFiltered
        filterQuery="zzz"
        onClearFilter={onClear}
        filteredEmptyTitle="No staff match this search"
        filteredEmptyBody="Try a different name or clear the filter"
      />,
    );

    expect(screen.getByText("No staff match this search")).toBeInTheDocument();
    expect(
      screen.getByText("Try a different name or clear the filter"),
    ).toBeInTheDocument();
    expect(screen.queryByText("No team yet")).not.toBeInTheDocument();
    expect(screen.queryByText("Invite your first staff member")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /clear search/i }));
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it("shows cold-start invite framing only when not filtered", () => {
    render(
      <StaffTable
        staff={[]}
        getRoleLabel={(r) => r}
        getRoleDescription={() => ""}
        tString={tString}
        formatDate={() => ""}
        handleUpdateRole={jest.fn()}
        handleRemoveStaff={jest.fn()}
        onInviteClick={jest.fn()}
        isFiltered={false}
      />,
    );

    expect(screen.getByText("No team yet")).toBeInTheDocument();
    expect(screen.queryByText("No staff match this search")).not.toBeInTheDocument();
  });
});
