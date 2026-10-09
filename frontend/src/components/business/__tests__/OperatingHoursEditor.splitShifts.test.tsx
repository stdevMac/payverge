/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import OperatingHoursEditor from "@/components/business/OperatingHoursEditor";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/components/business/operatingHoursDayName", () => ({
  getDayName: (d: number) => `Day ${d}`,
}));

type Hour = {
  day_of_week: number;
  open_time: string;
  close_time: string;
  is_closed: boolean;
};

describe("OperatingHoursEditor — split shifts", () => {
  it("loads multiple open/close ranges for the same day", () => {
    const hours: Hour[] = [
      { day_of_week: 1, open_time: "11:00", close_time: "15:00", is_closed: false },
      { day_of_week: 1, open_time: "19:00", close_time: "23:00", is_closed: false },
    ];
    render(
      <OperatingHoursEditor
        businessId={1}
        hours={hours as any}
        onHoursChange={jest.fn()}
      />,
    );

    const openInputs = screen.getAllByLabelText(/opens/i) as HTMLInputElement[];
    const closeInputs = screen.getAllByLabelText(
      /closes/i,
    ) as HTMLInputElement[];
    const openValues = openInputs.map((el) => el.value);
    const closeValues = closeInputs.map((el) => el.value);
    expect(openValues).toEqual(expect.arrayContaining(["11:00", "19:00"]));
    expect(closeValues).toEqual(expect.arrayContaining(["15:00", "23:00"]));
  });

  it("emits a second period when the operator adds a split shift", async () => {
    const onHoursChange = jest.fn();
    // Seed every day closed except Monday with one period so the only open
    // "add second period" control is Monday's.
    const hours: Hour[] = [
      { day_of_week: 0, open_time: "09:00", close_time: "17:00", is_closed: true },
      { day_of_week: 1, open_time: "11:00", close_time: "15:00", is_closed: false },
      { day_of_week: 2, open_time: "09:00", close_time: "17:00", is_closed: true },
      { day_of_week: 3, open_time: "09:00", close_time: "17:00", is_closed: true },
      { day_of_week: 4, open_time: "09:00", close_time: "17:00", is_closed: true },
      { day_of_week: 5, open_time: "09:00", close_time: "17:00", is_closed: true },
      { day_of_week: 6, open_time: "09:00", close_time: "17:00", is_closed: true },
    ];
    render(
      <OperatingHoursEditor
        businessId={1}
        hours={hours as any}
        onHoursChange={onHoursChange}
      />,
    );

    const addButton = screen.getByRole("button", {
      name: /add (a )?second period/i,
    });
    fireEvent.click(addButton);

    await waitFor(() => expect(onHoursChange).toHaveBeenCalled());
    const emitted = onHoursChange.mock.calls.at(-1)[0] as Hour[];
    const monday = emitted.filter((h) => h.day_of_week === 1 && !h.is_closed);
    expect(monday.length).toBe(2);
    expect(monday.map((h) => h.open_time).sort()).toEqual(["11:00", "19:00"]);
  });
});
