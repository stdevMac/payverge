/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";

import { ReservationTimeField } from "./ReservationTimeField";

const labels = {
  entryMode: "Entry mode",
  businessTime: "Business time",
  deviceTime: "Device time",
  dateTime: "Date and time",
  conversionPreview: "Conversion",
};

function renderField(props: Partial<Parameters<typeof ReservationTimeField>[0]> = {}) {
  return render(
    <ReservationTimeField
      value=""
      mode="business"
      businessTimeZone="America/Argentina/Buenos_Aires"
      deviceTimeZone="America/Argentina/Buenos_Aires"
      labels={labels}
      onValueChange={() => {}}
      onModeChange={() => {}}
      {...props}
    />,
  );
}

function todayInZone(timeZone: string): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
}

// L1-1: the create form must floor the picker at today so a past date cannot
// even be typed in by accident; the backend independently rejects elapsed
// slots. Edit mode carries no floor — existing (possibly past) reservations
// must stay editable without the picker flagging their stored time invalid.
describe("ReservationTimeField past-date floor", () => {
  it("carries a min of today (entry zone) when enforceTodayMin is set", () => {
    renderField({ enforceTodayMin: true });
    const input = screen.getByLabelText("Date and time");
    expect(input).toHaveAttribute(
      "min",
      `${todayInZone("America/Argentina/Buenos_Aires")}T00:00`,
    );
  });

  it("carries no min without enforceTodayMin (edit mode)", () => {
    renderField();
    const input = screen.getByLabelText("Date and time");
    expect(input).not.toHaveAttribute("min");
  });
});
