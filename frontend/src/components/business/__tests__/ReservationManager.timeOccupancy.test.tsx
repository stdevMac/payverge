/** @jest-environment jsdom */

import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ReservationTablePicker } from "../reservations/ReservationTablePicker";
import { ReservationTimeField } from "../reservations/ReservationTimeField";

const timeLabels = {
  entryMode: "Reservation time basis",
  businessTime: "Business time",
  deviceTime: "Device time",
  dateTime: "Date and time",
  conversionPreview: "Time conversion before save",
};

describe("reservation time and occupancy controls", () => {
  it("defaults to business time and previews the device conversion", () => {
    render(
      <ReservationTimeField
        value="2026-07-18T15:00"
        mode="business"
        businessTimeZone="America/New_York"
        deviceTimeZone="America/Argentina/Buenos_Aires"
        labels={timeLabels}
        onValueChange={jest.fn()}
        onModeChange={jest.fn()}
      />,
    );
    expect(screen.getByRole("radio", { name: "Business time" })).toBeChecked();
    expect(
      screen.getByText(/4:00 PM.*America\/Argentina\/Buenos_Aires/),
    ).toBeVisible();
  });

  it("requires acknowledgement before selecting an occupied near-term table", async () => {
    const onSelect = jest.fn();
    const onOverrideChange = jest.fn();
    render(
      <ReservationTablePicker
        options={[
          {
            id: 1,
            name: "Table 1",
            capacity: 4,
            occupancy_state: "occupied",
            active_bill_id: 71,
            active_bill_age_minutes: 90,
            reservation_available: true,
            recommended: false,
            requires_occupancy_override: true,
            conflict_reason: "occupied",
          },
        ]}
        selectedTableId={undefined}
        overrideAcknowledged={false}
        labels={{
          available: "Available",
          occupied: "Occupied — active bill",
          staleOccupied: "Stale occupied — old active bill",
          override: "Override active bill conflict for this reservation",
          overrideRequired: "Acknowledge the active bill before saving",
          reservationConflict: "Already reserved for this time",
          capacityConflict: "Too small for this party",
        }}
        onSelect={onSelect}
        onOverrideChange={onOverrideChange}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: /Table 1.*occupied/i }),
    );
    expect(
      screen.getByRole("checkbox", {
        name: /override active bill conflict/i,
      }),
    ).toBeVisible();
    expect(onOverrideChange).not.toHaveBeenCalledWith(true);
  });
});
