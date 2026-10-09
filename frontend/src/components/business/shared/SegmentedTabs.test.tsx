/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { Users } from "lucide-react";
import SegmentedTabs from "./SegmentedTabs";

const tabs = [
  { key: "people", label: "People", icon: Users },
  { key: "positions", label: "Positions" },
  { key: "communication", label: "Communication", badge: 3 },
];

describe("SegmentedTabs", () => {
  it("marks the active tab with aria-selected and the rest unselected", () => {
    render(
      <SegmentedTabs tabs={tabs} activeKey="positions" onChange={() => {}} />,
    );

    expect(screen.getByRole("tab", { name: /Positions/ }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: /People/ }).getAttribute("aria-selected")).toBe("false");
  });

  it("fires onChange with the clicked tab key", () => {
    const onChange = jest.fn();
    render(
      <SegmentedTabs tabs={tabs} activeKey="people" onChange={onChange} />,
    );

    fireEvent.click(screen.getByRole("tab", { name: /Communication/ }));
    expect(onChange).toHaveBeenCalledWith("communication");
  });

  it("renders a count badge only when badge > 0", () => {
    render(
      <SegmentedTabs tabs={tabs} activeKey="people" onChange={() => {}} />,
    );
    // communication has badge 3; people/positions have none.
    expect(screen.getByText("3")).toBeTruthy();
  });
});
