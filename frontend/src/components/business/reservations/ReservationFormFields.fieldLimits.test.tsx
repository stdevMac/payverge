/**
 * @jest-environment jsdom
 *
 * H-relay: the staff reservation form caps guest free text at the API limits
 * (backend/internal/services/reservation_field_limits.go).
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { ReservationFormFields } from "./ReservationFormFields";
import type { CreateReservationRequest } from "@/api/reservations";
import {
  RESERVATION_CUSTOMER_NAME_MAX_LENGTH,
  RESERVATION_CUSTOMER_PHONE_MAX_LENGTH,
  RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH,
} from "./reservationFieldLimits";

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

describe("ReservationFormFields field limits", () => {
  it("matches the API caps and sets maxLength on name, phone and requests", () => {
    expect(RESERVATION_CUSTOMER_NAME_MAX_LENGTH).toBe(80);
    expect(RESERVATION_CUSTOMER_PHONE_MAX_LENGTH).toBe(32);
    expect(RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH).toBe(300);

    const formData: CreateReservationRequest = {
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
        onChange={jest.fn()}
        labels={baseLabels}
        timeEntryMode="business"
        onTimeEntryModeChange={jest.fn()}
        businessTimeZone="UTC"
        deviceTimeZone="UTC"
        tableOptions={[]}
        occupancyOverrideAcknowledged={false}
        onOccupancyOverrideChange={jest.fn()}
        onTableClearedForPartySize={jest.fn()}
      />,
    );

    expect(screen.getByLabelText(/^name/i)).toHaveAttribute("maxlength", "80");
    expect(screen.getByLabelText(/^phone/i)).toHaveAttribute("maxlength", "32");
    expect(screen.getByLabelText(/^requests/i)).toHaveAttribute("maxlength", "300");
  });
});
