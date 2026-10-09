/** @jest-environment jsdom */
/**
 * Task 6.2 — Director's Console empty state.
 *
 * When an operator opens a thread that has no messages yet (e.g. the
 * "New chat" flow), the existing dashed-border placeholder reads as
 * empty/broken. The empty state instead frames the moment as an
 * invitation: a Sofia avatar, an open-ended prompt, and clickable
 * suggestion chips that kick off a new thread.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import EmptyState from "../EmptyState";

describe("Director empty state", () => {
  it("renders Sofia avatar + prompt + suggestion chips", () => {
    render(<EmptyState suggestions={["Plan A", "Plan B"]} onPick={jest.fn()} />);
    expect(
      screen.getByText(/What would you like to focus on/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Plan A" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Plan B" })).toBeInTheDocument();
  });

  it("invokes onPick with the suggestion text when a chip is clicked", () => {
    const onPick = jest.fn();
    render(<EmptyState suggestions={["Plan A"]} onPick={onPick} />);
    fireEvent.click(screen.getByRole("button", { name: "Plan A" }));
    expect(onPick).toHaveBeenCalledWith("Plan A");
  });
});
