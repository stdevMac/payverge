/**
 * @jest-environment jsdom
 *
 * L1-11: party-size change must clear an undersized table and surface why.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { ReservationFormFields } from "./ReservationFormFields";
import type { CreateReservationRequest, ReservationTableOption } from "@/api/reservations";

const baseLabels = {
  customerInfo: "Customer",
  customerName: "Name",
  customerPhone: "Phone",
  customerEmail: "Email",
  reservationDetails: "Details",
  partySize: "Party size",
  dateTime: "When",
  duration: "Duration",
  minutes: "min",
  additionalInfo: "More",
  specialRequests: "Requests",
  specialRequestsPlaceholder: "",
  notes: "Notes",
  notesPlaceholder: "",
  timeEntry: {
    modeLabel: "Time mode",
    business: "Business",
    device: "Device",
    preview: "Preview",
  },
  occupancy: {
    available: "Available",
    occupied: "Occupied",
    staleOccupied: "Stale",
    override: "Override",
    overrideRequired: "Required",
    reservationConflict: "Conflict",
    capacityConflict: "Too small",
    tableClearedForPartySize:
      "The selected table is too small for this party size. Choose another table.",
  },
};

const table2: ReservationTableOption = {
  id: 10,
  name: "Mesa 2",
  capacity: 2,
  reservation_available: true,
  recommended: true,
  occupancy_state: "available",
  requires_occupancy_override: false,
};

describe("ReservationFormFields party-size table invalidation (L1-11)", () => {
  it("clears table_id and notifies when party size exceeds selected capacity", () => {
    const onChange = jest.fn();
    const onCleared = jest.fn();
    const formData: CreateReservationRequest = {
      table_id: 10,
      customer_name: "Ada",
      customer_phone: "+1",
      customer_email: "",
      party_size: 2,
      reservation_time: "2026-08-10T19:00",
      duration: 120,
      special_requests: "",
      notes: "",
    };

    render(
      <ReservationFormFields
        formData={formData}
        onChange={onChange}
        labels={baseLabels}
        timeEntryMode="business"
        onTimeEntryModeChange={jest.fn()}
        businessTimeZone="UTC"
        deviceTimeZone="UTC"
        tableOptions={[table2]}
        occupancyOverrideAcknowledged={false}
        onOccupancyOverrideChange={jest.fn()}
        onTableClearedForPartySize={onCleared}
      />,
    );

    const partyInput = screen.getByLabelText(/party size/i);
    fireEvent.change(partyInput, { target: { value: "6" } });

    expect(onChange).toHaveBeenCalled();
    const next = onChange.mock.calls[onChange.mock.calls.length - 1][0];
    expect(next.party_size).toBe(6);
    expect(next.table_id).toBeUndefined();
    expect(onCleared).toHaveBeenCalledWith(
      baseLabels.occupancy.tableClearedForPartySize,
    );
  });

  it("keeps table_id when capacity still fits the new party size", () => {
    const onChange = jest.fn();
    const onCleared = jest.fn();
    const formData: CreateReservationRequest = {
      table_id: 10,
      customer_name: "Ada",
      customer_phone: "+1",
      customer_email: "",
      party_size: 2,
      reservation_time: "2026-08-10T19:00",
      duration: 120,
      special_requests: "",
      notes: "",
    };

    render(
      <ReservationFormFields
        formData={formData}
        onChange={onChange}
        labels={baseLabels}
        timeEntryMode="business"
        onTimeEntryModeChange={jest.fn()}
        businessTimeZone="UTC"
        deviceTimeZone="UTC"
        tableOptions={[{ ...table2, capacity: 8 }]}
        occupancyOverrideAcknowledged={false}
        onOccupancyOverrideChange={jest.fn()}
        onTableClearedForPartySize={onCleared}
      />,
    );

    fireEvent.change(screen.getByLabelText(/party size/i), {
      target: { value: "4" },
    });

    const next = onChange.mock.calls[onChange.mock.calls.length - 1][0];
    expect(next.party_size).toBe(4);
    // table_id not in patch when kept — form still has previous selection via parent
    expect(next.table_id).toBe(10);
    expect(onCleared).not.toHaveBeenCalled();
  });
});
