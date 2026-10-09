/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HoursSection } from "../HoursSection";

const t = (k: string) => k;

const baseSettings = {
  delivery_hours_same_as_business: true,
  delivery_start_time: "",
  delivery_end_time: "",
};

describe("HoursSection", () => {
  it("renders card title", () => {
    render(
      <HoursSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(screen.getByText("focused.hours.cardTitle")).toBeInTheDocument();
  });

  it("hides time pickers when same_as_business is true", () => {
    render(
      <HoursSection
        settings={{ ...baseSettings, delivery_hours_same_as_business: true }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    // time inputs should not be in the DOM
    expect(screen.queryByLabelText("focused.hours.startTime")).toBeNull();
    expect(screen.queryByLabelText("focused.hours.endTime")).toBeNull();
  });

  it("shows time pickers when same_as_business is false", () => {
    render(
      <HoursSection
        settings={{
          ...baseSettings,
          delivery_hours_same_as_business: false,
          delivery_start_time: "09:00",
          delivery_end_time: "21:00",
        }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    // Both time inputs should be visible
    const inputs = screen.getAllByDisplayValue(/.*/);
    const timeInputs = inputs.filter(
      (el) => (el as HTMLInputElement).type === "time",
    );
    expect(timeInputs.length).toBe(2);
  });

  it("fires onChange when toggle is clicked", async () => {
    const user = userEvent.setup();
    const onChange = jest.fn();
    render(
      <HoursSection
        settings={baseSettings}
        onChange={onChange}
        tString={t}
      />,
    );
    // The Switch renders a checkbox input
    const toggle = screen.getByRole("switch");
    await user.click(toggle);
    expect(onChange).toHaveBeenCalled();
  });
});
