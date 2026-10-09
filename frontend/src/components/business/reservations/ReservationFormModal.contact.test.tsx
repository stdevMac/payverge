/** @jest-environment jsdom */
/**
 * Create and edit reservation modals share formData (ReservationManager).
 * Phone/email must survive sequential typing in both modes.
 */
import React, { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CreateReservationRequest } from "@/api/reservations";
import {
  ReservationFormModal,
  type ReservationFormModalProps,
} from "./ReservationFormModal";
import type { ReservationFormFieldLabels } from "./ReservationFormFields";

const labels: ReservationFormFieldLabels = {
  customerInfo: "Customer",
  customerName: "Name",
  customerPhone: "Phone",
  customerEmail: "Email",
  reservationDetails: "Details",
  partySize: "Party",
  dateTime: "When",
  duration: "Duration",
  minutes: "min",
  additionalInfo: "More",
  specialRequests: "Requests",
  specialRequestsPlaceholder: "",
  notes: "Notes",
  notesPlaceholder: "",
  phoneInvalid: "Enter a valid phone number (digits only, at least 7).",
  phoneRequired: "Phone number is required.",
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
    capacityConflict: "Capacity",
  },
};

const emptyForm: CreateReservationRequest = {
  customer_name: "",
  customer_phone: "",
  customer_email: "",
  party_size: 2,
  reservation_time: "2026-08-10T19:00",
  duration: 90,
  special_requests: "",
  notes: "",
  table_id: undefined,
};

function DualModalHarness({
  open,
  initial = emptyForm,
}: {
  open: "create" | "edit";
  initial?: CreateReservationRequest;
}) {
  const [form, setForm] = useState<CreateReservationRequest>(initial);
  const shared: Pick<
    ReservationFormModalProps,
    | "formData"
    | "onFormDataChange"
    | "fieldLabels"
    | "timeEntryMode"
    | "onTimeEntryModeChange"
    | "businessTimeZone"
    | "deviceTimeZone"
    | "tableOptions"
    | "occupancyOverrideAcknowledged"
    | "onOccupancyOverrideChange"
    | "isSubmitting"
    | "onSubmit"
    | "onClose"
  > = {
    formData: form,
    onFormDataChange: (next) => setForm((current) => ({ ...current, ...next })),
    fieldLabels: labels,
    timeEntryMode: "business",
    onTimeEntryModeChange: jest.fn(),
    businessTimeZone: "UTC",
    deviceTimeZone: "UTC",
    tableOptions: [],
    occupancyOverrideAcknowledged: false,
    onOccupancyOverrideChange: jest.fn(),
    isSubmitting: false,
    onSubmit: jest.fn(),
    onClose: jest.fn(),
  };

  return (
    <>
      <ReservationFormModal
        mode="create"
        isOpen={open === "create"}
        title="Create"
        subtitle="Add"
        cancelLabel="Cancel"
        submitLabel="Create"
        {...shared}
      />
      <ReservationFormModal
        mode="edit"
        isOpen={open === "edit"}
        title="Edit"
        subtitle="Save"
        cancelLabel="Cancel"
        submitLabel="Save"
        {...shared}
      />
      <output data-testid="harness-phone-state">{form.customer_phone}</output>
      <output data-testid="harness-email-state">{form.customer_email}</output>
    </>
  );
}

describe("ReservationFormModal contact retention (create + edit)", () => {
  it("create modal retains sequential phone and email", async () => {
    const user = userEvent.setup();
    render(<DualModalHarness open="create" />);

    const phone = await screen.findByTestId("reservation-customer-phone");
    await user.type(phone, "5551112222");
    expect(phone).toHaveValue("5551112222");
    expect(screen.getByTestId("harness-phone-state")).toHaveTextContent(
      "5551112222",
    );

    const email = screen.getByTestId("reservation-customer-email");
    await user.type(email, "ada@example.com");
    expect(email).toHaveValue("ada@example.com");
    expect(phone).toHaveValue("5551112222");
    expect(screen.getByTestId("harness-email-state")).toHaveTextContent(
      "ada@example.com",
    );
  });

  it("edit modal retains appended phone digits and email edits", async () => {
    const user = userEvent.setup();
    render(
      <DualModalHarness
        open="edit"
        initial={{
          ...emptyForm,
          customer_name: "Ada",
          customer_phone: "5551112",
          customer_email: "ada@ex.com",
        }}
      />,
    );

    const phone = await screen.findByTestId("reservation-customer-phone");
    expect(phone).toHaveValue("5551112");
    await user.type(phone, "222");
    expect(phone).toHaveValue("5551112222");
    expect(screen.getByTestId("harness-phone-state")).toHaveTextContent(
      "5551112222",
    );

    const email = screen.getByTestId("reservation-customer-email");
    await user.type(email, ".ar");
    expect(email).toHaveValue("ada@ex.com.ar");
    expect(phone).toHaveValue("5551112222");
  });
});
