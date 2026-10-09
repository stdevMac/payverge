/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { AnimatedBadge } from "../AnimatedBadge";

describe("AnimatedBadge", () => {
  it("renders nothing for null, undefined, and zero counts", () => {
    const { rerender, container } = render(
      <AnimatedBadge count={null} label="Orders" />,
    );
    expect(container.textContent).toBe("");

    rerender(<AnimatedBadge count={undefined} label="Orders" />);
    expect(container.textContent).toBe("");

    rerender(<AnimatedBadge count={0} label="Orders" />);
    expect(container.textContent).toBe("");
  });

  it("caps large values while keeping the full accessible label", () => {
    render(<AnimatedBadge count={14} label="Kitchen orders" />);

    expect(screen.getByText("9+")).toBeTruthy();
    expect(screen.getByLabelText("14 Kitchen orders")).toBeTruthy();
  });

  // L2-16: history/results badges must show the exact total (same source as
  // the header results count). Default 9+ cap is fine for rail badges, not
  // for filter result totals that can be 34 or 234.
  it("shows the exact high count when cap is null (unlimited)", () => {
    render(<AnimatedBadge count={234} label="History" cap={null} />);

    expect(screen.getByText("234")).toBeTruthy();
    expect(screen.queryByText("9+")).toBeNull();
    expect(screen.getByLabelText("234 History")).toBeTruthy();
  });

  it("shows the exact high count when cap is Infinity", () => {
    render(<AnimatedBadge count={34} label="Results" cap={Infinity} />);

    expect(screen.getByText("34")).toBeTruthy();
    expect(screen.queryByText("9+")).toBeNull();
  });

  it("still caps when an explicit finite cap is provided", () => {
    render(<AnimatedBadge count={120} label="Alerts" cap={99} />);

    expect(screen.getByText("99+")).toBeTruthy();
    expect(screen.getByLabelText("120 Alerts")).toBeTruthy();
  });
});
