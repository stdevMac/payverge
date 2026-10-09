/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import PeriodTabs from "./PeriodTabs";

const OPTIONS = [
  { key: "day", label: "Day" },
  { key: "week", label: "Week" },
  { key: "month", label: "Month" },
];

describe("PeriodTabs", () => {
  it("renders one tab per option", () => {
    render(
      <PeriodTabs
        options={OPTIONS}
        value="week"
        onChange={jest.fn()}
        ariaLabel="Select period"
      />,
    );
    expect(screen.getAllByRole("tab")).toHaveLength(3);
    expect(screen.getByRole("tab", { name: "Day" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Week" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Month" })).toBeInTheDocument();
  });

  it("marks the value option as selected", () => {
    render(
      <PeriodTabs
        options={OPTIONS}
        value="week"
        onChange={jest.fn()}
        ariaLabel="Select period"
      />,
    );
    expect(screen.getByRole("tab", { name: "Week" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: "Day" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
  });

  it("fires onChange with the option key when a tab is clicked", () => {
    const onChange = jest.fn();
    render(
      <PeriodTabs
        options={OPTIONS}
        value="week"
        onChange={onChange}
        ariaLabel="Select period"
      />,
    );
    fireEvent.click(screen.getByRole("tab", { name: "Month" }));
    expect(onChange).toHaveBeenCalledWith("month");
  });
});
