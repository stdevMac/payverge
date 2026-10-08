/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";

import type { ReservationTableOption } from "@/api/reservations";
import { ReservationTablePicker } from "./ReservationTablePicker";

const labels = {
  available: "Available",
  occupied: "Occupied",
  staleOccupied: "Stale",
  override: "Override",
  overrideRequired: "Required",
  reservationConflict: "Conflict",
  capacityConflict: "Capacity",
  unassignedSpace: "Unassigned",
};

function option(
  partial: Partial<ReservationTableOption> & Pick<ReservationTableOption, "id" | "name">,
): ReservationTableOption {
  return {
    capacity: 4,
    occupancy_state: "available",
    reservation_available: true,
    recommended: false,
    requires_occupancy_override: false,
    ...partial,
  };
}

describe("ReservationTablePicker", () => {
  it("renders a flat list when no options have a space", () => {
    render(
      <ReservationTablePicker
        options={[option({ id: 1, name: "T1" }), option({ id: 2, name: "T2" })]}
        overrideAcknowledged={false}
        labels={labels}
        onSelect={jest.fn()}
        onOverrideChange={jest.fn()}
      />,
    );
    expect(screen.getByTestId("reservation-table-picker-flat")).toBeInTheDocument();
    expect(
      screen.queryByTestId("reservation-table-picker-grouped"),
    ).toBeNull();
  });

  it("groups by space when any option is space-linked", () => {
    render(
      <ReservationTablePicker
        options={[
          option({ id: 1, name: "Patio 1", space_id: 10, space_name: "Patio" }),
          option({ id: 2, name: "Main 1", space_id: 20, space_name: "Main" }),
          option({ id: 3, name: "Bar", space_id: null }),
        ]}
        overrideAcknowledged={false}
        labels={labels}
        onSelect={jest.fn()}
        onOverrideChange={jest.fn()}
      />,
    );
    expect(
      screen.getByTestId("reservation-table-picker-grouped"),
    ).toBeInTheDocument();
    expect(screen.getByText("Patio")).toBeInTheDocument();
    expect(screen.getByText("Main")).toBeInTheDocument();
    expect(screen.getByText("Unassigned")).toBeInTheDocument();
  });

  it("does not let the host pick a table under party capacity", () => {
    render(
      <ReservationTablePicker
        options={[
          option({
            id: 10,
            name: "Table 10",
            capacity: 2,
            reservation_available: false,
            conflict_reason: "capacity",
          }),
          option({ id: 2, name: "Table 2", capacity: 4 }),
        ]}
        overrideAcknowledged={false}
        labels={labels}
        onSelect={jest.fn()}
        onOverrideChange={jest.fn()}
      />,
    );
    expect(
      screen.getByRole("button", { name: /Table 10/i }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /Table 2/i }),
    ).not.toBeDisabled();
  });
});
