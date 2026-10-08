/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import StaffBottomNav, { type StaffBottomNavLabels } from "./StaffBottomNav";

const labels: StaffBottomNavLabels = {
  label: "Staff navigation",
  today: "Today",
  schedule: "Schedule",
  chat: "Chat",
  more: "More",
};

test("renders four tabs and fires onChange with the tab key", () => {
  const onChange = jest.fn();
  render(<StaffBottomNav active="today" onChange={onChange} labels={labels} />);
  fireEvent.click(screen.getByRole("button", { name: "More" }));
  expect(onChange).toHaveBeenCalledWith("more");
});

test("shows an attention dot only on badged tabs", () => {
  render(
    <StaffBottomNav
      active="today"
      onChange={jest.fn()}
      labels={labels}
      badgedTabs={{ more: true }}
    />,
  );
  expect(screen.getByTestId("nav-dot-more")).toBeInTheDocument();
  expect(screen.queryByTestId("nav-dot-today")).not.toBeInTheDocument();
  expect(screen.queryByTestId("nav-dot-chat")).not.toBeInTheDocument();
});

test("shows no dots when no tabs are badged", () => {
  render(<StaffBottomNav active="more" onChange={jest.fn()} labels={labels} />);
  expect(screen.queryByTestId("nav-dot-more")).not.toBeInTheDocument();
});
