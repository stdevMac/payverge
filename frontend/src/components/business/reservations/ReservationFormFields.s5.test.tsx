/** @jest-environment jsdom */
/**
 * L1-16 — Reservas phone: required, rejects letters, no native type=email bubble
 * on the email field; controlled NextUI isInvalid path.
 */
import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  ReservationFormFields,
  type ReservationFormFieldLabels,
} from "./ReservationFormFields";
import type { CreateReservationRequest } from "@/api/reservations";

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

const baseForm: CreateReservationRequest = {
  customer_name: "Ada",
  customer_phone: "",
  customer_email: "",
  party_size: 2,
  reservation_time: "2026-08-10T19:00",
  duration: 90,
  special_requests: "",
  notes: "",
  table_id: undefined,
};

function Harness({
  showRequiredErrors = false,
  initialPhone = "",
  initialEmail = "",
}: {
  showRequiredErrors?: boolean;
  initialPhone?: string;
  initialEmail?: string;
}) {
  const [form, setForm] = useState<CreateReservationRequest>({
    ...baseForm,
    customer_phone: initialPhone,
    customer_email: initialEmail,
  });
  return (
    <>
      <ReservationFormFields
        formData={form}
        onChange={(next) => setForm(next)}
        labels={labels}
        timeEntryMode="business"
        onTimeEntryModeChange={jest.fn()}
        businessTimeZone="UTC"
        deviceTimeZone="UTC"
        tableOptions={[]}
        occupancyOverrideAcknowledged={false}
        onOccupancyOverrideChange={jest.fn()}
        showRequiredErrors={showRequiredErrors}
      />
      <output data-testid="harness-phone-state">{form.customer_phone}</output>
      <output data-testid="harness-email-state">{form.customer_email}</output>
    </>
  );
}

describe("L1-16 ReservationFormFields phone validation", () => {
  it("marks phone required and uses type=tel (not native email on phone)", () => {
    render(<Harness />);
    const phone = screen.getByTestId("reservation-customer-phone");
    expect(phone).toHaveAttribute("type", "tel");
    expect(phone).toHaveAttribute("inputmode", "tel");
    // Required: empty + showRequiredErrors surfaces the required message.
  });

  it("shows localized invalid error for garbage phone abc", () => {
    render(<Harness initialPhone="abc" />);
    expect(
      screen.getByText("Enter a valid phone number (digits only, at least 7)."),
    ).toBeInTheDocument();
  });

  it("shows required error when submit attempted with empty phone", () => {
    render(<Harness showRequiredErrors />);
    expect(screen.getByText("Phone number is required.")).toBeInTheDocument();
  });

  it("email field is type=text (no Chrome English type=email bubble)", () => {
    render(<Harness />);
    const email = screen.getByLabelText("Email");
    expect(email).toHaveAttribute("type", "text");
    expect(email).toHaveAttribute("inputmode", "email");
  });

  it("clears invalid state when a valid phone is typed", () => {
    render(<Harness initialPhone="abc" />);
    expect(
      screen.getByText("Enter a valid phone number (digits only, at least 7)."),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("reservation-customer-phone"), {
      target: { value: "5551112222" },
    });
    expect(
      screen.queryByText(
        "Enter a valid phone number (digits only, at least 7).",
      ),
    ).not.toBeInTheDocument();
  });

  it("retains every character when a multi-digit phone is typed sequentially", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const phone = screen.getByTestId("reservation-customer-phone");
    const typed = "5551112222";

    for (let i = 0; i < typed.length; i += 1) {
      await user.type(phone, typed[i], {
        initialSelectionStart: i,
        initialSelectionEnd: i,
      });
      const soFar = typed.slice(0, i + 1);
      expect(phone).toHaveValue(soFar);
      expect(screen.getByTestId("harness-phone-state")).toHaveTextContent(
        soFar,
        { normalizeWhitespace: false },
      );
    }
  });

  it("retains a sequentially typed email address", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const email = screen.getByLabelText("Email");
    const typed = "ada@example.com";

    await user.type(email, typed);
    expect(email).toHaveValue(typed);
    expect(screen.getByTestId("harness-email-state")).toHaveTextContent(typed);
  });

  it("shows the invalid phone error after sequential short input and clears it once valid", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const phone = screen.getByTestId("reservation-customer-phone");

    await user.type(phone, "123456");
    expect(phone).toHaveValue("123456");
    expect(
      screen.getByText("Enter a valid phone number (digits only, at least 7)."),
    ).toBeInTheDocument();

    await user.type(phone, "7");
    expect(phone).toHaveValue("1234567");
    expect(
      screen.queryByText(
        "Enter a valid phone number (digits only, at least 7).",
      ),
    ).not.toBeInTheDocument();
  });

  it("composes phone then email patches without dropping the phone (stale formData)", () => {
    const updates: CreateReservationRequest[] = [];
    render(
      <ReservationFormFields
        formData={baseForm}
        onChange={(next) => updates.push(next)}
        labels={labels}
        timeEntryMode="business"
        onTimeEntryModeChange={jest.fn()}
        businessTimeZone="UTC"
        deviceTimeZone="UTC"
        tableOptions={[]}
        occupancyOverrideAcknowledged={false}
        onOccupancyOverrideChange={jest.fn()}
      />,
    );

    fireEvent.change(screen.getByTestId("reservation-customer-phone"), {
      target: { value: "5551112222" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "ada@example.com" },
    });

    expect(updates).toHaveLength(2);
    expect(updates[1].customer_phone).toBe("5551112222");
    expect(updates[1].customer_email).toBe("ada@example.com");
  });

  it("retains appended digits when editing an existing phone", async () => {
    const user = userEvent.setup();
    render(<Harness initialPhone="5551112" />);
    const phone = screen.getByTestId("reservation-customer-phone");
    expect(phone).toHaveValue("5551112");
    await user.type(phone, "222");
    expect(phone).toHaveValue("5551112222");
    expect(screen.getByTestId("harness-phone-state")).toHaveTextContent(
      "5551112222",
    );
  });
});
