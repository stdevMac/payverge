/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import SetupChecklist, { type SetupStep } from "./SetupChecklist";

function makeSteps(
  over: Partial<Record<string, () => void>> = {},
): SetupStep[] {
  return [
    {
      key: "positions",
      title: "Add positions",
      description: "Roles your team works",
      done: true,
    },
    {
      key: "invite",
      title: "Invite your team",
      description: "Send invites",
      done: false,
      actionLabel: "Invite",
      onAction: over.invite,
    },
    {
      key: "schedule",
      title: "Build the week",
      description: "Lay out shifts",
      done: false,
      actionLabel: "Go to schedule",
      onAction: over.schedule,
    },
  ];
}

describe("SetupChecklist", () => {
  it("renders the title and every step", () => {
    render(
      <SetupChecklist
        title="Set up your team"
        subtitle="Three quick steps"
        steps={makeSteps()}
      />,
    );
    expect(screen.getByText("Set up your team")).toBeTruthy();
    expect(screen.getByText("Add positions")).toBeTruthy();
    expect(screen.getByText("Invite your team")).toBeTruthy();
    expect(screen.getByText("Build the week")).toBeTruthy();
  });

  it("shows an action button only on the first not-done step", () => {
    render(
      <SetupChecklist
        title="t"
        subtitle="s"
        steps={makeSteps({ invite: () => {}, schedule: () => {} })}
      />,
    );
    // First incomplete step is "Invite your team" → its button shows.
    expect(screen.getByRole("button", { name: "Invite" })).toBeTruthy();
    // The later incomplete step ("Build the week") stays passive — no button.
    expect(screen.queryByRole("button", { name: "Go to schedule" })).toBeNull();
  });

  it("fires onAction for the current step", () => {
    const invite = jest.fn();
    render(
      <SetupChecklist title="t" subtitle="s" steps={makeSteps({ invite })} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Invite" }));
    expect(invite).toHaveBeenCalledTimes(1);
  });
});
